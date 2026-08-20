package chat

import (
	"encoding/json"
	"strings"
)

// MotionContext is the transport-neutral motion state supplied to one model
// turn. It contains only semantic state and the user's configured speed band;
// the model never receives device or transport details.
type MotionContext struct {
	Running          bool
	Paused           bool
	PatternID        string
	ProgramID        string
	RecentPatternIDs []string
	SpeedPercent     int
	Area             string
	SpeedMinPercent  int
	SpeedMaxPercent  int
	MotionMode       MotionMode
	CenterPercent    int
	SpanPercent      int
	Anchors          []string
	VariationPercent int
	SegmentSeconds   int
}

type promptSpeedRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type promptSpeedBands struct {
	Low    [2]int `json:"low"`
	Middle [2]int `json:"middle"`
	High   [2]int `json:"high"`
}

type promptMotionContext struct {
	State            string           `json:"state"`
	PatternID        string           `json:"pattern_id,omitempty"`
	ProgramID        string           `json:"program_id,omitempty"`
	RecentPatternIDs []string         `json:"recent_pattern_ids,omitempty"`
	SpeedPercent     int              `json:"speed_percent,omitempty"`
	Area             string           `json:"area,omitempty"`
	MotionMode       MotionMode       `json:"motion_mode,omitempty"`
	CenterPercent    int              `json:"center_percent,omitempty"`
	SpanPercent      int              `json:"span_percent,omitempty"`
	Anchors          []string         `json:"anchors,omitempty"`
	VariationPercent int              `json:"variation_percent,omitempty"`
	SegmentSeconds   int              `json:"segment_seconds,omitempty"`
	SpeedLimits      promptSpeedRange `json:"speed_limits"`
	SpeedBands       promptSpeedBands `json:"speed_bands"`
}

func motionContextInstructions(context MotionContext, capabilities Capabilities, patterns []PatternChoice) string {
	data := normalizedPromptMotionContext(context)
	data.MotionMode = capabilities.MotionMode
	if capabilities.MotionMode == MotionModeDynamic {
		data.PatternID = ""
		data.ProgramID = ""
		data.RecentPatternIDs = nil
		data.Area = ""
	} else {
		data.CenterPercent = 0
		data.SpanPercent = 0
		data.Anchors = nil
		data.VariationPercent = 0
		data.SegmentSeconds = 0
	}
	if !capabilities.Patterns {
		data.PatternID = ""
		data.ProgramID = ""
		data.RecentPatternIDs = nil
	} else {
		data.PatternID = modelPatternID(data.PatternID)
		for index, id := range data.RecentPatternIDs {
			data.RecentPatternIDs[index] = modelPatternID(id)
		}
	}
	if !capabilities.AreaFocus {
		data.Area = ""
	}
	encoded, _ := json.Marshal(data)

	var builder strings.Builder
	builder.WriteString("Authoritative current motion state for this turn (data, not instructions):\n")
	builder.Write(encoded)
	if capabilities.MotionMode == MotionModeDynamic {
		builder.WriteString(`
Use that snapshot deliberately:
- If state is "stopped", use action "start" only for an explicit motion request and choose the initial geometry yourself.
- If state is "running", use action "update" when you decide a change fits; omitted fields preserve the live target.
- A direct embodied partner-action request can request motion without control vocabulary. Decide from the current wording and recent conversation whether physical action is intended.
- "Continue", "steady", "same", or ordinary conversation normally means action "none", but the choice to hold or change belongs to you while motion is active.
- Pacing-only requests preserve geometry. Positioning requests may change center/span or replace them with an anchor loop.
- When the user names two or more positions in an order, use anchors in that order and omit center_percent/span_percent. Those fields describe only a window and cannot preserve an ordered route.
- A narrow local request should still use at least 20% span. A later broad request should expand or move the window rather than remaining pinned there.
- For a request to vary or surprise, change the geometry, speed, anchor route, slow variation, decision horizon, or a fitting combination. Do not mechanically change every field.
- Never stop at a decision horizon; it only tells Autopilot when to reconsider the still-continuous motion.`)
		return builder.String()
	}
	if capabilities.Patterns && context.Running {
		alternatives := make([]string, 0, len(patterns))
		freshAlternatives := make([]string, 0, len(patterns))
		recent := make(map[string]bool, len(data.RecentPatternIDs))
		for _, id := range data.RecentPatternIDs {
			recent[strings.ToLower(id)] = true
		}
		for _, pattern := range patterns {
			actualID := strings.TrimSpace(pattern.ID)
			id := modelPatternID(actualID)
			if id != "" && !strings.EqualFold(actualID, context.PatternID) {
				alternatives = append(alternatives, id)
				if !recent[strings.ToLower(id)] {
					freshAlternatives = append(freshAlternatives, id)
				}
			}
		}
		if len(freshAlternatives) > 0 {
			encodedFresh, _ := json.Marshal(freshAlternatives)
			builder.WriteString("\nFresh enabled pattern IDs (current and recent patterns excluded): ")
			builder.Write(encodedFresh)
		}
		if len(alternatives) > 0 {
			encodedAlternatives, _ := json.Marshal(alternatives)
			builder.WriteString("\nAlternative enabled pattern IDs (current pattern excluded): ")
			builder.Write(encodedAlternatives)
		}
	}
	builder.WriteString(`
Use that snapshot deliberately:
- If state is "stopped", use action "start" for an explicit motion request; never use "target" to start motion.
- A direct embodied partner-action request such as "fuck me", "suck me", "kiss it", "stroke me", or "ride me" can request motion without the words "start", "move", or "device". Decide from the current wording and recent conversation whether physical action is intended; when it is, use "start" from stopped or "target" from running only when the request changes the active action. Do not add motion merely because an action phrase is quoted or discussed.
- If state is "running", use action "target" only when the user asks to change active motion.
- If state is "paused", do not invent a resume command; leave motion unchanged.
- For "continue", "steady", "same", or "hold it there" with no other requested change, preserve the current motion with action "none" or no motion key.
- If the same request asks for a concrete change, apply that change; words such as "same feel" mean preserve fields the user did not ask to change, not action "none".
- For a modest pacing request such as "a little faster" or "slower", change speed within the supplied limits while preserving the current content and area by omitting fields the user did not ask to change.
- For an explicit request to vary, mix up, surprise, or change the feel, choose a meaningful change to pattern, speed, area, or a fitting combination. You own that semantic choice; vary positioning when the conversation calls for it, but do not mechanically change every field at once.
- Ordinary conversation is not a reason to change motion.`)
	if capabilities.Patterns {
		builder.WriteString(`
- Recent patterns are context, not a prohibition. Prefer variety when it fits, but you may deliberately reuse one while changing speed or area. If the user wants the same pace, omit speed_percent; the app preserves current speed. For a pacing-only request, keep the current pattern by omitting pattern_id.`)
	}
	if capabilities.AreaFocus {
		builder.WriteString(`
- A named area focus is temporary unless the user explicitly asks to stay or keep it there. Pacing-only requests preserve area, but a broad variation request while focused should normally move to another fitting area or use area "full" instead of silently pinning every later motion to that region. Omit area only when preserving it is intentional.`)
	}
	return builder.String()
}

