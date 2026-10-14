package transport

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

func TestReviewGlobalStopCannotSupersedeItsOwnZero(t *testing.T) {
	f := newCoyoteFixture(t)
	link := f.connect()
	f.confirm()
	if _, err := f.owner.AdjustStrength(estim.ChannelA, 5); err != nil {
		t.Fatal(err)
	}
	f.pace(1)
	f.waitFor("strength five", func(s CoyoteStatus) bool { return s.StrengthA == 5 && !s.StrengthPending })
	// Model a previous write holding the serialized wire lane. Global Stop
	// launches EmergencyZero concurrently with the engine's transport Stop.
	f.owner.writeMu.Lock()
	locked := true
	defer func() {
		if locked {
			f.owner.writeMu.Unlock()
		}
	}()
	f.owner.mu.Lock()
	start := f.owner.generation
	f.owner.mu.Unlock()
	waitGeneration := func(want uint64) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			f.owner.mu.Lock()
			got := f.owner.generation
			f.owner.mu.Unlock()
			if got >= want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("generation did not reach %d", want)
	}
	zeroDone := make(chan error, 1)
	go func() { zeroDone <- f.owner.EmergencyZero() }()
	waitGeneration(start + 1)
	stopDone := make(chan error, 1)
	go func() { _, err := f.owner.Stop(context.Background(), StopCommand{}); stopDone <- err }()
	waitGeneration(start + 2)
	f.owner.writeMu.Unlock()
	locked = false
	zeroErr, stopErr := <-zeroDone, <-stopDone
	f.clock.Advance(2 * time.Second)
	f.owner.paceOnce()
	device, status := link.Snapshot(), f.owner.Status()
	t.Logf("global stop outcomes: zero=%v stop=%v; device A=%d B=%d; UI known=%v A=%d B=%d", zeroErr, stopErr, device.StrengthA, device.StrengthB, status.StrengthKnown, status.StrengthA, status.StrengthB)
	if zeroErr != nil || stopErr != nil || device.StrengthA != 0 || device.StrengthB != 0 {
		t.Fatal("concurrent global Stop did not zero device strength")
	}
}

func TestReviewLoweringAnIdleChannelMustNotStartOutput(t *testing.T) {
	f := newCoyoteFixture(t)
	link := f.connect()
	f.confirm()
	if _, err := f.owner.AdjustStrength(estim.ChannelA, 5); err != nil {
		t.Fatal(err)
	}
	f.pace(1)
	f.waitFor("strength five", func(s CoyoteStatus) bool { return s.StrengthA == 5 && !s.StrengthPending })
	if _, err := f.owner.Stop(context.Background(), StopCommand{}); err != nil {
		t.Fatal(err)
	}
	if got := link.Snapshot().OutputA; got != 0 {
		t.Fatalf("fixture did not silence: %d", got)
	}
	if _, err := f.owner.AdjustStrength(estim.ChannelA, -1); err != nil {
		t.Fatal(err)
	}
	f.pace(1)
	device := link.Snapshot()
	t.Logf("lower from idle: strength=%d commanded waveform width=%d", device.StrengthA, device.OutputA)
	if device.OutputA != 0 {
		t.Fatal("a lowering safety action started a reference pulse on an idle channel")
	}
}

type reviewLimitFailureConnector struct {
	base *estim.SimulatedConnector
	link *reviewLimitFailureLink
}

func (c *reviewLimitFailureConnector) Simulated() bool { return true }
func (c *reviewLimitFailureConnector) Connect(ctx context.Context, req estim.ConnectRequest) (estim.Link, error) {
	link, err := c.base.Connect(ctx, req)
	if err != nil {
		return nil, err
	}
	c.link = &reviewLimitFailureLink{Link: link}
	return c.link, nil
}

type reviewLimitFailureLink struct {
	estim.Link
	fail atomic.Bool
}

func (l *reviewLimitFailureLink) Write(ctx context.Context, payload []byte, ack bool) error {
	if len(payload) == estim.LimitsBytes && l.fail.Load() {
		return errors.New("simulated BF write failure")
	}
	return l.Link.Write(ctx, payload, ack)
}

