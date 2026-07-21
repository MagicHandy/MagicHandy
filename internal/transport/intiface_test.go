package transport

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestIntifaceHandshakeScanningAndSelection(t *testing.T) {
	server := newFakeButtplugServer(t, 200)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)

	history := server.waitForCount(t, 2)
	if history[0].kind != "RequestServerInfo" || history[1].kind != "RequestDeviceList" {
		t.Fatalf("handshake order = %s, %s", history[0].kind, history[1].kind)
	}
	if history[0].number("MessageVersion") != buttplugMessageVersion {
		t.Fatalf("MessageVersion = %v", history[0].fields["MessageVersion"])
	}
	if history[0].text("ClientName") != defaultIntifaceClientName {
		t.Fatalf("ClientName = %q", history[0].text("ClientName"))
	}

	status := owner.Status()
	if !status.Connected || status.MaxPingTimeMillis != 200 || len(status.Devices) != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatalf("SelectDevice: %v", err)
	}
	status = owner.Status()
	if status.SelectedDeviceIndex == nil || *status.SelectedDeviceIndex != 7 {
		t.Fatalf("selected device = %+v", status.SelectedDeviceIndex)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := owner.StartScanning(ctx); err != nil {
		t.Fatalf("StartScanning: %v", err)
	}
	if !owner.Status().Scanning {
		t.Fatal("scanning did not become true")
	}
	if err := owner.StopScanning(ctx); err != nil {
		t.Fatalf("StopScanning: %v", err)
	}
	if owner.Status().Scanning {
		t.Fatal("scanning did not become false")
	}
	added := fakeLinearDevice(8, "Added Linear")
	added["Id"] = 0
	server.sendEvent(t, "DeviceAdded", added)
	waitForTest(t, time.Second, func() bool { return len(owner.Devices()) == 2 })
}

func TestIntifaceLinearPrecisionWindowReverseAndCrossChunk(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if _, err := owner.SetStrokeWindow(ctx, StrokeWindowCommand{MinPercent: 20, MaxPercent: 80, ReverseDirection: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "precision",
		Points: []TimedPoint{
			{PositionPercent: 12.345678, TimeMillis: 0},
			{PositionPercent: 25.123456, TimeMillis: 100},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "precision",
		Points:   []TimedPoint{{PositionPercent: 75.555555, TimeMillis: 200}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "precision"}); err != nil {
		t.Fatal(err)
	}

	first := server.waitForKind(t, "LinearCmd", time.Second)
	second := server.waitForKind(t, "LinearCmd", time.Second)
	assertLinearVectorDurationRange(t, first, 0, 75, 100, 0.649259264)
	assertLinearVectorDurationRange(t, second, 0, 75, 100, 0.34666667)
	if _, err := owner.Stop(ctx, StopCommand{Reason: "test"}); err != nil {
		t.Fatal(err)
	}
}

func TestIntifaceStopPreemptsQueuedLinearCommands(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "stop-order",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 100},
			{PositionPercent: 100, TimeMillis: 200},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "stop-order"}); err != nil {
		t.Fatal(err)
	}
	server.waitForAnyLinear(t, time.Second)
	if _, err := owner.Stop(ctx, StopCommand{Reason: "preempt"}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	time.Sleep(150 * time.Millisecond)

	history := server.historySnapshot()
	stopIndex := -1
	for index, message := range history {
		if message.kind == "StopDeviceCmd" {
			stopIndex = index
			continue
		}
		if stopIndex >= 0 && message.kind == "LinearCmd" {
			t.Fatalf("LinearCmd at history index %d followed StopDeviceCmd at %d", index, stopIndex)
		}
	}
}

func TestIntifaceCanceledQueuedWriteKeepsConnectionForStop(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	owner.writeMu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			owner.writeMu.Unlock()
		}
	}()
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	requestDone := make(chan error, 1)
	go func() {
		_, err := owner.request(requestCtx, "LinearCmd", map[string]any{
			"DeviceIndex": 7,
			"Vectors": []map[string]any{{
				"Index": 0, "Duration": 100, "Position": 0.5,
			}},
		})
		requestDone <- err
	}()
	waitForTest(t, time.Second, func() bool {
		owner.mu.Lock()
		defer owner.mu.Unlock()
		return len(owner.waiters) > 0
	})
	cancelRequest()
	owner.writeMu.Unlock()
	writeLocked = false

	if err := <-requestDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled LinearCmd = %v, want context canceled", err)
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), time.Second)
	defer cancelStop()
	if _, err := owner.Stop(stopCtx, StopCommand{Reason: "canceled_write"}); err != nil {
		t.Fatalf("Stop after canceled write: %v", err)
	}
	server.waitForKind(t, "StopDeviceCmd", time.Second)
}

