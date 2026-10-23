package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const phraseTestProposal = `{"edits":{"composition":"phrase","phrase":{"strokes":[{"bottom_percent":63,"top_percent":100,"count":2,"rate_percent":100},{"bottom_percent":0,"top_percent":100,"count":1,"rate_percent":70}]}},"reply":"Counted motion."}`

func TestHostedLayeredPhraseIsTransactionalAndEditable(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(30)
	_, score, _, err := parseHostedLayeredReply(phraseTestProposal, before, limits)
	if err != nil {
		t.Fatal(err)
	}
	if score.Phrase == nil || score.MinPercent != 0 || score.MaxPercent != 100 || !samePaceLayerValue(before, score) || score.SpeedPercent != before.SpeedPercent {
		t.Fatal("phrase did not replace only the intended content")
	}
	for _, tc := range []struct {
		raw    string
		phrase bool
	}{
		{`{"edits":{"composition":"manual","controls":{"speed_percent":{"op":"adjust","value":5}}},"reply":"Faster."}`, true},
		{`{"edits":{"composition":"manual","evolve":true},"reply":"Fresh timing."}`, true},
		{`{"edits":{"composition":"geometry","geometry":"wander"},"reply":"Wandering."}`, false},
		{`{"edits":{"composition":"manual","stroke_width":{"min_percent":25,"max_percent":45}},"reply":"New widths."}`, false},
	} {
		_, after, _, err := parseHostedLayeredReply(tc.raw, score, limits)
		if err != nil {
			t.Fatal(err)
		}
		if (after.Phrase != nil) != tc.phrase {
			t.Fatalf("wrong phrase retention: %s", tc.raw)
		}
		if tc.phrase && !reflect.DeepEqual(after.Phrase, score.Phrase) {
			t.Fatal("independent edit changed counts/reach/rates")
		}
		if after.Phrase != nil {
			after.Phrase.Strokes[0].Count = 8
			if score.Phrase.Strokes[0].Count != 2 {
				t.Fatal("edit aliases playing phrase")
			}
		}
	}
	for _, bad := range []string{
		strings.Replace(phraseTestProposal, `"bottom_percent":0,`, "", 1),
		strings.Replace(phraseTestProposal, `"bottom_percent":0`, `"bottom_percent":null`, 1),
		strings.Replace(phraseTestProposal, `"bottom_percent":0`, `"bottom_percent":0,"bottom_percent":10`, 1),
		strings.Replace(phraseTestProposal, `"top_percent":100`, `"top_percent":60`, 1),
		strings.Replace(phraseTestProposal, `"count":2`, `"count":9`, 1),
		strings.Replace(phraseTestProposal, `"composition":"phrase"`, `"composition":"manual"`, 1),
		strings.Replace(phraseTestProposal, `"composition":"phrase"`, `"composition":"phrase","controls":{"min_percent":{"op":"set","value":10}}`, 1),
	} {
		_, after, _, err := parseHostedLayeredReply(bad, score, limits)
		if err == nil || !reflect.DeepEqual(after, score) {
			t.Fatalf("invalid phrase accepted or mutated score: %s", bad)
		}
	}
}

func TestLayeredPhraseHostedOnlyContractAndSummary(t *testing.T) {
	limits := config.DefaultSettings().Motion
	local := string(LayeredResponseSchema(limits, false))
	if strings.Contains(local, `"phrase"`) {
		t.Fatal("new hosted vocabulary enlarged local output schema")
	}
	score := DefaultLayeredScore(25)
	state := MotionContext{Layered: &score, MotionMode: MotionModeLayered, CommandedSummary: &motion.PerceptualSummary{PositionMinPercent: 14.25, PositionMaxPercent: 85.8, MeanStrokePercent: 30}}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered}
	plain := motionContextInstructions(state, caps, nil)
	state.CommandedSummary = nil
	if plain != motionContextInstructions(state, caps, nil) {
		t.Fatal("compiled context enlarged local-only prompt")
	}
	if contractInstructions(caps) != layeredContract {
		t.Fatal("compact local contract changed")
	}
	caps.HostedMotion = true
	state.CommandedSummary = &motion.PerceptualSummary{PositionMinPercent: 14.25, PositionMaxPercent: 85.8, MeanStrokePercent: 30}
	if text := motionContextInstructions(state, caps, nil); !strings.Contains(text, `"min_percent":14.25`) || !strings.Contains(text, "not merely permission bounds") {
		t.Fatal("hosted context hid actual planned reach")
	}
	var root map[string]any
	if json.Unmarshal(layeredResponseSchema(limits, false, true), &root) != nil {
		t.Fatal("invalid schema")
	}
}

