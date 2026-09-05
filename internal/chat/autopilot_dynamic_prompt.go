package chat

import (
	"fmt"
	"strings"
)

// Dynamic motion has a different shape grammar from continuous score edits.
func dynamicAutopilotMotionMessage(context AutopilotContext, builder *strings.Builder) string {
	minimumSeconds, maximumSeconds := context.MotionMinSeconds, context.MotionMaxSeconds
	if minimumSeconds < 4 {
		minimumSeconds = 4
	}
	if maximumSeconds < minimumSeconds {
		maximumSeconds = 120
	}
	if context.CurrentSpeed > 0 {
		fmt.Fprintf(builder, "Current creative motion: %d%% speed, center %d%%, widest span %d%%",
			context.CurrentSpeed, context.CurrentCenter, context.CurrentSpan)
		if context.CurrentSpanProfile != "" {
			fmt.Fprintf(builder, ", span floor %d%%, span profile %q",
				context.CurrentSpanMin, context.CurrentSpanProfile)
		}
		fmt.Fprintf(builder, ", center/rhythm variation %d%%", context.CurrentVariation)
		if len(context.CurrentAnchors) > 0 {
			fmt.Fprintf(builder, ", anchors %q", context.CurrentAnchors)
		}
		if context.CurrentSectionCount > 0 {
			fmt.Fprintf(builder, ", %d-section phrase", context.CurrentSectionCount)
		}
		if context.CurrentSegment > 0 {
			fmt.Fprintf(builder, ", %d-second decision horizon", context.CurrentSegment)
		}
		builder.WriteString(".\n")
	} else {
		builder.WriteString("No Dynamic target is active. To begin Creative motion, use update with speed and either center/span or anchors, or use a complete sections phrase; none leaves Autopilot waiting.\n")
	}
	writeCompiledCreativeFeel(builder, context)
	writeRecentCreativeBands(builder, context.RecentPositionBands)
	writeSessionProgress(builder, context)
	builder.WriteString("Choose the next continuous stretch. Autopilot authorizes bounded choices without a new chat message; conversation and explicit user directions still win.\n")
	motionChangeLevel := normalizedMotionChangeLevel(context.MotionChangeLevel)
	fmt.Fprintf(builder, "- Motion change rate: %d/8 (%s). It shapes the session, not a mandatory change schedule: any one hold may fit, while high rates favor materially different phrases instead of nearby scalar edits.\n", motionChangeLevel, motionChangeBias(motionChangeLevel))
	builder.WriteString("- Use none for deliberate continuity. Use update only when the result should feel different; omitted fields preserve the target. Phrase age and compiled feel are the source of truth when earlier JSON edits amounted to continuity.\n")
	builder.WriteString("- Shape one continuous route with center/span or 2-6 named anchors, never both. Position 0% is base and 100% is tip; give both ends equal design weight. Broad motion approaches the full available position range rather than making a slightly wider midpoint band. Across the session, mix localized and broad routes and sometimes approach either end, without requiring endpoints, full range, or alternation on a schedule. Interior anchors are pass-through positions, not pauses.\n")
	writeDynamicSpanGuidance(builder, context)
	builder.WriteString("- variation_percent adds smooth correlated center/rhythm texture: 20-40 is subtle, 45-70 clearly organic, and 70-100 deliberately wild; zero is mechanically even, never jitter.\n")
	builder.WriteString("- Use 2-4 sections only for several distinct or evolving movement ideas; give each complete geometry, texture, and 2-12 cycles. Otherwise keep one coherent geometry.\n")
	fmt.Fprintf(builder, "- segment_seconds must be %d-%d and says when to reconsider, not when to stop. Vary it naturally rather than choosing one constant.\n",
		minimumSeconds, maximumSeconds)
	fmt.Fprintf(builder, "- Explore %d-%d%% speed across the session and rotate contrast among pace, outer travel band, stroke-length envelope, texture, and sections; pace alone does not change range.\n",
		context.SpeedMinPercent, context.SpeedMaxPercent)
	builder.WriteString("- Name one brief destination intent before encoding fields, so the geometry follows a motion idea instead of incrementing toward it. Never use start or stop: the scheduler owns start and only the user stops motion. Set next for fallback timing and variability for speed drift independent of geometry.")
	if feedback := strings.TrimSpace(context.MotionFeedback); feedback != "" {
		fmt.Fprintf(builder, "\nQuality feedback: %s", feedback)
	}
	return builder.String()
}
