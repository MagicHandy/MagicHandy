package httpapi

import (
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func (s *Server) handleSessionPolicy(w http.ResponseWriter, r *http.Request, actor accounts.Session) {
	if _, ok := s.requireAdministrator(w, r); !ok {
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			IdleMinutes *int `json:"idle_minutes"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if body.IdleMinutes == nil {
			writeError(w, http.StatusBadRequest, errors.New("idle_minutes is required; use 0 to disable idle sign-out"))
			return
		}
		retired, err := s.accounts.SetSessionPolicy(r.Context(), actor.Key, *body.IdleMinutes)
		if errors.Is(err, accounts.ErrInvalidSessionPolicy) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err != nil {
			s.writeSessionManagementError(w, err)
			return
		}
		s.endRevokedSessions(r.Context(), actor.Key, retired)
	}
	policy, err := s.accounts.SessionPolicy(r.Context())
	if err != nil {
		s.writeSessionManagementError(w, err)
		return
	}
	s.writeSessionManagementJSON(w, r, policy)
}
