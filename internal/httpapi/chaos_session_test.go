package httpapi

import (
	"math/rand"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestBuildChaosSessionForDurationFillsSegment(t *testing.T) {
	physics := motion.ChaoticPhysics{
		Velocidade:  45,
		Intensidade: 35,
		Regiao:      "meio_cabeca",
		TipoBatida:  "leve",
	}
	settings := config.DefaultSettings().Motion
	target := int64(8_000)

	session := buildChaosSessionForDuration(
		physics,
		settings,
		false,
		rand.New(rand.NewSource(7)),
		target,
	)
	if session.DurationMS < int(target) {
		t.Fatalf("session duration = %dms, want at least %dms", session.DurationMS, target)
	}
	if len(session.Points) < 8 {
		t.Fatalf("session points = %d, want full-zone curved strokes", len(session.Points))
	}
}

func TestBuildChaosSessionCabecaFluidoStaysInZone(t *testing.T) {
	physics := motion.ChaoticPhysics{
		Velocidade:  55,
		Intensidade: 45,
		Regiao:      "cabeca",
		TipoBatida:  "fluido",
	}
	settings := config.DefaultSettings().Motion
	session := buildChaosSessionForDurationFromPosition(
		physics,
		settings,
		false,
		rand.New(rand.NewSource(5)),
		4_000,
		48,
	)
	minPos, maxPos := session.Points[0].PositionPercent, session.Points[0].PositionPercent
	for _, point := range session.Points {
		if point.PositionPercent < minPos {
			minPos = point.PositionPercent
		}
		if point.PositionPercent > maxPos {
			maxPos = point.PositionPercent
		}
	}
	if minPos > 75 || maxPos < 90 {
		t.Fatalf("cabeca fluido after meio handoff = %d..%d, want ~70..100", minPos, maxPos)
	}
}

func TestBuildChaosSessionTurboCabecaPositions(t *testing.T) {
	physics := motion.ChaoticPhysics{
		Velocidade:  98,
		Regiao:      "cabeca",
		TipoBatida:  "turbo",
		AtrasoMS:    1,
	}
	settings := config.DefaultSettings().Motion
	session := buildChaosSessionForDurationFromBlend(
		physics,
		settings,
		false,
		rand.New(rand.NewSource(3)),
		2_500,
		motion.MotionBlendState{Position: 15, Velocity: 0},
	)
	if len(session.Points) < 20 {
		t.Fatalf("points = %d, want dense turbo stream", len(session.Points))
	}
	minPos, maxPos := session.Points[0].PositionPercent, session.Points[0].PositionPercent
	for _, point := range session.Points {
		if point.PositionPercent < minPos {
			minPos = point.PositionPercent
		}
		if point.PositionPercent > maxPos {
			maxPos = point.PositionPercent
		}
	}
	if minPos > 75 || maxPos < 90 {
		t.Fatalf("cabeca turbo session positions %d..%d, want ~70..100", minPos, maxPos)
	}
}
