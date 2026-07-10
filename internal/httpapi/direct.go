package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/funscript"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

var (
	errLibraryUnavailable = errors.New("pattern library is unavailable")
	errImportExtension    = errors.New("allowed extensions: .csv, .json, .funscript")
	errImportTooLarge     = errors.New("upload exceeds size limit")
	errDirectUnavailable  = errors.New("direct control is unavailable for the configured transport")
)

type directRuntime struct {
	mu              sync.Mutex
	active          bool
	recording       bool
	recordActions   []funscript.StoredAction
	recordStarted   time.Time
	lastSend        time.Time
	lastPositionPct float64
	streamID        string
}

func (s *Server) registerDirectRoutes(mux *http.ServeMux) {
	s.registerMotionVisualRoutes(mux)
	mux.HandleFunc("GET /api/motion/visual", s.handleMotionVisual)
	mux.HandleFunc("POST /api/motion/direct/start", s.handleDirectStart)
	mux.HandleFunc("POST /api/motion/direct", s.handleDirectMove)
	mux.HandleFunc("POST /api/motion/direct/stop", s.handleDirectStop)
	mux.HandleFunc("POST /api/motion/direct/recording/start", s.handleDirectRecordingStart)
	mux.HandleFunc("POST /api/motion/direct/recording/stop", s.handleDirectRecordingStop)
}

func (s *Server) handleMotionVisual(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.motionVisualPayload(r))
}

func (s *Server) handleDirectStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		// #region agent log
		agentDebugLog("H2", "direct.go:handleDirectStart", "controller_denied", map[string]any{"client_id": clientIDFromRequest(r)})
		// #endregion
		return
	}
	ctx := r.Context()
	s.stopManualQueuePlayer(ctx)
	s.stopAndClearMotionEngine(ctx, "direct_start")

	commandTransport, err := s.newSelectedMotionTransport()
	if err != nil {
		// #region agent log
		agentDebugLog("H2", "direct.go:handleDirectStart", "transport_unavailable", map[string]any{"error": err.Error()})
		// #endregion
		writeDirectError(w, err)
		return
	}
	stroke := transport.StrokeWindowCommand{MinPercent: 0, MaxPercent: 100}
	if direct, ok := commandTransport.(transport.DirectMotionCapable); ok {
		if err := direct.PrepareDirectMotion(ctx, stroke); err != nil {
			// #region agent log
			agentDebugLog("H5", "direct.go:handleDirectStart", "hdsp_prepare_failed", map[string]any{"error": err.Error()})
			// #endregion
			writeDirectError(w, err)
			return
		}
		// #region agent log
		agentDebugLog("H5", "direct.go:handleDirectStart", "hdsp_prepare_ok", map[string]any{
			"transport": commandTransport.Diagnostics().Name,
		})
		// #endregion
	} else {
		_, _ = commandTransport.SetStrokeWindow(ctx, stroke)
	}
	s.direct.mu.Lock()
	s.direct.active = true
	s.direct.streamID = "direct"
	s.direct.lastSend = time.Time{}
	s.direct.lastPositionPct = 50
	s.direct.mu.Unlock()

	// #region agent log
	agentDebugLog("H4", "direct.go:handleDirectStart", "direct_started", map[string]any{
		"transport": commandTransport.Diagnostics().Name,
	})
	// #endregion

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"min_pct":        0,
		"max_pct":        100,
		"transport":      commandTransport.Diagnostics().Name,
		"limits_enabled": true,
	})
}

