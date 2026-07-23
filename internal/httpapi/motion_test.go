package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type motionEnvelope struct {
	Available bool                     `json:"available"`
	Error     string                   `json:"error"`
	Engine    motion.ActiveMotionState `json:"engine"`
}

type blockingModeStartTransport struct {
	*transport.Fake
	entered chan struct{}
	once    sync.Once
}

type failingStrokeTransport struct {
	*transport.Fake
	mu         sync.Mutex
	failStroke bool
}

func (t *failingStrokeTransport) SetStrokeWindow(ctx context.Context, command transport.StrokeWindowCommand) (transport.CommandResult, error) {
	t.mu.Lock()
	fail := t.failStroke
	t.mu.Unlock()
	if !fail {
		return t.Fake.SetStrokeWindow(ctx, command)
	}
	err := errors.New("simulated stroke-window failure")
	return transport.CommandResult{
		Kind:      transport.CommandKindStrokeWindow,
		Transport: "failing_stroke",
		Status:    "failed",
	}, err
}

func (t *failingStrokeTransport) FailStrokeWindow() {
	t.mu.Lock()
	t.failStroke = true
	t.mu.Unlock()
}

func (b *blockingModeStartTransport) Play(ctx context.Context, _ transport.PlayCommand) (transport.CommandResult, error) {
	b.once.Do(func() { close(b.entered) })
	<-ctx.Done()
	return transport.CommandResult{Kind: transport.CommandKindPointsPlay}, ctx.Err()
}

func TestMotionEngineCreationIsSingleOwner(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})

	const callers = 16
	start := make(chan struct{})
	engines := make(chan *motion.Engine, callers)
	errs := make(chan error, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			engine, _, err := server.motionEngineForStart()
			if err != nil {
				errs <- err
				return
			}
			engines <- engine
		}()
	}
	close(start)
	workers.Wait()
	close(engines)
	close(errs)
	for err := range errs {
		t.Fatalf("motionEngineForStart: %v", err)
	}

	unique := make(map[*motion.Engine]struct{})
	for engine := range engines {
		unique[engine] = struct{}{}
	}
	if len(unique) != 1 {
		t.Fatalf("created %d idle motion engines, want exactly one", len(unique))
	}
}

func TestStopAndClearStopsIdleEngine(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	if _, _, err := server.motionEngineForStart(); err != nil {
		t.Fatalf("motionEngineForStart: %v", err)
	}

	if err := server.stopAndClearMotionEngine(t.Context(), "test_idle_clear"); err != nil {
		t.Fatalf("stopAndClearMotionEngine: %v", err)
	}
	if engine := server.currentMotionEngine(); engine != nil {
		t.Fatalf("motion engine survived clear: %+v", engine.Snapshot())
	}
	commands := fake.Commands()
	if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop {
		t.Fatalf("idle clear commands = %+v, want one explicit Stop", commands)
	}
}

func TestQuiesceStopsMotionAndRejectsNewControllerWork(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":30}`)
	if !started.Engine.Running {
		t.Fatal("motion did not start before quiesce")
	}

	server.Quiesce()
	if engine := server.currentMotionEngine(); engine != nil {
		t.Fatalf("motion engine survived quiesce: %+v", engine.Snapshot())
	}
	commands := fake.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("quiesce commands = %+v, want final Stop", commands)
	}

	startRecorder := httptest.NewRecorder()
	startRequest := withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":30}`)))
	server.Handler().ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("start during shutdown = %d, want %d: %s", startRecorder.Code, http.StatusServiceUnavailable, startRecorder.Body.String())
	}

	stopRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopRecorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	if stopRecorder.Code != http.StatusOK {
		t.Fatalf("Stop during shutdown = %d, want %d: %s", stopRecorder.Code, http.StatusOK, stopRecorder.Body.String())
	}
}

func TestQuiesceStopsBlockedModeStartBeforeDrainingPlanner(t *testing.T) {
	commandTransport := &blockingModeStartTransport{
		Fake:    transport.NewFake(),
		entered: make(chan struct{}),
	}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       commandTransport,
		MotionTransport: commandTransport,
	})
	if _, err := server.modes.Start(t.Context(), modes.ModeFreestyle); err != nil {
		t.Fatalf("start freestyle: %v", err)
	}

	select {
	case <-commandTransport.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("mode did not reach blocked transport startup")
	}

	done := make(chan struct{})
	go func() {
		server.Quiesce()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Quiesce waited on the planner before canceling transport startup")
	}
}

