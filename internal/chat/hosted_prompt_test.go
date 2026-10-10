package chat

import (
	"strings"
	"testing"
)

// The reach guide raised hosted and local steering alike, so every Creative v2
// model reads the same contract, in production and in the Lab.
func TestReachGuideLeadsEveryCreativeV2Contract(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		capabilities := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2, PreserveConversationText: hosted}
		if got := contractInstructions(capabilities); got != creativeV2ReachGuide+"\n\n"+creativeV2Contract {
			t.Fatalf("Creative v2 contract for hosted=%t lost the reach guide or changed", hosted)
		}
	}
	lab := LLMLabPrompts()["creative_v2"]
	if !strings.HasPrefix(lab, creativeV2ReachGuide) || strings.Count(lab, creativeV2ReachGuide) != 1 {
		t.Fatal("Lab prompt disagrees with production")
	}
	if baseline := WithoutCreativeV2ReachGuide(lab); strings.Contains(baseline, creativeV2ReachGuide) || !strings.HasSuffix(lab, baseline) {
		t.Fatal("evaluation baseline did not remove exactly the reach guide")
	}
}
