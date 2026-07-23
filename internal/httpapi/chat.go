package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

type chatStreamRequest struct {
	SessionID string        `json:"session_id,omitempty"`
	Message   string        `json:"message"`
	History   []llm.Message `json:"history,omitempty"`
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
	sessionID, err := s.resolveActiveChatSession(body.SessionID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	settings, _ := s.store.Snapshot()
	if isChatStopMessage(body.Message) {
		s.handleChatStopFastPath(w, r, sessionID, body.Message, settings.LLM.Provider, settings.LLM.Model)
		return
	}
	if !s.requireController(w, r) {
		return
	}
	if strings.TrimSpace(r.Header.Get(stopSequenceHeader)) == "" {
		writeError(w, http.StatusConflict, errors.New("chat requires the current Emergency Stop sequence"))
		return
	}
	stopSequence, err := s.requestStopSequence(r)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	chatCtx, finishChat, err := s.beginChat(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer finishChat()
	started := time.Now()

	prompt, ok, err := s.personalization.prompts.Resolve(settings.LLM.PromptSet)
	if err != nil {
		s.writePersonalizationStorageError(w, "prompt set", err)
		return
	}
	if !ok {
		// A deleted or unknown selection falls back to the bundled default so
		// chat keeps working; the status event reports what actually ran.
		prompt, _ = chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	}
	memories, err := s.personalization.memory.PromptTexts()
	if err != nil {
		s.writePersonalizationStorageError(w, "memory", err)
		return
	}
	capabilities := chatCapabilities(settings.LLM)
	patternChoices, err := s.chatPatternChoicesFor(capabilities)
	if err != nil {
		s.writeLibraryStorageError(w, err)
		return
	}
	provider, err := s.newLLMProvider(chatCtx, settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}

	setSSEHeaders(w)
	motionContext := s.chatMotionContext(settings.Motion)
	service := chat.Service{
		Provider:              provider,
		Prompt:                prompt,
		Model:                 settings.LLM.Model,
		MaxTokens:             settings.LLM.MaxOutputTokens,
		ReasoningMode:         settings.LLM.ReasoningMode,
		ReasoningBudgetTokens: managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current),
		Memories:              memories,
		Patterns:              patternChoices,
		MotionContext:         &motionContext,
		Capabilities:          &capabilities,
	}
	emit := sseEmitter(func(event string, payload any) error {
		return writeSSE(w, event, payload)
	})

	// The user message enters the shared log once streaming actually starts;
	// its seq rides the status event so the sending client can track what it
	// has already displayed.
	userSeq := s.appendChatMessageTo(sessionID, chat.MessageRoleUser, body.Message, clientIDFromRequest(r), nil)

	if err := emitChatStarted(emit, settings.LLM, prompt.ID, sessionID, userSeq); err != nil {
		return
	}

	result, err := service.Complete(chatCtx, chat.Request{
		Message: body.Message,
		History: body.History,
	}, func(event chat.StreamEvent) error {
		return emitChatStreamEvent(emit, event)
	})
	s.emitChatCompletionResult(chatCtx, stopSequence, emit, result, err, sessionID, chat.MessageDiagnostics{
		Source:    "interactive",
		Provider:  settings.LLM.Provider,
		Model:     settings.LLM.Model,
		PromptSet: prompt.ID,
	}, started)
}

func emitChatStarted(emit sseEmitter, settings config.LLMSettings, promptID, sessionID string, userSeq int64) error {
	return emit("status", map[string]any{
		"state":      "streaming",
		"provider":   settings.Provider,
		"model":      settings.Model,
		"prompt_set": promptID,
		"session_id": sessionID,
		"user_seq":   userSeq,
	})
}

func emitChatStreamEvent(emit sseEmitter, event chat.StreamEvent) error {
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
}

func managedLlamaReasoningBudget(settings config.LLMSettings, runtimeCurrent bool) int {
	if settings.Provider != config.LLMProviderLlamaCPP ||
		settings.LlamaCPPMode != config.LlamaCPPModeManaged ||
		settings.ReasoningMode != config.LLMReasoningAuto || !runtimeCurrent || settings.MaxOutputTokens < 2 {
		return 0
	}
	return settings.MaxOutputTokens / 2
}

func (s *Server) emitChatCompletionResult(ctx context.Context, stopSequence uint64, emit sseEmitter, result chat.Result, err error, sessionID string, diagnostics chat.MessageDiagnostics, started time.Time) {
	if err != nil {
		s.logger.Warn("chat stream failed", "provider", diagnostics.Provider, "error", err)
		_ = emit("error", map[string]string{"message": err.Error()})
		_ = emit("done", map[string]any{"ok": false})
		return
	}
	if s.chatCanceled(ctx, stopSequence) {
		_ = emit("error", map[string]string{"message": "Chat canceled by Emergency Stop."})
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
	diagnostics.RequestMillis = time.Since(started).Milliseconds()
	diagnostics.Repaired = result.Repaired
	diagnostics.SemanticFallback = result.SemanticFallback
	diagnostics.InitialMalformed = result.InitialMalformed
	if result.Response.Motion != nil {
		diagnostics.MotionAction = result.Response.Motion.Action
	}
	replySeq := s.appendChatMessageTo(sessionID, chat.MessageRoleAssistant, result.Response.Reply, "", &diagnostics)
	if s.chatCanceled(ctx, stopSequence) {
		if replySeq > 0 && s.chatLog != nil {
			if err := s.chatLog.Delete(replySeq); err != nil {
				s.logger.Warn("delete Stop-invalidated chat reply", "seq", replySeq, "error", err)
			}
		}
		_ = emit("error", map[string]string{"message": "Chat canceled by Emergency Stop."})
		_ = emit("done", map[string]any{"ok": false})
		return
	}

	if err := emit("message", map[string]any{
		"reply":             result.Response.Reply,
		"repaired":          result.Repaired,
		"semantic_fallback": result.SemanticFallback,
		"initial_malformed": result.InitialMalformed,
		"motion":            result.Response.Motion,
		"diagnostics":       diagnostics,
		"seq":               replySeq,
	}); err != nil {
		return
	}

	speech := s.enqueueSpeech(result.Response.Reply)
	if s.chatCanceled(ctx, stopSequence) {
		if speech != nil {
			s.voice.Worker(voice.RoleTTS).Cancel(speech)
		}
		_ = emit("error", map[string]string{"message": "Chat canceled by Emergency Stop."})
		_ = emit("done", map[string]any{"ok": false})
		return
	}
	dispatch, motionErr := s.dispatchChatMotionAt(ctx, result.Response.Motion, &stopSequence)
	if speech != nil {
		_ = emit("speech", map[string]any{"request_id": speech.ID})
	}
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
		"ok":                motionErr == nil,
		"repaired":          result.Repaired,
		"semantic_fallback": result.SemanticFallback,
	})
}