func callMotion(t *testing.T, server *Server, method, path, body string) motionEnvelope {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(method, path, reader)
	request = withController(request)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d, body = %s", method, path, recorder.Code, recorder.Body.String())
	}
	var envelope motionEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
	return envelope
}

func TestMotionStartStateStop(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":60}`)
	if !started.Available || !started.Engine.Running {
		t.Fatalf("expected running motion after start, got %+v", started)
	}
	if started.Engine.Target.SpeedPercent != 60 {
		t.Fatalf("target speed = %d, want 60", started.Engine.Target.SpeedPercent)
	}
	if started.Engine.Target.Source != motion.TargetSourceManualUI {
		t.Fatalf("target source = %q, want manual_ui", started.Engine.Target.Source)
	}
	restarted := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"pulse","speed_percent":30}`)
	if !restarted.Engine.Running || restarted.Engine.Target.PatternID != motion.PatternPulse {
		t.Fatalf("manual motion did not restart with the replacement target: %+v", restarted.Engine)
	}

	state := callMotion(t, server, http.MethodGet, "/api/motion/state", "")
	if !state.Engine.Running {
		t.Fatalf("state should report running: %+v", state)
	}

	stopped := callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
	if stopped.Engine.Running {
		t.Fatalf("motion should be stopped, got %+v", stopped)
	}
}

func TestManualMotionStartEndsActiveMode(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	if _, err := server.modes.Start(t.Context(), modes.ModeFreestyle); err != nil {
		t.Fatalf("start freestyle: %v", err)
	}
	if status := server.modes.Status(); !status.Active {
		t.Fatalf("freestyle status = %+v, want active", status)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if engine := server.currentMotionEngine(); engine == nil || !engine.Snapshot().Running {
		t.Fatal("freestyle did not start motion before manual takeover")
	}

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"pulse","speed_percent":30}`)
	if status := server.modes.Status(); status.Active {
		t.Fatalf("mode survived manual motion start: %+v", status)
	}
	if !started.Engine.Running || started.Engine.Target.Source != motion.TargetSourceManualUI {
		t.Fatalf("manual motion did not own the engine: %+v", started.Engine)
	}
}

func TestManualTargetCannotRelabelAutopilotMotion(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	if _, err := server.modes.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
			break
		}
		time.Sleep(time.Millisecond)
	}
	engine := server.currentMotionEngine()
	if engine == nil || engine.Snapshot().Target.Source != modes.ModeAutopilot {
		t.Fatalf("autopilot did not own motion before retarget: %+v", engine)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/motion/target", strings.NewReader(`{"pattern":"pulse","speed_percent":30}`))
	request = withController(request)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("manual retarget status = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if source := engine.Snapshot().Target.Source; source != modes.ModeAutopilot {
		t.Fatalf("target source = %q after rejected retarget, want %q", source, modes.ModeAutopilot)
	}
}

func TestManualTargetRequiresRunningManualTest(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":30}`)
	callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)

	request := httptest.NewRequest(http.MethodPost, "/api/motion/target", strings.NewReader(`{"pattern":"pulse","speed_percent":25}`))
	request = withController(request)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("idle manual retarget status = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if state := server.currentMotionEngine().Snapshot(); state.Running || state.Target.PatternID != motion.PatternStroke {
		t.Fatalf("idle engine changed after rejected retarget: %+v", state)
	}
}

func TestMotionStopWithoutEngineStillAttemptsConfiguredTransport(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(server.Close)

	recorder := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)).WithContext(ctx)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	commands := fake.Commands()
	if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want one unconditional Stop", commands)
	}
}

