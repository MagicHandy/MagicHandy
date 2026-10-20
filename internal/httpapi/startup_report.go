package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/startup"
)

func (s *Server) handleStartupReport(w http.ResponseWriter, r *http.Request) {
	if !s.requireNetworkAdministrator(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/api/startup/report" {
		w.Header().Set("Content-Disposition", `attachment; filename="magichandy-startup-report.json"`)
	}
	writeJSON(w, http.StatusOK, startup.ReadReports(s.store.DataDir()))
}
