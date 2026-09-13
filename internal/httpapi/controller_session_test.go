package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func claimAuthenticatedController(t *testing.T, server *Server, cookie *http.Cookie) controllerSnapshot {
	t.Helper()
	response := authenticatedControlRequest(server, cookie, http.MethodPost, "/api/controller/heartbeat", "test-controller", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("heartbeat: %d %s", response.Code, response.Body.String())
	}
	var snapshot controllerSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Active || !snapshot.HeartbeatRequired || snapshot.Generation == 0 {
		t.Fatalf("heartbeat did not establish protected control: %+v", snapshot)
	}
	return snapshot
}

func authenticatedControlRequest(server *Server, cookie *http.Cookie, method, route, clientID, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, route, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(controllerHeaderName, clientID)
	server.controller.mu.Lock()
	r.Header.Set(controllerGenerationHeader, strconv.FormatUint(server.controller.generation, 10))
	r.Header.Set(controllerEpochHeader, server.controller.epoch)
	server.controller.mu.Unlock()
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, r)
	return w
}

func newControllerSessionFixture(t *testing.T) (*Server, *accounts.Store, accounts.Account, *http.Cookie) {
	t.Helper()
	settings, err := config.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := accounts.New(settings.Datastore())
	if err != nil {
		t.Fatal(err)
	}
	fake := transport.NewFake()
	s := newTestServerWithStore(t, settings, Runtime{
		Accounts: store, AuthenticationRequired: true, MotionTransport: fake, Transport: fake,
	})
	admin, err := store.BootstrapAdmin(t.Context(), "owner", "a long review passphrase")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, store, admin, testSessionCookie(token)
}

