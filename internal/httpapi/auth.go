package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/audit"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

const (
	secureSessionCookieName   = "__Host-MagicHandy-Session"
	loopbackSessionCookieName = "MagicHandy-Session"
	sessionLookupTimeout      = time.Second
)

var errAuthenticationThrottled = errors.New("too many authentication attempts; try again later")

type authenticationOptions struct {
	Required      bool
	SecureCookies bool
}

type authenticationRuntime struct {
	options        authenticationOptions
	limiter        *loginLimiter
	required       *atomic.Bool
	unprotectedCtx context.Context
	endUnprotected context.CancelFunc
}

type authenticatedAccountContextKey struct{}
type authenticatedSessionContextKey struct{}

type authenticatedSessionState struct {
	session accounts.Session
	token   string
}

func newAuthenticationRuntime(options authenticationOptions, initialized bool) authenticationRuntime {
	required := &atomic.Bool{}
	required.Store(options.Required || initialized)
	unprotected, cancel := context.WithCancel(context.Background())
	if required.Load() {
		cancel()
	}
	return authenticationRuntime{options: options, limiter: newLoginLimiter(), required: required,
		unprotectedCtx: unprotected, endUnprotected: cancel}
}

func (a authenticationRuntime) authenticationRequired() bool {
	return a.required.Load()
}

func (a authenticationRuntime) requireAuthentication() {
	a.required.Store(true)
	a.endUnprotected()
}

func newAuthenticationComponents(store *config.Store, runtime Runtime) (*accounts.Store, authenticationRuntime, error) {
	accountStore := runtime.Accounts
	if accountStore == nil {
		var err error
		accountStore, err = accounts.New(store.Datastore())
		if err != nil {
			return nil, authenticationRuntime{}, err
		}
	}
	initialized, err := accountStore.Initialized(context.Background())
	if err != nil {
		return nil, authenticationRuntime{}, err
	}
	authRuntime := newAuthenticationRuntime(authenticationOptions{
		Required:      runtime.AuthenticationRequired,
		SecureCookies: runtime.SecureCookies,
	}, initialized)
	return accountStore, authRuntime, nil
}

func (s *Server) authenticationRoutes(mux *http.ServeMux) {
	s.controlGrantRoutes(mux)
	s.sessionManagementRoutes(mux)
	s.accountRecoveryRoutes(mux)
	s.noticePreferenceRoutes(mux)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthenticationStatus)
	mux.HandleFunc("POST /api/auth/bootstrap", s.handleAuthenticationBootstrap)
	mux.HandleFunc("POST /api/auth/login", credentialHandler(s.handleAuthenticationLogin))
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthenticationLogout)
	mux.HandleFunc("PUT /api/auth/password", credentialHandler(s.handleAuthenticationPassword))
	mux.HandleFunc("GET /api/auth/control-identities", s.handleControlIdentities)
	mux.HandleFunc("PUT /api/auth/control-identity", s.handleControlIdentity)
	mux.HandleFunc("PUT /api/auth/profile-image", s.handleProfileImageUpload)
	mux.HandleFunc("DELETE /api/auth/profile-image", s.handleProfileImageDelete)
	mux.HandleFunc("GET /api/accounts", s.handleAccountsList)
	mux.HandleFunc("POST /api/accounts", s.handleAccountCreate)
	mux.HandleFunc("PUT /api/accounts/{id}/password", credentialHandler(s.handleAccountPassword))
	mux.HandleFunc("PUT /api/accounts/{id}/disabled", s.handleAccountDisabled)
	mux.HandleFunc("GET /api/accounts/{id}/profile-image", s.handleProfileImage)
}

