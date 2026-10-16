package chat

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// nextCheckField is a hosted planner's own reconsideration horizon in whole
// seconds. The backend still owns the deadline: it varies and bounds the value
// before scheduling it, and chat, pause and Stop keep their usual authority.
const nextCheckField = "next_check_seconds"

func nextCheckSchema() map[string]any {
	return map[string]any{"type": "integer",
		"minimum": config.AutopilotAutoTimingMinSeconds, "maximum": config.AutopilotAutoTimingMaxSeconds}
}

// withNextCheckSchema requires next_check_seconds at the root of an object
// response schema. Only auto-timing turns call it, so local schemas keep their
// exact bytes and field order.
func withNextCheckSchema(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 {
		return schema
	}
	var root map[string]any
	if json.Unmarshal(schema, &root) != nil {
		return schema
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return schema
	}
	properties[nextCheckField] = nextCheckSchema()
	required, _ := root["required"].([]any)
	if !slices.Contains(required, any(nextCheckField)) {
		root["required"] = append(required, nextCheckField)
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return schema
	}
	return encoded
}

// takeNextCheck removes next_check_seconds from a reply so the strict mode
// parsers read their ordinary contract. A missing or unusable value yields 0,
// and the scheduler then samples its usual window.
func takeNextCheck(raw string) (string, int) {
	var root map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &root) != nil {
		return raw, 0
	}
	value, ok := root[nextCheckField]
	if !ok {
		return raw, 0
	}
	delete(root, nextCheckField)
	encoded, err := json.Marshal(root)
	if err != nil {
		return raw, 0
	}
	var seconds float64
	if json.Unmarshal(value, &seconds) != nil || math.IsNaN(seconds) || seconds < 1 {
		return string(encoded), 0
	}
	return string(encoded), int(math.Round(math.Min(seconds, 24*60*60)))
}

// writeAutoTiming tells a hosted planner that the horizon is its own choice,
// and shows its recent choices as observations, the way recent speeds are
// shown. The person's Motion change rate window is given as the reference for
// an ordinary stretch, not a bound: in sessions without a request, GPT-6 Sol
// chose 48-94 seconds at level 4 without it, a long time to feel one pattern.
// Continuous chat moves the next decision forward when the person speaks, so
// only those modes promise it.
func writeAutoTiming(builder *strings.Builder, context AutopilotContext, continuous bool) {
	level := normalizedMotionChangeLevel(context.MotionChangeLevel)
	fmt.Fprintf(builder, `Timing is yours: also return "next_check_seconds", a whole number from %d to %d, for how long the motion you choose, or keep, plays before you decide again. Until then the person feels this one motion: its texture may breathe, but its character stays the same, so a long stretch is a long time with one pattern.`,
		config.AutopilotAutoTimingMinSeconds, config.AutopilotAutoTimingMaxSeconds)
	if minimum, maximum := context.MotionMinSeconds, context.MotionMaxSeconds; minimum > 0 && maximum >= minimum {
		fmt.Fprintf(builder, " At their change appetite of %d/8 (%s), an ordinary stretch lasts about %d-%d seconds. Treat that as the reference and leave it when the moment gives a reason:",
			level, motionChangeBias(level), minimum, maximum)
	} else {
		fmt.Fprintf(builder, " Their change appetite is %d/8 (%s). Fit the length to the moment:", level, motionChangeBias(level))
	}
	builder.WriteString(" a quick accent or contrast can be briefer, a build can take longer to climb, and a stretch the person says they enjoy, or asked to keep, can run for minutes.")
	if continuous {
		builder.WriteString(" When the person speaks, chat answers them and you decide again soon after, so a long stretch never leaves them unheard.")
	}
	builder.WriteString("\n")
	if len(context.RecentCheckSeconds) == 0 {
		return
	}
	values := make([]string, len(context.RecentCheckSeconds))
	for i, seconds := range context.RecentCheckSeconds {
		values[i] = fmt.Sprintf("%d", seconds)
	}
	fmt.Fprintf(builder, "Your recent next_check_seconds, oldest to newest: %s. These are observations, not targets: stretches that all run one length feel mechanical.\n",
		strings.Join(values, ", "))
}
