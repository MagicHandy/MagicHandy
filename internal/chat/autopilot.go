package chat

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// AutopilotContext is bounded semantic context for an autonomous model turn.
// It contains no transport or engine details.
type AutopilotContext struct {
	Style            string
	SegmentIndex     int
	RecentPatternIDs []string
	SpeedMinPercent  int
	SpeedMaxPercent  int
	LastSay          string
	CurrentPatternID string
	CurrentSpeed     int
	CurrentArea      string
	AreaFocusEnabled bool
	// SessionTracking gates the three session facts below. When false they are
	// omitted from the prompt entirely rather than rendered as zeros, so the model
	// cannot reason from a number that means "unknown".
	SessionTracking       bool
	SessionSeconds        int
	SecondsAtCurrentSpeed int
	SpeedTrend            string
	// ArcEnabled gates the visible session buildup. Disabled means the buildup is
	// never described, so the model cannot act on progress the user was not shown.
	ArcEnabled bool
	ArcPercent int
}

// writeSessionProgress renders the backend-owned session facts. They are stated
// as observations, never as instructions: the model decides what to do with
// them, and none of them widen a limit.
func writeSessionProgress(builder *strings.Builder, context AutopilotContext) {
	if !context.SessionTracking {
		return
	}
	if context.SessionSeconds > 0 {
		fmt.Fprintf(builder, "Session so far: %s.", formatSessionSpan(context.SessionSeconds))
		if context.SecondsAtCurrentSpeed > 0 {
			fmt.Fprintf(builder, " This speed has held for %s.", formatSessionSpan(context.SecondsAtCurrentSpeed))
		}
		if trend := strings.TrimSpace(context.SpeedTrend); trend != "" && trend != "steady" {
			fmt.Fprintf(builder, " Speed has been %s.", trend)
		}
		builder.WriteString("\n")
	}
	if context.ArcEnabled {
		fmt.Fprintf(builder,
			"Session buildup: %d%% of the way along, and the user can see this progress. Aim higher within the allowed speed range as it fills and ease back as it empties. The allowed range itself never moves.\n",
			context.ArcPercent)
		builder.WriteString("Set arc to \"advance\" to move the buildup forward, \"ease\" to move it back, or \"hold\" to leave it. The app bounds each step.\n")
	}
}

func formatSessionSpan(seconds int) string {
	if seconds < 90 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	return fmt.Sprintf("%d minutes", (seconds+30)/60)
}

