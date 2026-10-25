package vibration

import "testing"

func TestStartupOffsetsSeekAndRealExhaustion(t *testing.T) {
	for _, follow := range []string{FollowSpeed, FollowDepth, FollowSteady} {
		for _, start := range []float64{0, 2000} {
			r := NewRenderer(Profile{Follow: follow})
			r.Restart(start)
			timeline := strokes(t, 500, 5000)
			for _, at := range []float64{start - 420, start, start + 80, start + 300} {
				if _, covered := r.Drive(timeline, at); !covered {
					t.Fatalf("%s start=%g at=%g: premature starvation", follow, start, at)
				}
			}
			if drive, covered := r.Drive(timeline, 5001); drive != 0 || covered {
				t.Fatalf("%s held beyond tail: %g %t", follow, drive, covered)
			}
			// A pruned/missing beginning after playback started is not pre-roll.
			timeline.Prune(3000)
			if _, covered := r.Drive(timeline, 2100); covered {
				t.Fatalf("%s treated a missing active interval as pre-roll", follow)
			}
		}
	}
}
