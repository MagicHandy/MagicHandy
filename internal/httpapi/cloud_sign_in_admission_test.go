package httpapi

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

// Exercise the complete Host/TLS/browser/authentication/controller stack. A
// handler-only test misses the public-origin policy before OAuth admission.
func TestChatGPTSignInPublicOriginAdmission(t *testing.T) {
	var contacted atomic.Int32
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		contacted.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer issuer.Close()
	settings, err := openTestStore(t)
	if err != nil {
		t.Fatal(err)
	}
	store, err := accounts.New(settings.Datastore())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := netaccess.Validate(netaccess.Config{Mode: netaccess.DirectHTTPS,
		ListenAddress: "192.168.0.12:49717", PublicURL: "https://control.example.test:8443",
		TLSCertificate: "fixture-cert", TLSPrivateKey: "fixture-key"})
	if err != nil {
		t.Fatal(err)
	}
	s := newTestServerWithStore(t, settings, Runtime{Accounts: store, AuthenticationRequired: true,
		SecureCookies: true, NetworkPolicy: policy, AllowedBrowserHosts: []string{"control.example.test"}, OpenAIIssuer: issuer.URL})
	admin, err := store.BootstrapAdmin(t.Context(), "owner", "a long review passphrase")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := store.Create(t.Context(), "operator", "another long passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	cookieFor := func(account accounts.Account) *http.Cookie {
		token, _, err := store.NewSession(t.Context(), account.ID)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: secureSessionCookieName, Value: token, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
	}
	adminCookie, operatorCookie := cookieFor(admin), cookieFor(operator)
	prepare := func(r *http.Request) {
		r.URL.Scheme, r.URL.Host, r.Host = "https", "control.example.test:8443", "control.example.test:8443"
		r.RemoteAddr = "192.168.0.1:51000" // Public-address hairpin does not prove browser locality.
		r.TLS = &tls.ConnectionState{}
		r.Header.Set("Origin", "https://control.example.test:8443")
	}
	heartbeat := authenticatedControlRequest(s, adminCookie, http.MethodPost, "/api/controller/heartbeat", "test-controller", `{}`, prepare)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", heartbeat.Code, heartbeat.Body.String())
	}
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		client string
		edit   func(*http.Request)
	}{
		{"signed out", &http.Cookie{Name: secureSessionCookieName, Value: "invalid", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}, "test-controller", nil},
		{"operator", operatorCookie, "test-controller", nil},
		{"read-only tab", adminCookie, "another-tab", nil},
		{"cross origin", adminCookie, "test-controller", func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") }},
		{"unconfigured host", adminCookie, "test-controller", func(r *http.Request) { r.Host = "192.168.0.12:49717" }},
		{"plaintext", adminCookie, "test-controller", func(r *http.Request) { r.TLS = nil }},
		{"forwarded direct traffic", adminCookie, "test-controller", func(r *http.Request) { r.Header.Set("X-Forwarded-For", "127.0.0.1") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := contacted.Load()
			w := authenticatedControlRequest(s, tc.cookie, http.MethodPost, "/api/llm/cloud/sign-in", tc.client, `{}`, prepare, func(r *http.Request) {
				if tc.edit != nil {
					tc.edit(r)
				}
			})
			if w.Code < 400 || contacted.Load() != before {
				t.Fatalf("admission=%d contacted=%d: %s", w.Code, contacted.Load()-before, w.Body.String())
			}
		})
	}
	w := authenticatedControlRequest(s, adminCookie, http.MethodPost, "/api/llm/cloud/sign-in", "test-controller", `{}`, prepare)
	if w.Code != http.StatusBadGateway || contacted.Load() == 0 {
		t.Fatalf("controlling administrator did not reach the local issuer fixture: %d %s", w.Code, w.Body.String())
	}
}
