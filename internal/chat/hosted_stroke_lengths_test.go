package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestHostedStrokeLengthsPreserveDomainSemantics(t *testing.T) {
	before := DefaultLayeredScore(55)
	limits := config.DefaultSettings().Motion
	limits.SpeedMaxPercent = 100
	for _, width := range []string{`{"shortest_percent":20,"widest_percent":45}`, `{"min_percent":20,"max_percent":45}`} {
		raw := `{"edits":{"composition":"manual","controls":{"min_percent":{"op":"set","value":55},"max_percent":{"op":"set","value":100}},"stroke_width":` + width + `},"reply":"Expanded coverage."}`
		_, got, _, err := parseHostedLayeredReply(raw, before, limits)
		if err != nil || got.MinPercent != 55 || got.MaxPercent != 100 || got.RangeFloorPercent != 20 || got.RangeCeilingPercent != 45 || got.SpeedPercent != 55 {
			t.Fatalf("%s: %+v %v", width, got, err)
		}
	}
	for _, width := range []string{`{"shortest_percent":60,"widest_percent":100}`, `{"shortest_percent":20}`, `{"shortest_percent":20,"widest_percent":45,"min_percent":20}`, `{"shortest_percent":20,"shortest_percent":25,"widest_percent":45}`, `{"shortest_percent":null,"widest_percent":45}`, `{"shortest_percent":50,"widest_percent":20}`} {
		raw := `{"edits":{"composition":"manual","controls":{"min_percent":{"op":"set","value":55},"max_percent":{"op":"set","value":100}},"stroke_width":` + width + `},"reply":"Test."}`
		if _, _, _, err := parseHostedLayeredReply(raw, before, limits); err == nil {
			t.Fatalf("invalid lengths accepted: %s", width)
		}
	}
}

func TestStrokeLengthVocabularyStaysInAdvancedContract(t *testing.T) {
	score := DefaultLayeredScore(55)
	state := MotionContext{Layered: &score, MotionMode: MotionModeLayered}
	for _, advanced := range []bool{false, true} {
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: advanced}
		schema := string(layeredResponseSchema(config.DefaultSettings().Motion, false, advanced))
		context := layeredContextInstructions(state, caps)
		if strings.Contains(schema, "shortest_percent") != advanced || strings.Contains(context, "shortest_percent") != advanced {
			t.Fatal("capability vocabulary leaked")
		}
		recalled, err := json.Marshal(earlierScoresContext([]EarlierScore{{Flow: score}}, state, advanced))
		if err != nil || strings.Contains(string(recalled), "shortest_percent") != advanced {
			t.Fatal("recall vocabulary disagrees with current score")
		}
	}
	legacy := layeredScoreContext(score)
	width, ok := legacy["stroke_width"].(LayeredStrokeWidth)
	if !ok || width.MinPercent != 20 || width.MaxPercent != 90 {
		t.Fatal("local context changed")
	}
	example := `{"edits":{"stroke_width":{"min_percent":20,"max_percent":40}},"reply":"Test."}`
	converted := hostedLayeredExample(example)
	if !strings.Contains(converted, "shortest_percent") {
		t.Fatal("example not translated")
	}
	var first, second any
	_ = json.Unmarshal([]byte(converted), &first)
	_ = json.Unmarshal([]byte(hostedLayeredExample(converted)), &second)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("example translation is not idempotent")
	}
}

func TestHostedFullBandUsesRequestedEndpointsWithoutWidthConflict(t *testing.T) {
	before := DefaultLayeredScore(55)
	limits := config.DefaultSettings().Motion
	limits.SpeedMaxPercent = 100
	raw := `{"edits":{"composition":"full_band","controls":{"min_percent":{"op":"set","value":55},"max_percent":{"op":"set","value":95}}},"reply":"Fill the requested band."}`
	_, got, _, err := parseHostedLayeredReply(raw, before, limits)
	if err != nil || got.MinPercent != 55 || got.MaxPercent != 95 || got.RangeFloorPercent != 40 || got.RangeCeilingPercent != 40 || got.SpeedPercent != before.SpeedPercent {
		t.Fatalf("full band: %+v %v", got, err)
	}
	var root map[string]any
	if err = json.Unmarshal(layeredResponseSchema(limits, false, true), &root); err != nil {
		t.Fatal(err)
	}
	choices := root["properties"].(map[string]any)["edits"].(map[string]any)["oneOf"].([]any)
	found := false
	for _, choice := range choices {
		fields, ok := choice.(map[string]any)["properties"].(map[string]any)
		if !ok {
			continue
		}
		if fields["composition"].(map[string]any)["const"] == "full_band" {
			found = true
			if fields["stroke_width"] != nil || fields["geometry"] != nil {
				t.Fatal("contradictory full-band shape fields offered")
			}
		}
	}
	if !found {
		t.Fatal("full-band composition missing")
	}
}
