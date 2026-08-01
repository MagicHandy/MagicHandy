package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// AutopilotKind selects one of the two independent autonomous model contracts.
type AutopilotKind string

const (
	// AutopilotKindMotion selects the motion-only planning contract.
	AutopilotKindMotion AutopilotKind = "motion"
	// AutopilotKindSpeech selects the independent spoken-check-in contract.
	AutopilotKindSpeech AutopilotKind = "speech"
)

// AutopilotTiming is a semantic timing preference. Deterministic scheduler code
// maps it into the user's bounded cadence window; the model never emits seconds.
type AutopilotTiming string

const (
	// AutopilotTimingSoon asks the scheduler to sample from the early window.
	AutopilotTimingSoon AutopilotTiming = "soon"
	// AutopilotTimingNormal asks the scheduler to sample from the middle window.
	AutopilotTimingNormal AutopilotTiming = "normal"
	// AutopilotTimingLater asks the scheduler to sample from the late window.
	AutopilotTimingLater AutopilotTiming = "later"
)

// AutopilotResponse is a validated motion-plan or speech-check-in response.
type AutopilotResponse struct {
	Reply  string
	Motion *MotionCommand
	Next   AutopilotTiming
	// Variability is how much the target should wander before the next boundary.
	// Unlike Next it is optional on the wire: a model that omits it gets ordinary
	// texture rather than a rejected turn, because it was added after the
	// contract shipped and a missing field is not a malformed response.
	Variability string
	// Arc is a request to move visible session buildup by one bounded step. It is
	// read only while the arc is enabled and can never set the value.
	Arc string
}

// AutopilotService runs the dedicated autonomous contracts through the same
// provider, personalization, history, semantic motion validation, and one-shot
// repair policy used by interactive chat.
type AutopilotService struct {
	Provider              llm.Provider
	Prompt                PromptSet
	Model                 string
	MaxTokens             int
	ReasoningMode         string
	ReasoningBudgetTokens int
	Memories              []string
	Patterns              []PatternChoice
	MotionContext         *MotionContext
	ConversationContext   *ConversationContext
	Capabilities          Capabilities
}

// Complete runs one autonomous decision and repairs malformed output once.
func (s AutopilotService) Complete(ctx context.Context, kind AutopilotKind, request Request) (AutopilotResponse, error) {
	if s.Provider == nil {
		return AutopilotResponse{}, errors.New("LLM provider is required")
	}
	if kind != AutopilotKindMotion && kind != AutopilotKindSpeech {
		return AutopilotResponse{}, fmt.Errorf("unknown Autopilot request kind %q", kind)
	}
	message, err := ValidateUserMessage(request.Message)
	if err != nil {
		return AutopilotResponse{}, err
	}
	prompt := s.Prompt
	if strings.TrimSpace(prompt.ID) == "" {
		prompt, _ = BuiltinPromptSetByID(DefaultPromptSetID)
	}
	system := composeAutopilotSystem(
		prompt,
		s.Memories,
		s.Patterns,
		s.Capabilities,
		s.MotionContext,
		s.ConversationContext,
		kind,
	)
	messages := buildMessages(system, request.History, message)
	raw, err := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:              messages,
		Model:                 s.Model,
		Temperature:           0.45,
		TopP:                  chatTopP,
		RepeatPenalty:         chatRepeatPenalty,
		RepeatLastN:           chatRepeatLastN,
		MaxTokens:             s.MaxTokens,
		ReasoningMode:         s.ReasoningMode,
		ReasoningBudgetTokens: s.ReasoningBudgetTokens,
	}, nil)
	truncated := errors.Is(err, llm.ErrOutputTruncated)
	if err != nil && !truncated {
		return AutopilotResponse{}, err
	}
	response, parseErr := s.parse(raw, kind)
	if parseErr == nil {
		return response, nil
	}
	if truncated {
		parseErr = fmt.Errorf("autopilot response was truncated before valid JSON: %w", parseErr)
	}

	repairContext := strings.TrimSpace(raw)
	if repairContext == "" {
		repairContext = emptyRepairContext
	}
	repairMessages := append([]llm.Message(nil), messages...)
	repairMessages = append(repairMessages, llm.Message{Role: "assistant", Content: repairContext})
	repairMessages = append(repairMessages, llm.Message{
		Role:    "user",
		Content: autopilotRepairPrompt(prompt.ID, kind, parseErr),
	})
	repairedRaw, repairErr := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:      repairMessages,
		Model:         s.Model,
		Temperature:   0,
		MaxTokens:     s.MaxTokens,
		ReasoningMode: "off",
	}, nil)
	if repairErr != nil && !errors.Is(repairErr, llm.ErrOutputTruncated) {
		return AutopilotResponse{}, fmt.Errorf("repair Autopilot response: %w", repairErr)
	}
	repaired, repairParseErr := s.parse(repairedRaw, kind)
	if repairParseErr != nil {
		return AutopilotResponse{}, fmt.Errorf("autopilot response stayed malformed after repair: %w", repairParseErr)
	}
	return repaired, nil
}

