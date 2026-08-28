package httpapi

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestAuthenticationBootstrapIsLoopbackOnlyAndSetsStrictSession(t *testing.T) {
	server, _ := newAuthenticationTestServer(t, true, true, nil)

	remote := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", strings.NewReader(`{
		"username":"owner","password":"correct horse battery staple"
	}`))
	remote.Header.Set("Content-Type", "application/json")
	remote.Host = "192.168.1.20:49717"
	remote.RemoteAddr = "192.168.1.10:50000"
	remoteRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(remoteRecorder, remote)
	if remoteRecorder.Code != http.StatusForbidden {
		t.Fatalf("remote bootstrap status = %d, want %d: %s", remoteRecorder.Code, http.StatusForbidden, remoteRecorder.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", strings.NewReader(`{
		"username":"owner","password":"correct horse battery staple"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Host = "127.0.0.1:49717"
	request.RemoteAddr = "127.0.0.1:50000"
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("bootstrap cookies = %+v, want one", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != secureSessionCookieName || !cookie.Secure || !cookie.HttpOnly ||
		cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.MaxAge != 0 {
		t.Fatalf("session cookie = %+v", cookie)
	}
	if strings.Contains(recorder.Body.String(), "correct horse") || strings.Contains(recorder.Body.String(), "password_hash") {
		t.Fatalf("bootstrap leaked credentials: %s", recorder.Body.String())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusRequest.AddCookie(cookie)
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, statusRequest)
	if statusRecorder.Code != http.StatusOK || !strings.Contains(statusRecorder.Body.String(), `"authenticated":true`) {
		t.Fatalf("authenticated status = %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}
}

func TestRequiredAuthenticationSupportsBasicBridgeThenSession(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, true, []string{"192.168.1.20"})
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	unauthenticated.Host = "192.168.1.20:49717"
	unauthenticated.RemoteAddr = "192.168.1.10:50000"
	unauthenticated.TLS = &tls.ConnectionState{}
	unauthenticatedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthenticatedRecorder, unauthenticated)
	if unauthenticatedRecorder.Code != http.StatusUnauthorized || unauthenticatedRecorder.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthenticated state = %d headers=%v body=%s", unauthenticatedRecorder.Code, unauthenticatedRecorder.Header(), unauthenticatedRecorder.Body.String())
	}

	basic := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	basic.Host = "192.168.1.20:49717"
	basic.RemoteAddr = "192.168.1.10:50000"
	basic.TLS = &tls.ConnectionState{}
	basic.SetBasicAuth("OWNER", "correct horse battery staple")
	basicRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(basicRecorder, basic)
	if basicRecorder.Code != http.StatusOK {
		t.Fatalf("Basic state status = %d: %s", basicRecorder.Code, basicRecorder.Body.String())
	}
	cookies := basicRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != secureSessionCookieName {
		t.Fatalf("Basic bridge cookies = %+v", cookies)
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	sessionRequest.Host = "192.168.1.20:49717"
	sessionRequest.RemoteAddr = "192.168.1.10:50001"
	sessionRequest.TLS = &tls.ConnectionState{}
	sessionRequest.AddCookie(cookies[0])
	sessionRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(sessionRecorder, sessionRequest)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("session state status = %d: %s", sessionRecorder.Code, sessionRecorder.Body.String())
	}
	if sessionRecorder.Header().Get("Strict-Transport-Security") == "" || sessionRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("security headers = %v", sessionRecorder.Header())
	}

	accountsRequest := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	accountsRequest.AddCookie(cookies[0])
	accountsRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(accountsRecorder, accountsRequest)
	if accountsRecorder.Code != http.StatusOK || !strings.Contains(accountsRecorder.Body.String(), admin.ID) {
		t.Fatalf("admin account list = %d: %s", accountsRecorder.Code, accountsRecorder.Body.String())
	}
}

func TestAccountManagementIsAdminOnlyAndNeverRequiresGUI(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, false, nil)
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	adminToken, _, err := accountStore.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("admin session: %v", err)
	}

	create := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader(`{
		"username":"operator","password":"another excellent password","role":"operator"
	}`))
	create.Header.Set("Content-Type", "application/json")
	// #nosec G124 -- request fixture; response-cookie security flags are asserted
	// separately and AddCookie serializes only name/value.
	create.AddCookie(&http.Cookie{Name: loopbackSessionCookieName, Value: adminToken})
	createRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create operator status = %d: %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Account accounts.Account `json:"account"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil || created.Account.Role != accounts.RoleOperator {
		t.Fatalf("created operator = (%+v, %v)", created, err)
	}

	operatorList := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	operatorList.SetBasicAuth("operator", "another excellent password")
	operatorListRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(operatorListRecorder, operatorList)
	if operatorListRecorder.Code != http.StatusForbidden {
		t.Fatalf("operator list status = %d, want %d: %s", operatorListRecorder.Code, http.StatusForbidden, operatorListRecorder.Body.String())
	}
}

func TestEmergencyStopRemainsAvailableWhenAuthenticationExpires(t *testing.T) {
	fake := transport.NewFake()
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	server := newTestServerWithStore(t, store, Runtime{
		Transport:              fake,
		MotionTransport:        fake,
		AuthenticationRequired: true,
		SecureCookies:          true,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unauthenticated Stop status = %d: %s", recorder.Code, recorder.Body.String())
	}
	commands := fake.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("Stop commands = %+v", commands)
	}
}

func TestRemoteBrowserOriginMustMatchConfiguredHTTPSHost(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, true, []string{"192.168.1.20"})
	if _, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	for name, test := range map[string]struct {
		host   string
		origin string
		want   int
	}{
		"allowed":        {host: "192.168.1.20:49717", origin: "https://192.168.1.20:49717", want: http.StatusOK},
		"wrong host":     {host: "attacker.example", origin: "https://attacker.example", want: http.StatusForbidden},
		"wrong scheme":   {host: "192.168.1.20:49717", origin: "http://192.168.1.20:49717", want: http.StatusForbidden},
		"foreign origin": {host: "192.168.1.20:49717", origin: "https://attacker.example", want: http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
			request.Host = test.host
			request.RemoteAddr = "192.168.1.10:50000"
			request.TLS = &tls.ConnectionState{}
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			request.SetBasicAuth("owner", "correct horse battery staple")
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestLoginUsesGenericFailuresAndThrottlesPerUsername(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, false, false, nil)
	if _, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	for attempt := 1; attempt <= 9; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{
			"username":"owner","password":"wrong password value"
		}`))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "192.168.1.10:50000"
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if attempt <= 8 {
			if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != "{\"error\":\"invalid username or password\"}\n" {
				t.Fatalf("attempt %d = %d: %s", attempt, recorder.Code, recorder.Body.String())
			}
		} else if recorder.Code != http.StatusTooManyRequests || !strings.Contains(recorder.Body.String(), "too many") {
			t.Fatalf("throttled attempt = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
}

func newAuthenticationTestServer(t *testing.T, required, secure bool, allowedHosts []string) (*Server, *accounts.Store) {
	t.Helper()
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	accountStore, err := accounts.New(store.Datastore())
	if err != nil {
		t.Fatalf("accounts.New: %v", err)
	}
	server := newTestServerWithStore(t, store, Runtime{
		Accounts:               accountStore,
		AuthenticationRequired: required,
		SecureCookies:          secure,
		AllowedBrowserHosts:    allowedHosts,
	})
	return server, accountStore
}
