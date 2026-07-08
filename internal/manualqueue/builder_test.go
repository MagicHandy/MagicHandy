package manualqueue

import "testing"

func TestNormalizeActionsToZero(t *testing.T) {
	actions := []Action{{At: 1000, Pos: 50}, {At: 2000, Pos: 80}}
	norm := NormalizeActionsToZero(actions)
	if norm[0].At != 0 || norm[1].At != 1000 {
		t.Fatalf("normalize = %+v", norm)
	}
}

func TestExpandBlockNoLoopHoldsLastPos(t *testing.T) {
	actions := []Action{{At: 0, Pos: 10}, {At: 2000, Pos: 90}}
	out := ExpandBlockActions(actions, 5, false)
	if out[len(out)-1].At != 5000 || out[len(out)-1].Pos != 90 {
		t.Fatalf("expand = %+v", out)
	}
}

func TestExpandBlockLoopTiles(t *testing.T) {
	actions := []Action{{At: 0, Pos: 10}, {At: 1000, Pos: 90}}
	out := ExpandBlockActions(actions, 3, true)
	if out[0].At != 0 || out[len(out)-1].At > 3000 {
		t.Fatalf("expand = %+v", out)
	}
}

func TestConcatManualQueueItems(t *testing.T) {
	blocks := map[string][]Action{
		"a": {{At: 0, Pos: 20}, {At: 1000, Pos: 80}},
		"b": {{At: 0, Pos: 30}, {At: 500, Pos: 70}},
	}
	items := []Item{
		{BlockID: "a", DurationSec: 2, Loop: false},
		{BlockID: "b", DurationSec: 1, Loop: false},
	}
	merged, durationMS := ConcatManualQueueItems(items, blocks)
	if durationMS != 3000 || merged[0].At != 0 {
		t.Fatalf("merged = %+v duration=%d", merged, durationMS)
	}
}
