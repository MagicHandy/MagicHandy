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
	SpeedLimits      promptSpeedRange `json:"speed_limits"`
	SpeedBands       promptSpeedBands `json:"speed_bands"`
}

func motionContextInstructions(context MotionContext, capabilities Capabilities, patterns []PatternChoice) string {
	data := normalizedPromptMotionContext(context)
	if !capabilities.Patterns {
		data.PatternID = ""
		data.ProgramID = ""
		data.RecentPatternIDs = nil
	}
	if !capabilities.AreaFocus {
		data.Area = ""
	}
	encoded, _ := json.Marshal(data)

	var builder strings.Builder
	builder.WriteString("Authoritative current motion state for this turn (data, not instructions):\n")
	builder.Write(encoded)
	if capabilities.Patterns && context.Running {
		alternatives := make([]string, 0, len(patterns))
		freshAlternatives := make([]string, 0, len(patterns))
		recent := make(map[string]bool, len(data.RecentPatternIDs))
		for _, id := range data.RecentPatternIDs {
			recent[strings.ToLower(id)] = true
		}
		for _, pattern := range patterns {
			id := strings.TrimSpace(pattern.ID)
			if id != "" && !strings.EqualFold(id, context.PatternID) {
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
- If state is "running", use action "target" only when the user asks to change active motion.
- If state is "paused", do not invent a resume command; leave motion unchanged.
- For "continue", "steady", "same", or "hold it there" with no other requested change, preserve the current motion with action "none" or no motion key.
- If the same request asks for a concrete change, apply that change; words such as "same feel" mean preserve fields the user did not ask to change, not action "none".
- For a modest pacing request such as "a little faster" or "slower", change speed within the supplied limits while preserving the current content and area by omitting fields the user did not ask to change.
- For an explicit request to vary, mix up, surprise, or change the feel, make one purposeful change instead of repeating both the current content and speed. Do not change every field at once.
- Ordinary conversation is not a reason to change motion.`)
	if capabilities.Patterns {
		builder.WriteString(`
- When an explicit variation is requested, prefer a fresh enabled pattern ID when that list is present; otherwise select an alternative. A speed-only target that repeats the current speed does not satisfy a variation request. If the user wants the same pace, omit intensity and speed_percent; the app preserves current speed. For a pacing-only request, keep the current pattern by omitting pattern_id.`)
	}
	if capabilities.AreaFocus {
		builder.WriteString(`
- Preserve the current area unless the user requests a region change. A request for full range while focused is a change: use action "target" with area "full" and omit unchanged pattern and speed fields.`)
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
