package workerclient

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/ble"
	"github.com/mapledaemon/MagicHandy/internal/handyble"
)

func TestHandyAndCoyoteWorkersHaveIndependentLifetimes(t *testing.T) {
	coyote := connectSimulated(t)
	connector := NewConnector(Options{Profile: ble.ProfileHandy, Path: simulatedWorker(t), Args: []string{"-simulate"}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	link, err := connector.Connect(ctx, ble.ConnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := handyble.NewSession(link)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := session.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	points := make([]handyble.Point, 100)
	for i := range points {
		points[i] = handyble.Point{Time: uint32(i * 10), Position: uint32(i % 101)}
	}
	if _, err := session.Execute(ctx, handyble.Request{Method: handyble.Add, Points: points, Tail: 99, StreamID: 1}); err != nil {
		t.Fatal(err)
	}
	// Kill one actual test-owned worker process. The other process and its BLE
	// protocol must remain responsive; no adapter is opened in either helper.
	if err := link.(*workerLink).process.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("Handy process loss was not delivered")
	}
	if battery, err := coyote.ReadBattery(context.Background()); err != nil || battery <= 0 {
		t.Fatalf("Coyote affected by Handy failure: %d %v", battery, err)
	}
}
