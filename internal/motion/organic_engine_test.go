package motion

import (
	"math"
	"math/rand"
	"testing"
)

func TestPerlinNoiseBounded(t *testing.T) {
	p := NewPerlinNoise(42)
	for x := 0.0; x < 5; x += 0.13 {
		for y := 0.0; y < 5; y += 0.17 {
			n := p.Noise2D(x, y)
			if n < -1.01 || n > 1.01 {
				t.Fatalf("noise out of range: %f", n)
			}
		}
	}
}

func TestGenerateOrganicWaypointsProducesContinuousMotion(t *testing.T) {
	cfg := OrganicConfig{
		BaseVelocity:     50,
		StrokeMin:        30,
		StrokeMax:        100,
		NoiseWeight:      0.35,
		Intensity:        55,
		Asymmetry:        0.5,
		SampleIntervalMS: 80,
	}
	stream := GenerateOrganicWaypoints(cfg, 4_000, false, rand.New(rand.NewSource(3)))
	if len(stream) < 12 {
		t.Fatalf("point count = %d, want continuous organic stream", len(stream))
	}
	minPos, maxPos := stream[0].Position, stream[0].Position
	reversals := 0
	for i, wp := range stream {
		if wp.Position < minPos {
			minPos = wp.Position
		}
		if wp.Position > maxPos {
			maxPos = wp.Position
		}
		if i >= 2 {
			prev := stream[i-1].Position - stream[i-2].Position
			cur := wp.Position - stream[i-1].Position
			if prev != 0 && cur != 0 && (prev > 0) != (cur > 0) {
				reversals++
			}
		}
	}
	if maxPos-minPos < 15 {
		t.Fatalf("stroke span = %d, want organic travel", maxPos-minPos)
	}
	if reversals < 2 {
		t.Fatalf("reversals = %d, want oscillatory perlin wave", reversals)
	}
}

func TestOrganicConfigFromPhysicsMapsRegion(t *testing.T) {
	cfg := OrganicConfigFromPhysics(ChaoticPhysics{
		Regiao:     "cabeca",
		Velocidade: 60,
		Intensidade: 50,
		TipoBatida: "fluido",
	})
	if cfg.StrokeMin != 70 || cfg.StrokeMax != 100 {
		t.Fatalf("cabeca bounds = %.0f..%.0f", cfg.StrokeMin, cfg.StrokeMax)
	}
	if cfg.SampleIntervalMS != ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "fluido", Velocidade: 60}) {
		t.Fatalf("sample interval = %d, want velocity-scaled fluido", cfg.SampleIntervalMS)
	}
}

func TestOrganicConfigFromPhysicsKeepsRegiaoOverDeviceStrokeWindow(t *testing.T) {
	cfg := OrganicConfigFromPhysics(ChaoticPhysics{
		Regiao:         "cabeca",
		Velocidade:     60,
		Intensidade:    50,
		TipoBatida:     "fluido",
		StrokeRangeMin: 0,
		StrokeRangeMax: 1,
	})
	if cfg.StrokeMin != 70 || cfg.StrokeMax != 100 {
		t.Fatalf("cabeca with device window on physics = %.0f..%.0f, want 70..100", cfg.StrokeMin, cfg.StrokeMax)
	}
}

func TestOrganicConfigFromPhysicsIntersectsSceneStrokeRange(t *testing.T) {
	cfg := OrganicConfigFromPhysics(ChaoticPhysics{
		Regiao:         "meio_cabeca",
		Velocidade:     50,
		Intensidade:    50,
		TipoBatida:     "fluido",
		StrokeRangeMin: 0.7,
		StrokeRangeMax: 1.0,
	})
	if cfg.StrokeMin != 70 || cfg.StrokeMax != 100 {
		t.Fatalf("intersected bounds = %.0f..%.0f, want 70..100", cfg.StrokeMin, cfg.StrokeMax)
	}
}

func TestCubicHermiteCrossfadeDuration(t *testing.T) {
	from := MotionBlendState{Position: 40, Velocity: 12}
	to := MotionBlendState{Position: 80, Velocity: -8}
	opts := DefaultCrossfadeOptions()
	opts.HardwareSafetyLock = false
	crossfade := CubicHermiteCrossfade(from, to, opts, rand.New(rand.NewSource(1)))
	if len(crossfade) < 3 || len(crossfade) > 5 {
		t.Fatalf("crossfade points = %d, want 3..5", len(crossfade))
	}
	total := 0
	for _, wp := range crossfade {
		total += wp.TimeDelta
	}
	if total < 450 || total > 1100 {
		t.Fatalf("crossfade duration = %dms, want ~500-1000ms", total)
	}
	last := crossfade[len(crossfade)-1].Position
	if math.Abs(float64(last-80)) > 12 {
		t.Fatalf("last crossfade pos = %d, want near 80", last)
	}
}

func TestStitchWithCrossfadeSmoothsStep(t *testing.T) {
	from := MotionBlendState{Position: 55, Velocity: 20}
	target := []ChaosWaypoint{
		{TimeDelta: 80, Position: 72},
		{TimeDelta: 80, Position: 88},
		{TimeDelta: 80, Position: 70},
	}
	opts := DefaultCrossfadeOptions()
	merged := StitchWithCrossfade(from, target, opts, rand.New(rand.NewSource(9)))
	if len(merged) <= len(target) {
		t.Fatalf("merged len = %d, want crossfade prefix", len(merged))
	}
	maxStep := 0
	for i := 1; i < len(merged) && i < 6; i++ {
		step := absIntValue(merged[i].Position - merged[i-1].Position)
		if step > maxStep {
			maxStep = step
		}
	}
	if maxStep > 22 {
		t.Fatalf("max early step = %d, want smooth hermite blend", maxStep)
	}
}

func TestGenerateProceduralStreamFromBlend(t *testing.T) {
	physics := ChaoticPhysics{
		Regiao:     "meio_cabeca",
		TipoBatida: "fluido",
		Velocidade: 50,
		Intensidade: 45,
	}
	from := MotionBlendState{Position: 62, Velocity: 15}
	stream := GenerateProceduralStreamFromBlend(physics, 5_000, false, rand.New(rand.NewSource(11)), from)
	if len(stream) < 10 {
		t.Fatal("expected blended stream")
	}
	if absIntValue(stream[0].Position-62) > 3 {
		t.Fatalf("first pos = %d, want near blend origin 62", stream[0].Position)
	}
}
