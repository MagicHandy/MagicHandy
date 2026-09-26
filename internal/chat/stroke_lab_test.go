package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func strokeLabLimits() config.MotionSettings {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 6, 31
	return limits
}

func TestStrokeLabContractExamplesParse(t *testing.T) {
	limits := strokeLabLimits()
	for method, contract := range map[string]string{LabMethodStrokeEnds: strokeEndsContract,
		LabMethodGroove: grooveContract, LabMethodPlainWords: plainWordsContract} {
		examples := 0
		for _, line := range strings.Split(contract, "\n") {
			if !strings.HasPrefix(line, `{"edits":`) {
				continue
			}
			examples++
			current := FreshStrokeLabScore(limits)
			if _, _, _, err := ParseStrokeLab(line, method, current, limits); err != nil {
				t.Fatalf("%s example does not parse: %s: %v", method, line, err)
			}
		}
		if examples < 5 {
			t.Fatalf("%s has only %d examples", method, examples)
		}
		if !strings.Contains(LLMLabPrompts()[method], depthFrame) {
			t.Fatalf("%s prompt omits the depth frame", method)
		}
	}
}

func TestStrokeLabPartialEditsPreserveTheRest(t *testing.T) {
	limits := strokeLabLimits()
	current := FreshStrokeLabScore(limits)
	for method, raw := range map[string]string{
		LabMethodStrokeEnds: `{"edits":{"speed_percent":20},"reply":"Slower."}`,
		LabMethodGroove:     `{"edits":{"speed_percent":20},"reply":"Slower."}`,
		LabMethodPlainWords: `{"edits":{"pace":"slow"},"reply":"Slower."}`,
	} {
		_, next, changed, err := ParseStrokeLab(raw, method, current, limits)
		if err != nil {
			t.Fatal(method, err)
		}
		if !reflect.DeepEqual(next.Strokes, current.Strokes) || !reflect.DeepEqual(changed, []string{"speed_percent"}) {
			t.Fatalf("%s pace edit changed %v", method, changed)
		}
	}
	_, next, _, err := ParseStrokeLab(`{"edits":{"depth":"middle"},"reply":"Not as deep."}`, LabMethodPlainWords, current, limits)
	if err != nil {
		t.Fatal(err)
	}
	if words := plainWordsOf(next, limits); words.Depth != "middle" || words.PullBack != "upper" || words.Variety != "breathing" {
		t.Fatalf("a depth edit changed other words: %+v", words)
	}
	// A depth word takes effect whatever the pull back: at the tip, the
	// stroke turns back just above it.
	_, next, _, err = ParseStrokeLab(`{"edits":{"depth":"tip"},"reply":"Only the tip."}`, LabMethodPlainWords, current, limits)
	if err != nil || next.Strokes.Bottom.AtPercent != plainDepthPoints["tip"] || next.Strokes.Top.AtPercent != 100 {
		t.Fatalf("a tip depth gave %+v: %v", next.Strokes, err)
	}
	_, next, _, err = ParseStrokeLab(`{"edits":{},"reply":"Nothing changes."}`, LabMethodGroove, current, limits)
	if err != nil || !reflect.DeepEqual(next, current) {
		t.Fatal("an empty edit changed the score", err)
	}
}

func TestStrokeLabRejectsIncompleteOrUnknownEdits(t *testing.T) {
	limits := strokeLabLimits()
	current := FreshStrokeLabScore(limits)
	for name, test := range map[string][2]string{
		"missing reply":     {LabMethodStrokeEnds, `{"edits":{}}`},
		"missing edits":     {LabMethodGroove, `{"reply":"Hi."}`},
		"partial turn":      {LabMethodStrokeEnds, `{"edits":{"bottom":{"at_percent":0}},"reply":"Deeper."}`},
		"turns too close":   {LabMethodStrokeEnds, `{"edits":{"bottom":{"at_percent":80,"vary_to_percent":80,"character":"steady"}},"reply":"Up."}`},
		"unknown field":     {LabMethodGroove, `{"edits":{"groove":{"bottom_at_percent":0,"top_at_percent":100,"feel":"steady","anchor":0}},"reply":"Full."}`},
		"old groove names":  {LabMethodGroove, `{"edits":{"groove":{"deepest_percent":0,"shallowest_percent":100,"feel":"steady"}},"reply":"Full."}`},
		"partial groove":    {LabMethodGroove, `{"edits":{"groove":{"bottom_at_percent":0}},"reply":"Deeper."}`},
		"unknown feel":      {LabMethodGroove, `{"edits":{"groove":{"bottom_at_percent":0,"top_at_percent":100,"feel":"bouncy"}},"reply":"Full."}`},
		"unknown word":      {LabMethodPlainWords, `{"edits":{"depth":"deepest"},"reply":"Deeper."}`},
		"pull back to base": {LabMethodPlainWords, `{"edits":{"pull_back":"base"},"reply":"Stay down."}`},
		"old length word":   {LabMethodPlainWords, `{"edits":{"length":"short"},"reply":"Shorter."}`},
		"unknown accent":    {LabMethodPlainWords, `{"edits":{"accents":["spins"]},"reply":"Spins."}`},
		"repeated accent":   {LabMethodStrokeEnds, `{"edits":{"accents":[{"move":"plunge","rate":"often"},{"move":"plunge","rate":"rarely"}]},"reply":"Plunges."}`},
		"speed above limit": {LabMethodStrokeEnds, `{"edits":{"speed_percent":40},"reply":"Faster."}`},
	} {
		_, next, _, err := ParseStrokeLab(test[1], test[0], current, limits)
		if err == nil || !reflect.DeepEqual(next, current) {
			t.Fatalf("%s was accepted or changed the score", name)
		}
	}
	if _, _, _, err := ParseLLMLab(`{"reply":"Hi.","controls":{}}`, "controls", current, limits); err == nil {
		t.Fatal("a comparison method accepted a stroke score")
	}
}