func TestIntifaceClosePreemptsQueuedLinearCommands(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "close-order",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 100},
			{PositionPercent: 100, TimeMillis: 200},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "close-order"}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "LinearCmd", time.Second)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	diagnostics := owner.Diagnostics()
	if diagnostics.LastResult == nil || diagnostics.LastResult.Kind != CommandKindStop || !diagnostics.LastResult.OK {
		t.Fatalf("close diagnostics = %+v, want successful Stop", diagnostics)
	}
	time.Sleep(150 * time.Millisecond)
	history := server.historySnapshot()
	stopIndex := -1
	for index, message := range history {
		if message.kind == "StopDeviceCmd" {
			stopIndex = index
			continue
		}
		if stopIndex >= 0 && message.kind == "LinearCmd" {
			t.Fatalf("LinearCmd at history index %d followed close StopDeviceCmd at %d", index, stopIndex)
		}
	}
	if stopIndex < 0 {
		t.Fatalf("close did not send StopDeviceCmd; history: %+v", history)
	}
}

func TestIntifaceCloseRejectsMotionWaitingBehindFinalStop(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	stopACK := make(chan struct{})
	server.stopACKHold = stopACK
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- owner.Close() }()
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	appendDone := make(chan error, 1)
	go func() {
		_, err := owner.AppendPoints(context.Background(), AppendPointsCommand{
			StreamID: "after-close",
			Points: []TimedPoint{
				{PositionPercent: 0, TimeMillis: 0},
				{PositionPercent: 100, TimeMillis: 100},
			},
		})
		appendDone <- err
	}()
	playDone := make(chan error, 1)
	go func() {
		_, err := owner.Play(context.Background(), PlayCommand{StreamID: "after-close"})
		playDone <- err
	}()
	close(stopACK)
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-appendDone; err == nil {
		t.Fatal("AppendPoints succeeded after Close began")
	}
	if err := <-playDone; err == nil {
		t.Fatal("Play succeeded after Close began")
	}
	if err := owner.StartScanning(context.Background()); err == nil {
		t.Fatal("StartScanning succeeded after Close")
	}
	if owner.Status().Scanning {
		t.Fatal("closed owner still reports active scanning")
	}
	time.Sleep(50 * time.Millisecond)
	history := server.historySnapshot()
	stopIndex := -1
	for index, message := range history {
		if message.kind == "StopDeviceCmd" {
			stopIndex = index
		} else if stopIndex >= 0 && message.kind == "LinearCmd" {
			t.Fatalf("LinearCmd at history index %d followed final StopDeviceCmd at %d", index, stopIndex)
		}
	}
}

func TestIntifaceDeviceRemovalCancelsPacer(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "removed",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 100},
			{PositionPercent: 100, TimeMillis: 300},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "removed"}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "LinearCmd", time.Second)
	beforeRemoval := len(server.historySnapshot())
	server.sendEvent(t, "DeviceRemoved", map[string]any{"Id": 0, "DeviceIndex": 7})
	waitForTest(t, time.Second, func() bool { return owner.Status().PlaybackState == "stale" })
	time.Sleep(100 * time.Millisecond)
	for _, message := range server.historySnapshot()[beforeRemoval:] {
		if message.kind == "LinearCmd" && !message.anchor {
			t.Fatal("pacer sent LinearCmd after selected device removal")
		}
	}
}

func TestIntifacePingFailureMarksConnectionStale(t *testing.T) {
	server := newFakeButtplugServer(t, 60)
	server.ackPing = false
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)

	waitForTest(t, time.Second, func() bool {
		status := owner.Status()
		return !status.Connected && status.PlaybackState == "stale"
	})
	server.waitForKind(t, "Ping", time.Second)
}

func TestIntifaceQueueUnderrunReportsStarvedAndStopsDevice(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "starve",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 20},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "starve"}); err != nil {
		t.Fatal(err)
	}
	server.waitForAnyLinear(t, time.Second)
	waitForTest(t, time.Second, func() bool { return owner.Status().PlaybackState == "starved" })
	server.waitForKind(t, "StopDeviceCmd", time.Second)
}

func TestIntifaceLinearFailureCancelsPlaybackAndStopsDevice(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	server.setLinearACKPlans(fakeLinearACK{kind: "Ok"}, fakeLinearACK{kind: "Error"})
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "rejected",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 20},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "rejected"}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "LinearCmd", time.Second)
	waitForTest(t, time.Second, func() bool { return owner.Status().PlaybackState == "rejected" })
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	status := owner.Status()
	if status.LinearRejectedCount != 1 || status.LastPacerFailure != "ack_rejected" {
		t.Fatalf("rejected ACK telemetry = %+v", status)
	}
}

func TestIntifaceRejectsInvalidPointsAndBoundedQueue(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	defer server.Close()
	owner := connectTestIntiface(t, server, 1)
	defer closeTestIntiface(t, owner)

	tests := []AppendPointsCommand{
		{StreamID: "bad", Points: []TimedPoint{{PositionPercent: math.NaN(), TimeMillis: 0}}},
		{StreamID: "bad", Points: []TimedPoint{{PositionPercent: 0, TimeMillis: 10}, {PositionPercent: 1, TimeMillis: 10}}},
		{StreamID: "bad", Points: []TimedPoint{{PositionPercent: 0, TimeMillis: -1}}},
	}
	for _, command := range tests {
		result, err := owner.AppendPoints(context.Background(), command)
		if err == nil || result.Status != "rejected" {
			t.Fatalf("AppendPoints(%+v) = %+v, %v", command, result, err)
		}
	}
	result, err := owner.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "full",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 10},
			{PositionPercent: 100, TimeMillis: 20},
		},
	})
	if err == nil || result.Status != "rejected" {
		t.Fatalf("queue overflow = %+v, %v", result, err)
	}
}

