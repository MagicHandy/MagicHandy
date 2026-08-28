package httpapi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
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
	if statusRecorder.Code != http.StatusOK ||
		!strings.Contains(statusRecorder.Body.String(), `"authenticated":true`) ||
		!strings.Contains(statusRecorder.Body.String(), `"authentication_required":true`) {
		t.Fatalf("authenticated status = %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}
}

func TestSetupCompletionSignsOutBootstrapSessionOnLoopbackHTTP(t *testing.T) {
	server, _ := newAuthenticationTestServer(t, false, false, nil)

	bootstrap := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", strings.NewReader(`{
		"username":"owner","password":"eight888"
	}`))
	bootstrap.Header.Set("Content-Type", "application/json")
	bootstrap.Host = "127.0.0.1:49717"
	bootstrap.RemoteAddr = "127.0.0.1:50000"
	bootstrapRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(bootstrapRecorder, bootstrap)
	if bootstrapRecorder.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, want %d: %s", bootstrapRecorder.Code, http.StatusCreated, bootstrapRecorder.Body.String())
	}
	cookies := bootstrapRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != loopbackSessionCookieName {
		t.Fatalf("bootstrap cookies = %+v, want loopback session", cookies)
	}
	bootstrapCookie := cookies[0]

	complete := withController(httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(`{
		"allow_unready_llm":true
	}`)))
	complete.Header.Set("Content-Type", "application/json")
	complete.Host = "127.0.0.1:49717"
	complete.RemoteAddr = "127.0.0.1:50000"
	complete.AddCookie(bootstrapCookie)
	completeRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(completeRecorder, complete)
	if completeRecorder.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want %d: %s", completeRecorder.Code, http.StatusOK, completeRecorder.Body.String())
	}
	if !strings.Contains(completeRecorder.Body.String(), `"signed_out":true`) {
		t.Fatalf("complete body = %s, want signed_out", completeRecorder.Body.String())
	}
	cleared := completeRecorder.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != loopbackSessionCookieName || cleared[0].MaxAge >= 0 {
		t.Fatalf("completion cookies = %+v, want expired loopback session", cleared)
	}

	status := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	status.Host = "127.0.0.1:49717"
	status.RemoteAddr = "127.0.0.1:50001"
	status.AddCookie(bootstrapCookie)
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, status)
	if statusRecorder.Code != http.StatusOK ||
		!strings.Contains(statusRecorder.Body.String(), `"authentication_required":true`) ||
		!strings.Contains(statusRecorder.Body.String(), `"authenticated":false`) {
		t.Fatalf("post-setup status = %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}

	private := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	private.Host = "127.0.0.1:49717"
	private.RemoteAddr = "127.0.0.1:50002"
	privateRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(privateRecorder, private)
	if privateRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("post-setup state status = %d, want %d: %s", privateRecorder.Code, http.StatusUnauthorized, privateRecorder.Body.String())
	}
}

func TestCompletedSetupReconfigurationPreservesOrdinarySession(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, false, nil)
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "eight888")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	saveSettings(t, server.store, func(current config.Settings) config.Settings {
		current.UI.SetupCompleted = true
		return current
	})
	token, _, err := accountStore.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	// #nosec G124 -- request fixture; AddCookie serializes name/value only.
	cookie := &http.Cookie{Name: loopbackSessionCookieName, Value: token}

	reconfigure := withController(httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(`{
		"allow_unready_llm":true
	}`)))
	reconfigure.Header.Set("Content-Type", "application/json")
	reconfigure.Host = "127.0.0.1:49717"
	reconfigure.RemoteAddr = "127.0.0.1:50003"
	reconfigure.AddCookie(cookie)
	reconfigureRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(reconfigureRecorder, reconfigure)
	if reconfigureRecorder.Code != http.StatusOK || strings.Contains(reconfigureRecorder.Body.String(), `"signed_out":true`) {
		t.Fatalf("reconfigure status = %d: %s", reconfigureRecorder.Code, reconfigureRecorder.Body.String())
	}

	status := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	status.Host = "127.0.0.1:49717"
	status.RemoteAddr = "127.0.0.1:50003"
	status.AddCookie(cookie)
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, status)
	if statusRecorder.Code != http.StatusOK || !strings.Contains(statusRecorder.Body.String(), `"authenticated":true`) {
		t.Fatalf("reconfigure auth status = %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}
}

func TestRequiredAuthenticationLoadsLoginShellThenUsesJSONSession(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, true, []string{"192.168.1.20"})
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	staticRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	staticRequest.Host = "192.168.1.20:49717"
	staticRequest.RemoteAddr = "192.168.1.10:50000"
	staticRequest.TLS = &tls.ConnectionState{}
	staticRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(staticRecorder, staticRequest)
	if staticRecorder.Code != http.StatusOK {
		t.Fatalf("login shell status = %d: %s", staticRecorder.Code, staticRecorder.Body.String())
	}

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	unauthenticated.Host = "192.168.1.20:49717"
	unauthenticated.RemoteAddr = "192.168.1.10:50000"
	unauthenticated.TLS = &tls.ConnectionState{}
	unauthenticatedRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthenticatedRecorder, unauthenticated)
	if unauthenticatedRecorder.Code != http.StatusUnauthorized || unauthenticatedRecorder.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("unauthenticated state = %d headers=%v body=%s", unauthenticatedRecorder.Code, unauthenticatedRecorder.Header(), unauthenticatedRecorder.Body.String())
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{
		"username":"OWNER","password":"correct horse battery staple"
	}`))
	login.Header.Set("Content-Type", "application/json")
	login.Host = "192.168.1.20:49717"
	login.RemoteAddr = "192.168.1.10:50000"
	login.TLS = &tls.ConnectionState{}
	loginRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("JSON login status = %d: %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	cookies := loginRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != secureSessionCookieName {
		t.Fatalf("login cookies = %+v", cookies)
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
	operator, err := accountStore.Authenticate(t.Context(), "operator", "another excellent password")
	if err != nil {
		t.Fatalf("authenticate operator: %v", err)
	}
	operatorToken, _, err := accountStore.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatalf("operator session: %v", err)
	}
	// #nosec G124 -- request fixture; AddCookie serializes name/value only.
	operatorList.AddCookie(&http.Cookie{Name: loopbackSessionCookieName, Value: operatorToken})
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