func TestMotionStopSequencePublishesRepeatedEmergencyStops(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
	callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("state status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var state struct {
		StopSequence uint64 `json:"stop_sequence"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.StopSequence != 2 {
		t.Fatalf("stop_sequence = %d, want 2", state.StopSequence)
	}
}

func TestMotionStopWithoutReachableTransportReportsFailure(t *testing.T) {
	server := newTestServerWithRuntime(t, Runtime{})
	t.Cleanup(server.Close)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "stop could not reach the configured transport") {
		t.Fatalf("stop failure was not explicit: %s", recorder.Body.String())
	}
}

func TestMotionStopWithoutEngineUsesSelectedCloudOwner(t *testing.T) {
	requests := make(chan capturedCloudRequest, 1)
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureCloudRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer cloudServer.Close()
	server := newTestServerWithRuntime(t, Runtime{CloudBaseURL: cloudServer.URL})
	t.Cleanup(server.Close)
	saveCloudSettings(t, server)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("stop status = %d: %s", recorder.Code, recorder.Body.String())
	}
	seen := readCapturedCloudRequest(t, requests)
	if seen.Path != "/hsp/stop" || seen.Method != http.MethodPut {
		t.Fatalf("Cloud Stop request = %+v", seen)
	}
}

func TestMotionStopWithoutEngineUsesSelectedBrowserBluetoothOwner(t *testing.T) {
	bridge := transport.NewBrowserBluetoothBridge()
	connected := true
	bridge.ConnectClient(transport.BrowserBluetoothClientStatus{
		ClientID:   "stop-client",
		Connected:  &connected,
		DeviceName: "Handy",
		Protocol:   "hsp_ble",
		Status:     "connected",
	})
	server := newTestServerWithRuntime(t, Runtime{BrowserBluetoothBridge: bridge})
	t.Cleanup(server.Close)
	saveBluetoothSettings(t, server)

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
		close(done)
	}()
	commands, err := bridge.NextCommands(t.Context(), "stop-client", time.Second)
	if err != nil || len(commands) != 1 || commands[0].Path != "hsp/stop" {
		t.Fatalf("Bluetooth Stop commands = %+v, %v", commands, err)
	}
	bridge.Acknowledge("stop-client", transport.BrowserBluetoothBridgeAck{ID: commands[0].ID, OK: true})
	waitForHTTPHandler(t, done)
	if recorder.Code != http.StatusOK {
		t.Fatalf("stop status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMotionStateReportsPausedEngine(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":60}`)
	if !started.Engine.Running {
		t.Fatalf("expected running motion after start, got %+v", started)
	}
	paused := callMotion(t, server, http.MethodPost, "/api/motion/pause", `{}`)
	if paused.Engine.Running || !paused.Engine.Paused {
		t.Fatalf("expected paused motion, got %+v", paused)
	}

	state := callMotion(t, server, http.MethodGet, "/api/motion/state", "")
	if !state.Available || state.Engine.Running || !state.Engine.Paused {
		t.Fatalf("state should expose paused engine for Resume UI: %+v", state)
	}
}

func TestMotionStartClampsSpeedToSettings(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent = 10
		settings.Motion.SpeedMaxPercent = 40
		return settings
	})

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":95}`)
	if started.Engine.Target.SpeedPercent != 40 {
		t.Fatalf("speed should clamp to max 40, got %d", started.Engine.Target.SpeedPercent)
	}
}

func TestMotionUnavailableWithoutSelectedTransportPrerequisites(t *testing.T) {
	server := newTestServerWithRuntime(t, Runtime{})
	t.Cleanup(server.Close)

	stateRecorder := httptest.NewRecorder()
	stateRequest := httptest.NewRequest(http.MethodGet, "/api/motion/state", nil)
	server.Handler().ServeHTTP(stateRecorder, stateRequest)
	if stateRecorder.Code != http.StatusOK {
		t.Fatalf("state status = %d, want %d", stateRecorder.Code, http.StatusOK)
	}
	var state motionEnvelope
	if err := json.Unmarshal(stateRecorder.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode motion state: %v", err)
	}
	if state.Available {
		t.Fatalf("motion state should be unavailable without Cloud credentials: %s", stateRecorder.Body.String())
	}
	if !strings.Contains(stateRecorder.Body.String(), "Handy connection key is required") {
		t.Fatalf("state body = %s, want credential error", stateRecorder.Body.String())
	}

	startRecorder := httptest.NewRecorder()
	startRequest := withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":35}`)))
	server.Handler().ServeHTTP(startRecorder, startRequest)
	if startRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("start status = %d, want %d: %s", startRecorder.Code, http.StatusServiceUnavailable, startRecorder.Body.String())
	}
	if strings.Contains(startRecorder.Body.String(), "fake_handy") {
		t.Fatalf("motion start should not fall back to fake transport: %s", startRecorder.Body.String())
	}
}