func TestIntifaceDelayedACKDoesNotBlockScheduledWrite(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	firstACK := make(chan struct{})
	server.setLinearACKPlans(
		fakeLinearACK{kind: "Ok"},
		fakeLinearACK{kind: "Ok", hold: firstACK},
		fakeLinearACK{kind: "Ok"},
	)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "async-acks",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 80},
			{PositionPercent: 100, TimeMillis: 160},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "async-acks"}); err != nil {
		t.Fatal(err)
	}
	anchor := server.waitForAnyLinear(t, time.Second)
	if !anchor.anchor {
		t.Fatal("first LinearCmd was not the startup anchor")
	}
	assertLinearVector(t, anchor, 0, 250, 0)

	first := server.waitForKind(t, "LinearCmd", time.Second)
	second := server.waitForKind(t, "LinearCmd", time.Second)
	if elapsed := first.receivedAt.Sub(anchor.receivedAt); elapsed < 200*time.Millisecond {
		t.Fatalf("first scheduled write followed anchor after %v, want completed startup anchor", elapsed)
	}
	if elapsed := second.receivedAt.Sub(first.receivedAt); elapsed < 40*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("second scheduled write delay = %v, want approximately 80ms", elapsed)
	}
	waitForTest(t, time.Second, func() bool { return owner.Status().LinearACKedCount == 2 })
	status := owner.Status()
	if status.PendingACKs != 1 {
		t.Fatalf("pending ACKs = %d, want held first ACK only; status: %+v", status.PendingACKs, status)
	}
	if status.LinearSentCount != 3 || status.LinearACKedCount != 2 {
		t.Fatalf("async ACK telemetry before release = %+v", status)
	}
	if status.QueueCoverageMillis <= 0 {
		t.Fatalf("active queue coverage = %dms, want positive", status.QueueCoverageMillis)
	}
	close(firstACK)
	waitForTest(t, time.Second, func() bool { return owner.Status().PendingACKs == 0 })
	status = owner.Status()
	if status.MaxACKLatencyMillis < 70 {
		t.Fatalf("max ACK latency = %dms, want held ACK latency", status.MaxACKLatencyMillis)
	}
}

func TestIntifaceACKTimeoutStopsAndCleansWaiter(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	server.setLinearACKPlans(fakeLinearACK{kind: "Ok"}, fakeLinearACK{dropped: true})
	defer server.Close()
	owner := connectTestIntifaceWithTimeout(t, server, 32, 50*time.Millisecond)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "timeout",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 200},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "timeout"}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "LinearCmd", time.Second)
	waitForTest(t, time.Second, func() bool {
		status := owner.Status()
		return status.PlaybackState == "stale" && status.LastPacerFailure == "ack_timeout"
	})
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	waitForTest(t, time.Second, func() bool { return owner.Status().PendingACKs == 0 })
	status := owner.Status()
	if status.LinearTimeoutCount != 1 || status.LinearRejectedCount != 0 || status.PendingACKs != 0 {
		t.Fatalf("timeout telemetry = %+v", status)
	}
	if got := status.RecentDispatches[len(status.RecentDispatches)-1].Status; got != "timeout" {
		t.Fatalf("timed-out dispatch status = %q", got)
	}
	owner.mu.Lock()
	waiters := len(owner.waiters)
	owner.mu.Unlock()
	if waiters != 0 {
		t.Fatalf("waiters after timeout recovery = %d", waiters)
	}
}

func TestIntifaceStopWithPendingACKPreventsLaterLinearAndIsolatesLateResponse(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	lateACK := make(chan struct{})
	server.setLinearACKPlans(
		fakeLinearACK{kind: "Ok"},
		fakeLinearACK{kind: "Error", hold: lateACK},
		fakeLinearACK{kind: "Ok"},
	)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "pending-stop",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 50, TimeMillis: 100},
			{PositionPercent: 100, TimeMillis: 200},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "pending-stop"}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "LinearCmd", time.Second)
	if _, err := owner.Stop(ctx, StopCommand{Reason: "pending_ack"}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	close(lateACK)
	time.Sleep(150 * time.Millisecond)

	history := server.historySnapshot()
	stopIndex := -1
	for index, message := range history {
		if message.kind == "StopDeviceCmd" {
			stopIndex = index
			continue
		}
		if stopIndex >= 0 && message.kind == "LinearCmd" {
			t.Fatalf("LinearCmd at history index %d followed StopDeviceCmd at %d", index, stopIndex)
		}
	}
	if stopIndex < 0 {
		t.Fatal("StopDeviceCmd was not received")
	}
	waitForTest(t, time.Second, func() bool { return owner.Status().PendingACKs == 0 })
	if state := owner.Status().PlaybackState; state != "idle" {
		t.Fatalf("late rejected ACK changed stopped playback state to %q", state)
	}
}