// AutopilotMotionMessage renders one motion-planning turn. Its strict model
// contract has no reply field; the independent speech clock owns spoken lines.
func AutopilotMotionMessage(context AutopilotContext) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Autopilot motion decision %d. You are steering the device autonomously between chat turns.\n", context.SegmentIndex+1)
	fmt.Fprintf(&builder, "Motion style preference: %s. Allowed speed range: %d-%d%%.\n", context.Style, context.SpeedMinPercent, context.SpeedMaxPercent)
	area := strings.TrimSpace(context.CurrentArea)
	if area == "" {
		area = AreaZoneFull
	}
	if context.CurrentPatternID != "" && context.CurrentSpeed > 0 {
		if containsPatternID(context.RecentPatternIDs, context.CurrentPatternID) {
			fmt.Fprintf(&builder, "Current motion: a recently played catalog pattern at %d%% speed in area %q.\n", context.CurrentSpeed, area)
		} else {
			fmt.Fprintf(&builder, "Current motion: an enabled catalog pattern at %d%% speed in area %q.\n", context.CurrentSpeed, area)
		}
	}
	if len(context.RecentPatternIDs) > 0 {
		builder.WriteString("The current catalog may omit recently played patterns. Use only IDs in that catalog. To continue the current pattern, use action \"none\", or omit pattern_id and use speed_percent to change only its pace.\n")
	}
	writeSessionProgress(&builder, context)
	builder.WriteString("Decide what happens for the next stretch using the recent conversation as the user's ongoing direction:\n")
	builder.WriteString("- To change motion, use action \"target\" and change only what should change; omitted fields preserve the live target.\n")
	builder.WriteString("- Pattern selects motion shape and speed_percent independently selects pace. Include both when changing both; omit either field to preserve it.\n")
	builder.WriteString("- A broad request to vary or change things up may change pattern, speed, area, or a fitting combination. Do not reduce every variation request to pattern cycling.\n")
	if context.AreaFocusEnabled {
		alternatives := autopilotAreaAlternatives(area)
		fmt.Fprintf(&builder,
			"- Area changes available now: %s. Suggested spatial contrast for this stretch: %q. Use it only when it fits the conversation; omit area to deliberately keep %q. A named focus is temporary unless the conversation asks to stay there.\n",
			strings.Join(alternatives, ", "), autopilotAreaSuggestion(context, alternatives), area)
	}
	builder.WriteString("- To deliberately keep the current motion going, set motion to {\"action\":\"none\"} or omit motion.\n")
	builder.WriteString("- Never use action \"start\" or \"stop\": only the scheduler starts and only the user stops motion.\n")
	// Every axis here collapses to one value unless the spread is asked for. A
	// live session held one pattern, chose "soon" every time, and kept speed
	// inside a narrow band for ten minutes.
	fmt.Fprintf(&builder, "- Use the width of %d-%d%% across the session rather than settling into one comfortable band. Easing down is what makes the next climb land, and several decisions in a row at nearly the same speed is the main thing to avoid.\n",
		context.SpeedMinPercent, context.SpeedMaxPercent)
	builder.WriteString("- Set next to soon, normal, or later for when motion should next be reconsidered. Do not provide seconds. Vary it: back-to-back short stretches read as flat as one long one.\n")
	builder.WriteString("- Set variability to settled, normal, or restless for how much the speed should wander before then. This is separate from next: a long stretch can still breathe, and a short one can stay flat.")
	return builder.String()
}

func containsPatternID(ids []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, id := range ids {
		if strings.EqualFold(strings.TrimSpace(id), target) {
			return true
		}
	}
	return false
}

func autopilotAreaAlternatives(current string) []string {
	current = strings.ToLower(strings.TrimSpace(current))
	alternatives := make([]string, 0, len(AreaZones())-1)
	for _, area := range AreaZones() {
		if area != current {
			alternatives = append(alternatives, area)
		}
	}
	return alternatives
}

func autopilotAreaSuggestion(context AutopilotContext, alternatives []string) string {
	if len(alternatives) == 0 {
		return ""
	}
	hash := fnv.New32a()
	_, _ = fmt.Fprintf(hash, "%d|%s|%s", context.SegmentIndex, context.CurrentPatternID, context.CurrentArea)
	return alternatives[int(hash.Sum32())%len(alternatives)]
}

// AutopilotSpeechMessage renders the independent autonomous speech turn.
func AutopilotSpeechMessage(context AutopilotContext) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Autopilot spoken check-in after motion decision %d.\n", context.SegmentIndex)
	if say := strings.TrimSpace(context.LastSay); say != "" {
		fmt.Fprintf(&builder, "The last autonomous line was: %q. Do not repeat or paraphrase it.\n", say)
	}
	if context.CurrentPatternID != "" && context.CurrentSpeed > 0 {
		area := strings.TrimSpace(context.CurrentArea)
		if area == "" {
			area = AreaZoneFull
		}
		fmt.Fprintf(&builder, "Current motion: a catalog pattern at %d%% speed in area %q.\n", context.CurrentSpeed, area)
	}
	writeSessionProgress(&builder, context)
	builder.WriteString("Write one short in-character line that fits the recent conversation (under 150 characters and no question that demands an answer).\n")
	builder.WriteString("Set next to soon, normal, or later for when another spoken check-in would feel natural. Do not provide seconds.")
	return builder.String()
}

// AutopilotDecisionMessage remains as a compatibility alias for callers that
// only need the motion prompt.
func AutopilotDecisionMessage(context AutopilotContext) string {
	return AutopilotMotionMessage(context)
}
