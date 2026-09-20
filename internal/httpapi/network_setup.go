package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func (s *Server) handleNetworkDiscover(w http.ResponseWriter, r *http.Request) {
	if !s.requireNetworkAdministrator(w, r) {
		return
	}
	result, err := s.networkAutomation.Discover(r.Context())
	if err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCertificatePrepare(w http.ResponseWriter, r *http.Request) {
	body, policy, ok := s.validateNetworkChange(w, r, false)
	if !ok {
		return
	}
	if policy.Config.CertificateMode == "" {
		writeError(w, http.StatusBadRequest, errors.New("choose an automatic certificate mode"))
		return
	}
	current, authenticated := authenticatedSession(r)
	if !authenticated {
		s.writeAuthenticationRequired(w)
		return
	}
	if !s.allowCredentialAttempt(r, current.session.Account.Username) {
		writeError(w, http.StatusTooManyRequests, errAuthenticationThrottled)
		return
	}
	// Confirm fresh account/session authority before dispatch. Certificate work
	// cannot change saved network access and never runs inside the DB writer.
	err := s.accounts.WithConfirmedAdministrator(r.Context(), current.session.Key, body.Password, func(*sql.Tx) error { return nil })
	if err != nil {
		if errors.Is(err, accounts.ErrInvalidSession) {
			s.writeAuthenticationRequired(w)
			return
		}
		if errors.Is(err, accounts.ErrInvalidCredentials) {
			s.recordRejectedLogin(r, nil)
		}
		writeError(w, http.StatusForbidden, errors.New("confirm your current administrator password to prepare HTTPS"))
		return
	}
	status, err := s.networkAutomation.Prepare(policy)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

func (s *Server) handleCertificatePreparation(w http.ResponseWriter, r *http.Request) {
	if !s.requireNetworkAdministrator(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.networkAutomation.Snapshot())
}

func (s *Server) handleLocalTrustCertificate(w http.ResponseWriter, r *http.Request) {
	if !s.requireNetworkAdministrator(w, r) {
		return
	}
	certificate, err := s.networkAutomation.LocalTrustCertificate()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="magichandy-local-trust.crt"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(certificate)
}