func TestIntifaceStopDuringStartupAnchor(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	anchorACK := make(chan struct{})
	server.setLinearACKPlans(fakeLinearACK{kind: "Ok", hold: anchorACK})
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	_, _ = owner.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "stop-anchor",
		Points: []TimedPoint{
			{PositionPercent: 10, TimeMillis: 0},
			{PositionPercent: 90, TimeMillis: 100},
		},
	})
	playDone := make(chan error, 1)
	go func() {
		_, err := owner.Play(context.Background(), PlayCommand{StreamID: "stop-anchor"})
		playDone <- err
	}()
	anchor := server.waitForAnyLinear(t, time.Second)
	assertLinearVector(t, anchor, 0, 250, 0.1)
	if state := owner.Status().PlaybackState; state != "anchoring" {
		t.Fatalf("playback state during held anchor = %q", state)
	}
	if _, err := owner.Stop(context.Background(), StopCommand{Reason: "anchor"}); err != nil {
		t.Fatal(err)
	}
	if err := <-playDone; !errors.Is(err, errPacerSuperseded) {
		t.Fatalf("Play interrupted by Stop = %v, want pacer superseded", err)
	}
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	stopIndex := len(server.historySnapshot()) - 1
	close(anchorACK)
	time.Sleep(300 * time.Millisecond)
	for index, message := range server.historySnapshot()[stopIndex+1:] {
		if message.kind == "LinearCmd" {
			t.Fatalf("LinearCmd at post-Stop offset %d during startup anchoring", index)
		}
	}
	waitForTest(t, time.Second, func() bool { return owner.Status().PendingACKs == 0 })
}

func TestIntifaceStopBarrierBlocksAppendAndPlay(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	stopACK := make(chan struct{})
	server.stopACKHold = stopACK
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	_, _ = owner.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "before-stop",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 100},
		},
	})

	stopDone := make(chan error, 1)
	go func() {
		_, err := owner.Stop(context.Background(), StopCommand{Reason: "barrier"})
		stopDone <- err
	}()
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	appendDone := make(chan struct{})
	go func() {
		_, _ = owner.AppendPoints(context.Background(), AppendPointsCommand{
			StreamID: "after-stop",
			Points: []TimedPoint{
				{PositionPercent: 0, TimeMillis: 0},
				{PositionPercent: 100, TimeMillis: 100},
			},
		})
		close(appendDone)
	}()
	playDone := make(chan struct{})
	go func() {
		_, _ = owner.Play(context.Background(), PlayCommand{StreamID: "before-stop"})
		close(playDone)
	}()
	assertChannelBlocked(t, appendDone, 50*time.Millisecond, "AppendPoints")
	assertChannelBlocked(t, playDone, 50*time.Millisecond, "Play")
	close(stopACK)
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-appendDone:
	case <-time.After(time.Second):
		t.Fatal("AppendPoints remained blocked after Stop completed")
	}
	select {
	case <-playDone:
	case <-time.After(time.Second):
		t.Fatal("Play remained blocked after Stop completed")
	}
}

func TestIntifaceReverseIsSnapshottedAtAppend(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = owner.SetStrokeWindow(ctx, StrokeWindowCommand{MinPercent: 0, MaxPercent: 100})
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "reverse-snapshot",
		Points: []TimedPoint{
			{PositionPercent: 10, TimeMillis: 0},
			{PositionPercent: 20, TimeMillis: 80},
		},
	})
	_, _ = owner.SetStrokeWindow(ctx, StrokeWindowCommand{MinPercent: 0, MaxPercent: 100, ReverseDirection: true})
	_, _ = owner.AppendPoints(ctx, AppendPointsCommand{
		StreamID: "reverse-snapshot",
		Points:   []TimedPoint{{PositionPercent: 30, TimeMillis: 160}},
	})
	_, _ = owner.SetStrokeWindow(ctx, StrokeWindowCommand{MinPercent: 0, MaxPercent: 100})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "reverse-snapshot"}); err != nil {
		t.Fatal(err)
	}
	anchor := server.waitForAnyLinear(t, time.Second)
	first := server.waitForKind(t, "LinearCmd", time.Second)
	second := server.waitForKind(t, "LinearCmd", time.Second)
	assertLinearVector(t, anchor, 0, 250, 0.1)
	// Live pacing may compress a segment by up to one quarter before rejecting it.
	assertLinearVectorDurationRange(t, first, 0, 60, 80, 0.2)
	assertLinearVectorDurationRange(t, second, 0, 60, 80, 0.7)
}

func TestIntifaceRejectsSelectionChangeDuringPlayback(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	server.sendEvent(t, "DeviceAdded", fakeLinearDevice(8, "Second Linear"))
	waitForTest(t, time.Second, func() bool { return len(owner.Devices()) == 2 })
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	_, _ = owner.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "selection-lock",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 500},
		},
	})
	if _, err := owner.Play(context.Background(), PlayCommand{StreamID: "selection-lock"}); err != nil {
		t.Fatal(err)
	}
	if err := owner.SelectDevice(8, 0); err == nil {
		t.Fatal("SelectDevice changed the actuator during playback")
	}
	status := owner.Status()
	if status.SelectedDeviceIndex == nil || *status.SelectedDeviceIndex != 7 {
		t.Fatalf("selected device changed during playback: %+v", status.SelectedDeviceIndex)
	}
}