func (s *Server) authenticateRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/motion/stop" {
			// Stop is public and must not wait for session/database admission.
			next.ServeHTTP(w, r.WithContext(audit.WithActor(r.Context(), audit.Actor{Type: "public"})))
			return
		}
		if readRequest(r) && !strings.HasPrefix(r.URL.Path, "/api/") {
			// The public shell and health endpoint must load during a datastore
			// outage too, including when this browser already has a login cookie.
			next.ServeHTTP(w, r)
			return
		}
		if session, token, err := s.sessionFromRequest(r); err == nil {
			ctx := context.WithValue(r.Context(), authenticatedAccountContextKey{}, session.Account)
			ctx = context.WithValue(ctx, authenticatedSessionContextKey{}, authenticatedSessionState{session: session, token: token})
			ctx = audit.WithActor(ctx, audit.Actor{Type: "account", AccountID: session.Account.ID, SessionID: session.ID})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		} else if !errors.Is(err, accounts.ErrInvalidSession) {
			finishUnreadBody(w, r)
			w.Header().Set("Retry-After", "1")
			writeBoundedJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session verification is temporarily unavailable; retry shortly"})
			return
		} else if token != "" {
			s.clearSessionCookie(w)
		}

		if s.auth.authenticationRequired() && !isPublicAuthenticationRequest(r) {
			finishUnreadBody(w, r)
			s.writeAuthenticationRequired(w)
			return
		}
		actorType := "public"
		if !s.auth.authenticationRequired() {
			actorType = "local"
		}
		next.ServeHTTP(w, r.WithContext(audit.WithActor(r.Context(), audit.Actor{Type: actorType})))
	})
}

func isPublicAuthenticationRequest(r *http.Request) bool {
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && !strings.HasPrefix(r.URL.Path, "/api/") {
		// The embedded shell must load before the React login boundary can ask
		// for credentials. No API or private state is exposed by this exception.
		return true
	}
	switch r.URL.Path {
	case "/healthz", "/api/auth/status", "/api/auth/bootstrap", "/api/auth/login":
		return true
	case "/api/motion/stop":
		// Stop remains a fail-safe operation even if a browser session expires.
		// Same-origin browser enforcement still runs outside this middleware.
		return r.Method == http.MethodPost
	case "/api/auth/recover":
		return r.Method == http.MethodPost
	case "/api/notice-preferences":
		// Only fixed explanatory notices in this browser's private preference
		// record are available before sign-in. No host setting is exposed.
		return readRequest(r) || r.Method == http.MethodPut || r.Method == http.MethodDelete
	default:
		return false
	}
}

func (s *Server) sessionFromRequest(r *http.Request) (accounts.Session, string, error) {
	cookie, err := r.Cookie(s.sessionCookieName())
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return accounts.Session{}, "", accounts.ErrInvalidSession
	}
	token := strings.TrimSpace(cookie.Value)
	resolve := s.accounts.ResolveSession
	if passiveSessionRequest(r) {
		resolve = s.accounts.InspectSession
	}
	ctx, cancel := context.WithTimeout(r.Context(), sessionLookupTimeout)
	defer cancel()
	session, err := resolve(ctx, token)
	if err != nil {
		return accounts.Session{}, token, err
	}
	return session, token, nil
}

func (s *Server) authenticatePassword(r *http.Request, username, password string) (accounts.Account, bool, error) {
	return s.authenticateAccount(r, username, func() (accounts.Account, error) {
		return s.accounts.Authenticate(r.Context(), username, password)
	})
}

func (s *Server) authenticateAccount(r *http.Request, username string, authenticate func() (accounts.Account, error)) (accounts.Account, bool, error) {
	if !s.allowCredentialAttempt(r, username) {
		return accounts.Account{}, false, errAuthenticationThrottled
	}
	account, err := authenticate()
	if errors.Is(err, accounts.ErrInvalidCredentials) {
		s.recordRejectedLogin(r, nil)
		return accounts.Account{}, false, nil
	}
	if err != nil {
		return accounts.Account{}, false, err
	}
	return account, true, nil
}

func (s *Server) allowCredentialAttempt(r *http.Request, username string) bool {
	address := netaccess.ClientIP(r)
	usernameKey := strings.ToLower(strings.TrimSpace(username))
	if !s.auth.limiter.Allow(address, usernameKey) {
		s.recordRejectedLogin(r, errAuthenticationThrottled)
		return false
	}
	return true
}

func (s *Server) writeAuthenticationRequired(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	writeBoundedJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
}

