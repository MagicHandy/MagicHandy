package httpapi

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// This session schedules inference only. Every accepted score uses the same
// FlowTarget, engine retargeting, sampler and transport as other motion sources.
// It deliberately tests the selected Lab contract instead of production's
// pattern fallback and variation policies, which would hide contract failures.
type labConversationSession struct {
	Active          bool   `json:"active"`
	Live            bool   `json:"live"`
	Autopilot       bool   `json:"autopilot"`
	Method          string `json:"method"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model"`
	SchemaGuided    bool   `json:"schema_guided"`
	IntervalSeconds int    `json:"interval_seconds"`
	Error           string `json:"error,omitempty"`
}

func (s *Server) handleLabConversationStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var options labConversationSession
	if err := decodeJSON(r, &options); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := normalizeLabConversationOptions(&options); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	stopSequence := s.stopSequence.Load()
	startCtx, release, err := s.beginLabMotion(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	finishStart, err := s.reserveLabConversation()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer finishStart()
	state := s.labState()
	ctx, finish, err := s.beginLabRequest(s.lifecycleCtx)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	stopStartup := context.AfterFunc(startCtx, cancel)
	done := make(chan struct{})
	options.Active, options.Error = true, ""
	s.lab.mu.Lock()
	s.lab.session, s.lab.sessionCtx, s.lab.sessionCancel, s.lab.sessionDone = options, ctx, cancel, done
	s.lab.lastActivity = time.Now()
	s.lab.mu.Unlock()
	started := false
	var startedEngine *motion.Engine
	defer func() {
		stopStartup()
		if !started {
			s.abortLabSession(done, startedEngine, cancel, finish)
		}
	}()
	if options.Live {
		startedEngine, err = s.startLabConversationMotion(ctx, stopSequence, state.Current)
		if err != nil {
			writeError(w, http.StatusBadGateway, errors.New(s.safeMotionErrorMessage(err)))
			return
		}
		s.lab.mu.Lock()
		s.lab.sessionEngine = startedEngine
		s.lab.mu.Unlock()
	}
	if ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		writeError(w, http.StatusConflict, errors.New("test start was canceled by Stop"))
		return
	}
	started = true
	go func() { defer finish(); defer s.finishLabSession(done); s.runLabAutopilot(ctx, options) }()
	writeJSON(w, http.StatusOK, s.labState())
}

func (s *Server) reserveLabConversation() (func(), error) {
	s.lab.mu.Lock()
	defer s.lab.mu.Unlock()
	if s.lab.busy || s.lab.session.Active || s.lab.sessionDone != nil {
		return nil, errors.New("end the current test before starting another")
	}
	s.lab.busy = true
	return func() { s.lab.mu.Lock(); s.lab.busy = false; s.lab.mu.Unlock() }, nil
}

func (s *Server) abortLabSession(done chan struct{}, engine *motion.Engine, cancel context.CancelFunc, finish func()) {
	cancel()
	if engine != nil && engine.Snapshot().Target.Label == "LLM Lab" {
		ctx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = engine.Stop(ctx, "lab_start_canceled")
		stopCancel()
	}
	finish()
	s.finishLabSession(done)
}

func (s *Server) finishLabSession(done chan struct{}) {
	s.lab.mu.Lock()
	defer s.lab.mu.Unlock()
	if s.lab.sessionDone == done {
		s.lab.session.Active = false
		s.lab.sessionDone, s.lab.sessionCancel, s.lab.sessionCtx, s.lab.sessionEngine = nil, nil, nil, nil
	}
	close(done)
}

// Invalidate synchronously; callers drain after stopping the engine so blocked
// startup/dispatch cannot delay Emergency Stop. Safe when no session exists.
func (s *Server) cancelLabSession() func() {
	s.lab.mu.Lock()
	if s.lab.sessionCancel != nil {
		s.lab.sessionCancel()
	}
	s.lab.session.Active = false
	done := s.lab.sessionDone
	s.lab.mu.Unlock()
	return func() {
		if done != nil {
			<-done
		}
	}
}

func (s *Server) interruptLabAutopilot(ctx context.Context) error {
	s.lab.mu.Lock()
	s.lab.lastActivity = time.Now()
	if s.lab.autoCancel != nil {
		s.lab.autoCancel()
	}
	done := s.lab.autoDone
	s.lab.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *Server) runLabAutopilot(ctx context.Context, options labConversationSession) {
	if !options.Autopilot {
		<-ctx.Done()
		return
	}
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	delay := labAutopilotDelay(options)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.lab.mu.Lock()
		ready := !s.lab.busy && time.Since(s.lab.lastActivity) >= delay
		if !ready {
			s.lab.mu.Unlock()
			continue
		}
		turnCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		s.lab.autoCancel, s.lab.autoDone = cancel, done
		s.lab.mu.Unlock()
		message := "Continue the conversation as Autopilot. Respect the user's latest requested limits and character. You may hold the current motion or make one small appropriate change. Never increase speed or widen the requested band without permission. Briefly describe what you actually change."
		if options.Method == "layered" {
			message = chat.LayeredAutopilotMessage()
		}
		state, err := s.runLabChat(turnCtx, labChatRequest{Message: message}, true)
		canceled := turnCtx.Err() != nil
		cancel()
		s.lab.mu.Lock()
		s.lab.autoCancel, s.lab.autoDone = nil, nil
		close(done)
		delay = labAutopilotDelay(options)
		if !canceled && (err != nil || (len(state.Turns) > 0 && !state.Turns[len(state.Turns)-1].Valid)) {
			s.lab.session.Autopilot = false
			s.lab.session.Error = "Autopilot paused after a failed reply. Inspect the response and restart the test."
			s.lab.mu.Unlock()
			<-ctx.Done()
			return
		}
		s.lab.mu.Unlock()
	}
}

