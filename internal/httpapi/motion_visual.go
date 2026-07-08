package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/store"
)

type visualSample struct {
	TMS    int64   `json:"t_ms"`
	PosPct float64 `json:"pos_pct"`
	At     time.Time
}

type motionVisualRuntime struct {
	mu        sync.Mutex
	origin    time.Time
	recent    []visualSample
	syncPrefs store.SyncPrefs
}

func (s *Server) loadSyncPrefs() {
	if s.store == nil || s.store.DB() == nil {
		return
	}
	prefs, err := s.store.DB().LoadSyncPrefs(context.Background())
	if err != nil {
		return
	}
	s.visual.mu.Lock()
	s.visual.syncPrefs = prefs
	s.visual.mu.Unlock()
}

func (s *Server) registerMotionVisualRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/motion/sync-offset", s.handleMotionSyncOffset)
	mux.HandleFunc("POST /api/motion/auto-sync", s.handleMotionAutoSync)
	mux.HandleFunc("GET /api/motion/direct/status", s.handleDirectStatus)
}

func (s *Server) handleMotionSyncOffset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OffsetMS int `json:"offset_ms"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.OffsetMS < -2000 || body.OffsetMS > 2000 {
		writeError(w, http.StatusBadRequest, errors.New("offset_ms must be between -2000 and 2000"))
		return
	}
	s.visual.mu.Lock()
	prefs := s.visual.syncPrefs
	prefs.OffsetMS = body.OffsetMS
	s.visual.syncPrefs = prefs
	s.visual.mu.Unlock()
	if s.store != nil && s.store.DB() != nil {
		_ = s.store.DB().SaveSyncPrefs(r.Context(), prefs)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"offset_ms": body.OffsetMS,
	})
}

func (s *Server) handleMotionAutoSync(w http.ResponseWriter, r *http.Request) {
	body := s.computeAndSaveAutoSync(r.Context())
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) autoSyncOffset(r *http.Request) map[string]any {
	return s.computeAndSaveAutoSync(r.Context())
}

func (s *Server) computeAndSaveAutoSync(ctx context.Context) map[string]any {
	rtt := int64(200)
	if diag := s.cloudDiagnostics(); diag.LastLatencyMillis > 0 {
		rtt = diag.LastLatencyMillis
	} else if cloud, err := s.newCloudTransport(); err == nil {
		if check, err := cloud.CheckConnection(ctx); err == nil && check.LatencyMillis > 0 {
			rtt = check.LatencyMillis
			s.saveCloudDiagnostics(cloud.Diagnostics())
		}
	}
	deviceLat := int(rtt * 18 / 10)
	if deviceLat < 80 {
		deviceLat = 80
	}
	if deviceLat > 450 {
		deviceLat = 450
	}
	clientLat := int(rtt * 9 / 10)
	offsetMS := -int(max(80, min(450, int64(deviceLat+clientLat/2))))

	measured := int(rtt)
	prefs := store.SyncPrefs{
		OffsetMS:        offsetMS,
		MeasuredRTTMS:   &measured,
		DeviceLatencyMS: &deviceLat,
		ClientLatencyMS: &clientLat,
	}
	s.visual.mu.Lock()
	s.visual.syncPrefs = prefs
	s.visual.mu.Unlock()
	if s.store != nil && s.store.DB() != nil {
		_ = s.store.DB().SaveSyncPrefs(ctx, prefs)
	}
	return map[string]any{
		"ok":                true,
		"offset_ms":         offsetMS,
		"measured_rtt_ms":   measured,
		"device_latency_ms": deviceLat,
		"client_latency_ms": clientLat,
	}
}

func (s *Server) handleDirectStatus(w http.ResponseWriter, _ *http.Request) {
	s.direct.mu.Lock()
	active := s.direct.active
	recording := s.direct.recording
	actionCount := len(s.direct.recordActions)
	s.direct.mu.Unlock()
	payload := map[string]any{
		"ok":                     true,
		"active":                 active,
		"min_pct":                0,
		"max_pct":                100,
		"recording":              recording,
		"recording_action_count": actionCount,
		"limits_enabled":         true,
	}
	if transport, err := s.newSelectedMotionTransport(); err == nil {
		payload["transport"] = transport.Diagnostics().Name
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) motionVisualPayload(_ *http.Request) map[string]any {
	settings, _ := s.store.Snapshot()
	positionPct := s.direct.lastPositionPct
	if positionPct == 0 {
		positionPct = 50
	}
	playbackActive := s.direct.active

	if engine := s.currentMotionEngine(); engine != nil {
		snapshot := engine.Snapshot()
		if snapshot.Running || snapshot.Paused {
			playbackActive = true
		}
		if snapshot.LastSample != nil {
			positionPct = float64(snapshot.LastSample.PositionPercent)
		}
	}

	s.manualQueue.mu.Lock()
	mqPlaying := s.manualQueue.playing && !s.manualQueue.paused
	s.manualQueue.mu.Unlock()

	playerSnap, playerActions := s.manualQueuePlayerSnapshot()
	if playerSnap.Running {
		playbackActive = true
		if !playerSnap.Paused {
			positionPct = playerSnap.PositionPct
		}
	} else if mqPlaying {
		playbackActive = true
	}

	s.visual.mu.Lock()
	prefs := s.visual.syncPrefs
	if s.visual.origin.IsZero() {
		s.visual.origin = time.Now()
	}
	s.recordVisualSampleLocked(positionPct)
	recent := s.visualRecentLocked(6000)
	s.visual.mu.Unlock()

	payload := map[string]any{
		"position_pct":      positionPct,
		"target_pct":        positionPct,
		"offset_ms":         prefs.OffsetMS,
		"stroke_min_pct":    float64(settings.Motion.StrokeMinPercent),
		"stroke_max_pct":    float64(settings.Motion.StrokeMaxPercent),
		"playback_active":   playbackActive,
		"recent":            recent,
		"live_position_pct": positionPct,
	}
	if prefs.MeasuredRTTMS != nil {
		payload["measured_rtt_ms"] = *prefs.MeasuredRTTMS
	}
	if prefs.DeviceLatencyMS != nil {
		payload["device_latency_ms"] = *prefs.DeviceLatencyMS
	}
	if prefs.ClientLatencyMS != nil {
		payload["client_latency_ms"] = *prefs.ClientLatencyMS
	}
	if curve := s.motionVisualCurveFromPlayer(playerSnap, playerActions); curve != nil {
		for key, value := range curve {
			payload[key] = value
		}
	}
	return payload
}

func (s *Server) motionVisualCurveFromPlayer(snap manualqueue.Snapshot, actions []manualqueue.Action) map[string]any {
	if !snap.Running || snap.Paused || len(actions) == 0 {
		return nil
	}
	curveActions := make([]map[string]int, len(actions))
	for i, action := range actions {
		curveActions[i] = map[string]int{"at": action.At, "pos": action.Pos}
	}
	return map[string]any{
		"curve_actions":     curveActions,
		"curve_elapsed_ms":  snap.PlayheadMS,
		"curve_duration_ms": snap.DurationMS,
	}
}

func (s *Server) recordVisualSampleLocked(positionPct float64) {
	now := time.Now()
	if len(s.visual.recent) > 0 {
		last := s.visual.recent[len(s.visual.recent)-1]
		if now.Sub(last.At) < 40*time.Millisecond && almostEqual(last.PosPct, positionPct) {
			return
		}
	}
	tms := now.Sub(s.visual.origin).Milliseconds()
	s.visual.recent = append(s.visual.recent, visualSample{
		TMS:    tms,
		PosPct: positionPct,
		At:     now,
	})
	cutoff := now.Add(-6 * time.Second)
	trim := 0
	for i, sample := range s.visual.recent {
		if sample.At.After(cutoff) {
			trim = i
			break
		}
	}
	if trim > 0 && trim < len(s.visual.recent) {
		s.visual.recent = s.visual.recent[trim:]
	}
	if len(s.visual.recent) > 512 {
		s.visual.recent = s.visual.recent[len(s.visual.recent)-512:]
	}
}

func (s *Server) visualRecentLocked(windowMS int64) []map[string]any {
	if len(s.visual.recent) == 0 {
		return []map[string]any{}
	}
	end := s.visual.recent[len(s.visual.recent)-1].TMS
	start := end - windowMS
	if start < 0 {
		start = 0
	}
	out := make([]map[string]any, 0, len(s.visual.recent))
	for _, sample := range s.visual.recent {
		if sample.TMS < start {
			continue
		}
		out = append(out, map[string]any{
			"t_ms":    sample.TMS - start,
			"pos_pct": sample.PosPct,
		})
	}
	return out
}

func almostEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.25
}

// NOTE: Go 1.25+ provides built-in `min`/`max` for ordered types, so we
// intentionally avoid helper functions with those names.