func (s *Server) handleDirectMove(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		// #region agent log
		agentDebugLog("H2", "direct.go:handleDirectMove", "controller_denied", nil)
		// #endregion
		return
	}
	var body struct {
		Normalized float64 `json:"normalized"`
		DurationMS int     `json:"duration_ms"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.DurationMS <= 0 {
		body.DurationMS = 66
	}
	if body.DurationMS > 10000 {
		body.DurationMS = 10000
	}

	s.direct.mu.Lock()
	if !s.direct.active {
		s.direct.mu.Unlock()
		// #region agent log
		agentDebugLog("H3", "direct.go:handleDirectMove", "direct_not_active", map[string]any{
			"normalized": body.Normalized,
		})
		// #endregion
		writeDirectError(w, errors.New("direct control is not active"))
		return
	}
	now := time.Now()
	positionPct := directPositionFromNormalized(body.Normalized, 0, 100)
	recording := s.direct.recording
	if recording {
		atMS := 0
		if !s.direct.recordStarted.IsZero() {
			atMS = int(now.Sub(s.direct.recordStarted).Milliseconds())
		}
		s.direct.recordActions = append(s.direct.recordActions, funscript.StoredAction{
			At:  atMS,
			Pos: positionPct,
		})
	}
	streamID := s.direct.streamID
	s.direct.mu.Unlock()

	commandTransport, err := s.newSelectedMotionTransport()
	if err != nil {
		writeDirectError(w, err)
		return
	}
	ctx := r.Context()

	if direct, ok := commandTransport.(transport.DirectMotionCapable); ok {
		s.direct.mu.Lock()
		s.direct.lastSend = now
		s.direct.lastPositionPct = positionPct
		s.direct.mu.Unlock()

		if err := s.sendDirectMoveHDSP(ctx, direct, body.Normalized, body.DurationMS, positionPct, commandTransport.Diagnostics().Name); err != nil {
			writeDirectError(w, err)
			return
		}
		s.outgoingSchedule().RecordDirectMove(positionPct, body.DurationMS)
		writeDirectMoveOK(w, positionPct, body.DurationMS, "hdsp")
		return
	}

	s.direct.mu.Lock()
	if !s.direct.lastSend.IsZero() && now.Sub(s.direct.lastSend) < 16*time.Millisecond {
		s.direct.mu.Unlock()
		// #region agent log
		agentDebugLog("H5", "direct.go:handleDirectMove", "server_skipped_rate_limit", map[string]any{
			"normalized": body.Normalized,
		})
		// #endregion
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": true})
		return
	}
	s.direct.lastSend = now
	s.direct.lastPositionPct = positionPct
	s.direct.mu.Unlock()

	if err := s.sendDirectMoveHSP(ctx, commandTransport, streamID, body.Normalized, positionPct, body.DurationMS, commandTransport.Diagnostics().Name); err != nil {
		writeDirectError(w, err)
		return
	}
	writeDirectMoveOK(w, positionPct, body.DurationMS, "hsp")
}

func writeDirectMoveOK(w http.ResponseWriter, positionPct float64, durationMS int, path string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"position_pct": positionPct,
		"duration_ms":  durationMS,
		"path":         path,
	})
}

func (s *Server) sendDirectMoveHDSP(
	ctx context.Context,
	direct transport.DirectMotionCapable,
	normalized float64,
	durationMS int,
	positionPct float64,
	transportName string,
) error {
	err := direct.EnqueueDirectMove(ctx, transport.DirectMoveCommand{
		Normalized:   normalized,
		DurationMS:   durationMS,
		StrokeMinPct: 0,
		StrokeMaxPct: 100,
	})
	if err != nil {
		agentDebugLog("H5", "direct.go:handleDirectMove", "hdsp_enqueue_failed", map[string]any{
			"error": err.Error(), "position_pct": positionPct,
		})
		return err
	}
	agentDebugLog("H5", "direct.go:handleDirectMove", "hdsp_enqueue_ok", map[string]any{
		"normalized": normalized, "position_pct": positionPct, "duration_ms": durationMS,
		"transport": transportName,
	})
	return nil
}

func (s *Server) sendDirectMoveHSP(
	ctx context.Context,
	commandTransport transport.Transport,
	streamID string,
	normalized float64,
	positionPct float64,
	durationMS int,
	transportName string,
) error {
	_, err := commandTransport.AddHSP(ctx, transport.HSPAddCommand{
		StreamID: streamID,
		Points: []transport.TimedPoint{{
			PositionPercent: int(positionPct + 0.5),
			TimeMillis:      0,
		}},
	})
	if err != nil {
		agentDebugLog("H5", "direct.go:handleDirectMove", "hsp_add_failed", map[string]any{
			"error": err.Error(), "position_pct": positionPct,
		})
		return err
	}
	_, err = commandTransport.PlayHSP(ctx, transport.HSPPlayCommand{
		StreamID:        streamID,
		StartTimeMillis: 0,
	})
	if err != nil {
		agentDebugLog("H5", "direct.go:handleDirectMove", "hsp_play_failed", map[string]any{
			"error": err.Error(), "position_pct": positionPct,
		})
		return err
	}
	agentDebugLog("H5", "direct.go:handleDirectMove", "hsp_move_ok", map[string]any{
		"normalized": normalized, "position_pct": positionPct, "duration_ms": durationMS,
		"transport": transportName,
	})
	return nil
}

func (s *Server) handleDirectStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.direct.mu.Lock()
	s.direct.active = false
	s.direct.recording = false
	s.direct.mu.Unlock()

	if commandTransport, err := s.newSelectedMotionTransport(); err == nil {
		if direct, ok := commandTransport.(transport.DirectMotionCapable); ok {
			_ = direct.StopDirectMotion(r.Context())
		} else {
			_, _ = commandTransport.Stop(r.Context(), transport.StopCommand{Reason: "direct_stop"})
		}
	}
	s.outgoingSchedule().Reset()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDirectRecordingStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	s.direct.mu.Lock()
	defer s.direct.mu.Unlock()
	if !s.direct.active {
		writeDirectError(w, errors.New("direct control is not active"))
		return
	}
	s.direct.recording = true
	s.direct.recordActions = nil
	s.direct.recordStarted = time.Now()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"recording":    true,
		"action_count": 0,
	})
}

func (s *Server) handleDirectRecordingStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		Title *string `json:"title"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}

	s.direct.mu.Lock()
	s.direct.recording = false
	actions := append([]funscript.StoredAction(nil), s.direct.recordActions...)
	s.direct.recordActions = nil
	s.direct.mu.Unlock()

	title := "Mouse recording"
	if body.Title != nil && *body.Title != "" {
		title = *body.Title
	}
	durationMS := 0
	if len(actions) > 0 {
		durationMS = actions[len(actions)-1].At
	}

	payload := map[string]any{
		"ok":            true,
		"recording":     false,
		"display_title": title,
		"duration_ms":   durationMS,
		"action_count":  len(actions),
		"favorite":      false,
	}

	if s.library != nil && len(actions) >= 2 {
		loaded := funscript.LoadedFunscript{
			Actions:      storedToActions(actions),
			SourceFormat: funscript.SourceFormatFunscript,
			Metadata: map[string]any{
				"name":    title,
				"creator": "mouse_recording",
			},
			SourcePath: "",
		}
		result, err := funscript.Ingest(loaded, title+".funscript")
		if err == nil {
			for i := range result.Blocks {
				result.Blocks[i].Tags = append(result.Blocks[i].Tags, "user_recorded", "mouse_recording")
			}
			if persisted, err := s.library.PersistIngestResult(r.Context(), result); err == nil {
				payload["block_id"] = persisted.FullBlockID
				payload["file_id"] = persisted.FileID
			}
		}
	}

	writeJSON(w, http.StatusOK, payload)
}

func directPositionFromNormalized(normalized float64, minPct, maxPct int) float64 {
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	lo := float64(minPct)
	hi := float64(maxPct)
	return lo + normalized*(hi-lo)
}

func writeDirectError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, errDirectUnavailable), errors.Is(err, errMotionUnavailable):
		status = http.StatusServiceUnavailable
	case strings.Contains(message, "not connected"):
		status = http.StatusServiceUnavailable
	case strings.Contains(message, "not active"):
		status = http.StatusConflict
	}
	writeError(w, status, err)
}

func decodeStringJSON(raw string, target any) error {
	return json.Unmarshal([]byte(raw), target)
}
