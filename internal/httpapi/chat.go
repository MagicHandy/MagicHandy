package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
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

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var body chatStreamRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	settings, _ := s.store.Snapshot()
	provider, err := s.newLLMProvider(settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}

	setSSEHeaders(w)
	service := chat.Service{
		Provider:    provider,
		PromptSetID: settings.LLM.PromptSet,
		Model:       settings.LLM.Model,
	}
	emit := func(event string, payload any) error {
		return writeSSE(w, event, payload)
	}

	if err := emit("status", map[string]any{
		"state":      "streaming",
		"provider":   settings.LLM.Provider,
		"model":      settings.LLM.Model,
		"prompt_set": settings.LLM.PromptSet,
	}); err != nil {
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

	dispatch, motionErr := s.dispatchChatMotion(r.Context(), result.Response.Motion)
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

func (s *Server) dispatchChatMotion(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	if command == nil || command.Action == "" || command.Action == chat.MotionActionNone {
		return chatMotionDispatch{Action: chat.MotionActionNone}, nil
	}

	switch command.Action {
	case chat.MotionActionStart:
		engine, err := s.motionEngineForStart()
		if err != nil {
			return chatMotionDispatch{Action: command.Action}, err
		}
		current := engine.Snapshot()
		target := chatMotionTarget(command, current)
		if current.Running {
			state, err := engine.ApplyTarget(ctx, target, "chat_start_retarget")
			return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
		}
		settings, _ := s.store.Snapshot()
		state, err := engine.Start(ctx, target, settings.Motion)
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
	case chat.MotionActionTarget:
		engine := s.currentMotionEngine()
		if engine == nil || !engine.Snapshot().Running {
			return chatMotionDispatch{Action: command.Action}, errors.New("motion is not running")
		}
		current := engine.Snapshot()
		state, err := engine.ApplyTarget(ctx, chatMotionTarget(command, current), "chat_target")
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
	case chat.MotionActionStop:
		engine := s.currentMotionEngine()
		if engine == nil {
			return chatMotionDispatch{Applied: true, Action: command.Action}, nil
		}
		state, err := engine.Stop(ctx, "chat_stop")
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
	default:
		return chatMotionDispatch{Action: command.Action}, fmt.Errorf("unsupported motion action %q", command.Action)
	}
}

func chatMotionTarget(command *chat.MotionCommand, current motion.ActiveMotionState) motion.MotionTarget {
	patternID := motion.PatternID(command.PatternID)
	speedPercent := 0
	if command.SpeedPercent != nil {
		speedPercent = *command.SpeedPercent
	}
	if current.Running {
		if patternID == "" {
			patternID = current.Target.PatternID
		}
		if speedPercent == 0 {
			speedPercent = current.Target.SpeedPercent
		}
	}
	return motion.MotionTarget{
		Label:        "Chat",
		Source:       "chat",
		PatternID:    patternID,
		SpeedPercent: speedPercent,
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
