package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// labStartingScore is the score a Lab test mode starts from: each generator
// needs its own authoritative format, with speed inside the saved limits.
func labStartingScore(method string, limits config.MotionSettings) motion.FlowSpec {
	initial := motion.DefaultFlowSpec()
	switch {
	case method == "layered":
		initial = chat.FreshLayeredScore(25)
	case method == "creative_v2":
		initial = chat.FreshCreativeV2Score(25)
	case chat.IsStrokeLabMethod(method):
		initial = chat.FreshStrokeLabScore(limits)
	}
	initial.SpeedPercent = max(limits.SpeedMinPercent, min(initial.SpeedPercent, limits.SpeedMaxPercent))
	return initial
}

type labCompareRequest struct {
	Message      string `json:"message"`
	Method       string `json:"method"`
	Model        string `json:"model"`
	SchemaGuided bool   `json:"schema_guided"`
}

// labCompareResult is one mode's answer with its compiled estimate: the
// summary and a 12-second position trace sampled every 100 ms.
type labCompareResult struct {
	Trial      chat.LLMLabTrial         `json:"trial"`
	Perceptual motion.PerceptualSummary `json:"perceptual"`
	Trace      []float64                `json:"trace"`
}

// handleLLMLabCompare runs one stateless trial: the request in one mode from
// that mode's starting score, without conversation history. It never records
// a turn, changes the Lab score or dispatches motion, so a client can put the
// same request to several modes and compare them.
func (s *Server) handleLLMLabCompare(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body labCompareRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(body.Message) == "" || len(body.Message) > 2000 {
		writeError(w, http.StatusBadRequest, errors.New("a message of at most 2000 characters is required"))
		return
	}
	result, err := s.runLabCompare(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) runLabCompare(parent context.Context, body labCompareRequest) (labCompareResult, error) {
	stopSequence := s.stopSequence.Load()
	prompt, ok := chat.LLMLabPrompts()[body.Method]
	if !ok {
		return labCompareResult{}, errors.New("unknown lab interface")
	}
	s.lab.mu.Lock()
	if s.lab.busy || s.lab.session.Active {
		s.lab.mu.Unlock()
		return labCompareResult{}, errors.New("end the lab test and wait for its reply before comparing modes")
	}
	s.lab.busy = true
	s.lab.lastActivity = time.Now()
	s.lab.mu.Unlock()
	defer func() { s.lab.mu.Lock(); s.lab.busy = false; s.lab.lastActivity = time.Now(); s.lab.mu.Unlock() }()
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return labCompareResult{}, err
	}
	trialCtx, finish, err := s.chatWorkspace.BeginTurn(parent, sessionID)
	if err != nil {
		return labCompareResult{}, err
	}
	defer finish()
	ctx, _, release, err := s.llmRequests.acquire(trialCtx, llmRequestInteractive)
	if err != nil {
		return labCompareResult{}, err
	}
	defer release()
	settings, _ := s.store.Snapshot()
	if model := strings.TrimSpace(body.Model); model != "" {
		settings.LLM.Model = model
	}
	provider, err := s.newLLMProvider(ctx, settings.LLM)
	if err != nil {
		return labCompareResult{}, err
	}
	start := labStartingScore(body.Method, settings.Motion)
	trial := chat.RunLLMLab(ctx, provider, settings.LLM.Model, body.Method, prompt, body.Message, start, settings.Motion, nil, body.SchemaGuided)
	if ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		return labCompareResult{}, errors.New("the comparison was canceled by Stop or its client")
	}
	trial = s.validateLabTrial(trial, llmLabState{Current: start, SettingsKey: motion.LabSettingsKey(settings.Motion)})
	result := labCompareResult{Trial: trial, Trace: []float64{}}
	if target, err := motion.FlowTarget(trial.After, settings.Motion); err == nil {
		plan := motion.NewMotionPlan("lab-compare", target, settings.Motion, 0, 0, time.Unix(0, 0))
		result.Perceptual = plan.Perceptual
		for millis := int64(0); millis <= 12000; millis += 100 {
			result.Trace = append(result.Trace, math.Round(plan.SampleAt(millis).PositionPercent*10)/10)
		}
	}
	return result, nil
}
