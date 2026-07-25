package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/media"
)

const maxReportedVideoDurationMillis = int64(30 * 24 * 60 * 60 * 1000)

func (s *Server) mediaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/videos", s.handleMediaVideos)
	mux.HandleFunc("GET /api/media/videos/{id}/stream", s.handleMediaVideoStream)
	mux.HandleFunc("GET /api/media/videos/{id}/funscript", s.handleMediaVideoFunscript)
	mux.HandleFunc("POST /api/media/sync", s.handleMediaSync)
	mux.HandleFunc("POST /api/media/scan", s.handleMediaScanStart)
	mux.HandleFunc("GET /api/media/scan", s.handleMediaScanState)
	mux.HandleFunc("DELETE /api/media/scan", s.handleMediaScanCancel)
	mux.HandleFunc("POST /api/media/duration", s.handleMediaDuration)
	mux.HandleFunc("POST /api/media/script-offset", s.handleMediaScriptOffset)
	mux.HandleFunc("POST /api/media/playback", s.handleMediaPlayback)
}

func (s *Server) handleMediaVideoFunscript(w http.ResponseWriter, r *http.Request) {
	videoID := strings.TrimSpace(r.PathValue("id"))
	script, err := s.media.LoadFunscript(r.Context(), videoID)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrVideoNotFound), errors.Is(err, media.ErrVideoUnavailable),
			errors.Is(err, media.ErrFunscriptNotFound), errors.Is(err, media.ErrFunscriptUnavailable):
			writeError(w, http.StatusNotFound, errors.New("paired funscript is unavailable"))
		case errors.Is(err, media.ErrFunscriptTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, err)
		case errors.Is(err, media.ErrFunscriptInvalid):
			writeError(w, http.StatusUnprocessableEntity, err)
		default:
			s.logger.Error("media funscript load failed", "video_id", videoID, "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("paired funscript could not be loaded"))
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"funscript": script})
}

func (s *Server) handleMediaSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence, err := s.requestStopSequence(r)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	var event mediaSyncEvent
	if err := decodeJSON(r, &event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	event.VideoID = strings.TrimSpace(event.VideoID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.State = strings.TrimSpace(event.State)
	event.Event = strings.TrimSpace(event.Event)
	if err := validateMediaSyncEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status, err := s.mediaSync.Handle(r.Context(), event, stopSequence)
	if err != nil {
		writeSyncError := func(statusCode int, message string) {
			responseStatus := status
			if responseStatus.Message == "" {
				responseStatus.Message = message
			}
			writeJSON(w, statusCode, map[string]any{"sync": responseStatus, "error": message})
		}
		switch {
		case errors.Is(err, errMediaMotionInterrupted):
			writeSyncError(http.StatusConflict, err.Error())
		case errors.Is(err, media.ErrVideoNotFound), errors.Is(err, media.ErrFunscriptNotFound),
			errors.Is(err, media.ErrFunscriptUnavailable):
			writeSyncError(http.StatusNotFound, "paired funscript is unavailable")
		case errors.Is(err, media.ErrFunscriptTooLarge):
			writeSyncError(http.StatusRequestEntityTooLarge, err.Error())
		case errors.Is(err, media.ErrFunscriptInvalid):
			writeSyncError(http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, errMotionUnavailable), errors.Is(err, errServerQuiescing):
			writeSyncError(http.StatusServiceUnavailable, s.safeMotionErrorMessage(err))
		default:
			safeError := s.safeMotionErrorMessage(err)
			s.logger.Warn("media synchronization failed", "video_id", event.VideoID, "event", event.Event, "error", safeError)
			writeSyncError(http.StatusBadGateway, safeError)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sync": status})
}

func validateMediaSyncEvent(event mediaSyncEvent) error {
	if event.VideoID == "" || event.MediaTimeMillis < 0 || event.MediaTimeMillis > maxReportedVideoDurationMillis {
		return errors.New("video id and a valid media time are required")
	}
	if event.SessionID == "" || cleanControllerClientID(event.SessionID) != event.SessionID || event.EventSequence == 0 {
		return errors.New("a valid media playback session and event sequence are required")
	}
	if event.PlaybackRate < 0.25 || event.PlaybackRate > 4 {
		return errors.New("video playback rate must be between 0.25 and 4")
	}
	switch event.State {
	case "playing":
		switch event.Event {
		case "play", "seeked", "ratechange", "resync", "heartbeat":
		default:
			return errors.New("playing media sync requires a valid event")
		}
	case "paused", "seeking", "ended", "closed":
		if strings.TrimSpace(event.Event) == "" {
			return errors.New("media sync event is required")
		}
	default:
		return errors.New("unsupported media sync state")
	}
	return nil
}

func (s *Server) handleMediaVideos(w http.ResponseWriter, r *http.Request) {
	videos, err := s.media.List(r.Context())
	if err != nil {
		s.logger.Error("media catalog list failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("media catalog could not be loaded"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": videos})
}

func (s *Server) handleMediaScanStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	settings, _ := s.store.Snapshot()
	state, err := s.media.StartScan(settings.Media.LibraryPaths)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrNoLocations):
			writeError(w, http.StatusBadRequest, err)
		case errors.Is(err, media.ErrScanBusy):
			writeError(w, http.StatusConflict, err)
		default:
			writeError(w, http.StatusInternalServerError, errors.New("media scan could not be started"))
		}
		return
	}
	s.logger.Info("media library scan started", "locations", len(settings.Media.LibraryPaths))
	writeJSON(w, http.StatusAccepted, map[string]any{"scan": state})
}

