package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// FreshCreativeV2Score selects a new replayable realization. The model edits
// the semantic grammar, never the random seed or generated point sequence.
func FreshCreativeV2Score(speed int) motion.FlowSpec {
	spec := motion.DefaultFlowSpec()
	gesture := motion.DefaultGestureSpec()
	spec.SpeedPercent, spec.RangeFloorPercent = speed, 10
	spec.Gesture, spec.Seed = &gesture, freshLayeredSeed(0)
	return spec
}

func creativeV2ScoreContext(spec motion.FlowSpec) map[string]any {
	encoded, _ := json.Marshal(spec.Gesture)
	fields := map[string]any{}
	_ = json.Unmarshal(encoded, &fields)
	fields["min_percent"], fields["max_percent"], fields["speed_percent"] = spec.MinPercent, spec.MaxPercent, spec.SpeedPercent
	for name, keys := range creativeV2Groups {
		group := map[string]any{}
		for from, to := range keys {
			group[from] = fields[to]
			delete(fields, to)
		}
		fields[name] = group
	}
	return fields
}

// ParseCreativeV2Reply merges an explicit partial edit transactionally. The
// complete score is kept behind the same internal continuous-motion handoff as
// Layered; the two model vocabularies remain disjoint.
func ParseCreativeV2Reply(raw string, current motion.FlowSpec, limits config.MotionSettings) (AssistantResponse, motion.FlowSpec, []string, error) {
	if current.Gesture == nil {
		return AssistantResponse{}, current, nil, errors.New("start a new Creative v2 score before using this interface")
	}
	var proposal struct {
		Reply   string                       `json:"reply"`
		NewMood *Mood                        `json:"new_mood,omitempty"`
		Edits   []map[string]json.RawMessage `json:"edits"`
	}
	if err := decodeLabObject(raw, &proposal); err != nil {
		return AssistantResponse{}, current, nil, err
	}
	proposal.Reply = strings.TrimSpace(proposal.Reply)
	if proposal.Edits == nil || len(proposal.Edits) > 8 || strings.TrimSpace(proposal.Reply) == "" || len(proposal.Reply) > 12000 {
		return AssistantResponse{}, current, nil, errors.New("creative v2 requires up to eight edits ([] for no change) and a non-empty bounded reply")
	}
	if proposal.NewMood != nil {
		if _, ok := validMood(*proposal.NewMood); !ok {
			return AssistantResponse{}, current, nil, errors.New("unknown mood")
		}
	}
	next, err := applyCreativeV2Edits(proposal.Edits, current, limits)
	if err != nil {
		return AssistantResponse{}, current, nil, err
	}
	return AssistantResponse{Reply: proposal.Reply, NewMood: proposal.NewMood}, next, labChangedControls(current, next), nil
}

func applyCreativeV2Edits(items []map[string]json.RawMessage, current motion.FlowSpec, limits config.MotionSettings) (motion.FlowSpec, error) {
	next := *motion.CloneFlowSpec(&current)
	encoded, _ := json.Marshal(next.Gesture)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	edits := map[string]json.RawMessage{}
	for _, item := range items {
		if len(item) != 1 {
			return current, errors.New("each Creative v2 edit must name exactly one control group")
		}
		for name, value := range item {
			if _, exists := edits[name]; exists {
				return current, fmt.Errorf("duplicate Creative v2 edit: %s", name)
			}
			edits[name] = value
		}
	}
	for name, value := range edits {
		if string(value) == "null" {
			return current, fmt.Errorf("%s must be omitted instead of null", name)
		}
		switch name {
		case "range", "focus", "sweep", "rebounds":
			if err := applyCreativeV2Group(name, value, &next, fields); err != nil {
				return current, err
			}
		case "evolve":
			var evolve bool
			if err := json.Unmarshal(value, &evolve); err != nil {
				return current, err
			}
			if evolve {
				next.Seed = freshLayeredSeed(current.Seed)
			}
		case "speed_percent":
			var number int
			if err := json.Unmarshal(value, &number); err != nil {
				return current, err
			}
			next.SpeedPercent = number
		default:
			if name != "inertia_percent" && name != "variation_percent" {
				return current, fmt.Errorf("unsupported Creative v2 control: %s", name)
			}
			fields[name] = value
		}
	}
	encoded, _ = json.Marshal(fields)
	if err := decodeLabObject(string(encoded), next.Gesture); err != nil {
		return current, err
	}
	if err := next.Validate(limits); err != nil {
		return current, err
	}
	return next, nil
}

var creativeV2Groups = map[string]map[string]string{
	"range":    {"min_percent": "min_percent", "max_percent": "max_percent"},
	"focus":    {"position_percent": "focus_percent", "width_percent": "focus_width_percent", "mix_percent": "focus_mix_percent"},
	"sweep":    {"faster_direction": "faster_direction", "contrast_percent": "contrast_percent"},
	"rebounds": {"count": "rebound_count", "retained_width_percent": "rebound_decay_percent"},
}

func applyCreativeV2Group(name string, raw json.RawMessage, next *motion.FlowSpec, fields map[string]json.RawMessage) error {
	var group map[string]json.RawMessage
	if err := json.Unmarshal(raw, &group); err != nil {
		return err
	}
	keys := creativeV2Groups[name]
	if len(group) != len(keys) {
		return fmt.Errorf("%s requires all its paired fields", name)
	}
	for from, to := range keys {
		value, ok := group[from]
		if !ok || string(value) == "null" {
			return fmt.Errorf("%s.%s is required", name, from)
		}
		if name == "range" {
			var number int
			if err := json.Unmarshal(value, &number); err != nil {
				return err
			}
			if from == "min_percent" {
				next.MinPercent = number
			} else {
				next.MaxPercent = number
			}
		} else {
			fields[to] = value
		}
	}
	return nil
}

