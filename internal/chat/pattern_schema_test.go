package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func continuousTestChoices() []PatternChoice {
	choices := []PatternChoice{}
	for _, recipe := range motion.ContinuousRecipes(25) {
		choices = append(choices, PatternChoice{ID: string(recipe.ID), Name: recipe.Name, Description: recipe.Description})
	}
	return choices
}

func TestAutopilotCatalogExampleMatchesItsOwnParserAndSchema(t *testing.T) {
	choices := continuousTestChoices()
	state := &MotionContext{Running: true, PatternID: string(motion.PatternFullSweeps), SpeedPercent: 25, SpeedMinPercent: 10, SpeedMaxPercent: 43}
	for _, kind := range []AutopilotKind{AutopilotKindMotion, AutopilotKindSpeech} {
		catalog := autopilotCatalog(choices, kind)
		parts := strings.Split(catalog, "Complete response example for this autonomous contract: ")
		if len(parts) != 2 {
			t.Fatal("missing complete autonomous example")
		}
		provider := &scriptedProvider{responses: []string{parts[1]}}
		service := AutopilotService{Provider: provider, Patterns: choices, Capabilities: FullCapabilities(), MotionContext: state}
		if _, err := service.Complete(t.Context(), kind, Request{Message: "Choose the next movement inside the saved limits."}); err != nil {
			t.Fatalf("%s catalog example failed its own contract: %v", kind, err)
		}
		if len(provider.requests) != 1 || len(provider.requests[0].JSONSchema) == 0 {
			t.Fatal("schema absent or valid example needed repair")
		}
		prompt := provider.requests[0].Messages[0].Content
		if strings.Contains(prompt, `"action":"start"`) || kind == AutopilotKindMotion && strings.Contains(prompt, `{"reply":`) {
			t.Fatal("interactive response example leaked into autonomous contract")
		}
		var schema map[string]any
		if err := json.Unmarshal(provider.requests[0].JSONSchema, &schema); err != nil {
			t.Fatal(err)
		}
		fields := schema["properties"].(map[string]any)
		if _, ok := fields["reply"]; ok != (kind == AutopilotKindSpeech) {
			t.Fatal("schema reply field disagrees with parser contract")
		}
	}
}

func TestLibrarySchemaIsNotAttachedToCreativeOrDisabledMotion(t *testing.T) {
	for _, capabilities := range []Capabilities{{}, {Motion: true, Patterns: true, MotionMode: MotionModeDynamic}} {
		if len(PatternResponseSchema(continuousTestChoices(), capabilities, nil)) != 0 {
			t.Fatal("library grammar restricted a different contract")
		}
	}
}