func (s *Server) sessionCookieName() string {
	if s.auth.options.SecureCookies {
		return secureSessionCookieName
	}
	return loopbackSessionCookieName
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	// #nosec G124 -- Secure is mandatory in TLS mode and deliberately false only
	// for the trusted loopback-HTTP mode; HttpOnly and SameSite stay mandatory.
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.auth.options.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Cache-Control", "no-store")
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	// #nosec G124 -- deletion must exactly match the mode-specific cookie flags;
	// the non-Secure variant exists only for trusted loopback HTTP.
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.auth.options.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	w.Header().Set("Cache-Control", "no-store")
}

func authenticatedAccount(r *http.Request) (accounts.Account, bool) {
	account, ok := r.Context().Value(authenticatedAccountContextKey{}).(accounts.Account)
	return account, ok
}

func authenticatedSession(r *http.Request) (authenticatedSessionState, bool) {
	session, ok := r.Context().Value(authenticatedSessionContextKey{}).(authenticatedSessionState)
	return session, ok
}

func (s *Server) requireAdministrator(w http.ResponseWriter, r *http.Request) (accounts.Account, bool) {
	account, ok := authenticatedAccount(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return accounts.Account{}, false
	}
	if account.Role != accounts.RoleAdmin || account.Disabled {
		writeError(w, http.StatusForbidden, errors.New("administrator access required"))
		return accounts.Account{}, false
	}
	return account, true
}

func (s *Server) handleAuthenticationStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), sessionLookupTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	initialized, err := s.accounts.Initialized(r.Context())
	if err != nil {
		writeBoundedJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account status is unavailable"})
		return
	}
	account, authenticated := authenticatedAccount(r)
	settings, _ := s.store.PublicSnapshot()
	controlIdentities := []accounts.ControlIdentity(nil)
	currentSessionID := ""
	if session, ok := authenticatedSession(r); ok {
		currentSessionID = session.session.ID
		controlIdentities, err = s.accounts.ControlIdentities(r.Context(), account.ID, session.session.ControlAccountID)
		if err != nil {
			s.logger.Warn("control identities could not be listed", "error", err)
			writeBoundedJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account status is unavailable"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized":             initialized,
		"authentication_required": s.auth.authenticationRequired(),
		"authenticated":           authenticated,
		"account":                 optionalAccount(account, authenticated),
		"bootstrap_available":     isLocalHostRequest(r),
		"ui_locale":               settings.UI.Locale,
		"control_identities":      controlIdentities,
		"session_id":              currentSessionID,
		"capabilities":            s.capabilities(r),
	})
}

func optionalAccount(account accounts.Account, present bool) any {
	if !present {
		return nil
	}
	return account
}

func (s *Server) handleAuthenticationBootstrap(w http.ResponseWriter, r *http.Request) {
	if !isLocalHostRequest(r) || !isSameOriginBrowserRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("the first account can be created only from the computer running MagicHandy"))
		return
	}
	if !requireJSONRequest(w, r) {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	account, err := s.accounts.BootstrapAdmin(r.Context(), body.Username, body.Password)
	if errors.Is(err, accounts.ErrAlreadyInitialized) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		if isAccountInputError(err) {
			writeError(w, http.StatusBadRequest, err)
		} else {
			s.logger.Warn("initial administrator account could not be created", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("the initial account could not be created"))
		}
		return
	}
	// Account existence is the durable opt-in to loopback password protection.
	// Switch the live middleware before session creation so a partial failure
	// fails closed; the newly created credentials can still use JSON login.
	s.auth.requireAuthentication()
	token, session, err := s.accounts.NewSessionWithClient(r.Context(), account.ID, sessionClientHint(r))
	if err != nil {
		if s.controller.BeginLocalLoss() {
			s.stopLostController("account_protection_enabled")
		}
		writeError(w, http.StatusInternalServerError, errors.New("the initial account was created but a session could not be started"))
		return
	}
	s.bootstrapController(r.WithContext(audit.WithActor(r.Context(), audit.Actor{Type: "account", AccountID: account.ID, SessionID: session.ID})), session.Key)
	s.setSessionCookie(w, token)
	s.logger.Info("initial administrator account created", "account_id", account.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"account": account})
}