// CreativeV2ResponseSchema is identical in production and the prompt Lab.
func CreativeV2ResponseSchema(limits config.MotionSettings, mood bool) json.RawMessage {
	integer := func(lo, hi int) any { return map[string]any{"type": "integer", "minimum": lo, "maximum": hi} }
	object := func(fields map[string]any, required []string) any {
		return map[string]any{"type": "object", "properties": fields, "required": required, "additionalProperties": false}
	}
	edits := map[string]any{
		"min_percent": integer(0, 90), "max_percent": integer(10, 100), "speed_percent": integer(limits.SpeedMinPercent, limits.SpeedMaxPercent),
		"focus_percent": integer(0, 100), "focus_width_percent": integer(10, 100), "focus_mix_percent": integer(0, 100),
		"faster_direction": map[string]any{"type": "string", "enum": []string{"even", "tip", "base"}},
		"contrast_percent": integer(0, 80), "inertia_percent": integer(0, 100), "rebound_count": integer(0, 4),
		"rebound_decay_percent": integer(25, 85), "variation_percent": integer(0, 100), "evolve": map[string]any{"type": "boolean"},
	}
	for name, keys := range creativeV2Groups {
		group := map[string]any{}
		required := []string{}
		for from, to := range keys {
			group[from] = edits[to]
			required = append(required, from)
			delete(edits, to)
		}
		slices.Sort(required)
		edits[name] = object(group, required)
	}
	choices := []any{}
	names := []string{}
	for name := range edits {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		choices = append(choices, object(map[string]any{name: edits[name]}, []string{name}))
	}
	fields := map[string]any{"edits": map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"oneOf": choices}}, "reply": map[string]any{"type": "string"}}
	if mood {
		fields["new_mood"] = map[string]any{"type": "string", "enum": Moods()}
	}
	encoded, _ := json.Marshal(object(fields, []string{"edits", "reply"}))
	return encoded
}

func creativeV2EditScope(message string, before, after motion.FlowSpec) error {
	if err := creativeV2RequestedCoverage(message, after); err != nil {
		return err
	}
	if before.SpeedPercent == after.SpeedPercent {
		return nil
	}
	message = normalizeMotionIntent(message)
	if negatesDynamicSpeedChange(message) || !hasIntentPhrase(message,
		"speed", "pace", "faster", "slower", "gentle", "gently", "gentler", "fast", "rapid", "quick", "quicker", "slow", "slowly", "intensity", "intense") {
		return errors.New("creative v2 changed pace without a pace request")
	}
	return nil
}

// Check explicit coverage claims without inventing a replacement plan. This
// catches a model emitting only rebounds while claiming it also moved the focus.
func creativeV2RequestedCoverage(message string, after motion.FlowSpec) error {
	message = normalizeMotionIntent(message)
	if motionIntentIsConversation(message) || explicitlyRefusesDynamicMotion(message) || after.Gesture == nil {
		return nil
	}
	g := after.Gesture
	if hasIntentPhrase(message, "rebounds at the base", "base rebounds", "bounce at the lower end") && g.FocusPercent != 0 {
		return errors.New("creative v2 did not move the requested rebound focus to the base")
	}
	if hasIntentPhrase(message, "rebounds at the tip", "tip rebounds", "bounce at the upper end") && g.FocusPercent != 100 {
		return errors.New("creative v2 did not move the requested rebound focus to the tip")
	}
	if hasIntentPhrase(message, "mix", "mixed", "interspersed") && hasIntentPhrase(message, "full strokes", "broad travel", "broad strokes") &&
		(g.FocusMixPercent == 0 || g.FocusMixPercent == 100) {
		return errors.New("creative v2 did not mix local and broad strokes as requested")
	}
	return nil
}

func creativeV2UserMayEdit(message string, running bool, before, after motion.FlowSpec) bool {
	if layeredUserMayEdit(message, running, before, after) {
		return true
	}
	message = normalizeMotionIntent(message)
	if !running || motionIntentIsConversation(message) || explicitlyRefusesDynamicMotion(message) {
		return false
	}
	if after.Gesture != nil && after.Gesture.ReboundCount == 0 {
		for _, phrase := range []string{"no bounces", "no bounce", "no rebounds", "without bounces", "without rebounds"} {
			message = strings.ReplaceAll(message, phrase, "remove rebounds")
		}
	}
	if after.Gesture != nil && after.Gesture.InertiaPercent == 0 {
		for _, phrase := range []string{"no inertia", "without inertia"} {
			message = strings.ReplaceAll(message, phrase, "remove inertia")
		}
	}
	return !motionIntentIsNegated(message) && hasIntentPhrase(message, "bounce", "bounces", "rebound", "rebounds", "sweep", "sweeps", "inertia", "momentum", "upward", "downward", "stroke", "strokes")
}

// CreativeV2CharacterUnchanged is shared by scheduled Lab and production turns.
// Realization seeds are intentionally excluded from this semantic comparison.
func CreativeV2CharacterUnchanged(before, after motion.FlowSpec) bool {
	return before.Gesture != nil && after.Gesture != nil && *before.Gesture == *after.Gesture &&
		before.SpeedPercent == after.SpeedPercent && before.MinPercent == after.MinPercent && before.MaxPercent == after.MaxPercent
}
