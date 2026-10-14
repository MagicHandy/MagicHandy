package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

func TestBrowserBluetoothOldSessionCannotCommandSameTabReconnect(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "same-tab"})
	old := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})
	bridge.DisconnectClient("same-tab", "lost")
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "same-tab"})
	ctx := context.Background()
	if _, err := old.AppendPoints(ctx, strokeCommand("old", 5000)); err == nil {
		t.Fatal("old BLE append entered new session")
	}
	if _, err := old.Play(ctx, PlayCommand{StreamID: "old"}); err == nil {
		t.Fatal("old BLE Play entered new session")
	}
	if _, err := old.Stop(ctx, StopCommand{Reason: "old_stop"}); err != nil {
		t.Fatal(err)
	}
	if bridge.Snapshot().Pending != 0 {
		t.Fatal("old session queued commands for replacement")
	}
	if old.Diagnostics().PlaybackState != "stale" {
		t.Fatal("old transport regained readiness")
	}
}

type failingLinkedOwner struct {
	*Fake
	fail         CommandKind
	disconnected bool
	stops        []StopCommand
}

func (o *failingLinkedOwner) AppendPoints(ctx context.Context, c AppendPointsCommand) (CommandResult, error) {
	if o.fail == CommandKindPointsAdd {
		return CommandResult{}, errors.New("append failed")
	}
	return o.Fake.AppendPoints(ctx, c)
}
func (o *failingLinkedOwner) Play(ctx context.Context, c PlayCommand) (CommandResult, error) {
	if o.fail == CommandKindPointsPlay {
		return CommandResult{}, errors.New("play failed")
	}
	return o.Fake.Play(ctx, c)
}
func (o *failingLinkedOwner) Stop(ctx context.Context, c StopCommand) (CommandResult, error) {
	o.stops = append(o.stops, c)
	if o.fail == CommandKindStop {
		return CommandResult{}, errors.New("stop failed")
	}
	return o.Fake.Stop(ctx, c)
}
func (o *failingLinkedOwner) Diagnostics() TransportDiagnostics {
	d := o.Fake.Diagnostics()
	d.Connected = !o.disconnected
	return d
}

func TestLinkedCommandFailuresStopBothAndLatch(t *testing.T) {
	for _, stage := range []CommandKind{CommandKindPointsAdd, CommandKindPointsPlay} {
		for _, side := range []int{0, 1} {
			for _, continueHealthy := range []bool{false, true} {
				a, b := &failingLinkedOwner{Fake: NewFake()}, &failingLinkedOwner{Fake: NewFake()}
				owners := []*failingLinkedOwner{a, b}
				f := NewFanOutWithPolicy(a, b, continueHealthy)
				owners[side].fail = stage
				ctx := context.Background()
				var err error
				if stage == CommandKindPointsAdd {
					_, err = f.AppendPoints(ctx, strokeCommand("s", 5000))
				} else {
					_, _ = f.AppendPoints(ctx, strokeCommand("s", 5000))
					_, err = f.Play(ctx, PlayCommand{StreamID: "s"})
				}
				if err == nil || len(a.stops) != 1 || len(b.stops) != 1 || !a.stops[0].Fault || !b.stops[0].Fault {
					t.Fatalf("startup failure not stopped: %s/%d/%t: %v", stage, side, continueHealthy, err)
				}
				owners[side].fail = ""
				if _, err := f.Play(ctx, PlayCommand{StreamID: "s"}); err == nil {
					t.Fatal("faulted session restarted")
				}
			}
		}
	}
}

func TestLinkedContinuationRetiresEitherFailedParticipant(t *testing.T) {
	for _, side := range []int{0, 1} {
		a, b := &failingLinkedOwner{Fake: NewFake()}, &failingLinkedOwner{Fake: NewFake()}
		owners := []*failingLinkedOwner{a, b}
		f := NewFanOutWithPolicy(a, b, true)
		ctx := context.Background()
		_, _ = f.AppendPoints(ctx, strokeCommand("s", 5000))
		_, _ = f.Play(ctx, PlayCommand{StreamID: "s"})
		owners[side].disconnected = true
		d := f.Diagnostics()
		if d.PlaybackState == "stale" || !d.Connected || d.LastError == "" || len(owners[side].stops) != 1 || !owners[side].stops[0].Fault {
			t.Fatalf("continuation: %+v", d)
		}
		owners[side].disconnected = false
		before := len(owners[side].Commands())
		if _, err := f.AppendPoints(ctx, strokeCommand("s", 7000)); err != nil {
			t.Fatal(err)
		}
		if len(owners[side].Commands()) != before {
			t.Fatal("reconnected participant hot-joined")
		}
		owners[1-side].disconnected = true
		if f.Diagnostics().PlaybackState != "stale" {
			t.Fatal("both failed but session remained healthy")
		}
	}
}

