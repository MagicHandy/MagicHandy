package transport

import (
	"context"
	"testing"
	"time"
)

func TestBluetoothRefillLatencyIncludesBridgeRoundTrip(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "latency"})
	owner, err := NewBrowserBluetoothTransport(bridge, BrowserBluetoothOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan CommandResult, 1)
	go func() {
		outcome, _ := owner.Play(context.Background(), PlayCommand{StreamID: "1"})
		result <- outcome
	}()
	commands, err := bridge.NextCommands(context.Background(), "latency", time.Second)
	if err != nil || len(commands) != 1 {
		t.Fatalf("commands: %v %v", commands, err)
	}
	if commands[0].Body["pause_on_starving"] != true {
		t.Fatal("BLE must explicitly pause on starvation")
	}
	time.Sleep(35 * time.Millisecond) // Delivery/HTTP delay outside browser's timer.
	bridge.Acknowledge("latency", BrowserBluetoothBridgeAck{ID: commands[0].ID, OK: true, ElapsedMillis: 1})
	if got := <-result; !got.OK || got.LatencyMillis < 30 {
		t.Fatalf("full-path result: %+v", got)
	}
	if got := owner.Diagnostics().PlaybackState; got != "play_requested" {
		t.Fatalf("write ack claimed physical playback: %q", got)
	}
	caps := owner.MotionTimingCapabilities()
	if caps.MinimumBufferedLead < time.Second || caps.MinimumMediaBufferedLead < caps.MinimumBufferedLead {
		t.Fatalf("insufficient BLE lead: %+v", caps)
	}
}

func TestBluetoothFeedbackRejectsStaleStreamsReportsAndStoppedRuns(t *testing.T) {
	bridge := NewBrowserBluetoothBridge()
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "current"})
	owner, err := NewBrowserBluetoothTransport(bridge, BrowserBluetoothOptions{})
	if err != nil {
		t.Fatal(err)
	}
	bridge.mu.Lock()
	bridge.preparePlaybackCommandLocked("hsp/play", map[string]any{"stream_id": 7}, "play-1")
	bridge.mu.Unlock()
	report := func(client string, sequence uint64, stream int, state string) {
		bridge.UpdateClient(BrowserBluetoothClientStatus{ClientID: client, HSPState: &BrowserBluetoothPlaybackState{
			Sequence: sequence, CommandID: "play-1", StreamID: stream, PlayState: state, Points: 30, CurrentTimeMillis: 500, TailPointStreamIndex: 42,
		}})
	}
	report("current", 2, 7, "playing")
	if got := owner.Diagnostics().PlaybackState; got != "playing" {
		t.Fatalf("feedback lost: %q", got)
	}
	report("current", 1, 7, "stopped")
	report("current", 3, 6, "starving")
	report("other", 4, 7, "starving")
	if got := owner.Diagnostics().PlaybackState; got != "playing" {
		t.Fatalf("stale/foreign feedback accepted: %q", got)
	}
	stateCopy := bridge.Snapshot().HSPState
	stateCopy.PlayState = "stopped"
	if bridge.Snapshot().HSPState.PlayState != "playing" {
		t.Fatal("snapshot aliases bridge state")
	}
	report("current", 5, 7, "starving")
	if got := owner.Diagnostics().PlaybackState; got != "starving" {
		t.Fatalf("starvation hidden: %q", got)
	}
	bridge.mu.Lock()
	bridge.preparePlaybackCommandLocked("hsp/stop", nil, "stop-1")
	bridge.mu.Unlock()
	report("current", 6, 7, "playing")
	if bridge.Snapshot().HSPState != nil {
		t.Fatal("late feedback revived stopped stream")
	}
	bridge.mu.Lock()
	bridge.preparePlaybackCommandLocked("hsp/play", map[string]any{"stream_id": 7}, "play-2")
	bridge.mu.Unlock()
	report("current", 7, 7, "starving")
	if bridge.Snapshot().HSPState != nil {
		t.Fatal("old HTTP report contaminated resumed playback")
	}
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "current"})
	if bridge.Snapshot().HSPState != nil {
		t.Fatal("reconnect kept old playback")
	}
}
