package chat

import (
	"strings"
	"testing"
)

func TestHostedReachGuideDoesNotChangeLocalContract(t *testing.T) {
	local := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}
	if got := contractInstructions(local); got != creativeV2Contract || strings.Contains(got, creativeV2ReachGuide) {
		t.Fatal("hosted planning changed the local contract or token budget")
	}
	local.HostedModel = true
	if got := contractInstructions(local); !strings.HasPrefix(got, creativeV2ReachGuide) || !strings.HasSuffix(got, creativeV2Contract) {
		t.Fatal("hosted guide missing or local contract rewritten")
	}
	if strings.Contains(LLMLabPrompts()["creative_v2"], creativeV2ReachGuide) || !strings.HasPrefix(HostedLLMLabPrompts()["creative_v2"], creativeV2ReachGuide) {
		t.Fatal("Lab prompt variants disagree with production")
	}
}