func TestIntifaceTimingGapStepCountAndResolution(t *testing.T) {
	fractionalSelection := intifaceSelection{timingGap: 333 * time.Millisecond}
	if interval := fractionalSelection.minimumPointInterval(); interval != 367*time.Millisecond {
		t.Fatalf("fractional timing floor = %v, want millisecond-safe 367ms", interval)
	}
	server := newFakeButtplugServer(t, 0)
	server.timingGap = 300
	server.stepCount = 200
	owner := connectTestIntiface(t, server, 32)
	status := owner.Status()
	if len(status.Devices) != 1 || status.Devices[0].DeviceMessageTimingGapMillis != 300 {
		t.Fatalf("timing gap snapshot = %+v", status.Devices)
	}
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	if resolution := owner.Status().SelectedResolutionPercent; math.Abs(resolution-0.5) > 1e-12 {
		t.Fatalf("selected resolution = %v, want 0.5", resolution)
	}
	sampling := owner.MotionSamplingCapabilities()
	if math.Abs(sampling.PositionResolutionPercent-0.5) > 1e-12 || !sampling.ResolutionAfterStrokeWindow {
		t.Fatalf("sampling capabilities = %+v, want physical 0.5%% resolution", sampling)
	}
	result, err := owner.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "too-fast",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 20},
		},
	})
	if err == nil || result.Status != "rejected" {
		t.Fatalf("sub-gap append = %+v, %v", result, err)
	}
	if _, err := owner.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "gap-anchor",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 330},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Play(context.Background(), PlayCommand{StreamID: "gap-anchor"}); err != nil {
		t.Fatal(err)
	}
	assertLinearVector(t, server.waitForAnyLinear(t, time.Second), 0, 300, 0)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	server.Close()

	zeroServer := newFakeButtplugServer(t, 0)
	zeroServer.stepCount = 0
	defer zeroServer.Close()
	zeroOwner := connectTestIntiface(t, zeroServer, 32)
	defer closeTestIntiface(t, zeroOwner)
	if err := zeroOwner.SelectDevice(7, 0); err == nil {
		t.Fatal("SelectDevice accepted a zero-StepCount actuator")
	}
}

func TestIntifacePaceDecision(t *testing.T) {
	base := time.Now()
	queue := []intifaceSegment{
		{startMillis: 0, duration: 50, position: 10},
		{startMillis: 50, duration: 100, position: 20},
	}
	decision := decideIntifacePace(base.Add(60*time.Millisecond), base, queue)
	if decision.coalesced != 1 || decision.segment != queue[1] || decision.failure != "" {
		t.Fatalf("coalescing decision = %+v", decision)
	}
	lateness, duration, err := decideIntifaceLiveDuration(base.Add(60*time.Millisecond), base.Add(50*time.Millisecond), base.Add(150*time.Millisecond), 100*time.Millisecond, 0)
	if err != nil || lateness != 10*time.Millisecond || duration != 90*time.Millisecond {
		t.Fatalf("live duration decision = %v, %v, %v", lateness, duration, err)
	}
	_, _, err = decideIntifaceLiveDuration(base.Add(76*time.Millisecond), base, base.Add(100*time.Millisecond), 100*time.Millisecond, 0)
	assertPacerCategory(t, err, "duration_compression")
	_, _, err = decideIntifaceLiveDuration(base.Add(101*time.Millisecond), base, base.Add(200*time.Millisecond), 200*time.Millisecond, 0)
	assertPacerCategory(t, err, "live_late")
	_, _, err = decideIntifaceLiveDuration(base.Add(5*time.Millisecond), base, base.Add(100*time.Millisecond), 100*time.Millisecond, 100*time.Millisecond)
	assertPacerCategory(t, err, "timing_gap")
	latest, err := latestIntifaceWriteTime(intifacePacedRequest{
		enforceSchedule: true, scheduledAt: base, scheduledEnd: base.Add(125 * time.Millisecond),
		originalDuration: 125 * time.Millisecond, minimumDuration: 100 * time.Millisecond,
	})
	if err != nil || latest.Sub(base) != 25*time.Millisecond {
		t.Fatalf("latest safe write = %v, %v; want 25ms message-gap margin", latest.Sub(base), err)
	}
}