func TestHostedNamedGeometryAcceptsCompatibleCustomCoverage(t *testing.T) {
	before := DefaultLayeredScore(32)
	before.MinPercent, before.MaxPercent = 0, 100
	for _, width := range []int{24, 37, 49} {
		raw, _ := json.Marshal(map[string]any{"edits": map[string]any{"composition": "geometry", "geometry": "full_and_tip", "stroke_width": map[string]int{"min_percent": width, "max_percent": 100}}, "reply": "Custom coverage."})
		_, after, _, err := parseHostedLayeredReply(string(raw), before, config.DefaultSettings().Motion)
		if err != nil {
			t.Fatal(err)
		}
		if after.RangeFloorPercent != width || after.AnchorPercent != 100 || after.SpeedPercent != before.SpeedPercent || !samePaceLayerValue(before, after) {
			t.Fatal("custom coverage changed unrelated pace or anchor")
		}
	}
}

func samePaceLayerValue(a, b motion.FlowSpec) bool {
	pace := func(s motion.FlowSpec) motion.FlowLayer {
		for _, l := range s.Layers {
			if l.Axis == "pace" {
				return l
			}
		}
		return motion.FlowLayer{}
	}
	return pace(a) == pace(b) && a.PaceVariationPercent == b.PaceVariationPercent
}

func TestPhraseFieldExitKeepsSharedAnchorAndRejectsFollowing(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(30)
	_, phrase, _, err := parseHostedLayeredReply(phraseTestProposal, before, limits)
	if err != nil {
		t.Fatal(err)
	}
	_, field, _, err := parseHostedLayeredReply(`{"edits":{"composition":"manual","stroke_width":{"min_percent":20,"max_percent":40}},"reply":"Field coverage."}`, phrase, limits)
	if err != nil || field.Phrase != nil || field.AnchorPercent != 100 {
		t.Fatalf("tip anchor lost on field exit: %+v %v", field, err)
	}
	bad := strings.Replace(phraseTestProposal, `"composition":"phrase"`, `"composition":"phrase","layers":{"pace":{"op":"follow","value":{"axis":"center","relation":"same","amount_percent":60,"low_rate_percent":50}}}`, 1)
	_, after, _, err := parseHostedLayeredReply(bad, before, limits)
	if err == nil || !reflect.DeepEqual(after, before) {
		t.Fatal("phrase accepted a field-axis follow edit")
	}
	// Existing follow is already materialized into independent timing. Keep
	// that envelope when the source geometry is replaced, without a dangling link.
	_, followed, _, err := parseHostedLayeredReply(`{"edits":{"composition":"geometry","geometry":"alternate_ends","layers":{"pace":{"op":"follow","value":{"axis":"center","relation":"same","amount_percent":60,"low_rate_percent":50}}}},"reply":"Linked timing."}`, before, limits)
	if err != nil {
		t.Fatal(err)
	}
	_, frozen, _, err := parseHostedLayeredReply(phraseTestProposal, followed, limits)
	if err != nil || !samePaceLayerValue(followed, frozen) {
		t.Fatal("materialized pace changed when its source was replaced")
	}
	_, released, _, err := parseHostedLayeredReply(`{"edits":{"composition":"geometry","geometry":"alternate_ends","layers":{"pace":{"op":"follow","value":{"axis":"center","relation":"same","amount_percent":60,"low_rate_percent":50}}}},"reply":"Return to linked field motion."}`, phrase, limits)
	if err != nil || released.Phrase != nil {
		t.Fatal("leaving a phrase prevented ordinary field following")
	}
}
