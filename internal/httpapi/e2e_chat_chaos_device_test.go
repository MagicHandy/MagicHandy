//go:build integration

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

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	e2eDeviceConnectionKey = "rySSd"
	e2eChaosLLMResponse    = `{"reply":"Simulação de teste E2E.","motion":{"action":"start","velocidade":95,"intensidade":100,"regiao":"cabeca_base","tipo_batida":"alto"}}`
	e2eHSPDispatchTimeout  = 8 * time.Second
	e2ePollInterval        = 25 * time.Millisecond
)

// recordingMotionTransport delegates to a real CloudRESTTransport while
// capturing HSP commands for E2E assertions.
type recordingMotionTransport struct {
	inner transport.Transport

	mu              sync.Mutex
	hspAdds         []transport.HSPAddCommand
	hspPlays        []transport.HSPPlayCommand
	transportErrors []string
}

func newRecordingMotionTransport(inner transport.Transport) *recordingMotionTransport {
	return &recordingMotionTransport{inner: inner}
}

func (r *recordingMotionTransport) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	return r.inner.Stop(ctx, command)
}

func (r *recordingMotionTransport) SetStrokeWindow(ctx context.Context, command transport.StrokeWindowCommand) (transport.CommandResult, error) {
	return r.inner.SetStrokeWindow(ctx, command)
}

func (r *recordingMotionTransport) AddHSP(ctx context.Context, command transport.HSPAddCommand) (transport.CommandResult, error) {
	r.mu.Lock()
	r.hspAdds = append(r.hspAdds, command)
	r.mu.Unlock()
	result, err := r.inner.AddHSP(ctx, command)
	r.recordOutcome(result, err)
	return result, err
}

func (r *recordingMotionTransport) PlayHSP(ctx context.Context, command transport.HSPPlayCommand) (transport.CommandResult, error) {
	r.mu.Lock()
	r.hspPlays = append(r.hspPlays, command)
	r.mu.Unlock()
	result, err := r.inner.PlayHSP(ctx, command)
	r.recordOutcome(result, err)
	return result, err
}

func (r *recordingMotionTransport) recordOutcome(result transport.CommandResult, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.transportErrors = append(r.transportErrors, err.Error())
	}
	if !result.OK {
		message := result.Error
		if message == "" {
			message = result.Status
		}
		r.transportErrors = append(r.transportErrors, message)
	}
}

func (r *recordingMotionTransport) Diagnostics() transport.TransportDiagnostics {
	return r.inner.Diagnostics()
}

func (r *recordingMotionTransport) snapshot() recordingMotionSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	adds := make([]transport.HSPAddCommand, len(r.hspAdds))
	copy(adds, r.hspAdds)
	return recordingMotionSnapshot{
		HSPAdds:  adds,
		HSPPlays: append([]transport.HSPPlayCommand(nil), r.hspPlays...),
		Errors:   append([]string(nil), r.transportErrors...),
	}
}

func (r *recordingMotionTransport) hspAddCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hspAdds)
}

func (r *recordingMotionTransport) hspPlayCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hspPlays)
}

func (r *recordingMotionTransport) transportErrorCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.transportErrors)
}

type recordingMotionSnapshot struct {
	HSPAdds  []transport.HSPAddCommand
	HSPPlays []transport.HSPPlayCommand
	Errors   []string
}

