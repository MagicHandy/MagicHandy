package chat

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestHostedRegionsRoundTripAndRedundantRemoval(t *testing.T) {
	before := DefaultLayeredScore(42)
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before}
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}
	schema := InteractiveResponseSchema(nil, caps, &state)
	if _, err := llm.OpenAISchema(schema); err != nil {
		t.Fatal(err)
	}
	wire := `{"proposal":{"action":"update","edits":{"composition":"phrase","phrase":{"regions":[{"bottom_percent":0,"top_percent":30,"count":1,"rate_percent":100,"cycle":"up_down"},{"bottom_percent":70,"top_percent":100,"count":1,"rate_percent":100,"cycle":"down_up"}]}},"reply":"Lower and upper cycles with connecting travel."}}`
	domain, err := llm.DomainOutput(wire, schema)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Capabilities: &caps, MotionContext: &state}
	response, err := service.parseLayeredResponse(domain, before, state, config.DefaultSettings().Motion, false)
	if err != nil || response.Motion == nil || response.Motion.Layered == nil {
		t.Fatal("regional cycles rejected", err)
	}
	score := *response.Motion.Layered
	if _, err := motion.FlowTarget(score, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"action":"update","edits":{"composition":"manual","layers":{"center":{"op":"remove"},"range":{"op":"remove"}}},"reply":"No field layers to remove."}`,
		strings.Replace(domain, `"composition":"phrase"`, `"composition":"phrase","layers":{"center":{"op":"remove"},"range":{"op":"remove"}}`, 1),
	} {
		response, err := service.parseLayeredResponse(raw, score, state, config.DefaultSettings().Motion, false)
		if err != nil || response.Motion != nil {
			t.Fatalf("equivalent removal discarded or retargeted a phrase: %v %+v", err, response.Motion)
		}
	}
	if _, _, _, err := parseHostedLayeredReply(strings.Replace(domain, `"top_percent":30`, `"top_percent":5`, 1), score, config.DefaultSettings().Motion); err == nil {
		t.Fatal("invalid region was silently fixed")
	}
	local := string(LayeredResponseSchema(config.DefaultSettings().Motion, false))
	if strings.Contains(local, `"turns"`) || strings.Contains(local, `"phrase"`) {
		t.Fatal("hosted regional grammar expanded local vocabulary")
	}
}

func TestLayeredAfterEdgeHasNoHiddenSoftnessOrSpeedCap(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	prior := DefaultLayeredScore(75)
	prior.MinPercent, prior.MaxPercent = 0, 100
	priorTarget, err := motion.FlowTarget(prior, limits)
	if err != nil {
		t.Fatal(err)
	}
	hold, err := motion.HoldingTarget(motion.ActiveMotionState{Target: priorTarget}, limits)
	if err != nil {
		t.Fatal(err)
	}
	before := *motion.CloneFlowSpec(hold.Flow)
	if before.TurnSoftnessPercent != 0 || before.CadenceHoldPercent != 0 || before.SpeedPercent != 20 {
		t.Fatal("holding motion introduced hidden shaping or lost its reduced pace")
	}
	for _, raw := range []string{
		`{"edits":{},"reply":"Unchanged."}`,
		`{"edits":{"controls":{"speed_percent":{"op":"set","value":20}}},"reply":"Same pace."}`,
	} {
		_, after, changed, err := parseHostedLayeredReply(raw, before, limits)
		if err != nil || len(changed) != 0 || !reflect.DeepEqual(before, after) {
			t.Fatalf("no-op changed the hold: %v", err)
		}
	}
	raw := `{"edits":{"composition":"full_slider","controls":{"speed_percent":{"op":"set","value":100}}},"reply":"Full travel at maximum speed."}`
	_, after, _, err := parseHostedLayeredReply(raw, before, limits)
	if err != nil || after.TurnSoftnessPercent != 0 || before.SpeedPercent != 20 {
		t.Fatalf("hold shaping leaked or mutated the active score: %+v %v", after, err)
	}
	target, err := motion.FlowTarget(after, limits)
	if err != nil {
		t.Fatal(err)
	}
	plan := motion.NewMotionPlan("released", target, limits, 0, 0, time.Unix(0, 0))
	if plan.Perceptual.CommandedMeanTravelPerSecond < 280 {
		t.Fatalf("released motion remains capped: %+v", plan.Perceptual.Pace)
	}
	// Real authored softness is still honored, even when its value happens to
	// match Edge. No numeric-value heuristic decides ownership.
	prior.TurnSoftnessPercent = 85
	_, authored, _, err := parseHostedLayeredReply(raw, prior, limits)
	if err != nil || authored.TurnSoftnessPercent != 85 {
		t.Fatal("authored softness was discarded", err)
	}
	bad := strings.Replace(raw, `"value":100`, `"value":101`, 1)
	_, rejected, _, err := parseHostedLayeredReply(bad, before, limits)
	if err == nil || !reflect.DeepEqual(rejected, before) {
		t.Fatal("rejected patch released the hold")
	}
}

func TestHostedPhraseRecallBeforeOperationsAndAuthority(t *testing.T) {
	current, earlier := DefaultLayeredScore(25), DefaultLayeredScore(35)
	earlier.Seed, earlier.PaceVariationPercent = 932, 0
	earlier.Layers = nil
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true}
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &current, SpeedMinPercent: 1, SpeedMaxPercent: 100,
		EarlierScores: []EarlierScore{{Flow: earlier}}}
	raw := `{"action":"update","edits":{"composition":"phrase","recall":1,"controls":{"speed_percent":{"op":"adjust","value":10}},"phrase":{"strokes":[{"bottom_percent":10,"top_percent":55,"count":3,"rate_percent":100},{"bottom_percent":10,"top_percent":90,"count":1,"rate_percent":100}]},"layers":{"pace":{"op":"edit","value":{"amount_percent":55,"low_rate_percent":55,"period_cycles":{"op":"set","value":8},"shape":"wave"}}}},"reply":"Earlier motion with counted travel and a faster pace."}`
	service := Service{Capabilities: &caps, MotionContext: &state}
	response, err := service.parseLayeredResponse(raw, current, state, config.DefaultSettings().Motion, false)
	if err != nil || response.Motion == nil {
		t.Fatalf("advertised phrase recall rejected: %v", err)
	}
	next := response.Motion.Layered
	if next.SpeedPercent != 45 || next.Seed != earlier.Seed || next.Phrase == nil || next.Phrase.Strokes[0].Count != 3 || len(next.Layers) != 1 || next.Layers[0].LowRatePercent != 55 {
		t.Fatalf("recall or independent pace lost: %+v", next)
	}
	if _, err := motion.FlowTarget(*next, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(raw, `"recall":1`, `"recall":2`, 1),
		strings.Replace(raw, `"recall":1`, `"recall":0`, 1),
		strings.Replace(raw, `"recall":1`, `"recall":1,"recall":1`, 1),
		strings.Replace(raw, `"pace":`, `"center":`, 1),
	} {
		if _, err := service.parseLayeredResponse(invalid, current, state, config.DefaultSettings().Motion, false); err == nil {
			t.Fatalf("invalid recalled phrase accepted: %s", invalid)
		}
	}
	state.Paused = true
	if _, err := service.parseLayeredResponse(raw, current, state, config.DefaultSettings().Motion, false); err == nil {
		t.Fatal("recall resumed a paused device")
	}
}

func TestLayeredInvalidRelativeControlsReportResolvedValue(t *testing.T) {
	before := DefaultLayeredScore(35)
	before.AnchorPercent = 0
	for _, field := range []struct {
		name  string
		value int
		want  string
	}{
		{"anchor_percent", -5, "anchor_percent resolves to -5; must be within 0..100"},
		{"memory_cycles", 30, "memory_cycles resolves to 40; must be within 2..32"},
		{"pace_variation_percent", 40, "pace_variation_percent resolves to 52; must be within 0..40"},
	} {
		raw := fmt.Sprintf(`{"edits":{"composition":"manual","controls":{%q:{"op":"adjust","value":%d}}},"reply":"Test."}`, field.name, field.value)
		_, after, _, err := parseHostedLayeredReply(raw, before, config.DefaultSettings().Motion)
		if err == nil || !strings.Contains(err.Error(), field.want) || !reflect.DeepEqual(after, before) {
			t.Fatalf("bounds error lost field or atomicity: %v", err)
		}
	}
}
