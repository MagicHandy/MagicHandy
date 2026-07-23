package chat

import (
	"testing"
)

func TestParseSceneDirectorResponse(t *testing.T) {
	raw := `{"action":"ride","intensity":8,"stroke_range":[0.1,0.9],"dialogue":"Keep going."}`
	director, err := ParseSceneDirectorResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if director.Action != SceneActionRide || director.Intensity != 8 {
		t.Fatalf("unexpected director: %+v", director)
	}
}

func TestSceneDirectorToMotionCommandStart(t *testing.T) {
	director := SceneDirectorResponse{
		Action:      SceneActionRide,
		Intensity:   8,
		StrokeRange: []float64{0.1, 0.9},
		Dialogue:    "Faster now.",
	}
	cmd := director.ToMotionCommand(false)
	if cmd == nil || cmd.Action != MotionActionStart {
		t.Fatalf("command = %+v, want start", cmd)
	}
	if len(cmd.StrokeRange) != 2 || cmd.Velocidade < 70 {
		t.Fatalf("physics mapping = %+v", cmd)
	}
}

func TestSceneDirectorToMotionCommandTargetWhenRunning(t *testing.T) {
	director := SceneDirectorResponse{
		Action:      SceneActionRide,
		Intensity:   5,
		StrokeRange: []float64{0.3, 0.85},
		Dialogue:    "Shift rhythm.",
	}
	cmd := director.ToMotionCommand(true)
	if cmd == nil || cmd.Action != MotionActionTarget {
		t.Fatalf("command = %+v, want target", cmd)
	}
}

func TestStrokeRangePercents(t *testing.T) {
	low, high := StrokeRangePercents([]float64{0.1, 0.9})
	if low != 10 || high != 90 {
		t.Fatalf("range = %d..%d", low, high)
	}
}

func TestParseAssistantResponseAcceptsSceneDirector(t *testing.T) {
	raw := `{"action":"pause","intensity":3,"stroke_range":[0.2,0.6],"dialogue":"Breathe with me."}`
	response, err := ParseAssistantResponseForMode(raw, "procedural")
	if err != nil {
		t.Fatal(err)
	}
	if response.Reply != "Breathe with me." {
		t.Fatalf("reply = %q", response.Reply)
	}
	if response.Motion == nil || response.Motion.TipoBatida != "lento" {
		t.Fatalf("motion = %+v", response.Motion)
	}
}
