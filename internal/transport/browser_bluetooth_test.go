package transport

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBrowserBluetoothTransportQueuesCommandAndWaitsForAck(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	connected := true
	bridge.ConnectClient(BrowserBluetoothClientStatus{
		ClientID:   "client-1",
		Connected:  &connected,
		DeviceName: "Handy",
		Protocol:   "hsp_ble",
	})
	bluetooth := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{ReverseDirection: true})

	done := make(chan resultAndError, 1)
	go func() {
		result, err := bluetooth.AddHSP(context.Background(), HSPAddCommand{
			StreamID: "7",
			Points: []TimedPoint{
				{PositionPercent: 25, TimeMillis: 10},
				{PositionPercent: 75, TimeMillis: 250},
			},
		})
		done <- resultAndError{result: result, err: err}
	}()

	commands, err := bridge.NextCommands(context.Background(), "client-1", time.Second)
	if err != nil {
		t.Fatalf("NextCommands: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(commands))
	}
	command := commands[0]
	if command.Path != "hsp/add" || command.Kind != CommandKindHSPAdd {
		t.Fatalf("command = %+v, want HSP add bridge command", command)
	}
	if command.Body["stream_id"] != 7 {
		t.Fatalf("stream_id = %#v, want 7", command.Body["stream_id"])
	}
	points, ok := command.Body["points"].([]map[string]any)
	if !ok || len(points) != 2 {
		t.Fatalf("points = %#v, want two bridge points", command.Body["points"])
	}
	if points[0]["x"] != 75 || points[1]["x"] != 25 {
		t.Fatalf("reverse points = %+v, want 75 then 25", points)
	}

	bridge.Acknowledge("client-1", BrowserBluetoothBridgeAck{
		ID:            command.ID,
		OK:            true,
		ElapsedMillis: 12.5,
		Response: map[string]any{
			"hsp_state": map[string]any{"play_state": "buffered"},
		},
	})

	outcome := readBluetoothOutcome(t, done)
	if outcome.err != nil {
		t.Fatalf("AddHSP: %v", outcome.err)
	}
	if !outcome.result.OK || outcome.result.Status != "browser_ack" {
		t.Fatalf("result = %+v, want browser ack", outcome.result)
	}
	if outcome.result.CommandID != command.ID {
		t.Fatalf("command id = %q, want %q", outcome.result.CommandID, command.ID)
	}
	diagnostics := bluetooth.Diagnostics()
	if diagnostics.LastCommand == nil || diagnostics.LastCommand.Kind != CommandKindHSPAdd {
		t.Fatalf("diagnostics last command = %+v, want HSP add", diagnostics.LastCommand)
	}
	if diagnostics.CommandCount != 1 {
		t.Fatalf("command count = %d, want 1", diagnostics.CommandCount)
	}
}

func TestBrowserBluetoothTransportReportsStaleBridgeWithoutFallback(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	bridge := NewBrowserBluetoothBridge(
		WithBrowserBluetoothClock(func() time.Time { return now }),
		WithBrowserBluetoothStaleAfter(100*time.Millisecond),
	)
	connected := true
	bridge.ConnectClient(BrowserBluetoothClientStatus{
		ClientID:  "client-1",
		Connected: &connected,
	})
	now = now.Add(time.Second)
	bluetooth := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})

	result, err := bluetooth.Stop(context.Background(), StopCommand{Reason: "test"})
	if err == nil {
		t.Fatal("Stop succeeded with stale browser bridge")
	}
	var bluetoothErr BrowserBluetoothError
	if !errors.As(err, &bluetoothErr) {
		t.Fatalf("error = %T %[1]v, want BrowserBluetoothError", err)
	}
	if bluetoothErr.Status != "bridge_stale" {
		t.Fatalf("status = %q, want bridge_stale", bluetoothErr.Status)
	}
	if result.OK || result.Transport != BrowserBluetoothName {
		t.Fatalf("result = %+v, want browser Bluetooth failure", result)
	}
}

func TestBrowserBluetoothTransportDistinguishesDeviceFailure(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	connected := true
	bridge.ConnectClient(BrowserBluetoothClientStatus{
		ClientID:  "client-1",
		Connected: &connected,
	})
	bluetooth := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})

	done := make(chan resultAndError, 1)
	go func() {
		result, err := bluetooth.Stop(context.Background(), StopCommand{Reason: "manual"})
		done <- resultAndError{result: result, err: err}
	}()

	commands, err := bridge.NextCommands(context.Background(), "client-1", time.Second)
	if err != nil {
		t.Fatalf("NextCommands: %v", err)
	}
	if len(commands) != 1 || commands[0].Path != "hsp/stop" {
		t.Fatalf("commands = %+v, want one HSP stop", commands)
	}
	bridge.Acknowledge("client-1", BrowserBluetoothBridgeAck{
		ID:     commands[0].ID,
		OK:     false,
		Status: "device_error",
		Error:  "GATT write failed",
	})

	outcome := readBluetoothOutcome(t, done)
	if outcome.err == nil {
		t.Fatal("Stop succeeded after device failure ack")
	}
	if outcome.result.Status != "device_error" {
		t.Fatalf("status = %q, want device_error", outcome.result.Status)
	}
	if diagnostics := bluetooth.Diagnostics(); diagnostics.LastResult == nil || diagnostics.LastResult.Status != "device_error" {
		t.Fatalf("diagnostics = %+v, want device_error last result", diagnostics)
	}
}