func TestMotionStartUsesSelectedCloudTransport(t *testing.T) {
	requests := make(chan capturedCloudRequest, 16)
	cloudServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureCloudRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/slider/state":
			// The test enables reverse direction. PatternStroke begins at semantic
			// zero, which maps to the physical top of the full stroke window.
			_, _ = w.Write([]byte(`{"result":{"position":1,"position_absolute":100,"speed_absolute":0}}`))
		case r.URL.Path == "/slider/stroke" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"result":{"min":0,"max":1,"min_absolute":0,"max_absolute":100}}`))
		case r.URL.Path == "/servertime":
			_, _ = fmt.Fprintf(w, `{"server_time":%d}`, time.Now().UnixMilli())
		default:
			_, _ = w.Write([]byte(`{"ok":true,"hsp_available":true,"playback_state":"playing"}`))
		}
	}))
	defer cloudServer.Close()

	server := newTestServerWithRuntime(t, Runtime{CloudBaseURL: cloudServer.URL})
	t.Cleanup(server.Close)
	saveCloudSettings(t, server)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":35}`)
	if !started.Available || !started.Engine.Running {
		t.Fatalf("expected Cloud-backed motion to run, got %+v", started)
	}
	if started.Engine.LastResult == nil || started.Engine.LastResult.Transport == "fake_handy" {
		t.Fatalf("motion result used fake transport: %+v", started.Engine.LastResult)
	}

	wantRequests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPut, path: "/hsp/stop"},
		{method: http.MethodGet, path: "/slider/state"},
		{method: http.MethodGet, path: "/slider/stroke"},
		{method: http.MethodPut, path: "/slider/stroke"},
		{method: http.MethodPut, path: "/hsp/setup"},
		{method: http.MethodPut, path: "/hsp/add"},
		{method: http.MethodPut, path: "/hsp/add"},
		{method: http.MethodGet, path: "/servertime"},
		{method: http.MethodPut, path: "/hsp/play"},
	}
	for _, want := range wantRequests {
		seen := readCapturedCloudRequest(t, requests)
		if seen.Method != want.method || seen.Path != want.path {
			t.Fatalf("request = %+v, want %s %s", seen, want.method, want.path)
		}
		if seen.Path != "/servertime" && (seen.ApplicationID != "dev-app-id" || seen.ConnectionKey != cloudTestConnectionKey) {
			t.Fatalf("auth headers = %+v, want settings-derived credentials", seen)
		}
	}
}

