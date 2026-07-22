package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	result, err := cloud.AppendPoints(context.Background(), AppendPointsCommand{
		StreamID: "stream-1",
		Points: []TimedPoint{
			{PositionPercent: 25, TimeMillis: 0},
			{PositionPercent: 75.25, TimeMillis: 250},
		},
	})
	if err != nil {
		t.Fatalf("AppendPoints: %v", err)
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
	if diagnostics.LastCommand == nil || diagnostics.LastCommand.PointsAdd == nil {
		t.Fatalf("diagnostics missing safe HSP add command: %+v", diagnostics.LastCommand)
	}
	if diagnostics.LastCommand.PointsAdd.Points[1].PositionPercent != 75.25 {
		t.Fatalf("diagnostic points = %+v, want safe semantic points", diagnostics.LastCommand.PointsAdd.Points)
	}
}

func TestCloudRESTTransportDeclaresBufferedLead(t *testing.T) {
	capabilities := (&CloudRESTTransport{}).MotionTimingCapabilities()
	if capabilities.MinimumBufferedLead != 1500*time.Millisecond {
		t.Fatalf("minimum buffered lead = %v, want 1.5s", capabilities.MinimumBufferedLead)
	}
}

func TestCloudRESTTransportRejectsHTTP200ErrorWithoutAdvancingTail(t *testing.T) {
	addAttempts := 0
	var addBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/hsp/setup":
			_, _ = w.Write([]byte(`{"result":{"play_state":2}}`))
		case "/hsp/add":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read add body: %v", err)
			}
			addBodies = append(addBodies, string(body))
			addAttempts++
			if addAttempts == 1 {
				_, _ = fmt.Fprintf(w, `{"error":{"code":1001,"name":"DeviceNotConnected","message":"private %s"}}`, cloudSecretFixture)
				return
			}
			_, _ = w.Write([]byte(`{"result":{"play_state":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	command := AppendPointsCommand{
		StreamID: "error-envelope",
		Points: []TimedPoint{
			{PositionPercent: 20, TimeMillis: 0},
			{PositionPercent: 80, TimeMillis: 250},
		},
	}
	result, err := cloud.AppendPoints(context.Background(), command)
	if err == nil || result.OK {
		t.Fatalf("first AppendPoints result = %+v, error = %v; want rejected HTTP 200 envelope", result, err)
	}
	if !strings.Contains(err.Error(), "DeviceNotConnected") || strings.Contains(err.Error(), cloudSecretFixture) {
		t.Fatalf("error = %q, want safe API code without response message", err)
	}
	if cloud.hspPointCount != 0 {
		t.Fatalf("point count = %d, want failed add not to advance the HSP tail", cloud.hspPointCount)
	}
	if diagnostics := cloud.Diagnostics(); diagnostics.Connected || diagnostics.LastResult == nil || diagnostics.LastResult.OK {
		t.Fatalf("diagnostics = %+v, want failed 200 response recorded as disconnected", diagnostics)
	}

	result, err = cloud.AppendPoints(context.Background(), command)
	if err != nil || !result.OK {
		t.Fatalf("retry AppendPoints result = %+v, error = %v", result, err)
	}
	if cloud.hspPointCount != len(command.Points) {
		t.Fatalf("point count = %d, want %d after accepted retry", cloud.hspPointCount, len(command.Points))
	}
	if len(addBodies) != 2 || !strings.Contains(addBodies[1], `"tail_point_stream_index":2`) || !strings.Contains(addBodies[1], `"flush":true`) {
		t.Fatalf("add bodies = %q, want retry to retain first-add tail and flush semantics", addBodies)
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

func TestCloudRESTTransportConnectionCheckParsesV3HSPState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": {
				"play_state": 2,
				"points": 0,
				"max_points": 9707,
				"current_point": 0,
				"current_time": 424639,
				"loop": true,
				"playback_rate": 1,
				"first_point_time": 0,
				"last_point_time": 0,
				"stream_id": 0,
				"tail_point_stream_index": 0,
				"tail_point_stream_index_threshold": 0,
				"pause_on_starving": false
			}
		}`))
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	check, err := cloud.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if !check.OK || !check.HSPAvailable || check.PlaybackState != "stopped" {
		t.Fatalf("check = %+v, want available stopped HSP", check)
	}
}

