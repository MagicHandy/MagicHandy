//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
)

func resolveLiveTestDataDir(t *testing.T) string {
	t.Helper()

	if override := strings.TrimSpace(os.Getenv("MAGICHANDY_DATA_DIR")); override != "" {
		return override
	}

	candidates := []string{}
	if defaultDir, err := config.ResolveDataDir(""); err == nil {
		candidates = append(candidates, defaultDir)
	}
	if wd, err := os.Getwd(); err == nil {
		// MagicHandy/.local-data when tests run from module root or internal/httpapi.
		for _, rel := range []string{".local-data", filepath.Join("..", ".local-data"), filepath.Join("..", "..", ".local-data")} {
			candidates = append(candidates, filepath.Clean(filepath.Join(wd, rel)))
		}
	}

	seen := map[string]struct{}{}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		store, err := config.OpenStore(dir)
		if err != nil {
			continue
		}
		settings, _ := store.Snapshot()
		_ = store.Close()
		if strings.TrimSpace(settings.Device.HandyConnectionKey) != "" {
			t.Logf("selected data_dir=%s (connection key present)", dir)
			return dir
		}
	}

	if defaultDir, err := config.ResolveDataDir(""); err == nil {
		return defaultDir
	}
	t.Fatal("could not resolve data directory")
	return ""
}

const (
	proceduralSyncSampleInterval = 250 * time.Millisecond
	proceduralSyncHoldAfterStart = 2 * time.Second
	proceduralSyncBetweenChain   = 550 * time.Millisecond
	proceduralSyncObserveAfter   = 10 * time.Second
	proceduralSyncChainInterval  = 2 * time.Second
	proceduralSyncMaxIdleGap     = 850 * time.Millisecond
	proceduralSyncMinActiveRatio = 0.80
	proceduralSyncMinPositionDelta = 3.0
)

