package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

// Read positions belong to a login and declared browser, independently of the
// controller. The derived database key contains no token or private session ID.
// Same-login tabs share their session's security boundary.
func chatCursorClientID(r *http.Request) string {
	clientID := clientIDFromRequest(r)
	if clientID == "" {
		return ""
	}
	if session, ok := authenticatedSession(r); ok {
		identity := sha256.Sum256([]byte("chat-cursor-v1\x00" + session.session.Key + "\x00" + clientID))
		return "login:" + hex.EncodeToString(identity[:])
	}
	return clientID
}

// Reads do not advance cursors. The committed snapshot supplies an independent
// revision cursor, because a pending reply can commit below the highest seq.
func (s *Server) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	request := chat.MessagePageRequest{SessionID: strings.TrimSpace(r.URL.Query().Get("session_id")), ClientID: chatCursorClientID(r)}
	if request.SessionID == "" {
		var err error
		request.SessionID, err = s.chatLog.ActiveSessionIDContext(r.Context())
		if err != nil {
			s.writeChatStorageError(w, err)
			return
		}
	}
	for key, target := range map[string]*int64{"after": &request.AfterSequence, "after_revision": nil} {
		if value := r.URL.Query().Get(key); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				writeError(w, http.StatusBadRequest, errors.New(key+" must be a non-negative integer"))
				return
			}
			if target != nil {
				*target = parsed
			} else {
				request.AfterRevision = &parsed
			}
		}
	}
	if request.AfterSequence > 0 && request.AfterRevision != nil {
		writeError(w, http.StatusBadRequest, errors.New("use either after or after_revision"))
		return
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, errors.New("limit must be a positive integer"))
			return
		}
		request.Limit = parsed
	}
	// Autopilot attaches speech after its message commit under this lock. A page
	// must not acknowledge past that message before its optional speech ID exists.
	s.chatSpeechMu.Lock()
	page, err := s.chatLog.ReadMessagePageContext(r.Context(), request)
	if err == nil {
		for index := range page.Messages {
			page.Messages[index].SpeechRequestID = s.chatSpeechRequests[page.Messages[index].Seq]
		}
	}
	s.chatSpeechMu.Unlock()
	if err != nil {
		if errors.Is(err, chat.ErrChatSessionNotFound) {
			s.writeChatSessionError(w, err)
		} else {
			s.writeChatStorageError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, struct {
		chat.MessagePage
		ServerEpoch string `json:"server_epoch"`
	}{page, s.controller.epoch})
}

func (s *Server) handleChatCursor(w http.ResponseWriter, r *http.Request) {
	clientID := chatCursorClientID(r)
	if clientID == "" {
		writeError(w, http.StatusBadRequest, errors.New("a client id header is required to advance a chat cursor"))
		return
	}
	var body struct {
		SessionID   string `json:"session_id,omitempty"`
		Seq         int64  `json:"seq"`
		Revision    *int64 `json:"revision,omitempty"`
		ServerEpoch string `json:"server_epoch,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Revision != nil && (body.ServerEpoch != s.controller.epoch || *body.Revision < 0) {
		writeError(w, http.StatusConflict, errors.New("chat recovery metadata expired; reload the conversation"))
		return
	}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		var err error
		sessionID, err = s.chatLog.ActiveSessionIDContext(r.Context())
		if err != nil {
			s.writeChatStorageError(w, err)
			return
		}
	}
	cursor, err := s.chatLog.AdvanceReadCursorContext(r.Context(), clientID, sessionID, body.Seq, body.Revision)
	if err != nil {
		if errors.Is(err, chat.ErrChatSessionNotFound) {
			s.writeChatSessionError(w, err)
		} else {
			s.writeChatStorageError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": cursor.Sequence, "cursor_revision": cursor.Revision, "session_id": sessionID})
}
