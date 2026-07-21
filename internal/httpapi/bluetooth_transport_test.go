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
)

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
	if !strings.Contains(recorder.Body.String(), "cloud REST dispatch owner is not selected") {
		t.Fatalf("response = %s, want dispatch owner error", recorder.Body.String())
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
