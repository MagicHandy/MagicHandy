package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type chatMessageRecord struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

type lsoCompatRuntime struct {
	mu                 sync.RWMutex
	messages           []chatMessageRecord
	lastPersonaMessage string
}

func (s *Server) lsoStatusPayload() map[string]any {
	settings, _ := s.store.Snapshot()
	intiface := s.intifaceDiagnostics()

	playbackActive := false
	if engine := s.currentMotionEngine(); engine != nil {
		playbackActive = engine.Snapshot().Running
	}

	s.manualQueue.mu.Lock()
	mqCount := len(s.manualQueue.items)
	mqPlaying := s.manualQueue.playing
	mqPaused := s.manualQueue.paused
	s.manualQueue.mu.Unlock()

	llmConnected := false
	var llmError any = nil
	if provider, err := s.newLLMProvider(settings.LLM); err == nil {
		llmStatus := provider.Status(context.Background())
		llmConnected = llmStatus.Available
		if !llmConnected && llmStatus.Message != "" {
			llmError = llmStatus.Message
		}
	} else {
		llmError = err.Error()
	}

	appState, _ := s.store.DB().LoadAppState()
	personaRow, personaID, _ := s.activePersonaSnapshot()
	personaName := "MagicHandy"
	var personaAvatar any = nil
	if personaRow.ID != "" {
		personaName = personaRow.Name
		personaAvatar = s.personaAvatarURLFor(personaID)
	}

	payload := map[string]any{
		"service":              serviceName,
		"version":              s.version.Version,
		"device_transport":     mapOwnerToDeviceTransport(settings.Device.HSPDispatchOwner),
		"intiface_connected":   intiface.Connected,
		"intiface_url":         settings.Device.IntifaceURL,
		"intiface_error":       intiface.Error,
		"device_label":         intiface.DeviceLabel,
		"device_connected":     intiface.DeviceReady,
		"use_mock":             false,
		"persona_id":           personaID,
		"persona_name":         personaName,
		"persona_avatar_url":   personaAvatar,
		"ollama_connected":     llmConnected,
		"ollama_error":         llmError,
		"ollama_model":         settings.LLM.Model,
		"ollama_url":           settings.LLM.LlamaCPPBaseURL,
		"operation_mode":       appState.OperationMode,
		"intensity":            50,
		"min_position":         settings.Motion.StrokeMinPercent,
		"max_position":         settings.Motion.StrokeMaxPercent,
		"buffer_sec":           0,
		"queue_preview":        []any{},
		"phase":                "idle",
		"playback_active":      playbackActive,
		"emergency_stop":       s.emergencyStopActive(),
		"chat_pending":         false,
		"planner_busy":         false,
		"auto_running":         false,
		"user_session_engaged": len(s.lsoCompat.messages) > 0,
		"manual_queue_count":   mqCount,
		"manual_queue_playing": mqPlaying,
		"manual_queue_paused":  mqPaused,
		"footer_status":        "MagicHandy unified",
		"ui":                   "embedded",
		"features": map[string]string{
			"motion":   "manual",
			"chat":     "llama.cpp",
			"library":  "sqlite",
			"intiface": "required",
		},
	}
	return payload
}

type intifaceDiag struct {
	Connected   bool
	DeviceReady bool
	Error       any
	DeviceLabel string
}

func (s *Server) intifaceDiagnostics() intifaceDiag {
	client := s.intifaceClient()
	if client == nil {
		return intifaceDiag{}
	}
	errVal := any(nil)
	if last := client.LastError(); last != "" {
		errVal = last
	}
	ready := client.Connected() && client.SelectedDeviceID() != ""
	return intifaceDiag{
		Connected:   client.Connected(),
		DeviceReady: ready,
		Error:       errVal,
		DeviceLabel: client.SelectedDeviceName(),
	}
}

func (s *Server) handleChatMessages(w http.ResponseWriter, _ *http.Request) {
	s.lsoCompat.mu.RLock()
	defer s.lsoCompat.mu.RUnlock()
	msgs := make([]chatMessageRecord, len(s.lsoCompat.messages))
	copy(msgs, s.lsoCompat.messages)
	writeJSON(w, http.StatusOK, map[string]any{
		"messages":             msgs,
		"last_persona_message": s.lsoCompat.lastPersonaMessage,
	})
}

type chatSendRequest struct {
	Text string `json:"text"`
}

func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	var body chatSendRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	text := trimSpace(body.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, errEmptyChatMessage)
		return
	}
	if isChatStopMessage(text) {
		s.stopAndClearMotionEngine(r.Context(), "chat_stop")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stopped": true})
		return
	}
	if !s.requireController(w, r) {
		return
	}

	s.appendChatMessage("user", text)

	settings, _ := s.store.Snapshot()
	provider, err := s.ensureLLMReady(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	prompt := s.chatPromptForRequest(settings)
	service := chat.Service{
		Provider: provider,
		Prompt:   prompt,
		Model:    settings.LLM.Model,
		Memories: s.personalization.memory.PromptTexts(),
	}

	history := s.chatHistoryForLLM()
	result, err := service.Complete(r.Context(), chat.Request{
		Message: text,
		History: history,
	}, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	if result.Malformed {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":        false,
			"error":     result.MalformedError,
			"malformed": true,
		})
		return
	}

	reply := result.Response.Reply
	s.appendChatMessage("assistant", reply)
	s.lsoCompat.mu.Lock()
	s.lsoCompat.lastPersonaMessage = reply
	s.lsoCompat.mu.Unlock()

	motionDispatch, motionErr := s.dispatchChatMotion(r.Context(), result.Response.Motion)
	response := map[string]any{
		"ok":      true,
		"reply":   reply,
		"pending": false,
		"stopped": false,
		"motion":  result.Response.Motion,
	}
	if motionDispatch.Applied || motionDispatch.Action != "" {
		response["motion_dispatch"] = motionDispatch
	}
	if motionErr != nil {
		response["motion_error"] = motionErr.Error()
		s.logger.Warn("chat motion dispatch failed", "error", motionErr, "action", result.Response.Motion)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) appendChatMessage(role, content string) {
	s.lsoCompat.mu.Lock()
	defer s.lsoCompat.mu.Unlock()
	s.lsoCompat.messages = append(s.lsoCompat.messages, chatMessageRecord{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if len(s.lsoCompat.messages) > 200 {
		s.lsoCompat.messages = s.lsoCompat.messages[len(s.lsoCompat.messages)-200:]
	}
}

func (s *Server) chatHistoryForLLM() []llm.Message {
	s.lsoCompat.mu.RLock()
	defer s.lsoCompat.mu.RUnlock()
	history := make([]llm.Message, 0, len(s.lsoCompat.messages))
	for _, msg := range s.lsoCompat.messages {
		history = append(history, llm.Message{Role: msg.Role, Content: msg.Content})
	}
	return history
}

var errEmptyChatMessage = errorString("chat message is required")

type errorString string

func (e errorString) Error() string { return string(e) }

func trimSpace(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[0] == '\n') {
		value = value[1:]
	}
	for len(value) > 0 {
		last := value[len(value)-1]
		if last != ' ' && last != '\t' && last != '\n' {
			break
		}
		value = value[:len(value)-1]
	}
	return value
}

func (s *Server) defaultDispatchOwner() string {
	settings, _ := s.store.Snapshot()
	if settings.Device.HSPDispatchOwner == "" {
		return config.DispatchOwnerIntiface
	}
	return settings.Device.HSPDispatchOwner
}