func labAutopilotDelay(options labConversationSession) time.Duration {
	delay := time.Duration(options.IntervalSeconds) * time.Second
	if options.Method == "layered" || options.Method == "creative_v2" {
		// #nosec G404 -- Scheduling jitter; never shorter than the user's quiet interval.
		delay += time.Duration(rand.Float64() * 0.5 * float64(delay))
	}
	return delay
}

func labContinuationMessage(method, fallback string, turns []chat.LLMLabTrial) string {
	switch method {
	case "layered":
		return chat.LayeredContinuationMessage(labHumanRequests(turns))
	case "creative_v2":
		return chat.CreativeV2ContinuationMessage(labHumanRequests(turns))
	default:
		return fallback
	}
}

func (s *Server) applyLabConversationTarget(ctx context.Context, stopSequence uint64, spec motion.FlowSpec) (bool, string) {
	s.lab.mu.Lock()
	engine, active := s.lab.sessionEngine, s.lab.session.Active && s.lab.session.Live
	s.lab.mu.Unlock()
	if !active || ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		return false, "Live test was stopped."
	}
	if engine == nil || s.currentMotionEngine() != engine {
		s.cancelLabSession()
		return false, "Live test ended because motion ownership changed."
	}
	current := engine.Snapshot()
	if current.Target.Label != "LLM Lab" || !current.Running {
		s.cancelLabSession()
		return false, "Live test ended because motion ownership changed."
	}
	settings, _ := s.store.Snapshot()
	target, err := motion.FlowTarget(spec, settings.Motion)
	if err == nil {
		target.Label = "LLM Lab"
		_, err = engine.ApplyTargetIfCurrent(ctx, target, "lab_conversation", current.PlanID)
	}
	if err != nil {
		return false, s.safeMotionErrorMessage(err)
	}
	return true, ""
}

func labAutopilotWithinRequest(before, after motion.FlowSpec) bool {
	lo, hi, speed := before.MinPercent, before.MaxPercent, before.SpeedPercent
	for _, step := range before.Steps {
		lo, hi, speed = min(lo, step.MinPercent), max(hi, step.MaxPercent), max(speed, step.SpeedPercent)
	}
	if after.MinPercent < lo || after.MaxPercent > hi || after.SpeedPercent > speed {
		return false
	}
	for _, step := range after.Steps {
		if step.MinPercent < lo || step.MaxPercent > hi || step.SpeedPercent > speed {
			return false
		}
	}
	return true
}

func normalizeLabConversationOptions(options *labConversationSession) error {
	prompts := chat.LLMLabPrompts()
	if _, ok := prompts[options.Method]; !ok {
		return errors.New("unknown lab interface")
	}
	if strings.TrimSpace(options.Prompt) == "" {
		options.Prompt = prompts[options.Method]
	}
	if len(options.Prompt) > 16000 || (!options.Live && !options.Autopilot) {
		return errors.New("select live motion or Autopilot with a valid prompt")
	}
	if options.IntervalSeconds == 0 {
		options.IntervalSeconds = 20
	}
	if options.IntervalSeconds < 5 || options.IntervalSeconds > 120 {
		return errors.New("autopilot interval must be 5–120 seconds")
	}
	return nil
}

func (s *Server) startLabConversationMotion(ctx context.Context, stopSequence uint64, spec motion.FlowSpec) (*motion.Engine, error) {
	if s.modes != nil {
		drain := s.modes.BeginUserStop()
		_, err := s.stopActiveMotionForReplacement(ctx, "lab_conversation_start")
		drain()
		if err != nil {
			return nil, err
		}
	}
	// A mode can publish startup while it drains. Stop it before taking the
	// final admission, just as the ordinary manual-start path does.
	if _, err := s.stopActiveMotionForReplacement(ctx, "lab_conversation_start"); err != nil {
		return nil, err
	}
	engine, admission, err := s.motionEngineForStart()
	if err != nil {
		return nil, err
	}
	settings, _ := s.store.Snapshot()
	target, err := motion.FlowTarget(spec, settings.Motion)
	if err != nil {
		return nil, err
	}
	target.Label = "LLM Lab"
	if ctx.Err() != nil || s.stopSequence.Load() != stopSequence {
		return nil, errors.New("test start was canceled by Stop")
	}
	_, err = engine.StartAtGeneration(ctx, target, settings.Motion, admission)
	return engine, err
}
