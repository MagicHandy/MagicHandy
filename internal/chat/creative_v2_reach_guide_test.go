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
	if LLMLabPromptsFor(false)["creative_v2"] != lab {
		t.Fatal("schema-enforced Lab prompt disagrees with production")
	}
	if baseline := WithoutCreativeV2ReachGuide(lab); strings.Contains(baseline, creativeV2ReachGuide) || !strings.HasSuffix(lab, baseline) {
		t.Fatal("evaluation baseline did not remove exactly the reach guide")
	}
}

// Without the response schema, the guide made local Gemma 12B write malformed
// edits, so a provider that may drop the schema keeps the earlier contract.
func TestCreativeV2ContractOmitsReachGuideWithoutSchema(t *testing.T) {
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2, SchemaUnenforced: true}
	if got := contractInstructions(capabilities); got != creativeV2Contract {
		t.Fatal("unenforced-schema Creative v2 contract carried the reach guide or changed")
	}
	lab := LLMLabPromptsFor(true)
	if lab["creative_v2"] != WithoutCreativeV2ReachGuide(LLMLabPrompts()["creative_v2"]) || strings.Contains(lab["creative_v2"], creativeV2ReachGuide) {
		t.Fatal("unenforced-schema Lab prompt disagrees with production")
	}
	if lab["layered"] != LLMLabPrompts()["layered"] {
		t.Fatal("only the Creative v2 Lab prompt depends on schema enforcement")
	}
}
