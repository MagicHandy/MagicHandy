package chat

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestCreativeV2PartialEditsAndRejectInvalidTransactions(t *testing.T) {
	s := FreshCreativeV2Score(25)
	limits := config.DefaultSettings().Motion
	_, next, _, err := ParseCreativeV2Reply(`{"edits":[{"focus":{"position_percent":0,"width_percent":45,"mix_percent":40}},{"rebounds":{"count":3,"retained_width_percent":75}}],"reply":"Base rebounds."}`, s, limits)
	if err != nil || next.Gesture.FocusPercent != 0 || next.Gesture.ReboundCount != 3 || next.SpeedPercent != 25 || s.Gesture.FocusPercent != 100 {
		t.Fatalf("partial edits %v %+v", err, next)
	}
	_, hold, changed, err := ParseCreativeV2Reply(`{"edits":[],"reply":"Holding."}`, next, limits)
	if err != nil || len(changed) != 0 || !reflect.DeepEqual(next, hold) {
		t.Fatal("hold changed score")
	}
	for _, raw := range []string{
		`{"reply":"missing edits"}`,
		`{"edits":[{"layers":[]}],"reply":"wrong contract"}`,
		`{"edits":[{"rebounds":null}],"reply":"null"}`,
		`{"edits":[{"focus":{"position_percent":0,"width_percent":99,"mix_percent":40}}],"reply":"too wide"}`,
		`{"edits":[{"focus":{"position_percent":0}}],"reply":"partial group"}`,
		`{"edits":[{"rebounds":{"count":5,"retained_width_percent":75}}],"reply":"too many"}`,
		`{"edits":[{"inertia_percent":1.5}],"reply":"fraction"}`,
		`{"edits":[{"sweep":{"faster_direction":"up","contrast_percent":40}}],"reply":"bad enum"}`,
		`{"edits":[{"inertia_percent":20},{"inertia_percent":40}],"reply":"duplicate"}`,
		`{"edits":[{"inertia_percent":20,"variation_percent":40}],"reply":"two groups in one item"}`,
		`{"edits":[{"seed":25}],"reply":"model seed"}`,
	} {
		_, rejected, _, err := ParseCreativeV2Reply(raw, s, limits)
		if err == nil || !reflect.DeepEqual(s, rejected) {
			t.Fatalf("invalid transaction %s: %v", raw, err)
		}
	}
}

func TestCreativeV2RejectsExplicitCoverageMismatch(t *testing.T) {
	for _, tc := range []struct {
		message    string
		focus, mix int
		reject     bool
	}{
		{"Mix full strokes with shrinking rebounds at the base.", 100, 55, true},
		{"Mix full strokes with shrinking rebounds at the base.", 0, 100, true},
		{"Mix full strokes with shrinking rebounds at the base.", 0, 55, false},
		{"Add tip rebounds.", 0, 55, true},
		{"Add tip rebounds.", 100, 55, false},
		{"What are base rebounds?", 100, 55, false},
		{"Do not move. Explain base rebounds.", 100, 55, false},
	} {
		s := FreshCreativeV2Score(25)
		s.Gesture.FocusPercent, s.Gesture.FocusMixPercent = tc.focus, tc.mix
		if err := creativeV2RequestedCoverage(tc.message, s); (err != nil) != tc.reject {
			t.Fatalf("%s: %v", tc.message, err)
		}
	}
}

func TestCreativeV2AuthorityAndContractIsolation(t *testing.T) {
	for _, tc := range []struct {
		message, raw                     string
		running, paused, reject, applied bool
	}{
		{"Start moving with shrinking base rebounds.", `{"edits":[{"focus":{"position_percent":0,"width_percent":45,"mix_percent":55}},{"rebounds":{"count":2,"retained_width_percent":75}}],"reply":"Starting."}`, false, false, false, true},
		{"Keep varying the motion.", `{"edits":[{"evolve":true}],"reply":"Fresh details."}`, true, false, false, true},
		{"What does inertia do?", `{"edits":[],"reply":"It shapes travel."}`, true, false, false, false},
		{"What does inertia do?", `{"edits":[{"inertia_percent":70}],"reply":"Changed."}`, true, false, true, false},
		{"Vary the motion.", `{"edits":[{"evolve":true}],"reply":"Changed."}`, true, true, true, false},
		{"Do not move.", `{"edits":[{"evolve":true}],"reply":"Changed."}`, false, false, true, false},
	} {
		provider := &layeredTestProvider{raw: tc.raw}
		s := FreshCreativeV2Score(25)
		service := Service{Provider: provider, Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}, MotionContext: &MotionContext{Running: tc.running, Paused: tc.paused, Layered: &s}}
		result, err := service.Complete(t.Context(), Request{Message: tc.message}, nil)
		if (err != nil) != tc.reject || (result.Response.Motion != nil) != tc.applied || provider.calls != 1 || result.Repaired || result.SemanticFallback {
			t.Fatalf("%s: %+v %v", tc.message, result, err)
		}
		system := provider.request.Messages[0].Content
		if !strings.Contains(system, "retained_width_percent") || strings.Contains(system, "span_profile") || strings.Contains(system, `"alternate_ends"`) {
			t.Fatal("mixed model contracts")
		}
	}
}
