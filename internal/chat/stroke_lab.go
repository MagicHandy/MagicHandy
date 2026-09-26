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

// The stroke Lab modes test three model-facing vocabularies over one stroke
// score (motion.StrokeSpec): numeric turn points, a groove with accents, and
// plain words. They share the engine and differ only in what the model writes,
// so live results compare vocabularies rather than generators.
const (
	LabMethodStrokeEnds = "stroke_ends"
	LabMethodGroove     = "groove"
	LabMethodPlainWords = "plain_words"
)

// IsStrokeLabMethod reports whether a Lab method edits a stroke score.
func IsStrokeLabMethod(method string) bool {
	return method == LabMethodStrokeEnds || method == LabMethodGroove || method == LabMethodPlainWords
}

// FreshStrokeLabScore is the stroke modes' starting score with a fresh seed:
// long strokes around the middle that breathe at a medium pace. Every
// vocabulary can state it exactly, so the model reads back clean values.
func FreshStrokeLabScore(limits config.MotionSettings) motion.FlowSpec {
	spec := motion.DefaultFlowSpec()
	spec.MinPercent, spec.MaxPercent, spec.PaceVariationPercent = 0, 100, 0
	spec.Seed = freshLayeredSeed(0)
	words := plainWords{Depth: "lower", PullBack: "upper", Variety: "breathing", Pace: "medium"}
	spec.SpeedPercent = plainSpeed(words.Pace, limits)
	bottom, top, variation := plainTurns(words)
	spec.Strokes = &motion.StrokeSpec{Bottom: bottom, Top: top, VariationPercent: variation}
	return spec
}

// strokeLabContext is the current score in the method's own vocabulary.
func strokeLabContext(method string, spec motion.FlowSpec, limits config.MotionSettings) any {
	if spec.Strokes == nil {
		return nil
	}
	k := *spec.Strokes
	accents := k.Accents
	if accents == nil {
		accents = []motion.StrokeAccent{}
	}
	switch method {
	case LabMethodGroove:
		bottomAt, topAt, feel := grooveOf(k)
		return map[string]any{"groove": map[string]any{"bottom_at_percent": bottomAt, "top_at_percent": topAt, "feel": feel},
			"accents": accents, "speed_percent": spec.SpeedPercent}
	case LabMethodPlainWords:
		return plainWordsOf(spec, limits)
	}
	return map[string]any{"bottom": k.Bottom, "top": k.Top, "accents": accents,
		"speed_percent": spec.SpeedPercent, "variation_percent": k.VariationPercent}
}

// ParseStrokeLab merges one reply's explicit edits into the stroke score. An
// invalid value rejects the whole reply; nothing is repaired or inferred.
func ParseStrokeLab(raw, method string, current motion.FlowSpec, limits config.MotionSettings) (string, motion.FlowSpec, []string, error) {
	if current.Strokes == nil {
		return "", current, nil, errors.New("start a new Lab score for this test mode")
	}
	var reply struct {
		Edits json.RawMessage `json:"edits"`
		Reply string          `json:"reply"`
	}
	if err := decodeLabObject(raw, &reply); err != nil {
		return "", current, nil, err
	}
	if strings.TrimSpace(reply.Reply) == "" || len(reply.Reply) > 2000 {
		return "", current, nil, errors.New("a brief reply is required")
	}
	if len(reply.Edits) == 0 || string(reply.Edits) == "null" {
		return "", current, nil, errors.New(`edits is required; use {} for no change`)
	}
	next := *motion.CloneFlowSpec(&current)
	var err error
	switch method {
	case LabMethodGroove:
		err = applyGrooveEdits(reply.Edits, &next)
	case LabMethodPlainWords:
		err = applyPlainEdits(reply.Edits, &next, limits)
	default:
		err = applyStrokeEndEdits(reply.Edits, &next)
	}
	if err != nil {
		return "", current, nil, err
	}
	if err := next.Validate(limits); err != nil {
		return "", current, nil, err
	}
	return reply.Reply, next, labChangedControls(current, next), nil
}

