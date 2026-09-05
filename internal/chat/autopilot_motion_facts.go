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
		var score any = layeredScoreContext(*context.CurrentFlow)
		if context.MotionMode == MotionModeCreativeV2 {
			score = creativeV2ScoreContext(*context.CurrentFlow)
		}
		encoded, _ := json.Marshal(score)
		fmt.Fprintf(builder, "Current continuous score (%s): %s\n", context.MotionMode, encoded)
		if context.MotionMode == MotionModeCreativeV2 {
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

func writeContinuousAutopilotContext(builder *strings.Builder, context AutopilotContext) {
	writeAutopilotMotionFacts(builder, context, true)
	writeSessionProgress(builder, context)
	writeRecentCreativeBands(builder, context.RecentPositionBands)
	level := normalizedMotionChangeLevel(context.MotionChangeLevel)
	fmt.Fprintf(builder, "Motion change preference: %d/8 (%s). This is a preference for session development, not a mandatory change schedule.\n", level, motionChangeBias(level))
	if context.MotionFeedback != "" {
		fmt.Fprintf(builder, "Quality feedback: %s\n", context.MotionFeedback)
	}
}
