package chat

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func schemaFields(t *testing.T, raw json.RawMessage) (map[string]any, []string) {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("schema: %v", err)
	}
	properties, _ := root["properties"].(map[string]any)
	var required []string
	for _, value := range root["required"].([]any) {
		required = append(required, value.(string))
	}
	return properties, required
}

func TestNextCheckSchemaIsRequiredOnlyWhereAdded(t *testing.T) {
	base := CreativeV2ResponseSchema(config.DefaultSettings().Motion, false)
	timed := withNextCheckSchema(base)
	properties, required := schemaFields(t, timed)
	field, _ := properties[nextCheckField].(map[string]any)
	if field["type"] != "integer" || field["minimum"] != float64(8) || field["maximum"] != float64(300) ||
		!slices.Contains(required, nextCheckField) {
		t.Fatalf("timed schema field = %v required = %v", field, required)
	}
	if strings.Contains(string(base), nextCheckField) {
		t.Fatal("the shared schema gained next_check_seconds")
	}
	if again := withNextCheckSchema(timed); strings.Count(string(again), `"next_check_seconds"`) != 2 {
		t.Fatalf("a second pass duplicated the field: %s", again)
	}
}

func TestTakeNextCheckLeavesTheModeContract(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		seconds int
		kept    bool
	}{
		{`{"edits":[],"next_check_seconds":95,"reply":"Holding."}`, 95, false},
		{`{"edits":[],"next_check_seconds":44.6,"reply":"Holding."}`, 45, false},
		{`{"edits":[],"next_check_seconds":"soon","reply":"Holding."}`, 0, false},
		{`{"edits":[],"reply":"Holding."}`, 0, true},
		{`not json`, 0, true},
	} {
		stripped, seconds := takeNextCheck(tc.raw)
		if seconds != tc.seconds || strings.Contains(stripped, nextCheckField) || (stripped == tc.raw) != tc.kept {
			t.Fatalf("takeNextCheck(%s) = %s, %d", tc.raw, stripped, seconds)
		}
	}
}

func TestAutoTimingPatternContractReplacesTheCategory(t *testing.T) {
	capabilities := FullCapabilities()
	plain := autopilotPatternSchema(defaultPatternChoices(), capabilities, nil, AutopilotKindMotion)
	capabilities.AutoTiming = true
	timed := autopilotPatternSchema(defaultPatternChoices(), capabilities, nil, AutopilotKindMotion)
	properties, required := schemaFields(t, timed)
	if properties["next"] != nil || properties[nextCheckField] == nil ||
		!slices.Equal(required, []string{"intent", nextCheckField, "variability"}) {
		t.Fatalf("auto timing schema = %v required %v", properties, required)
	}
	if properties, required := schemaFields(t, plain); properties["next"] == nil || properties[nextCheckField] != nil ||
		!slices.Contains(required, "next") {
		t.Fatalf("plain schema changed: %v required %v", properties, required)
	}
	speech := autopilotPatternSchema(defaultPatternChoices(), capabilities, nil, AutopilotKindSpeech)
	if strings.Contains(string(speech), nextCheckField) {
		t.Fatal("speech gained next_check_seconds")
	}

	service := AutopilotService{Capabilities: capabilities, Patterns: defaultPatternChoices(),
		MotionContext: &MotionContext{Running: true, PatternID: "stroke", SpeedPercent: 30}}
	raw, seconds := takeNextCheck(`{"intent":"let this settle","motion":{"action":"none"},"next_check_seconds":140,"variability":"settled"}`)
	response, err := service.parse(raw, AutopilotKindMotion)
	if err != nil || seconds != 140 || response.Next != AutopilotTimingNormal {
		t.Fatalf("auto timing parse = %+v, %d, %v", response, seconds, err)
	}
	service.Capabilities.AutoTiming = false
	if _, err := service.parse(raw, AutopilotKindMotion); err == nil {
		t.Fatal("the category stopped being required without auto timing")
	}
}

