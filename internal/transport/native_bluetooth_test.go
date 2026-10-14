package transport

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/handyble"
)

func TestNativeHandyUsesSharedMappingAndRetiresOldTransport(t *testing.T) {
	c := handyble.NewSimulatedConnector()
	owner := NewNativeBluetoothOwner(c)
	t.Cleanup(func() { _ = owner.Close() })
	if _, err := owner.Connect(t.Context(), ble.ConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	link := c.LastLink()
	before := len(link.Writes())
	transport := owner.NewTransport(BrowserBluetoothOptions{})
	if _, err := transport.SetStrokeWindow(t.Context(), StrokeWindowCommand{MinPercent: 10, MaxPercent: 90, ReverseDirection: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "story", Points: []TimedPoint{{TimeMillis: 0, PositionPercent: 20.4}, {TimeMillis: 200, PositionPercent: 80.6}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Play(t.Context(), PlayCommand{StreamID: "story"}); err != nil {
		t.Fatal(err)
	}
	writes := link.Writes()[before:]
	// IDs follow Initialize's Stop/three clock samples/clock set. Compare the
	// exact intended commands, including reversal exactly once and 0..100 units.
	stroke, _ := handyble.Encode(handyble.Request{Method: handyble.Stroke, ID: 6, Min: .1, Max: .9})
	add, _ := handyble.Encode(handyble.Request{Method: handyble.Add, ID: 8, Points: []handyble.Point{{Time: 0, Position: 80}, {Time: 200, Position: 19}}, Tail: 1})
	if len(writes) != 4 || !bytes.Equal(writes[0], stroke) || !bytes.Equal(writes[2], add) {
		t.Fatalf("unexpected native mapping: %x", writes)
	}
	if diagnostics := transport.Diagnostics(); diagnostics.Name != NativeBluetoothName || !diagnostics.Connected {
		t.Fatalf("diagnostics %+v", diagnostics)
	}
	if err := owner.Disconnect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Connect(t.Context(), ble.ConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	replacement := c.LastLink()
	count := len(replacement.Writes())
	if _, err := transport.Play(context.Background(), PlayCommand{StreamID: "old"}); err == nil {
		t.Fatal("old transport wrote to replacement connection")
	}
	if _, err := transport.Stop(t.Context(), StopCommand{}); err != nil {
		t.Fatal(err)
	}
	if len(replacement.Writes()) != count {
		t.Fatal("retired transport altered replacement connection")
	}
}

type delayedNativeConnector struct {
	*handyble.SimulatedConnector
	entered, release chan struct{}
}

func (c *delayedNativeConnector) Connect(context.Context, ble.ConnectRequest) (ble.Link, error) {
	close(c.entered)
	<-c.release // emulate a native connection completing after cancellation
	return c.SimulatedConnector.Connect(context.Background(), ble.ConnectRequest{})
}

func TestNativeLateConnectionAfterStopIsClosedWithoutCommands(t *testing.T) {
	c := &delayedNativeConnector{SimulatedConnector: handyble.NewSimulatedConnector(), entered: make(chan struct{}), release: make(chan struct{})}
	o := NewNativeBluetoothOwner(c)
	t.Cleanup(func() { _ = o.Close() })
	finished := make(chan error, 1)
	go func() { _, err := o.Connect(context.Background(), ble.ConnectRequest{}); finished <- err }()
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("connect did not start")
	}
	o.Interrupt()
	close(c.release)
	if err := <-finished; err == nil {
		t.Fatal("late connection reported ready after Stop")
	}
	if o.Status().Connected || len(c.LastLink().Writes()) != 0 {
		t.Fatal("late connection issued device commands")
	}
	if err := c.LastLink().Write(t.Context(), []byte{1}, true); err == nil {
		t.Fatal("late link was left open")
	}
}

type countedNativeConnector struct {
	*delayedNativeConnector
	calls atomic.Int32
}

func (c *countedNativeConnector) Connect(ctx context.Context, request ble.ConnectRequest) (ble.Link, error) {
	if c.calls.Add(1) > 1 {
		return nil, errors.New("second connection reached the radio")
	}
	return c.delayedNativeConnector.Connect(ctx, request)
}

func TestNativeDisconnectKeepsConnectionSlotUntilCanceledAttemptFinishes(t *testing.T) {
	c := &countedNativeConnector{delayedNativeConnector: &delayedNativeConnector{
		SimulatedConnector: handyble.NewSimulatedConnector(), entered: make(chan struct{}), release: make(chan struct{}),
	}}
	o := NewNativeBluetoothOwner(c)
	finished := make(chan error, 1)
	go func() { _, err := o.Connect(context.Background(), ble.ConnectRequest{}); finished <- err }()
	t.Cleanup(func() {
		close(c.release)
		if err := <-finished; err == nil {
			t.Error("canceled connection became ready")
		}
		_ = o.Close()
	})
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("connect did not start")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := o.Disconnect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("disconnect should stop waiting when canceled: %v", err)
	}
	if !o.Status().Connecting {
		t.Fatal("unfinished connect disappeared from backend state")
	}
	if _, err := o.Connect(t.Context(), ble.ConnectRequest{}); err == nil {
		t.Fatal("reconnect overlapped unfinished connection teardown")
	}
	if c.calls.Load() != 1 {
		t.Fatal("reconnect reached the radio before canceled connection finished")
	}
}
