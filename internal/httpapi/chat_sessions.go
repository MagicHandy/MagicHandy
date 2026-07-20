package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func (s *Server) chatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/chat/stream", s.handleChatStream)
	mux.HandleFunc("GET /api/chat/sessions", s.handleChatSessions)
	mux.HandleFunc("POST /api/chat/sessions", s.handleCreateChatSession)
	mux.HandleFunc("PUT /api/chat/sessions/{id}/active", s.handleActivateChatSession)
	mux.HandleFunc("PUT /api/chat/sessions/{id}/save", s.handleSaveChatSession)
	mux.HandleFunc("DELETE /api/chat/sessions/{id}", s.handleDeleteChatSession)
	mux.HandleFunc("GET /api/chat/messages", s.handleChatMessages)
	mux.HandleFunc("POST /api/chat/cursor", s.handleChatCursor)
}

func (s *Server) handleChatSessions(w http.ResponseWriter, _ *http.Request) {
	s.writeChatSessions(w, http.StatusOK)
}

func (s *Server) handleCreateChatSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	if s.chatGenerationActive() {
		writeError(w, http.StatusConflict, errors.New("wait for the active reply to finish before starting a new chat"))
		return
	}
	if s.autopilotActive() {
		writeError(w, http.StatusConflict, errors.New("stop Autopilot before starting a new chat"))
		return
	}
	var body struct {
		DiscardCurrentUnsaved bool `json:"discard_current_unsaved"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.chatLog.CreateSession(body.DiscardCurrentUnsaved); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
	s.writeChatSessions(w, http.StatusCreated)
}

func (s *Server) handleActivateChatSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	if s.chatGenerationActive() {
		writeError(w, http.StatusConflict, errors.New("wait for the active reply to finish before switching chats"))
		return
	}
	if s.autopilotActive() {
		writeError(w, http.StatusConflict, errors.New("stop Autopilot before switching chats"))
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("a chat session id is required"))
		return
	}
	discard := false
	if value := strings.TrimSpace(r.URL.Query().Get("discard_current_unsaved")); value != "" {
		var err error
		discard, err = strconv.ParseBool(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("discard_current_unsaved must be true or false"))
			return
		}
	}
	if _, err := s.chatLog.ActivateSession(id, discard); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
	s.writeChatSessions(w, http.StatusOK)
}

func (s *Server) handleSaveChatSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("a chat session id is required"))
		return
	}
	if _, err := s.chatLog.SaveSession(id); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
	s.writeChatSessions(w, http.StatusOK)
}

func (s *Server) handleDeleteChatSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("a chat session id is required"))
		return
	}
	if err := s.chatLog.DeleteSession(id); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
	s.writeChatSessions(w, http.StatusOK)
}

func (s *Server) writeChatSessions(w http.ResponseWriter, status int) {
	sessions, err := s.chatLog.Sessions()
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	activeID := ""
	for _, session := range sessions {
		if session.Active {
			activeID = session.ID
			break
		}
	}
	if sessions == nil {
		sessions = []chat.Session{}
	}
	writeJSON(w, status, map[string]any{
		"active_session_id": activeID,
		"sessions":          sessions,
	})
}

func (s *Server) writeChatSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, chat.ErrChatSessionNotFound):
		writeError(w, http.StatusNotFound, errors.New("chat session not found"))
	case errors.Is(err, chat.ErrActiveSessionDelete):
		writeError(w, http.StatusConflict, errors.New("switch away from a chat before deleting it"))
	case errors.Is(err, chat.ErrUnsavedSessionConflict):
		writeError(w, http.StatusConflict, errors.New("save or discard the active unsaved chat before continuing"))
	default:
		s.writeChatStorageError(w, err)
	}
}

func (s *Server) chatGenerationActive() bool {
	s.chatCancelMu.Lock()
	defer s.chatCancelMu.Unlock()
	return len(s.chatCancels) > 0
}

func (s *Server) autopilotActive() bool {
	return s.modes != nil && s.modes.Status().Mode == modes.ModeAutopilot
}