func TestE2EChatChaos(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		e2eChaosLLMResponse,
		e2eChaosLLMResponse,
		e2eChaosLLMResponse,
	}}
	server := newCloudTestServer(t, Runtime{
		LLMProvider: provider,
		Traces:      diagnostics.NewTraceRing(64),
	})
	saveE2EDeviceSettings(t, server)
	requireE2EDeviceReady(t, server)

	recorder := installE2ERecordingTransport(t, server)
	t.Cleanup(func() {
		stopE2EMotion(t, server)
	})

	t.Run("scenario_1_chaos_unlocked", func(t *testing.T) {
		saveSettings(t, server.store, func(settings config.Settings) config.Settings {
			settings.Motion.HardwareSafetyLock = false
			settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
			return settings
		})

		body := postChatStream(t, server, `{"message":"e2e chaos unlocked"}`)
		assertMotionApplied(t, body, true)

		waitForHSPDispatch(t, recorder)
		assertRecorderTransportOK(t, recorder)
		if state := waitForCloudMotionActive(t, server, recorder); state != "" {
			t.Logf("cloud playback_state=%q", state)
		}

		snap := recorder.snapshot()
		if len(snap.HSPPlays) == 0 || snap.HSPPlays[0].StartTimeMillis < 300 {
			t.Fatalf("HSP play start_time = %+v, want >=300ms lead alignment", snap.HSPPlays)
		}

		t.Log("device should be moving now — holding 3s for physical verification")
		time.Sleep(3 * time.Second)

		points := collectHSPPointsFromAdds(recorder, 0)
		if len(points) < 10 {
			t.Fatalf("HSP points = %d, want multiple micro-waypoints for tipo_batida alto", len(points))
		}
		minDelta := minConsecutiveDelta(points)
		if minDelta >= 30 {
			t.Fatalf("min consecutive delta = %dms, want <30ms with hardware_safety_lock disabled", minDelta)
		}
	})

	t.Run("scenario_2_debounce_800ms", func(t *testing.T) {
		saveSettings(t, server.store, func(settings config.Settings) config.Settings {
			settings.Motion.HardwareSafetyLock = false
			return settings
		})

		provider.responses = []string{e2eChaosLLMResponse, e2eChaosLLMResponse}
		addsBefore := recorder.hspAddCount()
		callsBefore := provider.callCount()

		body1 := postChatStream(t, server, `{"message":"e2e debounce first"}`)
		assertMotionApplied(t, body1, true)

		time.Sleep(100 * time.Millisecond)

		body2 := postChatStream(t, server, `{"message":"e2e debounce second"}`)
		if strings.Contains(body2, `"applied":true`) {
			t.Fatalf("second chat within 800ms should not apply motion:\n%s", body2)
		}
		if provider.callCount() != callsBefore+2 {
			t.Fatalf("provider calls = %d, want %d", provider.callCount(), callsBefore+2)
		}

		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if recorder.hspAddCount() > addsBefore {
				break
			}
			time.Sleep(e2ePollInterval)
		}

		addsAfterFirst := recorder.hspAddCount()
		time.Sleep(200 * time.Millisecond)
		if got := recorder.hspAddCount(); got != addsAfterFirst {
			t.Fatalf("HSP add count changed after debounced request: before=%d after=%d", addsAfterFirst, got)
		}

		stopE2EMotion(t, server)
	})

	t.Run("scenario_3_safety_lock_clamp", func(t *testing.T) {
		saveSettings(t, server.store, func(settings config.Settings) config.Settings {
			settings.Motion.HardwareSafetyLock = true
			return settings
		})

		provider.responses = []string{e2eChaosLLMResponse}
		addsBefore := recorder.hspAddCount()

		body := postChatStream(t, server, `{"message":"e2e safety lock"}`)
		assertMotionApplied(t, body, true)

		waitForHSPDispatchAfter(t, recorder, addsBefore)

		points := collectHSPPointsFromAdds(recorder, addsBefore)
		if len(points) < 2 {
			t.Fatalf("HSP points = %d, want >=2", len(points))
		}
		if min := minConsecutiveDelta(points); min < 30 {
			t.Fatalf("min consecutive delta = %dms, want >=30ms with hardware_safety_lock enabled", min)
		}
	})
}

func saveE2EDeviceSettings(t *testing.T, server *Server) {
	t.Helper()

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
		settings.Device.APIApplicationIDSource = config.ApplicationIDSourceBundled
		settings.Device.HandyConnectionKey = e2eDeviceConnectionKey
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		settings.Motion.HardwareSafetyLock = true
		return settings
	})
}

func requireE2EDeviceReady(t *testing.T, server *Server) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/device/transport", strings.NewReader(
		`{"transport":"cloud_rest","connection_key":"`+e2eDeviceConnectionKey+`"}`,
	))
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

	var check transport.ConnectionCheckResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &check); err != nil {
		t.Fatalf("decode cloud check: %v", err)
	}
	if !check.OK || !check.HSPAvailable {
		t.Skipf("HSP unavailable on device: %+v", check)
	}
}

func installE2ERecordingTransport(t *testing.T, server *Server) *recordingMotionTransport {
	t.Helper()

	cloud, err := server.newCloudTransport()
	if err != nil {
		t.Fatalf("newCloudTransport: %v", err)
	}
	recorder := newRecordingMotionTransport(cloud)

	server.motion.mu.Lock()
	server.motion.transport = recorder
	server.motion.mu.Unlock()

	return recorder
}