func TestIntifacePaceTelemetry(t *testing.T) {
	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	owner := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, owner)
	if err := owner.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	_, _ = owner.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "coalesce",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 10, TimeMillis: 20},
			{PositionPercent: 50, TimeMillis: 500},
		},
	})
	if _, err := owner.Play(context.Background(), PlayCommand{StreamID: "coalesce", StartTimeMillis: 30}); err != nil {
		t.Fatal(err)
	}
	server.waitForKind(t, "LinearCmd", time.Second)
	waitForTest(t, time.Second, func() bool {
		status := owner.Status()
		return status.LinearACKedCount == 2 && status.CoalescedSegments == 1
	})
	status := owner.Status()
	if status.LastWireDurationMillis < 350 || status.LastWireDurationMillis > 480 {
		t.Fatalf("effective duration telemetry = %dms", status.LastWireDurationMillis)
	}
	if status.LastSendLatenessMillis < 0 || status.LastSendLatenessMillis > 100 {
		t.Fatalf("send lateness telemetry = %dms", status.LastSendLatenessMillis)
	}
	if status.MaxSendLatenessMillis < status.LastSendLatenessMillis {
		t.Fatalf("max send lateness %dms is below last %dms", status.MaxSendLatenessMillis, status.LastSendLatenessMillis)
	}
	if len(status.RecentDispatches) != 2 || status.RecentDispatches[1].Status != "acked" || status.RecentDispatches[1].ACKLatencyMillis == nil {
		t.Fatalf("recent dispatch telemetry = %+v", status.RecentDispatches)
	}
	if _, err := time.Parse(time.RFC3339Nano, status.RecentDispatches[1].ActualSendTime); err != nil {
		t.Fatalf("actual UTC send time = %q: %v", status.RecentDispatches[1].ActualSendTime, err)
	}
}

func TestIntifaceEmptyQueueRecoveryRechecksConcurrentAppend(t *testing.T) {
	owner, err := NewIntiface(IntifaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owner.paceMu.Lock()
	owner.playing = true
	owner.generation = 4
	owner.playCtx = context.Background()
	owner.coverageEnd = time.Now().Add(-time.Millisecond)
	owner.queue = []intifaceSegment{{startMillis: 100, duration: 100, position: 50}}
	owner.paceMu.Unlock()
	owner.handleEmptyPacerQueue(context.Background(), 4, time.Now().Add(-time.Millisecond))
	owner.paceMu.Lock()
	playing := owner.playing
	queueDepth := len(owner.queue)
	owner.paceMu.Unlock()
	if !playing || queueDepth != 1 {
		t.Fatalf("concurrent append was discarded: playing=%v queue=%d", playing, queueDepth)
	}
}

func TestIntifaceRecentDispatchTelemetryReportsTruncation(t *testing.T) {
	owner, err := NewIntiface(IntifaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	selection := intifaceSelection{deviceIndex: 7, actuatorIndex: 0}
	for index := 0; index < maxIntifaceRecentDispatches+3; index++ {
		owner.recordLinearSent(intifaceRequest{
			id: uint32(index + 1), writtenAt: time.Now(), wireDuration: 100 * time.Millisecond,
		}, int64(index*100), selection, false)
	}
	status := owner.Status()
	if len(status.RecentDispatches) != maxIntifaceRecentDispatches || status.RecentDispatchesDropped != 3 || status.LinearSentCount != maxIntifaceRecentDispatches+3 {
		t.Fatalf("truncated dispatch telemetry = %+v", status)
	}
}

func TestIntifaceDefaultQueueAndPendingACKBounds(t *testing.T) {
	owner, err := NewIntiface(IntifaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if owner.options.QueueCapacity != 64 {
		t.Fatalf("default queue capacity = %d, want 64", owner.options.QueueCapacity)
	}

	server := newFakeButtplugServer(t, 0)
	defer server.Close()
	connected := connectTestIntiface(t, server, 32)
	defer closeTestIntiface(t, connected)
	if err := connected.SelectDevice(7, 0); err != nil {
		t.Fatal(err)
	}
	_, _ = connected.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "pending-bound",
		Points: []TimedPoint{
			{PositionPercent: 0, TimeMillis: 0},
			{PositionPercent: 100, TimeMillis: 100},
		},
	})
	connected.paceMu.Lock()
	connected.pendingACKs = maxIntifacePendingACKs
	connected.paceMu.Unlock()
	if _, err := connected.Play(context.Background(), PlayCommand{StreamID: "pending-bound"}); err == nil {
		t.Fatal("Play succeeded after pending ACK capacity was reached")
	}
	waitForTest(t, time.Second, func() bool { return connected.Status().LastPacerFailure == "pending_ack_limit" })
	server.waitForKind(t, "StopDeviceCmd", time.Second)
	for _, message := range server.historySnapshot() {
		if message.kind == "LinearCmd" {
			t.Fatal("pacer wrote LinearCmd after pending ACK capacity was reached")
		}
	}
	connected.paceMu.Lock()
	connected.pendingACKs = 0
	connected.paceMu.Unlock()
}

func assertPacerCategory(t *testing.T, err error, want string) {
	t.Helper()
	var pacerErr *intifacePacerError
	if !errors.As(err, &pacerErr) || pacerErr.category != want {
		t.Fatalf("pacer error = %v, want category %q", err, want)
	}
}

func assertChannelBlocked(t *testing.T, channel <-chan struct{}, duration time.Duration, operation string) {
	t.Helper()
	select {
	case <-channel:
		t.Fatalf("%s completed while StopDeviceCmd was in flight", operation)
	case <-time.After(duration):
	}
}

type fakeButtplugServer struct {
	t              *testing.T
	server         *httptest.Server
	maxPing        int64
	ackPing        bool
	timingGap      uint32
	stepCount      uint32
	writeMu        sync.Mutex
	mu             sync.Mutex
	conn           *websocket.Conn
	history        []wireButtplugMessage
	received       chan wireButtplugMessage
	connected      chan struct{}
	connectOne     sync.Once
	linearACKPlans []fakeLinearACK
	linearCount    int
	stopACKHold    <-chan struct{}
	done           chan struct{}
	doneOnce       sync.Once
	wg             sync.WaitGroup
}

type wireButtplugMessage struct {
	kind       string
	fields     map[string]any
	anchor     bool
	receivedAt time.Time
}

type fakeLinearACK struct {
	kind    string
	hold    <-chan struct{}
	delay   time.Duration
	dropped bool
}

func newFakeButtplugServer(t *testing.T, maxPing int64) *fakeButtplugServer {
	t.Helper()
	fake := &fakeButtplugServer{
		t:         t,
		maxPing:   maxPing,
		ackPing:   true,
		stepCount: 10000,
		received:  make(chan wireButtplugMessage, 128),
		connected: make(chan struct{}),
		done:      make(chan struct{}),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	return fake
}

func (f *fakeButtplugServer) URL() string {
	return "ws" + strings.TrimPrefix(f.server.URL, "http")
}

func (f *fakeButtplugServer) Close() {
	f.doneOnce.Do(func() { close(f.done) })
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	if conn != nil {
		_ = conn.CloseNow()
	}
	f.server.Close()
	f.wg.Wait()
}

func (f *fakeButtplugServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()
	f.connectOne.Do(func() { close(f.connected) })
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		messages, err := decodeWireMessages(data)
		if err != nil {
			return
		}
		for _, message := range messages {
			f.mu.Lock()
			message.receivedAt = time.Now()
			switch message.kind {
			case "LinearCmd":
				message.anchor = f.linearCount == 0
				f.linearCount++
			case "StopDeviceCmd":
				f.linearCount = 0
			}
			f.history = append(f.history, message)
			f.mu.Unlock()
			// Receipt is observable before and independently from protocol ACKs.
			f.received <- message
			id, ok := message.uint32("Id")
			if !ok {
				return
			}
			switch message.kind {
			case "RequestServerInfo":
				f.write("ServerInfo", map[string]any{
					"Id": id, "ServerName": "test", "MessageVersion": 3, "MaxPingTime": f.maxPing,
				})
			case "RequestDeviceList":
				f.write("DeviceList", map[string]any{"Id": id, "Devices": []any{fakeLinearDeviceWithCapabilities(7, "Test Linear", f.timingGap, f.stepCount)}})
			case "Ping":
				if f.ackPing {
					f.write("Ok", map[string]any{"Id": id})
				}
			case "LinearCmd":
				f.respondLinear(id)
			case "StopDeviceCmd":
				f.respondStop(id)
			default:
				f.write("Ok", map[string]any{"Id": id})
			}
		}
	}
}

func (f *fakeButtplugServer) respondStop(id uint32) {
	f.mu.Lock()
	hold := f.stopACKHold
	f.mu.Unlock()
	if hold == nil {
		f.write("Ok", map[string]any{"Id": id})
		return
	}
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		select {
		case <-hold:
			f.write("Ok", map[string]any{"Id": id})
		case <-f.done:
		}
	}()
}

