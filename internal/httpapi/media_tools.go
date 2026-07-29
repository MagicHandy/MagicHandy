package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/media"
)

func (s *Server) mediaToolRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/media/videos/{id}/thumbnail", s.handleMediaThumbnail)
	mux.HandleFunc("POST /api/media/videos/{id}/thumbnail", s.handleMediaThumbnailUpload)
	mux.HandleFunc("POST /api/media/compatibility", s.handleMediaCompatibility)
	mux.HandleFunc("GET /api/media/tools", s.handleMediaTools)
	mux.HandleFunc("GET /api/media/job", s.handleMediaJobState)
	mux.HandleFunc("DELETE /api/media/job", s.handleMediaJobCancel)
	mux.HandleFunc("POST /api/media/thumbnails", s.handleMediaThumbnailJob)
	mux.HandleFunc("DELETE /api/media/thumbnails", s.handleMediaThumbnailPurge)
	mux.HandleFunc("POST /api/media/convert", s.handleMediaConvert)
}

// mediaTools resolves the configured FFmpeg. Absent is a normal answer, so the
// status carries the reason rather than the call returning a bare error.
func (s *Server) mediaTools(ctx context.Context) (media.Tools, media.ToolStatus) {
	settings, _ := s.store.Snapshot()
	configured := strings.TrimSpace(settings.Media.FFmpegPath)
	status := media.ToolStatus{Configured: configured != ""}
	if configured == "" {
		return media.Tools{}, status
	}
	tools, err := media.ResolveTools(ctx, configured)
	if err != nil {
		status.Error = err.Error()
		status.FFmpegPath = configured
		return media.Tools{}, status
	}
	status.Available = true
	status.FFmpegPath = tools.FFmpegPath
	status.Version = tools.Version
	return tools, status
}

func (s *Server) handleMediaTools(w http.ResponseWriter, r *http.Request) {
	_, status := s.mediaTools(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"tools": status})
}

// requireMediaTools writes the standard unavailable response so every feature
// that needs FFmpeg fails the same way and says the same thing.
func (s *Server) requireMediaTools(w http.ResponseWriter, r *http.Request) (media.Tools, bool) {
	tools, status := s.mediaTools(r.Context())
	if status.Available {
		return tools, true
	}
	message := "FFmpeg is not configured. Set its path in Settings > Media."
	if status.Error != "" {
		message = status.Error
	}
	writeJSON(w, http.StatusPreconditionFailed, map[string]any{
		"error": message,
		"tools": status,
	})
	return media.Tools{}, false
}

func (s *Server) handleMediaThumbnail(w http.ResponseWriter, r *http.Request) {
	file, err := s.media.OpenThumbnail(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Covers change only when regenerated, and the row's timestamp changes with
	// them, so a short cache keeps a scrolling grid from refetching every tile.
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeContent(w, r, "thumbnail.jpg", info.ModTime(), file)
}

// handleMediaThumbnailUpload accepts a cover the browser captured from a video
// it had already decoded. No new dependency: the frame is free because the
// decode already happened for playback.
func (s *Server) handleMediaThumbnailUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	videoID := strings.TrimSpace(r.PathValue("id"))
	image, err := io.ReadAll(io.LimitReader(r.Body, media.MaxThumbnailBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("thumbnail could not be read"))
		return
	}
	if len(image) > media.MaxThumbnailBytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("thumbnail is too large"))
		return
	}
	if err := s.media.SaveThumbnail(r.Context(), videoID, image); err != nil {
		switch {
		case errors.Is(err, media.ErrVideoNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, media.ErrThumbnailInvalid):
			writeError(w, http.StatusUnsupportedMediaType, err)
		default:
			s.logger.Warn("thumbnail save failed", "video_id", videoID, "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("thumbnail could not be saved"))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved"})
}

