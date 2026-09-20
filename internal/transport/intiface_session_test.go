package transport

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestIntifaceKeepsWatchdogAliveDuringEnumeration(t *testing.T) {
	server := newFakeButtplugServer(t, 100)
	server.listAfterPing = true
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if !owner.Status().Connected || len(owner.Devices()) != 1 {
		t.Fatalf("discovery did not finish after first Ping: %+v", owner.Status())
	}
	server.waitForKind(t, "Ping", time.Second)
}

func TestIntifaceDiscoveryAndScanResponsesPreserveLaterEvents(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	added := fakeLinearDevice(8, "Replacement")
	added["Id"] = 0
	server.initialEvents = []map[string]any{
		{"DeviceRemoved": map[string]any{"Id": 0, "DeviceIndex": 7}},
		{"DeviceAdded": added},
	}
	server.scanFinishes = true
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	// The Ping response is a reader barrier after all events in the earlier frame.
	if err := owner.requestOK(t.Context(), "Ping", nil); err != nil {
		t.Fatal(err)
	}
	if devices := owner.Devices(); len(devices) != 1 || devices[0].DeviceIndex != 8 {
		t.Fatalf("discovery overwrote hotplug events: %+v", devices)
	}
	if err := owner.StartScanning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := owner.requestOK(t.Context(), "Ping", nil); err != nil {
		t.Fatal(err)
	}
	if owner.Status().Scanning {
		t.Fatal("StartScanning response overwrote ScanningFinished")
	}
}

func TestIntifaceRejectsOverflowingWatchdogDuration(t *testing.T) {
	server := newFakeButtplugServer(t, math.MaxInt64)
	defer server.Close()
	owner, err := NewIntiface(IntifaceOptions{Address: server.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestIntiface(t, owner)
	if err := owner.Connect(t.Context()); err == nil {
		t.Fatal("overflowing watchdog interval was accepted")
	}
}

func TestIntifaceUncorrelatedPingErrorRetiresSession(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	server.sendEvent(t, "Error", map[string]any{"Id": 0, "ErrorCode": 2, "ErrorMessage": "Ping timeout"})
	waitForTest(t, time.Second, func() bool { return !owner.Status().Connected })
	if _, err := owner.AppendPoints(context.Background(), AppendPointsCommand{StreamID: "stale", Points: []TimedPoint{{TimeMillis: 0, PositionPercent: 50}}}); err == nil {
		t.Fatal("protocol failure left motion admission open")
	}
}
