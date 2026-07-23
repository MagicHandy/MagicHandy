package motion

import (
	"math/rand"
	"testing"
)

func TestChaosRegionRanges(t *testing.T) {
	cases := map[string]regionRange{
		"cabeca":      {Min: 70, Max: 100},
		"meio":        {Min: 30, Max: 69},
		"base":        {Min: 0, Max: 29},
		"full":        {Min: 0, Max: 100},
		"meio_cabeca": {Min: 30, Max: 100},
		"meio_base":   {Min: 0, Max: 69},
	}
	for name, want := range cases {
		got, ok := chaosRegionRange(name)
		if !ok || got != want {
			t.Fatalf("%s = %+v ok=%v, want %+v", name, got, ok, want)
		}
	}
}

func TestEffectiveSemanticRegionPrefersRegiaoOverDeviceWindow(t *testing.T) {
	got := effectiveSemanticRegion(ChaoticPhysics{
		Regiao:         "cabeca",
		StrokeRangeMin: 0,
		StrokeRangeMax: 1,
	})
	if got.Min != 70 || got.Max != 100 {
		t.Fatalf("cabeca with device window = %+v, want 70..100", got)
	}
}

func TestEffectiveSemanticRegionFallsBackWhenSceneRangeConflicts(t *testing.T) {
	got := effectiveSemanticRegion(ChaoticPhysics{
		Regiao:         "cabeca",
		StrokeRangeMin: 0.1,
		StrokeRangeMax: 0.4,
	})
	if got.Min != 70 || got.Max != 100 {
		t.Fatalf("conflicting scene range = %+v, want cabeca 70..100", got)
	}
}

func TestGenerateStrokeWaypointsUsesFullZoneTravel(t *testing.T) {
	physics := ChaoticPhysics{
		Velocidade:  45,
		Intensidade: 40,
		Regiao:      "cabeca",
		TipoBatida:  "fluido",
		AtrasoMS:    160,
	}
	stream := GenerateStrokeWaypoints(physics, 5_000, false, rand.New(rand.NewSource(3)))
	if len(stream) < 6 {
		t.Fatalf("stream points = %d, want curved full-zone stroke", len(stream))
	}

	minPos, maxPos := stream[0].Position, stream[0].Position
	for _, wp := range stream {
		if wp.Position < minPos {
			minPos = wp.Position
		}
		if wp.Position > maxPos {
			maxPos = wp.Position
		}
	}
	if minPos > 75 || maxPos < 95 {
		t.Fatalf("cabeca stroke range = %d..%d, want near 70..100", minPos, maxPos)
	}
}

func TestGenerateStrokeWaypointsAvoidMicroSteps(t *testing.T) {
	physics := ChaoticPhysics{
		Regiao:     "meio",
		TipoBatida: "fluido",
		AtrasoMS:   160,
	}
	stream := GenerateStrokeWaypoints(physics, 2_500, false, rand.New(rand.NewSource(5)))
	for i := 1; i < len(stream); i++ {
		step := absIntValue(stream[i].Position - stream[i-1].Position)
		if step > 0 && step < 3 {
			t.Fatalf("micro step at %d: %d -> %d", i, stream[i-1].Position, stream[i].Position)
		}
	}
}

func TestResolveAtrasoMSDefaultsAndOverride(t *testing.T) {
	if got := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "fluido", Velocidade: 50}); got < 165 || got > 175 {
		t.Fatalf("fluido default = %d, want ~168 scaled at vel=50", got)
	}
	if got := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "vibrate"}); got != 1 {
		t.Fatalf("vibrate default = %d, want 1", got)
	}
	if got := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "fluido", AtrasoMS: 90, Velocidade: 50}); got >= 165 {
		t.Fatalf("override = %d, want explicit atraso below default fluido", got)
	}
}

func TestGenerateProceduralStreamCoversDuration(t *testing.T) {
	physics := ChaoticPhysics{
		Regiao:     "meio_cabeca",
		TipoBatida: "fluido",
		AtrasoMS:   160,
	}
	target := int64(8_000)
	stream := GenerateProceduralStream(physics, target, false, rand.New(rand.NewSource(11)))
	if ChaosWaypointsDurationMS(stream, proceduralStreamLeadMillis) < int(target) {
		t.Fatalf("duration = %dms, want >= %dms", ChaosWaypointsDurationMS(stream, proceduralStreamLeadMillis), target)
	}
}

func TestGenerateStrokeWaypointsContinuesFromPosition(t *testing.T) {
	physics := ChaoticPhysics{
		Regiao:     "cabeca",
		TipoBatida: "fluido",
		AtrasoMS:   160,
	}
	stream := GenerateStrokeWaypointsFromPosition(physics, 2_000, false, rand.New(rand.NewSource(7)), 85)
	if len(stream) == 0 {
		t.Fatal("expected stream")
	}
	if stream[0].Position != 85 {
		t.Fatalf("first position = %d, want continue from 85", stream[0].Position)
	}
}

func TestGenerateStrokeWaypointsBridgesAcrossZoneChange(t *testing.T) {
	base := ChaoticPhysics{Regiao: "base", TipoBatida: "lento", AtrasoMS: 160, Velocidade: 50}
	stream := GenerateStrokeWaypointsFromPosition(base, 2_000, false, rand.New(rand.NewSource(4)), 70)
	if len(stream) == 0 {
		t.Fatal("expected stream")
	}
	if stream[0].Position != 70 {
		t.Fatalf("first position = %d, want bridge from device position 70", stream[0].Position)
	}
	foundBase := false
	for _, wp := range stream {
		if wp.Position <= 29 {
			foundBase = true
		}
	}
	if !foundBase {
		t.Fatalf("stream never reached base zone")
	}
	maxStep := 0
	for i := 1; i < len(stream); i++ {
		step := absIntValue(stream[i].Position - stream[i-1].Position)
		if step > maxStep {
			maxStep = step
		}
	}
	if maxStep > 20 {
		t.Fatalf("max position step = %d, want smooth bridge into base zone", maxStep)
	}
}