func (s AutopilotService) parse(raw string, kind AutopilotKind) (AutopilotResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AutopilotResponse{}, errors.New("autopilot response is empty")
	}
	var response AutopilotResponse
	switch kind {
	case AutopilotKindMotion:
		var wire struct {
			Motion      *MotionCommand  `json:"motion,omitempty"`
			Next        AutopilotTiming `json:"next"`
			Variability string          `json:"variability,omitempty"`
			Arc         string          `json:"arc,omitempty"`
		}
		if err := decodeAutopilotJSON(raw, &wire); err != nil {
			return AutopilotResponse{}, err
		}
		response.Motion = wire.Motion
		response.Next = wire.Next
		response.Variability = strings.TrimSpace(wire.Variability)
		response.Arc = strings.TrimSpace(wire.Arc)
	case AutopilotKindSpeech:
		var wire struct {
			Reply       string          `json:"reply"`
			Motion      *MotionCommand  `json:"motion,omitempty"`
			Next        AutopilotTiming `json:"next"`
			Variability string          `json:"variability,omitempty"`
			Arc         string          `json:"arc,omitempty"`
		}
		if err := decodeAutopilotJSON(raw, &wire); err != nil {
			return AutopilotResponse{}, err
		}
		response.Reply = strings.TrimSpace(wire.Reply)
		response.Motion = wire.Motion
		response.Next = wire.Next
		response.Variability = strings.TrimSpace(wire.Variability)
		response.Arc = strings.TrimSpace(wire.Arc)
		if response.Reply == "" {
			return AutopilotResponse{}, errors.New("autopilot speech reply is required")
		}
	}
	if !validAutopilotTiming(response.Next) {
		return AutopilotResponse{}, fmt.Errorf("unknown Autopilot timing %q", response.Next)
	}

	// Reuse the interactive semantic validator with an inert non-empty reply.
	// Capability enforcement happens before validation, so a speech authority
	// that did not advertise motion cannot smuggle it through malformed output.
	validated := AssistantResponse{Reply: "autopilot", Motion: response.Motion}
	enforceCapabilities(&validated, s.Capabilities)
	var currentSpeed *int
	if s.MotionContext != nil && s.MotionContext.Running && s.MotionContext.SpeedPercent >= 1 &&
		s.MotionContext.SpeedPercent <= 100 {
		speed := s.MotionContext.SpeedPercent
		currentSpeed = &speed
	}
	preserveCurrentPatternSpeed(&validated, currentSpeed)
	patternsEnabled := s.Capabilities.Motion && s.Capabilities.Patterns
	if err := validateAssistantResponse(&validated, s.Patterns, patternsEnabled); err != nil {
		return AutopilotResponse{}, err
	}
	if validated.Motion != nil && validated.Motion.Action != MotionActionNone &&
		validated.Motion.Action != MotionActionTarget {
		return AutopilotResponse{}, errors.New("autopilot motion action must be target or none")
	}
	response.Motion = validated.Motion
	return response, nil
}

