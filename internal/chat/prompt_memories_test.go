package chat

import (
	"strings"
	"testing"
)

func TestPromptMemoriesHaveAggregateBudgetAndRelevantSelection(t *testing.T) {
	memories := make([]string, 200)
	for i := range memories {
		memories[i] = strings.Repeat(" hi", 666)
	}
	memories[0] = "The user's favorite color is amber."
	context := &MotionContext{UserRequests: []string{"What is my favorite color?"}}
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	composition := ComposePrompt(set, memories, nil, Capabilities{}, context, nil)
	if composition.MemoryCandidates != 200 || composition.MemoriesIncluded > 5 || composition.MemoriesIncluded < 1 {
		t.Fatalf("counts: %+v", composition)
	}
	if !strings.Contains(composition.Prompt, memories[0]) {
		t.Fatal("relevant older memory dropped")
	}
	if composition.Bytes > 20000 {
		t.Fatalf("memory flood remains: %d bytes", composition.Bytes)
	}
	for _, section := range composition.Sections {
		if section.ID == "memories" && section.Bytes > maxPromptMemoryBytes+len(memoryInstructionForPrompt(set.ID)) {
			t.Fatal("memory budget exceeded")
		}
	}
	if len(memories) != 200 || memories[199] != strings.Repeat(" hi", 666) {
		t.Fatal("saved memories changed")
	}
}