func TestBrowserBluetoothTransportMapsSemanticStreamID(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	connected := true
	bridge.ConnectClient(BrowserBluetoothClientStatus{
		ClientID:  "client-1",
		Connected: &connected,
	})
	bluetooth := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})

	done := make(chan resultAndError, 1)
	go func() {
		result, err := bluetooth.AddHSP(context.Background(), HSPAddCommand{
			StreamID: "motion-000001",
			Points:   []TimedPoint{{PositionPercent: 50, TimeMillis: 0}},
		})
		done <- resultAndError{result: result, err: err}
	}()

	commands, err := bridge.NextCommands(context.Background(), "client-1", time.Second)
	if err != nil {
		t.Fatalf("NextCommands: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(commands))
	}
	if commands[0].Path != "hsp/add" || commands[0].Body["stream_id"] != 1 {
		t.Fatalf("command = %+v, want semantic stream mapped to numeric 1", commands[0])
	}
	bridge.Acknowledge("client-1", BrowserBluetoothBridgeAck{
		ID: commands[0].ID,
		OK: true,
	})

	outcome := readBluetoothOutcome(t, done)
	if outcome.err != nil {
		t.Fatalf("AddHSP: %v", outcome.err)
	}
	diagnostics := bluetooth.Diagnostics()
	if diagnostics.LastCommand == nil ||
		diagnostics.LastCommand.HSPAdd == nil ||
		diagnostics.LastCommand.HSPAdd.StreamID != "motion-000001" {
		t.Fatalf("diagnostics last command = %+v, want semantic stream id retained", diagnostics.LastCommand)
	}
}

func TestBrowserBluetoothTransportReusesMappedStreamIDForPlay(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	connected := true
	bridge.ConnectClient(BrowserBluetoothClientStatus{
		ClientID:  "client-1",
		Connected: &connected,
	})
	bluetooth := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})

	addDone := make(chan resultAndError, 1)
	go func() {
		result, err := bluetooth.AddHSP(context.Background(), HSPAddCommand{
			StreamID: "motion-000001",
			Points:   []TimedPoint{{PositionPercent: 50, TimeMillis: 0}},
		})
		addDone <- resultAndError{result: result, err: err}
	}()
	addCommands, err := bridge.NextCommands(context.Background(), "client-1", time.Second)
	if err != nil {
		t.Fatalf("NextCommands add: %v", err)
	}
	if len(addCommands) != 1 || addCommands[0].Body["stream_id"] != 1 {
		t.Fatalf("add commands = %+v, want numeric stream 1", addCommands)
	}
	bridge.Acknowledge("client-1", BrowserBluetoothBridgeAck{ID: addCommands[0].ID, OK: true})
	if outcome := readBluetoothOutcome(t, addDone); outcome.err != nil {
		t.Fatalf("AddHSP: %v", outcome.err)
	}

	playDone := make(chan resultAndError, 1)
	go func() {
		result, err := bluetooth.PlayHSP(context.Background(), HSPPlayCommand{StreamID: "motion-000001"})
		playDone <- resultAndError{result: result, err: err}
	}()
	playCommands, err := bridge.NextCommands(context.Background(), "client-1", time.Second)
	if err != nil {
		t.Fatalf("NextCommands play: %v", err)
	}
	if len(playCommands) != 1 || playCommands[0].Path != "hsp/play" || playCommands[0].Body["stream_id"] != 1 {
		t.Fatalf("play commands = %+v, want same numeric stream 1", playCommands)
	}
	bridge.Acknowledge("client-1", BrowserBluetoothBridgeAck{ID: playCommands[0].ID, OK: true})
	if outcome := readBluetoothOutcome(t, playDone); outcome.err != nil {
		t.Fatalf("PlayHSP: %v", outcome.err)
	}
}

func TestBrowserBluetoothConnectionCheckUsesBridgeReadinessWithoutStateProbe(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	connected := true
	bridge.ConnectClient(BrowserBluetoothClientStatus{
		ClientID:   "client-1",
		Connected:  &connected,
		DeviceName: "Handy",
		Protocol:   "hsp_ble",
		Status:     "connected",
	})
	bluetooth := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})

	check, err := bluetooth.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if !check.OK || !check.HSPAvailable || check.Status != "connected" {
		t.Fatalf("check = %+v, want ready bridge check", check)
	}
	commands, err := bridge.NextCommands(context.Background(), "client-1", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NextCommands: %v", err)
	}
	if len(commands) != 0 {
		t.Fatalf("commands = %+v, want connection check to avoid hsp/state probe", commands)
	}
	if diagnostics := bluetooth.Diagnostics(); diagnostics.CommandCount != 0 {
		t.Fatalf("diagnostics command count = %d, want no command", diagnostics.CommandCount)
	}
}

type resultAndError struct {
	result CommandResult
	err    error
}

func newTestBrowserBluetoothTransport(t *testing.T, bridge *BrowserBluetoothBridge, options BrowserBluetoothOptions) *BrowserBluetoothTransport {
	t.Helper()

	bluetooth, err := NewBrowserBluetoothTransport(bridge, options)
	if err != nil {
		t.Fatalf("NewBrowserBluetoothTransport: %v", err)
	}
	return bluetooth
}

func readBluetoothOutcome(t *testing.T, done <-chan resultAndError) resultAndError {
	t.Helper()

	select {
	case outcome := <-done:
		return outcome
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Bluetooth transport result")
		return resultAndError{}
	}
}