// TestProceduralSyncUninterruptedOnRealDevice validates procedural chat motion on a
// real Handy using the connection key persisted in SQLite (AppData or MAGICHANDY_DATA_DIR).
// Run: go test -tags=integration ./internal/httpapi -run TestProceduralSyncUninterruptedOnRealDevice -v -timeout 5m
func TestProceduralSyncUninterruptedOnRealDevice(t *testing.T) {
	server := newLiveSQLiteTestServer(t)
	recorder := installE2ERecordingTransport(t, server)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		settings.Motion.HardwareSafetyLock = false
		settings.Diagnostics.Verbosity = config.DiagnosticsVerbosityDebug
		if settings.Device.HSPDispatchOwner == "" {
			settings.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
		}
		return settings
	})

	settings, _ := server.store.Snapshot()
	if strings.TrimSpace(settings.Device.HandyConnectionKey) == "" {
		t.Skip("no handy_connection_key in SQLite store — configure device in Settings first")
	}
	t.Logf("using data_dir=%s dispatch_owner=%s key_len=%d",
		server.store.DataDir(),
		settings.Device.HSPDispatchOwner,
		len(strings.TrimSpace(settings.Device.HandyConnectionKey)),
	)

	requireLiveDeviceFromStore(t, server)
	t.Cleanup(func() { stopProceduralSyncMotion(t, server) })

	ctx := context.Background()
	scenarios := []struct {
		name    string
		command *chat.MotionCommand
	}{
		{
			name: "start_fluido_meio_cabeca",
			command: &chat.MotionCommand{
				Action:     chat.MotionActionStart,
				Velocidade: 55,
				Intensidade: 65,
				Regiao:     "meio_cabeca",
				TipoBatida: "fluido",
			},
		},
		{
			name: "chain_target_meio_base",
			command: &chat.MotionCommand{
				Action:     chat.MotionActionTarget,
				Velocidade: 60,
				Intensidade: 70,
				Regiao:     "meio_base",
				TipoBatida: "fluido",
			},
		},
		{
			name: "chain_target_cabeca_base",
			command: &chat.MotionCommand{
				Action:     chat.MotionActionTarget,
				Velocidade: 62,
				Intensidade: 72,
				Regiao:     "cabeca_base",
				TipoBatida: "fluido",
			},
		},
	}

	errorsBefore := recorder.transportErrorCount()
	addsBefore := recorder.hspAddCount()

	gen := bumpChatChaosGeneration(server)
	if err := server.playChatChaoticMotion(ctx, cloneMotionCommand(scenarios[0].command), settings, gen); err != nil {
		t.Fatalf("start playChatChaoticMotion: %v", err)
	}
	waitForHSPDispatchAfter(t, recorder, addsBefore)
	assertRecorderTransportOKSince(t, recorder, errorsBefore)
	t.Logf("after start: hsp_plays=%d hsp_adds=%d", recorder.hspPlayCount(), recorder.hspAddCount())

	t.Log("holding 2s after start — observe uninterrupted baseline motion")
	observeDeadline := time.Now().Add(proceduralSyncObserveAfter)
	samples := sampleProceduralSync(t, server, 2*time.Second)

	chainCommands := scenarios[1:]
	for index, scenario := range chainCommands {
		time.Sleep(proceduralSyncChainInterval)
		gen = bumpChatChaosGeneration(server)
		addsBefore = recorder.hspAddCount()
		if err := server.playChatChaoticMotion(ctx, cloneMotionCommand(scenario.command), settings, gen); err != nil {
			t.Fatalf("%s playChatChaoticMotion: %v", scenario.name, err)
		}
		waitForHSPDispatchAfter(t, recorder, addsBefore)
		assertRecorderTransportOK(t, recorder)
		t.Logf("after %s: hsp_plays=%d hsp_adds=%d player_running=%v",
			scenario.name, recorder.hspPlayCount(), recorder.hspAddCount(), isChatChaosPlayerRunning(server))
		samples = append(samples, sampleProceduralSync(t, server, proceduralSyncChainInterval)...)
		if index == len(chainCommands)-1 {
			break
		}
	}

	remaining := time.Until(observeDeadline)
	if remaining > 0 {
		t.Logf("observing %.1fs continuous playback after chained targets", remaining.Seconds())
		samples = append(samples, sampleProceduralSync(t, server, remaining)...)
	}

	for index, sample := range samples {
		moving := sample.PlaybackActive || sample.PlayerRunning
		if !moving {
			t.Logf("idle sample[%d] cloud=%q player=%v pos=%.1f", index, sample.CloudState, sample.PlayerRunning, sample.PositionPct)
		}
	}

	coreSamples := trimTrailingIdle(samples)
	if len(coreSamples) == 0 {
		t.Fatal("no active sync samples before session end")
	}
	report := analyzeProceduralSyncSamples(t, server, coreSamples, recorder)
	t.Logf("sync report: %+v", report)

	if report.TransportErrors > errorsBefore {
		t.Fatalf("transport errors during session: %v", recorder.snapshot().Errors[errorsBefore:])
	}
	if report.ActiveRatio < proceduralSyncMinActiveRatio {
		t.Fatalf("playback_active ratio = %.2f, want >= %.2f (interrupted motion)", report.ActiveRatio, proceduralSyncMinActiveRatio)
	}
	if report.MaxIdleGap > proceduralSyncMaxIdleGap {
		t.Fatalf("max idle gap = %s, want <= %s (motion interrupted)", report.MaxIdleGap, proceduralSyncMaxIdleGap)
	}
	if report.PositionRange < proceduralSyncMinPositionDelta {
		t.Fatalf("position range = %.2f, want >= %.2f (device/static playhead)", report.PositionRange, proceduralSyncMinPositionDelta)
	}
	if report.StarvationEvents > 0 {
		t.Fatalf("starvation_risk events = %d, want 0", report.StarvationEvents)
	}
	if report.MaxHSPPointGapMS > 180 && report.MaxHSPPointGapMS < 10000 {
		t.Logf("warning: max HSP point gap %dms — review trace pacing", report.MaxHSPPointGapMS)
	}

	points := collectHSPPointsFromAdds(recorder, 0)
	if len(points) < 20 {
		t.Fatalf("HSP points = %d, want rich procedural stream", len(points))
	}
	if minDelta := minConsecutiveDelta(points); minDelta > 150 {
		t.Fatalf("min HSP delta = %dms, want fluid pacing <150ms", minDelta)
	}

	t.Log("procedural sync validation PASSED — motion uninterrupted within latency budget")
}

func newLiveSQLiteTestServer(t *testing.T) *Server {
	return newLiveSQLiteTestServerWithRuntime(t, Runtime{
		Traces: diagnostics.NewTraceRing(128),
	})
}

