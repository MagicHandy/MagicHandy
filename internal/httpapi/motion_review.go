package httpapi

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type motionReviewJob struct {
	input                                                                       chat.MotionReviewInput
	settings                                                                    config.Settings
	reviewer                                                                    config.LLMSettings
	state                                                                       motion.ActiveMotionState
	ticket                                                                      modes.ReviewTicket
	scope                                                                       commandScope
	commandSequence, stopSequence, controllerGeneration, authRevision, revision uint64
	actor                                                                       controllerIdentity
	queuedAt                                                                    time.Time
}

func (s *Server) reviewLocalChoice(settings config.LLMSettings) bool {
	return settings.MotionReviewer.Enabled && !settings.IsHosted() && chatCapabilities(settings, nil).Motion
}

func (s *Server) beforeChatReview(settings config.LLMSettings, capabilities chat.Capabilities) *motion.MotionTarget {
	if !s.reviewLocalChoice(settings) || !capabilities.Motion || capabilities.MotionHolder != chat.MotionHolderSettings {
		return nil
	}
	before := motion.MotionTarget{}
	if engine := s.currentMotionEngine(); engine != nil {
		before = engine.Snapshot().Target
	}
	return &before
}

func (s *Server) reviewCompletedChat(ctx context.Context, completed bool, before *motion.MotionTarget, stop, activity uint64, request string) {
	if !completed || before == nil || s.chatCanceled(ctx, stop) {
		return
	}
	if engine := s.currentMotionEngine(); engine != nil {
		applied := engine.Snapshot()
		s.modes.NotifyChatActivityComplete(activity)
		s.queueChatMotionReview(*before, applied, request)
	}
}

func (s *Server) queueChatMotionReview(before motion.MotionTarget, applied motion.ActiveMotionState, request string) {
	settings, _ := s.store.Snapshot()
	if !s.reviewLocalChoice(settings.LLM.ConversationSettings()) {
		return
	}
	s.queueMotionReview(settings, applied, chat.MotionReviewInput{Source: "chat", Request: request, Before: before})
}

func (s *Server) reviewAutopilotChoice(input modes.DecisionInput, state motion.ActiveMotionState) {
	settings, _ := s.store.Snapshot()
	planning, err := settings.LLM.PlanningSettings()
	if err != nil || settings.LLM.MotionPlanner.Provider == config.MotionPlannerDecisions || !s.reviewLocalChoice(planning) {
		return
	}
	capabilities := chatCapabilities(settings.LLM, nil)
	facts := autopilotPromptContext(input, capabilities)
	// Autonomous speech, names, and personalization do not belong in a motion review.
	facts.LastSay, facts.Style, facts.CurrentAnchors = "", "", nil
	before := motion.MotionTarget{Flow: input.CurrentFlow, Dynamic: input.CurrentDynamic, PatternID: input.CurrentPatternID, SpeedPercent: input.CurrentSpeed, AreaFocus: input.CurrentAreaFocus}
	s.queueMotionReview(settings, state, chat.MotionReviewInput{Source: "autopilot", Before: before, Autopilot: &facts})
}

func (s *Server) queueMotionReview(settings config.Settings, state motion.ActiveMotionState, input chat.MotionReviewInput) {
	if !state.Running || state.Starting || state.Paused || state.PlanID == "" {
		return
	}
	if state.Target.Source != "chat" && state.Target.Source != modes.ModeAutopilot {
		return
	}
	reviewer, err := settings.LLM.ReviewerSettings()
	if err != nil {
		return
	}
	ticket, ok := s.modes.MotionReviewTicket()
	if !ok {
		return
	}
	input.After, input.Limits = state.Target, settings.Motion
	job := motionReviewJob{input: input, settings: settings, reviewer: reviewer, state: state, ticket: ticket, queuedAt: time.Now(), stopSequence: s.stopSequence.Load()}
	s.commands.mu.Lock()
	job.scope, job.commandSequence = s.commands.scope, s.commands.motionSequence
	s.commands.mu.Unlock()
	s.controller.mu.Lock()
	job.actor, job.controllerGeneration = s.controller.active, s.controller.generation
	s.controller.mu.Unlock()
	s.cloudPlanning.mu.Lock()
	job.authRevision = s.cloudPlanning.revision
	s.cloudPlanning.mu.Unlock()
	s.submitMotionReview(job)
}

