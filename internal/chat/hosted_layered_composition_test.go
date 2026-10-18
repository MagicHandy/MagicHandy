package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestHostedLayeredCompositionDiscriminatorsAndFirstPassExamples(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	_ = json.Unmarshal(layeredResponseSchema(config.DefaultSettings().Motion, false, true), &schema)
	var edits struct {
		OneOf []struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"oneOf"`
	}
	_ = json.Unmarshal(schema.Properties["edits"], &edits)
	for i, choice := range []string{"manual", "geometry", "full_slider"} {
		if got := string(edits.OneOf[i+1].Properties["composition"]); got != `{"const":"`+choice+`"}` {
			t.Fatalf("composition is not uniquely selected before controls: %s", got)
		}
	}
	contract := hostedLayeredContract(layeredContract)
	if strings.Contains(contract, `"layers":[`) || strings.Contains(contract, `"remove_layers":`) || strings.Contains(contract, "Emit controls before geometry") || hostedLayeredContract(contract) != contract {
		t.Fatal("first contract pass contains conflicting legacy examples or ordering")
	}
}

func TestHostedLayeredGeometryCompositionRetainsDomainResults(t *testing.T) {
	limits := config.DefaultSettings().Motion
	current := DefaultLayeredScore(25)
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true}
	state := MotionContext{Running: true, Autopilot: true, Layered: &current, MotionMode: MotionModeLayered}
	schema := InteractiveResponseSchema(nil, caps, &state)
	for _, geometry := range layeredGeometries {
		t.Run(geometry, func(t *testing.T) {
			wire := `{"proposal":{"action":"update","stay_unchanged":false,"reply":"Test.","edits":{"composition":"geometry","geometry":"` + geometry + `","controls":null,"layers":null,"evolve":null}}}`
			if geometry == "full_slider" {
				wire = `{"proposal":{"action":"update","stay_unchanged":false,"reply":"Test.","edits":{"composition":"full_slider","controls":null,"layers":null,"evolve":null}}}`
			}
			domain, err := llm.DomainOutput(wire, schema)
			if err != nil {
				t.Fatal(err)
			}
			_, got, _, err := parseHostedLayeredReply(domain, current, limits)
			want, domainErr := ApplyLayeredEdit(LayeredEdit{Geometry: geometry}, current, limits)
			if err != nil || domainErr != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("named operation changed semantics: %v %v", err, domainErr)
			}
		})
	}
}

func TestHostedLayerOperationsAreExclusiveAndKeepDomainEdits(t *testing.T) {
	current := DefaultLayeredScore(25)
	limits := config.DefaultSettings().Motion
	good := `{"edits":{"composition":"manual","layers":{"center":{"op":"remove"},"pace":{"op":"edit","value":{"period_cycles":{"op":"adjust","value":4}}}}},"reply":"Test."}`
	_, got, _, err := parseHostedLayeredReply(good, current, limits)
	_, want, _, domainErr := ParseLayeredReply(`{"edits":{"remove_layers":["center"],"layers":[{"axis":"pace","period_change_cycles":4}]},"reply":"Test."}`, current, limits)
	if err != nil || domainErr != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("axis operations changed semantics: %v %v", err, domainErr)
	}
	for _, edits := range []string{
		`{"layers":{"pace":{"op":"remove","amount_percent":20}}}`,
		`{"layers":{"pace":{"op":"edit","axis":"center"}}}`,
		`{"layers":{"pace":{"op":"edit","value":{"axis":"center"}}}}`,
		`{"layers":{"pace":{"op":"edit","value":null}}}`,
		`{"layers":{"pace":{"op":"remove","value":{}}}}`,
		`{"layers":{"pace":{"op":"edit","period_cycles":{"op":"adjust","value":4}}},"remove_layers":["pace"]}`,
		`{"layers":{"pace":{"op":"edit"},"pace":{"op":"remove"}}}`,
		`{"layers":{"invalid":{"op":"remove"}}}`,
		`{"layers":{"pace":null}}`,
	} {
		_, got, changed, err := parseHostedLayeredReply(`{"edits":`+edits+`,"reply":"Test."}`, current, limits)
		if err == nil || len(changed) != 0 || !reflect.DeepEqual(got, current) {
			t.Fatalf("invalid axis operation escaped: %s %v", edits, err)
		}
	}
}

func TestHostedLayeredCompositionRejectsCompetingShapeEdits(t *testing.T) {
	current := DefaultLayeredScore(25)
	before := motion.CloneFlowSpec(&current)
	for _, edits := range []string{
		`{"composition":null}`,
		`{"composition":"other"}`,
		`{"composition":"geometry"}`,
		`{"composition":"manual","geometry":"wander"}`,
		`{"composition":"geometry","geometry":"tip_anchor","controls":{"anchor_percent":{"op":"set","value":80}}}`,
		`{"composition":"geometry","geometry":"wander","controls":{"variation_mode":{"op":"set","value":"waves"}}}`,
		`{"composition":"geometry","geometry":"full_slider","controls":{"min_percent":{"op":"set","value":20}}}`,
		`{"composition":"geometry","geometry":"full_band","stroke_width":{"min_percent":20,"max_percent":40}}`,
		`{"composition":"geometry","geometry":"base_anchor","layers":[{"axis":"center","amount_percent":35}]}`,
		`{"composition":"geometry","geometry":"full_and_base","remove_layers":["range"]}`,
	} {
		_, got, changed, err := parseHostedLayeredReply(`{"edits":`+edits+`,"reply":"Test."}`, current, config.DefaultSettings().Motion)
		if err == nil || len(changed) != 0 || !reflect.DeepEqual(got, *before) || !reflect.DeepEqual(current, *before) {
			t.Fatalf("conflicting composition escaped: %s %v", edits, err)
		}
	}
}

func TestHostedLayeredManualCompositionKeepsCustomShapeFreedom(t *testing.T) {
	current := DefaultLayeredScore(25)
	raw := `{"edits":{"composition":"manual","controls":{"anchor_percent":{"op":"set","value":72},"variation_mode":{"op":"set","value":"waves"}},"stroke_width":{"min_percent":20,"max_percent":40},"layers":[{"axis":"center","amount_percent":60,"period_cycles":{"op":"set","value":17},"shape":"wave"}]},"reply":"Custom shape."}`
	_, got, _, err := parseHostedLayeredReply(raw, current, config.DefaultSettings().Motion)
	if err != nil || got.AnchorPercent != 72 || got.VariationMode != "waves" || got.RangeFloorPercent != 20 || got.RangeCeilingPercent != 40 || got.SpeedPercent != 25 {
		t.Fatalf("manual shape lost a control: %+v %v", got, err)
	}
}

func TestHostedLayeredRecallIsOfferedForEveryComposition(t *testing.T) {
	current := DefaultLayeredScore(25)
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &current, EarlierScores: []EarlierScore{{Flow: current}}}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true}
	var root map[string]any
	if err := json.Unmarshal(InteractiveResponseSchema(nil, caps, &state), &root); err != nil {
		t.Fatal(err)
	}
	branches := root["oneOf"].([]any)
	update := branches[1].(map[string]any)["properties"].(map[string]any)
	choices := update["edits"].(map[string]any)["oneOf"].([]any)
	for _, choice := range choices[1:] {
		properties := choice.(map[string]any)["properties"].(map[string]any)
		if properties["recall"] == nil {
			t.Fatal("composition lost recall")
		}
	}
}
