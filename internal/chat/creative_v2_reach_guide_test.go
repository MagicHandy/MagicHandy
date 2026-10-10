package chat

import (
	"strings"
	"testing"
)

// The reach guide raised local steering on the eight-turn fixture, so the
// Creative v2 contract and its Lab prompt both lead with it.
func TestCreativeV2ContractLeadsWithReachGuide(t *testing.T) {
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}
	if got := contractInstructions(capabilities); got != creativeV2ReachGuide+"\n\n"+creativeV2Contract {
		t.Fatal("Creative v2 contract lost the reach guide or changed")
	}
	lab := LLMLabPrompts()["creative_v2"]
	if !strings.HasPrefix(lab, creativeV2ReachGuide) || strings.Count(lab, creativeV2ReachGuide) != 1 {
		t.Fatal("Lab prompt disagrees with production")
	}
	if baseline := WithoutCreativeV2ReachGuide(lab); strings.Contains(baseline, creativeV2ReachGuide) || !strings.HasSuffix(lab, baseline) {
		t.Fatal("evaluation baseline did not remove exactly the reach guide")
	}
}
