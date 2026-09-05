package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// The lab owns a separate, bounded in-memory conversation. Production chat,
// personas and saved prompts are not its memory or persistence mechanism.
type llmLabRuntime struct {
	mu            sync.Mutex
	startMu       sync.Mutex
	requests      map[uint64]context.CancelFunc
	nextRequest   uint64
	current       *motion.FlowSpec
	turns         []chat.LLMLabTrial
	revision      uint64
	busy          bool
	session       labConversationSession
	sessionCtx    context.Context
	sessionCancel context.CancelFunc
	sessionDone   chan struct{}
	sessionEngine *motion.Engine
	autoCancel    context.CancelFunc
	autoDone      chan struct{}
	lastActivity  time.Time
}

type llmLabState struct {
	Current     motion.FlowSpec        `json:"current"`
	Turns       []chat.LLMLabTrial     `json:"turns"`
	Revision    uint64                 `json:"revision"`
	Busy        bool                   `json:"busy"`
	Prompts     map[string]string      `json:"prompts"`
	Model       string                 `json:"model"`
	SettingsKey string                 `json:"settings_key"`
	Limits      config.MotionSettings  `json:"limits"`
	Session     labConversationSession `json:"session"`
}

func (s *Server) labState() llmLabState {
	settings, _ := s.store.Snapshot()
	s.lab.mu.Lock()
	defer s.lab.mu.Unlock()
	if s.lab.current == nil {
		initial := motion.DefaultFlowSpec()
		initial.SpeedPercent = max(settings.Motion.SpeedMinPercent, min(initial.SpeedPercent, settings.Motion.SpeedMaxPercent))
		s.lab.current = &initial
	}
	return llmLabState{Current: *s.lab.current, Turns: append([]chat.LLMLabTrial{}, s.lab.turns...),
		Revision: s.lab.revision, Busy: s.lab.busy, Prompts: chat.LLMLabPrompts(), Model: settings.LLM.Model,
		SettingsKey: motion.LabSettingsKey(settings.Motion), Limits: settings.Motion, Session: s.lab.session}
}

func (s *Server) handleLLMLabState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.labState())
}

