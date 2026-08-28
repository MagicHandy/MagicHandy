package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

const (
	secureSessionCookieName   = "__Host-MagicHandy-Session"
	loopbackSessionCookieName = "MagicHandy-Session"
)

var errAuthenticationThrottled = errors.New("too many authentication attempts; try again later")

type authenticationOptions struct {
	Required      bool
	SecureCookies bool
}

type authenticationRuntime struct {
	options  authenticationOptions
	limiter  *loginLimiter
	required *atomic.Bool
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
	return authenticationRuntime{options: options, limiter: newLoginLimiter(), required: required}
}

func (a authenticationRuntime) authenticationRequired() bool {
	return a.required.Load()
}

func (a authenticationRuntime) requireAuthentication() {
	a.required.Store(true)
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
	mux.HandleFunc("GET /api/auth/status", s.handleAuthenticationStatus)
	mux.HandleFunc("POST /api/auth/bootstrap", s.handleAuthenticationBootstrap)
	mux.HandleFunc("POST /api/auth/login", s.handleAuthenticationLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthenticationLogout)
	mux.HandleFunc("PUT /api/auth/password", s.handleAuthenticationPassword)
	mux.HandleFunc("GET /api/auth/control-identities", s.handleControlIdentities)
	mux.HandleFunc("PUT /api/auth/control-identity", s.handleControlIdentity)
	mux.HandleFunc("PUT /api/auth/profile-image", s.handleProfileImageUpload)
	mux.HandleFunc("DELETE /api/auth/profile-image", s.handleProfileImageDelete)
	mux.HandleFunc("GET /api/accounts", s.handleAccountsList)
	mux.HandleFunc("POST /api/accounts", s.handleAccountCreate)
	mux.HandleFunc("PUT /api/accounts/{id}/password", s.handleAccountPassword)
	mux.HandleFunc("PUT /api/accounts/{id}/disabled", s.handleAccountDisabled)
	mux.HandleFunc("GET /api/accounts/{id}/profile-image", s.handleProfileImage)
}

func (s *Server) authenticateRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if session, token, ok := s.sessionFromRequest(r); ok {
			ctx := context.WithValue(r.Context(), authenticatedAccountContextKey{}, session.Account)
			ctx = context.WithValue(ctx, authenticatedSessionContextKey{}, authenticatedSessionState{session: session, token: token})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		} else if token != "" {
			s.clearSessionCookie(w)
		}

		if s.auth.authenticationRequired() && !isPublicAuthenticationRequest(r) {
			s.writeAuthenticationRequired(w)
			return
		}
		next.ServeHTTP(w, r)
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
	default:
		return false
	}
}

func (s *Server) sessionFromRequest(r *http.Request) (accounts.Session, string, bool) {
	cookie, err := r.Cookie(s.sessionCookieName())
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return accounts.Session{}, "", false
	}
	token := strings.TrimSpace(cookie.Value)
	session, err := s.accounts.ResolveSession(r.Context(), token)
	if err != nil {
		return accounts.Session{}, token, false
	}
	return session, token, true
}

func (s *Server) authenticatePassword(r *http.Request, username, password string) (accounts.Account, bool, error) {
	address := remoteHost(r.RemoteAddr)
	usernameKey := strings.ToLower(strings.TrimSpace(username))
	if !s.auth.limiter.Allow(address, usernameKey) {
		return accounts.Account{}, false, errAuthenticationThrottled
	}
	account, err := s.accounts.Authenticate(r.Context(), username, password)
	if errors.Is(err, accounts.ErrInvalidCredentials) {
		return accounts.Account{}, false, nil
	}
	if err != nil {
		return accounts.Account{}, false, err
	}
	return account, true, nil
}

func remoteHost(remoteAddress string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddress))
	if err != nil {
		return strings.Trim(strings.TrimSpace(remoteAddress), "[]")
	}
	return host
}

