package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/audit"
)

func recoveryRequest(t *testing.T, s *Server, cookie *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestRecoveryManagementRequiresOwnLoginAndPassword(t *testing.T) {
	s, store, _, adminCookie := newControllerSessionFixture(t)
	operator, err := store.Create(t.Context(), "recovery-operator", "operator recovery password", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := testSessionCookie(token)
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		w := recoveryRequest(t, s, nil, method, "/api/auth/recovery-codes", nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous recovery management: %d", w.Code)
		}
	}
	wrong := recoveryRequest(t, s, cookie, http.MethodPost, "/api/auth/recovery-codes", map[string]string{"password": "incorrect password"})
	if wrong.Code != http.StatusForbidden || wrong.Header().Get("Set-Cookie") != "" {
		t.Fatal("wrong password changed authentication state")
	}
	foreign := recoveryRequest(t, s, adminCookie, http.MethodPost, "/api/auth/recovery-codes", map[string]string{"password": "a long review passphrase", "account_id": operator.ID})
	if foreign.Code != http.StatusBadRequest {
		t.Fatal("management accepted another account identity")
	}
	w := recoveryRequest(t, s, cookie, http.MethodPost, "/api/auth/recovery-codes", map[string]string{"password": "operator recovery password"})
	var codes accounts.RecoveryCodes
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &codes) != nil || len(codes.Codes) != 8 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("observer could not issue own recovery codes")
	}
	assertRecoveryStatusOwnership(t, s, cookie, adminCookie, codes)
	removed := recoveryRequest(t, s, cookie, http.MethodDelete, "/api/auth/recovery-codes", map[string]string{"password": "operator recovery password"})
	if removed.Code != http.StatusOK || !strings.Contains(removed.Body.String(), `"remaining":0`) {
		t.Fatal("recovery removal failed")
	}
	if _, err := store.RecoverPassword(t.Context(), operator.Username, codes.Codes[0], "replacement password"); !errors.Is(err, accounts.ErrInvalidRecoveryCode) {
		t.Fatal("removed code still authenticates", err)
	}
}

func assertRecoveryStatusOwnership(t *testing.T, s *Server, cookie, adminCookie *http.Cookie, codes accounts.RecoveryCodes) {
	t.Helper()
	for _, caller := range []struct {
		cookie *http.Cookie
		count  int
	}{{cookie, 8}, {adminCookie, 0}} {
		view := recoveryRequest(t, s, caller.cookie, http.MethodGet, "/api/auth/recovery-codes", nil)
		var status accounts.RecoveryStatus
		if view.Code != http.StatusOK || json.Unmarshal(view.Body.Bytes(), &status) != nil || status.Remaining != caller.count {
			t.Fatal("recovery status crossed an account boundary")
		}
		for _, code := range codes.Codes {
			if strings.Contains(view.Body.String(), code) {
				t.Fatal("status disclosed saved credentials")
			}
		}
	}
}

