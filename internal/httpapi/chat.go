package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/voice"
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
		s.handleChatStopFastPath(w, r, body.Message, settings.LLM.Provider, settings.LLM.Model)
		return
	}
	if !s.requireController(w, r) {
		return
	}

	provider, err := s.newLLMProvider(settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}

	setSSEHeaders(w)
	prompt, ok := s.personalization.prompts.Resolve(settings.LLM.PromptSet)
	if !ok {
		// A deleted or unknown selection falls back to the bundled default so
		// chat keeps working; the status event reports what actually ran.
		prompt, _ = chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	}
	service := chat.Service{
		Provider: provider,
		Prompt:   prompt,
		Model:    settings.LLM.Model,
		Memories: s.personalization.memory.PromptTexts(),
		Patterns: s.chatPatternChoices(),
	}
	emit := sseEmitter(func(event string, payload any) error {
		return writeSSE(w, event, payload)
	})

	// The user message enters the shared log once streaming actually starts;
	// its seq rides the status event so the sending client can track what it
	// has already displayed.
	userSeq := s.appendChatMessage(chat.MessageRoleUser, body.Message, clientIDFromRequest(r))

	if err := emit("status", map[string]any{
		"state":      "streaming",
		"provider":   settings.LLM.Provider,
		"model":      settings.LLM.Model,
		"prompt_set": prompt.ID,
		"user_seq":   userSeq,
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
	s.emitChatCompletionResult(r, emit, result, err, settings.LLM.Provider)
}

func (s *Server) emitChatCompletionResult(r *http.Request, emit sseEmitter, result chat.Result, err error, provider string) {
	if err != nil {
		s.logger.Warn("chat stream failed", "provider", provider, "error", err)
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

	// Lockstep delivery ordering (ADR 0003): the reply enters the shared log
	// (displayed) before any TTS enqueue, so a spoken reply is always also
	// shown. Error and malformed paths returned above — they never reach the
	// log or TTS.
	replySeq := s.appendChatMessage(chat.MessageRoleAssistant, result.Response.Reply, "")

	if err := emit("message", map[string]any{
		"reply":             result.Response.Reply,
		"repaired":          result.Repaired,
		"initial_malformed": result.InitialMalformed,
		"motion":            result.Response.Motion,
		"seq":               replySeq,
	}); err != nil {
		return
	}

	if speech := s.enqueueSpeech(result.Response.Reply); speech != nil {
		_ = emit("speech", map[string]any{"request_id": speech.ID})
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
		target := s.chatMotionTarget(command, current)
		s.notifyChatTarget(target)
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
		target := s.chatMotionTarget(command, current)
		s.notifyChatTarget(target)
		state, err := engine.ApplyTarget(ctx, target, "chat_target")
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
	case chat.MotionActionStop:
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
	default:
		return chatMotionDispatch{Action: command.Action}, fmt.Errorf("unsupported motion action %q", command.Action)
	}
}

func (s *Server) notifyChatTarget(target motion.MotionTarget) {
	if s.modes != nil {
		s.modes.NotifyChatTarget(target)
	}
}

func (s *Server) handleChatStopFastPath(w http.ResponseWriter, r *http.Request, message string, provider string, model string) {
	setSSEHeaders(w)
	emit := func(event string, payload any) error {
		return writeSSE(w, event, payload)
	}
	userSeq := s.appendChatMessage(chat.MessageRoleUser, message, clientIDFromRequest(r))
	if err := emit("status", map[string]any{
		"state":    "deterministic_stop",
		"provider": provider,
		"model":    model,
		"user_seq": userSeq,
	}); err != nil {
		return
	}
	// The deterministic reply is displayed, so it enters the shared log like
	// any other reply; a stop confirmation is deliberately never spoken —
	// physical Stop must not wait on TTS.
	replySeq := s.appendChatMessage(chat.MessageRoleAssistant, "Stopping motion.", "")
	command := &chat.MotionCommand{Action: chat.MotionActionStop}
	if err := emit("message", map[string]any{
		"reply":             "Stopping motion.",
		"repaired":          false,
		"initial_malformed": false,
		"motion":            command,
		"seq":               replySeq,
	}); err != nil {
		return
	}
	dispatch, motionErr := s.dispatchChatMotion(r.Context(), command)
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

// appendChatMessage adds one message to the shared log; a log failure is a
// diagnostics problem, never a chat outage (returns 0).
func (s *Server) appendChatMessage(role string, content string, clientID string) int64 {
	if s.chatLog == nil {
		return 0
	}
	seq, err := s.chatLog.Append(role, content, clientID)
	if err != nil {
		s.logger.Warn("chat log append failed", "role", role, "error", err)
		return 0
	}
	return seq
}

// enqueueSpeech submits the displayed reply to the TTS worker when the
// speak-replies setting is on. It runs strictly after the log append, and
// its failures stay in the voice request log — never in chat.
func (s *Server) enqueueSpeech(reply string) *voice.PendingRequest {
	settings, _ := s.store.Snapshot()
	if !settings.Voice.Enabled || !settings.Voice.SpeakReplies {
		return nil
	}
	pending, err := s.voice.Worker(voice.RoleTTS).Submit(voice.Request{
		Type: voice.RequestSpeak,
		Text: reply,
	})
	if err != nil {
		s.logger.Warn("TTS enqueue skipped", "error", err)
		return nil
	}
	s.voice.Track(pending)
	return pending
}

// chatState is the /api/state block other tabs poll for continuity.
func (s *Server) chatState() map[string]any {
	var latest int64
	if s.chatLog != nil {
		if seq, err := s.chatLog.LatestSeq(); err == nil {
			latest = seq
		}
	}
	return map[string]any{"latest_seq": latest}
}

// handleChatMessages reads the shared log non-destructively. Reads never
// consume anything: cursors only move via the explicit cursor endpoint.
func (s *Server) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	after := int64(0)
	if value := r.URL.Query().Get("after"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, errors.New("after must be a non-negative integer"))
			return
		}
		after = parsed
	}
	limit := 0
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, errors.New("limit must be a positive integer"))
			return
		}
		limit = parsed
	}

	messages, err := s.chatLog.After(after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	latest, err := s.chatLog.LatestSeq()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cursor, err := s.chatLog.Cursor(clientIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if messages == nil {
		messages = []chat.LogMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"messages":   messages,
		"latest_seq": latest,
		"cursor":     cursor,
	})
}

