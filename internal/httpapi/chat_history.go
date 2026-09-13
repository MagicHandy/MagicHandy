package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	request, err := parseChatHistoryRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.SessionID == "" {
		var err error
		request.SessionID, err = s.chatLog.ActiveSessionIDContext(r.Context())
		if err != nil {
			s.writeChatStorageError(w, err)
			return
		}
	}
	// Autopilot attaches speech after its message commit under this lock. A page
	// must not acknowledge past that message before its optional speech ID exists.
	if err := s.chatSpeechMu.Lock(ctx); err != nil {
		return
	}
	page, err := s.chatLog.ReadMessagePageContext(r.Context(), request)
	if err == nil {
		hostAdministration := s.capabilities(r).ConfigureHost
		for index := range page.Messages {
			if !hostAdministration {
				page.Messages[index].Diagnostics = nil
				page.Messages[index].DiagnosticsOmitted = false
				page.Messages[index].ClientID = ""
			}
			if id := s.chatSpeechRequests[page.Messages[index].Seq]; len(id) <= 128 {
				page.Messages[index].SpeechRequestID = id
			}
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
	data, err := json.Marshal(struct {
		chat.MessagePage
		ServerEpoch string `json:"server_epoch"`
	}{page, s.controller.epoch})
	if err != nil || len(data)+1 > chat.MessagePageMaxBytes {
		s.writeChatStorageError(w, errors.New("chat page encoding exceeded its budget"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = writeContextBytes(ctx, w, append(data, '\n'))
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