func newLiveSQLiteTestServerWithRuntime(t *testing.T, runtime Runtime) *Server {
	t.Helper()

	if runtime.Traces == nil {
		runtime.Traces = diagnostics.NewTraceRing(128)
	}
	dataDir := resolveLiveTestDataDir(t)
	store, err := config.OpenStore(dataDir)
	if err != nil {
		t.Fatalf("OpenStore(%s): %v", dataDir, err)
	}

	server, err := New(testStaticFS(), slog.New(slog.NewTextHandler(io.Discard, nil)), store, runtime, VersionInfo{
		Version: "integration",
		Commit:  "live",
	})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	t.Cleanup(func() { server.Close() })
	return server
}

type proceduralSyncSample struct {
	At             time.Time
	PlaybackActive bool
	CloudState     string
	PlayerRunning  bool
	PositionPct    float64
	BufferAheadMS  *int64
}

type proceduralSyncReport struct {
	SampleCount      int
	ActiveRatio      float64
	MaxIdleGap       time.Duration
	PositionRange    float64
	StarvationEvents int
	TransportErrors  int
	MaxHSPPointGapMS int64
}

func sampleProceduralSync(t *testing.T, server *Server, duration time.Duration) []proceduralSyncSample {
	t.Helper()

	deadline := time.Now().Add(duration)
	var samples []proceduralSyncSample
	cloudPoll := time.Time{}
	for time.Now().Before(deadline) {
		cloudState := ""
		if cloudPoll.IsZero() || time.Since(cloudPoll) >= 2*time.Second {
			cloudState = fetchCloudPlaybackState(t, server)
			cloudPoll = time.Now()
		}
		samples = append(samples, captureProceduralSyncSample(server, cloudState))
		time.Sleep(proceduralSyncSampleInterval)
	}
	return samples
}

func captureProceduralSyncSample(server *Server, cloudState string) proceduralSyncSample {
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/motion/visual", nil))
	var visual map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &visual)

	active, _ := visual["playback_active"].(bool)
	pos, _ := visual["position_pct"].(float64)

	playerRunning := false
	server.chatChaos.mu.Lock()
	if p := server.chatChaos.player; p != nil {
		snap := p.Snapshot()
		playerRunning = snap.Running && !snap.Paused
	}
	server.chatChaos.mu.Unlock()

	var bufferAhead *int64
	if v, ok := visual["buffer_ahead_ms"].(float64); ok {
		b := int64(v)
		bufferAhead = &b
	}

	return proceduralSyncSample{
		At:             time.Now(),
		PlaybackActive: active,
		CloudState:     cloudState,
		PlayerRunning:  playerRunning,
		PositionPct:    pos,
		BufferAheadMS:  bufferAhead,
	}
}

func isChatChaosPlayerRunning(server *Server) bool {
	server.chatChaos.mu.Lock()
	defer server.chatChaos.mu.Unlock()
	if p := server.chatChaos.player; p != nil {
		snap := p.Snapshot()
		return snap.Running && !snap.Paused
	}
	return false
}

func trimTrailingIdle(samples []proceduralSyncSample) []proceduralSyncSample {
	for len(samples) > 0 {
		last := samples[len(samples)-1]
		if last.PlayerRunning || last.PlaybackActive {
			break
		}
		samples = samples[:len(samples)-1]
	}
	return samples
}

func analyzeProceduralSyncSamples(t *testing.T, server *Server, samples []proceduralSyncSample, recorder *recordingMotionTransport) proceduralSyncReport {
	t.Helper()

	if len(samples) == 0 {
		t.Fatal("no sync samples collected")
	}

	activeCount := 0
	minPos, maxPos := samples[0].PositionPct, samples[0].PositionPct
	var idleStart *time.Time
	maxIdle := time.Duration(0)
	starvation := 0

	for _, sample := range samples {
		moving := sample.PlaybackActive || sample.PlayerRunning
		if moving {
			activeCount++
			idleStart = nil
		} else if idleStart == nil {
			now := sample.At
			idleStart = &now
		} else {
			gap := sample.At.Sub(*idleStart)
			if gap > maxIdle {
				maxIdle = gap
			}
		}
		if sample.PositionPct < minPos {
			minPos = sample.PositionPct
		}
		if sample.PositionPct > maxPos {
			maxPos = sample.PositionPct
		}
		if sample.BufferAheadMS != nil && *sample.BufferAheadMS < 650 && sample.PlayerRunning {
			starvation++
		}
	}

	starvation += countHandyLogStarvation(server)

	points := collectHSPPointsFromAdds(recorder, 0)
	maxGap := int64(0)
	for i := 1; i < len(points); i++ {
		gap := points[i].TimeMillis - points[i-1].TimeMillis
		if gap > maxGap {
			maxGap = gap
		}
	}

	return proceduralSyncReport{
		SampleCount:      len(samples),
		ActiveRatio:      float64(activeCount) / float64(len(samples)),
		MaxIdleGap:       maxIdle,
		PositionRange:    math.Abs(maxPos - minPos),
		StarvationEvents: starvation,
		TransportErrors:  len(recorder.snapshot().Errors),
		MaxHSPPointGapMS: maxGap,
	}
}

