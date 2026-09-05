package chat

import (
	"fmt"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLibraryLabNamingSelectsTheSameMotionAndPreservesPace(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	current := motion.DefaultFlowSpec()
	for _, method := range []string{"library", "library_descriptive", "library_actions"} {
		id := libraryLabHandle(method, "flow-tip-anchored")
		_, next, _, err := ParseLLMLab(fmt.Sprintf(`{"reply":"Variable reach returning to the tip.","recipe_id":%q}`, id), method, current, limits)
		if err != nil || next.AnchorPercent != 100 || next.SpeedPercent != 25 || next.RangeFloorPercent != 25 {
			t.Fatalf("%s selection: %+v %v", method, next, err)
		}
		_, faster, changed, err := ParseLLMLab(`{"reply":"Only pace changes.","speed_percent":35}`, method, next, limits)
		if err != nil || faster.AnchorPercent != 100 || faster.SpeedPercent != 35 || len(changed) != 1 || changed[0] != "speed_percent" {
			t.Fatalf("%s pace-only changed shape: %+v %v", method, faster, err)
		}
		if _, _, _, err := ParseLLMLab(`{"reply":"Unknown","recipe_id":"does_not_exist"}`, method, next, limits); err == nil {
			t.Fatal("unknown recipe accepted")
		}
	}
}

func TestSectionPaceEditsChangeActualSectionSpeeds(t *testing.T) {
	limits := config.DefaultSettings().Motion
	recipe, _ := motion.ContinuousRecipeByID("flow-wide-narrow", 25)
	_, next, _, err := ParseLLMLab(`{"reply":"Pace 35.","controls":{"speed_percent":35}}`, "controls", recipe.Spec, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range next.Steps {
		if step.SpeedPercent != 35 {
			t.Fatal("accepted pace edit was inert")
		}
	}
	if _, _, _, err := ParseLLMLab(`{"reply":"Range","controls":{"max_percent":90}}`, "controls", recipe.Spec, limits); err == nil {
		t.Fatal("accepted ineffective global section range")
	}
}

func TestNamedContinuousStartRetainsAuthorizationBoundaries(t *testing.T) {
	command := MotionCommand{Action: MotionActionStart, PatternID: string(motion.PatternPaceWave)}
	for _, test := range []struct {
		message string
		want    bool
	}{
		{"start a pace wave at 30 percent", true},
		{"do not start a pace wave", false},
		{"is it safe to start a pace wave?", false},
		{"tell me about starting a pace wave", false},
		{"I like the pace wave", false},
	} {
		if got := authorizesNamedContinuousStart(test.message, command, FullCapabilities(), nil); got != test.want {
			t.Fatalf("%q authorization=%v", test.message, got)
		}
	}
}

func TestCompoundPaceRequestPreservesRunningPatternAuthority(t *testing.T) {
	speed := 30
	state := &MotionContext{Running: true, PatternID: string(motion.PatternFullSweeps), SpeedPercent: 25, Area: AreaZoneFull}
	command := MotionCommand{Action: MotionActionTarget, SpeedPercent: &speed}
	message := "Keep that exact movement shape and change only speed to 30 percent."
	if !authorizesPreservedPatternPace(message, command, FullCapabilities(), state) {
		t.Fatal("explicit matching pace-only edit rejected")
	}
	for _, text := range []string{"Keep moving but do not change only speed to 30", "Keep this question in mind: is it safe to change only speed to 30?", "Keep the story about how to change only speed to 30", "Keep the shape and change only speed to 35"} {
		if authorizesPreservedPatternPace(text, command, FullCapabilities(), state) {
			t.Fatalf("nonmatching or unauthorized request accepted: %q", text)
		}
	}
	for _, invalid := range []MotionCommand{{Action: MotionActionStart, SpeedPercent: &speed}, {Action: MotionActionTarget, SpeedPercent: &speed, PatternID: "flow-upper-strokes"}, {Action: MotionActionTarget, SpeedPercent: &speed, Area: AreaZoneTip}} {
		if authorizesPreservedPatternPace(message, invalid, FullCapabilities(), state) {
			t.Fatal("pace-only request authorized another axis or start")
		}
	}
	state.Running = false
	if authorizesPreservedPatternPace(message, command, FullCapabilities(), state) {
		t.Fatal("idle pace change authorized")
	}
}
