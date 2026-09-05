package chat

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLayeredPartialEditsPreserveOtherAxesAndEvolveGeometry(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(25)
	original := motion.CloneFlowSpec(&before)
	_, after, changed, err := ParseLayeredReply(`{"edits":{"layers":[{"axis":"pace","period_cycles":19}],"evolve":true},"reply":"Longer pace trend."}`, before, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 || after.Seed == before.Seed || after.Layers[1].PeriodCycles != 19 {
		t.Fatalf("incomplete change: %+v %v", after, changed)
	}
	expected := motion.CloneFlowSpec(&before)
	expected.Seed = after.Seed
	expected.Layers[1].PeriodCycles = 19
	if !reflect.DeepEqual(after, *expected) || !reflect.DeepEqual(before, *original) {
		t.Fatal("partial edit changed unsupplied fields or aliased original")
	}
	_, relative, _, err := ParseLayeredReply(`{"edits":{"layers":[{"axis":"pace","period_change_cycles":4}]},"reply":"Longer trend."}`, before, limits)
	if err != nil || relative.Layers[1].PeriodCycles != before.Layers[1].PeriodCycles+4 {
		t.Fatal("relative timing did not add to the current period", err)
	}
	for _, raw := range []string{
		`{"edits":{"controls":{"turn_softness_percent":80}},"reply":"bad"}`,
		`{"edits":{"change_by":{"speed_percent":-100}},"reply":"bad"}`,
		`{"edits":{"layers":[{"axis":"center","amount_percent":50},{"axis":"center","period_cycles":8}]},"reply":"bad"}`,
		`{"edits":{"layers":[{"axis":"pace","shape":"random"}]},"reply":"bad"}`,
		`{"edits":{"layers":[{"axis":"pace","period_cycles":8}],"remove_layers":["pace"]},"reply":"bad"}`,
		`{"edits":{"layers":[{"axis":"pace"}]},"reply":"bad"}`,
		`{"edits":{"layers":[{"axis":"pace","period_cycles":null}]},"reply":"bad"}`,
		`{"edits":{"evolve":null},"reply":"bad"}`,
		`{"edits":{"controls":null},"reply":"bad"}`,
		`{"edits":{"steps":[]},"reply":"bad"}`,
		`{"edits":{"layers":[{"axis":"pace","period_change_cycles":-30}]},"reply":"bad"}`,
		`{"edits":{"layers":[{"axis":"pace","period_cycles":8,"period_change_cycles":4}]},"reply":"bad"}`,
	} {
		_, got, _, err := ParseLayeredReply(raw, before, limits)
		if err == nil || !reflect.DeepEqual(got, before) || !reflect.DeepEqual(before, *original) {
			t.Fatalf("nontransactional rejection %s: %v", raw, err)
		}
	}
}

func TestFreshLayeredVariationKeepsTheAuthoredLimitsReplayable(t *testing.T) {
	base := DefaultLayeredScore(25)
	fresh := FreshLayeredScore(25)
	if fresh.Seed == 0 || fresh.Seed == base.Seed {
		t.Fatal("fresh conversation used the fixed fixture seed")
	}
	base.Seed = fresh.Seed
	if !reflect.DeepEqual(fresh, base) {
		t.Fatal("random realization changed authored controls")
	}
	for range 16 {
		next, err := ApplyLayeredEdit(LayeredEdit{Evolve: true}, fresh, config.DefaultSettings().Motion)
		if err != nil || next.Seed == 0 || next.Seed == fresh.Seed {
			t.Fatal("evolution repeated the current realization", err)
		}
		fresh.Seed = next.Seed
		if !reflect.DeepEqual(next, fresh) {
			t.Fatal("evolution changed limits or layer controls")
		}
	}
}

func TestLayeredGeometryExamplesAndSchema(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(25)
	_, localized, _, err := ParseLayeredReply(`{"edits":{"stroke_width":{"min_percent":20,"max_percent":20},"layers":[{"axis":"center","amount_percent":100,"period_cycles":8,"shape":"alternate"}],"remove_layers":["range"]},"reply":"Alternate both ends."}`, before, limits)
	if err != nil {
		t.Fatal(err)
	}
	_, broadTip, _, err := ParseLayeredReply(`{"edits":{"stroke_width":{"min_percent":20,"max_percent":90},"controls":{"anchor_percent":100},"layers":[{"axis":"range","amount_percent":100,"period_cycles":8,"shape":"alternate"}],"remove_layers":["center"]},"reply":"Broad and tip."}`, localized, limits)
	if err != nil {
		t.Fatal(err)
	}
	if broadTip.Layers[0] != before.Layers[1] || broadTip.SpeedPercent != 25 || len(broadTip.Layers) != 2 {
		t.Fatal("geometry edit lost ongoing pace variation")
	}
	for _, spec := range []motion.FlowSpec{localized, broadTip} {
		if _, err := motion.FlowTarget(spec, limits); err != nil {
			t.Fatal(err)
		}
	}
	schema := string(LayeredResponseSchema(limits, true))
	if !json.Valid([]byte(schema)) || strings.Contains(schema, "turn_softness") || strings.Contains(schema, "steps") || !strings.Contains(schema, "alternate") {
		t.Fatal("incorrect schema")
	}
}

func TestLayeredGeometryOperationsAreAtomicAndKeepPace(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(25)
	for _, geometry := range layeredGeometries {
		next, err := ApplyLayeredEdit(LayeredEdit{Geometry: geometry}, before, limits)
		if err != nil {
			t.Fatal(geometry, err)
		}
		if next.SpeedPercent != before.SpeedPercent || next.PaceVariationPercent != before.PaceVariationPercent {
			t.Fatal("geometry changed pace")
		}
		found := false
		for _, layer := range next.Layers {
			if layer == before.Layers[1] {
				found = true
			}
		}
		if !found {
			t.Fatal("geometry lost pace layer")
		}
		if _, err := motion.FlowTarget(next, limits); err != nil {
			t.Fatal(err)
		}
	}
	_, next, _, err := ParseLayeredReply(`{"edits":{"geometry":"alternate_ends","stroke_width":{"min_percent":20,"max_percent":90}},"reply":"bad"}`, before, limits)
	if err == nil || !reflect.DeepEqual(next, before) {
		t.Fatal("accepted contradictory geometry")
	}
	_, next, _, err = ParseLayeredReply(`{"edits":{"controls":{"min_percent":0,"max_percent":100},"geometry":"full_and_tip"},"reply":"Whole band."}`, before, limits)
	if err != nil || next.RangeCeilingPercent != 100 {
		t.Fatal("geometry did not adapt to the same-turn band edit", err)
	}
}

func TestLayeredContinuationHonorsExactHoldAndLaterChanges(t *testing.T) {
	hold := "Keep this exact pattern repeating. No changes from now on."
	for _, tc := range []struct {
		requests []string
		hold     bool
	}{
		{[]string{"Vary the motion.", hold}, true},
		{[]string{hold, "What does the pace layer do?"}, true},
		{[]string{hold, "Actually, vary the motion again."}, false},
		{[]string{"Alternate tip and base. Keep speed unchanged."}, false},
	} {
		if LayeredExactHoldRequested(tc.requests) != tc.hold {
			t.Fatal(tc.requests)
		}
	}
	provider := &layeredTestProvider{raw: `{"edits":{"evolve":true},"reply":"Fresh."}`}
	spec := DefaultLayeredScore(25)
	service := AutopilotService{Provider: provider, Capabilities: Capabilities{Motion: true, MotionMode: MotionModeLayered}, MotionContext: &MotionContext{Running: true, Layered: &spec, UserRequests: []string{hold}}}
	if _, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: "Continue"}); err == nil {
		t.Fatal("accepted evolution during exact hold")
	}
	if provider.calls != 1 || !strings.Contains(provider.request.Messages[len(provider.request.Messages)-1].Content, "HOLD EXACT") {
		t.Fatal("hold policy not provided without repair")
	}
}

