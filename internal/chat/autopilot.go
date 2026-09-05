package chat

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// AutopilotContext is bounded semantic context for an autonomous model turn.
// It contains no transport or engine details.
type AutopilotContext struct {
	Style                string
	SegmentIndex         int
	RecentPatternIDs     []string
	SpeedMinPercent      int
	SpeedMaxPercent      int
	LastSay              string
	CurrentPatternID     string
	CurrentSpeed         int
	CurrentArea          string
	AreaFocusEnabled     bool
	MotionMode           MotionMode
	CurrentCenter        int
	CurrentSpan          int
	CurrentSpanMin       int
	CurrentSpanProfile   string
	CurrentAnchors       []string
	CurrentVariation     int
	CurrentSegment       int
	CurrentSectionCount  int
	CurrentFlow          *motion.FlowSpec
	CommandedMeanTravel  int
	CommandedPeakSpeed   int
	CommandedPositionMin int
	CommandedPositionMax int
	MeanStrokeLength     int
	LocalStrokeCV        int
	LocalStrokeRange     int
	RecentPositionBands  []PositionBand
	MotionFeedback       string
	MotionMinSeconds     int
	MotionMaxSeconds     int
	MotionChangeLevel    int
	// SessionTracking gates the session facts below. When false they are
	// omitted from the prompt entirely rather than rendered as zeros, so the model
	// cannot reason from a number that means "unknown".
	SessionTracking          bool
	SessionSeconds           int
	SecondsAtCurrentSpeed    int
	SecondsAtCurrentPhrase   int
	DecisionsAtCurrentPhrase int
	ConsecutiveHolds         int
	SpeedTrend               string
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
		if context.SecondsAtCurrentPhrase > 0 {
			fmt.Fprintf(builder, " The current motion phrase has remained materially unchanged for %s",
				formatSessionSpan(context.SecondsAtCurrentPhrase))
			if context.DecisionsAtCurrentPhrase > 0 {
				fmt.Fprintf(builder, " across %d reconsiderations", context.DecisionsAtCurrentPhrase)
			}
			builder.WriteString(".")
		}
		if context.ConsecutiveHolds > 0 {
			if context.ConsecutiveHolds == 1 {
				builder.WriteString(" The previous motion outcome was a hold.")
			} else {
				fmt.Fprintf(builder, " The last %d motion outcomes were holds.", context.ConsecutiveHolds)
			}
		}
		builder.WriteString("\n")
	}
	if context.ArcEnabled {
		fmt.Fprintf(builder,
			"Session buildup: %d%% of the configured active-session duration, and the user can see this progress. Let it inform pacing within the allowed speed range. The clock and allowed range never move because of your response.\n",
			context.ArcPercent)
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
	if context.MotionMode == MotionModeLayered || context.MotionMode == MotionModeCreativeV2 {
		writeContinuousAutopilotContext(&builder, context)
		return builder.String()
	}
	if context.MotionMode == MotionModeDynamic {
		return dynamicAutopilotMotionMessage(context, &builder)
	}
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

func normalizedMotionChangeLevel(level int) int {
	if level < 1 || level > 8 {
		return 4
	}
	return level
}

func motionChangeBias(level int) string {
	switch {
	case level <= 2:
		return "strong-continuity bias"
	case level <= 5:
		return "balanced-continuity bias"
	default:
		return "frequent-contrast bias"
	}
}

// PositionBand is one previously compiled outer position envelope. Hidden
// autonomous decisions are not part of chat history, so this compact bounded
// memory lets the model see whether its recent range choices really differed.
type PositionBand struct {
	Minimum int
	Maximum int
}

func writeRecentCreativeBands(builder *strings.Builder, bands []PositionBand) {
	if len(bands) == 0 {
		return
	}
	builder.WriteString("Recent compiled position bands, oldest to newest: ")
	for index, band := range bands {
		if index > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(builder, "%d-%d%%", band.Minimum, band.Maximum)
	}
	builder.WriteString(". These are observations of actual engine curves, not required targets; use them to recognize when one range character has dominated.\n")
}

func writeCompiledCreativeFeel(builder *strings.Builder, context AutopilotContext) {
	if context.CommandedPeakSpeed <= 0 {
		return
	}
	fmt.Fprintf(builder,
		"Compiled feel: position band %d-%d%%, about %d%% travel per second on average, %d%%/s peak carriage velocity, %d%% mean stroke length, and the least varied 12-second window has %d%% length range with %d%% coefficient of variation. These are measured from the engine curve, not inferred from the JSON fields. A small numeric edit that leaves this feel similar is still continuity, not a fresh phrase.\n",
		context.CommandedPositionMin, context.CommandedPositionMax,
		context.CommandedMeanTravel, context.CommandedPeakSpeed,
		context.MeanStrokeLength, context.LocalStrokeRange, context.LocalStrokeCV)
}

func writeDynamicSpanGuidance(builder *strings.Builder, context AutopilotContext) {
	builder.WriteString("- To vary stroke length inside the stretch, provide span_min_percent (at least 20 and strictly below the widest span) and choose span_profile breathe, wander, or contrast. Widen span_percent in the same update when the current span leaves no room. Use steady to clear range variation.\n")
	if context.CurrentSpeed <= 0 || context.CurrentSectionCount > 0 {
		return
	}
	if context.CurrentSpan <= 20 {
		fmt.Fprintf(builder, "- The current widest span is %d%%, so it has no legal inner range. To select a variable span profile, include a larger span_percent and a floor from 20 to one less than that new span.\n", context.CurrentSpan)
		return
	}
	fmt.Fprintf(builder, "- If preserving the current %d%% widest span, its usable span_min_percent range is 20-%d. A wider span may set a different ceiling.\n", context.CurrentSpan, context.CurrentSpan-1)
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

const autopilotSpeechTimingInstruction = "Set next to soon, normal, or later for when another spoken check-in would feel natural. Do not provide seconds."

// AutopilotSpeechMessage renders the independent autonomous speech turn.
func AutopilotSpeechMessage(context AutopilotContext) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Autopilot spoken check-in after motion decision %d.\n", context.SegmentIndex)
	if say := strings.TrimSpace(context.LastSay); say != "" {
		fmt.Fprintf(&builder, "The last autonomous line was: %q. Continue the same conversation naturally, without repeating this line or restarting the session.\n", say)
	}
	writeAutopilotSpeechFacts(&builder, context)
	if context.SessionTracking && context.SessionSeconds > 0 {
		fmt.Fprintf(&builder, "This Autopilot session has been running for %s.\n", formatSessionSpan(context.SessionSeconds))
	}
	builder.WriteString("Write one short line in the selected voice that fits the ongoing conversation (under 150 characters and no question that demands an answer). Let the previous exchange lead into this one; a new setting or event is not needed for every message. Do not invent a scene when no scene has been established. Ground any motion description in one specific current fact, such as working region, changing stroke length, directional timing or pace variation, instead of repeatedly asserting smoothness or steadiness. Describe commanded motion without claiming sensed feedback or force.\n")
	builder.WriteString(autopilotSpeechTimingInstruction)
	return builder.String()
}

// AutopilotDecisionMessage remains as a compatibility alias for callers that
// only need the motion prompt.
func AutopilotDecisionMessage(context AutopilotContext) string {
	return AutopilotMotionMessage(context)
}
