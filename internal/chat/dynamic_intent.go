package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// describesUnsatisfiedDynamicMotion distinguishes a correction from a motion
// refusal. Broad negation remains the safe default, but conversational forms
// such as "you're not touching it" describe a failed running result; silently
// dropping the model's corrective update makes the assistant lie about motion.
func describesUnsatisfiedDynamicMotion(message string) bool {
	return hasIntentPrefix(message,
		"you're not ", "youre not ", "you are not ", "you aren't ", "you arent ",
		"you're still not", "youre still not", "you are still not", "still not",
		"that's not ", "thats not ", "that is not ", "it's not ", "its not ",
		"it is not ", "not enough", "not quite", "not there",
	)
}

func explicitlyRefusesDynamicMotion(message string) bool {
	return hasIntentPhrase(message,
		"do not move", "don't move", "dont move", "never move", "stop moving",
		"stop the motion", "pause", "wait", "hold still", "no motion",
		"not allowed", "not permitted", "not supposed", "not going to", "not yet", "not now",
	)
}

func dynamicUpdateBypassesNegationGate(message string, command MotionCommand) bool {
	if !motionIntentIsNegated(message) && !negatesDynamicSpeedChange(message) &&
		!negatesDynamicRangeChange(message) {
		return true
	}
	if !describesUnsatisfiedDynamicMotion(message) || explicitlyRefusesDynamicMotion(message) {
		return false
	}
	return command.SpeedPercent != nil || dynamicCommandChangesRange(command) ||
		command.VariationPercent != nil || command.SegmentSeconds != nil
}

func requestsFullDynamicRange(message string) bool {
	return hasIntentPhrase(message,
		"full range", "whole range", "entire range", "full length", "whole length",
		"entire length", "the full thing", "full thing", "the whole thing", "whole stroke",
	)
}

// validateDynamicRequestedCoverage checks only explicit position language in
// the current user turn. It does not plan motion or infer geometry from reply
// prose; it prevents a model from claiming a requested reach while leaving the
// authoritative running window unchanged or outside that region.
func validateDynamicRequestedCoverage(command *MotionCommand, context MotionContext, userMessage string) error {
	message := normalizeMotionIntent(userMessage)
	if !context.Running || context.Paused {
		return nil
	}
	positionRequested := requestsDynamicPositionChange(message)
	unsatisfiedCorrection := describesUnsatisfiedDynamicMotion(message) && !explicitlyRefusesDynamicMotion(message)
	if !positionRequested && !unsatisfiedCorrection {
		return nil
	}
	if command == nil || command.Action == MotionActionNone ||
		(command.Action != MotionActionUpdate && command.Action != MotionActionStart) {
		if !positionRequested {
			return nil
		}
		return errDynamicUpdateMissing
	}
	if positionRequested && !dynamicCommandChangesPosition(*command, context) {
		return fmt.Errorf("%w: a position request must change center/span or anchors from the current target", errDynamicPositionScope)
	}
	if err := validateDynamicPositionChangeScope(*command, context, message); err != nil {
		return err
	}
	if !positionRequested {
		return nil
	}

	minimum, maximum, ok := effectiveDynamicCommandWindow(command, context)
	if !ok {
		return errDynamicCoverage
	}
	return validateDynamicWindowCoverage(minimum, maximum, message)
}

func dynamicCommandChangesPosition(command MotionCommand, context MotionContext) bool {
	if len(command.Sections) > 0 {
		return true
	}
	if len(command.Anchors) > 0 {
		if len(command.Anchors) != len(context.Anchors) {
			return true
		}
		for index, anchor := range command.Anchors {
			if !strings.EqualFold(strings.TrimSpace(anchor), strings.TrimSpace(context.Anchors[index])) {
				return true
			}
		}
		return false
	}
	return command.CenterPercent != nil && *command.CenterPercent != context.CenterPercent ||
		command.SpanPercent != nil && *command.SpanPercent != context.SpanPercent
}

func validateDynamicWindowCoverage(minimum, maximum int, message string) error {
	if requestsFullDynamicRange(message) {
		if minimum > 12 || maximum < 88 {
			return fmt.Errorf("%w: full-range wording must reach both base and tip", errDynamicCoverage)
		}
		return nil
	}
	position, label, ok := requestedDynamicPosition(message)
	if !ok {
		return nil
	}
	if position == dynamicAnchorPositions["tip"] {
		if maximum < position-4 {
			return fmt.Errorf("%w: %s requires the outer window to reach at least %d percent", errDynamicCoverage, label, position-4)
		}
		return nil
	}
	if position == dynamicAnchorPositions["base"] {
		if minimum > position+4 {
			return fmt.Errorf("%w: %s requires the outer window to reach at most %d percent", errDynamicCoverage, label, position+4)
		}
		return nil
	}
	if minimum > position || maximum < position {
		return fmt.Errorf("%w: %s requires the outer window to include %d percent", errDynamicCoverage, label, position)
	}
	return nil
}

