package chat

import (
	"strings"
	"testing"
)

func TestPaceOnlyStrokersAreDescribedTruthfully(t *testing.T) {
	vacuglide := &OutputContext{Strokers: 1, PaceOnly: 1}
	text := outputContextInstructions(vacuglide)
	if !strings.Contains(text, "PACE-ONLY STROKER") || !strings.Contains(text, `"pace_only_strokers":1`) {
		t.Fatalf("instructions = %q, want the pace-only guide and fact", text)
	}
	// A Handy alone composes nothing here, so its prompt stays byte for byte.
	if got := outputContextInstructions(&OutputContext{Strokers: 1}); got != "" {
		t.Fatalf("a Handy-only output context composed %q", got)
	}
	if !paceOnlyOnly(vacuglide) || paceOnlyOnly(&OutputContext{Strokers: 2, PaceOnly: 1}) || paceOnlyOnly(nil) {
		t.Fatal("pace-only playback is only playback whose every stroker follows pace")
	}

	simple := Capabilities{Motion: true, SimpleMotion: true, MotionMode: MotionModeLayered}
	alone := playbackOutputInstructions(simple, &MotionContext{Outputs: vacuglide})
	if !strings.Contains(alone, simplePaceOnlyGuide) {
		t.Fatalf("Simple instructions with a VacuGlide alone = %q", alone)
	}
	withHandy := playbackOutputInstructions(simple, &MotionContext{Outputs: &OutputContext{Strokers: 2, PaceOnly: 1}})
	if strings.Contains(withHandy, simplePaceOnlyGuide) {
		t.Fatal("with The Handy playing, Simple must keep its stroke vocabulary")
	}
}
