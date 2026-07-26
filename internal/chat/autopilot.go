package chat

import (
	"fmt"
	"strings"
)

// AutopilotContext is the bounded, deterministic context one Autopilot
// curation turn sees. It deliberately contains no transport or engine detail —
// only what the model needs to curate the next segment (ADR 0006 boundary).
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
}

// AutopilotDecisionMessage renders the user-role message for one Autopilot
// check-in. The system prompt (ComposeSystemWithPatterns) already carries the
// strict JSON contract and the enabled pattern catalog; this message only
// frames the decision. The model may change motion by curating an enabled
// pattern, leave motion unchanged, and say one short line — it may never stop
// motion, because stopping belongs to the user.
func AutopilotDecisionMessage(context AutopilotContext) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Autopilot check-in %d. You are steering the device autonomously between chat turns.\n", context.SegmentIndex+1)
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
	if say := strings.TrimSpace(context.LastSay); say != "" {
		fmt.Fprintf(&builder, "The last line you spoke was: %q. Do not repeat it.\n", say)
	}
	builder.WriteString("Decide what happens for the next stretch using the recent conversation as the user's ongoing direction:\n")
	builder.WriteString("- To change motion, use action \"target\" and include only the pattern_id, intensity, or area fields that should change; omitted fields preserve the live target.\n")
	builder.WriteString("- A broad request to vary or change things up may change pattern, speed, area, or a fitting combination. Do not reduce every variation request to pattern cycling.\n")
	if context.AreaFocusEnabled && area != AreaZoneFull {
		builder.WriteString("- The current named area focus is temporary. Unless the user explicitly asked to stay there, broad variation should normally move the focus or set area to \"full\".\n")
	}
	builder.WriteString("- To deliberately keep the current motion going, set motion to {\"action\":\"none\"} or omit motion.\n")
	builder.WriteString("- Never use action \"stop\": only the user stops motion.\n")
	builder.WriteString("Set reply to one short in-character line to speak aloud right now (under 150 characters, no questions that demand an answer).")
	return builder.String()
}