func countHandyLogStarvation(server *Server) int {
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/diagnostics/handy-log?limit=200", nil))
	if rec.Code != http.StatusOK {
		return 0
	}
	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		return 0
	}
	count := 0
	for _, entry := range payload.Entries {
		event, _ := entry["event"].(string)
		if strings.Contains(event, "starvation") {
			count++
		}
		if details, ok := entry["details"].(map[string]any); ok {
			if risk, ok := details["starvation_risk"].(bool); ok && risk {
				count++
			}
		}
	}
	return count
}

func requireLiveDeviceFromStore(t *testing.T, server *Server) {
	t.Helper()

	settings, _ := server.store.Snapshot()
	key := strings.TrimSpace(settings.Device.HandyConnectionKey)
	if key == "" {
		t.Skip("no connection key in store")
	}

	owner := settings.Device.HSPDispatchOwner
	if owner == "" {
		owner = config.DispatchOwnerCloudREST
	}

	recorder := httptest.NewRecorder()
	body := `{"transport":"cloud_rest","connection_key":` + jsonString(key) + `}`
	request := httptest.NewRequest(http.MethodPost, "/api/device/transport", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("device transport status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/device/connect", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Skipf("device not reachable (connect status %d): %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"connected":true`) {
		t.Skipf("device not connected: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/transport/cloud/check", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Skipf("cloud check failed (status %d): %s", recorder.Code, recorder.Body.String())
	}

	var check struct {
		OK           bool `json:"ok"`
		HSPAvailable bool `json:"hsp_available"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &check); err != nil {
		t.Fatalf("decode cloud check: %v", err)
	}
	if !check.OK || !check.HSPAvailable {
		t.Skipf("HSP unavailable on device: ok=%v hsp=%v", check.OK, check.HSPAvailable)
	}
	t.Logf("device connected via %s (HSP available)", owner)
}

func jsonString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func stopProceduralSyncMotion(t *testing.T, server *Server) {
	t.Helper()
	ctx := context.Background()
	server.cancelChatChaosMotion(ctx)
	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/transport/cloud/stop", nil))
	server.Handler().ServeHTTP(recorder, request)
}

// TestProceduralBridgeFillerDevice validates bridge filler keeps player_running after
// the initial segment without a second HSP play restart.
func TestProceduralBridgeFillerDevice(t *testing.T) {
	server := newLiveSQLiteTestServer(t)
	installE2ERecordingTransport(t, server)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		settings.Motion.HardwareSafetyLock = false
		return settings
	})

	settings, _ := server.store.Snapshot()
	if strings.TrimSpace(settings.Device.HandyConnectionKey) == "" {
		t.Skip("no handy_connection_key in SQLite store")
	}
	requireLiveDeviceFromStore(t, server)
	t.Cleanup(func() { stopProceduralSyncMotion(t, server) })

	ctx := context.Background()
	command := &chat.MotionCommand{
		Action:     chat.MotionActionStart,
		Velocidade: 50,
		Intensidade: 55,
		Regiao:     "meio",
		TipoBatida: "fluido",
	}
	server.chatChaos.mu.Lock()
	server.chatChaos.generation = 1
	gen := server.chatChaos.generation
	server.chatChaos.mu.Unlock()

	if err := server.playChatChaoticMotion(ctx, command, settings, gen); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)
	server.chatChaosMaybeLoopBridge(ctx, gen, settings)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		server.chatChaos.mu.Lock()
		player := server.chatChaos.player
		server.chatChaos.mu.Unlock()
		if player != nil && player.Running() && player.TimelineEndMS() > 15_000 {
			t.Logf("bridge extended timeline to %dms", player.TimelineEndMS())
			return
		}
		server.chatChaosMaybeLoopBridge(ctx, gen, settings)
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("bridge filler did not extend procedural timeline on device")
}
