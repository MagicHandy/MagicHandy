package transport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeScalarDevice advertises ScalarCmd features of the given actuator types,
// each with stepCount steps.
func fakeScalarDevice(index uint32, name string, stepCount uint32, actuators ...string) map[string]any {
	features := make([]map[string]any, 0, len(actuators))
	for _, actuator := range actuators {
		features = append(features, map[string]any{"FeatureDescriptor": actuator + " motor", "ActuatorType": actuator, "StepCount": stepCount})
	}
	return map[string]any{
		"DeviceIndex":            index,
		"DeviceName":             name,
		"DeviceMessageTimingGap": 0,
		"DeviceMessages":         map[string]any{"ScalarCmd": features, "StopDeviceCmd": map[string]any{}},
	}
}

func connectTestHub(t *testing.T, server *fakeButtplugServer) *IntifaceHub {
	t.Helper()
	hub, err := NewIntifaceHub(IntifaceOptions{Address: server.URL()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := hub.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = hub.Close() })
	return hub
}

// claimingOwner stands in for a second output bound to the same session.
type claimingOwner struct{ *Intiface }

func TestIntifaceHubParsesScalarActuatorsWithOriginalIndices(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	server.extraDevices = []map[string]any{fakeScalarDevice(3, "Test Vibrator", 20, "Vibrate", "Rotate", "Vibrate")}
	defer server.Close()
	hub := connectTestHub(t, server)
	status := hub.Status()
	if !status.Connected || status.PingWatchdog || len(status.Devices) != 2 {
		t.Fatalf("status = %+v", status)
	}
	vibrator := status.Devices[0]
	if vibrator.DeviceIndex != 3 || len(vibrator.ScalarActuators) != 3 || vibrator.ScalarActuators[2].Index != 2 || vibrator.ScalarActuators[1].ActuatorType != "Rotate" || vibrator.ScalarActuators[0].StepCount != 20 {
		t.Fatalf("scalar actuators = %+v", vibrator.ScalarActuators)
	}
	data, err := json.Marshal(status.Devices)
	if err != nil {
		t.Fatal(err)
	}
	// A device without linear actuators must report an empty list, not null.
	if !strings.Contains(string(data), `"linear_actuators":[]`) || !strings.Contains(string(data), `"scalar_actuators":[]`) {
		t.Fatalf("actuator lists must always be arrays: %s", data)
	}
}

func TestIntifaceHubSelectStopsTheNewDeviceFirst(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	hub := connectTestHub(t, server)
	linear := hub.Linear()
	if err := linear.Select(context.Background(), 7, 0); err != nil {
		t.Fatal(err)
	}
	stop := server.waitForKind(t, "StopDeviceCmd", time.Second)
	if index, _ := stop.uint32("DeviceIndex"); index != 7 {
		t.Fatalf("Select stopped device %d, want 7", index)
	}
	if status := linear.Status(); status.SelectedDeviceIndex == nil || *status.SelectedDeviceIndex != 7 {
		t.Fatalf("selection = %+v", status)
	}
}

func TestIntifaceHubSelectFailsWhenTheDeviceCannotBeStopped(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	server.rejectStop = true
	defer server.Close()
	hub := connectTestHub(t, server)
	linear := hub.Linear()
	if err := linear.Select(context.Background(), 7, 0); err == nil {
		t.Fatal("selected a device whose Stop was rejected")
	}
	if linear.Status().SelectedDeviceIndex != nil {
		t.Fatal("a failed Select must leave nothing selected")
	}
	other := &claimingOwner{}
	hub.session.mu.Lock()
	err := hub.session.claimLocked(other, 7)
	hub.session.mu.Unlock()
	if err != nil {
		t.Fatalf("a failed Select kept its claim: %v", err)
	}
}

func TestIntifaceHubClaimsOneRolePerDeviceUntilRelease(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	hub := connectTestHub(t, server)
	linear := hub.Linear()
	if err := linear.Select(context.Background(), 7, 0); err != nil {
		t.Fatal(err)
	}
	other := &claimingOwner{}
	hub.session.mu.Lock()
	err := hub.session.claimLocked(other, 7)
	hub.session.mu.Unlock()
	if !errors.Is(err, errIntifaceDeviceClaimed) {
		t.Fatalf("a second output claimed the stroker's device: %v", err)
	}
	if err := linear.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !hub.Status().Connected || linear.Status().SelectedDeviceIndex != nil {
		t.Fatal("Release must keep the session and drop the selection")
	}
	hub.session.mu.Lock()
	err = hub.session.claimLocked(other, 7)
	hub.session.mu.Unlock()
	if err != nil {
		t.Fatalf("Release kept the claim: %v", err)
	}
}

func TestIntifaceHubReturningDeviceIsANewIncarnation(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	hub := connectTestHub(t, server)
	linear := hub.Linear()
	if err := linear.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	first := linear.selection.incarnation
	server.sendEvent(t, "DeviceRemoved", map[string]any{"DeviceIndex": 7})
	server.sendEvent(t, "DeviceAdded", fakeLinearDevice(7, "Test Linear"))
	waitForTest(t, time.Second, func() bool {
		hub.session.mu.Lock()
		defer hub.session.mu.Unlock()
		incarnation, ok := hub.session.incarnationLocked(7)
		return ok && incarnation != first
	})
	if _, err := linear.selectedDevice(); err == nil {
		t.Fatal("the old selection survived the device leaving and returning")
	}
	if err := linear.SelectDevice(7, 0); err != nil || linear.selection.incarnation == first {
		t.Fatalf("reselect = %v, incarnation %d", err, linear.selection.incarnation)
	}
}

func TestIntifaceHubEmergencyStopHoldsTheBarrier(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	hold := make(chan struct{})
	server.stopAllHold = hold
	defer server.Close()
	hub := connectTestHub(t, server)
	done := make(chan error, 1)
	go func() { done <- hub.EmergencyStop(context.Background()) }()
	server.waitForKind(t, "StopAllDevices", time.Second)
	if err := hub.session.requestOK(context.Background(), "StartScanning", nil); !errors.Is(err, errIntifaceStopping) {
		t.Fatalf("a write passed the emergency barrier: %v", err)
	}
	if err := hub.session.requestOK(context.Background(), "Ping", nil); err != nil {
		t.Fatalf("the keepalive must pass the barrier: %v", err)
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := hub.StartScanning(context.Background()); err != nil {
		t.Fatalf("the barrier outlived the Stop: %v", err)
	}
}

func TestIntifaceRequestCapacityReservesStopAndPing(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	hub := connectTestHub(t, server)
	session := hub.session
	session.mu.Lock()
	for id := uint32(1_000_000); len(session.waiters) < maxIntifaceWaiters; id++ {
		session.waiters[id] = &intifaceWaiter{response: make(chan buttplugMessage, 1)}
	}
	session.mu.Unlock()
	if err := session.requestOK(context.Background(), "StartScanning", nil); !errors.Is(err, errIntifaceRequestCapacity) {
		t.Fatalf("an ordinary request exceeded the capacity: %v", err)
	}
	if err := session.requestOK(context.Background(), "Ping", nil); err != nil {
		t.Fatalf("Ping must use the reserve: %v", err)
	}
	session.mu.Lock()
	for id := range session.waiters {
		if id >= 1_000_000 {
			delete(session.waiters, id)
		}
	}
	session.mu.Unlock()
}
