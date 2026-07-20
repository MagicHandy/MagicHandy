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
	mux.HandleFunc("POST /api/media/scan", s.handleMediaScanStart)
	mux.HandleFunc("GET /api/media/scan", s.handleMediaScanState)
	mux.HandleFunc("DELETE /api/media/scan", s.handleMediaScanCancel)
	mux.HandleFunc("POST /api/media/duration", s.handleMediaDuration)
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
	contentType := mime.TypeByExtension(filepath.Ext(video.RelativePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(video.RelativePath), info.ModTime(), file)
}

func (s *Server) mediaState(ctx context.Context) map[string]any {
	summary, err := s.media.Summary(ctx)
	if err != nil {
		return map[string]any{"available": false, "error": "media catalog unavailable"}
	}
	return map[string]any{
		"available":       true,
		"video_count":     summary.VideoCount,
		"available_count": summary.AvailableCount,
		"paired_count":    summary.PairedCount,
		"scan":            s.media.ScanState(),
	}
}
