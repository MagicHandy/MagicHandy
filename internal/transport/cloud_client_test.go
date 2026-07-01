package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudRESTTransportDispatchesHSPCommands(t *testing.T) {
	var seen []capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		seen = append(seen, capturedRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			ApplicationID: r.Header.Get("X-Api-Key"),
			ConnectionKey: r.Header.Get("X-Connection-Key"),
			Body:          string(body),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"hsp_available":true,"playback_state":"playing"}`))
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	result, err := cloud.AddHSP(context.Background(), HSPAddCommand{
		StreamID: "stream-1",
		Points: []TimedPoint{
			{PositionPercent: 25, TimeMillis: 0},
			{PositionPercent: 75, TimeMillis: 250},
		},
	})
	if err != nil {
		t.Fatalf("AddHSP: %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
	if len(seen) != 2 {
		t.Fatalf("request count = %d, want setup plus add", len(seen))
	}
	if seen[0].Method != http.MethodPut || seen[0].Path != "/hsp/setup" {
		t.Fatalf("request = %+v, want HSP setup path", seen[0])
	}
	if seen[0].ApplicationID != "app-public-id" || seen[0].ConnectionKey != cloudSecretFixture {
		t.Fatalf("auth headers = %+v, want app id and connection key", seen[0])
	}
	if !strings.Contains(seen[0].Body, `"stream_id":1`) {
		t.Fatalf("body = %s, want setup stream id", seen[0].Body)
	}
	if seen[1].Method != http.MethodPut || seen[1].Path != "/hsp/add" {
		t.Fatalf("request = %+v, want HSP add path", seen[1])
	}
	if seen[1].ApplicationID != "app-public-id" || seen[1].ConnectionKey != cloudSecretFixture {
		t.Fatalf("auth headers = %+v, want app id and connection key", seen[1])
	}
	if !strings.Contains(seen[1].Body, `"x":25`) || !strings.Contains(seen[1].Body, `"t":250`) ||
		!strings.Contains(seen[1].Body, `"flush":true`) {
		t.Fatalf("body = %s, want HSP points", seen[1].Body)
	}

	diagnostics := cloud.Diagnostics()
	if diagnostics.LastCommand == nil || diagnostics.LastCommand.HSPAdd == nil {
		t.Fatalf("diagnostics missing safe HSP add command: %+v", diagnostics.LastCommand)
	}
	if diagnostics.LastCommand.HSPAdd.Points[1].PositionPercent != 75 {
		t.Fatalf("diagnostic points = %+v, want safe HSP points", diagnostics.LastCommand.HSPAdd.Points)
	}
}

func TestCloudRESTTransportRedactsDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rejected "+cloudSecretFixture, http.StatusUnauthorized)
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	_, err := cloud.Stop(context.Background(), StopCommand{Reason: cloudSecretFixture})
	if err == nil {
		t.Fatal("Stop succeeded against unauthorized server")
	}

	data, err := json.Marshal(cloud.Diagnostics())
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	if strings.Contains(string(data), cloudSecretFixture) {
		t.Fatalf("diagnostics leaked secret: %s", data)
	}
	if !strings.Contains(string(data), `"status":"http_401"`) {
		t.Fatalf("diagnostics = %s, want HTTP status", data)
	}
}

func TestCloudRESTTransportConnectionCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hsp/state" {
			t.Fatalf("path = %q, want /hsp/state", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hsp_available":true,"playback_state":"idle"}`))
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	check, err := cloud.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if !check.OK || !check.HSPAvailable || check.PlaybackState != "idle" {
		t.Fatalf("check = %+v, want available idle", check)
	}
}

func TestCloudRESTTransportConnectionCheckReportsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hsp_available":false,"playback_state":"unsupported"}`))
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	check, err := cloud.CheckConnection(context.Background())
	if err == nil {
		t.Fatal("CheckConnection succeeded for unavailable HSP")
	}
	var unavailable HSPUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %T %[1]v, want HSPUnavailableError", err)
	}
	if check.OK || check.HSPAvailable {
		t.Fatalf("check = %+v, want unavailable", check)
	}
}

func TestCloudRESTTransportReadState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"available":true,"state":"buffering"}`))
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	state, result, err := cloud.ReadState(context.Background())
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if !result.OK || !state.Available || state.PlaybackState != "buffering" {
		t.Fatalf("state = %+v result = %+v, want buffering", state, result)
	}
}

func TestCloudRESTTransportSSEListener(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sse" {
			t.Fatalf("path = %q, want /sse", r.URL.Path)
		}
		if r.URL.Query().Get("ck") != cloudSecretFixture || r.URL.Query().Get("apikey") != "app-public-id" {
			t.Fatalf("query = %q, want SSE credentials", r.URL.RawQuery)
		}
		if r.URL.Query().Get("events") == "" {
			t.Fatalf("query = %q, want event subscriptions", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("accept = %q, want text/event-stream", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: state\ndata: {\"playback_state\":\"playing\"}\n\n"))
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	var events []HSPStateEvent
	err := cloud.ListenStateEvents(context.Background(), func(event HSPStateEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("ListenStateEvents: %v", err)
	}
	if len(events) != 1 || events[0].Event != "state" {
		t.Fatalf("events = %+v, want one state event", events)
	}
	if !strings.Contains(string(events[0].Data), "playing") {
		t.Fatalf("event data = %s, want playing", events[0].Data)
	}
}

func newTestCloudTransport(t *testing.T, baseURL string) *CloudRESTTransport {
	t.Helper()

	cloud, err := NewCloudRESTTransport(
		validCloudPrerequisites(),
		CloudBuildOptions{},
		CloudEndpointConfig{BaseURL: baseURL},
		&http.Client{},
	)
	if err != nil {
		t.Fatalf("NewCloudRESTTransport: %v", err)
	}
	return cloud
}

type capturedRequest struct {
	Method        string
	Path          string
	ApplicationID string
	ConnectionKey string
	Body          string
}
