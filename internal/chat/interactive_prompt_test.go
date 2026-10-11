package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestInteractiveInspectorMatchesContinuousProviderRequest(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeCreativeV2, MotionModeLayered} {
		for _, state := range []MotionContext{{}, {Running: true}, {Running: true, Paused: true}, {Running: true, Autopilot: true}} {
			state.MotionMode, state.SpeedMinPercent, state.SpeedMaxPercent = mode, 10, 50
			score := FreshCreativeV2Score(25)
			empty := "[]"
			if mode == MotionModeLayered {
				score, empty = DefaultLayeredScore(25), "{}"
			}
			state.Layered = &score
			provider := &layeredTestProvider{raw: `{"action":"none","stay_unchanged":false,"edits":` + empty + `,"reply":"A question answered."}`}
			capabilities := Capabilities{Motion: true, MotionMode: mode}
			prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
			service := Service{Provider: provider, Prompt: prompt, Capabilities: &capabilities, MotionContext: &state, ReasoningMode: "on"}
			if _, err := service.Complete(t.Context(), Request{Message: "Explain the controls."}, nil); err != nil {
				t.Fatal(err)
			}
			inspected := ComposeInteractivePrompt(prompt, nil, nil, capabilities, &state, nil)
			if inspected.Prompt != provider.request.Messages[0].Content || string(InteractiveResponseSchema(nil, capabilities, &state)) != string(provider.request.JSONSchema) {
				t.Fatalf("%s inspector differs from the actual provider request", mode)
			}
			if provider.calls != 1 || provider.request.ReasoningMode != "on" {
				t.Fatal("inspection refactor changed generation count or reasoning")
			}
			if inspected.Bytes != len(inspected.Prompt) || inspected.Characters != utf8.RuneCountInString(inspected.Prompt) {
				t.Fatal("stale composition counts")
			}
			for _, section := range inspected.Sections {
				if section.Bytes != len(section.Text) || section.Characters != utf8.RuneCountInString(section.Text) {
					t.Fatalf("stale section counts: %s", section.ID)
				}
				if section.ID == "response_contract" {
					if strings.Contains(section.Text, `{"edits"`) {
						t.Fatal("live contract demonstrates an action-less reply")
					}
					checkLiveExamples(t, section.Text, service, state)
				}
			}
		}
	}
}

func checkLiveExamples(t *testing.T, contract string, service Service, state MotionContext) {
	t.Helper()
	for _, line := range strings.Split(contract, "\n") {
		start := strings.Index(line, `{"action":`)
		if start < 0 || strings.Contains(line, "none|start|update") {
			continue
		}
		raw := strings.TrimSpace(line[start:])
		if strings.Contains(raw, "...") {
			continue
		}
		var example json.RawMessage
		if err := json.NewDecoder(strings.NewReader(raw)).Decode(&example); err != nil {
			t.Fatalf("invalid example: %s", raw)
		}
		raw = string(example)
		provider := &layeredTestProvider{raw: raw}
		service.Provider = provider
		if _, err := service.Complete(t.Context(), Request{Message: "Example only."}, nil); err != nil {
			t.Fatalf("example cannot satisfy live validation: %s: %v", raw, err)
		}
		if state.Autopilot && !strings.Contains(raw, `"stay_unchanged":false`) {
			t.Fatal("example omits the required Autopilot wish")
		}
	}
}
