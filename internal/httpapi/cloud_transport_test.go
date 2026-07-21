package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const cloudTestConnectionKey = "test-connection-key"

func TestResolveCloudApplicationIDFallsBackToBundled(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Device.APIApplicationIDSource = config.ApplicationIDSourceDeveloperOverride
	settings.Device.APIApplicationIDOverride = "  "

	if got := resolveCloudApplicationID(settings); got != config.BundledAPIApplicationID {
		t.Fatalf("application ID = %q, want bundled public v3 ID", got)
	}
}

func TestCloudConnectionCheckEndpointReadsState(t *testing.T) {
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hsp/state" {
			t.Fatalf("path = %q, want /hsp/state", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hsp_available":true,"playback_state":"idle"}`))
	}))
	defer cloudServer.Close()

	server := newCloudTestServer(t, Runtime{CloudBaseURL: cloudServer.URL})
	saveCloudSettings(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/transport/cloud/check", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var check transport.ConnectionCheckResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &check); err != nil {
		t.Fatalf("decode check response: %v", err)
	}
	if !check.OK || !check.HSPAvailable || check.PlaybackState != "idle" {
		t.Fatalf("check = %+v, want idle available", check)
	}
}

func TestCloudManualTransportFailureRedactsSecrets(t *testing.T) {
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rejected "+cloudTestConnectionKey, http.StatusUnauthorized)
	}))
	defer cloudServer.Close()

	server := newCloudTestServer(t, Runtime{CloudBaseURL: cloudServer.URL})
	saveCloudSettings(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/transport/cloud/stop", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
	if contains(recorder.Body.String(), cloudTestConnectionKey) {
		t.Fatalf("cloud error response leaked connection key: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/transport/cloud/diagnostics", nil)
	server.Handler().ServeHTTP(recorder, request)
	if contains(recorder.Body.String(), cloudTestConnectionKey) {
		t.Fatalf("cloud diagnostics leaked connection key: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"http_401"`) {
		t.Fatalf("diagnostics = %s, want upstream HTTP status", recorder.Body.String())
	}
}

func TestCloudMotionFailureRedactsSecrets(t *testing.T) {
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rejected "+cloudTestConnectionKey, http.StatusUnauthorized)
	}))
	defer cloudServer.Close()

	server := newCloudTestServer(t, Runtime{CloudBaseURL: cloudServer.URL})
	saveCloudSettings(t, server)
	request := withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":20}`)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	if contains(recorder.Body.String(), cloudTestConnectionKey) {
		t.Fatalf("motion error response leaked connection key: %s", recorder.Body.String())
	}
	traceJSON, err := json.Marshal(server.traces.Export())
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	if strings.Contains(string(traceJSON), cloudTestConnectionKey) {
		t.Fatalf("motion trace leaked connection key: %s", traceJSON)
	}
}

func TestCloudEventsEndpointProxiesSSE(t *testing.T) {
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sse" {
			t.Fatalf("path = %q, want /sse", r.URL.Path)
		}
		if r.URL.Query().Get("ck") != cloudTestConnectionKey || r.URL.Query().Get("apikey") != "dev-app-id" {
			t.Fatalf("query = %q, want SSE credentials", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("accept = %q, want text/event-stream", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: state\ndata: {\"playback_state\":\"playing\"}\n\n"))
	}))
	defer cloudServer.Close()

	server := newCloudTestServer(t, Runtime{CloudBaseURL: cloudServer.URL})
	saveCloudSettings(t, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/transport/cloud/events", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}
	if !strings.Contains(recorder.Body.String(), "event: state") ||
		!strings.Contains(recorder.Body.String(), `"playback_state":"playing"`) {
		t.Fatalf("SSE body = %q, want proxied state event", recorder.Body.String())
	}
}

func newCloudTestServer(t *testing.T, runtime Runtime) *Server {
	t.Helper()

	if runtime.Traces == nil {
		runtime.Traces = diagnostics.NewTraceRing(8)
	}
	if runtime.Transport == nil {
		runtime.Transport = transport.NewFake()
	}
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	server, err := New(testStaticFS(), slog.New(slog.NewTextHandler(io.Discard, nil)), store, runtime, VersionInfo{
		Version: "test",
		Commit:  "test",
	})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	t.Cleanup(server.Close)
	return server
}

func testStaticFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>MagicHandy</title>")},
		"app.css":    {Data: []byte("body { margin: 0; }")},
		"app.js":     {Data: []byte("console.log('ready');")},
	}
}

func saveCloudSettings(t *testing.T, server *Server) {
	t.Helper()

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.APIApplicationIDSource = config.ApplicationIDSourceDeveloperOverride
		settings.Device.APIApplicationIDOverride = "dev-app-id"
		settings.Device.HandyConnectionKey = cloudTestConnectionKey
		settings.Motion.ReverseDirection = true
		return settings
	})
}

func captureCloudRequest(t *testing.T, r *http.Request) capturedCloudRequest {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return capturedCloudRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		ApplicationID: r.Header.Get("X-Api-Key"),
		ConnectionKey: r.Header.Get("X-Connection-Key"),
		Body:          string(body),
	}
}

func readCapturedCloudRequest(t *testing.T, requests <-chan capturedCloudRequest) capturedCloudRequest {
	t.Helper()

	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for captured cloud request")
		return capturedCloudRequest{}
	}
}

type capturedCloudRequest struct {
	Method        string
	Path          string
	ApplicationID string
	ConnectionKey string
	Body          string
}
