package transport

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// clockedOwner reports a fixed playback origin after Play, like an owner
// with its own pre-roll.
type clockedOwner struct {
	*failingLinkedOwner
	start time.Time
	plays int
}

func (o *clockedOwner) Play(ctx context.Context, c PlayCommand) (CommandResult, error) {
	o.plays++
	return o.failingLinkedOwner.Play(ctx, c)
}
func (o *clockedOwner) PlaybackStartTime() time.Time { return o.start }

// alignedOwner records the clock it was started on.
type alignedOwner struct {
	*failingLinkedOwner
	startedAt []time.Time
	plays     int
}

func (o *alignedOwner) Play(ctx context.Context, c PlayCommand) (CommandResult, error) {
	o.plays++
	return o.failingLinkedOwner.Play(ctx, c)
}
func (o *alignedOwner) PlayAligned(ctx context.Context, c PlayCommand, start time.Time) (CommandResult, error) {
	o.startedAt = append(o.startedAt, start)
	return o.failingLinkedOwner.Play(ctx, c)
}

func threeOwners() []*failingLinkedOwner {
	return []*failingLinkedOwner{{Fake: NewFake()}, {Fake: NewFake()}, {Fake: NewFake()}}
}

func linkedThree(owners []*failingLinkedOwner, continueHealthy bool) *FanOut {
	return NewLinkedSession([]LinkedParticipant{
		{ID: "handy", Label: "Handy", Transport: owners[0]},
		{ID: "coyote", Label: "Coyote", Transport: owners[1]},
		{ID: "vibrator", Label: "Vibrator", Transport: owners[2]},
	}, continueHealthy).(*FanOut)
}

func TestLinkedSessionContinuesWhileAnyParticipantIsHealthy(t *testing.T) {
	orders := [][]int{{2, 1, 0}, {0, 2, 1}, {1, 0, 2}}
	for _, order := range orders {
		owners := threeOwners()
		f := linkedThree(owners, true)
		ctx := context.Background()
		_, _ = f.AppendPoints(ctx, strokeCommand("s", 5000))
		if _, err := f.Play(ctx, PlayCommand{StreamID: "s"}); err != nil {
			t.Fatal(err)
		}
		for step, failed := range order[:2] {
			owners[failed].disconnected = true
			d := f.Diagnostics()
			if d.PlaybackState == "stale" || len(owners[failed].stops) != 1 || !owners[failed].stops[0].Fault {
				t.Fatalf("order %v step %d: %+v", order, step, d)
			}
		}
		status := f.LinkedSessionStatus()
		if status.Status != "degraded" || len(status.RetiredDevices) != 2 {
			t.Fatalf("order %v: status %+v", order, status)
		}
		survivor := order[2]
		before := len(owners[survivor].Commands())
		if _, err := f.AppendPoints(ctx, strokeCommand("s", 7000)); err != nil || len(owners[survivor].Commands()) != before+1 {
			t.Fatalf("order %v: survivor did not continue: %v", order, err)
		}
		owners[survivor].disconnected = true
		if f.Diagnostics().PlaybackState != "stale" || f.LinkedSessionStatus().Status != "failed" {
			t.Fatalf("order %v: last failure did not stop the run", order)
		}
	}
}

func TestLinkedSessionStopAllStopsEveryParticipant(t *testing.T) {
	for failed := range 3 {
		owners := threeOwners()
		f := linkedThree(owners, false)
		ctx := context.Background()
		_, _ = f.AppendPoints(ctx, strokeCommand("s", 5000))
		_, _ = f.Play(ctx, PlayCommand{StreamID: "s"})
		owners[failed].disconnected = true
		if f.Diagnostics().PlaybackState != "stale" {
			t.Fatalf("participant %d failure did not latch", failed)
		}
		if _, err := f.Stop(ctx, StopCommand{Reason: "recover"}); err != nil {
			t.Fatal(err)
		}
		for index, owner := range owners {
			if len(owner.stops) == 0 || !owner.stops[len(owner.stops)-1].Fault {
				t.Fatalf("participant %d after failure of %d: stops %+v", index, failed, owner.stops)
			}
		}
	}
}

func TestLinkedSessionStartsCompanionsOnTheClock(t *testing.T) {
	start := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	clock := &clockedOwner{failingLinkedOwner: &failingLinkedOwner{Fake: NewFake()}, start: start}
	aligned := &alignedOwner{failingLinkedOwner: &failingLinkedOwner{Fake: NewFake()}}
	legacy := &failingLinkedOwner{Fake: NewFake()}
	f := NewLinkedSession([]LinkedParticipant{
		{ID: "handy", Label: "Handy", Transport: clock},
		{ID: "coyote", Label: "Coyote", Transport: aligned},
		{ID: "vibrator", Label: "Vibrator", Transport: legacy},
	}, false).(*FanOut)
	ctx := context.Background()
	_, _ = f.AppendPoints(ctx, strokeCommand("s", 5000))
	if _, err := f.Play(ctx, PlayCommand{StreamID: "s"}); err != nil {
		t.Fatal(err)
	}
	if aligned.plays != 0 || !reflect.DeepEqual(aligned.startedAt, []time.Time{start}) {
		t.Fatalf("companion was not started on the clock: plays %d, starts %v", aligned.plays, aligned.startedAt)
	}
	if f.PlaybackStartTime() != start {
		t.Fatal("the session clock must follow the first participant")
	}
}

func TestLinkedSessionHandsTheClockOnWhenItRetires(t *testing.T) {
	owners := threeOwners()
	start := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	second := &clockedOwner{failingLinkedOwner: owners[1], start: start}
	third := &alignedOwner{failingLinkedOwner: owners[2]}
	f := NewLinkedSession([]LinkedParticipant{
		{ID: "handy", Label: "Handy", Transport: owners[0]},
		{ID: "coyote", Label: "Coyote", Transport: second},
		{ID: "vibrator", Label: "Vibrator", Transport: third},
	}, true).(*FanOut)
	ctx := context.Background()
	_, _ = f.AppendPoints(ctx, strokeCommand("s", 5000))
	_, _ = f.Play(ctx, PlayCommand{StreamID: "s"})
	owners[0].fail = CommandKindPointsPlay
	if _, err := f.Play(ctx, PlayCommand{StreamID: "s"}); err != nil {
		t.Fatalf("resume after the clock failed: %v", err)
	}
	// The first Play had no clock origin (the fake Handy reports none). On
	// resume the Coyote stand-in becomes the clock and the vibrator starts on it.
	if second.plays != 2 || !reflect.DeepEqual(third.startedAt, []time.Time{start}) {
		t.Fatalf("the next participant did not take the clock: plays %d, starts %v", second.plays, third.startedAt)
	}
	if f.PlaybackStartTime() != start {
		t.Fatal("the engine still reads the retired participant's clock")
	}
}

func TestLinkedStopErrorsNameEachParticipant(t *testing.T) {
	owners := threeOwners()
	owners[1].fail = CommandKindStop
	owners[2].fail = CommandKindStop
	f := linkedThree(owners, false)
	_, err := f.Stop(context.Background(), StopCommand{Reason: "stop"})
	if err == nil || !strings.Contains(err.Error(), "Coyote:") || !strings.Contains(err.Error(), "Vibrator:") {
		t.Fatalf("joined Stop error = %v", err)
	}
	if len(owners[0].stops) != 1 {
		t.Fatal("failing companions blocked the clock's Stop")
	}
}
