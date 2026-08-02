package httpapi

import "net/http"

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "1"
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.updates.Check(r.Context(), refresh))
}