func TestLinkedContinuationHandlesAppendFailureAndStopErrors(t *testing.T) {
	a, b := &failingLinkedOwner{Fake: NewFake()}, &failingLinkedOwner{Fake: NewFake()}
	f := NewFanOutWithPolicy(a, b, true)
	ctx := context.Background()
	_, _ = f.Play(ctx, PlayCommand{StreamID: "s"})
	b.fail = CommandKindPointsAdd
	if result, err := f.AppendPoints(ctx, strokeCommand("s", 5000)); err != nil || !result.OK {
		t.Fatalf("healthy participant lost: %+v %v", result, err)
	}
	b.fail = CommandKindStop
	if result, err := f.Stop(ctx, StopCommand{Reason: "stop"}); err == nil || result.OK {
		t.Fatal("companion Stop failure was swallowed")
	}
	if len(a.stops) == 0 {
		t.Fatal("failed companion blocked primary Stop")
	}
}

func TestCoyoteSessionReadinessAndReconnectFence(t *testing.T) {
	f := newCoyoteFixture(t)
	old := f.connect()
	if _, err := f.owner.MotionTransport(); err == nil {
		t.Fatal("pending handshake admitted playback")
	}
	f.confirm()
	session, err := f.owner.MotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = session.AppendPoints(ctx, strokeCommand("old", 5000))
	if _, err := session.Play(ctx, PlayCommand{StreamID: "old"}); err != nil {
		t.Fatalf("known zero must be accepted: %v", err)
	}
	old.Lose(errors.New("lost"))
	f.waitFor("disconnect", func(s CoyoteStatus) bool { return !s.Connected })
	newLink := f.connect()
	f.confirm()
	before := newLink.Snapshot().Packets
	if _, err := session.AppendPoints(ctx, strokeCommand("old", 6000)); err == nil {
		t.Fatal("old append accepted")
	}
	if _, err := session.Play(ctx, PlayCommand{StreamID: "old"}); err == nil {
		t.Fatal("old Play accepted")
	}
	_, _ = session.Stop(ctx, StopCommand{Fault: true})
	if newLink.Snapshot().Packets != before {
		t.Fatal("old session commanded replacement link")
	}
	if session.Diagnostics().PlaybackState != "stale" {
		t.Fatal("old session regained readiness")
	}
}

func TestCoyoteFaultStopZeroesWhilePausePreservesStrength(t *testing.T) {
	f := newCoyoteFixture(t)
	link := f.connect()
	f.confirm()
	_, _ = f.owner.AdjustStrength(estim.ChannelA, 5)
	f.pace(1)
	f.waitFor("raise", func(s CoyoteStatus) bool { return s.StrengthA == 5 && !s.StrengthPending })
	session, err := f.owner.MotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = session.Stop(context.Background(), StopCommand{Reason: "pause"})
	if link.Snapshot().StrengthA != 5 {
		t.Fatal("Pause changed strength")
	}
	_, err = session.Stop(context.Background(), StopCommand{Fault: true})
	if err != nil {
		t.Fatal(err)
	}
	if link.Snapshot().StrengthA != 0 {
		t.Fatal("fault Stop did not zero")
	}
}

func TestRemovedBluetoothCannotBeRevivedByDelayedHeartbeat(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "tab"})
	bridge.DisconnectClient("tab", "removed")
	connected := true
	bridge.UpdateClient(BrowserBluetoothClientStatus{ClientID: "tab", Connected: &connected})
	if bridge.Snapshot().Connected {
		t.Fatal("a delayed heartbeat revived the removed link")
	}
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "tab"})
	if !bridge.Snapshot().Connected {
		t.Fatal("explicit reconnect must work")
	}
}
