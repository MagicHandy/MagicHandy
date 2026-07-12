package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestIntifaceHTTPRuntimeConnectSelectDispatchAndDisconnect(t *testing.T) {
	fake := newHTTPAPIButtplugServer(t)
	server := newTestServerWithRuntime(t, Runtime{})
	defer func() {
		server.Close()
		fake.Close()
	}()
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerIntiface
		settings.Device.IntifaceServerAddress = fake.URL()
		return settings
	})

	connect := callIntifaceAPI(t, server, http.MethodPost, "/api/transport/intiface/connect", `{}`)
	if !connect.Status.Connected || len(connect.Status.Devices) != 1 {
		t.Fatalf("connect snapshot = %+v, want one connected device", connect)
	}
	reconnectRequest := withController(httptest.NewRequest(http.MethodPost, "/api/transport/intiface/connect", strings.NewReader(`{}`)))
	reconnectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(reconnectRecorder, reconnectRequest)
	if reconnectRecorder.Code != http.StatusConflict {
		t.Fatalf("second connect status = %d, want %d", reconnectRecorder.Code, http.StatusConflict)
	}
	selected := callIntifaceAPI(t, server, http.MethodPost, "/api/transport/intiface/select", `{"device_index":7,"actuator_index":0}`)
	if selected.Status.SelectedDeviceIndex == nil || *selected.Status.SelectedDeviceIndex != 7 {
		t.Fatalf("selection = %+v, want device 7", selected.Status)
	}

	owner, err := server.newSelectedMotionTransport()
	if err != nil {
		t.Fatalf("newSelectedMotionTransport: %v", err)
	}
	ctx := context.Background()
	if _, err := owner.AppendPoints(ctx, transport.AppendPointsCommand{
		StreamID: "http-runtime",
		Points: []transport.TimedPoint{
			{PositionPercent: 10.25, TimeMillis: 0},
			{PositionPercent: 90.75, TimeMillis: 50},
		},
	}); err != nil {
		t.Fatalf("AppendPoints: %v", err)
	}
	if _, err := owner.Play(ctx, transport.PlayCommand{StreamID: "http-runtime"}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	fake.waitForKind(t, "LinearCmd")

	disconnected := callIntifaceAPI(t, server, http.MethodPost, "/api/transport/intiface/disconnect", `{}`)
	if disconnected.Status.Connected {
		t.Fatalf("disconnect snapshot = %+v, want disconnected", disconnected)
	}
	fake.waitForKind(t, "StopDeviceCmd")
	if _, err := server.newSelectedMotionTransport(); err == nil {
		t.Fatal("selected transport remained available after disconnect")
	}
}

func TestIntifaceDisconnectedSnapshotUsesEmptyDeviceList(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	snapshot := server.intifaceSnapshot()
	if snapshot.Status.Devices == nil || len(snapshot.Status.Devices) != 0 {
		t.Fatalf("devices = %#v, want a non-nil empty list", snapshot.Status.Devices)
	}
}

func TestIntifaceOwnerSwitchStopsActiveEngineBeforeClosingSession(t *testing.T) {
	fake := newHTTPAPIButtplugServer(t)
	server := newTestServerWithRuntime(t, Runtime{})
	defer func() {
		server.Close()
		fake.Close()
	}()
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerIntiface
		settings.Device.IntifaceServerAddress = fake.URL()
		settings.Motion.SpeedMaxPercent = 30
		return settings
	})
	callIntifaceAPI(t, server, http.MethodPost, "/api/transport/intiface/connect", `{}`)
	callIntifaceAPI(t, server, http.MethodPost, "/api/transport/intiface/select", `{"device_index":7,"actuator_index":0}`)

	engine, err := server.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	previous, _ := server.store.Snapshot()
	if _, err := engine.Start(context.Background(), motion.MotionTarget{
		Source:       "owner_switch_test",
		PatternID:    motion.PatternID("stroke"),
		SpeedPercent: 20,
	}, previous.Motion); err != nil {
		t.Fatal(err)
	}
	fake.waitForKind(t, "LinearCmd")

	next := previous
	next.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
	if _, err := server.store.Save(next); err != nil {
		t.Fatal(err)
	}
	server.applySettingsRuntimeTransition(context.Background(), previous, next)
	fake.waitForKind(t, "StopDeviceCmd")
	if server.currentMotionEngine() != nil {
		t.Fatal("motion engine remained installed after dispatch-owner switch")
	}
	if _, err := server.currentIntiface(); err == nil {
		t.Fatal("Intiface session remained available after dispatch-owner switch")
	}
}

func callIntifaceAPI(t *testing.T, server *Server, method, path, body string) intifaceSnapshot {
	t.Helper()
	request := withController(httptest.NewRequest(method, path, strings.NewReader(body)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d body = %s", method, path, recorder.Code, recorder.Body.String())
	}
	var snapshot intifaceSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return snapshot
}

type httpAPIButtplugServer struct {
	server  *httptest.Server
	mu      sync.Mutex
	conn    *websocket.Conn
	history []string
	notify  chan string
}

func newHTTPAPIButtplugServer(t *testing.T) *httpAPIButtplugServer {
	t.Helper()
	fake := &httpAPIButtplugServer{notify: make(chan string, 64)}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	return fake
}

func (f *httpAPIButtplugServer) URL() string {
	return "ws" + strings.TrimPrefix(f.server.URL, "http")
}

func (f *httpAPIButtplugServer) Close() {
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	if conn != nil {
		_ = conn.CloseNow()
	}
	f.server.Close()
}

func (f *httpAPIButtplugServer) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var envelopes []map[string]json.RawMessage
		if json.Unmarshal(data, &envelopes) != nil {
			return
		}
		for _, envelope := range envelopes {
			for kind, raw := range envelope {
				var fields map[string]any
				if json.Unmarshal(raw, &fields) != nil {
					return
				}
				id := uint32(fields["Id"].(float64))
				f.mu.Lock()
				f.history = append(f.history, kind)
				f.mu.Unlock()
				f.notify <- kind
				switch kind {
				case "RequestServerInfo":
					f.write("ServerInfo", map[string]any{"Id": id, "ServerName": "test", "MessageVersion": 3, "MaxPingTime": 1000})
				case "RequestDeviceList":
					device := map[string]any{
						"DeviceIndex": 7,
						"DeviceName":  "Test Linear",
						"DeviceMessages": map[string]any{"LinearCmd": []map[string]any{{
							"FeatureDescriptor": "Position", "ActuatorType": "Position", "StepCount": 10000,
						}}},
					}
					f.write("DeviceList", map[string]any{"Id": id, "Devices": []any{device}})
				default:
					f.write("Ok", map[string]any{"Id": id})
				}
			}
		}
	}
}

func (f *httpAPIButtplugServer) write(kind string, fields map[string]any) {
	data, _ := json.Marshal([]map[string]any{{kind: fields}})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn != nil {
		_ = f.conn.Write(context.Background(), websocket.MessageText, data)
	}
}

func (f *httpAPIButtplugServer) waitForKind(t *testing.T, want string) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case got := <-f.notify:
			if got == want {
				return
			}
		case <-timer.C:
			f.mu.Lock()
			history := append([]string(nil), f.history...)
			f.mu.Unlock()
			t.Fatalf("timed out waiting for %s; history: %v", want, history)
		}
	}
}
