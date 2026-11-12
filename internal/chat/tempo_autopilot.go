package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func tempoAutopilotContract(tier tempoTier, state *MotionContext, autoTiming bool) string {
	next := `"next":"soon"|"normal"|"later" says when to reconsider: soon after a change that needs checking, later while a plan unfolds.`
	if autoTiming {
		next = `next_check_seconds is how many seconds until you reconsider; next stays "normal".`
	}
	return autopilotMotionAuthority + `
RESPONSE CONTRACT: Reply with one JSON object: {"intent": string, "tempo": object or null, "air_steps": integer or null, "next": string}.
intent is a short private planning note naming what the next stretch should feel like, written before you choose numbers. tempo null keeps the current tempo; a hold is often right while a routine or sequence unfolds. ` + next + `
` + tempoDeviceGuide(tier, state) + `
Vary the tempo over the session the way a person would: builds, plateaus, easing off and returning. Recent conversation, explicit directions and boundaries always win.`
}

const tempoAutopilotGuard = `FINAL OUTPUT GUARD: Output only the JSON object with intent, tempo, air_steps and next.`

func tempoAutopilotSchema(tier tempoTier, state *MotionContext, autoTiming bool, floor int) json.RawMessage {
	var root map[string]any
	_ = json.Unmarshal(tempoResponseSchema(tier, state), &root)
	properties := root["properties"].(map[string]any)
	delete(properties, "reply")
	delete(properties, "stop")
	properties["intent"] = map[string]any{"type": "string"}
	properties["next"] = map[string]any{"type": "string", "enum": []string{"soon", "normal", "later"}}
	required := []string{"intent", "tempo", "air_steps", "next"}
	if autoTiming {
		properties[nextCheckField] = nextCheckSchema(floor)
		required = append(required, nextCheckField)
	}
	root["required"] = required
	encoded, _ := json.Marshal(root)
	return orderedTempoSchema(encoded, tier)
}

// parseTempoAutopilot validates an Autopilot tempo turn. A null tempo and
// air is a hold.
func parseTempoAutopilot(raw string, tier tempoTier, state *MotionContext) (AutopilotResponse, error) {
	root, err := uniqueLayeredObject(json.RawMessage(strings.TrimSpace(raw)))
	if err != nil {
		return AutopilotResponse{}, err
	}
	response := AutopilotResponse{Next: AutopilotTimingNormal, Variability: "settled"}
	if value, ok := root["intent"]; ok {
		_ = json.Unmarshal(value, &response.Intent)
	}
	if value, ok := root["next"]; ok && string(value) != "null" {
		var next string
		if err := json.Unmarshal(value, &next); err != nil || !validAutopilotTiming(AutopilotTiming(next)) {
			return response, errors.New(`next must be "soon", "normal" or "later"`)
		}
		response.Next = AutopilotTiming(next)
	}
	// Reuse the interactive validation with an inert reply and no Stop.
	shaped := map[string]json.RawMessage{"reply": json.RawMessage(`"autopilot"`), "stop": json.RawMessage(`false`)}
	for _, name := range []string{"tempo", "air_steps"} {
		if value, ok := root[name]; ok {
			shaped[name] = value
		}
	}
	encoded, _ := json.Marshal(shaped)
	validated, err := parseTempoResponse(string(encoded), tier, state)
	if err != nil {
		return response, err
	}
	response.Motion = validated.Motion
	return response, nil
}

// completeTempoAutopilot is an Autopilot motion turn when tempo is the motion
// (ADR 0071). Hosted output is held on failure; local output may repair once.
func (s AutopilotService) completeTempoAutopilot(ctx context.Context, request Request, kind AutopilotKind) (AutopilotResponse, error) {
	tier := tempoTierOf(s.Capabilities)
	autoTiming := s.autoTiming(kind)
	contract, guard := tempoAutopilotContract(tier, s.MotionContext, autoTiming), tempoAutopilotGuard
	schema := tempoAutopilotSchema(tier, s.MotionContext, autoTiming, s.Capabilities.AutoTimingFloor)
	if kind == AutopilotKindSpeech {
		contract, schema = s.tempoSpeechContract(tier)
		guard = `FINAL OUTPUT GUARD: Output only the JSON object with intent, reply, tempo, air_steps and next.`
		request.Message += "\n" + autopilotSpeechDecisionInstruction
	}
	system := tempoSystem(s.Prompt, s.Memories, s.ConversationContext, s.PromptBudget, s.MotionContext, s.Capabilities,
		contract, guard)
	messages := capabilityMessages(system, request.History, request.Message, s.Capabilities)
	temperature := s.Temperature
	if temperature <= 0 {
		temperature = 0.45
	}
	input := llm.ChatRequest{
		Messages:               messages,
		HistoryMessagesDropped: request.HistoryMessagesDropped + hostedHistoryDropped(request.History, messages, s.Capabilities),
		Model:                  s.Model,
		Temperature:            min(temperature, 1.2),
		TopP:                   chatTopP,
		RepeatPenalty:          chatRepeatPenalty,
		RepeatLastN:            chatRepeatLastN,
		MaxTokens:              s.MaxTokens,
		ReasoningMode:          s.ReasoningMode,
		ReasoningBudgetTokens:  s.ReasoningBudgetTokens,
		JSONSchema:             schema,
	}
	parse := func(raw string) (AutopilotResponse, error) {
		if kind == AutopilotKindSpeech {
			return parseTempoSpeech(raw, tier, s.MotionContext, s.Capabilities.Motion)
		}
		nextCheck := 0
		if autoTiming {
			raw, nextCheck = takeNextCheck(raw)
		}
		response, err := parseTempoAutopilot(raw, tier, s.MotionContext)
		response.NextCheckSeconds = nextCheck
		return response, err
	}
	raw, err := s.Provider.StreamChat(ctx, input, nil)
	if err != nil && !errors.Is(err, llm.ErrOutputTruncated) {
		return AutopilotResponse{}, err
	}
	response, parseErr := parse(raw)
	if parseErr == nil {
		return response, ctx.Err()
	}
	if s.Capabilities.PreserveConversationText {
		return AutopilotResponse{}, &llm.CloudError{Kind: "incomplete"}
	}
	repair := append(append([]llm.Message(nil), messages...),
		llm.Message{Role: "assistant", Content: truncateUTF8Bytes(strings.TrimSpace(raw), 4096)},
		llm.Message{Role: "user", Content: "That response was not valid: " + parseErr.Error() + ". Reply again with only the corrected JSON object."})
	input.Messages, input.PreserveTailMessages, input.Temperature, input.ReasoningMode = repair, 3, 0, "off"
	repairRaw, err := s.Provider.StreamChat(ctx, input, nil)
	if err != nil && !errors.Is(err, llm.ErrOutputTruncated) {
		return AutopilotResponse{}, err
	}
	if repaired, err := parse(repairRaw); err == nil {
		return repaired, ctx.Err()
	}
	return AutopilotResponse{}, fmt.Errorf("the Autopilot tempo reply was not valid: %w", parseErr)
}