func TestAutoTimingReachesThePatternPrompt(t *testing.T) {
	capabilities := FullCapabilities()
	capabilities.AutoTiming = true
	system := composeAutopilotSystem(PromptSet{}, nil, defaultPatternChoices(), capabilities, nil, nil, AutopilotKindMotion)
	if !strings.Contains(system, `"next_check_seconds":<whole number 8-300>`) || strings.Contains(system, `"next":"soon"|"normal"|"later"`) ||
		!strings.Contains(system, `"next_check_seconds":<seconds>`) {
		t.Fatal("pattern contract did not swap the category for next_check_seconds")
	}
	message := AutopilotMotionMessage(AutopilotContext{AutoTiming: true, MotionChangeLevel: 6, RecentCheckSeconds: []int{45, 40, 45},
		CurrentPatternID: "stroke", CurrentSpeed: 30, SpeedMinPercent: 10, SpeedMaxPercent: 80, MotionMinSeconds: 10, MotionMaxSeconds: 35})
	if !strings.Contains(message, `Timing is yours: also return "next_check_seconds"`) ||
		!strings.Contains(message, "an ordinary stretch lasts about 10-35 seconds. Treat that as the reference") ||
		!strings.Contains(message, "oldest to newest: 45, 40, 45") || strings.Contains(message, "Set next to soon") ||
		strings.Contains(message, "never leaves them unheard") {
		t.Fatalf("pattern message = %s", message)
	}
}

func TestContinuousAutoTimingCarriesTheHorizon(t *testing.T) {
	score := FreshCreativeV2Score(30)
	provider := &layeredTestProvider{raw: `{"edits":[{"evolve":true}],"next_check_seconds":150,"reply":"Letting it build."}`}
	capabilities := Capabilities{Motion: true, MotionMode: MotionModeCreativeV2, AutoTiming: true}
	service := AutopilotService{Provider: provider, Capabilities: capabilities,
		MotionContext: &MotionContext{Running: true, MotionMode: MotionModeCreativeV2, Layered: &score}}
	message := AutopilotMotionMessage(AutopilotContext{AutoTiming: true, MotionMode: MotionModeCreativeV2,
		CurrentSpeed: 30, CurrentFlow: &score, MotionChangeLevel: 4, SpeedMinPercent: 10, SpeedMaxPercent: 80})
	if !strings.Contains(message, "never leaves them unheard") || strings.Contains(message, "Motion change preference:") {
		t.Fatalf("continuous message = %s", message)
	}
	response, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: message})
	if err != nil || response.NextCheckSeconds != 150 || response.Motion == nil {
		t.Fatalf("response = %+v, %v", response, err)
	}
	if properties, required := schemaFields(t, provider.request.JSONSchema); properties[nextCheckField] == nil ||
		!slices.Contains(required, nextCheckField) {
		t.Fatalf("continuous schema lacks a required next_check_seconds: %s", provider.request.JSONSchema)
	}

	// Speech keeps its own cadence even when the motion route has auto timing.
	provider.raw = `{"edits":[],"reply":"Still with you."}`
	if _, err := service.Complete(t.Context(), AutopilotKindSpeech, Request{Message: AutopilotSpeechMessage(AutopilotContext{})}); err != nil {
		t.Fatalf("speech turn: %v", err)
	}
	if strings.Contains(string(provider.request.JSONSchema), nextCheckField) {
		t.Fatal("speech schema gained next_check_seconds")
	}

	// Without auto timing the continuous schema keeps its exact bytes.
	service.Capabilities.AutoTiming = false
	provider.raw = `{"edits":[{"evolve":true}],"reply":"Fresh."}`
	response, err = service.Complete(t.Context(), AutopilotKindMotion, Request{Message: "Continue"})
	if err != nil || response.NextCheckSeconds != 0 || strings.Contains(string(provider.request.JSONSchema), nextCheckField) {
		t.Fatalf("plain continuous turn = %+v, %v", response, err)
	}
}