func TestExistingAccountRequiresAuthenticationAfterRestart(t *testing.T) {
	store, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	accountStore, err := accounts.New(store.Datastore())
	if err != nil {
		t.Fatalf("accounts.New: %v", err)
	}
	if _, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	server := newTestServerWithStore(t, store, Runtime{Accounts: accountStore})

	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("state status = %d, want %d: %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusRequest.Host = "127.0.0.1:49717"
	statusRequest.RemoteAddr = "127.0.0.1:50000"
	statusRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusRecorder, statusRequest)
	if statusRecorder.Code != http.StatusOK || !strings.Contains(statusRecorder.Body.String(), `"authentication_required":true`) {
		t.Fatalf("auth status = %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}
}

func TestRemoteBrowserOriginMustMatchConfiguredHTTPSHost(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, true, []string{"192.168.1.20"})
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	token, _, err := accountStore.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("admin session: %v", err)
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
			// #nosec G124 -- request fixture; AddCookie serializes name/value only.
			request.AddCookie(&http.Cookie{Name: secureSessionCookieName, Value: token})
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestAuthenticatedAccountCanChangeOwnPasswordAndRevokesSession(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, false, nil)
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	token, _, err := accountStore.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("admin session: %v", err)
	}

	change := httptest.NewRequest(http.MethodPut, "/api/auth/password", strings.NewReader(`{
		"current_password":"correct horse battery staple",
		"new_password":"a different excellent passphrase"
	}`))
	change.Header.Set("Content-Type", "application/json")
	// #nosec G124 -- request fixture; AddCookie serializes name/value only.
	change.AddCookie(&http.Cookie{Name: loopbackSessionCookieName, Value: token})
	changeRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(changeRecorder, change)
	if changeRecorder.Code != http.StatusNoContent {
		t.Fatalf("password change status = %d: %s", changeRecorder.Code, changeRecorder.Body.String())
	}
	if _, err := accountStore.ResolveSession(t.Context(), token); !errors.Is(err, accounts.ErrInvalidSession) {
		t.Fatalf("old session error = %v, want ErrInvalidSession", err)
	}
	if _, err := accountStore.Authenticate(t.Context(), "owner", "a different excellent passphrase"); err != nil {
		t.Fatalf("new password login: %v", err)
	}
}