func (f *fakeButtplugServer) respondLinear(id uint32) {
	f.mu.Lock()
	index := f.linearCount - 1
	plan := fakeLinearACK{kind: "Ok"}
	if index >= 0 && index < len(f.linearACKPlans) {
		plan = f.linearACKPlans[index]
	}
	f.mu.Unlock()
	if plan.dropped {
		return
	}
	respond := func() {
		kind := plan.kind
		if kind == "" {
			kind = "Ok"
		}
		fields := map[string]any{"Id": id}
		if kind == "Error" {
			fields["ErrorCode"] = 1
			fields["ErrorMessage"] = "test rejection"
		}
		f.write(kind, fields)
	}
	if plan.hold == nil && plan.delay == 0 {
		respond()
		return
	}
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		if plan.hold != nil {
			select {
			case <-plan.hold:
			case <-f.done:
				return
			}
		}
		if plan.delay > 0 {
			timer := time.NewTimer(plan.delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-f.done:
				return
			}
		}
		respond()
	}()
}

func (f *fakeButtplugServer) setLinearACKPlans(plans ...fakeLinearACK) {
	f.mu.Lock()
	f.linearACKPlans = append([]fakeLinearACK(nil), plans...)
	f.mu.Unlock()
}

func (f *fakeButtplugServer) write(kind string, fields map[string]any) {
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	if conn == nil {
		return
	}
	data, _ := json.Marshal([]map[string]any{{kind: fields}})
	f.writeMu.Lock()
	_ = conn.Write(context.Background(), websocket.MessageText, data)
	f.writeMu.Unlock()
}

func (f *fakeButtplugServer) sendEvent(t *testing.T, kind string, fields map[string]any) {
	t.Helper()
	select {
	case <-f.connected:
	case <-time.After(time.Second):
		t.Fatal("fake Buttplug websocket did not connect")
	}
	f.write(kind, fields)
}

