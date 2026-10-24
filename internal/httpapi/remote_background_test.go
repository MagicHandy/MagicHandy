package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func observeInBackground(t *testing.T, s *Server, cookie *http.Cookie, tab string) controllerSnapshot {
	t.Helper()
	response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/controller", tab, "", func(r *http.Request) {
		r.Header.Set(controllerBackgroundHeader, "1")
	})
	if response.Code != http.StatusOK {
		t.Fatalf("background observation: %d %s", response.Code, response.Body.String())
	}
	var snapshot controllerSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// commandWithTicket sends a control command with the ticket the tab observed,
// not the server's newest one.
func commandWithTicket(s *Server, cookie *http.Cookie, route, ticket string) int {
	return authenticatedRemoteRequest(s, cookie, http.MethodPost, route, "test-controller", `{}`, func(r *http.Request) {
		r.Header.Set(commandTicketHeader, ticket)
	}).Code
}

// A hidden desktop renews nothing itself (ADR 0029). While its phone keeps the
// session alive, the phone's renewals issue the tickets the hidden tab acts
// with; it learns them by observing in the background and loses that
// authority once the phone goes quiet (ADR 0056).
func TestPhoneHeldDesktopActsWithThePhonesTickets(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	startRemoteFixtureMotion(t, s, desktop)
	advance := manualControllerClock(s)

	advance(commandTicketLifetime + time.Second)
	hidden := observeInBackground(t, s, desktop, "test-controller")
	if !hidden.Active || hidden.RemoteHeld || hidden.CommandTicket != "" {
		t.Fatalf("a hidden desktop without a phone = %+v", hidden)
	}
	if !remoteKeepaliveRenewed(t, s, phone, "phone-tab") {
		t.Fatal("the phone did not renew its desktop's lease")
	}
	held := observeInBackground(t, s, desktop, "test-controller")
	if !held.Active || !held.RemoteHeld || held.CommandTicket == "" {
		t.Fatalf("a phone-held desktop = %+v", held)
	}
	if code := commandWithTicket(s, desktop, "/api/motion/pause", held.CommandTicket); code != http.StatusOK {
		t.Fatalf("the phone-held desktop could not act: %d", code)
	}

	// Observing in the background renews nothing: once the phone is quiet the
	// ticket lapses first, then the lease, and the usual Stop runs.
	advance(commandTicketLifetime + time.Second)
	observeInBackground(t, s, desktop, "test-controller")
	if code := commandWithTicket(s, desktop, "/api/motion/resume", held.CommandTicket); code != http.StatusConflict {
		t.Fatalf("a lapsed phone ticket admitted a command: %d", code)
	}
	advance(controllerLeaseTTL)
	s.checkAccessLifetimes()
	if late := observeInBackground(t, s, desktop, "test-controller"); late.Active || late.RemoteHeld {
		t.Fatalf("the desktop kept control after its phone went quiet: %+v", late)
	}
}

func TestPhoneHoldEndsWithTheLeaseAndNeverOutlivesAHandoff(t *testing.T) {
	now := time.Unix(1_000, 0)
	c := newControllerRuntime()
	c.clock = func() time.Time { return now }
	owner := controllerIdentity{clientID: "desktop-tab", sessionKey: "desktop-login"}
	c.Heartbeat(owner, 0)
	if c.Peek(owner).RemoteHeld {
		t.Fatal("held before any phone renewed the lease")
	}
	now = now.Add(5 * time.Second)
	if !c.RemoteRenew(owner) || !c.Peek(owner).RemoteHeld {
		t.Fatal("a phone renewal did not hold the session")
	}
	if other := c.Peek(controllerIdentity{clientID: "laptop-tab", sessionKey: "desktop-login"}); other.RemoteHeld {
		t.Fatal("another tab was told the phone holds it")
	}
	// The owner's own heartbeats keep the lease, but the hold lapses one lease
	// period after the last phone renewal.
	now = now.Add(10 * time.Second)
	c.Heartbeat(owner, c.generation)
	now = now.Add(6 * time.Second)
	c.Heartbeat(owner, c.generation)
	if !c.Peek(owner).Active || c.Peek(owner).RemoteHeld {
		t.Fatal("the hold outlived the phone's last renewal")
	}
	c.RemoteRenew(owner)
	if _, started, err := c.BeginTakeover(controllerIdentity{clientID: "laptop-tab", sessionKey: "laptop-login"}); err != nil || !started {
		t.Fatalf("takeover: %v %v", started, err)
	}
	if _, err := c.CompleteTakeover(controllerIdentity{clientID: "laptop-tab", sessionKey: "laptop-login"}); err != nil {
		t.Fatal(err)
	}
	if c.Peek(controllerIdentity{clientID: "laptop-tab", sessionKey: "laptop-login"}).RemoteHeld {
		t.Fatal("a new controller inherited the previous owner's phone hold")
	}
}

// An unprotected local tab claims and renews by observing. Hidden, it watches
// without keeping the lease, so another local tab can still take over once it
// lapses.
func TestBackgroundObservationNeverRenewsALocalLease(t *testing.T) {
	now := time.Unix(1_000, 0)
	c := newControllerRuntime()
	c.clock = func() time.Time { return now }
	local := controllerIdentity{clientID: "local-tab"}
	if !c.Observe(local).Active {
		t.Fatal("a local tab did not claim an idle server")
	}
	for range 3 {
		now = now.Add(6 * time.Second)
		c.Peek(local)
	}
	if c.Peek(local).Active {
		t.Fatal("background observations kept a local lease")
	}

	s := newTestServer(t)
	request := func(background bool) controllerSnapshot {
		r := withControllerID(httptest.NewRequest(http.MethodGet, "/api/controller", nil), "local-tab")
		if background {
			r.Header.Set(controllerBackgroundHeader, "1")
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		var snapshot controllerSnapshot
		if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	advance := manualControllerClock(s)
	if !request(false).Active {
		t.Fatal("the local tab did not claim control")
	}
	advance(10 * time.Second)
	request(true)
	advance(6 * time.Second)
	if request(true).Active {
		t.Fatal("a background observation renewed the local lease")
	}
}
