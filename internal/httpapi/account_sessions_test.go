package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func readOwnSessions(t *testing.T, s *Server, cookie *http.Cookie) []managedSessionView {
	t.Helper()
	response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/auth/sessions", "session-review", "")
	if response.Code != http.StatusOK || response.Body.Len() > 32<<10 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("session list response: %d, %d bytes", response.Code, response.Body.Len())
	}
	var body struct {
		Sessions []managedSessionView `json:"sessions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Sessions
}

func TestOwnSessionRoutesRequireLoginAndRespectAccountBoundary(t *testing.T) {
	s, store, _, adminCookie := newControllerSessionFixture(t)
	operator, err := store.Create(t.Context(), "observer", "synthetic observer passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	token, session, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := testSessionCookie(token)
	sessions := readOwnSessions(t, s, cookie)
	if len(sessions) != 1 || !sessions[0].Current || sessions[0].ID != session.ID || sessions[0].Controller || sessions[0].DeviceGateway {
		t.Fatal("observer session view is incorrect")
	}
	data, err := json.Marshal(sessions)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) || strings.Contains(string(data), session.Key) {
		t.Fatal("session response disclosed authentication material")
	}
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		response := authenticatedControlRequest(s, adminCookie, method, "/api/auth/sessions/"+session.ID, "review", `{"name":"not my device"}`)
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-account session write: %d", response.Code)
		}
	}
	response := authenticatedControlRequest(s, cookie, http.MethodPatch, "/api/auth/sessions/"+session.ID, "review", `{"name":"My phone"}`)
	if response.Code != http.StatusOK || readOwnSessions(t, s, cookie)[0].Name != "My phone" {
		t.Fatal("observer cannot name own login")
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(method, "/api/auth/sessions", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous session access: %d", w.Code)
		}
	}
}

func TestSelfRevocationAcknowledgesAndClosesExistingHTTPStream(t *testing.T) {
	for _, route := range []string{"session", "logout"} {
		t.Run(route, func(t *testing.T) {
			s, store, _, cookie := newControllerSessionFixture(t)
			claimAuthenticatedController(t, s, cookie)
			session, err := store.InspectSession(t.Context(), cookie.Value)
			if err != nil {
				t.Fatal(err)
			}
			host := httptest.NewServer(s.Handler())
			defer host.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			r, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+"/api/motion/events?client_id=observer", nil)
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
				t.Fatal("motion stream did not start")
			}
			s.access.mu.Lock()
			active := s.access.sessions[session.Key]
			s.access.mu.Unlock()
			if active == nil {
				t.Fatal("stream was not registered")
			}
			method, path, wantStatus := http.MethodDelete, "/api/auth/sessions/"+session.ID, http.StatusOK
			if route == "logout" {
				method, path, wantStatus = http.MethodPost, "/api/auth/logout", http.StatusNoContent
			}
			r, err = http.NewRequestWithContext(ctx, method, host.URL+path, strings.NewReader(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set(controllerHeaderName, "test-controller")
			reply, err := host.Client().Do(r)
			if err != nil {
				t.Fatalf("self-revocation acknowledgement: %v", err)
			}
			body, err := io.ReadAll(reply.Body)
			_ = reply.Body.Close()
			if err != nil || reply.StatusCode != wantStatus {
				t.Fatalf("self-revocation framing: %d, %v", reply.StatusCode, err)
			}
			if route == "session" && !strings.Contains(string(body), `"current_revoked":true`) {
				t.Fatal("current-session acknowledgement missing")
			}
			if active.ctx.Err() == nil {
				t.Fatal("revocation acknowledged before active work lost access")
			}
			_, _ = io.Copy(io.Discard, reader)
			if ctx.Err() != nil {
				t.Fatal("revoked stream survived until client timeout")
			}
			if _, err := store.InspectSession(t.Context(), cookie.Value); !errors.Is(err, accounts.ErrInvalidSession) {
				t.Fatal("revoked login still authenticates")
			}
			assertSessionCookieCleared(t, reply.Cookies())
		})
	}
}

func assertSessionCookieCleared(t *testing.T, cookies []*http.Cookie) {
	t.Helper()
	for _, value := range cookies {
		if value.Name == loopbackSessionCookieName && value.MaxAge < 0 {
			return
		}
	}
	t.Fatal("self-revocation did not clear browser cookie")
}

func TestRevokingPeerGatewayKeepsManagementAcknowledgement(t *testing.T) {
	s, store, admin, gatewayCookie, _ := newGatewaySessionFixture(t)
	token, actor, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	actorCookie := testSessionCookie(token)
	response := authenticatedControlRequest(s, actorCookie, http.MethodPost, "/api/controller/takeover", "remote-controller", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("synthetic controller handoff: %d", response.Code)
	}
	peer, err := store.InspectSession(t.Context(), gatewayCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	response = authenticatedControlRequest(s, actorCookie, http.MethodDelete, "/api/auth/sessions/"+peer.ID, "remote-controller", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"current_revoked":false`) {
		t.Fatal("peer gateway revocation interrupted acknowledgement")
	}
	if key := s.bluetoothGatewaySessionKey(); key != "" {
		t.Fatal("revoked login retained the device gateway")
	}
	if _, err := store.CheckSession(t.Context(), actor.Key); err != nil {
		t.Fatal("revoking gateway ended the caller's login")
	}
}

func TestSessionClientHintsAreCoarseAndBounded(t *testing.T) {
	for _, item := range []struct{ agent, browser, platform string }{
		{"Mozilla Windows Chrome/123 Safari/537 Edg/123", "edge", "windows"},
		{"Mozilla iPhone CriOS/120 Mobile/123 Safari/123", "chrome", "ios"},
		{"Mozilla Macintosh Mobile/123 Safari/123", "safari", "ios"},
		{"Mozilla Android Firefox/120", "firefox", "android"},
		{strings.Repeat("x", 512) + "secret host path Windows Chrome/100", "other", "other"},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		r.Header.Set("User-Agent", item.agent)
		if hint := sessionClientHint(r); hint.Browser != item.browser || hint.Platform != item.platform {
			t.Fatalf("client classification: %+v", hint)
		}
	}
}
