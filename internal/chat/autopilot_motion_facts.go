package chat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Speech needs the same semantic motion facts even when its authority is chat
// only. A Flow ID is not a catalog pattern, and Dynamic has no catalog ID.
func writeAutopilotMotionFacts(builder *strings.Builder, context AutopilotContext, planning bool) {
	builder.WriteString("Motion domain: one linear stroking slider, 0% at the base and 100% at the tip. These are commanded estimates, not physical feedback.\n")
	if context.CurrentSpeed <= 0 {
		builder.WriteString("No motion target is active yet.\n")
		return
	}
	switch context.MotionMode {
	case MotionModeLayered, MotionModeCreativeV2:
		if context.CurrentFlow == nil {
			builder.WriteString("The next continuous score has not been chosen yet.\n")
			return
		}
		// A mode switch changes the next decision's grammar before it replaces
		// the active segment. Describe that segment by its actual score type.
		activeMode := MotionModeLayered
		var score any = layeredScoreContext(*context.CurrentFlow)
		if context.CurrentFlow.Gesture != nil {
			activeMode = MotionModeCreativeV2
			score = creativeV2ScoreContext(*context.CurrentFlow)
		}
		encoded, _ := json.Marshal(score)
		fmt.Fprintf(builder, "Current continuous score (%s): %s\n", activeMode, encoded)
		if activeMode != context.MotionMode {
			fmt.Fprintf(builder, "The active score still belongs to %s; the selected control mode is %s. Use the selected mode's current_score from the authoritative state for edits. These facts describe the preceding motion until a new update is accepted.\n", activeMode, context.MotionMode)
		}
		if activeMode == MotionModeCreativeV2 {
			builder.WriteString("Focus position locates local work; width is its travel distance; mix blends local work with broad strokes. Rebounds apply only during local work. Sweep contrast changes relative direction timing. This mode has no separately editable pace, center or range layers.\n")
		} else {
			builder.WriteString("The anchor is a reference for stroke placement; a center layer moves that working region. Pace layers vary travel rate, and range layers vary stroke length.\n")
		}
		builder.WriteString("An unchanged score can still vary internally. These controls describe the active phrase; no instantaneous layer phase or live slider position is supplied.\n")
	case MotionModeDynamic:
		fmt.Fprintf(builder, "Current Creative motion: %d%% speed, center %d%%, widest span %d%%, shortest span %d%%, span character %s, variation %d%%, %d sections.\n", context.CurrentSpeed, context.CurrentCenter, context.CurrentSpan, context.CurrentSpanMin, context.CurrentSpanProfile, context.CurrentVariation, context.CurrentSectionCount)
	default:
		fmt.Fprintf(builder, "Current catalog motion: %d%% speed, area %s.\n", context.CurrentSpeed, context.CurrentArea)
	}
	if planning {
		writeCompiledCreativeFeel(builder, context)
	}
}

// writeRecentSpeeds states what pace recent stretches used, and how long the
// pace has stayed near the saved minimum. Motion-only turns are never replayed
// as dialogue, so without this a slow stretch never looked old enough to end.
func writeRecentSpeeds(builder *strings.Builder, context AutopilotContext) {
	steps := context.RecentSpeeds
	if len(steps) < 2 {
		return
	}
	speeds := make([]string, len(steps))
	for i, step := range steps {
		speeds[i] = fmt.Sprintf("%d%%", step.SpeedPercent)
	}
	fmt.Fprintf(builder, "Your recent stretch speeds, oldest to newest, over the last %s: %s.\n",
		formatSessionSpan(steps[0].SecondsAgo), strings.Join(speeds, ", "))
	lowerThird := context.SpeedMinPercent + (context.SpeedMaxPercent-context.SpeedMinPercent)/3
	low := -1
	for i := len(steps) - 1; i >= 0 && steps[i].SpeedPercent <= lowerThird; i-- {
		low = steps[i].SecondsAgo
	}
	if low > 0 {
		fmt.Fprintf(builder, "Pace has stayed in the lower third of the saved range for %s.\n", formatSessionSpan(low))
	}
}

func writeContinuousAutopilotContext(builder *strings.Builder, context AutopilotContext) {
	writeAutopilotMotionFacts(builder, context, true)
	writeSessionProgress(builder, context)
	writeRecentCreativeBands(builder, context.RecentPositionBands)
	level := normalizedMotionChangeLevel(context.MotionChangeLevel)
	fmt.Fprintf(builder, "Motion change preference: %d/8 (%s). This is a preference for session development, not a mandatory change schedule.\n", level, motionChangeBias(level))
	writeRecentSpeeds(builder, context)
	// Every axis collapses to one value unless the spread is asked for; the
	// continuous modes never received the pace line the catalog modes use.
	fmt.Fprintf(builder, "Pace: use the width of the saved %d-%d%% speed range across the session rather than settling into one comfortable band. Easing down is what makes the next climb land; several stretches in a row at nearly the same speed_percent read as flat. When they say it is too much, or say they are close and you choose to hold them off, set speed_percent within a few points of %d%% at once, not partway, and keep it there while that was less than about a minute ago. Once the pace has stayed in the lower third for a minute or two and they have said nothing new since, rebuild: raise speed_percent by about 5 points each stretch and keep rising over the following stretches until it suits the moment again. A wish for a slow pace, such as going slow to make it last, is lasting: keep pace in the lower third and vary it there until they say otherwise.\n",
		context.SpeedMinPercent, context.SpeedMaxPercent, context.SpeedMinPercent)
	if context.MotionFeedback != "" {
		fmt.Fprintf(builder, "Quality feedback: %s\n", context.MotionFeedback)
	}
}
