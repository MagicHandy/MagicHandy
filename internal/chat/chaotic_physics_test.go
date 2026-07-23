package chat

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestChaoticPhysicsFromCommandMapsPhysicalAction(t *testing.T) {
	command := &MotionCommand{
		Action:         MotionActionStart,
		PhysicalAction: "deepthroat",
		Velocidade:     70,
		Intensidade:    60,
		Regiao:         "cabeca",
		TipoBatida:     "fluido",
		AtrasoMS:       120,
		StrokeRange:    []float64{0.7, 1.0},
	}

	physics := ChaoticPhysicsFromCommand(command)
	if physics.Action != "deepthroat" {
		t.Fatalf("action = %q, want deepthroat", physics.Action)
	}
	if !physics.StrokeProfile.HasBottomBounce {
		t.Fatal("expected deepthroat bottom bounce profile")
	}
	if physics.StrokeRangeMin != 0.7 || physics.StrokeRangeMax != 1.0 {
		t.Fatalf("stroke range = %.2f..%.2f", physics.StrokeRangeMin, physics.StrokeRangeMax)
	}
}

func TestChaoticPhysicsFromCommandRidingAsymmetry(t *testing.T) {
	command := &MotionCommand{
		Action:         MotionActionTarget,
		PhysicalAction: "riding",
		Velocidade:     55,
		Intensidade:    50,
		Regiao:         "meio",
		TipoBatida:     "fluido",
	}
	physics := ChaoticPhysicsFromCommand(command)
	if physics.StrokeProfile.DownstrokeRatio >= physics.StrokeProfile.UpstrokeRatio {
		t.Fatalf("riding profile = down %.2f up %.2f, want down < up",
			physics.StrokeProfile.DownstrokeRatio, physics.StrokeProfile.UpstrokeRatio)
	}
}

func TestChaoticPhysicsFromCommandNilSafe(t *testing.T) {
	physics := ChaoticPhysicsFromCommand(nil)
	if physics != (motion.ChaoticPhysics{}) {
		t.Fatalf("nil command = %+v, want zero value", physics)
	}
}
