package httpapi

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

var errChatRepairStale = errors.New("repair expired because the conversation, motion, or controls changed; send your request again")

type chatRepairContextKey struct{}

// One active chat means one bounded, process-local repair continuation. The
// rejected output itself is retained through the existing diagnostic log; this
// slot only makes its original validated execution context resumable briefly.
type chatRepairRuntime struct {
	mu      sync.Mutex
	pending *chatRepairCandidate
}

func (r *chatRepairRuntime) clear() {
	r.mu.Lock()
	r.pending = nil
	r.mu.Unlock()
}

type chatRepairCandidate struct {
	manualResume         *manualVibrationResume
	service              chat.Service
	settings             config.LLMSettings
	prompt               interactiveChatPromptContext
	result               chat.Result
	diagnostics          chat.MessageDiagnostics
	admission            cloudPlanAdmission
	workspaceRevision    uint64
	userSeq              int64
	actor                controllerIdentity
	controllerGeneration uint64
	hosted               hostedChatGuard
	engine               *motion.Engine
	state                motion.ActiveMotionState
	expires              time.Time
	active               atomic.Bool
	published            atomic.Bool
	dispatched           atomic.Bool
	evidenceID           atomic.Int64
}

func (s *Server) withOptionalChatRepair(ctx context.Context, advanced bool, session string, stop uint64, settings config.Settings) context.Context {
	if !advanced {
		return ctx
	}
	repairCtx, err := s.prepareChatRepair(ctx, session, stop, settings)
	if err == nil {
		return repairCtx
	}
	// Optional repair bookkeeping cannot prevent an otherwise valid reply.
	// Failed capture closes only repair admission, never its context guards.
	s.logger.Warn("response repair context unavailable")
	return ctx
}

func bindChatRepair(ctx context.Context, userSeq int64, service chat.Service, settings config.LLMSettings, prompt interactiveChatPromptContext) {
	if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok {
		candidate.service, candidate.settings, candidate.prompt = service, settings, prompt
		candidate.admission.chatRevision, candidate.userSeq = userSeq, userSeq
	}
}

func (s *Server) prepareChatRepair(ctx context.Context, session string, stop uint64, settings config.Settings) (context.Context, error) {
	admission, err := s.captureCloudAdmission(ctx, settings)
	if err != nil || admission.session != session || admission.sequence != stop {
		return ctx, errChatRepairStale
	}
	candidate := &chatRepairCandidate{admission: admission,
		workspaceRevision: s.chatWorkspace.Revision(), expires: time.Now().Add(5 * time.Minute)}
	candidate.hosted, _ = ctx.Value(hostedChatGuardKey{}).(hostedChatGuard)
	candidate.manualResume, _ = ctx.Value(manualVibrationResumeKey{}).(*manualVibrationResume)
	s.controller.mu.Lock()
	candidate.actor, candidate.controllerGeneration = s.controller.active, s.controller.generation
	s.controller.mu.Unlock()
	candidate.engine = s.currentMotionEngine()
	if candidate.engine != nil {
		candidate.state = candidate.engine.Snapshot()
	}
	return context.WithValue(ctx, chatRepairContextKey{}, candidate), nil
}

func (s *Server) chatRepairCurrent(ctx context.Context, candidate *chatRepairCandidate) bool {
	if candidate == nil || ctx.Err() != nil || s.stopSequence.Load() != candidate.admission.sequence ||
		s.chatWorkspace.Revision() != candidate.workspaceRevision {
		return false
	}
	s.controller.mu.Lock()
	current := s.controller.active == candidate.actor && s.controller.generation == candidate.controllerGeneration &&
		s.controller.snapshotLocked(candidate.actor).Active
	s.controller.mu.Unlock()
	if !current || s.hostedChatRetired(context.WithValue(ctx, hostedChatGuardKey{}, candidate.hosted)) {
		return false
	}
	if !candidate.published.Load() {
		if s.validateCloudAdmission(ctx, candidate.admission) != nil {
			return false
		}
	} else {
		if !s.chatRepairSettingsCurrent(candidate.admission.settings) {
			return false
		}
	}
	return s.chatRepairMotionCurrent(candidate)
}

func (s *Server) chatRepairSettingsCurrent(original config.Settings) bool {
	settings, _ := s.store.Snapshot()
	return reflect.DeepEqual(settings.LLM, original.LLM) && settings.Motion == original.Motion &&
		settings.Autopilot == original.Autopilot && reflect.DeepEqual(settings.Device, original.Device)
}

func (s *Server) chatRepairMotionCurrent(candidate *chatRepairCandidate) bool {
	if candidate.dispatched.Load() {
		return true
	}
	if resume := candidate.manualResume; resume != nil && !s.manualVibrationResumeCurrent(context.Background(), resume) {
		return false
	}
	engine := s.currentMotionEngine()
	if engine != candidate.engine {
		return false
	}
	if engine == nil {
		return true
	}
	state := engine.Snapshot()
	return state.Generation == candidate.state.Generation && state.PlanID == candidate.state.PlanID &&
		state.Running == candidate.state.Running && state.Starting == candidate.state.Starting && state.Paused == candidate.state.Paused
}

func (s *Server) activateChatRepair(ctx context.Context) error {
	candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate)
	if !ok || !s.chatRepairCurrent(ctx, candidate) {
		return errChatRepairStale
	}
	candidate.active.Store(true)
	return nil
}

func (s *Server) chatRepairRetired(ctx context.Context) bool {
	candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate)
	return ok && candidate.active.Load() && !s.chatRepairCurrent(ctx, candidate)
}

func (s *Server) markChatRepairPublished(ctx context.Context) {
	if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok {
		candidate.published.Store(true)
	}
}

