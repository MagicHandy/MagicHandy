package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// The lab owns a separate, bounded in-memory conversation. Production chat,
// personas and saved prompts are not its memory or persistence mechanism.
type llmLabRuntime struct {
	mu          sync.Mutex
	startMu     sync.Mutex
	requests    map[uint64]context.CancelFunc
	nextRequest uint64
	current     *motion.FlowSpec
	turns       []chat.LLMLabTrial
	revision    uint64
	busy        bool
}

type llmLabState struct {
	Current     motion.FlowSpec       `json:"current"`
	Turns       []chat.LLMLabTrial    `json:"turns"`
	Revision    uint64                `json:"revision"`
	Busy        bool                  `json:"busy"`
	Prompts     map[string]string     `json:"prompts"`
	Model       string                `json:"model"`
	SettingsKey string                `json:"settings_key"`
	Limits      config.MotionSettings `json:"limits"`
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
		SettingsKey: motion.LabSettingsKey(settings.Motion), Limits: settings.Motion}
}

func (s *Server) handleLLMLabState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.labState())
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
	if s.lab.busy {
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
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Message      string `json:"message"`
		Method       string `json:"method"`
		Prompt       string `json:"prompt"`
		Model        string `json:"model"`
		Revision     uint64 `json:"revision"`
		SchemaGuided bool   `json:"schema_guided"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state := s.labState()
	if body.Prompt == "" {
		body.Prompt = state.Prompts[body.Method]
	}
	if _, ok := state.Prompts[body.Method]; !ok {
		writeError(w, http.StatusBadRequest, errors.New("unknown lab interface"))
		return
	}
	s.lab.mu.Lock()
	if s.lab.busy || body.Revision != s.lab.revision {
		s.lab.mu.Unlock()
		writeError(w, http.StatusConflict, errors.New("lab state changed; refresh before sending"))
		return
	}
	s.lab.busy = true
	s.lab.mu.Unlock()
	defer func() { s.lab.mu.Lock(); s.lab.busy = false; s.lab.mu.Unlock() }()
	stopSequence := s.stopSequence.Load()
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	trialCtx, finish, err := s.beginChat(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer finish()
	ctx, _, release, err := s.llmRequests.acquire(trialCtx, llmRequestInteractive)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	settings, _ := s.store.Snapshot()
	if model := strings.TrimSpace(body.Model); model != "" {
		settings.LLM.Model = model
	}
	provider, err := s.newLLMProvider(ctx, settings.LLM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	history := []llm.Message{}
	for _, turn := range state.Turns {
		if turn.Method == body.Method && turn.Prompt == body.Prompt {
			history = append(history, llm.Message{Role: "user", Content: turn.Message}, llm.Message{Role: "assistant", Content: turn.Raw})
		}
	}
	trial := chat.RunLLMLab(ctx, provider, settings.LLM.Model, body.Method, body.Prompt, body.Message, state.Current, settings.Motion, history, body.SchemaGuided)
	if ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		writeError(w, http.StatusConflict, errors.New("lab trial was canceled by Stop or its client"))
		return
	}
	response, err := s.recordLabTrial(ctx, stopSequence, trial, state)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	response.Busy = false
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
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
