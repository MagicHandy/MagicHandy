package httpapi

import (
	"net/http"
	"strings"
)

// Method alone cannot distinguish user activity from browser bookkeeping.
// Keep automatic acknowledgements passive, including media sync until its
// bounded, decoded event identifies an explicit playback action.
func passiveSessionRequest(r *http.Request) bool {
	if readRequest(r) {
		return true
	}
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/auth/logout", "/api/controller/heartbeat", "/api/chat/cursor",
		"/api/transport/bluetooth/status", "/api/transport/bluetooth/ack",
		"/api/transport/bluetooth/disconnect", "/api/media/sync", "/api/media/duration":
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/api/voice/requests/") && strings.HasSuffix(r.URL.Path, "/played")
}

func (s *Server) recordSessionActivity(w http.ResponseWriter, r *http.Request) bool {
	if session, ok := authenticatedSession(r); ok {
		if _, err := s.accounts.ResolveSession(r.Context(), session.token); err != nil {
			s.writeAuthenticationRequired(w)
			return false
		}
	}
	return true
}

func explicitMediaActivity(event string) bool {
	switch event {
	case "play", "pause", "seeking", "seeked", "ratechange", "resync":
		return true
	default:
		return false
	}
}