func (s *Server) chatCanceled(ctx context.Context, stopSequence uint64) bool {
	return ctx.Err() != nil || s.stopSequence.Load() != stopSequence
}

func (s *Server) dispatchChatMotion(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	return s.dispatchChatMotionAt(ctx, command, nil)
}

func (s *Server) dispatchChatMotionAt(ctx context.Context, command *chat.MotionCommand, stopSequence *uint64) (chatMotionDispatch, error) {
	if stopSequence != nil && s.stopSequence.Load() != *stopSequence {
		action := ""
		if command != nil {
			action = command.Action
		}
		return chatMotionDispatch{Action: action}, errors.New("chat motion canceled by Emergency Stop")
	}
	return s.dispatchChatMotionLocked(ctx, command)
}

func (s *Server) dispatchChatMotionLocked(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	if command == nil || command.Action == "" || command.Action == chat.MotionActionNone {
		return chatMotionDispatch{Action: chat.MotionActionNone}, nil
	}

	switch command.Action {
	case chat.MotionActionStart:
		engine, admission, err := s.motionEngineForStart()
		if err != nil {
			return chatMotionDispatch{Action: command.Action}, err
		}
		current := engine.Snapshot()
		target, err := s.chatMotionTarget(command, current)
		if err != nil {
			return chatMotionDispatch{Action: command.Action}, err
		}
		s.notifyChatTarget(target)
		if current.Running {
			state, err := engine.ApplyTarget(ctx, target, "chat_start_retarget")
			return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
		}
		settings, _ := s.store.Snapshot()
		state, err := engine.StartAtGeneration(ctx, target, settings.Motion, admission)
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
	case chat.MotionActionTarget:
		engine := s.currentMotionEngine()
		if engine == nil {
			return chatMotionDispatch{Action: command.Action}, errors.New("motion is not running; use start to begin")
		}
		current := engine.Snapshot()
		if !current.Running {
			return chatMotionDispatch{Action: command.Action, Engine: current}, errors.New("motion is not running; use start to begin")
		}
		target, err := s.chatMotionTarget(command, current)
		if err != nil {
			return chatMotionDispatch{Action: command.Action}, err
		}
		s.notifyChatTarget(target)
		state, err := engine.ApplyTarget(ctx, target, "chat_target")
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, err
	case chat.MotionActionStop:
		// A chat stop is a user stop: modes end and keepalive stands down.
		finishModeStop := func() {}
		if s.modes != nil {
			s.modes.NotifyChatStop()
			finishModeStop = s.modes.BeginUserStop()
		}
		defer finishModeStop()
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

func (s *Server) requestStopSequence(r *http.Request) (uint64, error) {
	current := s.stopSequence.Load()
	value := strings.TrimSpace(r.Header.Get(stopSequenceHeader))
	if value == "" {
		return current, nil
	}
	expected, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, errors.New("invalid Emergency Stop sequence")
	}
	if expected != current {
		return 0, errors.New("request was invalidated by Emergency Stop")
	}
	return expected, nil
}

func (s *Server) beginChat(parent context.Context, sessionID string) (context.Context, context.CancelFunc, error) {
	s.chatLifecycleMu.Lock()
	defer s.chatLifecycleMu.Unlock()
	activeID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return nil, nil, errors.New("chat session is unavailable")
	}
	if activeID != sessionID {
		return nil, nil, errors.New("the selected chat is no longer active; refresh the conversation tabs")
	}
	ctx, cancel := context.WithCancel(parent)
	s.chatCancelMu.Lock()
	if len(s.chatCancels) > 0 {
		s.chatCancelMu.Unlock()
		cancel()
		return nil, nil, errors.New("one chat reply is already active")
	}
	s.nextChatID++
	id := s.nextChatID
	if s.chatCancels == nil {
		s.chatCancels = make(map[uint64]context.CancelFunc)
	}
	s.chatCancels[id] = cancel
	s.chatCancelMu.Unlock()
	return ctx, func() {
		cancel()
		s.chatCancelMu.Lock()
		delete(s.chatCancels, id)
		s.chatCancelMu.Unlock()
	}, nil
}

