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
	if err != nil || next.Gesture.FocusPercent != 0 || next.Gesture.FocusRoamPercent != 0 || next.Gesture.ReboundCount != 3 || next.SpeedPercent != 25 || s.Gesture.FocusPercent != 50 {
		t.Fatalf("partial edits %v %+v", err, next)
	}
	// A local width wider than the band has one reading: as wide as the band
	// allows. Rejecting it used to discard every other edit in the turn.
	_, clamped, _, err := ParseCreativeV2Reply(`{"edits":[{"range":{"min_percent":30,"max_percent":70}},{"focus":{"position_percent":0,"width_percent":60,"mix_percent":40,"roam_percent":0}},{"speed_percent":30}],"reply":"Narrower."}`, s, limits)
	if err != nil || clamped.Gesture.FocusWidthPercent != 40 || clamped.MinPercent != 30 || clamped.SpeedPercent != 30 {
		t.Fatalf("narrowed band did not clamp the local width: %v %+v", err, clamped)
	}
	_, hold, changed, err := ParseCreativeV2Reply(`{"edits":[],"reply":"Holding."}`, next, limits)
	if err != nil || len(changed) != 0 || !reflect.DeepEqual(next, hold) {
		t.Fatal("hold changed score")
	}
	for _, raw := range []string{
		`{"reply":"missing edits"}`,
		`{"edits":[{"layers":[]}],"reply":"wrong contract"}`,
		`{"edits":[{"rebounds":null}],"reply":"null"}`,
		`{"edits":[{"focus":{"position_percent":0}}],"reply":"partial group"}`,
		`{"edits":[{"rebounds":{"count":5,"retained_width_percent":75}}],"reply":"too many"}`,
		`{"edits":[{"inertia_percent":1.5}],"reply":"fraction"}`,
		`{"edits":[{"sweep":{"faster_direction":"up","contrast_percent":40}}],"reply":"bad enum"}`,
		`{"edits":[{"inertia_percent":20},{"inertia_percent":40}],"reply":"duplicate"}`,
		`{"edits":[{"inertia_percent":20,"variation_percent":40},{"inertia_percent":40}],"reply":"duplicate across items"}`,
		`{"edits":[{"seed":25}],"reply":"model seed"}`,
		`{"edits":{"inertia_percent":20,"seed":25},"reply":"model seed in an object"}`,
		`{"edits":"faster","reply":"not edits"}`,
	} {
		_, rejected, _, err := ParseCreativeV2Reply(raw, s, limits)
		if err == nil || !reflect.DeepEqual(s, rejected) {
			t.Fatalf("invalid transaction %s: %v", raw, err)
		}
	}
}

// A server that ignores the response schema lets the model write several
// controls in one item, or an object of controls. These are local Gemma 12B
// replies written without the schema, the reported "exactly one control group"
// rejection. Items apply together, so each has the same reading as one control
// per item; a group field outside its group still has no single reading.
func TestCreativeV2AcceptsEditsWrittenWithoutTheSchema(t *testing.T) {
	s := FreshCreativeV2Score(40)
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
	for _, tc := range []struct{ unconstrained, canonical string }{
		{`{"action":"update","edits":[{"speed_percent":50,"inertia_percent":40}],"reply":"Faster, with more inertia."}`,
			`{"action":"update","edits":[{"speed_percent":50},{"inertia_percent":40}],"reply":"Faster, with more inertia."}`},
		{`{"action":"update","edits":[{"focus":{"mix_percent":40,"position_percent":95,"roam_percent":0,"width_percent":25},"speed_percent":45}],"reply":"Faster, near the tip."}`,
			`{"action":"update","edits":[{"speed_percent":45},{"focus":{"position_percent":95,"width_percent":25,"mix_percent":40,"roam_percent":0}}],"reply":"Faster, near the tip."}`},
		{`{"action":"update","edits":[{"speed_percent":50,"variation_percent":80},{"rebounds":{"count":2,"retained_width_percent":60}}],"reply":"Faster and freer, with rebounds."}`,
			`{"action":"update","edits":[{"speed_percent":50},{"variation_percent":80},{"rebounds":{"count":2,"retained_width_percent":60}}],"reply":"Faster and freer, with rebounds."}`},
		{`{"action":"update","edits":{"sweep":{"contrast_percent":40,"faster_direction":"tip"},"variation_percent":85},"reply":"Quicker toward the tip."}`,
			`{"action":"update","edits":[{"sweep":{"faster_direction":"tip","contrast_percent":40}},{"variation_percent":85}],"reply":"Quicker toward the tip."}`},
		{`{"action":"update","edits":[{},{"speed_percent":30}],"reply":"Slower."}`,
			`{"action":"update","edits":[{"speed_percent":30}],"reply":"Slower."}`},
		{`{"action":"none","edits":{},"reply":"Holding."}`, `{"action":"none","edits":[],"reply":"Holding."}`},
	} {
		want, wantNext, wantChanged, err := ParseCreativeV2Reply(tc.canonical, s, limits)
		if err != nil {
			t.Fatalf("canonical %s: %v", tc.canonical, err)
		}
		got, next, changed, err := ParseCreativeV2Reply(tc.unconstrained, s, limits)
		if err != nil || !reflect.DeepEqual(next, wantNext) || !reflect.DeepEqual(changed, wantChanged) || got.Reply != want.Reply || got.continuousAction != want.continuousAction {
			t.Fatalf("%s: %v\n got %+v %v\nwant %+v %v", tc.unconstrained, err, next, changed, wantNext, wantChanged)
		}
	}
	_, rejected, _, err := ParseCreativeV2Reply(`{"action":"update","edits":[{"mix_percent":0,"range":{"min_percent":5,"max_percent":95}},{"speed_percent":20}],"reply":"Slower, the whole length."}`, s, limits)
	if err == nil || !strings.Contains(err.Error(), "mix_percent") || !reflect.DeepEqual(s, rejected) {
		t.Fatalf("a focus field outside its group was accepted: %v", err)
	}
}

