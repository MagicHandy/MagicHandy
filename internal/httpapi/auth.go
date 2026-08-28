package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

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
	options authenticationOptions
	limiter *loginLimiter
}

type authenticatedAccountContextKey struct{}

func newAuthenticationRuntime(options authenticationOptions) authenticationRuntime {
	return authenticationRuntime{options: options, limiter: newLoginLimiter()}
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
	authRuntime := newAuthenticationRuntime(authenticationOptions{
		Required:      runtime.AuthenticationRequired,
		SecureCookies: runtime.SecureCookies,
	})
	return accountStore, authRuntime, nil
}

func (s *Server) authenticationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/status", s.handleAuthenticationStatus)
	mux.HandleFunc("POST /api/auth/bootstrap", s.handleAuthenticationBootstrap)
	mux.HandleFunc("POST /api/auth/login", s.handleAuthenticationLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthenticationLogout)
	mux.HandleFunc("GET /api/accounts", s.handleAccountsList)
	mux.HandleFunc("POST /api/accounts", s.handleAccountCreate)
	mux.HandleFunc("PUT /api/accounts/{id}/password", s.handleAccountPassword)
	mux.HandleFunc("PUT /api/accounts/{id}/disabled", s.handleAccountDisabled)
}

func (s *Server) authenticateRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if session, token, ok := s.sessionFromRequest(r); ok {
			ctx := context.WithValue(r.Context(), authenticatedAccountContextKey{}, session.Account)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		} else if token != "" {
			s.clearSessionCookie(w)
		}

		if username, password, ok := r.BasicAuth(); ok && !isPublicAuthenticationRequest(r) {
			account, allowed, err := s.authenticatePassword(r, username, password)
			if errors.Is(err, errAuthenticationThrottled) {
				writeError(w, http.StatusTooManyRequests, errAuthenticationThrottled)
				return
			}
			if err != nil {
				s.logger.Warn("account authentication failed internally", "error", err)
			}
			if allowed {
				token, _, sessionErr := s.accounts.NewSession(r.Context(), account.ID)
				if sessionErr != nil {
					s.logger.Warn("authenticated account session could not be created", "error", sessionErr)
					s.writeAuthenticationRequired(w)
					return
				}
				s.setSessionCookie(w, token)
				ctx := context.WithValue(r.Context(), authenticatedAccountContextKey{}, account)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if !isPublicAuthenticationRequest(r) {
				s.writeAuthenticationRequired(w)
				return
			}
		}

		if s.auth.options.Required && !isPublicAuthenticationRequest(r) {
			s.writeAuthenticationRequired(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPublicAuthenticationRequest(r *http.Request) bool {
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
	w.Header().Set("WWW-Authenticate", `Basic realm="MagicHandy", charset="UTF-8"`)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized":             initialized,
		"authentication_required": s.auth.options.Required,
		"authenticated":           authenticated,
		"account":                 optionalAccount(account, authenticated),
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
	token, _, err := s.accounts.NewSession(r.Context(), account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("the initial account was created but a session could not be started"))
		return
	}
	s.setSessionCookie(w, token)
	s.logger.Info("initial administrator account created", "account_id", account.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"account": account})
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
