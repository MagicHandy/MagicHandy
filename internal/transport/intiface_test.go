package transport

import (
	"context"
	"encoding/json"
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
	assertLinearVector(t, first, 0, 100, 0.649259264)
	assertLinearVector(t, second, 0, 100, 0.34666667)
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
	server.waitForKind(t, "LinearCmd", time.Second)
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
			{PositionPercent: 0, TimeMillis: 80},
			{PositionPercent: 100, TimeMillis: 160},
		},
	})
	if _, err := owner.Play(ctx, PlayCommand{StreamID: "removed"}); err != nil {
		t.Fatal(err)
	}
	server.sendEvent(t, "DeviceRemoved", map[string]any{"Id": 0, "DeviceIndex": 7})
	waitForTest(t, time.Second, func() bool { return owner.Status().PlaybackState == "stale" })
	time.Sleep(100 * time.Millisecond)
	for _, message := range server.historySnapshot() {
		if message.kind == "LinearCmd" {
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
	server.waitForKind(t, "LinearCmd", time.Second)
	waitForTest(t, time.Second, func() bool { return owner.Status().PlaybackState == "starved" })
	server.waitForKind(t, "StopDeviceCmd", time.Second)
}

func TestIntifaceLinearFailureCancelsPlaybackAndStopsDevice(t *testing.T) {
	server := newFakeButtplugServer(t, 1000)
	server.rejectLinear = true
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

type fakeButtplugServer struct {
	t            *testing.T
	server       *httptest.Server
	maxPing      int64
	ackPing      bool
	rejectLinear bool
	writeMu      sync.Mutex
	mu           sync.Mutex
	conn         *websocket.Conn
	history      []wireButtplugMessage
	received     chan wireButtplugMessage
	connected    chan struct{}
	connectOne   sync.Once
}

type wireButtplugMessage struct {
	kind   string
	fields map[string]any
}

func newFakeButtplugServer(t *testing.T, maxPing int64) *fakeButtplugServer {
	t.Helper()
	fake := &fakeButtplugServer{
		t:         t,
		maxPing:   maxPing,
		ackPing:   true,
		received:  make(chan wireButtplugMessage, 128),
		connected: make(chan struct{}),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	return fake
}

func (f *fakeButtplugServer) URL() string {
	return "ws" + strings.TrimPrefix(f.server.URL, "http")
}

func (f *fakeButtplugServer) Close() {
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	if conn != nil {
		_ = conn.CloseNow()
	}
	f.server.Close()
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
			f.history = append(f.history, message)
			f.mu.Unlock()
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
				f.write("DeviceList", map[string]any{"Id": id, "Devices": []any{fakeLinearDevice(7, "Test Linear")}})
			case "Ping":
				if f.ackPing {
					f.write("Ok", map[string]any{"Id": id})
				}
			case "LinearCmd":
				if f.rejectLinear {
					f.write("Error", map[string]any{"Id": id, "ErrorCode": 1, "ErrorMessage": "test rejection"})
				} else {
					f.write("Ok", map[string]any{"Id": id})
				}
			default:
				f.write("Ok", map[string]any{"Id": id})
			}
			// Test waiters observe a command only after its protocol response has
			// been written, so preemption assertions do not race the fake ACK.
			f.received <- message
		}
	}
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
			if message.kind == kind {
				return message
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s; history: %+v", kind, f.historySnapshot())
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
	return map[string]any{
		"DeviceIndex": index,
		"DeviceName":  name,
		"DeviceMessages": map[string]any{
			"LinearCmd": []map[string]any{{
				"FeatureDescriptor": "Position", "ActuatorType": "Position", "StepCount": 10000,
			}},
		},
	}
}

func connectTestIntiface(t *testing.T, server *fakeButtplugServer, capacity int) *Intiface {
	t.Helper()
	owner, err := NewIntiface(IntifaceOptions{Address: server.URL(), QueueCapacity: capacity})
	if err != nil {
		t.Fatal(err)
	}
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