func (s *Server) writeAuthenticationRequired(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
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
	initialized, err := s.accounts.Initialized(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("account status is unavailable"))
		return
	}
	account, authenticated := authenticatedAccount(r)
	settings, _ := s.store.PublicSnapshot()
	controlIdentities := []accounts.ControlIdentity(nil)
	if session, ok := authenticatedSession(r); ok {
		controlIdentities, err = s.accounts.ControlIdentities(r.Context(), account.ID, session.session.ControlAccountID)
		if err != nil {
			s.logger.Warn("control identities could not be listed", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("account status is unavailable"))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized":             initialized,
		"authentication_required": s.auth.authenticationRequired(),
		"authenticated":           authenticated,
		"account":                 optionalAccount(account, authenticated),
		"bootstrap_available":     isLoopbackRemote(r.RemoteAddr) && isLoopbackHost(r.Host),
		"ui_locale":               settings.UI.Locale,
		"control_identities":      controlIdentities,
	})
}

func optionalAccount(account accounts.Account, present bool) any {
	if !present {
		return nil
	}
	return account
}

func (s *Server) handleAuthenticationBootstrap(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRemote(r.RemoteAddr) || !isLoopbackHost(r.Host) || !isSameOriginBrowserRequest(r) {
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
	token, _, err := s.accounts.NewSession(r.Context(), account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("the initial account was created but a session could not be started"))
		return
	}
	s.setSessionCookie(w, token)
	s.logger.Info("initial administrator account created", "account_id", account.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"account": account})
}

func (s *Server) handleAuthenticationPassword(w http.ResponseWriter, r *http.Request) {
	account, authenticated := authenticatedAccount(r)
	if !authenticated {
		s.writeAuthenticationRequired(w)
		return
	}
	if !requireJSONRequest(w, r) {
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	verified, allowed, err := s.authenticatePassword(r, account.Username, body.CurrentPassword)
	if errors.Is(err, errAuthenticationThrottled) {
		writeError(w, http.StatusTooManyRequests, errAuthenticationThrottled)
		return
	}
	if err != nil {
		s.logger.Warn("account password confirmation failed internally", "error", err)
		writeError(w, http.StatusServiceUnavailable, errors.New("password change is temporarily unavailable"))
		return
	}
	if !allowed || verified.ID != account.ID {
		writeError(w, http.StatusUnauthorized, accounts.ErrInvalidCredentials)
		return
	}
	if err := s.accounts.SetPassword(r.Context(), account.ID, body.NewPassword); err != nil {
		if errors.Is(err, accounts.ErrInvalidPassword) {
			writeError(w, http.StatusBadRequest, err)
		} else {
			s.logger.Warn("account password could not be changed", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("account password could not be changed"))
		}
		return
	}
	s.clearSessionCookie(w)
	w.Header().Set("Clear-Site-Data", `"cookies"`)
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
	http.ServeContent(w, r, "profile.jpg", info.ModTime(), file)
}

func (s *Server) handleAuthenticationLogin(w http.ResponseWriter, r *http.Request) {
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
	account, allowed, err := s.authenticatePassword(r, body.Username, body.Password)
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
	token, session, err := s.accounts.NewSession(r.Context(), account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("login session could not be created"))
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleAuthenticationLogout(w http.ResponseWriter, r *http.Request) {
	_, token, authenticated := s.sessionFromRequest(r)
	if !authenticated {
		if _, ok := authenticatedAccount(r); !ok {
			s.writeAuthenticationRequired(w)
			return
		}
	}
	if token != "" {
		if err := s.accounts.RevokeSession(r.Context(), token); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("session could not be revoked"))
			return
		}
	}
	s.clearSessionCookie(w)
	w.Header().Set("Clear-Site-Data", `"cookies"`)
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
	if !requireJSONRequest(w, r) {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.accounts.SetPassword(r.Context(), r.PathValue("id"), body.Password); err != nil {
		switch {
		case errors.Is(err, accounts.ErrNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, accounts.ErrInvalidPassword):
			writeError(w, http.StatusBadRequest, err)
		default:
			s.logger.Warn("account password could not be changed", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("account password could not be changed"))
		}
		return
	}
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