type layeredTestProvider struct {
	raw     string
	calls   int
	request llm.ChatRequest
}

func (p *layeredTestProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Available: true}
}
func (p *layeredTestProvider) StreamChat(_ context.Context, request llm.ChatRequest, _ func(string) error) (string, error) {
	p.calls++
	p.request = request
	return p.raw, nil
}

func TestLayeredProductionAuthorityAndNoRepair(t *testing.T) {
	for _, tc := range []struct {
		name, message, raw              string
		running, paused, reject, motion bool
	}{
		{"start", "Start moving gently.", `{"edits":{},"reply":"Starting."}`, false, false, false, true},
		{"question", "What does the pace layer do?", `{"edits":{},"reply":"It varies pace."}`, true, false, false, false},
		{"question edit", "What does the pace layer do?", `{"edits":{"evolve":true},"reply":"Varying."}`, true, false, true, false},
		{"evolve", "Keep varying the motion.", `{"edits":{"evolve":true},"reply":"Fresh details."}`, true, false, false, true},
		{"gentler", "Keep the current motion but make it gentler.", `{"edits":{"change_by":{"speed_percent":-5}},"reply":"Five points slower."}`, true, false, false, true},
		{"paused", "Vary the motion.", `{"edits":{"evolve":true},"reply":"Fresh details."}`, true, true, true, false},
		{"refusal", "Do not move.", `{"edits":{"evolve":true},"reply":"Moving."}`, false, false, true, false},
		{"invalid", "Move gently.", `{"edits":{"controls":{"turn_softness_percent":70}},"reply":"Moving."}`, true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &layeredTestProvider{raw: tc.raw}
			spec := DefaultLayeredScore(25)
			service := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeLayered}, MotionContext: &MotionContext{Running: tc.running, Paused: tc.paused, Layered: &spec}}
			result, err := service.Complete(t.Context(), Request{Message: tc.message}, nil)
			if (err != nil) != tc.reject || (result.Response.Motion != nil) != tc.motion || result.Raw != tc.raw || provider.calls != 1 || result.Repaired || result.SemanticFallback {
				t.Fatalf("unexpected result %+v err=%v calls=%d", result, err, provider.calls)
			}
		})
	}
}
