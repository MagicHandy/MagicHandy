package chat

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
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
		facts := AutopilotMotionMessage(AutopilotContext{Hosted: advanced, MotionMode: MotionModeLayered, CurrentFlow: &score, CurrentSpeed: score.SpeedPercent})
		if strings.Contains(facts, "shortest_percent") != advanced {
			t.Errorf("Autopilot vocabulary disagrees with capability %t: %s", advanced, facts)
		}
		provider := &scriptedProvider{responses: []string{`{"verdict":"keep","confidence":"high","reason":"none","feedback":"No correction needed.","correction":null}`}}
		service := MotionReviewService{Provider: provider, Capabilities: caps}
		_, err = service.Complete(t.Context(), MotionReviewInput{Source: "chat", Before: motion.MotionTarget{Flow: &score}, After: motion.MotionTarget{Flow: &score}, Limits: config.DefaultSettings().Motion, Context: state})
		if err != nil {
			t.Fatal(err)
		}
		var evidence map[string]json.RawMessage
		if err := json.Unmarshal([]byte(provider.requests[0].Messages[1].Content), &evidence); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"before", "applied"} {
			if strings.Contains(string(evidence[key]), "shortest_percent") != advanced || (advanced && strings.Contains(string(evidence[key]), `"stroke_width":{"min_percent"`)) {
				t.Errorf("review %s vocabulary disagrees with capability %t: %s", key, advanced, evidence[key])
			}
		}
	}
	legacy := layeredScoreContext(score, false)
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

func TestHostedStrokeLengthErrorsUseAdvertisedFields(t *testing.T) {
	limits := config.DefaultSettings().Motion
	for _, tc := range []struct {
		name, edits, want string
	}{
		{"geometry conflict", `"geometry":"alternate_ends","controls":{"min_percent":{"op":"set","value":80},"max_percent":{"op":"set","value":100}},"stroke_width":{"shortest_percent":15,"widest_percent":20}`, "stroke_width.widest_percent <= 10"},
		{"reversed lengths", `"stroke_width":{"shortest_percent":50,"widest_percent":20}`, "shortest_percent <= widest_percent"},
		{"length exceeds band", `"controls":{"min_percent":{"op":"set","value":80},"max_percent":{"op":"set","value":100}},"stroke_width":{"shortest_percent":30,"widest_percent":40}`, "stroke_width.shortest_percent resolves to 30"},
		{"ceiling exceeds band", `"controls":{"min_percent":{"op":"set","value":80},"max_percent":{"op":"set","value":100}},"stroke_width":{"shortest_percent":10,"widest_percent":40}`, "stroke_width.widest_percent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"edits":{` + tc.edits + `},"reply":"Test."}`
			before := DefaultLayeredScore(25)
			_, after, _, err := parseHostedLayeredReply(raw, before, limits)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "stroke_width.max_percent") || strings.Contains(err.Error(), "range_floor_percent") || strings.Contains(err.Error(), "range ceiling") {
				t.Fatalf("repair refers to fields absent from its schema: %v", err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected edit mutated motion")
			}
			// The domain diagnostics remain unchanged for Simple/Intermediate,
			// and the Advanced wrapper preserves the typed underlying conflict.
			normalized, normalizeErr := normalizeLayeredOperations(raw)
			if normalizeErr != nil {
				t.Fatal(normalizeErr)
			}
			_, _, _, localErr := ParseLayeredReply(normalized, before, limits)
			if localErr == nil || strings.Contains(localErr.Error(), "shortest_percent") || strings.Contains(localErr.Error(), "widest_percent") {
				t.Fatalf("local diagnostic changed: %v", localErr)
			}
			if tc.name == "geometry conflict" {
				var conflict *LayeredGeometryConflict
				if !errors.As(err, &conflict) {
					t.Fatal("Advanced diagnostic lost its typed geometry conflict")
				}
			}
		})
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