func TestProfileImageAPIIsPrivateAndSessionScoped(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, false, nil)
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	operator, err := accountStore.Create(t.Context(), "operator", "another excellent password", accounts.RoleOperator)
	if err != nil {
		t.Fatalf("Create operator: %v", err)
	}
	adminToken, _, err := accountStore.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("admin session: %v", err)
	}
	operatorToken, _, err := accountStore.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatalf("operator session: %v", err)
	}
	adminCookie := &http.Cookie{Name: loopbackSessionCookieName, Value: adminToken}       // #nosec G124 -- test fixture.
	operatorCookie := &http.Cookie{Name: loopbackSessionCookieName, Value: operatorToken} // #nosec G124 -- test fixture.

	invalid := httptest.NewRequest(http.MethodPut, "/api/auth/profile-image", strings.NewReader("not an image"))
	invalid.AddCookie(adminCookie)
	invalidRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRecorder, invalid)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid profile status = %d: %s", invalidRecorder.Code, invalidRecorder.Body.String())
	}

	imageBytes := profileImageFixture(t)
	upload := httptest.NewRequest(http.MethodPut, "/api/auth/profile-image", bytes.NewReader(imageBytes))
	upload.Header.Set("Content-Type", "image/jpeg")
	upload.AddCookie(adminCookie)
	uploadRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadRecorder, upload)
	if uploadRecorder.Code != http.StatusOK || !strings.Contains(uploadRecorder.Body.String(), `"has_profile_image":true`) {
		t.Fatalf("profile upload status = %d: %s", uploadRecorder.Code, uploadRecorder.Body.String())
	}

	open := httptest.NewRequest(http.MethodGet, "/api/accounts/"+admin.ID+"/profile-image", nil)
	open.AddCookie(adminCookie)
	openRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(openRecorder, open)
	if openRecorder.Code != http.StatusOK || openRecorder.Header().Get("Content-Type") != "image/jpeg" ||
		openRecorder.Header().Get("Cache-Control") != "private, max-age=60" || !bytes.Equal(openRecorder.Body.Bytes(), imageBytes) {
		t.Fatalf("profile response = %d headers=%v equal=%t", openRecorder.Code, openRecorder.Header(), bytes.Equal(openRecorder.Body.Bytes(), imageBytes))
	}

	private := httptest.NewRequest(http.MethodGet, "/api/accounts/"+admin.ID+"/profile-image", nil)
	private.AddCookie(operatorCookie)
	privateRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(privateRecorder, private)
	if privateRecorder.Code != http.StatusNotFound {
		t.Fatalf("unlinked profile status = %d, want %d", privateRecorder.Code, http.StatusNotFound)
	}

	remove := httptest.NewRequest(http.MethodDelete, "/api/auth/profile-image", nil)
	remove.AddCookie(adminCookie)
	removeRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(removeRecorder, remove)
	if removeRecorder.Code != http.StatusOK || !strings.Contains(removeRecorder.Body.String(), `"has_profile_image":false`) {
		t.Fatalf("profile delete status = %d: %s", removeRecorder.Code, removeRecorder.Body.String())
	}
	missing := httptest.NewRequest(http.MethodGet, "/api/accounts/"+admin.ID+"/profile-image", nil)
	missing.AddCookie(adminCookie)
	missingRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("deleted profile status = %d, want %d", missingRecorder.Code, http.StatusNotFound)
	}
}

func TestControlIdentityAPIStartsAtSelfAndRejectsUnlinkedAccounts(t *testing.T) {
	server, accountStore := newAuthenticationTestServer(t, true, false, nil)
	admin, err := accountStore.BootstrapAdmin(t.Context(), "owner", "correct horse battery staple")
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	operator, err := accountStore.Create(t.Context(), "operator", "another excellent password", accounts.RoleOperator)
	if err != nil {
		t.Fatalf("Create operator: %v", err)
	}
	adminToken, _, err := accountStore.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("admin session: %v", err)
	}
	adminCookie := &http.Cookie{Name: loopbackSessionCookieName, Value: adminToken} // #nosec G124 -- test fixture.

	identities := httptest.NewRequest(http.MethodGet, "/api/auth/control-identities", nil)
	identities.AddCookie(adminCookie)
	identitiesRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(identitiesRecorder, identities)
	if identitiesRecorder.Code != http.StatusOK || !strings.Contains(identitiesRecorder.Body.String(), `"relationship":"self"`) ||
		!strings.Contains(identitiesRecorder.Body.String(), `"selected":true`) || strings.Contains(identitiesRecorder.Body.String(), operator.ID) {
		t.Fatalf("control identities = %d: %s", identitiesRecorder.Code, identitiesRecorder.Body.String())
	}

	selectUnlinked := httptest.NewRequest(http.MethodPut, "/api/auth/control-identity", strings.NewReader(`{"account_id":"`+operator.ID+`"}`))
	selectUnlinked.Header.Set("Content-Type", "application/json")
	selectUnlinked.AddCookie(adminCookie)
	selectRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(selectRecorder, selectUnlinked)
	if selectRecorder.Code != http.StatusForbidden {
		t.Fatalf("unlinked control selection status = %d: %s", selectRecorder.Code, selectRecorder.Body.String())
	}
}

func profileImageFixture(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(48 + x), G: uint8(72 + y), B: 120, A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode profile fixture: %v", err)
	}
	return output.Bytes()
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
