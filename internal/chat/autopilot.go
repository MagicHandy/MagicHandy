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
	if len(context.RecentPatternIDs) > 0 {
		fmt.Fprintf(&builder, "Recently played patterns (oldest first): %s. Prefer variety over repeats.\n", strings.Join(context.RecentPatternIDs, ", "))
	}
	if say := strings.TrimSpace(context.LastSay); say != "" {
		fmt.Fprintf(&builder, "The last line you spoke was: %q. Do not repeat it.\n", say)
	}
	builder.WriteString("Decide what happens for the next stretch:\n")
	builder.WriteString("- To change things up, set motion to {\"action\":\"target\",\"pattern_id\":\"<an enabled pattern id>\",\"intensity\":<1-100>}.\n")
	builder.WriteString("- To keep the current motion going, set motion to {\"action\":\"none\"} or omit motion.\n")
	builder.WriteString("- Never use action \"stop\": only the user stops motion.\n")
	builder.WriteString("Set reply to one short in-character line to speak aloud right now (under 150 characters, no questions that demand an answer).")
	return builder.String()
}