// handleChatCursor advances the caller's own cursor (monotonic). Each client
// owns exactly one cursor, so no controller lease is involved.
func (s *Server) handleChatCursor(w http.ResponseWriter, r *http.Request) {
	clientID := clientIDFromRequest(r)
	if clientID == "" {
		writeError(w, http.StatusBadRequest, errors.New("a client id header is required to advance a chat cursor"))
		return
	}
	var body struct {
		Seq int64 `json:"seq"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cursor, err := s.chatLog.AdvanceCursor(clientID, body.Seq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": cursor})
}

func (s *Server) chatMotionTarget(command *chat.MotionCommand, current motion.ActiveMotionState) motion.MotionTarget {
	patternID := motion.PatternID(command.PatternID)
	speedPercent := 0
	if command.Intensity != nil {
		speedPercent = *command.Intensity
	} else if command.SpeedPercent != nil {
		speedPercent = *command.SpeedPercent
	}
	var definition *motion.PatternDefinition
	var programDefinition *motion.ProgramDefinition
	var programID string
	if patternID != "" {
		if resolved, ok := s.patterns.ResolveEnabled(string(patternID)); ok {
			definition = &resolved
		} else {
			// The enabled set may change while the model is streaming. Never apply
			// a now-disabled selection; fall back to the deterministic target.
			patternID = ""
		}
	}
	if current.Running {
		if patternID == "" {
			if current.Target.Program != nil {
				copied := *current.Target.Program
				copied.Points = append([]motion.CurvePoint(nil), current.Target.Program.Points...)
				programDefinition = &copied
				programID = current.Target.ProgramID
			} else {
				patternID = current.Target.PatternID
			}
			if programDefinition == nil && current.Target.Pattern != nil {
				copied := *current.Target.Pattern
				copied.Points = append([]motion.CurvePoint(nil), current.Target.Pattern.Points...)
				copied.Tags = append([]string(nil), current.Target.Pattern.Tags...)
				definition = &copied
			}
		}
		if speedPercent == 0 {
			speedPercent = current.Target.SpeedPercent
		}
	}
	return motion.MotionTarget{
		Label:        "Chat",
		Source:       "chat",
		PatternID:    patternID,
		ProgramID:    programID,
		SpeedPercent: speedPercent,
		Pattern:      definition,
		Program:      programDefinition,
	}
}

func (s *Server) chatPatternChoices() []chat.PatternChoice {
	choices := s.patterns.EnabledChoices()
	result := make([]chat.PatternChoice, 0, len(choices))
	for _, choice := range choices {
		result = append(result, chat.PatternChoice{
			ID: choice.ID, Name: choice.Name, Description: choice.Description,
			Tags: choice.Tags, Weight: choice.Weight,
		})
	}
	return result
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