func applyStrokeEndEdits(raw json.RawMessage, next *motion.FlowSpec) error {
	var edits struct {
		Bottom           json.RawMessage        `json:"bottom"`
		Top              json.RawMessage        `json:"top"`
		Accents          *[]motion.StrokeAccent `json:"accents"`
		SpeedPercent     *int                   `json:"speed_percent"`
		VariationPercent *int                   `json:"variation_percent"`
		Evolve           bool                   `json:"evolve"`
	}
	if err := decodeLabObject(string(raw), &edits); err != nil {
		return err
	}
	for _, turn := range []struct {
		raw  json.RawMessage
		into *motion.StrokeTurn
	}{{edits.Bottom, &next.Strokes.Bottom}, {edits.Top, &next.Strokes.Top}} {
		if len(turn.raw) == 0 {
			continue
		}
		if err := decodeStrokeTurn(turn.raw, turn.into); err != nil {
			return err
		}
	}
	if edits.Accents != nil {
		next.Strokes.Accents = slices.Clone(*edits.Accents)
	}
	if edits.SpeedPercent != nil {
		next.SpeedPercent = *edits.SpeedPercent
	}
	if edits.VariationPercent != nil {
		next.Strokes.VariationPercent = *edits.VariationPercent
	}
	if edits.Evolve {
		next.Seed = freshLayeredSeed(next.Seed)
	}
	return nil
}

// decodeStrokeTurn requires all three fields, so an omitted one cannot
// silently become zero.
func decodeStrokeTurn(raw json.RawMessage, into *motion.StrokeTurn) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errors.New("a turn must be an object")
	}
	for _, key := range []string{"at_percent", "vary_to_percent", "character"} {
		if _, ok := fields[key]; !ok {
			return errors.New("a turn needs at_percent, vary_to_percent and character")
		}
	}
	return decodeLabObject(string(raw), into)
}

func applyGrooveEdits(raw json.RawMessage, next *motion.FlowSpec) error {
	var edits struct {
		Groove *struct {
			BottomAtPercent *int    `json:"bottom_at_percent"`
			TopAtPercent    *int    `json:"top_at_percent"`
			Feel            *string `json:"feel"`
		} `json:"groove"`
		Accents      *[]motion.StrokeAccent `json:"accents"`
		SpeedPercent *int                   `json:"speed_percent"`
		Evolve       bool                   `json:"evolve"`
	}
	if err := decodeLabObject(string(raw), &edits); err != nil {
		return err
	}
	if groove := edits.Groove; groove != nil {
		if groove.BottomAtPercent == nil || groove.TopAtPercent == nil || groove.Feel == nil {
			return errors.New("groove needs bottom_at_percent, top_at_percent and feel")
		}
		if !slices.Contains(grooveFeels, *groove.Feel) {
			return errors.New("groove feel must be steady, breathing or wandering")
		}
		bottomAt, topAt := *groove.BottomAtPercent, *groove.TopAtPercent
		if bottomAt < 0 || topAt > 100 || topAt-bottomAt < 10 {
			return errors.New("the groove turns back at least 10 above where it bottoms out, inside 0-100")
		}
		next.Strokes.Bottom, next.Strokes.Top, next.Strokes.VariationPercent = grooveTurns(bottomAt, topAt, *groove.Feel)
	}
	if edits.Accents != nil {
		next.Strokes.Accents = slices.Clone(*edits.Accents)
	}
	if edits.SpeedPercent != nil {
		next.SpeedPercent = *edits.SpeedPercent
	}
	if edits.Evolve {
		next.Seed = freshLayeredSeed(next.Seed)
	}
	return nil
}