func validateDynamicPositionChangeScope(command MotionCommand, context MotionContext, message string) error {
	if !requestsDynamicPositionChange(message) &&
		(!describesUnsatisfiedDynamicMotion(message) || explicitlyRefusesDynamicMotion(message)) {
		return nil
	}
	if !requestsDynamicSpeedChange(message) && command.SpeedPercent != nil &&
		*command.SpeedPercent != context.SpeedPercent {
		return fmt.Errorf("%w: omit speed_percent unless the user also changes pace", errDynamicPositionScope)
	}
	if len(command.Sections) > 0 && !requestsDynamicSpanEnvelopeChange(message) &&
		!requestsDynamicTextureChange(message) {
		return fmt.Errorf("%w: use one geometry for a position-only correction; sections replace multiple motion axes", errDynamicPositionScope)
	}
	if !requestsDynamicSpanEnvelopeChange(message) {
		if command.SpanMinPercent != nil && *command.SpanMinPercent != context.SpanMinPercent {
			return fmt.Errorf("%w: omit span_min_percent unless the user also changes stroke-length variation", errDynamicPositionScope)
		}
		profile := strings.ToLower(strings.TrimSpace(context.SpanProfile))
		if command.SpanProfile != "" && command.SpanProfile != profile {
			return fmt.Errorf("%w: omit span_profile unless the user also changes stroke-length variation", errDynamicPositionScope)
		}
	}
	if !requestsDynamicTextureChange(message) && command.VariationPercent != nil &&
		*command.VariationPercent != context.VariationPercent {
		return fmt.Errorf("%w: omit variation_percent unless the user also changes center or rhythm texture", errDynamicPositionScope)
	}
	return nil
}

func requestsDynamicPositionChange(message string) bool {
	if message == "" || motionIntentIsConversation(message) || explicitlyRefusesDynamicMotion(message) {
		return false
	}
	if requestsFullDynamicRange(message) {
		return true
	}
	if _, _, ok := requestedDynamicPosition(message); !ok {
		return false
	}
	return describesUnsatisfiedDynamicMotion(message) || placesMotion(message) ||
		hasIntentPhrase(message, "touch", "touching", "reach", "reaching", "cover", "covering") ||
		containsAnyExact(message, "tip", "the tip", "base", "the base", "middle", "the middle")
}

func requestsDynamicSpanEnvelopeChange(message string) bool {
	return hasIntentPhrase(message,
		"stroke length", "stroke lengths", "range variation", "vary the range", "vary range",
		"breathe", "wander", "contrast", "fixed length", "fixed stroke", "steady strokes",
		"tight strokes", "broad strokes", "short strokes", "long strokes",
	)
}

func requestedDynamicPosition(message string) (int, string, bool) {
	switch {
	case hasIntentPhrase(message, "tip", "head", "top", "shallow", "shallowly"):
		return dynamicAnchorPositions["tip"], "tip", true
	case hasIntentPhrase(message, "base", "root", "bottom", "deep", "deeply"):
		return dynamicAnchorPositions["base"], "base", true
	case hasIntentPhrase(message, "upper"):
		return dynamicAnchorPositions["upper"], "upper region", true
	case hasIntentPhrase(message, "lower"):
		return dynamicAnchorPositions["lower"], "lower region", true
	case hasIntentPhrase(message, "middle", "mid", "shaft", "center", "centre"):
		return dynamicAnchorPositions["middle"], "middle region", true
	default:
		return 0, "", false
	}
}

func effectiveDynamicCommandWindow(command *MotionCommand, context MotionContext) (int, int, bool) {
	if command != nil && len(command.Sections) >= 2 {
		return effectiveDynamicSectionsWindow(command.Sections)
	}
	anchors := effectiveDynamicAnchorNames(command, context)
	if len(anchors) >= 2 {
		return dynamicAnchorNamesWindow(anchors)
	}
	return effectiveDynamicCenterSpanWindow(command, context)
}

func effectiveDynamicSectionsWindow(sections []DynamicSectionCommand) (int, int, bool) {
	minimum, maximum := 100, 0
	for _, section := range sections {
		sectionCommand := motionCommandFromSection(section)
		sectionMinimum, sectionMaximum, ok := effectiveDynamicCommandWindow(&sectionCommand, MotionContext{})
		if !ok {
			return 0, 0, false
		}
		minimum = min(minimum, sectionMinimum)
		maximum = max(maximum, sectionMaximum)
	}
	return minimum, maximum, maximum-minimum >= 20
}

func effectiveDynamicAnchorNames(command *MotionCommand, context MotionContext) []string {
	if command == nil {
		return context.Anchors
	}
	if len(command.Anchors) >= 2 {
		return command.Anchors
	}
	if command.CenterPercent != nil || command.SpanPercent != nil {
		return nil
	}
	return context.Anchors
}