func TestCloudRESTTransportReportsPlayRequestMidpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/servertime" {
			_, _ = fmt.Fprintf(w, `{"server_time":%d}`, time.Now().UnixMilli())
			return
		}
		if r.URL.Path == "/hsp/play" {
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"result":{"play_state":1}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	before := time.Now()
	if _, err := cloud.Play(context.Background(), PlayCommand{StreamID: "clock", StartTimeMillis: 250}); err != nil {
		t.Fatalf("Play: %v", err)
	}
	after := time.Now()
	origin := cloud.PlaybackStartTime()
	if origin.IsZero() || origin.Before(before.Add(-260*time.Millisecond)) || origin.After(after.Add(-240*time.Millisecond)) {
		t.Fatalf("playback origin = %v, want request midpoint minus 250ms within %v..%v", origin, before, after)
	}
}

func TestParseHSPPlaybackStateRejectsInvalidNumericEnums(t *testing.T) {
	for _, value := range []any{-1, 5, 1.5, math.NaN(), math.Inf(1), ""} {
		if state, ok := parseHSPPlaybackState(value); ok {
			t.Fatalf("parseHSPPlaybackState(%v) = %q, true; want rejected", value, state)
		}
	}
	for value, want := range map[int]string{
		0: "not_initialized",
		1: "playing",
		2: "stopped",
		3: "paused",
		4: "starving",
	} {
		if state, ok := parseHSPPlaybackState(value); !ok || state != want {
			t.Fatalf("parseHSPPlaybackState(%d) = %q, %t; want %q, true", value, state, ok, want)
		}
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
	if check.Message != unavailable.Message || check.Message == "" {
		t.Fatalf("check message = %q, want %q", check.Message, unavailable.Message)
	}
}

func TestCloudRESTTransportConnectionCheckHonorsOKFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"playback_state":"unsupported"}`))
	}))
	defer server.Close()

	cloud := newTestCloudTransport(t, server.URL)
	check, err := cloud.CheckConnection(context.Background())
	if err == nil {
		t.Fatal("CheckConnection succeeded for ok=false response")
	}
	var unavailable HSPUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %T %[1]v, want HSPUnavailableError", err)
	}
	if check.OK || check.HSPAvailable {
		t.Fatalf("check = %+v, want unavailable", check)
	}
}

func TestCloudRESTTransportConnectionCheckRejectsUnrecognizedBodies(t *testing.T) {
	for _, body := range []string{"", "not-json", `{}`, `{"unexpected":true}`} {
		t.Run(fmt.Sprintf("body_%q", body), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			cloud := newTestCloudTransport(t, server.URL)
			check, err := cloud.CheckConnection(context.Background())
			if err == nil {
				t.Fatalf("CheckConnection accepted unrecognized body %q", body)
			}
			if check.OK || check.HSPAvailable {
				t.Fatalf("check = %+v, want unavailable", check)
			}
		})
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

func TestCloudRESTTransportSSEErrorsDoNotLeakCredentialURL(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s", request.URL.String())
	})}
	cloud, err := NewCloudRESTTransport(
		validCloudPrerequisites(),
		CloudBuildOptions{},
		CloudEndpointConfig{BaseURL: "https://cloud.invalid/"},
		client,
	)
	if err != nil {
		t.Fatalf("NewCloudRESTTransport: %v", err)
	}

	err = cloud.ListenStateEvents(context.Background(), nil)
	if err == nil {
		t.Fatal("ListenStateEvents succeeded after transport failure")
	}
	for _, value := range []string{err.Error(), cloud.Diagnostics().LastError} {
		if strings.Contains(value, cloudSecretFixture) || strings.Contains(value, "app-public-id") {
			t.Fatalf("event-stream error leaked credential URL: %q", value)
		}
	}
}

func TestParseServerTimeRejectsNonFiniteAndNonPositiveValues(t *testing.T) {
	for _, value := range []any{"NaN", "Infinity", "-Infinity", "-1", "0", -1.0, 0.0} {
		if parsed, ok := parseServerTimeValue(value); ok {
			t.Fatalf("parseServerTimeValue(%#v) = %d, true; want rejected", value, parsed)
		}
	}
	if parsed, ok := parseServerTimeValue("1700000000123.4"); !ok || parsed != 1700000000123 {
		t.Fatalf("valid server time = %d, %v", parsed, ok)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
