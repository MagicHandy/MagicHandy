package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/store"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type motionVisualRuntime struct {
	mu        sync.Mutex
	syncPrefs store.SyncPrefs
	schedule  *transport.OutgoingSchedule
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
	mux.HandleFunc("GET /api/motion/visual/stream", s.handleMotionVisualStream)
}

func (s *Server) handleMotionVisualStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming responses are unavailable"))
		return
	}
	setSSEHeaders(w)
	w.WriteHeader(http.StatusOK)

	emit := func() bool {
		s.visual.mu.Lock()
		offsetMS := s.visual.syncPrefs.OffsetMS
		s.visual.mu.Unlock()
		payload := s.outgoingSchedule().VisualStreamSnapshot(time.Now(), offsetMS)
		if err := writeSSE(w, "visual", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !emit() {
		return
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !emit() {
				return
			}
		}
	}
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

func (s *Server) outgoingSchedule() *transport.OutgoingSchedule {
	s.visual.mu.Lock()
	defer s.visual.mu.Unlock()
	if s.visual.schedule == nil {
		s.visual.schedule = transport.NewOutgoingSchedule()
	}
	return s.visual.schedule
}

func (s *Server) motionVisualPayload(_ *http.Request) map[string]any {
	settings, _ := s.store.Snapshot()
	now := time.Now()

	s.visual.mu.Lock()
	prefs := s.visual.syncPrefs
	s.visual.mu.Unlock()

	positionPct := 50.0
	playbackActive := false
	scheduleView := s.outgoingSchedule().VisualSnapshot(now, prefs.OffsetMS)

	if scheduleView != nil {
		if pos, ok := scheduleView["position_pct"].(float64); ok {
			positionPct = pos
		}
		playbackActive = true
	} else {
		positionPct, playbackActive = s.motionVisualFallbackPosition()
	}

	payload := map[string]any{
		"position_pct":      positionPct,
		"target_pct":        positionPct,
		"offset_ms":         prefs.OffsetMS,
		"stroke_min_pct":    float64(settings.Motion.StrokeMinPercent),
		"stroke_max_pct":    float64(settings.Motion.StrokeMaxPercent),
		"playback_active":   playbackActive,
		"live_position_pct": positionPct,
		"recent":            []map[string]any{},
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
	if scheduleView != nil {
		for key, value := range scheduleView {
			payload[key] = value
		}
	}
	return payload
}

func (s *Server) motionVisualFallbackPosition() (float64, bool) {
	positionPct := 50.0
	playbackActive := false

	s.direct.mu.Lock()
	if s.direct.active {
		playbackActive = true
		if s.direct.lastPositionPct != 0 {
			positionPct = s.direct.lastPositionPct
		}
	}
	s.direct.mu.Unlock()

	if engine := s.currentMotionEngine(); engine != nil {
		snapshot := engine.Snapshot()
		if snapshot.Running || snapshot.Paused {
			playbackActive = true
		}
		if snapshot.LastSample != nil {
			positionPct = float64(snapshot.LastSample.PositionPercent)
		}
	}

	playerSnap, _ := s.manualQueuePlayerSnapshot()
	if proceduralSnap, ok := s.activeProceduralPlayerSnapshot(); ok {
		playerSnap = proceduralSnap
	}
	if playerSnap.Running && !playerSnap.Paused {
		playbackActive = true
		positionPct = playerSnap.PositionPct
	}

	s.manualQueue.mu.Lock()
	if s.manualQueue.playing && !s.manualQueue.paused {
		playbackActive = true
	}
	s.manualQueue.mu.Unlock()

	return positionPct, playbackActive
}