func TestStrokeVocabulariesRoundTrip(t *testing.T) {
	limits := strokeLabLimits()
	for _, depth := range plainDepths {
		for _, pullBack := range plainPullBacks {
			for _, variety := range grooveFeels {
				words := plainWords{Depth: depth, PullBack: pullBack, Variety: variety, Accents: []string{}, AccentRate: "sometimes", Pace: "fast"}
				spec := FreshStrokeLabScore(limits)
				spec.SpeedPercent = plainSpeed(words.Pace, limits)
				spec.Strokes.Bottom, spec.Strokes.Top, spec.Strokes.VariationPercent = plainTurns(words)
				if err := spec.Validate(limits); err != nil {
					t.Fatalf("%s %s %s: %v", depth, pullBack, variety, err)
				}
				if spec.Strokes.Bottom.AtPercent != plainDepthPoints[depth] {
					t.Fatalf("depth %s bottoms out at %d", depth, spec.Strokes.Bottom.AtPercent)
				}
				// Pull backs that give the same strokes may read back as the
				// one nearest the tip; the strokes they describe must not change.
				read := plainWordsOf(spec, limits)
				readBottom, readTop, readVariation := plainTurns(read)
				if readBottom != spec.Strokes.Bottom || readTop != spec.Strokes.Top || readVariation != spec.Strokes.VariationPercent ||
					read.Depth != depth || read.Pace != words.Pace || read.Variety != words.Variety {
					t.Fatalf("%+v read back as %+v", words, read)
				}
				if plainPullBackPoints[pullBack] >= plainDepthPoints[depth]+plainMinimumTravel && read.PullBack != pullBack {
					t.Fatalf("%s %s read back as pull back %s", depth, pullBack, read.PullBack)
				}
				// A breathing stroke that reaches the base or the tip holds that
				// end still, which a groove's single feel cannot state.
				anchored := variety == "breathing" && (spec.Strokes.Bottom.AtPercent == 0 || spec.Strokes.Top.AtPercent == 100)
				bottomAt, topAt, feel := grooveOf(*spec.Strokes)
				bottom, top, _ := grooveTurns(bottomAt, topAt, feel)
				if (bottom != spec.Strokes.Bottom || top != spec.Strokes.Top) && !anchored {
					t.Fatalf("groove %d-%d %s did not reproduce %+v", bottomAt, topAt, feel, spec.Strokes)
				}
			}
		}
	}
}

func TestStrokeLabSchemasAndContexts(t *testing.T) {
	limits := strokeLabLimits()
	current := FreshStrokeLabScore(limits)
	for _, method := range []string{LabMethodStrokeEnds, LabMethodGroove, LabMethodPlainWords} {
		var schema map[string]any
		if err := json.Unmarshal(LLMLabSchema(method, limits), &schema); err != nil {
			t.Fatal(method, err)
		}
		if required := schema["required"]; !reflect.DeepEqual(required, []any{"edits", "reply"}) {
			t.Fatalf("%s schema requires %v", method, required)
		}
		encoded, _ := json.Marshal(strokeLabContext(method, current, limits))
		if len(encoded) < 20 || strings.Contains(string(encoded), "null") {
			t.Fatalf("%s context is incomplete: %s", method, encoded)
		}
		if !strings.Contains(StrokeLabContinuationMessage(method), "AUTOPILOT") {
			t.Fatalf("%s continuation omits the shared judgment", method)
		}
	}
	if words := plainWordsOf(current, limits); words.Depth != "lower" || words.PullBack != "upper" || words.Variety != "breathing" || words.Pace != "medium" {
		t.Fatalf("the fresh score reads as %+v", words)
	}
	if _, _, feel := grooveOf(*current.Strokes); feel != "breathing" {
		t.Fatalf("the fresh score's groove feel is %s", feel)
	}
	if err := current.Validate(limits); err != nil {
		t.Fatal(err)
	}
	if _, err := motion.FlowTarget(current, limits); err != nil {
		t.Fatal(err)
	}
}
