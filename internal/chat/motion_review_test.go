package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestMotionReviewOnlyAcceptsBoundedCorrections(t *testing.T) {
	limits := config.DefaultSettings().Motion
	current := FreshCreativeV2Score(25)
	service := MotionReviewService{Capabilities: Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}}
	state := MotionContext{Layered: &current, SpeedPercent: 25, MotionMode: MotionModeCreativeV2}
	for _, test := range []struct {
		name, correction string
		valid            bool
	}{
		{"edit", `{"edits":[{"speed_percent":30}],"reply":"Pace corrected."}`, true},
		{"empty", `{"edits":[],"reply":"No edit."}`, false},
		{"start", `{"action":"start","edits":[{"speed_percent":30}],"reply":"Start."}`, false},
		{"stop", `{"action":"stop","edits":[{"speed_percent":30}],"reply":"Stop."}`, false},
		{"standing_hold", `{"stay_unchanged":true,"edits":[{"speed_percent":30}],"reply":"Hold."}`, false},
		{"limits", `{"edits":[{"speed_percent":101}],"reply":"Too fast."}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := `{"verdict":"correct","confidence":"high","reason":"wrong_pace","feedback":"Pace mismatch.","correction":` + test.correction + `}`
			result, err := service.parse(raw, state, limits)
			if (err == nil) != test.valid {
				t.Fatalf("review=%+v err=%v", result, err)
			}
			if test.valid && (result.Motion.Action != MotionActionUpdate || result.Motion.Layered.SpeedPercent != 30) {
				t.Fatalf("correction=%+v", result.Motion)
			}
		})
	}
	for _, raw := range []string{
		`{"verdict":"keep","confidence":"low","reason":"wrong_pace","correction":null}`,
		`{"verdict":"correct","confidence":"medium","reason":"wrong_pace","correction":{}}`,
		`{"verdict":"keep","confidence":"high","reason":"none","correction":{},"extra":1}`,
		`{"verdict":"keep","confidence":"high","reason":"none","correction":null} {}`,
	} {
		if _, err := service.parse(raw, state, limits); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestMotionReviewRejectsCosmeticSeedChange(t *testing.T) {
	current := FreshCreativeV2Score(25)
	service := MotionReviewService{Capabilities: Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}}
	raw := `{"verdict":"correct","confidence":"high","reason":"missed_edit","feedback":"Refresh seed.","correction":{"edits":[{"evolve":true}],"reply":"Refreshed."}}`
	result, err := service.parse(raw, MotionContext{Layered: &current, MotionMode: MotionModeCreativeV2}, config.DefaultSettings().Motion)
	if err != nil || result.Verdict != "keep" || result.Motion != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestMotionReviewEvidenceOmitsPersonaAndPriorScores(t *testing.T) {
	// Its typed evidence has no persona or memory fields. Context also drops the
	// ordinary model's earlier scores and user requests in favor of its bounded input.
	current := FreshCreativeV2Score(25)
	encoded, err := json.Marshal(reviewTarget(motion.MotionTarget{Flow: &current, Label: "private persona label"}))
	if err != nil || strings.Contains(string(encoded), "private") || !strings.Contains(string(encoded), "seed") {
		t.Fatalf("evidence %s: %v", encoded, err)
	}
}
