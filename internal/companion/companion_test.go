package companion

import (
	"math"
	"testing"
	"time"
)

func TestTimelineReplacesTheReplannedTail(t *testing.T) {
	timeline := &Timeline{}
	_ = timeline.Append([]Point{{0, 0}, {100, 50}, {200, 100}})
	_ = timeline.Append([]Point{{150, 20}, {300, 40}})
	if position, _ := timeline.At(200); position >= 50 {
		t.Fatalf("At(200) = %.1f; the old tail should be replaced", position)
	}
	timeline.Prune(160)
	if timeline.Len() != 2 {
		t.Fatalf("Prune kept %d points, want only the segment spanning the cut", timeline.Len())
	}
	if _, ok := timeline.At(160); !ok {
		t.Fatal("pruning must keep the segment that spans the cut")
	}
	if _, ok := timeline.At(140); ok {
		t.Fatal("pruned time must no longer be covered")
	}
}

func TestTimelineAppendIsAtomicAndFinite(t *testing.T) {
	timeline := &Timeline{}
	_ = timeline.Append([]Point{{0, 10}, {100, 20}})
	for _, bad := range [][]Point{
		{{200, 30}, {300, 101}},
		{{200, math.NaN()}},
		{{200, 30}, {-1, 0}},
	} {
		if err := timeline.Append(bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
		if end, _ := timeline.End(); end != 100 || timeline.Len() != 2 {
			t.Fatalf("a rejected append changed the timeline: end %d len %d", end, timeline.Len())
		}
	}
	if _, ok := timeline.At(math.NaN()); ok {
		t.Fatal("NaN time must not be covered")
	}
	if !timeline.Covers(0, 100) || timeline.Covers(50, 101) {
		t.Fatal("Covers must match the retained interval")
	}
}

func TestClockMapsStreamTime(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	var clock Clock
	if clock.Playing() {
		t.Fatal("zero clock must be stopped")
	}
	clock.Start(now, 500, 120*time.Millisecond)
	if got := clock.StreamMillis(now.Add(120 * time.Millisecond)); got != 500 {
		t.Fatalf("stream time at the preroll = %.1f, want 500", got)
	}
	clock.StartAt(now)
	if clock.Anchor() != now || clock.StreamMillis(now.Add(time.Second)) != 1000 {
		t.Fatal("aligned start must use the given anchor")
	}
	clock.Stop()
	if clock.Playing() {
		t.Fatal("Stop must clear the anchor")
	}
}

func TestCoverageReportsStarvingThenStarved(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	coverage := NewCoverage(750 * time.Millisecond)
	if coverage.Observe(now, true) != StateCovered {
		t.Fatal("covered tick")
	}
	if coverage.Observe(now, false) != StateStarving || coverage.Observe(now.Add(700*time.Millisecond), false) != StateStarving {
		t.Fatal("within the grace period the state is starving")
	}
	if coverage.Observe(now.Add(750*time.Millisecond), false) != StateStarved {
		t.Fatal("after the grace period the state is starved")
	}
	coverage.Reset()
	if coverage.Observe(now.Add(800*time.Millisecond), false) != StateStarving {
		t.Fatal("Reset starts a new grace period")
	}
}
