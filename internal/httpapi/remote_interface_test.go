package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/remote"
)

func remotePortalRequest(s *Server, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:49718"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:49718")
	r.Header.Set(controllerHeaderName, "remote-phone")
	r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.RemoteHandler(RemoteInterfaceOptions{}).ServeHTTP(w, r)
	return w
}

func remoteOnlyLogin(t *testing.T, s *Server, store *accounts.Store, admin accounts.Account, desktop *http.Cookie) (accounts.Account, *http.Cookie) {
	t.Helper()
	actor, err := store.InspectSession(t.Context(), desktop.Value)
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.CreateWithAccessForSession(t.Context(), actor.Key, "remote-user", "synthetic remote passphrase", accounts.RoleOperator, accounts.InterfaceRemote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GrantControl(t.Context(), admin.ID, account.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	r := remotePortalRequest(s, nil, http.MethodPost, "/api/auth/login", `{"username":"remote-user","password":"synthetic remote passphrase"}`)
	if r.Code != http.StatusOK || len(r.Result().Cookies()) != 1 {
		t.Fatalf("remote login: %d %s", r.Code, r.Body.String())
	}
	return account, r.Result().Cookies()[0]
}

func TestRemoteInterfaceLimitsAPIsAndBindsSessions(t *testing.T) {
	s, store, admin, desktop, _ := newRemoteFixture(t)
	_, cookie := remoteOnlyLogin(t, s, store, admin, desktop)
	for _, path := range []string{"/api/state", "/api/settings", "/api/accounts", "/api/chat/messages", "/api/media/videos", "/api/diagnostics", "/api/controller"} {
		r := remotePortalRequest(s, cookie, http.MethodGet, path, "")
		if r.Code != http.StatusNotFound {
			t.Errorf("remote exposed %s: %d", path, r.Code)
		}
	}
	for _, path := range []string{"/api/controller/takeover", "/api/motion/start", "/api/remote/presence", "/api/remote/commands/anything/claim", "/api/auth/bootstrap"} {
		r := remotePortalRequest(s, cookie, http.MethodPost, path, `{}`)
		if r.Code != http.StatusNotFound && r.Code != http.StatusMethodNotAllowed {
			t.Errorf("remote exposed %s: %d", path, r.Code)
		}
	}
	// Renaming a cookie cannot turn either token into a login to the other app.
	for _, test := range []struct {
		remote bool
		token  string
	}{{true, desktop.Value}, {false, cookie.Value}} {
		var r *httptest.ResponseRecorder
		if test.remote {
			r = remotePortalRequest(s, &http.Cookie{Name: cookie.Name, Value: test.token, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}, http.MethodGet, "/api/remote/state", "")
		} else {
			r = authenticatedRemoteRequest(s, &http.Cookie{Name: desktop.Name, Value: test.token, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}, http.MethodGet, "/api/state", "phone", "")
		}
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("cross-interface token accepted: %d", r.Code)
		}
	}
	login := authenticatedRemoteRequest(s, &http.Cookie{Name: loopbackSessionCookieName, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}, http.MethodPost, "/api/auth/login", "phone", `{"username":"remote-user","password":"synthetic remote passphrase"}`)
	if login.Code != http.StatusForbidden || len(login.Result().Cookies()) != 0 {
		t.Fatalf("remote-only main login: %d", login.Code)
	}
	status := remotePortalRequest(s, cookie, http.MethodGet, "/api/auth/status", "")
	var payload struct {
		Interface    string              `json:"interface"`
		Capabilities accountCapabilities `json:"capabilities"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Interface != "remote" || payload.Capabilities.Control || payload.Capabilities.ConfigureHost || payload.Capabilities.SharedData || !payload.Capabilities.RemoteControl {
		t.Fatalf("remote capabilities: %+v", payload)
	}
}

func TestRemoteOnlyGrantTargetsIssuerAndCannotSurviveReplacement(t *testing.T) {
	s, store, admin, desktop, _ := newRemoteFixture(t)
	account, cookie := remoteOnlyLogin(t, s, store, admin, desktop)
	state := remotePortalRequest(s, cookie, http.MethodGet, "/api/remote/state", "")
	if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), "Take 07") {
		t.Fatalf("delegated desktop: %d %s", state.Code, state.Body.String())
	}
	sent := remotePortalRequest(s, cookie, http.MethodPost, "/api/remote/commands", `{"target":"video","action":"pause","video_id":"clip"}`)
	var payload struct {
		Command remote.Command `json:"command"`
	}
	if sent.Code != http.StatusAccepted {
		t.Fatalf("send: %d %s", sent.Code, sent.Body.String())
	}
	if err := json.Unmarshal(sent.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GrantControl(t.Context(), admin.ID, account.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	claim := authenticatedRemoteRequest(s, desktop, http.MethodPost, "/api/remote/commands/"+payload.Command.ID+"/claim", "test-controller", `{}`)
	if claim.Code != http.StatusConflict {
		t.Fatalf("old grant command claimed: %d %s", claim.Code, claim.Body.String())
	}
	if err := store.RevokeControl(t.Context(), admin.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	if got := remotePortalRequest(s, cookie, http.MethodGet, "/api/remote/state", ""); got.Code != http.StatusForbidden {
		t.Fatalf("revoked read: %d", got.Code)
	}
	if got := remotePortalRequest(s, cookie, http.MethodPost, "/api/motion/stop", `{}`); got.Code != http.StatusOK {
		t.Fatalf("Stop after revocation: %d %s", got.Code, got.Body.String())
	}
}

func TestRemoteLogoutDoesNotClearMainCookie(t *testing.T) {
	s, store, admin, desktop, _ := newRemoteFixture(t)
	_, cookie := remoteOnlyLogin(t, s, store, admin, desktop)
	r := remotePortalRequest(s, cookie, http.MethodPost, "/api/auth/logout", `{}`)
	if r.Code != http.StatusNoContent || r.Header().Get("Clear-Site-Data") != "" {
		t.Fatalf("logout cleared another interface: %d %v", r.Code, r.Header())
	}
	if cookies := r.Result().Cookies(); len(cookies) != 1 || cookies[0].Name != cookie.Name || cookies[0].MaxAge != -1 {
		t.Fatalf("incorrect cookie deletion: %v", cookies)
	}
	if _, err := store.InspectSession(t.Context(), desktop.Value); err != nil {
		t.Fatal("desktop login lost", err)
	}
}

// siblingPageLoad is what a browser sends when a link on another MagicHandy
// port opens this one, such as the main app's Remote menu item.
func siblingPageLoad(method, host, path string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(method, "http://"+host+path, nil)
	r.Header.Set("Sec-Fetch-Site", "same-site")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.Header.Set("Sec-Fetch-User", "?1")
	for name, value := range headers {
		r.Header.Set(name, value)
	}
	return r
}

func TestSiblingPortLinksOpenThePageButAPIsStaySameOrigin(t *testing.T) {
	s := newTestServer(t)
	for host, handler := range map[string]http.Handler{
		"127.0.0.1:49717": s.Handler(),
		"127.0.0.1:49718": s.RemoteHandler(RemoteInterfaceOptions{}),
	} {
		t.Run(host, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, siblingPageLoad(http.MethodGet, host, "/", nil))
			if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("page opened from the sibling port: %d %q %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
			}
			for name, r := range map[string]*http.Request{
				"API page load":    siblingPageLoad(http.MethodGet, host, "/api/auth/status", nil),
				"form post":        siblingPageLoad(http.MethodPost, host, "/", nil),
				"other site":       siblingPageLoad(http.MethodGet, host, "/", map[string]string{"Sec-Fetch-Site": "cross-site"}),
				"script fetch":     siblingPageLoad(http.MethodGet, host, "/", map[string]string{"Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"}),
				"frame":            siblingPageLoad(http.MethodGet, host, "/", map[string]string{"Sec-Fetch-Dest": "iframe"}),
				"with an Origin":   siblingPageLoad(http.MethodGet, host, "/", map[string]string{"Origin": "http://127.0.0.1:49717"}),
				"rebound hostname": siblingPageLoad(http.MethodGet, "attacker.example:49718", "/", nil),
			} {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != http.StatusForbidden {
					t.Errorf("%s: status %d, want %d", name, w.Code, http.StatusForbidden)
				}
			}
		})
	}
}

func TestRemoteInterfaceRejectsMainOriginButKeepsPublicStop(t *testing.T) {
	s, _, _, _, _ := newRemoteFixture(t)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:49718/api/motion/stop", strings.NewReader(`{}`))
	r.Header.Set("Origin", "http://127.0.0.1:49717")
	w := httptest.NewRecorder()
	s.RemoteHandler(RemoteInterfaceOptions{}).ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-port CSRF accepted: %d", w.Code)
	}
	if got := remotePortalRequest(s, nil, http.MethodPost, "/api/motion/stop", `{}`); got.Code != http.StatusOK {
		t.Fatalf("public Stop: %d", got.Code)
	}
}
