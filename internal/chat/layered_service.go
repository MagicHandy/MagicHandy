package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func layeredContextScore(state *MotionContext) (motion.FlowSpec, config.MotionSettings) {
	limits := config.DefaultSettings().Motion
	speed := 25
	if state != nil {
		if state.SpeedMinPercent > 0 {
			limits.SpeedMinPercent = state.SpeedMinPercent
		}
		if state.SpeedMaxPercent > 0 {
			limits.SpeedMaxPercent = state.SpeedMaxPercent
		}
		if state.SpeedPercent > 0 {
			speed = state.SpeedPercent
		}
		if state.Layered != nil {
			return *motion.CloneFlowSpec(state.Layered), limits
		}
	}
	return FreshLayeredScore(max(limits.SpeedMinPercent, min(speed, limits.SpeedMaxPercent))), limits
}

func layeredContextInstructions(state MotionContext) string {
	score, limits := layeredContextScore(&state)
	encoded, _ := json.Marshal(map[string]any{
		"current_score": layeredScoreContext(score), "running": state.Running, "paused": state.Paused,
		"saved_limits":                      map[string]int{"speed_min_percent": limits.SpeedMinPercent, "speed_max_percent": limits.SpeedMaxPercent},
		"engine_envelope":                   state.Envelope,
		"recent_user_requests_oldest_first": state.UserRequests,
	})
	return "Authoritative Layered state, refreshed for this turn:\n" + string(encoded) + "\n" + labPlanningContextGuide +
		"A stopped device starts only for a direct motion request; a paused device cannot be resumed by model edits. Ordinary conversation and questions require reply only."
}

func (s Service) completeLayered(ctx context.Context, request Request, emit func(StreamEvent) error) (Result, error) {
	capabilities := s.capabilities()
	prompt := s.Prompt
	if strings.TrimSpace(prompt.ID) == "" {
		prompt, _ = BuiltinPromptSetByID(DefaultPromptSetID)
	}
	state := MotionContext{}
	if s.MotionContext != nil {
		state = *s.MotionContext
	}
	state.MotionMode = MotionModeLayered
	system := composeSystem(prompt, s.Memories, nil, capabilities, &state, s.ConversationContext)
	current, limits := layeredContextScore(&state)
	schema := LayeredResponseSchema(limits, capabilities.MoodTracking)
	if !capabilities.Motion {
		schema = nil
	}
	maxTokens := s.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1536
	}
	raw, err := s.Provider.StreamChat(ctx, llm.ChatRequest{Messages: buildMessages(system, request.History, request.Message),
		Model: s.Model, Temperature: 0.15, MaxTokens: maxTokens, ReasoningMode: s.ReasoningMode,
		ReasoningBudgetTokens: s.ReasoningBudgetTokens, JSONSchema: schema}, func(delta string) error {
		return emitEvent(emit, StreamEvent{Type: "delta", Phase: "initial", Text: delta})
	})
	result := Result{Raw: raw}
	if err != nil {
		return result, err
	}
	response, next, changed, err := ParseLayeredReply(raw, current, limits)
	if err == nil {
		err = s.authorizeLayeredReply(&response, current, next, changed, request.Message, state)
	}
	if err != nil {
		result.Malformed, result.InitialMalformed, result.MalformedError = true, true, err.Error()
		_ = emitEvent(emit, StreamEvent{Type: "malformed", Phase: "initial", Error: err.Error()})
		return result, fmt.Errorf("layered response rejected: %w", err)
	}
	if !capabilities.MoodTracking {
		response.NewMood = nil
	}
	result.Response = response
	return result, nil
}

func (s Service) authorizeLayeredReply(response *AssistantResponse, before, after motion.FlowSpec, changed []string, message string, state MotionContext) error {
	if !s.capabilities().Motion {
		return nil
	}
	if state.Paused {
		if len(changed) > 0 {
			return errors.New("layered motion is paused; edits cannot resume it")
		}
		return nil
	}
	authorized := s.TrustedMotionInput || layeredUserMayEdit(message, state.Running, before, after)
	if !authorized {
		if len(changed) > 0 {
			return errors.New("the response changed motion outside the current request")
		}
		return nil
	}
	if state.Running && len(changed) == 0 {
		return nil
	}
	action := MotionActionStart
	if state.Running {
		action = MotionActionUpdate
	}
	response.Motion = &MotionCommand{Action: action, Layered: motion.CloneFlowSpec(&after)}
	return nil
}

func layeredUserMayEdit(message string, running bool, before, after motion.FlowSpec) bool {
	if !running {
		return userAuthorizesMotion(message, MotionActionStart)
	}
	message = normalizeMotionIntent(message)
	if motionIntentIsConversation(message) {
		return false
	}
	// Scoped preservation is permitted only when the actual output obeys it.
	if negatesDynamicSpeedChange(message) && before.SpeedPercent == after.SpeedPercent {
		for _, phrase := range []string{"do not change the pace", "don't change the pace", "do not change speed", "don't change speed", "without changing speed", "without changing the pace"} {
			message = strings.ReplaceAll(message, phrase, "")
		}
	}
	return !motionIntentIsNegated(message) && (userAuthorizesMotion(message, MotionActionUpdate) || hasIntentPhrase(message,
		"alternate", "alternating", "vary", "varying", "variation", "layer", "layers", "reach", "range", "stroke", "strokes", "jerk", "tip", "base", "center", "gentle", "gently", "gentler", "slower", "faster", "natural", "organic", "rhythm", "evolve"))
}

func (s AutopilotService) completeLayeredAutopilot(ctx context.Context, kind AutopilotKind, request Request) (AutopilotResponse, error) {
	service := Service{Provider: s.Provider, Prompt: s.Prompt, Model: s.Model, MaxTokens: s.MaxTokens,
		ReasoningMode: s.ReasoningMode, ReasoningBudgetTokens: s.ReasoningBudgetTokens, Memories: s.Memories,
		MotionContext: s.MotionContext, ConversationContext: s.ConversationContext, Capabilities: &s.Capabilities, TrustedMotionInput: true}
	if kind == AutopilotKindMotion {
		var requests []string
		if s.MotionContext != nil {
			requests = s.MotionContext.UserRequests
		}
		request.Message = LayeredContinuationMessage(requests)
	}
	result, err := service.completeLayered(ctx, request, nil)
	if err != nil {
		return AutopilotResponse{}, err
	}
	command := result.Response.Motion
	if kind == AutopilotKindMotion && command != nil && s.MotionContext != nil && LayeredExactHoldRequested(s.MotionContext.UserRequests) {
		return AutopilotResponse{}, errors.New("layered Autopilot changed an explicitly fixed score")
	}
	if command != nil {
		command.Action = MotionActionUpdate
	}
	if kind == AutopilotKindMotion && command != nil && command.Layered != nil && s.MotionContext != nil && s.MotionContext.Layered != nil {
		before, after := s.MotionContext.Layered, command.Layered
		if after.SpeedPercent > before.SpeedPercent || after.MinPercent < before.MinPercent || after.MaxPercent > before.MaxPercent {
			return AutopilotResponse{}, errors.New("layered Autopilot cannot raise speed or widen the requested band")
		}
	}
	return AutopilotResponse{Reply: result.Response.Reply, Motion: command, Next: AutopilotTimingNormal, Variability: "settled"}, nil
}
