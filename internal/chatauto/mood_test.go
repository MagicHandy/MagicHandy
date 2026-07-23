package chatauto

import (
	"testing"
	"time"
)

func TestMoodAtElapsed(t *testing.T) {
	progress, humor := MoodAtElapsed(3*time.Minute+45*time.Second, 10, true)
	if progress < 40 || progress > 48 {
		t.Fatalf("progress = %v, want eased ~44", progress)
	}
	if humor != HumorTesao {
		t.Fatalf("humor = %q, want tesao", humor)
	}

	_, humor = MoodAtElapsed(10*time.Minute, 10, true)
	if humor != HumorDominatrix {
		t.Fatalf("humor = %q, want dominatrix", humor)
	}

	_, humor = MoodAtElapsed(10*time.Minute, 10, false)
	if humor != HumorIntensa {
		t.Fatalf("humor = %q, want intensa when dominatrix disabled", humor)
	}
}

func TestBoostIntensity(t *testing.T) {
	if got := BoostIntensity(3, 80); got != 5 {
		t.Fatalf("boost = %d, want 5", got)
	}
	if got := BoostIntensity(9, 100); got != 10 {
		t.Fatalf("boost = %d, want capped at 10", got)
	}
}
