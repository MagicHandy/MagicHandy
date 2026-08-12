package chat

import "testing"

// The contract asks for exactly one pacing representation. Rejecting the pair
// outright was costing the whole decision: measured against the live 26B, 60% of
// autopilot motion decisions died on this rule, every one of them fell back to
// the deterministic planner, and that is what flattened session speed.
func TestResolveOverspecifiedPacing(t *testing.T) {
	speed, intensity := 40, 70
	for _, test := range []struct {
		name          string
		motion        MotionCommand
		wantSpeed     *int
		wantIntensity *int
	}{
		{
			name:          "a named pattern is paced by intensity",
			motion:        MotionCommand{PatternID: "stroke", Intensity: &intensity, SpeedPercent: &speed},
			wantIntensity: &intensity,
		},
		{
			name:      "intensity without a pattern is meaningless so speed wins",
			motion:    MotionCommand{Intensity: &intensity, SpeedPercent: &speed},
			wantSpeed: &speed,
		},
		{
			name:      "speed alone is untouched",
			motion:    MotionCommand{SpeedPercent: &speed},
			wantSpeed: &speed,
		},
		{
			name:          "pattern with intensity alone is untouched",
			motion:        MotionCommand{PatternID: "stroke", Intensity: &intensity},
			wantIntensity: &intensity,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := test.motion
			resolveOverspecifiedPacing(&command)
			if (command.SpeedPercent == nil) != (test.wantSpeed == nil) {
				t.Fatalf("speed_percent = %v, want %v", command.SpeedPercent, test.wantSpeed)
			}
			if (command.Intensity == nil) != (test.wantIntensity == nil) {
				t.Fatalf("intensity = %v, want %v", command.Intensity, test.wantIntensity)
			}
			// Whichever field survives must carry the model's own number: the
			// resolver chooses between two supplied values and never invents one.
			if command.SpeedPercent != nil && *command.SpeedPercent != speed {
				t.Errorf("speed_percent was rewritten to %d", *command.SpeedPercent)
			}
			if command.Intensity != nil && *command.Intensity != intensity {
				t.Errorf("intensity was rewritten to %d", *command.Intensity)
			}
		})
	}
}

// A nil motion is the common "no change" decision and must not panic.
func TestResolveOverspecifiedPacingIgnoresAbsentMotion(_ *testing.T) {
	resolveOverspecifiedPacing(nil)
}

// The resolver runs before validation, so an over-specified command now survives
// the parse instead of costing the turn.
func TestOverspecifiedPacingNoLongerFailsTheTurn(t *testing.T) {
	raw := `{"reply":"Harder then.","motion":{"action":"target","pattern_id":"stroke","intensity":70,"speed_percent":40}}`
	response, err := ParseAssistantResponseWithPatterns(raw, defaultPatternChoices())
	if err != nil {
		t.Fatalf("over-specified pacing still fails the turn: %v", err)
	}
	if response.Motion == nil || response.Motion.Intensity == nil || *response.Motion.Intensity != 70 {
		t.Fatalf("expected the curated intensity to survive, got %+v", response.Motion)
	}
	if response.Motion.SpeedPercent != nil {
		t.Fatalf("redundant speed_percent should have been dropped, got %d", *response.Motion.SpeedPercent)
	}
}
