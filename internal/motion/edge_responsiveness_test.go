package motion

import (
	"context"
	"math"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEdgeDoesNotAccelerateDefaultSteadyReversals(t *testing.T) {
	limits := config.DefaultSettings().Motion
	for _, pace := range []int{20, 35, 50, 80} {
		for _, width := range []int{20, 50, 100} {
			f := DefaultFlowSpec()
			f.MinPercent, f.MaxPercent = 50-width/2, 50+width/2
			f.RangeFloorPercent, f.RangeCeilingPercent = width, width
			f.SpeedPercent, f.PaceVariationPercent, f.AnchorPercent = pace, 0, 50
			target, err := FlowTarget(f, limits)
			if err != nil {
				t.Fatal(err)
			}
			prior := NewMotionPlan("before", target, limits, 0, 0, time.Time{}).Perceptual
			hold, err := HoldingTarget(ActiveMotionState{Target: target, Perceptual: prior}, limits)
			if err != nil {
				t.Fatal(err)
			}
			actual := NewMotionPlan("hold", hold, limits, 0, 0, time.Time{}).Perceptual
			beforeRate := prior.CommandedMeanTravelPerSecond / prior.MeanStrokePercent
			afterRate := actual.CommandedMeanTravelPerSecond / actual.MeanStrokePercent
			if afterRate > beforeRate*1.005 || actual.CommandedPeakVelocityPerSecond > prior.CommandedPeakVelocityPerSecond*1.005 ||
				hold.SpeedPercent < limits.SpeedMinPercent || actual.PositionMaxPercent > math.Ceil(prior.PositionMaxPercent) || actual.PositionMinPercent < math.Floor(prior.PositionMinPercent) {
				t.Fatalf("pace %d width %d: prior %.3f turns/s, hold %.3f turns/s", pace, width, beforeRate, afterRate)
			}
		}
	}
}

func TestEdgeDeclinesMovingHoldThatCannotFitSavedMinimum(t *testing.T) {
	limits := config.DefaultSettings().Motion
	pre := ActiveMotionState{Target: MotionTarget{SpeedPercent: 35}, Perceptual: PerceptualSummary{
		PositionMinPercent: 10, PositionMaxPercent: 90, MeanStrokePercent: 70, CommandedMeanTravelPerSecond: 45, CommandedPeakVelocityPerSecond: 65}}
	if _, err := HoldingTarget(pre, limits); err == nil {
		t.Fatal("moving hold accelerated a slow fitted pattern at the saved floor")
	}
	limits.SpeedMinPercent = 1
	hold, err := HoldingTarget(pre, limits)
	if err != nil {
		t.Fatal(err)
	}
	if hold.Flow.LoopCycles != 0 {
		t.Fatal("compact hold fitting leaked into the editable score")
	}
	p := NewMotionPlan("check", hold, limits, 0, 0, time.Time{}).Perceptual
	if p.CommandedPeakVelocityPerSecond > 65*1.005 || p.CommandedMeanTravelPerSecond/p.MeanStrokePercent > 45./70*1.005 {
		t.Fatal("hold exceeded the effective input pace")
	}
}

func TestEdgePhaseWaitsForTheSelectedFramesReceipt(t *testing.T) {
	e := &Engine{running: true, runEpoch: 3, edgeHandoffMillis: 1000, plan: MotionPlan{Target: MotionTarget{Source: "edge"}},
		transition: &planTransition{startMillis: 1000, strokeStartMillis: 1200, endMillis: 1600, stroke: &quinticSegment{}}}
	accepted := transport.CommandResult{Kind: transport.CommandKindPointsAdd, OK: true}
	for _, receipt := range []transport.CommandResult{{Kind: transport.CommandKindPointsAdd}, {Kind: transport.CommandKindStop, OK: true}} {
		e.acknowledgeEdgeFrame(3, receipt, nil, &MotionSample{TimeMillis: 1450})
		if e.holdingPhaseLocked(1400) != "queued" {
			t.Fatal("an unconfirmed append advanced Edge")
		}
	}
	for _, late := range []struct {
		epoch uint64
		tail  int64
	}{{2, 1700}, {3, 999}} {
		e.acknowledgeEdgeFrame(late.epoch, accepted, nil, &MotionSample{TimeMillis: late.tail})
		if e.holdingPhaseLocked(1400) != "queued" {
			t.Fatal("an older acknowledgment advanced Edge")
		}
	}
	e.acknowledgeEdgeFrame(3, accepted, nil, &MotionSample{TimeMillis: 1450})
	for _, at := range []struct {
		ms   int64
		want string
	}{{1199, "queued"}, {1200, "slowing"}, {1599, "slowing"}, {1600, "holding"}} {
		if got := e.holdingPhaseLocked(at.ms); got != at.want {
			t.Fatalf("%d: %s", at.ms, got)
		}
	}
	e.edgeFrameAccepted = false
	e.plan.Target.TempoOnly = true
	e.tempo = &tempoPlan{fromMillis: 200, revision: 5}
	e.acknowledgeEdgeFrame(3, accepted, &transport.TempoFrame{Revision: 4}, nil)
	if e.holdingPhaseLocked(800) != "queued" {
		t.Fatal("superseded motor envelope was acknowledged")
	}
	e.acknowledgeEdgeFrame(3, accepted, &transport.TempoFrame{Revision: 5}, nil)
	if e.holdingPhaseLocked(800) != "slowing" || e.holdingPhaseLocked(1200) != "holding" {
		t.Fatal("motor blend timing lost")
	}
}

type edgeWithoutStopReceipt struct{ *transport.Fake }

func (owner edgeWithoutStopReceipt) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	result, err := owner.Fake.Stop(ctx, command)
	if command.Reason == "edge_still" {
		result.OK = false
	}
	return result, err
}