func TestReviewFailedCeilingWriteCannotRemainApplied(t *testing.T) {
	f := newCoyoteFixture(t)
	connector := &reviewLimitFailureConnector{base: f.connector}
	f.owner.connector = connector
	link := f.connect()
	f.confirm()
	if _, err := f.owner.AdjustStrength(estim.ChannelA, 5); err != nil {
		t.Fatal(err)
	}
	f.pace(1)
	f.waitFor("strength five", func(s CoyoteStatus) bool { return s.StrengthA == 5 && !s.StrengthPending })
	if _, err := f.owner.AppendPoints(context.Background(), strokeCommand("review", 10000)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Play(context.Background(), PlayCommand{StreamID: "review"}); err != nil {
		t.Fatal(err)
	}
	f.pace(25)
	f.owner.mu.Lock()
	next := f.owner.config
	f.owner.mu.Unlock()
	next.Limits.StrengthA = 1
	connector.link.fail.Store(true)
	firstErr := f.owner.Configure(next)
	f.pace(1)
	// A later successful retry must really send BF again, not treat the failed
	// configuration as already applied.
	connector.link.fail.Store(false)
	retryErr := f.owner.Configure(next)
	device, status := link.Snapshot(), f.owner.Status()
	t.Logf("ceiling write=%v retry=%v; device ceiling=%d strength=%d output=%d; UI ceiling=%d connected=%v", firstErr, retryErr, device.CeilingA, device.StrengthA, device.OutputA, status.CeilingA, status.Connected)
	if firstErr == nil {
		t.Fatal("fixture did not reject BF")
	}
	if retryErr == nil && status.Connected && device.CeilingA != next.Limits.StrengthA {
		t.Fatal("failed ceiling persisted as applied; retry silently succeeded without updating the hardware")
	}
	if !device.Closed || device.OutputA != 0 || status.Connected || status.LimitsApplied {
		t.Fatalf("uncertain ceiling did not zero and retire the connection: device=%+v status=%+v", device, status)
	}
	if _, err := f.owner.Play(context.Background(), PlayCommand{StreamID: "review"}); err == nil {
		t.Fatal("playback was allowed after a failed ceiling write")
	}
	if _, err := f.owner.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if applied := f.connector.LastLink().Snapshot(); applied.CeilingA != 1 || applied.StrengthA != 0 || applied.Limits != 1 {
		t.Fatalf("reconnect did not apply the requested limits and zero: %+v", applied)
	}
}

func TestCoyoteTestAndRaiseAreFencedByStop(t *testing.T) {
	f := newCoyoteFixture(t)
	link := f.connect()
	f.confirm()
	admitted := f.owner.ControlGeneration()
	if err := f.owner.EmergencyZero(); err != nil {
		t.Fatal(err)
	}
	f.confirm()
	if _, err := f.owner.TestChannel(context.Background(), admitted, estim.ChannelA); !errors.Is(err, errCoyoteSuperseded) {
		t.Fatalf("a pre-Stop test was accepted: %v", err)
	}
	if _, err := f.owner.AdjustStrengthAt(context.Background(), admitted, estim.ChannelA, 1); !errors.Is(err, errCoyoteSuperseded) {
		t.Fatalf("a pre-Stop increase was accepted: %v", err)
	}
	f.pace(3)
	if state := link.Snapshot(); state.OutputA != 0 || state.StrengthA != 0 {
		t.Fatalf("stale controls reached the device: %+v", state)
	}
}

func TestCoyoteStopCancelsExplicitTestAndQueuedStrength(t *testing.T) {
	f := newCoyoteFixture(t)
	link := f.connect()
	f.confirm()
	_, _ = f.owner.AdjustStrength(estim.ChannelA, 5)
	f.pace(1)
	f.waitFor("report", func(status CoyoteStatus) bool { return status.StrengthA == 5 && !status.StrengthPending })
	if _, err := f.owner.TestChannel(context.Background(), f.owner.ControlGeneration(), estim.ChannelA); err != nil {
		t.Fatal(err)
	}
	f.pace(1)
	_, _ = f.owner.AdjustStrength(estim.ChannelA, 1)
	if _, err := f.owner.Stop(context.Background(), StopCommand{Reason: "pause"}); err != nil {
		t.Fatal(err)
	}
	packets := link.Snapshot().Packets
	f.pace(30)
	if state := link.Snapshot(); state.Packets != packets || state.StrengthA != 5 || state.OutputA != 0 {
		t.Fatalf("queued output survived Stop: %+v", state)
	}
}

func TestCoyoteGlobalStopHoldsAdmissionUntilBothOutputsFinish(t *testing.T) {
	f := newCoyoteFixture(t)
	f.connect()
	f.confirm()
	finish := f.owner.BeginEmergencyStop()
	f.waitFor("zero delivery", func(status CoyoteStatus) bool { return status.StrengthSource == estim.StrengthZeroCommand })
	if _, err := f.owner.AdjustStrength(estim.ChannelA, 1); !errors.Is(err, estim.ErrStrengthUnconfirmed) {
		t.Fatalf("an increase entered while the other output was stopping: %v", err)
	}
	if _, err := f.owner.TestChannel(context.Background(), f.owner.ControlGeneration(), estim.ChannelA); !errors.Is(err, errCoyoteNotReady) {
		t.Fatalf("a test entered while the other output was stopping: %v", err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	f.confirm()
	if _, err := f.owner.AdjustStrength(estim.ChannelA, 1); err != nil {
		t.Fatalf("a deliberate new increase after Stop was rejected: %v", err)
	}
}
