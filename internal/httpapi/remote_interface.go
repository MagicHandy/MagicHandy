package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

type interfaceContextKey struct{}

func requestInterface(r *http.Request) string {
	if r != nil && r.Context().Value(interfaceContextKey{}) == accounts.InterfaceRemote {
		return accounts.InterfaceRemote
	}
	return accounts.InterfaceFull
}

// RemoteInterfaceOptions describes an independently validated socket boundary.
// Both interfaces borrow the same domains and Stop path; no engine or worker is
// duplicated. Cookies have distinct names AND server-side session audiences.
type RemoteInterfaceOptions struct {
	AllowedBrowserHosts []string
	NetworkPolicy       *netaccess.Policy
	SecureCookies       bool
}

// RemoteHandler exposes an explicit allowlist. Even administrator credentials
// cannot turn this listener into the main app, a file server or a controller.
func (s *Server) RemoteHandler(options RemoteInterfaceOptions) http.Handler {
	mux := http.NewServeMux()
	s.publicShellRoutes(mux)
	s.authenticationPortalRoutes(mux)
	s.remoteClientRoutes(mux)
	admitted := s.admitHTTPRequests(s.authenticateRequests(s.trackSessionActivity(mux)))
	browser := protectBrowserRequests(options.AllowedBrowserHosts, admitted)
	protected := logRequests(s.logger, securityHeaders(options.SecureCookies, protectNetworkPolicy(options.NetworkPolicy, browser)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protected.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), interfaceContextKey{}, accounts.InterfaceRemote)))
	})
}

func (s *Server) handleAccountInterfaceAccess(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdministrator(w, r); !ok {
		return
	}
	actor, ok := authenticatedSession(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	var body struct {
		Access string `json:"interface_access"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	keys, err := s.accounts.SetInterfaceAccessForSession(r.Context(), actor.session.Key, r.PathValue("id"), body.Access)
	if err != nil {
		if errors.Is(err, accounts.ErrInvalidSession) {
			s.writeAuthenticationRequired(w)
		} else {
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	s.endRevokedSessions(r.Context(), actor.session.Key, keys)
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// Common registrations stay in one inventory; each listener explicitly chooses
// only the surfaces it serves.
func (s *Server) publicShellRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /api/motion/stop", s.handleMotionStop)
	mux.HandleFunc("GET /", s.handleStatic)
}
func (s *Server) authenticationPortalRoutes(mux *http.ServeMux) {
	s.sessionManagementRoutes(mux)
	s.accountRecoveryRoutes(mux)
	s.noticePreferenceRoutes(mux)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthenticationStatus)
	mux.HandleFunc("POST /api/auth/login", credentialHandler(s.handleAuthenticationLogin))
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthenticationLogout)
	mux.HandleFunc("PUT /api/auth/password", credentialHandler(s.handleAuthenticationPassword))
}
func (s *Server) remoteClientRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/remote/state", s.handleRemoteState)
	mux.HandleFunc("GET /api/remote/events", s.handleRemoteEvents)
	mux.HandleFunc("POST /api/remote/commands", s.handleRemoteCommand)
	mux.HandleFunc("GET /api/remote/videos", s.handleRemoteVideos)
	mux.HandleFunc("GET /api/remote/chat/messages", s.handleRemoteMessages)
}