func (s *Server) markChatRepairDispatched(ctx context.Context) {
	if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok {
		candidate.dispatched.Store(true)
	}
}

func (s *Server) applyChatTargetAtRepairBase(ctx context.Context, engine *motion.Engine, target motion.MotionTarget, reason string) (motion.ActiveMotionState, error) {
	if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok && candidate.active.Load() {
		if engine != candidate.engine || candidate.state.PlanID == "" {
			return engine.Snapshot(), errChatRepairStale
		}
		return engine.ApplyTargetIfCurrent(ctx, target, reason, candidate.state.PlanID)
	}
	return engine.ApplyTarget(ctx, target, reason)
}

func (s *Server) offerChatRepair(ctx context.Context, result chat.Result, diagnostics chat.MessageDiagnostics) map[string]any {
	candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate)
	if !ok || candidate.settings.HostedResponseRepair() || !result.CanRepair() || result.FailureID == 0 ||
		!s.chatRepairCurrent(ctx, candidate) {
		return nil
	}
	candidate.result, candidate.diagnostics = result, diagnostics
	s.chatRepair.mu.Lock()
	s.chatRepair.pending = candidate
	s.chatRepair.mu.Unlock()
	return map[string]any{"failure_id": result.FailureID, "available": true}
}

func (s *Server) handleChatRepair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID   string `json:"session_id"`
		FailureID   int64  `json:"failure_id"`
		MotionOwner string `json:"motion_owner,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil || body.FailureID <= 0 || body.SessionID == "" {
		writeError(w, http.StatusBadRequest, errors.New("a session and failed response are required"))
		return
	}
	holder, validHolder := chatMotionHolder(body.MotionOwner)
	if !validHolder {
		writeError(w, http.StatusBadRequest, errors.New(`motion_owner must be "script", "off" or empty`))
		return
	}
	session, sequence, ok := s.admitChatTurn(w, r, body.SessionID)
	if !ok {
		return
	}
	s.chatRepair.mu.Lock()
	candidate := s.chatRepair.pending
	if candidate == nil || candidate.result.FailureID != body.FailureID || candidate.admission.session != session {
		s.chatRepair.mu.Unlock()
		writeError(w, http.StatusConflict, errChatRepairStale)
		return
	}
	if time.Now().After(candidate.expires) || sequence != candidate.admission.sequence ||
		holder != candidate.service.Capabilities.MotionHolder || !s.chatRepairCurrent(r.Context(), candidate) {
		s.chatRepair.pending = nil
		s.chatRepair.mu.Unlock()
		writeError(w, http.StatusConflict, errChatRepairStale)
		return
	}
	ctx, finish, err := s.chatWorkspace.BeginRepairTurn(r.Context(), session, candidate.workspaceRevision)
	if err != nil {
		s.chatRepair.mu.Unlock()
		writeError(w, http.StatusConflict, errChatRepairStale)
		return
	}
	// Claim only after workspace admission succeeds. A click that races the
	// original stream's final cleanup cannot consume its available correction.
	s.chatRepair.pending = nil
	candidate.workspaceRevision = s.chatWorkspace.Revision()
	s.chatRepair.mu.Unlock()
	defer finish()
	ctx = context.WithValue(ctx, hostedChatGuardKey{}, candidate.hosted)
	ctx = context.WithValue(ctx, chatRepairContextKey{}, candidate)
	if candidate.manualResume != nil {
		ctx = context.WithValue(ctx, manualVibrationResumeKey{}, candidate.manualResume)
	}
	if err := s.activateChatRepair(ctx); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	settings, _ := s.store.Snapshot()
	activity, started := s.beginChatActivity(settings.Voice)
	defer s.modes.NotifyChatActivityComplete(activity)
	emit := s.clientChatEmitter(w, r)
	setSSEHeaders(w)
	if err := emitChatStarted(emit, candidate.settings, candidate.service.Prompt.ID, session, candidate.userSeq,
		candidate.prompt.CurrentMood, candidate.prompt.Persona); err != nil {
		return
	}
	result, diagnostics, err := s.completeInteractiveRepair(ctx, candidate, emit, started)
	applyPersonaStartingArea(&result, *candidate.service.Capabilities, candidate.prompt.Persona)
	s.emitChatCompletionResult(ctx, sequence, emit, result, err, session, diagnostics, candidate.prompt.CurrentMood, started)
}

func (s *Server) completeInteractiveRepair(ctx context.Context, candidate *chatRepairCandidate, emit sseEmitter,
	started time.Time,
) (chat.Result, chat.MessageDiagnostics, error) {
	diagnostics := candidate.diagnostics
	providerCtx, wait, release, err := s.acquireProviderRequest(ctx, llmRequestInteractive, candidate.settings)
	if err != nil {
		result := candidate.result
		result.RepairAttempted = true
		return result, diagnostics, err
	}
	defer release()
	service := candidate.service
	timed := newTimedLLMProvider(service.Provider)
	service.Provider = timed
	result, err := service.Repair(providerCtx, candidate.result, func(event chat.StreamEvent) error {
		if event.Type == "repair_start" && s.chatRepairRetired(ctx) {
			return errChatRepairStale
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return emitChatStreamEvent(emit, event)
	})
	if providerCtx.Err() != nil {
		err = context.Cause(providerCtx)
	}
	var timing chat.MessageDiagnostics
	timed.applyDiagnostics(&timing, started, 0, wait)
	diagnostics.ProviderCalls += timing.ProviderCalls
	diagnostics.RepairMillis = timing.GenerationMillis
	diagnostics.SchedulerWaitMillis += timing.SchedulerWaitMillis
	return result, diagnostics, err
}