func testSessionCookie(token string) *http.Cookie {
	return &http.Cookie{Name: loopbackSessionCookieName, Value: token, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
}

func TestControllerRejectsCopiedTabIDAcrossAuthenticatedSessions(t *testing.T) {
	s, store, admin, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	operator, err := store.Create(t.Context(), "operator", "another long passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GrantControl(t.Context(), admin.ID, operator.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, account := range []accounts.Account{admin, operator} {
		token, _, err := store.NewSession(t.Context(), account.ID)
		if err != nil {
			t.Fatal(err)
		}
		copied := testSessionCookie(token)
		for _, route := range []string{"/api/controller", "/api/controller/heartbeat"} {
			method := http.MethodGet
			if strings.HasSuffix(route, "heartbeat") {
				method = http.MethodPost
			}
			response := authenticatedControlRequest(s, copied, method, route, "test-controller", `{}`)
			var snapshot controllerSnapshot
			if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || snapshot.Active || !snapshot.ReadOnly {
				t.Fatalf("copied ID gained ownership through %s: %s", route, response.Body.String())
			}
		}
		response := authenticatedControlRequest(s, copied, http.MethodPost, "/api/motion/start", "test-controller", `{"speed_percent":20}`)
		if response.Code != http.StatusConflict {
			t.Fatalf("copied ID started motion: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestAuthenticatedObservationDoesNotClaimOrRenewControl(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/controller", "test-controller", ``)
	var snapshot controllerSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Active || snapshot.Generation != 0 {
		t.Fatalf("observation claimed control: %+v", snapshot)
	}
	claimAuthenticatedController(t, s, cookie)
	s.controller.mu.Lock()
	lastSeen := time.Now().Add(-5 * time.Second)
	s.controller.lastSeenAt = lastSeen
	s.controller.mu.Unlock()
	_ = authenticatedControlRequest(s, cookie, http.MethodGet, "/api/state", "test-controller", ``)
	s.controller.mu.Lock()
	unchanged := s.controller.lastSeenAt.Equal(lastSeen)
	s.controller.mu.Unlock()
	if !unchanged {
		t.Fatal("state polling renewed protected control")
	}
}

func TestControllerWatchdogStopsExpiredLeaseAndRequiresExplicitTakeover(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	initial := claimAuthenticatedController(t, s, cookie)
	started := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/motion/start", "test-controller", `{"speed_percent":20}`)
	if started.Code != http.StatusOK {
		t.Fatalf("start: %d %s", started.Code, started.Body.String())
	}
	s.controller.mu.Lock()
	s.controller.lastSeenAt = time.Now().Add(-controllerLeaseTTL - time.Second)
	s.controller.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (s.stopSequence.Load() == 0 || s.currentMotionEngine().Snapshot().Running) {
		time.Sleep(10 * time.Millisecond)
	}
	engine := s.currentMotionEngine()
	if s.stopSequence.Load() == 0 || engine == nil || engine.Snapshot().Running {
		t.Fatal("watchdog did not stop the abandoned controller")
	}
	response := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/controller/heartbeat", "test-controller", `{}`)
	var snapshot controllerSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Active || snapshot.Generation <= initial.Generation {
		t.Fatalf("heartbeat silently restored expired ownership: %+v", snapshot)
	}
	response = authenticatedControlRequest(s, cookie, http.MethodPost, "/api/controller/takeover", "test-controller", `{}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"active":true`) || engine.Snapshot().Running {
		t.Fatalf("explicit takeover did not recover stopped control: %s", response.Body.String())
	}
}

func TestBufferedStartCannotCrossEmergencyStop(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	initial := claimAuthenticatedController(t, s, cookie)
	stopped := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/motion/stop", "test-controller", `{}`)
	if stopped.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", stopped.Code, stopped.Body.String())
	}
	r := httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":20}`))
	r.AddCookie(cookie)
	r.Header.Set(controllerHeaderName, "test-controller")
	r.Header.Set(controllerGenerationHeader, strconv.FormatUint(initial.Generation, 10))
	r.Header.Set(controllerEpochHeader, initial.Epoch)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusConflict || s.currentMotionEngine() != nil {
		t.Fatalf("delayed pre-Stop command reached motion: %d %s", w.Code, w.Body.String())
	}
}

func TestRevokedSessionClosesAlreadyOpenMotionStream(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/api/motion/events?client_id=observer", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(cookie)
	response, err := httpServer.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	reader := bufio.NewReader(response.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != "event: motion\n" {
		t.Fatalf("stream opening: %q %v", line, err)
	}
	if err := store.RevokeSession(t.Context(), cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, reader)
	if ctx.Err() != nil {
		t.Fatal("revoked stream survived until the client timeout")
	}
}

func TestEmergencyStopBypassesAuthenticationStorage(t *testing.T) {
	// No account store is installed: any authentication lookup would panic.
	s := &Server{}
	called := false
	handler := s.authenticateRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil)
	cookie := &http.Cookie{Name: secureSessionCookieName, Value: "expired-or-stolen-cookie", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !called || w.Code != http.StatusNoContent {
		t.Fatal("Stop was gated by authentication")
	}
}

func TestOldControllerGenerationCannotCommandAfterReacquisition(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	initial := claimAuthenticatedController(t, s, cookie)
	for _, clientID := range []string{"second-tab", "test-controller"} {
		response := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/controller/takeover", clientID, `{}`)
		if response.Code != http.StatusOK {
			t.Fatalf("takeover: %d %s", response.Code, response.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":20}`))
	r.AddCookie(cookie)
	r.Header.Set(controllerHeaderName, "test-controller")
	r.Header.Set(controllerGenerationHeader, strconv.FormatUint(initial.Generation, 10))
	r.Header.Set(controllerEpochHeader, initial.Epoch)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusConflict || s.currentMotionEngine() != nil {
		t.Fatalf("old ownership generation reached motion: %d %s", w.Code, w.Body.String())
	}
}

func TestPriorServerEpochCannotCommandWithMatchingGeneration(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	initial := claimAuthenticatedController(t, s, cookie)
	for _, route := range []string{"/api/motion/start", "/api/controller/takeover"} {
		r := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"speed_percent":20}`))
		r.AddCookie(cookie)
		r.Header.Set(controllerHeaderName, "test-controller")
		r.Header.Set(controllerGenerationHeader, strconv.FormatUint(initial.Generation, 10))
		r.Header.Set(controllerEpochHeader, "previous-server-process")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusConflict || s.currentMotionEngine() != nil {
			t.Fatalf("old process regained authority via %s: %d", route, w.Code)
		}
	}
}

func TestDelayedHeartbeatCannotRenewNewOwnershipGeneration(t *testing.T) {
	controller := newControllerRuntime()
	actor := controllerIdentity{clientID: "tab", sessionKey: "session"}
	initial := controller.Heartbeat(actor, 0)
	controller.AdvanceStopGeneration()
	previous := time.Now().Add(-5 * time.Second)
	controller.lastSeenAt = previous
	controller.Heartbeat(actor, initial.Generation)
	if !controller.lastSeenAt.Equal(previous) {
		t.Fatal("delayed heartbeat renewed a different ownership generation")
	}
}
