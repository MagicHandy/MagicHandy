package httpapi

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"sort"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

func (s *Server) networkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/network", s.handleNetworkStatus)
	mux.HandleFunc("POST /api/network/validate", s.handleNetworkValidate)
	mux.HandleFunc("PUT /api/network", s.handleNetworkSave)
	mux.HandleFunc("GET /api/network/report", s.handleNetworkReport)
}

func (s *Server) protectNetworkRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.networkPolicy != nil {
			accepted, err := s.networkPolicy.Accept(r)
			if err != nil {
				rejectRequest(w, r, http.StatusForbidden, err)
				return
			}
			r = accepted
		} else if netaccess.HasForwardingHeaders(r) {
			rejectRequest(w, r, http.StatusForbidden, errors.New("forwarded traffic requires an explicit trusted proxy configuration"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireNetworkAdministrator(w http.ResponseWriter, r *http.Request) bool {
	if !s.auth.authenticationRequired() && isLocalHostRequest(r) {
		return true
	}
	_, ok := s.requireAdministrator(w, r)
	return ok
}

func (s *Server) activeNetworkConfig(r *http.Request) netaccess.Config {
	if s.networkPolicy != nil {
		return s.networkPolicy.Config
	}
	// Legacy CLI launches are displayed honestly. Saving an explicit policy
	// migrates the next startup, not the already-running socket.
	return netaccess.Config{Mode: "legacy", ListenAddress: r.Host}
}

func (s *Server) handleNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireNetworkAdministrator(w, r) {
		return
	}
	saved, err := netaccess.Load(r.Context(), s.store.Datastore())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("saved network settings are unavailable; use a local recovery launch"))
		return
	}
	active := s.activeNetworkConfig(r)
	payload := map[string]any{"active": active, "saved": saved,
		"restart_required": saved != nil && !reflect.DeepEqual(*saved, active),
		"interfaces":       networkInterfaces(), "forwarded": netaccess.IsForwarded(r),
		"authentication_required": s.auth.authenticationRequired(), "secure_cookie": s.auth.options.SecureCookies}
	payload["request_admission"] = s.requestAdmission.snapshot()
	payload["stop_admission"] = s.stopAdmission.snapshot()
	if s.networkCertificates != nil {
		payload["certificate"] = s.networkCertificates.Status()
	}
	writeJSON(w, http.StatusOK, payload)
}

type networkChange struct {
	Config   netaccess.Config `json:"config"`
	Password string           `json:"password,omitempty"`
}

func (s *Server) validateNetworkChange(w http.ResponseWriter, r *http.Request) (networkChange, *netaccess.Policy, bool) {
	var body networkChange
	if !s.requireNetworkAdministrator(w, r) {
		return body, nil, false
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return body, nil, false
	}
	policy, err := netaccess.Validate(body.Config)
	if err == nil && policy.Config.Mode != netaccess.Local {
		count, countErr := s.accounts.EnabledCount(r.Context())
		if countErr != nil || count == 0 {
			err = errors.New("create the administrator account before enabling remote access")
		}
	}
	if err == nil && policy.Config.Mode == netaccess.DirectHTTPS {
		_, err = netaccess.LoadCertificates(policy)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return body, nil, false
	}
	return body, policy, true
}

func (s *Server) handleNetworkValidate(w http.ResponseWriter, r *http.Request) {
	_, policy, ok := s.validateNetworkChange(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "config": policy.Config,
		"message": "Configuration is valid on this host. Client trust, DNS, firewall and external reachability still need a connection test."})
}

func (s *Server) handleNetworkSave(w http.ResponseWriter, r *http.Request) {
	body, policy, ok := s.validateNetworkChange(w, r)
	if !ok {
		return
	}
	var err error
	if current, authenticated := authenticatedSession(r); authenticated {
		if !s.allowCredentialAttempt(r, current.session.Account.Username) {
			writeError(w, http.StatusTooManyRequests, errAuthenticationThrottled)
			return
		}
		err = s.accounts.WithConfirmedAdministrator(r.Context(), current.session.Key, body.Password, func(tx *sql.Tx) error {
			return netaccess.SaveTx(r.Context(), tx, policy.Config)
		})
	} else {
		// Bootstrap-local configuration is allowed only while accounts are
		// still absent. Serialize that condition with first-account creation.
		err = s.store.Datastore().WithTx(r.Context(), func(tx *sql.Tx) error {
			var count int
			if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM user_accounts`).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return accounts.ErrInvalidSession
			}
			return netaccess.SaveTx(r.Context(), tx, policy.Config)
		})
	}
	if err != nil {
		switch {
		case errors.Is(err, accounts.ErrInvalidSession):
			s.writeAuthenticationRequired(w)
		case errors.Is(err, accounts.ErrInvalidCredentials), errors.Is(err, accounts.ErrAdministratorRequired):
			if errors.Is(err, accounts.ErrInvalidCredentials) {
				s.recordRejectedLogin(r, nil)
			}
			writeError(w, http.StatusForbidden, errors.New("confirm your current administrator password to change remote access"))
		default:
			writeError(w, http.StatusInternalServerError, errors.New("network configuration could not be saved"))
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": policy.Config, "restart_required": true})
}

type networkInterface struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Loopback bool   `json:"loopback"`
}

func networkInterfaces() []networkInterface {
	result := []networkInterface{}
	interfaces, err := net.Interfaces()
	if err != nil {
		return result
	}
	for _, adapter := range interfaces {
		if adapter.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := adapter.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil || prefix.Addr().IsLinkLocalUnicast() || prefix.Addr().IsMulticast() {
				continue
			}
			result = append(result, networkInterface{Name: adapter.Name, Address: prefix.Addr().String(), Loopback: prefix.Addr().IsLoopback()})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Address < result[j].Address })
	return result
}

func (s *Server) handleNetworkReport(w http.ResponseWriter, r *http.Request) {
	if !s.requireNetworkAdministrator(w, r) {
		return
	}
	active := s.activeNetworkConfig(r)
	controller := s.controllerState(r)
	settings, _ := s.store.PublicSnapshot()
	report := map[string]any{"report_version": 1, "generated_at": time.Now().UTC(), "app": s.version,
		"network_mode": active.Mode, "forwarded": netaccess.IsForwarded(r),
		"authentication_required": s.auth.authenticationRequired(), "secure_cookie": s.auth.options.SecureCookies,
		"controller_active": controller.Active, "lease_remaining_ms": controller.LeaseExpiresInMillis,
		"ownership_generation": controller.Generation, "stop_sequence": s.stopSequence.Load(),
		"dispatch_owner":    settings.Device.HSPDispatchOwner,
		"request_admission": s.requestAdmission.snapshot(),
		"stop_admission":    s.stopAdmission.snapshot(),
		"privacy":           "This report excludes credentials, host paths, addresses, chat, media and audio. Share it with the developer when reporting a connection problem."}
	if s.networkCertificates != nil {
		status := s.networkCertificates.Status()
		report["certificate_expires_at"], report["certificate_reload_failed"] = status.NotAfter, status.ReloadError
	}
	w.Header().Set("Content-Disposition", `attachment; filename="magichandy-connection-report.json"`)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, report)
}