func (s *Server) handleAuthenticationPassword(w http.ResponseWriter, r *http.Request) {
	current, authenticated := authenticatedSession(r)
	if !authenticated {
		s.writeAuthenticationRequired(w)
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeCredentialRequest(w, r, &body) {
		return
	}
	if !s.allowCredentialAttempt(r, current.session.Account.Username) {
		writeError(w, http.StatusTooManyRequests, errAuthenticationThrottled)
		return
	}
	keys, err := s.accounts.ChangeOwnPassword(r.Context(), current.session.Key, body.CurrentPassword, body.NewPassword)
	if err != nil {
		if errors.Is(err, accounts.ErrInvalidPassword) {
			writeError(w, http.StatusBadRequest, err)
		} else {
			if errors.Is(err, accounts.ErrInvalidCredentials) {
				s.recordRejectedLogin(r, nil)
			}
			s.writeRecoveryError(w, err)
		}
		return
	}
	s.clearSessionCookie(w)
	w.Header().Set("Clear-Site-Data", `"cookies"`)
	s.endRevokedSessions(r.Context(), current.session.Key, keys)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleControlIdentities(w http.ResponseWriter, r *http.Request) {
	session, ok := authenticatedSession(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	identities, err := s.accounts.ControlIdentities(r.Context(), session.session.Account.ID, session.session.ControlAccountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("control identities are unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"control_identities": identities})
}

func (s *Server) handleControlIdentity(w http.ResponseWriter, r *http.Request) {
	session, ok := authenticatedSession(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	if !requireJSONRequest(w, r) {
		return
	}
	var body struct {
		AccountID string `json:"account_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.accounts.SetControlIdentity(r.Context(), session.token, body.AccountID); err != nil {
		if errors.Is(err, accounts.ErrControlIdentityNotAllowed) {
			writeError(w, http.StatusForbidden, err)
		} else if errors.Is(err, accounts.ErrInvalidSession) {
			s.writeAuthenticationRequired(w)
		} else {
			s.logger.Warn("control identity could not be changed", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("control identity could not be changed"))
		}
		return
	}
	identities, err := s.accounts.ControlIdentities(r.Context(), session.session.Account.ID, strings.TrimSpace(body.AccountID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("control identities are unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"control_identities": identities})
}

func (s *Server) handleProfileImageUpload(w http.ResponseWriter, r *http.Request) {
	account, ok := authenticatedAccount(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, accounts.MaxProfileImageBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("profile image could not be read"))
		return
	}
	if len(data) > accounts.MaxProfileImageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("profile image is too large"))
		return
	}
	updated, err := s.accounts.SaveProfileImage(r.Context(), account.ID, data)
	if err != nil {
		if errors.Is(err, accounts.ErrProfileImageInvalid) {
			writeError(w, http.StatusBadRequest, err)
		} else {
			s.logger.Warn("account profile image could not be saved", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("profile image could not be saved"))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": updated})
}

func (s *Server) handleProfileImageDelete(w http.ResponseWriter, r *http.Request) {
	account, ok := authenticatedAccount(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	updated, err := s.accounts.DeleteProfileImage(r.Context(), account.ID)
	if err != nil {
		s.logger.Warn("account profile image could not be removed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("profile image could not be removed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": updated})
}

func (s *Server) handleProfileImage(w http.ResponseWriter, r *http.Request) {
	viewer, ok := authenticatedAccount(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	targetID := strings.TrimSpace(r.PathValue("id"))
	allowed, err := s.accounts.CanViewProfile(r.Context(), viewer, targetID)
	if err != nil || !allowed {
		http.NotFound(w, r)
		return
	}
	file, err := s.accounts.OpenProfileImage(r.Context(), targetID)
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
	w.Header().Set("Cache-Control", "private, max-age=60")
	serveBoundedContent(w, r, "profile.jpg", info.ModTime(), file)
}

func (s *Server) handleAuthenticationLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeCredentialRequest(w, r, &body) {
		return
	}
	var token string
	var session accounts.Session
	_, allowed, err := s.authenticateAccount(r, body.Username, func() (accounts.Account, error) {
		var err error
		token, session, err = s.accounts.LoginWithClient(r.Context(), body.Username, body.Password, sessionClientHint(r))
		return session.Account, err
	})
	if errors.Is(err, errAuthenticationThrottled) {
		writeError(w, http.StatusTooManyRequests, errAuthenticationThrottled)
		return
	}
	if err != nil {
		s.logger.Warn("account login failed internally", "error", err)
		writeError(w, http.StatusServiceUnavailable, errors.New("login is temporarily unavailable"))
		return
	}
	if !allowed {
		writeError(w, http.StatusUnauthorized, accounts.ErrInvalidCredentials)
		return
	}
	s.setSessionCookie(w, token)
	writeBoundedJSON(w, http.StatusOK, session)
}

func (s *Server) handleAuthenticationLogout(w http.ResponseWriter, r *http.Request) {
	current, ok := authenticatedSession(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	key, err := s.accounts.RevokeOwnSession(r.Context(), current.session.Key, current.session.ID)
	if err != nil {
		s.writeSessionManagementError(w, err)
		return
	}
	s.clearSessionCookie(w)
	w.Header().Set("Clear-Site-Data", `"cookies"`)
	s.endRevokedSessions(r.Context(), key, []string{key})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAccountsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdministrator(w, r); !ok {
		return
	}
	listed, err := s.accounts.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("accounts are unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": listed})
}

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdministrator(w, r); !ok {
		return
	}
	if !requireJSONRequest(w, r) {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	account, err := s.accounts.Create(r.Context(), body.Username, body.Password, body.Role)
	if err != nil {
		if isAccountInputError(err) || errors.Is(err, accounts.ErrUsernameTaken) {
			writeError(w, http.StatusBadRequest, err)
		} else {
			s.logger.Warn("user account could not be created", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("account could not be created"))
		}
		return
	}
	s.logger.Info("user account created", "account_id", account.ID, "role", account.Role)
	writeJSON(w, http.StatusCreated, map[string]any{"account": account})
}

func (s *Server) handleAccountPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdministrator(w, r); !ok {
		return
	}
	current, ok := authenticatedSession(r)
	if !ok {
		s.writeAuthenticationRequired(w)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decodeCredentialRequest(w, r, &body) {
		return
	}
	keys, err := s.accounts.ResetPasswordForSession(r.Context(), current.session.Key, r.PathValue("id"), body.Password)
	if err != nil {
		switch {
		case errors.Is(err, accounts.ErrNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, accounts.ErrInvalidPassword):
			writeError(w, http.StatusBadRequest, err)
		case errors.Is(err, accounts.ErrInvalidSession):
			s.writeAuthenticationRequired(w)
		default:
			s.logger.Warn("account password could not be changed", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("account password could not be changed"))
		}
		return
	}
	if current.session.Account.ID == r.PathValue("id") {
		s.clearSessionCookie(w)
	}
	s.endRevokedSessions(r.Context(), current.session.Key, keys)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAccountDisabled(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdministrator(w, r); !ok {
		return
	}
	if !requireJSONRequest(w, r) {
		return
	}
	var body struct {
		Disabled bool `json:"disabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.accounts.SetDisabled(r.Context(), r.PathValue("id"), body.Disabled); err != nil {
		switch {
		case errors.Is(err, accounts.ErrNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, accounts.ErrLastAdmin):
			writeError(w, http.StatusConflict, err)
		default:
			s.logger.Warn("account enabled state could not be changed", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("account enabled state could not be changed"))
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isAccountInputError(err error) bool {
	return errors.Is(err, accounts.ErrInvalidUsername) ||
		errors.Is(err, accounts.ErrInvalidPassword) ||
		errors.Is(err, accounts.ErrInvalidRole)
}

func requireJSONRequest(w http.ResponseWriter, r *http.Request) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("application/json is required"))
		return false
	}
	return true
}

func securityHeaders(secure bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "base-uri 'self'; frame-ancestors 'none'; object-src 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if secure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}