func applyPlainEdits(raw json.RawMessage, next *motion.FlowSpec, limits config.MotionSettings) error {
	var edits struct {
		Depth      *string   `json:"depth"`
		PullBack   *string   `json:"pull_back"`
		Variety    *string   `json:"variety"`
		Accents    *[]string `json:"accents"`
		AccentRate *string   `json:"accent_rate"`
		Pace       *string   `json:"pace"`
		Evolve     bool      `json:"evolve"`
	}
	if err := decodeLabObject(string(raw), &edits); err != nil {
		return err
	}
	words := plainWordsOf(*next, limits)
	for _, field := range []struct {
		value   *string
		into    *string
		allowed []string
		name    string
	}{
		{edits.Depth, &words.Depth, plainDepths, "depth"}, {edits.PullBack, &words.PullBack, plainPullBacks, "pull_back"},
		{edits.Variety, &words.Variety, grooveFeels, "variety"}, {edits.AccentRate, &words.AccentRate, motion.StrokeAccentRates, "accent_rate"},
		{edits.Pace, &words.Pace, plainPaces, "pace"},
	} {
		if field.value == nil {
			continue
		}
		if !slices.Contains(field.allowed, *field.value) {
			return fmt.Errorf("%s must be one of %s", field.name, strings.Join(field.allowed, ", "))
		}
		*field.into = *field.value
	}
	if edits.Accents != nil {
		for _, word := range *edits.Accents {
			if !slices.Contains(plainAccentWords, word) {
				return fmt.Errorf("accents must be from %s", strings.Join(plainAccentWords, ", "))
			}
		}
		words.Accents = *edits.Accents
	}
	if edits.Depth != nil || edits.PullBack != nil || edits.Variety != nil {
		next.Strokes.Bottom, next.Strokes.Top, next.Strokes.VariationPercent = plainTurns(words)
	}
	if edits.Accents != nil || edits.AccentRate != nil {
		next.Strokes.Accents = nil
		for _, word := range words.Accents {
			move := plainAccentMoves[slices.Index(plainAccentWords, word)]
			next.Strokes.Accents = append(next.Strokes.Accents, motion.StrokeAccent{Move: move, Rate: words.AccentRate})
		}
	}
	if edits.Pace != nil {
		next.SpeedPercent = plainSpeed(words.Pace, limits)
	}
	if edits.Evolve {
		next.Seed = freshLayeredSeed(next.Seed)
	}
	return nil
}

// StrokeLabContinuationMessage is the Lab Autopilot policy for a stroke mode.
// It reuses the production continuous judgment: recent requests shape the
// motion, and the model develops what they leave open.
func StrokeLabContinuationMessage(method string) string {
	open := map[string]string{
		LabMethodStrokeEnds: "Stroke ends can develop how deep strokes go, where they turn near the tip, how each turn varies, accents and pace.",
		LabMethodGroove:     "Groove and accents can develop the groove, its feel, the accents woven into it and pace.",
		LabMethodPlainWords: "Plain words can develop depth, pull back, variety, accents and pace.",
	}[method]
	return continuousAutopilotMessage("{}") + " " + open +
		" A previous model choice is not a user constraint: keep what the person asked for and develop the rest."
}

// StrokeLabSchema constrains syntax, names and ranges, not intent. The
// strict parser still runs on schema-guided and plain output alike.
func StrokeLabSchema(method string, limits config.MotionSettings) json.RawMessage {
	integer := func(minimum, maximum int) map[string]any {
		return map[string]any{"type": "integer", "minimum": minimum, "maximum": maximum}
	}
	enum := func(values []string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	object := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	accent := object(map[string]any{"move": enum(motion.StrokeAccentMoves), "rate": enum(motion.StrokeAccentRates)}, []string{"move", "rate"})
	accents := map[string]any{"type": "array", "items": accent, "maxItems": 3}
	speed, evolve := integer(limits.SpeedMinPercent, limits.SpeedMaxPercent), map[string]any{"type": "boolean"}
	var edits map[string]any
	switch method {
	case LabMethodGroove:
		groove := object(map[string]any{"bottom_at_percent": integer(0, 90), "top_at_percent": integer(10, 100), "feel": enum(grooveFeels)},
			[]string{"bottom_at_percent", "top_at_percent", "feel"})
		edits = object(map[string]any{"groove": groove, "accents": accents, "speed_percent": speed, "evolve": evolve}, []string{})
	case LabMethodPlainWords:
		words := map[string]any{"type": "array", "items": enum(plainAccentWords), "maxItems": 3}
		edits = object(map[string]any{"depth": enum(plainDepths), "pull_back": enum(plainPullBacks), "variety": enum(grooveFeels),
			"accents": words, "accent_rate": enum(motion.StrokeAccentRates), "pace": enum(plainPaces), "evolve": evolve}, []string{})
	default:
		turn := object(map[string]any{"at_percent": integer(0, 100), "vary_to_percent": integer(0, 100), "character": enum(motion.StrokeCharacters)},
			[]string{"at_percent", "vary_to_percent", "character"})
		edits = object(map[string]any{"bottom": turn, "top": turn, "accents": accents, "speed_percent": speed,
			"variation_percent": integer(0, 100), "evolve": evolve}, []string{})
	}
	encoded, _ := json.Marshal(object(map[string]any{"edits": edits, "reply": map[string]any{"type": "string"}}, []string{"edits", "reply"}))
	return encoded
}
