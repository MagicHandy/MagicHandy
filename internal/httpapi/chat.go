package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type chatStreamRequest struct {
	Message string        `json:"message"`
	History []llm.Message `json:"history,omitempty"`
}

type chatMotionDispatch struct {
	Applied bool                     `json:"applied"`
	Action  string                   `json:"action,omitempty"`
	Engine  motion.ActiveMotionState `json:"engine,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

type sseEmitter func(string, any) error

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var body chatStreamRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	settings, _ := s.store.Snapshot()
	if isChatStopMessage(body.Message) {
		s.handleChatStopFastPath(w, r, settings)
		return
	}
	if !s.requireController(w, r) {
		return
	}

	provider, err := s.ensureLLMReady(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}

	prompt := s.chatPromptForRequest(settings)
	service := chat.Service{
		Provider:             provider,
		Prompt:               prompt,
		Model:                settings.LLM.Model,
		Memories:             s.personalization.memory.PromptTexts(),
		UserProfile:          settings.UserProfile,
		MotionGenerationMode: settings.Motion.MotionGenerationMode,
	}

	setSSEHeaders(w)
	emit := sseEmitter(func(event string, payload any) error {
		return writeSSE(w, event, payload)
	})

	if err := emit("status", map[string]any{
		"state":      "streaming",
		"provider":   settings.LLM.Provider,
		"model":      settings.LLM.Model,
		"prompt_set": prompt.ID,
	}); err != nil {
		return
	}

	if s.shouldUseDirectorChatMode(settings) {
		s.handleChatStreamDirector(w, r, body, settings, provider, emit)
		return
	}

	result, err := service.Complete(r.Context(), chat.Request{
		Message: body.Message,
		History: body.History,
	}, func(event chat.StreamEvent) error {
		switch event.Type {
		case "delta", "repair_delta":
			return emit(event.Type, map[string]any{
				"phase": event.Phase,
				"text":  event.Text,
			})
		case "malformed":
			return emit("malformed", map[string]any{
				"repaired":    false,
				"recoverable": true,
				"phase":       event.Phase,
				"error":       event.Error,
			})
		default:
			return nil
		}
	})
	s.emitChatCompletionResult(r, emit, result, err, settings)
}

func (s *Server) emitChatCompletionResult(r *http.Request, emit sseEmitter, result chat.Result, err error, settings config.Settings) {
	if err != nil {
		s.logger.Warn("chat stream failed", "provider", settings.LLM.Provider, "error", err)
		_ = emit("error", map[string]string{"message": err.Error()})
		_ = emit("done", map[string]any{"ok": false})
		return
	}
	if result.Malformed {
		_ = emit("malformed", map[string]any{
			"repaired":    false,
			"recoverable": false,
			"error":       result.MalformedError,
		})
		_ = emit("done", map[string]any{"ok": false, "malformed": true})
		return
	}
	if result.Repaired {
		_ = emit("malformed", map[string]any{
			"repaired":    true,
			"recoverable": false,
			"error":       result.MalformedError,
		})
	}

	if err := emit("message", map[string]any{
		"reply":             result.Response.Reply,
		"repaired":          result.Repaired,
		"initial_malformed": result.InitialMalformed,
		"motion":            result.Response.Motion,
	}); err != nil {
		return
	}

	dispatch, motionErr := s.dispatchChatMotionForResult(r.Context(), result.Response.Motion, settings)
	if motionErr != nil {
		dispatch.Error = motionErr.Error()
		s.logger.Warn("chat motion dispatch failed", "action", dispatch.Action, "error", motionErr)
	}
	if dispatch.Applied || dispatch.Error != "" {
		if err := emit("motion", dispatch); err != nil {
			return
		}
	}

	_ = emit("done", map[string]any{
		"ok":       motionErr == nil,
		"repaired": result.Repaired,
	})
}

func (s *Server) dispatchChatMotionForResult(
	ctx context.Context,
	command *chat.MotionCommand,
	settings config.Settings,
) (chatMotionDispatch, error) {
	if s.shouldUseChaoticChatMotion(command, settings) {
		return s.dispatchChatChaoticMotionAsync(ctx, command, settings), nil
	}
	return s.dispatchChatMotion(ctx, command)
}

func (s *Server) dispatchChatMotion(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	if err := s.ensureIntifaceDeviceForMotion(ctx); err != nil {
		return s.dispatchChatMotionIntifaceUnavailable(command, err)
	}
	if command == nil || command.Action == "" || command.Action == chat.MotionActionNone {
		return s.dispatchChatMotionNoActionOrLibrary(ctx, command)
	}

	settings, _ := s.store.Snapshot()
	switch command.Action {
	case chat.MotionActionStart:
		return s.dispatchChatMotionStart(ctx, command, settings)
	case chat.MotionActionTarget:
		return s.dispatchChatMotionTarget(ctx, command)
	case chat.MotionActionStop:
		return s.dispatchChatMotionStop(ctx, command)
	default:
		return chatMotionDispatch{Action: command.Action}, fmt.Errorf("unsupported motion action %q", command.Action)
	}
}

func (s *Server) dispatchChatMotionIntifaceUnavailable(command *chat.MotionCommand, err error) (chatMotionDispatch, error) {
	if command == nil || command.Action == "" || command.Action == chat.MotionActionNone {
		return chatMotionDispatch{Action: chat.MotionActionNone}, nil
	}
	return chatMotionDispatch{Action: command.Action, Error: err.Error()}, err
}

func (s *Server) dispatchChatMotionNoActionOrLibrary(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	if command == nil {
		return chatMotionDispatch{Action: chat.MotionActionNone}, nil
	}
	if strings.TrimSpace(command.LibraryBlockID) != "" {
		if err := s.enqueueChatLibraryBlock(command.LibraryBlockID); err != nil {
			return chatMotionDispatch{Action: chat.MotionActionNone}, err
		}
		return chatMotionDispatch{Applied: true, Action: chat.MotionActionNone}, nil
	}
	if strings.TrimSpace(command.PadraoID) != "" {
		blockID, err := s.resolveLibraryBlockByPatternTag(ctx, command.PadraoID)
		if err != nil {
			return chatMotionDispatch{Action: chat.MotionActionNone}, err
		}
		if err := s.enqueueChatLibraryBlock(blockID); err != nil {
			return chatMotionDispatch{Action: chat.MotionActionNone}, err
		}
		return chatMotionDispatch{Applied: true, Action: chat.MotionActionNone}, nil
	}
	return chatMotionDispatch{Action: chat.MotionActionNone}, nil
}

func (s *Server) dispatchChatMotionStart(ctx context.Context, command *chat.MotionCommand, settings config.Settings) (chatMotionDispatch, error) {
	engine, err := s.motionEngineForStart()
	if err != nil {
		return chatMotionDispatch{Action: command.Action}, err
	}
	current := engine.Snapshot()
	target := chatMotionTarget(command, current, settings.Motion.MotionGenerationMode)
	s.notifyChatTarget(target)
	if current.Running {
		state, err := engine.ApplyTarget(ctx, target, "chat_start_retarget")
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
	}
	state, err := engine.Start(ctx, target, settings.Motion)
	return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
}

func (s *Server) dispatchChatMotionTarget(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	engine := s.currentMotionEngine()
	if engine == nil || !engine.Snapshot().Running {
		return chatMotionDispatch{Action: command.Action}, errors.New("motion is not running")
	}
	current := engine.Snapshot()
	settings, _ := s.store.Snapshot()
	target := chatMotionTarget(command, current, settings.Motion.MotionGenerationMode)
	s.notifyChatTarget(target)
	state, err := engine.ApplyTarget(ctx, target, "chat_target")
	return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
}

func (s *Server) dispatchChatMotionStop(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	// A chat stop is a user stop: modes end and keepalive stands down.
	if s.modes != nil {
		s.modes.NotifyChatStop()
		s.modes.NotifyUserStop()
	}
	engine := s.currentMotionEngine()
	if engine == nil {
		return chatMotionDispatch{Applied: true, Action: command.Action}, nil
	}
	state, err := engine.Stop(ctx, "chat_stop")
	return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
}

func (s *Server) notifyChatTarget(target motion.MotionTarget) {
	if s.modes != nil {
		s.modes.NotifyChatTarget(target)
	}
}

func (s *Server) handleChatStopFastPath(w http.ResponseWriter, r *http.Request, settings config.Settings) {
	setSSEHeaders(w)
	emit := func(event string, payload any) error {
		return writeSSE(w, event, payload)
	}
	if err := emit("status", map[string]any{
		"state":    "deterministic_stop",
		"provider": settings.LLM.Provider,
		"model":    settings.LLM.Model,
	}); err != nil {
		return
	}
	command := &chat.MotionCommand{Action: chat.MotionActionStop}
	if err := emit("message", map[string]any{
		"reply":             "Stopping motion.",
		"repaired":          false,
		"initial_malformed": false,
		"motion":            command,
	}); err != nil {
		return
	}
	dispatch, motionErr := s.dispatchChatMotionForResult(r.Context(), command, settings)
	if motionErr != nil {
		dispatch.Error = motionErr.Error()
		s.logger.Warn("chat deterministic stop failed", "error", motionErr)
	}
	if err := emit("motion", dispatch); err != nil {
		return
	}
	_ = emit("done", map[string]any{
		"ok": motionErr == nil,
	})
}

func chatMotionTarget(command *chat.MotionCommand, current motion.ActiveMotionState, generationMode string) motion.MotionTarget {
	return chat.MotionTargetFromCommand(command, current, generationMode)
}

func isChatStopMessage(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	normalized = strings.Trim(normalized, " \t\r\n.!?")
	switch normalized {
	case "stop", "stop motion", "stop the motion", "pause", "pause motion", "pause the motion",
		"end", "end motion", "end the motion", "emergency stop":
		return true
	default:
		return false
	}
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeSSE(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode SSE payload: %w", err)
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
