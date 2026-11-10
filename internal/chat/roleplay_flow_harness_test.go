//go:build liveeval

package chat

import (
	"encoding/json"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestRoleplayCaptureDoesNotRenderStoppedMotion(t *testing.T) {
	t.Setenv("MAGICHANDY_ROLEPLAY_MODE", "layered")
	provider := &hostedLayeredCapture{Provider: &scriptedProvider{responses: []string{
		`{"action":"stop","edits":{},"reply":"Stopped."}`,
		`{"action":"none","edits":{},"reply":"You are welcome."}`,
	}}}
	rows := runRoleplayScene(t, provider, "scripted", roleplayScene{key: "stop", running: true, speed: 30,
		persona: roleplayPersona{voice: VoiceUtility}, turns: []string{"Stop now.", "Thank you."}})
	if len(rows) != 2 {
		t.Fatalf("got %d turns", len(rows))
	}
	for _, row := range rows {
		if _, hasMovingScore := row["after"]; hasMovingScore || row["selected"] != "stop" || row["running_after"] != false {
			t.Fatalf("stopped turn would be rendered as motion: %v", row)
		}
		if _, claimsIntent := row["intent_pass"]; claimsIntent {
			t.Fatal("contract acceptance must not be labelled an intent pass")
		}
	}
}

func TestRoleplayJudgeRejectsMalformedLabels(t *testing.T) {
	label := `{"onset":"none","pace":"steady","depth":"unstated","length":"unstated","withholding":false,"later_only":false}`
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"missing", "pace", nil}, {"unknown", "extra", true},
		{"wrong enum", "pace", "rapid"}, {"wrong type", "withholding", "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(label), &decoded); err != nil {
				t.Fatal(err)
			}
			if err := validateRoleplayJudgment(decoded, roleplayTurnJudgeSchema); err != nil {
				t.Fatal(err)
			}
			decoded[tc.key] = tc.value
			if err := validateRoleplayJudgment(decoded, roleplayTurnJudgeSchema); err == nil {
				t.Fatal("accepted malformed judge label")
			}
		})
	}
	for _, score := range []float64{0, 1.5, 6} {
		decoded := map[string]any{"coherence": score, "responsiveness": float64(4), "character": float64(3), "note": "Consistent."}
		if err := validateRoleplayJudgment(decoded, roleplaySceneJudgeSchema); err == nil {
			t.Errorf("accepted invalid rating %v", score)
		}
	}
}

func TestLifecycleCountedPatternAcceptsBothPhraseContracts(t *testing.T) {
	groups := []motion.PhraseStroke{{BottomPercent: 75, TopPercent: 100, Count: 2, RatePercent: 100},
		{BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100}}
	for _, phrase := range []*motion.FlowPhrase{
		{Strokes: groups},
		{Regions: []motion.PhraseRegion{{PhraseStroke: groups[0], Cycle: "up_down"}, {PhraseStroke: groups[1], Cycle: "up_down"}}},
	} {
		command := &MotionCommand{Action: MotionActionStart, Layered: &motion.FlowSpec{Phrase: phrase}}
		if !lifecycleCountedPattern(command) {
			t.Fatal("rejected valid two-short/one-full pattern")
		}
		if len(phrase.Strokes) > 0 {
			phrase.Strokes[1].BottomPercent = 20
		} else {
			phrase.Regions[1].BottomPercent = 20
		}
		if lifecycleCountedPattern(command) {
			t.Fatal("accepted pattern missing the base")
		}
	}
}

// The scene is deliberately ordinary conversation. This fresh live check tests
// lifecycle authority and feedback, not generation of explicit persona prose.
func TestHostedSceneLifecycleLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := runRoleplayScene(t, provider, model, roleplayScene{key: "utility-feedback", speed: 30,
		persona: roleplayPersona{key: "utility", name: "Alex", voice: VoiceUtility, style: StyleNeutral},
		turns: []string{"Hello. I am getting everything ready.", "All set. You can begin now.",
			"That pace works well. Keep the motion exactly as it is.", "I am done for now. End playback and leave the device still.",
			"Thank you, that last run worked well."}})
	saveHostedLayeredRows(t, rows)
	want := []bool{false, true, true, false, false}
	for i, row := range rows {
		if row["running_after"] != want[i] || row["repaired"] != false || row["fallback"] != false {
			t.Errorf("turn %d did not satisfy lifecycle without repair; retained in capture", i+1)
		}
	}
}
