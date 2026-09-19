package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func (s *Server) accountRecoveryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/recovery-codes", s.sessionManagementHandler(s.handleRecoveryCodeStatus))
	mux.HandleFunc("POST /api/auth/recovery-codes", s.sessionManagementHandler(s.handleRecoveryCodeChange))
	mux.HandleFunc("DELETE /api/auth/recovery-codes", s.sessionManagementHandler(s.handleRecoveryCodeChange))
	mux.HandleFunc("POST /api/auth/recover", credentialHandler(s.handleRecoverPassword))
}

func credentialHandler(action http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		action(w, r.WithContext(ctx))
	}
}

// Credentials have a small body budget and a separate upload deadline. The
// deadline is cleared only after a complete decode; malformed/unused uploads
// close safely instead of holding a login slot while net/http drains the body.
func decodeCredentialRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])) != "application/json" {
		rejectRequest(w, r, http.StatusUnsupportedMediaType, errors.New("application/json is required"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := decodeJSON(r, target); err != nil {
		rejectRequest(w, r, http.StatusBadRequest, err)
		return false
	}
	_ = controller.SetReadDeadline(time.Time{})
	return true
}

func (s *Server) handleRecoveryCodeStatus(w http.ResponseWriter, r *http.Request, actor accounts.Session) {
	status, err := s.accounts.RecoveryStatus(r.Context(), actor.Key)
	if err != nil {
		s.writeRecoveryError(w, err)
		return
	}
	s.writeSessionManagementJSON(w, r, status)
}

func (s *Server) handleRecoveryCodeChange(w http.ResponseWriter, r *http.Request, actor accounts.Session) {
	var body struct {
		Password string `json:"password"`
	}
	if !decodeCredentialRequest(w, r, &body) {
		return
	}
	if !s.allowCredentialAttempt(r, actor.Account.Username) {
		s.writeRecoveryError(w, errAuthenticationThrottled)
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.accounts.RemoveRecoveryCodes(r.Context(), actor.Key, body.Password); err != nil {
			if errors.Is(err, accounts.ErrInvalidCredentials) {
				s.recordRejectedLogin(r, nil)
			}
			s.writeRecoveryError(w, err)
			return
		}
		s.writeSessionManagementJSON(w, r, accounts.RecoveryStatus{Limit: accounts.MaxRecoveryCodes})
		return
	}
	codes, err := s.accounts.ReplaceRecoveryCodes(r.Context(), actor.Key, body.Password)
	if err != nil {
		if errors.Is(err, accounts.ErrInvalidCredentials) {
			s.recordRejectedLogin(r, nil)
		}
		s.writeRecoveryError(w, err)
		return
	}
	s.writeSessionManagementJSON(w, r, codes)
}

func (s *Server) handleRecoverPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if !decodeCredentialRequest(w, r, &body) {
		return
	}
	if !s.allowCredentialAttempt(r, body.Username) {
		s.writeRecoveryError(w, errAuthenticationThrottled)
		return
	}
	result, err := s.accounts.RecoverPassword(r.Context(), body.Username, body.Code, body.Password)
	if err != nil {
		if errors.Is(err, accounts.ErrInvalidRecoveryCode) {
			s.recordRejectedLogin(r, nil)
		}
		s.writeRecoveryError(w, err)
		return
	}
	actorKey := ""
	if current, ok := authenticatedSession(r); ok && current.session.Account.ID == result.AccountID {
		actorKey = current.session.Key
		s.clearSessionCookie(w)
	}
	s.endRevokedSessions(r.Context(), actorKey, result.SessionKeys)
	s.writeSessionManagementJSON(w, r, map[string]bool{"recovered": true})
}

func (s *Server) writeRecoveryError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	message := "account recovery is temporarily unavailable"
	switch {
	case errors.Is(err, accounts.ErrInvalidSession):
		s.writeAuthenticationRequired(w)
		return
	case errors.Is(err, accounts.ErrInvalidRecoveryCode):
		status, message = http.StatusUnauthorized, accounts.ErrInvalidRecoveryCode.Error()
	case errors.Is(err, accounts.ErrInvalidCredentials):
		status, message = http.StatusForbidden, "the current password is incorrect"
	case errors.Is(err, accounts.ErrInvalidPassword):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, errAuthenticationThrottled):
		status, message = http.StatusTooManyRequests, errAuthenticationThrottled.Error()
	}
	writeBoundedJSON(w, status, map[string]string{"error": message})
}
