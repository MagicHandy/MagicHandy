package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func (s *Server) sessionManagementRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/sessions", s.sessionManagementHandler(s.handleOwnSessions))
	mux.HandleFunc("DELETE /api/auth/sessions", s.sessionManagementHandler(s.handleRevokeOtherSessions))
	mux.HandleFunc("PATCH /api/auth/sessions/{id}", s.sessionManagementHandler(s.handleRenameOwnSession))
	mux.HandleFunc("DELETE /api/auth/sessions/{id}", s.sessionManagementHandler(s.handleRevokeOwnSession))
}

func (s *Server) sessionManagementHandler(action func(http.ResponseWriter, *http.Request, accounts.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current, ok := authenticatedSession(r)
		if !ok {
			s.writeAuthenticationRequired(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		action(w, r.WithContext(ctx), current.session)
	}
}

type managedSessionView struct {
	accounts.SessionInfo
	Controller    bool `json:"controller"`
	DeviceGateway bool `json:"device_gateway"`
}

func (s *Server) handleOwnSessions(w http.ResponseWriter, r *http.Request, actor accounts.Session) {
	sessions, err := s.accounts.ListOwnSessions(r.Context(), actor.Key)
	if err != nil {
		s.writeSessionManagementError(w, err)
		return
	}
	controller, gateway := s.controller.SessionKey(), s.bluetoothGatewaySessionKey()
	views := make([]managedSessionView, 0, len(sessions))
	for _, session := range sessions {
		views = append(views, managedSessionView{SessionInfo: session, Controller: controller != "" && session.Key == controller, DeviceGateway: gateway != "" && session.Key == gateway})
	}
	s.writeSessionManagementJSON(w, r, map[string]any{"sessions": views, "limit": accounts.MaxSessionsPerAccount, "current_session_id": actor.ID})
}

func (s *Server) handleRenameOwnSession(w http.ResponseWriter, r *http.Request, actor accounts.Session) {
	if !requireJSONRequest(w, r) {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.accounts.RenameOwnSession(r.Context(), actor.Key, r.PathValue("id"), body.Name); err != nil {
		s.writeSessionManagementError(w, err)
		return
	}
	s.writeSessionManagementJSON(w, r, map[string]bool{"updated": true})
}

func (s *Server) handleRevokeOwnSession(w http.ResponseWriter, r *http.Request, actor accounts.Session) {
	key, err := s.accounts.RevokeOwnSession(r.Context(), actor.Key, r.PathValue("id"))
	if err != nil {
		s.writeSessionManagementError(w, err)
		return
	}
	current := key == actor.Key
	if current {
		s.clearSessionCookie(w)
	}
	s.endRevokedSessions(r.Context(), actor.Key, []string{key})
	s.writeSessionManagementJSON(w, r, map[string]any{"revoked": 1, "current_revoked": current})
}

func (s *Server) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request, actor accounts.Session) {
	keys, err := s.accounts.RevokeOtherSessions(r.Context(), actor.Key)
	if err != nil {
		s.writeSessionManagementError(w, err)
		return
	}
	s.endRevokedSessions(r.Context(), actor.Key, keys)
	s.writeSessionManagementJSON(w, r, map[string]any{"revoked": len(keys), "current_revoked": false})
}

// Once a revocation is durable, its caller may finish a bounded acknowledgement
// even when it revoked itself. Other work loses access immediately. Client and
// server shutdown cancellation remain attached; no new authority is created.
func (s *Server) endRevokedSessions(origin context.Context, actorKey string, keys []string) {
	// Retiring a peer gateway may also stop this controller generation. The
	// already-committed management request still needs its acknowledgement.
	if detach, ok := origin.Value(controllerCancellationKey{}).(func() bool); ok {
		detach()
	}
	for _, key := range keys {
		if key == actorKey {
			if detach, ok := origin.Value(sessionCancellationKey{}).(func() bool); ok {
				detach()
			}
			break
		}
	}
	for _, key := range keys {
		s.access.revoke(key)
		s.checkBluetoothGatewayLifetime(key)
		if s.controller.BeginLoss(key, false) {
			s.scheduleLostControllerStop("controller_session_revoked")
		}
	}
}

func (s *Server) writeSessionManagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, accounts.ErrInvalidSession):
		s.writeAuthenticationRequired(w)
	case errors.Is(err, accounts.ErrManagedSessionNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, accounts.ErrInvalidSessionName):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusServiceUnavailable, errors.New("session management is temporarily unavailable"))
	}
}

func (s *Server) writeSessionManagementJSON(w http.ResponseWriter, r *http.Request, payload any) {
	data, err := json.Marshal(payload)
	if err != nil || len(data) > 32<<10 {
		s.writeSessionManagementError(w, errors.New("session response exceeded its budget"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := writeContextBytes(r.Context(), w, append(data, '\n')); err != nil {
		panic(http.ErrAbortHandler)
	}
}

// Narrow method/shape matching avoids granting future unrelated auth endpoints
// the self-service exception. Domain methods still recheck ownership at apply.
func ownSessionManagementRoute(r *http.Request) bool {
	if r.URL.Path == "/api/auth/sessions" {
		return r.Method == http.MethodDelete
	}
	id, found := strings.CutPrefix(r.URL.Path, "/api/auth/sessions/")
	return found && id != "" && !strings.Contains(id, "/") && (r.Method == http.MethodDelete || r.Method == http.MethodPatch)
}

func sessionClientHint(r *http.Request) accounts.SessionClient {
	agent := r.UserAgent()
	if len(agent) > 512 {
		agent = agent[:512]
	}
	agent = strings.ToLower(agent)
	client := accounts.SessionClient{Browser: "other", Platform: "other"}
	switch {
	case strings.Contains(agent, "edg/") || strings.Contains(agent, "edgios/") || strings.Contains(agent, "edga/"):
		client.Browser = "edge"
	case strings.Contains(agent, "firefox/") || strings.Contains(agent, "fxios/"):
		client.Browser = "firefox"
	case strings.Contains(agent, "chrome/") || strings.Contains(agent, "crios/"):
		client.Browser = "chrome"
	case strings.Contains(agent, "safari/"):
		client.Browser = "safari"
	}
	switch {
	case strings.Contains(agent, "android"):
		client.Platform = "android"
	case strings.Contains(agent, "iphone") || strings.Contains(agent, "ipad") || (strings.Contains(agent, "macintosh") && strings.Contains(agent, "mobile/")):
		client.Platform = "ios"
	case strings.Contains(agent, "windows"):
		client.Platform = "windows"
	case strings.Contains(agent, "macintosh") || strings.Contains(agent, "mac os x"):
		client.Platform = "macos"
	case strings.Contains(agent, "linux"):
		client.Platform = "linux"
	}
	return client
}