func (s *Server) cancelActiveChats() {
	s.chatCancelMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.chatCancels))
	for id, cancel := range s.chatCancels {
		cancels = append(cancels, cancel)
		delete(s.chatCancels, id)
	}
	s.chatCancelMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Server) notifyChatTarget(target motion.MotionTarget) {
	if s.modes != nil {
		s.modes.NotifyChatTarget(target)
	}
}

func (s *Server) resolveActiveChatSession(requested string) (string, error) {
	activeID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return "", errors.New("chat session is unavailable")
	}
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != activeID {
		return "", errors.New("the selected chat is no longer active; refresh the conversation tabs")
	}
	return activeID, nil
}

func (s *Server) handleChatStopFastPath(w http.ResponseWriter, r *http.Request, sessionID string, message string, provider string, model string) {
	s.chatLifecycleMu.Lock()
	activeID, err := s.chatLog.ActiveSessionID()
	if err != nil || activeID != sessionID {
		s.chatLifecycleMu.Unlock()
		writeError(w, http.StatusConflict, errors.New("the selected chat is no longer active; refresh the conversation tabs"))
		return
	}
	userSeq := s.appendChatMessageTo(sessionID, chat.MessageRoleUser, message, clientIDFromRequest(r), nil)
	// The deterministic reply is displayed, so it enters the shared log like
	// any other reply; a stop confirmation is deliberately never spoken —
	// physical Stop must not wait on TTS.
	diagnostics := chat.MessageDiagnostics{
		Source:       "deterministic_stop",
		Provider:     provider,
		Model:        model,
		MotionAction: chat.MotionActionStop,
	}
	replySeq := s.appendChatMessageTo(sessionID, chat.MessageRoleAssistant, "Stopping motion.", "", &diagnostics)
	s.chatLifecycleMu.Unlock()
	setSSEHeaders(w)
	emit := func(event string, payload any) error {
		return writeSSE(w, event, payload)
	}
	if err := emit("status", map[string]any{
		"state":      "deterministic_stop",
		"provider":   provider,
		"model":      model,
		"session_id": sessionID,
		"user_seq":   userSeq,
	}); err != nil {
		return
	}
	command := &chat.MotionCommand{Action: chat.MotionActionStop}
	if err := emit("message", map[string]any{
		"reply":             "Stopping motion.",
		"repaired":          false,
		"initial_malformed": false,
		"motion":            command,
		"diagnostics":       diagnostics,
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

func (s *Server) appendChatMessageTo(sessionID string, role string, content string, clientID string, diagnostics *chat.MessageDiagnostics) int64 {
	if s.chatLog == nil {
		return 0
	}
	seq, err := s.chatLog.AppendTo(sessionID, role, content, clientID, diagnostics)
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
	pending, err := s.voice.Submit(voice.RoleTTS, voice.Request{
		Type: voice.RequestSpeak,
		Text: reply,
	})
	if err != nil {
		s.logger.Warn("TTS enqueue skipped", "error", err)
		return nil
	}
	return pending
}

// chatState is the /api/state block other tabs poll for continuity.
func (s *Server) chatState() map[string]any {
	if s.chatLog == nil {
		return map[string]any{"available": false, "latest_seq": int64(0), "active_session_id": ""}
	}
	activeID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return map[string]any{"available": false, "latest_seq": int64(0), "active_session_id": ""}
	}
	latest, err := s.chatLog.LatestSeq()
	if err != nil {
		return map[string]any{"available": false, "latest_seq": int64(0), "active_session_id": activeID}
	}
	return map[string]any{"available": true, "latest_seq": latest, "active_session_id": activeID}
}

// handleChatMessages reads the shared log non-destructively. Reads never
// consume anything: cursors only move via the explicit cursor endpoint.
func (s *Server) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		var err error
		sessionID, err = s.chatLog.ActiveSessionID()
		if err != nil {
			s.writeChatStorageError(w, err)
			return
		}
	}
	if _, err := s.chatLog.Session(sessionID); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
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

	// Autopilot appends the visible message before enqueuing TTS. Sharing this
	// short lock with that delivery path prevents a client from observing and
	// advancing past the row before its optional speech request ID is attached.
	s.chatSpeechMu.Lock()
	messages, err := s.chatLog.AfterSession(sessionID, after, limit)
	if err == nil {
		for index := range messages {
			messages[index].SpeechRequestID = s.chatSpeechRequests[messages[index].Seq]
		}
	}
	s.chatSpeechMu.Unlock()
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	latest, err := s.chatLog.LatestSeqSession(sessionID)
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	cursor, err := s.chatLog.CursorSession(clientIDFromRequest(r), sessionID)
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	if messages == nil {
		messages = []chat.LogMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"messages":   messages,
		"latest_seq": latest,
		"cursor":     cursor,
		"session_id": sessionID,
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
		SessionID string `json:"session_id,omitempty"`
		Seq       int64  `json:"seq"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		var err error
		sessionID, err = s.chatLog.ActiveSessionID()
		if err != nil {
			s.writeChatStorageError(w, err)
			return
		}
	}
	if _, err := s.chatLog.Session(sessionID); err != nil {
		s.writeChatSessionError(w, err)
		return
	}
	cursor, err := s.chatLog.AdvanceCursorSession(clientID, sessionID, body.Seq)
	if err != nil {
		s.writeChatStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": cursor, "session_id": sessionID})
}

func (s *Server) writeChatStorageError(w http.ResponseWriter, err error) {
	s.logger.Error("chat log storage operation failed", "error", err)
	writeError(w, http.StatusInternalServerError, errors.New("chat history storage is unavailable"))
}

func (s *Server) chatMotionTarget(command *chat.MotionCommand, current motion.ActiveMotionState) (motion.MotionTarget, error) {
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
		resolved, ok, err := s.patterns.ResolveEnabled(string(patternID))
		if err != nil {
			return motion.MotionTarget{}, fmt.Errorf("resolve chat pattern: %w", err)
		}
		if ok {
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
		AreaFocus:    resolveAreaFocus(command.Area, current),
	}, nil
}

// resolveAreaFocus maps a named zone onto the engine's bounded focus window.
// An unset zone preserves the running target's focus (a focus persists until
// changed — the STGPT-RV behavior); "full" explicitly clears it.
func resolveAreaFocus(zone string, current motion.ActiveMotionState) *motion.AreaFocus {
	if zone == "" {
		if current.Running && current.Target.AreaFocus != nil {
			carried := *current.Target.AreaFocus
			return &carried
		}
		return nil
	}
	focus, ok := zoneAreaFocus(zone)
	if !ok {
		return nil
	}
	return focus
}

// zoneAreaFocus localizes a named zone into a bounded relative window. Zones
// are semantic thirds with overlap so transitions stay smooth; "full" clears
// focus entirely.
func zoneAreaFocus(zone string) (*motion.AreaFocus, bool) {
	switch zone {
	case chat.AreaZoneTip:
		return &motion.AreaFocus{MinPercent: 66, MaxPercent: 100}, true
	case chat.AreaZoneShaft:
		return &motion.AreaFocus{MinPercent: 33, MaxPercent: 67}, true
	case chat.AreaZoneBase:
		return &motion.AreaFocus{MinPercent: 0, MaxPercent: 34}, true
	case chat.AreaZoneFull:
		return nil, true
	default:
		return nil, false
	}
}

func (s *Server) chatMotionContext(settings config.MotionSettings) chat.MotionContext {
	context := chat.MotionContext{
		SpeedMinPercent: settings.SpeedMinPercent,
		SpeedMaxPercent: settings.SpeedMaxPercent,
	}
	engine := s.currentMotionEngine()
	if engine == nil {
		return context
	}
	snapshot := engine.Snapshot()
	if !snapshot.Running && !snapshot.Paused {
		return context
	}
	context.Running = snapshot.Running
	context.Paused = snapshot.Paused
	context.PatternID = string(snapshot.Target.PatternID)
	context.ProgramID = snapshot.Target.ProgramID
	context.RecentPatternIDs = s.recentChatPatternIDs(4)
	context.SpeedPercent = snapshot.Target.SpeedPercent
	context.Area = chatAreaZone(snapshot.Target.AreaFocus)
	return context
}

func (s *Server) recentChatPatternIDs(limit int) []string {
	if s.traces == nil || limit < 1 {
		return nil
	}
	patterns := make([]string, 0, limit)
	for _, row := range s.traces.Rows() {
		if row.Source != "chat" {
			continue
		}
		patternID := ""
		if row.Target != nil {
			patternID = strings.TrimSpace(row.Target.PatternIdentifier)
		}
		if row.Retarget != nil && strings.TrimSpace(row.Retarget.NextPatternIdentifier) != "" {
			patternID = strings.TrimSpace(row.Retarget.NextPatternIdentifier)
		}
		if patternID == "" || (len(patterns) > 0 && strings.EqualFold(patterns[len(patterns)-1], patternID)) {
			continue
		}
		patterns = append(patterns, patternID)
	}
	if len(patterns) > limit {
		patterns = patterns[len(patterns)-limit:]
	}
	return patterns
}

func chatAreaZone(focus *motion.AreaFocus) string {
	if focus == nil {
		return chat.AreaZoneFull
	}
	switch *focus {
	case motion.AreaFocus{MinPercent: 66, MaxPercent: 100}:
		return chat.AreaZoneTip
	case motion.AreaFocus{MinPercent: 33, MaxPercent: 67}:
		return chat.AreaZoneShaft
	case motion.AreaFocus{MinPercent: 0, MaxPercent: 34}:
		return chat.AreaZoneBase
	default:
		return "custom"
	}
}

// chatPatternChoicesFor builds the model-visible catalog for the enabled
// capability gates: experimental-tagged patterns stay in the library UI but
// leave the model's menu unless the user opted in.
func (s *Server) chatPatternChoicesFor(capabilities chat.Capabilities) ([]chat.PatternChoice, error) {
	if !capabilities.Motion || !capabilities.Patterns {
		return nil, nil
	}
	choices, err := s.patterns.EnabledChoices()
	if err != nil {
		return nil, err
	}
	result := make([]chat.PatternChoice, 0, len(choices))
	for _, choice := range choices {
		if !capabilities.ExperimentalPatterns && slices.Contains(choice.Tags, motion.TagExperimental) {
			continue
		}
		result = append(result, chat.PatternChoice{
			ID: choice.ID, Name: choice.Name, Description: choice.Description,
			Tags: choice.Tags, Weight: choice.Weight,
		})
	}
	return result, nil
}

// chatCapabilities resolves the settings gates into the chat-layer shape.
func chatCapabilities(settings config.LLMSettings) chat.Capabilities {
	resolved := settings.Capabilities()
	return chat.Capabilities{
		Motion:               resolved.Motion,
		Patterns:             resolved.Patterns,
		AreaFocus:            resolved.AreaFocus,
		ExperimentalPatterns: resolved.ExperimentalPatterns,
		Voice:                chatVoiceLevel(settings.ChatVoice),
	}
}

func chatVoiceLevel(voice string) chat.VoiceLevel {
	switch voice {
	case config.LLMChatVoiceWarm:
		return chat.VoiceWarm
	case config.LLMChatVoiceIntimate:
		return chat.VoiceIntimate
	case config.LLMChatVoiceExplicit:
		return chat.VoiceExplicit
	default:
		return chat.VoiceUtility
	}
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
