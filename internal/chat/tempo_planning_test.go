package chat

import (
	"strings"
	"testing"
)

func TestTempoPlanningUsesNativeVocabularyAndTheSameTimingFloor(t *testing.T) {
	message := AutopilotMotionMessage(AutopilotContext{TempoOnly: true, Hosted: true,
		MotionMode: MotionModeLayered, AutoTiming: true, AutoTimingFloor: 45,
		SessionTracking: true, SessionSeconds: 150, ArcEnabled: true, ArcPercent: 25, ArcMinutes: 10})
	for _, unwanted := range []string{"full_slider", "speed_percent", "edits", "No continuous score"} {
		if strings.Contains(message, unwanted) {
			t.Fatalf("position instruction %q reached native planning: %s", unwanted, message)
		}
	}
	for _, want := range []string{"2 minutes 30 seconds", "Session buildup: 25%", "45 to 300", "A reconsideration is not a new start time"} {
		if !strings.Contains(message, want) {
			t.Fatalf("missing %q: %s", want, message)
		}
	}
	fields, _ := schemaFields(t, tempoAutopilotSchema(tempoAdvanced, nil, true, 45))
	field := fields[nextCheckField].(map[string]any)
	if field["minimum"] != float64(45) || field["maximum"] != float64(300) {
		t.Fatalf("timing range: %v", field)
	}
}

func TestTempoContextCarriesEachRequestAge(t *testing.T) {
	state := MotionContext{UserRequests: []string{"Build over the next two minutes", "That feels good"}, UserRequestSecondsAgo: []int{90, 10}}
	text := tempoStateSection(&state)
	for _, want := range []string{`"seconds_ago":90`, `"seconds_ago":10`, "later conversation does not restart"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
}

func TestSessionSpanDoesNotRoundAwayATimedDeadline(t *testing.T) {
	if got := formatSessionSpan(150); got != "2 minutes 30 seconds" {
		t.Fatal(got)
	}
	if got := formatSessionSpan(179); got != "2 minutes 59 seconds" {
		t.Fatal(got)
	}
}
