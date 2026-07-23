package motion

import (
	"math/rand"
	"testing"
)

func TestGenerateTurboWaypointsRapidOscillation(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  92,
		Intensidade: 70,
		Regiao:      "meio",
		TipoBatida:  "vibrate",
		AtrasoMS:    1,
	}
	stream := GenerateTurboWaypointsForDuration(physics, 2_500, false, rand.New(rand.NewSource(11)), -1)
	if len(stream) < 20 {
		t.Fatalf("stream points = %d, want hardware vibrate timeline", len(stream))
	}

	minPos, maxPos := TurboWaypointSpan(stream)
	span := maxPos - minPos
	if span < turboVibrateRangeMin {
		t.Fatalf("vibrate span = %d, want >=%dpt travel window", span, turboVibrateRangeMin)
	}

	reversals := 0
	for i := 2; i < len(stream); i++ {
		prev := stream[i-1].Position - stream[i-2].Position
		cur := stream[i].Position - stream[i-1].Position
		if prev != 0 && cur != 0 && (prev > 0) != (cur > 0) {
			reversals++
		}
	}
	if reversals < 10 {
		t.Fatalf("reversals = %d, want rapid low/high alternation", reversals)
	}

	for i := 1; i < len(stream); i++ {
		if stream[i].TimeDelta < turboVibrateHalfCycleMinMS {
			t.Fatalf("delta[%d]=%dms, want hardware half-cycle >=%dms", i, stream[i].TimeDelta, turboVibrateHalfCycleMinMS)
		}
		if stream[i].Position != minPos && stream[i].Position != maxPos {
			t.Fatalf("pos[%d]=%d, want only endpoints %d or %d", i, stream[i].Position, minPos, maxPos)
		}
	}
}

func TestGenerateTurboShortRangeWindow(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  95,
		Intensidade: 80,
		Regiao:      "cabeca",
		TipoBatida:  "turbo",
		AtrasoMS:    1,
	}
	stream := GenerateTurboWaypointsForDuration(physics, 1_200, false, rand.New(rand.NewSource(5)), 85)
	minPos, maxPos := TurboWaypointSpan(stream)
	span := maxPos - minPos
	if span < turboVibrateRangeMin {
		t.Fatalf("turbo span = %d, want >=%dpt buzz window in cabeca", span, turboVibrateRangeMin)
	}
	if minPos < 70 || maxPos > 100 {
		t.Fatalf("turbo window %d..%d outside cabeca 70..100", minPos, maxPos)
	}
}

func TestGenerateTurboVibrateCentersOnZone(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  90,
		Intensidade: 60,
		Regiao:      "base",
		TipoBatida:  "vibrate",
	}
	stream := GenerateTurboWaypointsForDuration(physics, 800, false, rand.New(rand.NewSource(2)), -1)
	minPos, maxPos := TurboWaypointSpan(stream)
	if minPos != 0 || maxPos != 29 {
		t.Fatalf("base vibrate window %d..%d, want full base 0..29", minPos, maxPos)
	}
}

func TestVibrateMeioUsesFullZoneSpan(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  90,
		Regiao:      "meio",
		TipoBatida:  "vibrate",
	}
	stream := GenerateTurboWaypointsForDuration(physics, 1_200, false, rand.New(rand.NewSource(4)), -1)
	minPos, maxPos := TurboWaypointSpan(stream)
	if minPos != 30 || maxPos != 69 {
		t.Fatalf("meio vibrate %d..%d, want full zone 30..69", minPos, maxPos)
	}
	if maxPos-minPos < turboVibrateRangeMin {
		t.Fatalf("span=%d, want >=%d", maxPos-minPos, turboVibrateRangeMin)
	}
}

func TestGenerateTurboVeryFastFullZoneSweep(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  95,
		Regiao:      "full",
		TipoBatida:  "very_fast",
		AtrasoMS:    1,
	}
	stream := GenerateTurboWaypointsForDuration(physics, 1_500, false, rand.New(rand.NewSource(3)), -1)
	minPos, maxPos := TurboWaypointSpan(stream)
	if maxPos-minPos < 50 {
		t.Fatalf("span=%d, want very_fast full-zone sweep >=50", maxPos-minPos)
	}
	for i := 1; i < len(stream); i++ {
		if stream[i].TimeDelta < veryFastStepMinMS {
			t.Fatalf("delta[%d]=%dms, want very_fast step >=%dms", i, stream[i].TimeDelta, veryFastStepMinMS)
		}
	}
}

func TestGenerateTurboCabecaIgnoresDeviceStrokeWindowOnPhysics(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:     95,
		Regiao:         "cabeca",
		TipoBatida:     "very_fast",
		StrokeRangeMin: 0.3,
		StrokeRangeMax: 0.69,
	}
	stream := GenerateTurboWaypointsForDuration(physics, 1_200, false, rand.New(rand.NewSource(9)), -1)
	minPos, maxPos := TurboWaypointSpan(stream)
	if minPos < 70 || maxPos > 100 {
		t.Fatalf("very_fast cabeca with mid stroke window on physics = %d..%d, want 70..100", minPos, maxPos)
	}
}

func TestTurboBuzzShorterThanVeryFast(t *testing.T) {
	region := regionRange{Min: 30, Max: 69}
	vf := generateVeryFastSweep(region, ChaoticPhysics{Velocidade: 90, TipoBatida: "very_fast"}, 800, rand.New(rand.NewSource(1)))
	turbo := generateTurboHardwareVibrate(40, 58, turboVibrateHalfCycleMS(90, "turbo"), 800)
	vfMin, vfMax := TurboWaypointSpan(vf)
	tbMin, tbMax := TurboWaypointSpan(turbo)
	if vfMax-vfMin <= tbMax-tbMin {
		t.Fatalf("very_fast span=%d should exceed turbo buzz span=%d", vfMax-vfMin, tbMax-tbMin)
	}
}

func TestTurboVibrateHalfCycleScalesWithVelocity(t *testing.T) {
	slow := turboVibrateHalfCycleMS(30, "vibrate")
	fast := turboVibrateHalfCycleMS(95, "vibrate")
	if fast >= slow {
		t.Fatalf("fast half=%d slow half=%d, want faster velocity → shorter cycle", fast, slow)
	}
}

func TestGenerateProceduralStreamFromBlendSkipsCrossfadeForTurbo(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  90,
		Regiao:      "meio",
		TipoBatida:  "vibrate",
	}
	from := MotionBlendState{Position: 62, Velocity: 0}
	stream := GenerateProceduralStreamFromBlend(physics, 600, false, rand.New(rand.NewSource(1)), from)
	minPos, maxPos := TurboWaypointSpan(stream)
	if maxPos-minPos < turboVibrateRangeMin {
		t.Fatalf("blend span=%d, want turbo zigzag preserved", maxPos-minPos)
	}
}

func TestGenerateTurboWaypointsBypassesHardwareSafetyLock(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  90,
		Regiao:      "meio",
		TipoBatida:  "vibrate",
		AtrasoMS:    1,
	}
	stream := GenerateTurboWaypointsForDuration(physics, 500, true, rand.New(rand.NewSource(17)), -1)
	if len(stream) < 4 {
		t.Fatalf("stream points = %d, want vibrate output", len(stream))
	}
	minPos, maxPos := TurboWaypointSpan(stream)
	if maxPos-minPos < turboVibrateRangeMin {
		t.Fatalf("span=%d with safety lock on, want >=%d", maxPos-minPos, turboVibrateRangeMin)
	}
}