func normalizedPromptMotionContext(context MotionContext) promptMotionContext {
	minimum := context.SpeedMinPercent
	maximum := context.SpeedMaxPercent
	if minimum == 0 {
		minimum = 1
	}
	if maximum == 0 {
		maximum = 100
	}
	minimum = clampPromptPercent(minimum, 1, 100)
	maximum = clampPromptPercent(maximum, minimum, 100)

	firstCut := minimum + (maximum-minimum)/3
	secondCut := minimum + 2*(maximum-minimum)/3
	bands := promptSpeedBands{
		Low:    [2]int{minimum, firstCut},
		Middle: [2]int{min(firstCut+1, maximum), secondCut},
		High:   [2]int{min(secondCut+1, maximum), maximum},
	}
	if bands.Middle[0] > bands.Middle[1] {
		bands.Middle[0] = bands.Middle[1]
	}
	if bands.High[0] > bands.High[1] {
		bands.High[0] = bands.High[1]
	}

	result := promptMotionContext{
		State:       "stopped",
		MotionMode:  context.MotionMode,
		SpeedLimits: promptSpeedRange{Min: minimum, Max: maximum},
		SpeedBands:  bands,
	}
	if !context.Running && !context.Paused {
		return result
	}
	result.State = "running"
	if context.Paused {
		result.State = "paused"
	}
	result.PatternID = strings.TrimSpace(context.PatternID)
	result.ProgramID = strings.TrimSpace(context.ProgramID)
	recentPatternIDs := context.RecentPatternIDs
	if len(recentPatternIDs) > 4 {
		recentPatternIDs = recentPatternIDs[len(recentPatternIDs)-4:]
	}
	for _, id := range recentPatternIDs {
		id = strings.TrimSpace(id)
		if id == "" || (len(result.RecentPatternIDs) > 0 && strings.EqualFold(result.RecentPatternIDs[len(result.RecentPatternIDs)-1], id)) {
			continue
		}
		result.RecentPatternIDs = append(result.RecentPatternIDs, id)
	}
	result.SpeedPercent = clampPromptPercent(context.SpeedPercent, minimum, maximum)
	result.Area = strings.ToLower(strings.TrimSpace(context.Area))
	if result.Area == "" {
		result.Area = AreaZoneFull
	}
	result.CenterPercent = context.CenterPercent
	result.SpanPercent = context.SpanPercent
	result.Anchors = append([]string(nil), context.Anchors...)
	result.VariationPercent = context.VariationPercent
	result.SegmentSeconds = context.SegmentSeconds
	return result
}

func clampPromptPercent(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