func stopE2EMotion(t *testing.T, server *Server) {
	t.Helper()

	_ = postChatStream(t, server, `{"message":"stop"}`)

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/transport/cloud/stop", nil))
	server.Handler().ServeHTTP(recorder, request)
}

func assertMotionApplied(t *testing.T, sseBody string, want bool) {
	t.Helper()

	applied, found := parseMotionSSEApplied(sseBody)
	if !found {
		t.Fatalf("SSE body missing motion event:\n%s", sseBody)
	}
	if applied != want {
		t.Fatalf("motion applied = %v, want %v\n%s", applied, want, sseBody)
	}
}

func parseMotionSSEApplied(body string) (applied bool, found bool) {
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var eventName string
		var dataLine string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event: ") {
				eventName = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
			}
		}
		if eventName != "motion" || dataLine == "" {
			continue
		}
		var payload chatMotionDispatch
		if err := json.Unmarshal([]byte(dataLine), &payload); err != nil {
			continue
		}
		return payload.Applied, true
	}
	return false, false
}

func waitForHSPDispatch(t *testing.T, recorder *recordingMotionTransport) {
	t.Helper()
	waitForHSPDispatchAfter(t, recorder, 0)
}

func waitForHSPDispatchAfter(t *testing.T, recorder *recordingMotionTransport, addsBefore int) {
	t.Helper()

	deadline := time.Now().Add(e2eHSPDispatchTimeout)
	for time.Now().Before(deadline) {
		snap := recorder.snapshot()
		if len(snap.HSPAdds) > addsBefore && len(snap.HSPPlays) > 0 {
			return
		}
		time.Sleep(e2ePollInterval)
	}
	t.Fatalf("timed out waiting for HSP dispatch (adds=%d plays=%d errors=%v)",
		recorder.hspAddCount(), recorder.hspPlayCount(), recorder.snapshot().Errors)
}

func assertRecorderTransportOK(t *testing.T, recorder *recordingMotionTransport) {
	t.Helper()
	assertRecorderTransportOKSince(t, recorder, 0)
}

func assertRecorderTransportOKSince(t *testing.T, recorder *recordingMotionTransport, errorsBefore int) {
	t.Helper()

	errors := recorder.snapshot().Errors
	if len(errors) > errorsBefore {
		t.Fatalf("cloud transport errors: %v", errors[errorsBefore:])
	}
}

func waitForCloudMotionActive(t *testing.T, server *Server, recorder *recordingMotionTransport) string {
	t.Helper()

	active := map[string]bool{
		"playing":    true,
		"buffered":   true,
		"buffering":  true,
		"running":    true,
		"inprogress": true,
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if errors := recorder.snapshot().Errors; len(errors) > 0 {
			t.Fatalf("cloud transport errors before motion active: %v", errors)
		}
		state := fetchCloudPlaybackState(t, server)
		if active[strings.ToLower(state)] {
			return state
		}
		if recorder.hspPlayCount() > 0 && state != "" && state != "idle" && state != "unknown" {
			return state
		}
		time.Sleep(e2ePollInterval)
	}

	if recorder.hspPlayCount() > 0 && len(recorder.snapshot().Errors) == 0 {
		t.Logf("cloud state=%q after successful HSP play — continuing physical hold", fetchCloudPlaybackState(t, server))
		return ""
	}
	t.Fatalf("timed out waiting for cloud motion (state=%q plays=%d errors=%v)",
		fetchCloudPlaybackState(t, server), recorder.hspPlayCount(), recorder.snapshot().Errors)
	return ""
}

func fetchCloudPlaybackState(t *testing.T, server *Server) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/transport/cloud/state", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return ""
	}
	var payload struct {
		State transport.HSPStateSnapshot `json:"state"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode cloud state: %v", err)
	}
	return payload.State.PlaybackState
}

func collectHSPPointsFromAdds(recorder *recordingMotionTransport, skipAdds int) []transport.TimedPoint {
	snap := recorder.snapshot()
	var points []transport.TimedPoint
	for index := skipAdds; index < len(snap.HSPAdds); index++ {
		points = append(points, snap.HSPAdds[index].Points...)
	}
	return points
}

func minConsecutiveDelta(points []transport.TimedPoint) int64 {
	if len(points) < 2 {
		return 0
	}
	minDelta := points[1].TimeMillis - points[0].TimeMillis
	for index := 2; index < len(points); index++ {
		delta := points[index].TimeMillis - points[index-1].TimeMillis
		if delta < minDelta {
			minDelta = delta
		}
	}
	return minDelta
}
