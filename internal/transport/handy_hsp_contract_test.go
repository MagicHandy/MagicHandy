package transport

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandyOwnersRejectInvalidPointsBeforeDispatch(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"result":{}}`))
	}))
	defer server.Close()
	cloud := newTestCloudTransport(t, server.URL)
	bridge := NewBrowserBluetoothBridge()
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "test"})
	ble := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})
	cases := [][]TimedPoint{
		nil,
		{{TimeMillis: -1, PositionPercent: 50}},
		{{TimeMillis: 1 << 32, PositionPercent: 50}},
		{{TimeMillis: 0, PositionPercent: math.NaN()}},
		{{TimeMillis: 0, PositionPercent: 50}, {TimeMillis: 0, PositionPercent: 70}},
		{{TimeMillis: 10, PositionPercent: 50}, {TimeMillis: 5, PositionPercent: 70}},
	}
	for _, owner := range []Transport{cloud, ble} {
		for _, points := range cases {
			result, err := owner.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "invalid", Points: points})
			if err == nil || result.OK {
				t.Fatalf("%T accepted invalid points: %+v", owner, points)
			}
		}
		for _, start := range []int64{-1, 1 << 31, math.MaxInt64} {
			if _, err := owner.Play(t.Context(), PlayCommand{StreamID: "invalid", StartTimeMillis: start}); err == nil {
				t.Fatalf("%T accepted invalid start: %d", owner, start)
			}
		}
	}
	if calls.Load() != 0 || bridge.Snapshot().Pending != 0 {
		t.Fatal("invalid input reached a device or clock endpoint")
	}
	if got := ble.MotionSamplingCapabilities().PositionResolutionPercent; got != 1 {
		t.Fatalf("Bluetooth advertised %g%% resolution; firmware Point.x is integer percent", got)
	}
}

func TestCloudRejectsMalformedSuccessWithoutAdvancingBuffer(t *testing.T) {
	for _, body := range []string{"", "<html>proxy error</html>", "null", "[]", `{"ok":false}`, `{"result":`, `{"result":{}}` + strings.Repeat(" ", 64*1024)} {
		t.Run(body[:min(len(body), 20)], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/hsp/setup" {
					_, _ = w.Write([]byte(`{"result":{}}`))
					return
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			cloud := newTestCloudTransport(t, server.URL)
			result, err := cloud.AppendPoints(t.Context(), AppendPointsCommand{StreamID: "test", Points: []TimedPoint{{TimeMillis: 0, PositionPercent: 50}}})
			if err == nil || result.OK || cloud.hspPointCount != 0 {
				t.Fatalf("invalid response advanced buffer: %+v, err=%v, points=%d", result, err, cloud.hspPointCount)
			}
		})
	}
}

func TestHandyOwnersTrackZeroBasedTailAcrossAppends(t *testing.T) {
	var tails []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hsp/add" {
			var body struct {
				Tail int `json:"tail_point_stream_index"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			tails = append(tails, body.Tail)
		}
		_, _ = w.Write([]byte(`{"result":{}}`))
	}))
	defer server.Close()
	cloud := newTestCloudTransport(t, server.URL)
	bridge := NewBrowserBluetoothBridge()
	bridge.ConnectClient(BrowserBluetoothClientStatus{ClientID: "tails"})
	ble := newTestBrowserBluetoothTransport(t, bridge, BrowserBluetoothOptions{})
	for index, batch := range []AppendPointsCommand{
		{StreamID: "one", Points: []TimedPoint{{TimeMillis: 0, PositionPercent: 20}, {TimeMillis: 100, PositionPercent: 80}}},
		{StreamID: "one", Points: []TimedPoint{{TimeMillis: 200, PositionPercent: 20}}},
		{StreamID: "two", Points: []TimedPoint{{TimeMillis: 0, PositionPercent: 20}, {TimeMillis: 100, PositionPercent: 80}}},
	} {
		if _, err := cloud.AppendPoints(t.Context(), batch); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := ble.AppendPoints(t.Context(), batch); done <- err }()
		commands, err := bridge.NextCommands(t.Context(), "tails", time.Second)
		if err != nil || len(commands) != 1 {
			t.Fatalf("Bluetooth append: %+v, %v", commands, err)
		}
		bridge.Acknowledge("tails", BrowserBluetoothBridgeAck{ID: commands[0].ID, OK: true})
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		want := []int{1, 2, 1}[index]
		if tails[index] != want || commands[0].Body["tail_point_stream_index"] != want {
			t.Fatalf("append %d: Cloud tail=%d, BLE tail=%v, want=%d", index, tails[index], commands[0].Body["tail_point_stream_index"], want)
		}
	}
}

func TestCloudStopPreemptsWorkFollowingSetupOrClockSync(t *testing.T) {
	for _, path := range []string{"/hsp/setup", "/servertime"} {
		t.Run(path, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			var motionCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == path {
					close(entered)
					<-release
				}
				if r.URL.Path == "/hsp/add" || r.URL.Path == "/hsp/play" {
					motionCalls.Add(1)
				}
				_, _ = w.Write([]byte(`{"result":{},"server_time":1700000000000}`))
			}))
			defer server.Close()
			defer once.Do(func() { close(release) })
			cloud := newTestCloudTransport(t, server.URL)
			done := make(chan error, 1)
			go func() {
				var err error
				if path == "/servertime" {
					_, err = cloud.Play(context.Background(), PlayCommand{StreamID: "test"})
				} else {
					_, err = cloud.AppendPoints(context.Background(), AppendPointsCommand{StreamID: "test", Points: []TimedPoint{{TimeMillis: 0, PositionPercent: 50}}})
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request did not enter the preparation stage")
			}
			stopDone := make(chan error, 1)
			go func() { _, err := cloud.Stop(context.Background(), StopCommand{}); stopDone <- err }()
			waitForTest(t, time.Second, func() bool { return cloud.motionGate.stops.Load() > 0 })
			once.Do(func() { close(release) })
			if err := <-done; err == nil {
				t.Fatal("preempted operation reported success")
			}
			if err := <-stopDone; err != nil {
				t.Fatal(err)
			}
			if motionCalls.Load() != 0 {
				t.Fatal("motion was dispatched after Stop admission")
			}
		})
	}
}