func (s *Server) motionReviewCurrent(ctx context.Context, job motionReviewJob) bool {
	if ctx.Err() != nil || s.stopSequence.Load() != job.stopSequence {
		return false
	}
	settings, _ := s.store.Snapshot()
	if !reflect.DeepEqual(settings.LLM, job.settings.LLM) || settings.Motion != job.settings.Motion || settings.Autopilot != job.settings.Autopilot || !reflect.DeepEqual(settings.Device, job.settings.Device) {
		return false
	}
	s.commands.mu.Lock()
	current := s.commands.scope == job.scope && s.commands.motionSequence == job.commandSequence
	s.commands.mu.Unlock()
	s.controller.mu.Lock()
	current = current && s.controller.generation == job.controllerGeneration && s.controller.active == job.actor
	s.controller.mu.Unlock()
	s.cloudPlanning.mu.Lock()
	current = current && s.cloudPlanning.revision == job.authRevision
	s.cloudPlanning.mu.Unlock()
	ticket, eligible := s.modes.MotionReviewTicket()
	engine := s.currentMotionEngine()
	if !current || !eligible || ticket != job.ticket || engine == nil {
		return false
	}
	state := engine.Snapshot()
	return state.Running && !state.Paused && !state.Starting && state.PlanID == job.state.PlanID && state.Generation == job.state.Generation
}

func (s *Server) runMotionReview(ctx context.Context, job motionReviewJob, cancel context.CancelFunc) (status motionReviewStatus) {
	status = motionReviewStatus{State: "skipped", Source: job.input.Source, Model: job.reviewer.Model}
	defer func() { status.ElapsedMillis = time.Since(job.queuedAt).Milliseconds() }()
	// Bind to the live controller without borrowing the completed HTTP request's
	// lifetime. Ownership loss synchronously cancels the reviewer and its handoff.
	detach := s.controller.bindRequest(job.actor, cancel)
	if detach == nil {
		return status
	}
	defer detach()
	if !s.motionReviewCurrent(ctx, job) {
		return status
	}
	admission, err := s.captureCloudAdmission(ctx, job.settings)
	if err != nil {
		return status
	}
	requests, err := s.chatLog.RecentUserRequestTimelineContext(ctx, admission.session)
	if err != nil {
		return status
	}
	job.input.RecentRequests, _ = chat.UserRequestTimeline(requests, time.Now())
	job.input.Context = s.chatMotionContext(job.settings.Motion, job.settings.LLM)
	capabilities := chatCapabilities(job.settings.LLM, nil)
	// The reviewed choice is local, but its correction uses the reviewer's
	// provider contract. Do not change the local model's prompt or history.
	capabilities.LayeredControlOperations = job.reviewer.IsHosted()
	patterns, err := s.chatPatternChoicesFor(capabilities)
	if err != nil {
		return status
	}
	provider, err := s.newLLMProvider(ctx, job.reviewer)
	if err != nil {
		status.State = "unavailable"
		return status
	}
	cloudCtx, _, release, err := s.acquireProviderRequest(ctx, llmRequestAutonomous, job.reviewer)
	if err != nil {
		return status
	}
	defer release()
	if !s.motionReviewCurrent(cloudCtx, job) {
		return status
	}
	service := chat.MotionReviewService{Provider: provider, Model: job.reviewer.Model, Capabilities: capabilities, Patterns: patterns}
	review, err := service.Complete(cloudCtx, job.input)
	if !s.motionReviewCurrent(cloudCtx, job) || s.validateCloudAdmission(cloudCtx, admission) != nil {
		return status
	}
	if err != nil {
		status.State = "unavailable"
		return status
	}
	if review.Verdict == "keep" {
		status.State = "kept"
		return status
	}
	if s.applyMotionReview(cloudCtx, job, admission, review.Motion) != nil {
		return status
	}
	status.State, status.Reason = "corrected", review.Reason
	return status
}

func (s *Server) applyMotionReview(ctx context.Context, job motionReviewJob, admission cloudPlanAdmission, command *chat.MotionCommand) error {
	// Use the same serialized control lane as immediate user commands. Stop
	// bypasses this lane and invalidates both the context and the engine epoch.
	release, err := s.commands.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	if !s.motionReviewCurrent(ctx, job) || s.validateCloudAdmission(ctx, admission) != nil {
		return errors.New("motion review expired")
	}
	if command == nil || (command.Action != chat.MotionActionTarget && command.Action != chat.MotionActionUpdate) {
		return errors.New("review cannot start or stop motion")
	}
	if err := s.validateChatMotionMode(command); err != nil {
		return err
	}
	engine := s.currentMotionEngine()
	target, err := s.chatMotionTarget(command, engine.Snapshot())
	if err != nil {
		return err
	}
	target.Source = job.state.Target.Source
	return s.modes.ApplyMotionReview(ctx, job.ticket, target, func(operation context.Context) (motion.ActiveMotionState, error) {
		return engine.ApplyTargetIfCurrent(operation, target, "second_opinion", job.state.PlanID)
	})
}