func (f *fakeButtplugServer) waitForKind(t *testing.T, kind string, timeout time.Duration) wireButtplugMessage {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case message := <-f.received:
			if message.kind == kind && (kind != "LinearCmd" || !message.anchor) {
				return message
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s; history: %+v", kind, f.historySnapshot())
		}
	}
}

func (f *fakeButtplugServer) waitForAnyLinear(t *testing.T, timeout time.Duration) wireButtplugMessage {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case message := <-f.received:
			if message.kind == "LinearCmd" {
				return message
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for LinearCmd; history: %+v", f.historySnapshot())
		}
	}
}

func (f *fakeButtplugServer) waitForCount(t *testing.T, count int) []wireButtplugMessage {
	t.Helper()
	waitForTest(t, time.Second, func() bool { return len(f.historySnapshot()) >= count })
	return f.historySnapshot()
}

func (f *fakeButtplugServer) historySnapshot() []wireButtplugMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]wireButtplugMessage(nil), f.history...)
}

func decodeWireMessages(data []byte) ([]wireButtplugMessage, error) {
	var envelopes []map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelopes); err != nil {
		return nil, err
	}
	messages := make([]wireButtplugMessage, 0, len(envelopes))
	for _, envelope := range envelopes {
		for kind, raw := range envelope {
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, err
			}
			messages = append(messages, wireButtplugMessage{kind: kind, fields: fields})
		}
	}
	return messages, nil
}

func (m wireButtplugMessage) number(key string) int {
	value, _ := m.fields[key].(float64)
	return int(value)
}

func (m wireButtplugMessage) uint32(key string) (uint32, bool) {
	value, ok := m.fields[key].(float64)
	if !ok || value < 0 || value > float64(^uint32(0)) || value != math.Trunc(value) {
		return 0, false
	}
	return uint32(value), true
}

func (m wireButtplugMessage) text(key string) string {
	value, _ := m.fields[key].(string)
	return value
}

func fakeLinearDevice(index uint32, name string) map[string]any {
	return fakeLinearDeviceWithCapabilities(index, name, 0, 10000)
}

func fakeLinearDeviceWithCapabilities(index uint32, name string, timingGap, stepCount uint32) map[string]any {
	return map[string]any{
		"DeviceIndex":            index,
		"DeviceName":             name,
		"DeviceMessageTimingGap": timingGap,
		"DeviceMessages": map[string]any{
			"LinearCmd": []map[string]any{{
				"FeatureDescriptor": "Position", "ActuatorType": "Position", "StepCount": stepCount,
			}},
		},
	}
}

func connectTestIntiface(t *testing.T, server *fakeButtplugServer, capacity int) *Intiface {
	return connectTestIntifaceWithTimeout(t, server, capacity, defaultIntifaceResponseTime)
}

func connectTestIntifaceWithTimeout(t *testing.T, server *fakeButtplugServer, capacity int, responseTimeout time.Duration) *Intiface {
	t.Helper()
	owner, err := NewIntiface(IntifaceOptions{Address: server.URL(), QueueCapacity: capacity})
	if err != nil {
		t.Fatal(err)
	}
	owner.responseTimeout = responseTimeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := owner.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return owner
}

func closeTestIntiface(t *testing.T, owner *Intiface) {
	t.Helper()
	if err := owner.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func assertLinearVector(t *testing.T, message wireButtplugMessage, index, duration int, position float64) {
	t.Helper()
	vectors, ok := message.fields["Vectors"].([]any)
	if !ok || len(vectors) != 1 {
		t.Fatalf("Vectors = %#v", message.fields["Vectors"])
	}
	vector, ok := vectors[0].(map[string]any)
	if !ok {
		t.Fatalf("vector = %#v", vectors[0])
	}
	if int(vector["Index"].(float64)) != index || int(vector["Duration"].(float64)) != duration {
		t.Fatalf("vector index/duration = %#v", vector)
	}
	if got := vector["Position"].(float64); math.Abs(got-position) > 1e-12 {
		t.Fatalf("Position = %.12f, want %.12f", got, position)
	}
}

func assertLinearVectorDurationRange(t *testing.T, message wireButtplugMessage, index, minimumDuration, maximumDuration int, position float64) {
	t.Helper()
	vectors, ok := message.fields["Vectors"].([]any)
	if !ok || len(vectors) != 1 {
		t.Fatalf("Vectors = %#v", message.fields["Vectors"])
	}
	vector, ok := vectors[0].(map[string]any)
	if !ok {
		t.Fatalf("vector = %#v", vectors[0])
	}
	duration := int(vector["Duration"].(float64))
	if int(vector["Index"].(float64)) != index || duration < minimumDuration || duration > maximumDuration {
		t.Fatalf("vector index/duration = %#v, want duration %d..%d", vector, minimumDuration, maximumDuration)
	}
	if got := vector["Position"].(float64); math.Abs(got-position) > 1e-12 {
		t.Fatalf("Position = %.12f, want %.12f", got, position)
	}
}

func waitForTest(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition did not become true before timeout")
}
