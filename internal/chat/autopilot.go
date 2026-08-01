package chat

import (
	"fmt"
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
		fmt.Fprintf(&builder, "Current motion: pattern %q at %d%% speed in area %q.\n", context.CurrentPatternID, context.CurrentSpeed, area)
	}
	if len(context.RecentPatternIDs) > 0 {
		fmt.Fprintf(&builder, "Recently played patterns (oldest first): %s. Treat these as context, not a ban on deliberate reuse.\n", strings.Join(context.RecentPatternIDs, ", "))
	}
	writeSessionProgress(&builder, context)
	builder.WriteString("Decide what happens for the next stretch using the recent conversation as the user's ongoing direction:\n")
	builder.WriteString("- To change motion, use action \"target\" and include only the pattern_id, intensity, speed_percent, or area fields that should change; omitted fields preserve the live target.\n")
	builder.WriteString("- A broad request to vary or change things up may change pattern, speed, area, or a fitting combination. Do not reduce every variation request to pattern cycling.\n")
	if context.AreaFocusEnabled && area != AreaZoneFull {
		builder.WriteString("- The current named area focus is temporary. Unless the user explicitly asked to stay there, broad variation should normally move the focus or set area to \"full\".\n")
	}
	builder.WriteString("- To deliberately keep the current motion going, set motion to {\"action\":\"none\"} or omit motion.\n")
	builder.WriteString("- Never use action \"start\" or \"stop\": only the scheduler starts and only the user stops motion.\n")
	builder.WriteString("- Set next to soon, normal, or later for when motion should next be reconsidered. Do not provide seconds.\n")
	builder.WriteString("- Set variability to settled, normal, or restless for how much the speed should wander before then. This is separate from next: a long stretch can still breathe, and a short one can stay flat.")
	return builder.String()
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
		fmt.Fprintf(&builder, "Current motion: pattern %q at %d%% speed in area %q.\n", context.CurrentPatternID, context.CurrentSpeed, area)
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
