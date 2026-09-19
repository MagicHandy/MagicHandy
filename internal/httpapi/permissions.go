package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

type accountCapabilities struct {
	Control       bool `json:"control"`
	ConfigureHost bool `json:"configure_host"`
	SharedData    bool `json:"shared_data"`
}

const administratorHostAccessRequired = "administrator access required for host configuration, files and module management"

func (s *Server) capabilities(r *http.Request) accountCapabilities {
	if session, ok := authenticatedSession(r); ok {
		return accountCapabilities{Control: session.session.CanControl(time.Now()),
			ConfigureHost: session.session.Account.Role == accounts.RoleAdmin, SharedData: true}
	}
	local := !s.auth.authenticationRequired()
	return accountCapabilities{Control: local, ConfigureHost: local, SharedData: local}
}

// Every mutation outside the explicit control/self-service allowlist requires
// an administrator. Adding a new host operation cannot accidentally grant it to
// operators. Route handlers still enforce ownership, origins and body bounds.
func (s *Server) authorizeRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicAuthenticationRequest(r) || !s.auth.authenticationRequired() {
			next.ServeHTTP(w, r)
			return
		}
		capabilities := s.capabilities(r)
		if capabilities.ConfigureHost || selfServiceRoute(r) || bluetoothGatewaySessionRoute(r) || (readRequest(r) && !hostPrivateRead(r.URL.Path)) {
			next.ServeHTTP(w, r)
			return
		}
		if capabilities.Control && controlRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		message := administratorHostAccessRequired
		if controlRoute(r) {
			message = "this account is an observer; ask the administrator for a control permission"
		}
		rejectRequest(w, r, http.StatusForbidden, errors.New(message))
	})
}

func readRequest(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead
}

func hostPrivateRead(route string) bool {
	return hostDiagnosticsRoute(route) || route == "/api/audit" || strings.HasPrefix(route, "/api/audit/") || route == "/api/network" || strings.HasPrefix(route, "/api/network/") ||
		route == "/api/setup" || strings.HasPrefix(route, "/api/setup/") ||
		(strings.HasPrefix(route, "/api/llm/") && route != "/api/llm/status") ||
		route == "/api/media/tools" || route == "/api/accounts" ||
		(strings.HasPrefix(route, "/api/accounts/") && !singleResourceAction(route, "/api/accounts/", "/profile-image"))
}

func hostDiagnosticsRoute(route string) bool {
	if strings.HasPrefix(route, "/api/labs/") || strings.HasPrefix(route, "/api/diagnostics/") || route == "/api/traces" || strings.HasPrefix(route, "/api/traces/") {
		return true
	}
	switch route {
	case "/api/transport/cloud/state", "/api/transport/cloud/events", "/api/transport/cloud/diagnostics",
		"/api/transport/bluetooth/state", "/api/transport/bluetooth/events", "/api/transport/bluetooth/diagnostics",
		"/api/transport/intiface/diagnostics":
		return true
	default:
		return false
	}
}

func singleResourceAction(route, prefix, action string) bool {
	id, ok := strings.CutPrefix(route, prefix)
	if !ok || !strings.HasSuffix(id, action) {
		return false
	}
	id = strings.TrimSuffix(id, action)
	return id != "" && !strings.Contains(id, "/")
}

func selfServiceRoute(r *http.Request) bool {
	if r.URL.Path == "/api/auth/recovery-codes" && (r.Method == http.MethodPost || r.Method == http.MethodDelete) {
		return true
	}
	if ownSessionManagementRoute(r) {
		return true
	}
	switch r.URL.Path {
	case "/api/auth/logout", "/api/auth/password", "/api/auth/control-identity", "/api/auth/profile-image", "/api/controller/heartbeat", "/api/chat/cursor":
		return true
	default:
		return false
	}
}

func controlRoute(r *http.Request) bool {
	route := r.URL.Path
	if strings.HasPrefix(route, "/api/motion/lab/") {
		return false
	}
	if strings.HasPrefix(route, "/api/motion/") || strings.HasPrefix(route, "/api/modes/") || strings.HasPrefix(route, "/api/chat/") {
		return true
	}
	if strings.HasPrefix(route, "/api/voice/requests/") ||
		(strings.HasPrefix(route, "/api/library/") && strings.HasSuffix(route, "/play")) ||
		singleResourceAction(route, "/api/library/feedback/", "/undo") {
		return true
	}
	switch route {
	case "/api/controller/takeover", "/api/media/sync", "/api/media/duration", "/api/media/script-offset", "/api/media/playback",
		"/api/voice/transcriptions", "/api/voice/preferences", "/api/voice/input-preferences", "/api/library/feedback",
		"/api/transport/bluetooth/status", "/api/transport/bluetooth/ack", "/api/settings/llm-motion-mode":
		return true
	default:
		return false
	}
}

func (s *Server) controlGrantRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/accounts/{id}/control-grant", s.handleControlGrant)
	mux.HandleFunc("PUT /api/accounts/{id}/control-grant", s.handleControlGrant)
	mux.HandleFunc("DELETE /api/accounts/{id}/control-grant", s.handleControlGrant)
}

func (s *Server) handleControlGrant(w http.ResponseWriter, r *http.Request) {
	administrator, ok := s.requireAdministrator(w, r)
	if !ok {
		return
	}
	accountID := r.PathValue("id")
	var grant *accounts.ControlGrant
	var err error
	switch r.Method {
	case http.MethodGet:
		grant, err = s.accounts.ControlGrant(r.Context(), accountID)
	case http.MethodPut:
		var body struct {
			DurationMinutes int `json:"duration_minutes"`
		}
		if decodeErr := decodeJSON(r, &body); decodeErr != nil {
			writeError(w, http.StatusBadRequest, decodeErr)
			return
		}
		if body.DurationMinutes < 1 || body.DurationMinutes > 720 {
			writeError(w, http.StatusBadRequest, errors.New("control permission duration must be from 1 to 720 minutes"))
			return
		}
		grant, err = s.accounts.GrantControl(r.Context(), administrator.ID, accountID, time.Duration(body.DurationMinutes)*time.Minute)
	case http.MethodDelete:
		err = s.accounts.RevokeControl(r.Context(), administrator.ID, accountID)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("control permission could not be read or changed; check that the operator is enabled"))
		return
	}
	if !readRequest(r) {
		// Fence old ownership before acknowledging either revocation or grant
		// replacement. The periodic watchdog is the fallback for external edits.
		s.checkAccessLifetimes()
	}
	writeJSON(w, http.StatusOK, map[string]any{"grant": grant})
}