func TestMediaSpeedPolicyChanged(t *testing.T) {
	base := config.DefaultSettings().Motion
	base.SpeedMaxPercent = 60
	enabled := base
	enabled.ApplyVideoSpeedLimit = true
	maxChangedOff := base
	maxChangedOff.SpeedMaxPercent = 40
	maxChangedOn := enabled
	maxChangedOn.SpeedMaxPercent = 40
	minimumChangedOn := enabled
	minimumChangedOn.SpeedMinPercent++

	testCases := []struct {
		name     string
		previous config.MotionSettings
		next     config.MotionSettings
		want     bool
	}{
		{name: "unchanged", previous: base, next: base},
		{name: "maximum changes while disabled", previous: base, next: maxChangedOff},
		{name: "enabled", previous: base, next: enabled, want: true},
		{name: "disabled", previous: enabled, next: base, want: true},
		{name: "maximum changes while enabled", previous: enabled, next: maxChangedOn, want: true},
		{name: "minimum changes while enabled", previous: enabled, next: minimumChangedOn},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := mediaSpeedPolicyChanged(testCase.previous, testCase.next); got != testCase.want {
				t.Fatalf("mediaSpeedPolicyChanged() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestMotionRefreshAppliesToActiveLoopImmediately(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":70}`)
	if !started.Engine.Running {
		t.Fatalf("expected running motion")
	}

	tighter := config.MotionSettings{SpeedMinPercent: 10, SpeedMaxPercent: 25, StrokeMinPercent: 0, StrokeMaxPercent: 100}
	if err := server.refreshActiveMotion(context.Background(), tighter); err != nil {
		t.Fatalf("refreshActiveMotion: %v", err)
	}

	snapshot := server.motion.engine.Snapshot()
	if !snapshot.Running {
		t.Fatalf("refresh must not stop active motion")
	}
	if snapshot.Settings.SpeedMaxPercent != 25 {
		t.Fatalf("settings not applied: max = %d, want 25", snapshot.Settings.SpeedMaxPercent)
	}
	if snapshot.Target.SpeedPercent > 25 {
		t.Fatalf("target speed %d should be reclamped to <= 25", snapshot.Target.SpeedPercent)
	}
}

func TestMotionQuickReportsSavedButFailedRuntimeRefresh(t *testing.T) {
	commandTransport := &failingStrokeTransport{Fake: transport.NewFake()}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       commandTransport,
		MotionTransport: commandTransport,
	})
	if started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":30}`); !started.Engine.Running {
		t.Fatal("motion did not start")
	}
	commandTransport.FailStrokeWindow()

	request := withController(httptest.NewRequest(http.MethodPost, "/api/motion/quick", strings.NewReader(`{"stroke_min_percent":10}`)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	var response struct {
		Error  string                   `json:"error"`
		Engine motion.ActiveMotionState `json:"engine"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error == "" || response.Engine.Running {
		t.Fatalf("response = %+v, want saved-settings warning and stopped engine", response)
	}
	settings, _ := server.store.Snapshot()
	if settings.Motion.StrokeMinPercent != 10 {
		t.Fatalf("saved stroke minimum = %d, want 10", settings.Motion.StrokeMinPercent)
	}
	commands := commandTransport.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want recovery Stop", commands)
	}
}

func TestCloudCredentialChangeStopsAndClearsExistingMotionEngine(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	if started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":30}`); !started.Engine.Running {
		t.Fatal("motion did not start")
	}
	previous, _ := server.store.Snapshot()
	next := previous
	next.Device.HandyConnectionKey = "replacement-test-key"
	if _, err := server.store.Save(next); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := server.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatalf("applySettingsRuntimeTransition: %v", err)
	}
	if engine := server.currentMotionEngine(); engine != nil {
		t.Fatalf("motion engine retained old Cloud credential: %+v", engine.Snapshot())
	}
	commands := fake.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("commands = %+v, want Stop before Cloud transport rebuild", commands)
	}
}

func TestPromptOnlySettingsTransitionDoesNotRefreshActiveMotion(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	defer server.Close()
	if started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":30}`); !started.Engine.Running {
		t.Fatal("motion did not start")
	}
	previous, _ := server.store.Snapshot()
	next := previous
	next.LLM.ChatVoice = config.LLMChatVoiceIntimate
	next.LLM.UserAnatomy = config.LLMUserAnatomyCustom
	next.LLM.CustomAnatomy = "chosen wording"
	next.LLM.PersonaDescription = "An energetic partner"
	before := len(fake.Commands())
	if err := server.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatalf("applySettingsRuntimeTransition: %v", err)
	}
	if after := len(fake.Commands()); after != before {
		t.Fatalf("prompt-only settings emitted transport commands: before=%d after=%d commands=%+v", before, after, fake.Commands())
	}
}

func TestMotionStateExposedInAggregateState(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	var body struct {
		Motion struct {
			Available bool `json:"available"`
		} `json:"motion"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if !body.Motion.Available {
		t.Fatalf("state should report motion available with a command transport")
	}
}

func TestMotionEventsStreamsMotionState(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/motion/events?client_id=events-client", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("motion events request: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(response.Body)
	var block strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE line: %v", err)
		}
		if line == "\n" || line == "\r\n" {
			break
		}
		block.WriteString(line)
	}
	if !strings.Contains(block.String(), "event: motion") || !strings.Contains(block.String(), `"available":true`) {
		t.Fatalf("SSE block = %q, want motion availability event", block.String())
	}
}
