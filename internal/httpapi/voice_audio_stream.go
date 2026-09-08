package httpapi

import (
	"errors"
	"net/http"
	"strconv"
)

func (s *Server) handleVoiceRequestAudioChunk(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 || offset > 8<<20 {
		writeError(w, http.StatusBadRequest, errors.New("invalid audio offset"))
		return
	}
	pending, ok := s.voice.Request(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("unknown voice request"))
		return
	}
	chunk, err := pending.AudioChunk(offset)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, chunk)
}
