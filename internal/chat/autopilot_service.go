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
	// Motion turns require the model to choose it explicitly so a scheduler hold
	// cannot acquire backend-invented movement after the model answered.
	Variability string
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
	var patternErr unknownPatternError
	if errors.As(parseErr, &patternErr) {
		// The model selected outside the turn-specific autonomous catalog. A live
		// 26B repeated the same unavailable ID during repair, doubling decision
		// latency without recovering. Let the existing planner fallback choose a
		// bounded target immediately; interactive chat retains its repair path.
		return AutopilotResponse{}, parseErr
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
	response, err := decodeAutopilotResponse(raw, kind)
	if err != nil {
		return AutopilotResponse{}, err
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
	dynamicEnabled := s.Capabilities.Motion && s.Capabilities.MotionMode == MotionModeDynamic
	// Give repair the complete effective-envelope invariant on its first failure.
	// Running updates often preserve their outer span, which the field-local
	// range validator cannot see; reporting only "20-100" led small local models
	// to repair an inner floor into a still-invalid value above a narrow span.
	if dynamicEnabled {
		if err := validateDynamicSpanEnvelopeState(validated.Motion, s.MotionContext); err != nil {
			return AutopilotResponse{}, err
		}
	}
	if err := validateAssistantResponse(&validated, s.Patterns, patternsEnabled, dynamicEnabled); err != nil {
		return AutopilotResponse{}, err
	}
	if validated.Motion != nil && validated.Motion.Action != MotionActionNone &&
		validated.Motion.Action != MotionActionTarget && validated.Motion.Action != MotionActionUpdate {
		return AutopilotResponse{}, errors.New("autopilot motion action must be target, update, or none")
	}
	response.Motion = validated.Motion
	if err := normalizeAutopilotSpeechVariability(&response, kind); err != nil {
		return AutopilotResponse{}, err
	}
	return response, nil
}

func decodeAutopilotResponse(raw string, kind AutopilotKind) (AutopilotResponse, error) {
	switch kind {
	case AutopilotKindMotion:
		var wire struct {
			Motion      *MotionCommand  `json:"motion,omitempty"`
			Next        AutopilotTiming `json:"next"`
			Variability string          `json:"variability,omitempty"`
		}
		if err := decodeAutopilotJSON(raw, &wire); err != nil {
			return AutopilotResponse{}, err
		}
		response := AutopilotResponse{
			Motion: wire.Motion, Next: wire.Next,
			Variability: strings.TrimSpace(wire.Variability),
		}
		if !validAutopilotVariability(response.Variability) {
			return AutopilotResponse{}, fmt.Errorf("unknown Autopilot variability %q", response.Variability)
		}
		return response, nil
	case AutopilotKindSpeech:
		var wire struct {
			Reply       string          `json:"reply"`
			Motion      *MotionCommand  `json:"motion,omitempty"`
			Next        AutopilotTiming `json:"next"`
			Variability string          `json:"variability,omitempty"`
		}
		if err := decodeAutopilotJSON(raw, &wire); err != nil {
			return AutopilotResponse{}, err
		}
		response := AutopilotResponse{
			Reply: strings.TrimSpace(wire.Reply), Motion: wire.Motion, Next: wire.Next,
			Variability: strings.TrimSpace(wire.Variability),
		}
		if response.Reply == "" {
			return AutopilotResponse{}, errors.New("autopilot speech reply is required")
		}
		return response, nil
	default:
		return AutopilotResponse{}, fmt.Errorf("unknown Autopilot kind %q", kind)
	}
}

func normalizeAutopilotSpeechVariability(response *AutopilotResponse, kind AutopilotKind) error {
	if kind != AutopilotKindSpeech {
		return nil
	}
	if response.Motion != nil && (response.Motion.Action == MotionActionTarget || response.Motion.Action == MotionActionUpdate) {
		if !validAutopilotVariability(response.Variability) {
			return fmt.Errorf("autopilot speech target requires settled, normal, or restless variability, got %q", response.Variability)
		}
		return nil
	}
	// Variability has no meaning without admitted target motion. Clear it after
	// capability enforcement so an unadvertised motion object stays inert instead
	// of leaking a scheduler preference.
	response.Variability = ""
	return nil
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

func validAutopilotVariability(variability string) bool {
	return variability == "settled" || variability == "normal" || variability == "restless"
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
			sections = append(sections, autopilotOutputGuard(kind, capabilities))
		case "motion_context":
			// AutopilotMotionMessage and AutopilotSpeechMessage carry the
			// authoritative state for their dedicated contracts. Interactive
			// start/target instructions contradict Autopilot's update-only
			// authority and must not enter this prompt.
			continue
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
		builder.WriteString(`The object requires "next":"soon"|"normal"|"later" and "variability":"settled"|"normal"|"restless". Do not include a "reply" field.`)
	}
	if capabilities.MotionMode == MotionModeDynamic && kind == AutopilotKindMotion {
		builder.WriteString("\nThe timing value is a relative fallback preference. segment_seconds is allowed only inside dynamic motion and stays inside the supplied user bounds.\n")
	} else {
		builder.WriteString("\nThe timing value is a relative preference only. Never emit seconds, a duration, or a deadline.\n")
	}
	builder.WriteByte('\n')
	if !capabilities.Motion {
		builder.WriteString(`Motion control is unavailable for this turn. Do not include a "motion" field.`)
		return builder.String()
	}
	if capabilities.MotionMode == MotionModeDynamic {
		builder.WriteString(`The optional "motion" value may be {"action":"none"} or use action "update" to provide or change Dynamic motion. Never use "start", "target", or "stop".`)
	} else {
		builder.WriteString(`The optional "motion" value may be {"action":"none"} or use action "target" to change active motion. Never use "start", "update", or "stop".`)
	}
	if kind == AutopilotKindSpeech {
		builder.WriteString(` A target motion requires "variability":"settled"|"normal"|"restless"; omit variability when motion is omitted or uses "none".`)
	}
	builder.WriteString("\nOmitted target fields preserve the live target. Never invent device commands, pattern IDs, URLs, or transport details.")
	if capabilities.MotionMode == MotionModeDynamic {
		builder.WriteString(` Dynamic updates may contain speed_percent; center_percent with span_percent or 2-6 named anchors; span_min_percent with span_profile "breathe", "wander", "contrast", or "steady"; variation_percent; segment_seconds; or 2-4 sections. A variable span profile requires span_min_percent of at least 20 and strictly below the effective widest span; widen span_percent in the same update if needed. Each section uses center/span or anchors plus optional span envelope, variation_percent, and cycles. Sections replace single-phrase geometry. Use anchors or center/span, never both.`)
		return builder.String()
	}
	if capabilities.Patterns {
		builder.WriteString(` Pattern selects shape and speed_percent selects pace. A pattern_id may omit speed_percent to preserve the live pace; include both only when changing both.`)
	} else {
		builder.WriteString(` Pattern selection is unavailable. Do not include "pattern_id"; use "speed_percent" for pace changes.`)
	}
	if capabilities.AreaFocus {
		builder.WriteString(` "area" may be "tip", "shaft", "base", or "full".`)
	} else {
		builder.WriteString(` Area focus is unavailable. Do not include "area".`)
	}
	return builder.String()
}

func autopilotOutputGuard(kind AutopilotKind, capabilities Capabilities) string {
	if kind == AutopilotKindSpeech {
		if capabilities.Motion {
			return `FINAL OUTPUT: return only {"reply":"<one short in-character line>","next":"soon|normal|later"} plus optional "arc" or allowed target "motion" with matching "variability".`
		}
		return `FINAL OUTPUT: return only {"reply":"<one short in-character line>","next":"soon|normal|later"} plus an optional "arc" field. No motion.`
	}
	return `FINAL OUTPUT: return only {"next":"soon|normal|later","variability":"settled|normal|restless"} plus optional allowed "motion" and "arc" fields. No reply text.`
}

func autopilotRepairPrompt(promptID string, kind AutopilotKind, parseError error) string {
	return fmt.Sprintf(`Repair your previous MagicHandy Autopilot %s response.

Return exactly one JSON object matching the Autopilot contract in the system prompt. Do not add markdown, comments, code fences, or extra keys.

Validation error:
%s

Prompt set:
%s`, kind, strings.TrimSpace(parseError.Error()), promptID)
}