func (s *Server) handleLLMLabStatus(w http.ResponseWriter, _ *http.Request) {
	s.lab.mu.Lock()
	status := struct {
		Revision uint64                 `json:"revision"`
		Busy     bool                   `json:"busy"`
		Session  labConversationSession `json:"session"`
	}{s.lab.revision, s.lab.busy, s.lab.session}
	s.lab.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleLLMLabReset(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Spec motion.FlowSpec `json:"spec"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, _ := s.store.Snapshot()
	if err := body.Spec.Validate(settings.Motion); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.lab.mu.Lock()
	if s.lab.busy || s.lab.session.Active {
		s.lab.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("a lab reply is still active"))
		return
	}
	s.lab.current, s.lab.turns = &body.Spec, nil
	s.lab.revision++
	s.lab.mu.Unlock()
	writeJSON(w, http.StatusOK, s.labState())
}

func (s *Server) handleLLMLabChat(w http.ResponseWriter, r *http.Request) {
	var body labChatRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if isChatStopMessage(body.Message) {
		if _, err := s.emergencyStop(r.Context(), "lab_chat_stop"); err != nil {
			writeError(w, http.StatusBadGateway, errors.New(s.safeMotionErrorMessage(err)))
			return
		}
		writeJSON(w, http.StatusOK, s.labState())
		return
	}
	if !s.requireController(w, r) {
		return
	}
	if err := s.interruptLabAutopilot(r.Context()); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	response, err := s.runLabChat(r.Context(), body, false)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	response.Busy = false
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

type labChatRequest struct {
	Message      string `json:"message"`
	Method       string `json:"method"`
	Prompt       string `json:"prompt"`
	Model        string `json:"model"`
	Revision     uint64 `json:"revision"`
	SchemaGuided bool   `json:"schema_guided"`
}

func (s *Server) runLabChat(parent context.Context, body labChatRequest, automatic bool) (llmLabState, error) {
	stopSequence := s.stopSequence.Load()
	state := s.labState()
	if state.Session.Active {
		body.Method, body.Prompt, body.Model, body.SchemaGuided = state.Session.Method, state.Session.Prompt, state.Session.Model, state.Session.SchemaGuided
	}
	if body.Prompt == "" {
		body.Prompt = state.Prompts[body.Method]
	}
	if _, ok := state.Prompts[body.Method]; !ok {
		return llmLabState{}, errors.New("unknown lab interface")
	}
	s.lab.mu.Lock()
	if state.Session.Active {
		if !s.lab.session.Active || s.lab.sessionCtx == nil {
			s.lab.mu.Unlock()
			return llmLabState{}, errors.New("lab session ended")
		}
		ctx, cancel := context.WithCancel(parent)
		unlink := context.AfterFunc(s.lab.sessionCtx, cancel)
		defer func() { unlink(); cancel() }()
		parent = ctx
	}
	if s.lab.busy || (!state.Session.Active && body.Revision != s.lab.revision) {
		s.lab.mu.Unlock()
		return llmLabState{}, errors.New("lab state changed; refresh before sending")
	}
	s.lab.busy = true
	s.lab.lastActivity = time.Now()
	s.lab.mu.Unlock()
	defer func() { s.lab.mu.Lock(); s.lab.busy = false; s.lab.lastActivity = time.Now(); s.lab.mu.Unlock() }()
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return llmLabState{}, err
	}
	trialCtx, finish, err := s.beginChat(parent, sessionID)
	if err != nil {
		return llmLabState{}, err
	}
	defer finish()
	priority := llmRequestInteractive
	if automatic {
		priority = llmRequestAutonomous
	}
	ctx, _, release, err := s.llmRequests.acquire(trialCtx, priority)
	if err != nil {
		return llmLabState{}, err
	}
	defer release()
	settings, _ := s.store.Snapshot()
	if model := strings.TrimSpace(body.Model); model != "" {
		settings.LLM.Model = model
	}
	provider, err := s.newLLMProvider(ctx, settings.LLM)
	if err != nil {
		return llmLabState{}, err
	}
	history := labConversationHistory(state.Turns, body)
	trial := chat.RunLLMLab(ctx, provider, settings.LLM.Model, body.Method, body.Prompt, body.Message, state.Current, settings.Motion, history, body.SchemaGuided)
	trial.Autopilot = automatic
	if ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		return llmLabState{}, errors.New("lab trial was canceled by Stop or its client")
	}
	return s.recordLabTrial(ctx, stopSequence, trial, state)
}

func (s *Server) recordLabTrial(ctx context.Context, stopSequence uint64, trial chat.LLMLabTrial, state llmLabState) (llmLabState, error) {
	latestSettings, _ := s.store.Snapshot()
	if motion.LabSettingsKey(latestSettings.Motion) != state.SettingsKey {
		trial.Valid, trial.After, trial.Error = false, state.Current, "motion limits changed during generation; retry the trial"
	}
	if trial.Valid {
		if _, err := motion.FlowTarget(trial.After, latestSettings.Motion); err != nil {
			trial.Valid, trial.After, trial.Error = false, state.Current, err.Error()
		}
	}
	if !trial.Valid {
		trial.Changed = []string{}
	}
	if trial.Autopilot && trial.Valid && !labAutopilotWithinRequest(trial.Before, trial.After) {
		trial.Valid, trial.After, trial.Changed = false, trial.Before, []string{}
		trial.Error = "Autopilot cannot increase speed or widen the requested band."
	}
	if trial.Valid && len(trial.Changed) > 0 && state.Session.Active && state.Session.Live {
		trial.MotionApplied, trial.MotionError = s.applyLabConversationTarget(ctx, stopSequence, trial.After)
	}
	s.lab.mu.Lock()
	if ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		s.lab.mu.Unlock()
		return llmLabState{}, errors.New("lab trial was canceled by Stop or its client")
	}
	if trial.Valid {
		s.lab.current = &trial.After
	}
	s.lab.turns = append(s.lab.turns, trial)
	if len(s.lab.turns) > 20 {
		s.lab.turns = s.lab.turns[len(s.lab.turns)-20:]
	}
	s.lab.revision++
	s.lab.mu.Unlock()
	return s.labState(), nil
}

func labConversationHistory(turns []chat.LLMLabTrial, body labChatRequest) []llm.Message {
	history := []llm.Message{}
	for _, turn := range turns {
		if turn.Method == body.Method && turn.Prompt == body.Prompt {
			history = append(history, llm.Message{Role: "user", Content: turn.Message}, llm.Message{Role: "assistant", Content: turn.Raw})
		}
	}
	return history
}
