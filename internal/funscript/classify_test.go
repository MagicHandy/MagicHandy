package funscript

import "testing"

func actions(pairs [][2]float64) []Action {
	out := make([]Action, len(pairs))
	for i, pair := range pairs {
		out[i] = Action{At: int(pair[0]), Pos: pair[1]}
	}
	return out
}

func TestSlowFullStrokeMonotonic(t *testing.T) {
	block := actions([][2]float64{{0, 10}, {4000, 85}})
	features, err := ExtractBlockFeatures(block)
	if err != nil {
		t.Fatalf("ExtractBlockFeatures: %v", err)
	}
	result := ClassifyBlock(features, block)
	if result.Speed != "slow" {
		t.Fatalf("speed = %q, want slow", result.Speed)
	}
	if result.StrokeLength != "full" {
		t.Fatalf("stroke_length = %q, want full", result.StrokeLength)
	}
}

func TestFastShortOscillation(t *testing.T) {
	block := actions([][2]float64{
		{0, 44}, {350, 54}, {700, 44}, {1050, 54}, {1400, 44},
	})
	features, err := ExtractBlockFeatures(block)
	if err != nil {
		t.Fatalf("ExtractBlockFeatures: %v", err)
	}
	result := ClassifyBlock(features, block)
	bpm := ComputeStrokeBPM(block, features.DurationMS)["bpm"].(float64)
	if result.StrokeLength != "micro" && result.StrokeLength != "short" {
		t.Fatalf("stroke_length = %q, want micro or short", result.StrokeLength)
	}
	if result.Speed != PaceFromBPM(bpm) {
		t.Fatalf("speed = %q, want %q from bpm %.1f", result.Speed, PaceFromBPM(bpm), bpm)
	}
}

func TestDenseKeyframesNotAlwaysVeryFast(t *testing.T) {
	block := actions([][2]float64{
		{0, 20}, {500, 55}, {1000, 20}, {1500, 55}, {2000, 20},
	})
	features, err := ExtractBlockFeatures(block)
	if err != nil {
		t.Fatalf("ExtractBlockFeatures: %v", err)
	}
	result := ClassifyBlock(features, block)
	bpm := ComputeStrokeBPM(block, features.DurationMS)["bpm"].(float64)
	if result.Speed != PaceFromBPM(bpm) {
		t.Fatalf("speed = %q, want %q", result.Speed, PaceFromBPM(bpm))
	}
	if result.Speed == "very_fast" {
		t.Fatal("did not expect very_fast")
	}
}

func TestMicroHoldHeavy(t *testing.T) {
	block := actions([][2]float64{{0, 48}, {2000, 48}, {4000, 52}, {6000, 48}})
	features, err := ExtractBlockFeatures(block)
	if err != nil {
		t.Fatalf("ExtractBlockFeatures: %v", err)
	}
	result := ClassifyBlock(features, block)
	if result.StrokeLength != "micro" {
		t.Fatalf("stroke_length = %q, want micro", result.StrokeLength)
	}
	if result.Rhythm != "pause_hold" && result.Rhythm != "steady" {
		t.Fatalf("rhythm = %q, want pause_hold or steady", result.Rhythm)
	}
}
