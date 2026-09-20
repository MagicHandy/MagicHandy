package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

func TestForwardedLocalAddressNeverGrantsLocalPrivileges(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	policy, err := netaccess.Validate(netaccess.Config{Mode: netaccess.TrustedProxy, ListenAddress: "127.0.0.1:49717",
		PublicURL: "https://localhost", TrustedProxies: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	s.networkPolicy = policy
	r := httptest.NewRequest(http.MethodGet, "http://localhost/api/auth/status", nil)
	r.RemoteAddr = "127.0.0.1:30000"
	r.Header.Set("Origin", "https://localhost")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "localhost")
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"bootstrap_available":false`) {
		t.Fatalf("proxy bootstrap privilege: %d %s", w.Code, w.Body.String())
	}
	accepted, err := policy.Accept(r)
	if err != nil || isLocalHostRequest(accepted) {
		t.Fatal("forwarded loopback identity became local")
	}
	for _, route := range []string{"/api/auth/bootstrap", "/api/host/path-picker"} {
		r.Method, r.URL.Path = http.MethodPost, route
		w = httptest.NewRecorder()
		if route == "/api/auth/bootstrap" {
			s.handleAuthenticationBootstrap(w, accepted)
		} else {
			s.handleHostPathPicker(w, accepted)
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s did not deny proxy: %d", route, w.Code)
		}
	}
}

func TestOperatorPermissionMatrixAndRevocationFence(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		name := "timed"
		if permanent {
			name = "permanent"
		}
		t.Run(name, func(t *testing.T) { assertOperatorPermissionMatrix(t, permanent) })
	}
}

func assertOperatorPermissionMatrix(t *testing.T, permanent bool) {
	t.Helper()
	s, store, admin, _ := newControllerSessionFixture(t)
	operator, err := store.Create(t.Context(), "observer", "a strong observer passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := testSessionCookie(token)
	for _, route := range []string{"/api/controller/takeover", "/api/motion/start", "/api/setup/jobs", "/api/llm/load", "/api/host/path-picker", "/api/settings/reset"} {
		w := authenticatedControlRequest(s, cookie, http.MethodPost, route, "test-controller", `{}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("observer allowed %s: %d %s", route, w.Code, w.Body.String())
		}
	}
	if permanent {
		_, err = store.GrantPermanentControl(t.Context(), admin.ID, operator.ID)
	} else {
		_, err = store.GrantControl(t.Context(), admin.ID, operator.ID, time.Hour)
	}
	if err != nil {
		t.Fatal(err)
	}
	claimAuthenticatedController(t, s, cookie)
	for _, route := range []string{"/api/llm/load", "/api/settings/reset", "/api/host/path-picker", "/api/setup/jobs"} {
		w := authenticatedControlRequest(s, cookie, http.MethodPost, route, "test-controller", `{}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("controller gained host privileges %s: %d", route, w.Code)
		}
	}
	if err := store.RevokeControl(t.Context(), admin.ID, operator.ID); err != nil {
		t.Fatal(err)
	}
	s.checkAccessLifetimes()
	w := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/motion/start", "test-controller", `{"speed_percent":20}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("revoked control accepted: %d", w.Code)
	}
	w = authenticatedControlRequest(s, cookie, http.MethodGet, "/api/controller", "test-controller", ``)
	var snapshot controllerSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Active || w.Code != http.StatusOK {
		t.Fatal("revocation did not retain read-only access")
	}
}