func TestStillHoldRequiresAConfirmedTransportStop(t *testing.T) {
	owner := edgeWithoutStopReceipt{transport.NewFake()}
	engine, err := NewEngine(EngineOptions{Transport: owner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "test_cleanup") })
	before, err := engine.Start(t.Context(), MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 35}, config.DefaultSettings().Motion)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := engine.HoldStill(t.Context(), before.PlanID)
	if err == nil || !paused.Paused || paused.Running || paused.StillHold || paused.LastError == "" {
		t.Fatalf("unconfirmed stop reported as a still hold: paused=%t running=%t still=%t err=%v", paused.Paused, paused.Running, paused.StillHold, err)
	}
}

func TestEdgeUsesFirstUncommittedFrameWhenRefillIsDue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner := transport.NewFake()
		traces := diagnostics.NewTraceRing(128)
		engine, err := NewEngine(EngineOptions{Transport: owner, Traces: traces})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = engine.Stop(context.Background(), "cleanup") }()
		limits := config.DefaultSettings().Motion
		before, err := engine.Start(t.Context(), MotionTarget{PatternID: PatternFullSweeps, SpeedPercent: 50}, limits)
		if err != nil {
			t.Fatal(err)
		}
		// Press in the part of a polling interval where the accepted buffer
		// has crossed its refill threshold, before the next scheduler wake-up.
		due := false
		for range 100 {
			time.Sleep(10 * time.Millisecond)
			engine.mu.Lock()
			due = engine.bufferedTailMillisLocked() < engine.estimatedPlaybackMillisLocked(engine.now())+engine.leadMillisLocked()
			engine.mu.Unlock()
			if due {
				break
			}
		}
		if !due {
			t.Fatal("test did not reach an actual pending refill")
		}
		pressed := engine.Snapshot()
		target, err := HoldingTarget(pressed, limits)
		if err != nil {
			t.Fatal(err)
		}
		state, err := engine.ApplyTargetIfCurrent(t.Context(), target, "edge_hold", before.PlanID)
		if err != nil || state.EdgePhase != "queued" {
			t.Fatalf("first frame %q %v", state.EdgePhase, err)
		}
		found := false
		for _, row := range traces.Rows() {
			if row.Reason == "retarget_lead_points" {
				t.Fatal("old motion was refilled after Edge")
			}
			if row.Reason == "edge_hold" && row.Retarget != nil {
				found = true
				if row.Retarget.SelectedHandoffMillis != pressed.NextSampleMillis {
					t.Fatal("Edge skipped a mutable frame")
				}
				t.Logf("press-to-first-editable-sample %d ms", row.Retarget.SelectedLeadMillis)
			}
		}
		if !found {
			t.Fatal("missing retarget evidence")
		}
		time.Sleep(4 * time.Second)
		if got := engine.Snapshot().EdgePhase; got != "holding" {
			t.Fatalf("settled phase %q", got)
		}
	})
}
