package motion

import (
	"math/rand"
	"testing"
)

func TestResolveStrokeProfileRidingAsymmetric(t *testing.T) {
	profile := StrokeProfileFromAction("riding")
	if profile.DownstrokeRatio >= profile.UpstrokeRatio {
		t.Fatalf("riding down=%.2f up=%.2f, want faster descent", profile.DownstrokeRatio, profile.UpstrokeRatio)
	}
	if profile.HasBottomBounce {
		t.Fatal("riding should not bounce")
	}
}

func TestResolveStrokeProfileDeepthroatBounce(t *testing.T) {
	profile := StrokeProfileFromAction("deepthroat")
	if !profile.HasBottomBounce {
		t.Fatal("deepthroat should have bottom bounce")
	}
	if profile.DownstrokeRatio >= 0.5 {
		t.Fatalf("deepthroat down=%.2f, want aggressive descent", profile.DownstrokeRatio)
	}
}

func TestGenerateOrganicBlockDeepthroatInjectsBounce(t *testing.T) {
	cfg := OrganicConfig{
		BaseVelocity:     60,
		StrokeMin:        0,
		StrokeMax:        100,
		NoiseWeight:      0.3,
		Intensity:        60,
		Asymmetry:        0.45,
		SampleIntervalMS: 80,
		StrokeProfile: StrokeProfile{
			DownstrokeRatio: 0.30,
			UpstrokeRatio:   0.70,
			HasBottomBounce: true,
		},
	}
	cfg = normalizeOrganicConfig(cfg)
	perlin := NewPerlinNoise(42)
	stream := generateOrganicBlock(cfg, 6_000, false, rand.New(rand.NewSource(3)), perlin, 0, 10, 20, 60, 50, 12, 80, 0.8, 100)
	if len(stream) < 20 {
		t.Fatalf("stream=%d points, want organic block", len(stream))
	}
	bouncePairs := 0
	for i := 2; i < len(stream); i++ {
		a, b, c := stream[i-2].Position, stream[i-1].Position, stream[i].Position
		if a < b && b > c {
			bouncePairs++
		}
	}
	if bouncePairs < 1 {
		t.Fatalf("bounce pairs=%d, want bottom gag micro-keyframes", bouncePairs)
	}
}

func TestGenerateOrganicBlockRidingFasterDescent(t *testing.T) {
	cfg := OrganicConfig{
		BaseVelocity:     70,
		StrokeMin:        20,
		StrokeMax:        90,
		NoiseWeight:      0.25,
		Intensity:        55,
		SampleIntervalMS: 60,
		StrokeProfile:    StrokeProfileFromAction("riding"),
	}
	cfg = normalizeOrganicConfig(cfg)
	perlin := NewPerlinNoise(7)
	rng := rand.New(rand.NewSource(5))
	stream := generateOrganicBlock(cfg, 4_000, false, rng, perlin, 0, 5, 15, 45, 55, 10, 60, 0.7, 70)
	if len(stream) < 12 {
		t.Fatal("expected riding block")
	}
	downMS, upMS := 0, 0
	prev := stream[0].Position
	descending := false
	for i := 1; i < len(stream); i++ {
		cur := stream[i].Position
		if cur < prev {
			descending = true
			downMS += stream[i].TimeDelta
		} else if cur > prev {
			if descending {
				descending = false
			}
			upMS += stream[i].TimeDelta
		}
		prev = cur
	}
	if downMS == 0 || upMS == 0 {
		t.Fatalf("downMS=%d upMS=%d, want both phases", downMS, upMS)
	}
	ratio := float64(downMS) / float64(downMS+upMS)
	if ratio > 0.55 {
		t.Fatalf("down ratio=%.2f, want riding gravity < 0.5", ratio)
	}
}
