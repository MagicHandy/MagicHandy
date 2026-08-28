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

func (s *Server) handleChatSessions(w http.ResponseWriter, r *http.Request) {
	s.writeChatSessions(w, r, http.StatusOK)
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
	s.writeChatSessions(w, r, http.StatusCreated)
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
	s.writeChatSessions(w, r, http.StatusOK)
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
	s.writeChatSessions(w, r, http.StatusOK)
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
	s.writeChatSessions(w, r, http.StatusOK)
}

type chatSessionView struct {
	chat.Session
	PersonaName string `json:"persona_name"`
}

func (s *Server) writeChatSessions(w http.ResponseWriter, r *http.Request, status int) {
	sessions, err := s.chatLog.Sessions()
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	personas, err := s.personas.List(r.Context())
	if err != nil {
		s.writePersonalizationStorageError(w, "persona", err)
		return
	}
	personaNames := make(map[string]string, len(personas))
	for _, item := range personas {
		personaNames[item.ID] = item.Name
	}
	activeID := ""
	views := make([]chatSessionView, 0, len(sessions))
	for _, session := range sessions {
		if session.Active {
			activeID = session.ID
		}
		name := personaNames[session.PersonaID]
		if name == "" {
			name = defaultPersonaName
		}
		views = append(views, chatSessionView{Session: session, PersonaName: name})
	}
	writeJSON(w, status, map[string]any{
		"active_session_id": activeID,
		"sessions":          views,
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