func TestCreativeV2RoamingIsAnIndependentAtomicControl(t *testing.T) {
	s := FreshCreativeV2Score(25)
	s.Gesture.FocusPercent, s.Gesture.FocusRoamPercent = 100, 0
	_, next, _, err := ParseCreativeV2Reply(`{"edits":[{"focus":{"position_percent":100,"width_percent":25,"mix_percent":40,"roam_percent":100}}],"reply":"The location can roam."}`, s, config.DefaultSettings().Motion)
	if err != nil || next.Gesture.FocusRoamPercent != 100 || s.Gesture.FocusRoamPercent != 0 || next.SpeedPercent != s.SpeedPercent {
		t.Fatalf("roam edit: %+v %v", next, err)
	}
	want := s
	gesture := *s.Gesture
	gesture.FocusRoamPercent = 100
	want.Gesture = &gesture
	if !reflect.DeepEqual(next, want) {
		t.Fatal("roaming replaced unrelated motion controls")
	}
	if creativeV2ScoreContext(s)["focus"].(map[string]any)["roam_percent"] != float64(0) {
		t.Fatal("held focus vanished from model context")
	}
}

func TestCreativeV2AuthorityAndContractIsolation(t *testing.T) {
	for _, tc := range []struct {
		message, raw                     string
		running, paused, reject, applied bool
	}{
		{"Start moving with shrinking base rebounds.", `{"action":"start","edits":[{"focus":{"position_percent":0,"width_percent":45,"mix_percent":55}},{"rebounds":{"count":2,"retained_width_percent":75}}],"reply":"Starting."}`, false, false, false, true},
		{"Keep varying the motion.", `{"action":"update","edits":[{"evolve":true}],"reply":"Fresh details."}`, true, false, false, true},
		{"Increase speed by exactly 5 percentage points.", `{"action":"update","edits":[{"speed_percent":30}],"reply":"Five points faster."}`, true, false, false, true},
		{"Do not increase speed.", `{"action":"none","edits":[{"speed_percent":30}],"reply":"Faster."}`, true, false, true, false},
		{"What happens if we increase speed?", `{"action":"none","edits":[{"speed_percent":30}],"reply":"Faster."}`, true, false, true, false},
		{"Increase speed by exactly 5 percentage points.", `{"action":"update","edits":[{"speed_percent":30}],"reply":"Faster."}`, false, false, true, false},
		{"What does inertia do?", `{"action":"none","edits":[],"reply":"It shapes travel."}`, true, false, false, false},
		{"What does inertia do?", `{"action":"none","edits":[{"inertia_percent":70}],"reply":"Changed."}`, true, false, true, false},
		{"Vary the motion.", `{"action":"update","edits":[{"evolve":true}],"reply":"Changed."}`, true, true, true, false},
		{"Do not move.", `{"action":"none","edits":[{"evolve":true}],"reply":"Changed."}`, false, false, true, false},
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
