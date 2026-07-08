package motion

import (
	"math/rand"
	"testing"
)

func TestResolveAtrasoMSScalesWithVelocidade(t *testing.T) {
	slow := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "fluido", Velocidade: 15})
	fast := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "fluido", Velocidade: 90})
	if slow <= fast {
		t.Fatalf("velocidade scaling: slow=%d fast=%d, want slow > fast", slow, fast)
	}
}

func TestSummarizeMotionTraceCabecaZone(t *testing.T) {
	physics := ChaoticPhysics{
		Regiao:      "cabeca",
		TipoBatida:  "fluido",
		AtrasoMS:    160,
		Velocidade:  50,
		Intensidade: 60,
	}
	waypoints := GenerateStrokeWaypoints(physics, 3_000, false, rand.New(rand.NewSource(9)))
	trace := SummarizeMotionTrace(physics, waypoints, 300, -1)
	if trace.PosMin > trace.ZoneMin+8 || trace.PosMax < trace.ZoneMax-8 {
		t.Fatalf("cabeca trace pos=%d..%d zone=%d..%d", trace.PosMin, trace.PosMax, trace.ZoneMin, trace.ZoneMax)
	}
	if trace.PointsInZonePct < 85 {
		t.Fatalf("points in zone = %.1f%%, want >=85%%", trace.PointsInZonePct)
	}
}

func TestHigherIntensidadeShortensTailDeltas(t *testing.T) {
	positions := []int{70, 80, 90, 100}
	low := positionsToTimedWaypoints(positions, ChaoticPhysics{Intensidade: 20}, 160, false, nil)
	high := positionsToTimedWaypoints(positions, ChaoticPhysics{Intensidade: 95}, 160, false, nil)
	if len(low) < 3 || len(high) < 3 {
		t.Fatal("expected waypoints")
	}
	mid := len(low) / 2
	if high[mid].TimeDelta >= low[mid].TimeDelta {
		t.Fatalf("mid delta low=%d high=%d, want high < low", low[mid].TimeDelta, high[mid].TimeDelta)
	}
}
