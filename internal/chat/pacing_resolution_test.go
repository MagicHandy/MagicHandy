package chat

import "testing"

// intensity was the old pattern-specific name for pace. Responses and saved
// history may still contain it, but validation exposes one canonical field.
func TestNormalizePacing(t *testing.T) {
	speed, intensity := 40, 70
	for _, test := range []struct {
		name      string
		motion    MotionCommand
		wantSpeed int
	}{
		{
			name:      "canonical speed wins when both arrive",
			motion:    MotionCommand{PatternID: "stroke", Intensity: &intensity, SpeedPercent: &speed},
			wantSpeed: speed,
		},
		{
			name:      "canonical speed wins without a pattern",
			motion:    MotionCommand{Intensity: &intensity, SpeedPercent: &speed},
			wantSpeed: speed,
		},
		{
			name:      "speed alone is untouched",
			motion:    MotionCommand{SpeedPercent: &speed},
			wantSpeed: speed,
		},
		{
			name:      "legacy intensity is converted",
			motion:    MotionCommand{PatternID: "stroke", Intensity: &intensity},
			wantSpeed: intensity,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := test.motion
			normalizePacing(&command)
			if command.SpeedPercent == nil || *command.SpeedPercent != test.wantSpeed {
				t.Fatalf("speed_percent = %v, want %d", command.SpeedPercent, test.wantSpeed)
			}
			if command.Intensity != nil {
				t.Fatalf("legacy intensity survived normalization: %d", *command.Intensity)
			}
		})
	}
}

// A nil motion is the common "no change" decision and must not panic.
func TestNormalizePacingIgnoresAbsentMotion(_ *testing.T) {
	normalizePacing(nil)
}

// The resolver runs before validation, so an over-specified command now survives
// the parse instead of costing the turn.
func TestOverspecifiedPacingNoLongerFailsTheTurn(t *testing.T) {
	raw := `{"reply":"Harder then.","motion":{"action":"target","pattern_id":"stroke","intensity":70,"speed_percent":40}}`
	response, err := ParseAssistantResponseWithPatterns(raw, defaultPatternChoices())
	if err != nil {
		t.Fatalf("over-specified pacing still fails the turn: %v", err)
	}
	if response.Motion == nil || response.Motion.SpeedPercent == nil || *response.Motion.SpeedPercent != 40 {
		t.Fatalf("expected canonical speed_percent to survive, got %+v", response.Motion)
	}
	if response.Motion.Intensity != nil {
		t.Fatalf("legacy intensity should have been dropped, got %d", *response.Motion.Intensity)
	}
}
