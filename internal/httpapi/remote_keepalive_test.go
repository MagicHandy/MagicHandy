package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/remote"
)

// manualControllerClock lets a test age the controller lease without waiting.
func manualControllerClock(s *Server) func(time.Duration) {
	var mu sync.Mutex
	now := time.Now()
	s.controller.mu.Lock()
	s.controller.clock = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	s.controller.mu.Unlock()
	return func(d time.Duration) {
		mu.Lock()
		now = now.Add(d)
		mu.Unlock()
	}
}

func remoteKeepaliveRenewed(t *testing.T, s *Server, cookie *http.Cookie, tab string) bool {
	t.Helper()
	response := authenticatedRemoteRequest(s, cookie, http.MethodPost, "/api/remote/keepalive", tab, `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("keepalive: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Keepalive remoteKeepalive `json:"keepalive"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Keepalive.Renewed
}

func startRemoteFixtureMotion(t *testing.T, s *Server, desktop *http.Cookie) {
	t.Helper()
	if response := authenticatedRemoteRequest(s, desktop, http.MethodPost, "/api/motion/start", "test-controller", `{"pattern":"flow-full-sweeps","speed_percent":35}`); response.Code != http.StatusOK {
		t.Fatalf("start: %d %s", response.Code, response.Body.String())
	}
}

// A hidden desktop pauses its heartbeat (ADR 0029). A phone in front keeps
// its lease alive, so the lost-controller watchdog leaves the session running;
// once the phone goes quiet too, the usual Stop follows.
func TestRemoteKeepaliveKeepsAHiddenDesktopSession(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	startRemoteFixtureMotion(t, s, desktop)
	advance := manualControllerClock(s)
	owner := s.controller.Owner()

	for range 4 {
		advance(10 * time.Second)
		if !remoteKeepaliveRenewed(t, s, phone, "phone-tab") {
			t.Fatal("the phone did not renew its desktop's lease")
		}
		s.checkAccessLifetimes()
	}
	if !s.controller.OwnerLive(owner) || !s.currentMotionEngine().Snapshot().Running {
		t.Fatal("the session stopped while the phone kept it alive")
	}
	if s.controller.Owner() != owner {
		t.Fatal("the keepalive moved control")
	}

	advance(controllerLeaseTTL + time.Second)
	s.checkAccessLifetimes()
	deadline := time.Now().Add(3 * time.Second)
	for s.currentMotionEngine().Snapshot().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.currentMotionEngine().Snapshot().Running || s.controller.OwnerLive(owner) {
		t.Fatal("an abandoned session kept running after the phone went quiet")
	}
	if remoteKeepaliveRenewed(t, s, phone, "phone-tab") {
		t.Fatal("a keepalive revived an expired lease")
	}
}

func TestRemoteKeepaliveStaysWithinTheDesktopAccount(t *testing.T) {
	s, store, admin, _, _ := newRemoteFixture(t)
	operator := newAdmissionIdentity(t, store, admin.ID, "operator", true)
	advance := manualControllerClock(s)
	s.controller.mu.Lock()
	before := s.controller.lastSeenAt
	s.controller.mu.Unlock()
	advance(5 * time.Second)
	if remoteKeepaliveRenewed(t, s, operator.cookie, "operator-tab") {
		t.Fatal("another account kept the desktop alive")
	}
	s.controller.mu.Lock()
	after := s.controller.lastSeenAt
	s.controller.mu.Unlock()
	if !after.Equal(before) {
		t.Fatal("a refused keepalive renewed the lease")
	}
	if !passiveSessionRequest(httptest.NewRequest(http.MethodPost, "/api/remote/keepalive", nil)) {
		t.Fatal("an automatic keepalive would extend login idle time")
	}
}

func TestRemoteRenewOnlyExtendsALiveLease(t *testing.T) {
	now := time.Unix(1_000, 0)
	c := newControllerRuntime()
	c.clock = func() time.Time { return now }
	owner := controllerIdentity{clientID: "desktop-tab", sessionKey: "desktop-login"}
	if c.RemoteRenew(owner) {
		t.Fatal("renewed without a controller")
	}
	c.Heartbeat(owner, 0)
	tickets := len(c.tickets)
	now = now.Add(10 * time.Second)
	if c.RemoteRenew(controllerIdentity{clientID: "phone-tab", sessionKey: "desktop-login"}) {
		t.Fatal("renewed for another tab")
	}
	// The renewal issues the hidden owner's next ticket (ADR 0056).
	if !c.RemoteRenew(owner) || !c.lastSeenAt.Equal(now) || len(c.tickets) != tickets+1 {
		t.Fatal("a live lease was not renewed with a fresh command ticket")
	}
	now = now.Add(controllerLeaseTTL + time.Second)
	if c.RemoteRenew(owner) || c.OwnerLive(owner) {
		t.Fatal("an expired lease was revived")
	}
}

func TestRemoteRenewNeverCrossesAHandoff(t *testing.T) {
	c := newControllerRuntime()
	owner := controllerIdentity{clientID: "desktop-tab", sessionKey: "desktop-login"}
	c.Heartbeat(owner, 0)
	if _, started, err := c.BeginTakeover(controllerIdentity{clientID: "laptop-tab", sessionKey: "laptop-login"}); err != nil || !started {
		t.Fatalf("takeover: %v %v", started, err)
	}
	if c.RemoteRenew(owner) {
		t.Fatal("a keepalive renewed a lease under takeover")
	}
}

// The phone's view of the session outlives the desktop's presence, which a
// hidden desktop withdraws, and stays within the desktop's account.
func TestRemoteSessionOutlivesWithdrawnPresence(t *testing.T) {
	s, store, admin, desktop, phone := newRemoteFixture(t)
	startRemoteFixtureMotion(t, s, desktop)
	if response := authenticatedRemoteRequest(s, desktop, http.MethodDelete, "/api/remote/presence", "test-controller", ""); response.Code != http.StatusOK {
		t.Fatalf("withdraw: %d", response.Code)
	}
	state := readRemoteState(t, s, phone)
	if state.Connected || state.Session == nil || !state.Session.Alive || !state.Session.StopOnExpiry || !state.Session.Running || !state.Session.EdgeAllowed {
		t.Fatalf("hidden desktop session = %+v / %+v", state, state.Session)
	}
	operator := newAdmissionIdentity(t, store, admin.ID, "operator", true)
	if other := readRemoteState(t, s, operator.cookie); other.Session != nil && other.Session.Alive {
		t.Fatalf("another account saw the session: %+v", other.Session)
	}
}

func TestLocalRemoteSessionDoesNotPromiseStopOnLeaseExpiry(t *testing.T) {
	s := newTestServer(t)
	s.controller.Observe(controllerIdentity{clientID: "local-desktop"})
	session := s.remoteSession(t.Context(), remote.Identity{})
	if !session.Alive || session.StopOnExpiry {
		t.Fatalf("unprotected local session = %+v", session)
	}
}
