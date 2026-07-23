package chat

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/motion/semantic"
)

func TestMotionTargetFromCommandProceduralMapsPhysics(t *testing.T) {
	command := &MotionCommand{
		Action:      MotionActionTarget,
		Velocidade:  72,
		Regiao:      "meio_cabeca",
		TipoBatida:  "fluido",
		Intensidade: 65,
	}
	target := MotionTargetFromCommand(command, motion.ActiveMotionState{}, config.MotionGenerationModeProcedural)
	if target.SpeedPercent != 72 {
		t.Fatalf("speed = %d, want 72", target.SpeedPercent)
	}
	if target.AreaFocus == nil {
		t.Fatal("expected area focus from regiao")
	}
	if target.AreaFocus.MinPercent != 30 || target.AreaFocus.MaxPercent != 70 {
		t.Fatalf("area focus = %+v, want 30..70 (shaft zone via semantic)", target.AreaFocus)
	}
	if target.Label != "Chat" || target.Source != "chat" {
		t.Fatalf("target = %+v", target)
	}
}

func TestMotionTargetFromCommandProceduralLegacyFallback(t *testing.T) {
	speed := 40
	command := &MotionCommand{
		Action:         MotionActionStart,
		SpeedPercent:   &speed,
		IntensidadeLegacy: "alta",
		Estilo:         "pulsante",
	}
	target := MotionTargetFromCommand(command, motion.ActiveMotionState{}, config.MotionGenerationModeProcedural)
	if target.SpeedPercent != 40 {
		t.Fatalf("speed = %d, want legacy speed_percent", target.SpeedPercent)
	}
	if target.PatternID != motion.PatternTease {
		t.Fatalf("pattern = %q, want tease from pulsante", target.PatternID)
	}
}

func TestMotionTargetFromCommandProceduralStrokeRangeOverrides(t *testing.T) {
	command := &MotionCommand{
		Action:      MotionActionStart,
		Velocidade:  50,
		Regiao:      "base",
		StrokeRange: []float64{0.1, 0.4},
	}
	target := MotionTargetFromCommand(command, motion.ActiveMotionState{}, config.MotionGenerationModeProcedural)
	if target.AreaFocus == nil || target.AreaFocus.MinPercent != 10 || target.AreaFocus.MaxPercent != 40 {
		t.Fatalf("area focus = %+v, want stroke_range override 10..40", target.AreaFocus)
	}
}

func TestMotionTargetFromCommandUsesCustomMotionPreferences(t *testing.T) {
	prefs := semantic.DefaultMotionPreferences()
	prefs.Zones[semantic.ZoneTip] = semantic.ZoneRange{Min: 0.75, Max: 0.95}
	command := &MotionCommand{
		Action:     MotionActionStart,
		Velocidade: 60,
		Regiao:     "cabeca",
	}
	target := MotionTargetFromCommandWithPreferences(
		command,
		motion.ActiveMotionState{},
		config.MotionGenerationModeProcedural,
		prefs,
	)
	if target.AreaFocus == nil || target.AreaFocus.MinPercent != 75 || target.AreaFocus.MaxPercent != 95 {
		t.Fatalf("area focus = %+v, want custom tip 75..95", target.AreaFocus)
	}
}
