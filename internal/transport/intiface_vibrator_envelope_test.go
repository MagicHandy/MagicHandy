package transport

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

func TestVibratorIndependentEnvelopeAndCoverage(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(.4, vibration.FollowSteady)
	command := strokeCommand("independent", 10_000)
	command.Vibration = &VibrationFrame{Revision: 1, Points: []TimedLevel{{TimeMillis: 0, LevelPercent: 50}, {TimeMillis: 4000, LevelPercent: 50}}}
	if _, err := run.AppendPoints(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Play(t.Context(), PlayCommand{StreamID: command.StreamID}); err != nil {
		t.Fatal(err)
	}
	f.pace(3 * time.Second)
	for _, level := range lastScalars(t, f.server) {
		if level != .2 {
			t.Fatalf("50%% of armed40%% = %v", level)
		}
	}
	// Linear coverage remains long, but independent output must stop at its own tail.
	f.pace(2 * time.Second)
	for _, level := range lastScalars(t, f.server) {
		if level != 0 {
			t.Fatal("missing scalar coverage retained output")
		}
	}
}

func TestVibratorEnvelopeRevisionFencesPreparedOutput(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(.4, vibration.FollowSteady)
	command := strokeCommand("independent", 10_000)
	_, _ = run.AppendPoints(t.Context(), command)
	_, _ = run.Play(t.Context(), PlayCommand{StreamID: command.StreamID})
	f.clock.Advance(3 * time.Second)
	f.vibrator.mu.Lock()
	prepared, ok := f.vibrator.composeLocked(f.clock.Now())
	f.vibrator.mu.Unlock()
	if !ok {
		t.Fatal("missing prepared level")
	}
	frame := AppendPointsCommand{StreamID: command.StreamID, Vibration: &VibrationFrame{Revision: 2, Points: []TimedLevel{{TimeMillis: 0}, {TimeMillis: 10_000}}}}
	if _, err := run.AppendPoints(t.Context(), frame); err != nil {
		t.Fatal(err)
	}
	f.vibrator.send(context.Background(), prepared)
	f.pace(0)
	for _, level := range lastScalars(t, f.server) {
		if level != 0 {
			t.Fatal("old command crossed envelope off")
		}
	}
	frame.Vibration.Revision = 1
	if _, err := run.AppendPoints(t.Context(), frame); err == nil {
		t.Fatal("old frame replaced newer envelope")
	}
}

func TestVibratorRejectsWholeAppendBeforeChangingEitherLane(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(.4, vibration.FollowSteady)
	initial := strokeCommand("current", 10_000)
	if _, err := run.AppendPoints(t.Context(), initial); err != nil {
		t.Fatal(err)
	}
	for _, stream := range []string{"current", "replacement"} {
		bad := strokeCommand(stream, 20_000)
		bad.Vibration = &VibrationFrame{Revision: 1, Points: []TimedLevel{{TimeMillis: 1, LevelPercent: 50}, {TimeMillis: 0, LevelPercent: 50}}}
		if _, err := run.AppendPoints(t.Context(), bad); err == nil {
			t.Fatal("unordered envelope accepted")
		}
		f.vibrator.mu.Lock()
		tail, _ := f.vibrator.stream.timeline.End()
		id, independent := f.vibrator.stream.id, f.vibrator.stream.independent
		f.vibrator.mu.Unlock()
		if tail != 10_000 || id != "current" || independent {
			t.Fatalf("partial append: %s %d %t", id, tail, independent)
		}
	}
}
