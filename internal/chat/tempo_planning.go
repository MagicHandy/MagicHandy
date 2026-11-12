package chat

import (
	"fmt"
	"strings"
)

// Native planning must not inherit Layered's position edits, depth bands or
// startup rules just because Layered is the saved motion method. The same
// scheduler and session clock still own when this plan is applied.
func tempoAutopilotMessage(context AutopilotContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Autopilot tempo decision %d. Choose the next stretch for the VacuGlide using its native tempo contract.\n", context.SegmentIndex+1)
	if context.Technical {
		b.WriteString("This turn receives device and session facts only; the person's conversation is not shared.\n")
	} else if context.LastHumanSecondsAgo != nil {
		fmt.Fprintf(&b, "The person last spoke %s ago. This is a planning turn, not a spoken reply.\n", formatSessionSpan(*context.LastHumanSecondsAgo))
	}
	writeSessionProgress(&b, context)
	b.WriteString("Use the current tempo and level in TEMPO CONTEXT. If no tempo is playing, choose one to begin; null only keeps an existing plan. Let a routine, ramp or sequence unfold instead of restarting it at each check.\n")
	if context.Hosted || context.Intermediate {
		b.WriteString("Resolve the scope of recent directions from their meaning and recorded age. A lasting preference remains until the person changes it; a timed request uses the interval from when it was made. Praise or unrelated conversation does not restart its timer. Shape the remaining plan to arrive at the requested time, then follow whatever continuation was requested. A reconsideration is not a new start time. If no continuation was specified, do not invent a stop or a permanent maximum.\n")
	}
	if context.AutoTiming {
		writeAutoTiming(&b, context, true)
	} else {
		b.WriteString("Set next to soon, normal or later for when to reconsider. A hold lets the existing plan keep unfolding.\n")
	}
	return b.String()
}