func decodeAutopilotJSON(raw string, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("autopilot response must be strict JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("autopilot response must contain exactly one JSON object")
	}
	return nil
}

func validAutopilotTiming(timing AutopilotTiming) bool {
	return timing == AutopilotTimingSoon || timing == AutopilotTimingNormal || timing == AutopilotTimingLater
}

func composeAutopilotSystem(
	set PromptSet,
	memories []string,
	patterns []PatternChoice,
	capabilities Capabilities,
	motionContext *MotionContext,
	conversationContext *ConversationContext,
	kind AutopilotKind,
) string {
	composition := composePrompt(set, memories, patterns, capabilities, motionContext, conversationContext)
	sections := make([]string, 0, len(composition.Sections))
	for _, section := range composition.Sections {
		switch section.ID {
		case "response_contract":
			sections = append(sections, autopilotContract(kind, capabilities))
		case "output_guard":
			sections = append(sections, autopilotOutputGuard(kind))
		case "voice_identity", "reaction_style", "voice_check", "language_reminder":
			if kind == AutopilotKindSpeech {
				sections = append(sections, section.Text)
			}
		default:
			sections = append(sections, section.Text)
		}
	}
	return strings.Join(sections, "\n\n")
}

func autopilotContract(kind AutopilotKind, capabilities Capabilities) string {
	var builder strings.Builder
	builder.WriteString("Return exactly one JSON object and no markdown, code fences, prose outside JSON, or extra keys.\n")
	if kind == AutopilotKindSpeech {
		builder.WriteString(`The object requires a non-empty "reply" string and "next":"soon"|"normal"|"later".`)
	} else {
		builder.WriteString(`The object requires "next":"soon"|"normal"|"later". Do not include a "reply" field.`)
	}
	builder.WriteString("\nThe timing value is a relative preference only. Never emit seconds, a duration, or a deadline.\n")
	if !capabilities.Motion {
		builder.WriteString(`Motion control is unavailable for this turn. Do not include a "motion" field.`)
		return builder.String()
	}
	builder.WriteString(`The optional "motion" value may be {"action":"none"} or use action "target" to change active motion. Never use "start" or "stop".`)
	builder.WriteString("\nOmitted target fields preserve the live target. Never invent device commands, pattern IDs, URLs, or transport details.")
	if capabilities.Patterns {
		builder.WriteString(` To select an enabled pattern, include "pattern_id" and "intensity" together and omit "speed_percent".`)
	} else {
		builder.WriteString(` Pattern selection is unavailable. Do not include "pattern_id" or "intensity"; use "speed_percent" for pace changes.`)
	}
	if capabilities.AreaFocus {
		builder.WriteString(` "area" may be "tip", "shaft", "base", or "full".`)
	} else {
		builder.WriteString(` Area focus is unavailable. Do not include "area".`)
	}
	return builder.String()
}

func autopilotOutputGuard(kind AutopilotKind) string {
	if kind == AutopilotKindSpeech {
		return `FINAL OUTPUT: return only {"reply":"<one short in-character line>","next":"soon|normal|later"} plus an optional allowed "motion" object.`
	}
	return `FINAL OUTPUT: return only {"next":"soon|normal|later"} plus an optional allowed "motion" object. No reply text.`
}

func autopilotRepairPrompt(promptID string, kind AutopilotKind, parseError error) string {
	return fmt.Sprintf(`Repair your previous MagicHandy Autopilot %s response.

Return exactly one JSON object matching the Autopilot contract in the system prompt. Do not add markdown, comments, code fences, or extra keys.

Validation error:
%s

Prompt set:
%s`, kind, strings.TrimSpace(parseError.Error()), promptID)
}
