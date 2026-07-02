package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestBluetoothManualHSPAddQueuesBrowserCommandAndTraces(t *testing.T) {
	bridge := transport.NewBrowserBluetoothBridge()
	server := newCloudTestServer(t, Runtime{BrowserBluetoothBridge: bridge})
	saveBluetoothSettings(t, server)

	connectRecorder := httptest.NewRecorder()
	connectRequest := httptest.NewRequest(http.MethodPost, "/api/transport/bluetooth/connect", strings.NewReader(`{
		"client_id": "client-1",
		"connected": true,
		"device_name": "Handy",
		"protocol": "hsp_ble"
	}`))
	connectRequest = withController(connectRequest)
	server.Handler().ServeHTTP(connectRecorder, connectRequest)
	if connectRecorder.Code != http.StatusOK {
		t.Fatalf("connect status = %d, want %d: %s", connectRecorder.Code, http.StatusOK, connectRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/transport/bluetooth/hsp-add", strings.NewReader(`{
		"stream_id": "7",
		"points": [{"position_percent": 25, "time_ms": 10}]
	}`)))
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(recorder, request)
		close(done)
	}()

	commands, err := bridge.NextCommands(t.Context(), "client-1", time.Second)
	if err != nil {
		t.Fatalf("NextCommands: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("command count = %d, want 1", len(commands))
	}
	if commands[0].Path != "hsp/add" || commands[0].Body["stream_id"] != 7 {
		t.Fatalf("command = %+v, want numeric HSP add", commands[0])
	}
	bridge.Acknowledge("client-1", transport.BrowserBluetoothBridgeAck{
		ID:            commands[0].ID,
		OK:            true,
		ElapsedMillis: 8,
	})
	waitForHTTPHandler(t, done)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response bluetoothCommandResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Result.OK || response.Result.Kind != transport.CommandKindHSPAdd {
		t.Fatalf("result = %+v, want successful HSP add", response.Result)
	}
	rows := server.traces.Rows()
	if len(rows) != 1 || rows[0].TransportResult.Kind != transport.CommandKindHSPAdd {
		t.Fatalf("trace rows = %+v, want one Bluetooth HSP add row", rows)
	}
}

func TestCloudEndpointDoesNotFallbackWhenBluetoothIsSelected(t *testing.T) {
	var cloudHits atomic.Int32
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cloudHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer cloudServer.Close()

	server := newCloudTestServer(t, Runtime{CloudBaseURL: cloudServer.URL})
	saveCloudSettings(t, server)
	saveBluetoothSettings(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/transport/cloud/check", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if cloudHits.Load() != 0 {
		t.Fatalf("cloud endpoint was called %d time(s), want no fallback", cloudHits.Load())
	}
	if !strings.Contains(recorder.Body.String(), "Cloud REST dispatch owner is not selected") {
		t.Fatalf("response = %s, want dispatch owner error", recorder.Body.String())
	}
}

func TestBluetoothSelectedUnavailableReturnsBridgeFailure(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})
	saveBluetoothSettings(t, server)

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/transport/bluetooth/hsp-play", strings.NewReader(`{
		"stream_id": "7",
		"start_time_ms": 0
	}`)))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	var response bluetoothCommandResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Result.Status != "bridge_unavailable" {
		t.Fatalf("result = %+v, want bridge_unavailable", response.Result)
	}
	if response.Bridge.Ready {
		t.Fatalf("bridge = %+v, want not ready", response.Bridge)
	}
}

func TestBluetoothStatusRecordsBrowserUnsupported(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/transport/bluetooth/status", strings.NewReader(`{
		"client_id": "client-unsupported",
		"connected": false,
		"supported": false,
		"status": "unsupported",
		"message": "Web Bluetooth is not available in this browser."
	}`))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response bluetoothStatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Bluetooth.Supported || response.Bluetooth.Status != "unsupported" {
		t.Fatalf("bluetooth = %+v, want unsupported browser status", response.Bluetooth)
	}
}

func saveBluetoothSettings(t *testing.T, server *Server) {
	t.Helper()

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerBrowserBluetooth
		return settings
	})
}

func waitForHTTPHandler(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for HTTP handler")
	}
}