func TestRecoveryUsesGenericFailuresSharedThrottleAndSecretFreeAudit(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	session, err := store.InspectSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := store.ReplaceRecoveryCodes(t.Context(), session.Key, "a long review passphrase")
	if err != nil {
		t.Fatal(err)
	}
	var generic string
	for _, username := range []string{"owner", "missing-account"} {
		w := recoveryRequest(t, s, nil, http.MethodPost, "/api/auth/recover", map[string]string{"username": username, "code": "invalid recovery fixture", "password": "new recovery password"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("invalid recovery status %d", w.Code)
		}
		if generic == "" {
			generic = w.Body.String()
		} else if generic != w.Body.String() {
			t.Fatal("recovery disclosed account existence")
		}
	}
	// Seven additional password attempts exhaust the same account bucket used
	// by its first recovery attempt. Changing credential endpoints cannot reset it.
	for range 7 {
		// #nosec G101 -- intentionally incorrect, synthetic regression fixture.
		w := recoveryRequest(t, s, nil, http.MethodPost, "/api/auth/login", map[string]string{"username": "owner", "password": "wrong login fixture"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("premature login throttle: %d", w.Code)
		}
	}
	w := recoveryRequest(t, s, nil, http.MethodPost, "/api/auth/recover", map[string]string{"username": "owner", "code": codes.Codes[0], "password": "new recovery password"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("recovery bypassed password throttle")
	}
	s.accessAudit.Close()
	export := recoveryRequest(t, s, cookie, http.MethodGet, "/api/audit/export", nil)
	var page audit.Page
	if export.Code != http.StatusOK || json.Unmarshal(export.Body.Bytes(), &page) != nil {
		t.Fatal("audit export failed")
	}
	failed, throttled := 0, 0
	for _, event := range page.Events {
		if event.Kind == audit.CredentialCheckFailed {
			failed++
		}
		if event.Kind == audit.CredentialCheckThrottled {
			throttled++
		}
	}
	if failed != 2 || throttled != 1 {
		t.Fatalf("missing recovery security events: failed=%d throttled=%d", failed, throttled)
	}
	for _, secret := range []string{cookie.Value, session.Key, codes.Codes[0], "new recovery password", "invalid recovery fixture", "missing-account", "wrong login fixture"} {
		if strings.Contains(export.Body.String(), secret) {
			t.Fatal("credential audit disclosed submitted content")
		}
	}
}

func TestRecoveryRejectsCrossOriginAndMalformedBodiesWithoutConsumingCode(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	session, err := store.InspectSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := store.ReplaceRecoveryCodes(t.Context(), session.Key, "a long review passphrase")
	if err != nil {
		t.Fatal(err)
	}
	valid := fmt.Sprintf(`{"username":"owner","code":%q,"password":"recovered password"}`, codes.Codes[0])
	for _, item := range []struct {
		body, contentType, origin string
		status                    int
	}{
		{valid, "application/json", "https://untrusted.example", http.StatusForbidden},
		{valid, "text/plain", "", http.StatusUnsupportedMediaType},
		{strings.TrimSuffix(valid, "}") + `,"unexpected":true}`, "application/json", "", http.StatusBadRequest},
		{valid + ` {}`, "application/json", "", http.StatusBadRequest},
		{valid + strings.Repeat(" ", 8<<10), "application/json", "", http.StatusBadRequest},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/recover", strings.NewReader(item.body))
		r.Header.Set("Content-Type", item.contentType)
		if item.origin != "" {
			r.Header.Set("Origin", item.origin)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != item.status {
			t.Fatalf("invalid recovery admission: %d, want %d", w.Code, item.status)
		}
	}
	if _, err := store.RecoverPassword(t.Context(), "owner", codes.Codes[0], "recovered password"); err != nil {
		t.Fatal("rejected request consumed the credential", err)
	}
}

func TestRecoveryClosesLiveStreamsAndRetiresControllerBeforeAcknowledging(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		t.Run(fmt.Sprint("http2=", http2), func(t *testing.T) {
			s, store, _, cookie := newControllerSessionFixture(t)
			claimAuthenticatedController(t, s, cookie)
			session, err := store.InspectSession(t.Context(), cookie.Value)
			if err != nil {
				t.Fatal(err)
			}
			codes, err := store.ReplaceRecoveryCodes(t.Context(), session.Key, "a long review passphrase")
			if err != nil {
				t.Fatal(err)
			}
			host := httptest.NewUnstartedServer(s.Handler())
			host.EnableHTTP2 = http2
			if http2 {
				host.StartTLS()
			} else {
				host.Start()
			}
			defer host.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			r, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+"/api/motion/events?client_id=recovery-observer", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.AddCookie(cookie)
			stream, err := host.Client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stream.Body.Close() }()
			reader := bufio.NewReader(stream.Body)
			if line, err := reader.ReadString('\n'); err != nil || line != "event: motion\n" {
				t.Fatal("stream did not open", err)
			}
			s.access.mu.Lock()
			active := s.access.sessions[session.Key]
			s.access.mu.Unlock()
			if active == nil {
				t.Fatal("stream had no session lifetime")
			}
			recoverThroughHTTP(ctx, t, host, cookie, codes.Codes[0])
			if active.ctx.Err() == nil {
				t.Fatal("recovery acknowledged before revoking active work")
			}
			_, _ = io.Copy(io.Discard, reader)
			if ctx.Err() != nil {
				t.Fatal("revoked stream survived until client timeout")
			}
			if _, err := store.CheckSession(t.Context(), session.Key); !errors.Is(err, accounts.ErrInvalidSession) {
				t.Fatal("recovered login still authenticates", err)
			}
			for deadline := time.Now().Add(time.Second); s.stopSequence.Load() == 0 && time.Now().Before(deadline); {
				time.Sleep(time.Millisecond)
			}
			if s.stopSequence.Load() == 0 || s.controller.SessionKey() != "" {
				t.Fatal("recovery did not retire controller through Stop")
			}
		})
	}
}

func recoverThroughHTTP(ctx context.Context, t *testing.T, host *httptest.Server, cookie *http.Cookie, code string) {
	t.Helper()
	body := fmt.Sprintf(`{"username":"owner","code":%q,"password":"recovered account password"}`, code)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, host.URL+"/api/auth/recover", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(cookie)
	reply, err := host.Client().Do(r)
	if err != nil {
		t.Fatal("recovery acknowledgement interrupted", err)
	}
	data, err := io.ReadAll(reply.Body)
	_ = reply.Body.Close()
	if err != nil || reply.StatusCode != http.StatusOK || !strings.Contains(string(data), `"recovered":true`) {
		t.Fatal("recovery acknowledgement incomplete", err)
	}
	assertSessionCookieCleared(t, reply.Cookies())
}
