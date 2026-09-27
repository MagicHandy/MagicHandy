package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/media"
)

// Video curation (titles, ratings, notes, tags) is library content, not a
// device command: any account with control permission may edit it, including
// a phone that does not hold the controller lease, and nothing here reaches
// the motion engine. Admission is the control lane in authorizeRoutes.
func (s *Server) mediaMetadataRoutes(mux *http.ServeMux) {
	mux.HandleFunc("PATCH /api/media/videos/{id}/metadata", s.handleMediaMetadata)
	mux.HandleFunc("POST /api/media/videos/tags", s.handleMediaBulkTags)
	mux.HandleFunc("GET /api/media/tags", s.handleMediaTags)
	mux.HandleFunc("POST /api/media/tags/rename", s.handleMediaTagRename)
	mux.HandleFunc("POST /api/media/tags/delete", s.handleMediaTagDelete)
}

func (s *Server) handleMediaMetadata(w http.ResponseWriter, r *http.Request) {
	var patch media.MetadataPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if patch.Title == nil && patch.Rating == nil && patch.Notes == nil && patch.Tags == nil {
		writeError(w, http.StatusBadRequest, errors.New("change at least one of title, rating, notes or tags"))
		return
	}
	video, err := s.media.UpdateMetadata(r.Context(), strings.TrimSpace(r.PathValue("id")), patch)
	if err != nil {
		s.writeMetadataError(w, err, "video details could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"video": s.clientVideos(r, []media.Video{video})[0]})
}

func (s *Server) handleMediaBulkTags(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []string `json:"ids"`
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	videos, err := s.media.UpdateTags(r.Context(), body.IDs, body.Add, body.Remove)
	if err != nil {
		s.writeMetadataError(w, err, "tags could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": s.clientVideos(r, videos)})
}

func (s *Server) handleMediaTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.media.Tags(r.Context())
	if err != nil {
		s.logger.Error("media tag list failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("tags could not be loaded"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handleMediaTagRename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	renamed, err := s.media.RenameTag(r.Context(), body.From, body.To)
	if err != nil {
		s.writeMetadataError(w, err, "the tag could not be renamed")
		return
	}
	s.writeTagList(w, r, map[string]any{"renamed": renamed})
}

func (s *Server) handleMediaTagDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tag string `json:"tag"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	removed, err := s.media.DeleteTag(r.Context(), body.Tag)
	if err != nil {
		s.writeMetadataError(w, err, "the tag could not be removed")
		return
	}
	s.writeTagList(w, r, map[string]any{"removed": removed})
}

// writeTagList answers a library-wide tag edit with the resulting tag list, so
// the client does not need a second read to redraw its filters.
func (s *Server) writeTagList(w http.ResponseWriter, r *http.Request, payload map[string]any) {
	tags, err := s.media.Tags(r.Context())
	if err != nil {
		s.logger.Error("media tag list failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("tags could not be loaded"))
		return
	}
	payload["tags"] = tags
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) writeMetadataError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, media.ErrInvalidMetadata):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, media.ErrVideoNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		s.logger.Error("media metadata write failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New(fallback))
	}
}