func dynamicAnchorNamesWindow(anchors []string) (int, int, bool) {
	minimum, maximum := 100, 0
	for _, anchor := range anchors {
		position, ok := DynamicAnchorPosition(anchor)
		if !ok {
			return 0, 0, false
		}
		minimum = min(minimum, position)
		maximum = max(maximum, position)
	}
	return minimum, maximum, maximum-minimum >= 20
}

func effectiveDynamicCenterSpanWindow(command *MotionCommand, context MotionContext) (int, int, bool) {
	center, span := context.CenterPercent, context.SpanPercent
	if command != nil {
		if command.CenterPercent != nil {
			center = *command.CenterPercent
		}
		if command.SpanPercent != nil {
			span = *command.SpanPercent
		}
	}
	if center < 0 || center > 100 || span < 20 || span > 100 {
		return 0, 0, false
	}
	minimum := center - span/2
	maximum := minimum + span
	if minimum < 0 {
		maximum -= minimum
		minimum = 0
	}
	if maximum > 100 {
		minimum -= maximum - 100
		maximum = 100
	}
	return max(0, minimum), min(100, maximum), true
}

// contextualDynamicCorrectionIntent gives terse follow-through such as
// "you're still not" the last user-authored request it is correcting. The
// model already receives that history; semantic validation should not forget
// the axis just because the next user turn is elliptical.
func contextualDynamicCorrectionIntent(message string, history []llm.Message) string {
	normalized := normalizeMotionIntent(message)
	if !describesUnsatisfiedDynamicMotion(normalized) || explicitlyRefusesDynamicMotion(normalized) {
		return message
	}
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].Role != "user" {
			continue
		}
		previous := strings.TrimSpace(history[index].Content)
		if previous != "" {
			return previous + "\n" + message
		}
	}
	return message
}

// normalizeDynamicPaceOnlyResponse prevents copied snapshot fields from
// collapsing a running multi-section phrase. It is axis normalization, not a
// motion decision: when the current turn asks only for pace, every geometry,
// texture, and horizon field is inert model repetition and omitted values are
// authoritatively preserved by the backend.
func normalizeDynamicPaceOnlyResponse(raw, userMessage string) string {
	message := normalizeMotionIntent(userMessage)
	if !requestsDynamicSpeedChange(message) || requestsDynamicPositionChange(message) ||
		requestsDynamicSpanEnvelopeChange(message) || requestsDynamicTextureChange(message) {
		return raw
	}
	response, err := decodeAssistantResponse(raw)
	if err != nil || response.Motion == nil ||
		!strings.EqualFold(strings.TrimSpace(response.Motion.Action), MotionActionUpdate) {
		return raw
	}
	response.Motion.CenterPercent = nil
	response.Motion.SpanPercent = nil
	response.Motion.SpanMinPercent = nil
	response.Motion.SpanProfile = ""
	response.Motion.Anchors = nil
	response.Motion.VariationPercent = nil
	response.Motion.SegmentSeconds = nil
	response.Motion.Sections = nil
	encoded, err := json.Marshal(response)
	if err != nil {
		return raw
	}
	return string(encoded)
}

// normalizeDynamicPositionOnlyResponse treats fields on unrelated axes as
// inert model noise. The user authorized a geometry correction, not an
// unsolicited rewrite of pace, range texture, or variation. Strict JSON and
// the remaining geometry are still validated normally.
func normalizeDynamicPositionOnlyResponse(raw, userMessage string) string {
	message := normalizeMotionIntent(userMessage)
	if !requestsDynamicPositionChange(message) {
		return raw
	}
	response, err := decodeAssistantResponse(raw)
	if err != nil || response.Motion == nil ||
		!strings.EqualFold(strings.TrimSpace(response.Motion.Action), MotionActionUpdate) {
		return raw
	}
	if !requestsDynamicSpeedChange(message) {
		response.Motion.SpeedPercent = nil
		response.Motion.Intensity = nil
	}
	if !requestsDynamicSpanEnvelopeChange(message) {
		response.Motion.SpanMinPercent = nil
		response.Motion.SpanProfile = ""
	}
	if !requestsDynamicTextureChange(message) {
		response.Motion.VariationPercent = nil
	}
	response.Motion.SegmentSeconds = nil
	if response.Motion.CenterPercent == nil && response.Motion.SpanPercent == nil &&
		len(response.Motion.Anchors) == 0 && len(response.Motion.Sections) == 0 {
		// Keep the original response so semantic coverage can report the actual
		// missing position instead of reducing copied model noise to a generic
		// fieldless-update error. That gives the one repair turn useful guidance.
		return raw
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return raw
	}
	return string(encoded)
}
