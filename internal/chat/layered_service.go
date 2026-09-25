package chat

import (
	"context"
	"encoding/json"
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
		if state.Layered != nil && ((state.Layered.Gesture != nil) == (state.MotionMode == MotionModeCreativeV2)) {
			return *motion.CloneFlowSpec(state.Layered), limits
		}
	}
	if state != nil && state.MotionMode == MotionModeCreativeV2 {
		return FreshCreativeV2Score(max(limits.SpeedMinPercent, min(speed, limits.SpeedMaxPercent))), limits
	}
	return FreshLayeredScore(max(limits.SpeedMinPercent, min(speed, limits.SpeedMaxPercent))), limits
}

func layeredContextInstructions(state MotionContext) string {
	score, limits := layeredContextScore(&state)
	var scoreContext any = layeredScoreContext(score)
	if state.MotionMode == MotionModeCreativeV2 {
		scoreContext = creativeV2ScoreContext(score)
	}
	context := map[string]any{
		"current_score": scoreContext, "running": state.Running, "paused": state.Paused,
		"saved_limits":                      map[string]int{"speed_min_percent": limits.SpeedMinPercent, "speed_max_percent": limits.SpeedMaxPercent},
		"engine_envelope":                   state.Envelope,
		"recent_user_requests_oldest_first": state.UserRequests,
	}
	if state.Autopilot {
		context["autopilot"] = "composing between chat turns"
		if state.StandingHold {
			context["autopilot"] = "holding the motion unchanged at the user's request"
		}
	}
	encoded, _ := json.Marshal(context)
	return "Authoritative continuous motion state, refreshed for this turn:\n" + string(encoded) + "\n" + labPlanningContextGuide +
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
	state.MotionMode = capabilities.MotionMode
	current, limits := layeredContextScore(&state)
	// The prompt and parser must share the same freshly seeded starting score.
	state.Layered = &current
	system := composeSystem(prompt, s.Memories, nil, capabilities, &state, s.ConversationContext, s.PromptBudget)
	schema := LayeredResponseSchema(limits, capabilities.MoodTracking)
	parser := ParseLayeredReply
	if capabilities.MotionMode == MotionModeCreativeV2 {
		schema, parser = CreativeV2ResponseSchema(limits, capabilities.MoodTracking), ParseCreativeV2Reply
	}
	if !s.TrustedMotionInput {
		guard := continuousOutputGuard(capabilities)
		system = strings.TrimSuffix(system, guard) + continuousActionGuideFor(state) + "\n\n" + guard
		schema = continuousActionSchema(schema, state)
	}
	if !capabilities.Motion {
		schema = nil
	}
	maxTokens := s.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1536
	}
	temperature := chatTemperature
	if s.TrustedMotionInput && s.AutonomousTemperature > 0 {
		temperature = min(s.AutonomousTemperature, 1.2)
	}
	raw, err := s.Provider.StreamChat(ctx, llm.ChatRequest{Messages: continuousMessages(system, request.History, request.Message),
		Model: s.Model, Temperature: temperature, TopP: chatTopP, RepeatPenalty: chatRepeatPenalty, RepeatLastN: chatRepeatLastN,
		MaxTokens: maxTokens, ReasoningMode: s.ReasoningMode,
		ReasoningBudgetTokens: s.ReasoningBudgetTokens, JSONSchema: schema}, func(delta string) error {
		return emitEvent(emit, StreamEvent{Type: "delta", Phase: "initial", Text: delta})
	})
	result := Result{Raw: raw}
	if err != nil {
		return result, err
	}
	response, next, changed, err := parser(raw, current, limits)
	if err == nil {
		err = s.authorizeLayeredReply(&response, next, changed, state)
	}
	if err != nil {
		result.Malformed, result.InitialMalformed, result.MalformedError = true, true, err.Error()
		_ = emitEvent(emit, StreamEvent{Type: "malformed", Phase: "initial", Error: err.Error()})
		return result, fmt.Errorf("%s response rejected: %w", capabilities.MotionMode, err)
	}
	if !capabilities.MoodTracking {
		response.NewMood = nil
	}
	if s.TrustedMotionInput || !state.Autopilot {
		// A standing wish counts only where live chat was asked for it.
		response.StayUnchanged = nil
	}
	result.Response = response
	return result, nil
}

func (s AutopilotService) completeLayeredAutopilot(ctx context.Context, kind AutopilotKind, request Request) (AutopilotResponse, error) {
	service := Service{Provider: s.Provider, Prompt: s.Prompt, Model: s.Model, MaxTokens: s.MaxTokens,
		ReasoningMode: s.ReasoningMode, ReasoningBudgetTokens: s.ReasoningBudgetTokens, Memories: s.Memories,
		MotionContext: s.MotionContext, ConversationContext: s.ConversationContext, Capabilities: &s.Capabilities, TrustedMotionInput: true,
		AutonomousTemperature: s.Temperature, PromptBudget: s.PromptBudget}
	if kind == AutopilotKindMotion {
		continuation := LayeredContinuationMessage()
		if s.Capabilities.MotionMode == MotionModeCreativeV2 {
			continuation = CreativeV2ContinuationMessage()
		}
		request.Message = request.Message + "\n\n" + continuation
	} else {
		request.Message = strings.TrimSuffix(request.Message, autopilotSpeechTimingInstruction) +
			"Use this continuous mode's edits/reply contract. Omit timing fields; its scheduler retains normal bounded timing."
	}
	result, err := service.completeLayered(ctx, request, nil)
	if err != nil {
		return AutopilotResponse{}, err
	}
	command := result.Response.Motion
	// Saved limits were already enforced by the parser. What the human asked
	// for is honored by the model's judgment of recent requests, not by a
	// word-matched lock here.
	if kind == AutopilotKindMotion && command != nil && s.MotionContext != nil {
		relaxUnchosenAccents(s.MotionContext.Layered, command.Layered)
	}
	if command != nil {
		command.Action = MotionActionUpdate
	}
	return AutopilotResponse{Reply: result.Response.Reply, Motion: command, Next: AutopilotTimingNormal, Variability: "settled"}, nil
}