// handleMediaCompatibility records what playback revealed about a file.
//
// This is the signal no extension check can produce. An .mp4 holding HEVC is a
// perfectly ordinary catalog row until a browser without an HEVC decoder — a
// Firefox, on most platforms — refuses it. The element's own error code is the
// most authoritative evidence available: it is this browser, on this machine,
// right now, rather than a table of claimed support.
func (s *Server) handleMediaCompatibility(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		ID    string `json:"id"`
		State string `json:"compatibility"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body.ID = strings.TrimSpace(body.ID)
	state := media.Compatibility(strings.TrimSpace(body.State))
	// Only what playback can witness. "unsupported_container" is the scanner's
	// to assign, and a client must not be able to talk the app out of a repair
	// offer by asserting a state it did not observe.
	if state != media.CompatibilityPlayable && state != media.CompatibilityUnsupportedCodec {
		writeError(w, http.StatusBadRequest, errors.New("unsupported playback compatibility report"))
		return
	}
	if err := s.media.SetCompatibility(r.Context(), body.ID, state); err != nil {
		if errors.Is(err, media.ErrVideoNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, errors.New("playback result could not be saved"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved", "compatibility": state})
}

// scanFollowUp builds the opt-in work that rides a successful scan. A startup
// scan uses the same saved choices as a manually started scan.
//
// Conversion is queued after thumbnails because it is the far longer job, and
// because StartConversionJob would be refused while the other still holds the
// single job slot.
func (s *Server) scanFollowUp(settings config.MediaSettings) func(media.ScanState) {
	if !settings.GenerateThumbnailsOnScan && !settings.ConvertIncompatibleOnScan {
		return nil
	}
	return func(_ media.ScanState) {
		ctx := context.Background()
		tools, status := s.mediaTools(ctx)
		if !status.Available {
			s.logger.Warn("scan follow-up skipped", "reason", "ffmpeg unavailable", "error", status.Error)
			return
		}
		if settings.GenerateThumbnailsOnScan {
			if _, err := s.media.StartThumbnailJob(ctx, tools, false); err != nil {
				s.logger.Warn("scan thumbnail follow-up failed", "error", err)
				return
			}
			s.media.WaitForJob()
		}
		if !settings.ConvertIncompatibleOnScan {
			return
		}
		if _, err := s.media.StartConversionJob(ctx, tools, settings, nil); err != nil {
			s.logger.Info("scan conversion follow-up skipped", "reason", err)
		}
	}
}

func (s *Server) handleMediaJobState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"job": s.media.JobState()})
}

func (s *Server) handleMediaJobCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": s.media.CancelJob()})
}

func (s *Server) handleMediaThumbnailJob(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	tools, ok := s.requireMediaTools(w, r)
	if !ok {
		return
	}
	var body struct {
		Redo bool `json:"redo"`
	}
	_ = decodeJSON(r, &body)
	state, err := s.media.StartThumbnailJob(r.Context(), tools, body.Redo)
	s.writeJobStart(w, state, err)
}

func (s *Server) handleMediaThumbnailPurge(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	removed, err := s.media.ClearThumbnails(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("thumbnails could not be cleared"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cleared", "removed": removed})
}

// handleMediaConvert repairs files that cannot play. An empty id list sweeps
// the library; either way only established-incompatible files are converted.
func (s *Server) handleMediaConvert(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	tools, ok := s.requireMediaTools(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	_ = decodeJSON(r, &body)
	const maxConversionSelection = 500
	if len(body.IDs) > maxConversionSelection {
		writeError(w, http.StatusBadRequest, errors.New("too many files selected"))
		return
	}
	settings, _ := s.store.Snapshot()
	state, err := s.media.StartConversionJob(r.Context(), tools, settings.Media, body.IDs)
	s.writeJobStart(w, state, err)
}

func (s *Server) writeJobStart(w http.ResponseWriter, state media.JobState, err error) {
	if err == nil {
		writeJSON(w, http.StatusAccepted, map[string]any{"job": state})
		return
	}
	switch {
	case errors.Is(err, media.ErrJobBusy):
		writeJSON(w, http.StatusConflict, map[string]any{"job": state, "error": err.Error()})
	case errors.Is(err, media.ErrClosed):
		writeError(w, http.StatusServiceUnavailable, errors.New("the app is shutting down"))
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}
