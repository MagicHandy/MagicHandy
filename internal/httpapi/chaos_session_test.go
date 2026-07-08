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