func (s *Server) handleMediaScanState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"scan": s.media.ScanState()})
}

func (s *Server) handleMediaScanCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scan": s.media.CancelScan()})
}

func (s *Server) handleMediaDuration(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		ID             string `json:"id"`
		DurationMillis int64  `json:"duration_ms"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body.ID = strings.TrimSpace(body.ID)
	if body.ID == "" || body.DurationMillis <= 0 || body.DurationMillis > maxReportedVideoDurationMillis {
		writeError(w, http.StatusBadRequest, errors.New("video id and a valid duration are required"))
		return
	}
	if err := s.media.SetDuration(r.Context(), body.ID, body.DurationMillis); err != nil {
		if errors.Is(err, media.ErrVideoNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, errors.New("video duration could not be saved"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved"})
}

// handleMediaPlayback patches only the paired-playback filters, so the floating
// panel does not have to round-trip the whole settings document to change one
// dial. Saving runs the normal settings path, which stops an active run because
// filters change points the device has already accepted.
func (s *Server) handleMediaPlayback(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		ScriptSmoothingPercent *int  `json:"script_smoothing_percent,omitempty"`
		PeakRoundingMillis     *int  `json:"peak_rounding_ms,omitempty"`
		ApplyVideoSpeedLimit   *bool `json:"apply_video_speed_limit,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	current, _ := s.store.Snapshot()
	previous := current
	if body.ScriptSmoothingPercent != nil {
		current.Media.ScriptSmoothingPercent = *body.ScriptSmoothingPercent
	}
	if body.PeakRoundingMillis != nil {
		current.Media.PeakRoundingMillis = *body.PeakRoundingMillis
	}
	if body.ApplyVideoSpeedLimit != nil {
		current.Motion.ApplyVideoSpeedLimit = *body.ApplyVideoSpeedLimit
	}
	saved, err := s.store.Save(current)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Filters change the points a run was built from, so the shared settings
	// transition stops an active clock-locked run exactly as a full save would.
	runtimeErr := s.applySettingsRuntimeTransition(r.Context(), previous, saved)
	payload := map[string]any{
		"media":  saved.Public().Media,
		"motion": saved.Public().Motion,
	}
	status := http.StatusOK
	if runtimeErr != nil {
		status = http.StatusBadGateway
		payload["error"] = "filters were saved, but the active run could not apply them"
	}
	writeJSON(w, status, payload)
}

// handleMediaScriptOffset stores this pairing's calibration and, when the video
// is the one currently playing, applies it to the live run. Nothing stops: the
// offset moves the clock the video follows, not the points the device holds.
func (s *Server) handleMediaScriptOffset(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		ID           string `json:"id"`
		OffsetMillis int    `json:"script_offset_ms"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body.ID = strings.TrimSpace(body.ID)
	if body.ID == "" {
		writeError(w, http.StatusBadRequest, errors.New("video id is required"))
		return
	}
	if err := s.media.SetScriptOffset(r.Context(), body.ID, body.OffsetMillis); err != nil {
		if errors.Is(err, media.ErrVideoNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload := map[string]any{"status": "saved", "script_offset_ms": body.OffsetMillis}
	if s.mediaSync != nil {
		settings, _ := s.store.Snapshot()
		payload["sync"] = s.mediaSync.SetScriptOffset(
			body.ID,
			settings.Media.ScriptOffsetMillis+body.OffsetMillis,
		)
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleMediaVideoStream(w http.ResponseWriter, r *http.Request) {
	videoID := r.PathValue("id")
	file, video, err := s.media.OpenVideo(r.Context(), videoID)
	if err != nil {
		if errors.Is(err, media.ErrVideoNotFound) || errors.Is(err, media.ErrVideoUnavailable) {
			s.logger.Warn("media stream unavailable", "video_id", videoID, "reason", err)
			http.NotFound(w, r)
			return
		}
		s.logger.Error("media stream resolution failed", "video_id", videoID, "error", err)
		http.Error(w, "video could not be opened", http.StatusInternalServerError)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mediaContentType(video.RelativePath)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(video.RelativePath), info.ModTime(), file)
}

func mediaContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	default:
		return mime.TypeByExtension(filepath.Ext(path))
	}
}

func (s *Server) mediaState(ctx context.Context) map[string]any {
	summary, err := s.media.Summary(ctx)
	if err != nil {
		return map[string]any{"available": false, "error": "media catalog unavailable"}
	}
	state := map[string]any{
		"available":       true,
		"video_count":     summary.VideoCount,
		"available_count": summary.AvailableCount,
		"paired_count":    summary.PairedCount,
		"scan":            s.media.ScanState(),
	}
	if s.mediaSync != nil {
		state["sync"] = s.mediaSync.Status()
	}
	return state
}
