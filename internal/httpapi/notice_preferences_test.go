package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/notices"
)

func noticeRequest(s *Server, method, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/notice-preferences", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func noticeCookie(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	w := noticeRequest(s, http.MethodGet, "")
	if w.Code != http.StatusOK {
		t.Fatalf("notice read: %d %s", w.Code, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if strings.HasSuffix(cookie.Name, "MagicHandy-Notices") {
			return cookie
		}
	}
	t.Fatal("missing browser preference cookie")
	return nil
}

func readHidden(t *testing.T, s *Server, cookies ...*http.Cookie) notices.Snapshot {
	t.Helper()
	w := noticeRequest(s, http.MethodGet, "", cookies...)
	var result notices.Snapshot
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("notice read: %d %s", w.Code, w.Body.String())
	}
	return result
}

func assertBrowserNoticeIsolation(t *testing.T, s *Server, a, b *http.Cookie) {
	t.Helper()
	if !a.HttpOnly || a.SameSite != http.SameSiteStrictMode || a.Path != "/" || a.MaxAge <= 0 || a.Value == b.Value {
		t.Fatal("invalid preference cookie")
	}
	w := noticeRequest(s, http.MethodPut, `{"notice_id":"sign-in-safety","hidden":true}`, a)
	if w.Code != http.StatusOK {
		t.Fatalf("anonymous dismiss: %d %s", w.Code, w.Body.String())
	}
	if len(readHidden(t, s, b).Hidden) != 0 {
		t.Fatal("browser choices leaked")
	}
}

func TestNoticePreferencesArePrivateAndAccountChoicesFollowOtherBrowsers(t *testing.T) {
	s, store, _, adminCookie := newControllerSessionFixture(t)
	a, b := noticeCookie(t, s), noticeCookie(t, s)
	assertBrowserNoticeIsolation(t, s, a, b)
	w := noticeRequest(s, http.MethodPut, `{"notice_id":"model-generation","hidden":true}`, a, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("account dismiss: %d %s", w.Code, w.Body.String())
	}
	other := readHidden(t, s, b, adminCookie)
	if other.Scope != "account" || !slices.Equal(other.Hidden, []string{"model-generation"}) {
		t.Fatalf("account choices did not follow: %+v", other)
	}
	operator, err := store.Create(t.Context(), "notice-observer", "synthetic notice password", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	opCookie := testSessionCookie(token)
	if len(readHidden(t, s, b, opCookie).Hidden) != 0 {
		t.Fatal("account choices leaked")
	}
	w = noticeRequest(s, http.MethodPut, `{"notice_id":"device-firmware","hidden":true}`, b, opCookie)
	if w.Code != http.StatusOK {
		t.Fatal("observer cannot change own preferences", w.Code)
	}
	if noticeRequest(s, http.MethodDelete, "", a, adminCookie).Code != http.StatusOK {
		t.Fatal("restore failed")
	}
	if len(readHidden(t, s, a, adminCookie).Hidden) != 0 || len(readHidden(t, s, b, opCookie).Hidden) != 1 {
		t.Fatal("restore crossed account boundary")
	}
	var raw string
	if err = s.store.Datastore().SQL().QueryRow(`SELECT owner_key FROM notice_preferences LIMIT 1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, a.Value) || strings.Contains(w.Body.String(), a.Value) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("preference identity disclosed or response cached")
	}
}

func TestNoticePreferencesRejectUnknownIDsForeignOwnersAndExpiredScope(t *testing.T) {
	s, store, _, adminCookie := newControllerSessionFixture(t)
	browser := noticeCookie(t, s)
	for _, body := range []string{`{"notice_id":"emergency-stop-control","hidden":true}`, `{"notice_id":"model-generation","hidden":true,"account_id":"someone-else"}`, `{"notice_id":"model-generation"}`, strings.Repeat("x", 1025)} {
		if w := noticeRequest(s, http.MethodPut, body, browser, adminCookie); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid preference accepted: %d", w.Code)
		}
	}
	if w := noticeRequest(s, http.MethodPut, `{"notice_id":"sign-in-safety","hidden":true}`); w.Code != http.StatusBadRequest {
		t.Fatal("write without browser identity accepted")
	}
	session, err := store.ResolveSession(t.Context(), adminCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RevokeSession(t.Context(), adminCookie.Value); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPut, "/api/notice-preferences", strings.NewReader(`{"notice_id":"model-generation","hidden":true}`))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(browser)
	r = r.WithContext(context.WithValue(r.Context(), authenticatedSessionContextKey{}, authenticatedSessionState{session: session, token: adminCookie.Value}))
	w := httptest.NewRecorder()
	s.handleNoticePreferences(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked queued write accepted: %d", w.Code)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/notice-preferences", nil)
	r.AddCookie(browser)
	r.Header.Set("X-MagicHandy-Notice-Scope", "account")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("expired account silently switched to browser preferences")
	}
}

func TestNoticePreferencesKeepSameOriginBoundaryAndSecureCookie(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	s.auth.options.SecureCookies = true
	browser := noticeCookie(t, s)
	if !browser.Secure || browser.Name != "__Host-MagicHandy-Notices" {
		t.Fatal("HTTPS preference cookie is not host-scoped and secure")
	}
	r := httptest.NewRequest(http.MethodPut, "/api/notice-preferences", strings.NewReader(`{"notice_id":"sign-in-safety","hidden":true}`))
	r.AddCookie(browser)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://foreign.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin accepted: %d", w.Code)
	}
}
