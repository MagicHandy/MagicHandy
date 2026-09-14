package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestAdmissionBoundsPeersAndReleasesAllEntries(t *testing.T) {
	var admission requestAdmissionRuntime
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for i := 0; i < requestLaneLimits[ordinaryLane].global; i++ {
		peer := fmt.Sprintf("synthetic-peer-%d", i/requestLaneLimits[ordinaryLane].peer)
		release, ok := admission.enter(ordinaryLane, peer)
		if !ok {
			t.Fatal("ordinary request rejected before its bound")
		}
		releases = append(releases, release)
		if i+1 == requestLaneLimits[ordinaryLane].peer {
			if _, ok := admission.enter(ordinaryLane, peer); ok {
				t.Fatal("one peer exceeded its budget")
			}
		}
	}
	if _, ok := admission.enter(ordinaryLane, "extra"); ok {
		t.Fatal("global request budget was exceeded")
	}
	for _, lane := range []requestLane{loginLane, shellLane, livenessLane, controlLane} {
		release, ok := admission.enter(lane, "synthetic-peer-0")
		if !ok {
			t.Fatal("ordinary work exhausted another lane")
		}
		release()
	}
	for _, release := range releases {
		release()
	}
	releases = nil
	for _, lane := range admission.snapshot() {
		if lane.Active != 0 {
			t.Fatal("request capacity leaked")
		}
	}
	if len(admission.lanes[ordinaryLane].peers) != 0 {
		t.Fatal("inactive peer identities accumulated")
	}
}

func TestRequestPressurePreservesOwnerHeartbeatControlAndPublicStop(t *testing.T) {
	s, store, admin, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	other := newAdmissionIdentity(t, store, admin.ID, "pressure-observer", false)
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	// Represent occupied ordinary handlers at both admission stages.
	for i := 0; i < requestLaneLimits[ordinaryLane].global; i++ {
		release, ok := s.requestAdmission.enter(ordinaryLane, fmt.Sprintf("peer-%d", i/64))
		if !ok {
			t.Fatal("could not occupy ordinary capacity")
		}
		releases = append(releases, release)
	}
	for range 32 {
		entry, leave := s.access.enter(s.controller.SessionKey())
		if entry == nil {
			t.Fatal("could not occupy owner ordinary capacity")
		}
		releases = append(releases, leave)
	}
	observer := authenticatedControlRequest(s, other.cookie, http.MethodGet, "/api/state", "test-controller", "")
	if observer.Code != http.StatusTooManyRequests {
		t.Fatal("observer obtained the owner's reservation")
	}
	for _, request := range []struct{ path, body string }{
		{"/api/controller/heartbeat", `{}`}, {"/api/motion/quick", `{"speed_min_percent":18}`},
	} {
		result := authenticatedControlRequest(s, cookie, http.MethodPost, request.path, "test-controller", request.body)
		if result.Code != http.StatusOK {
			t.Fatalf("owner request blocked by ordinary work: %s %d %s", request.path, result.Code, result.Body.String())
		}
	}
	state := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/state", "test-controller", "")
	if state.Code != http.StatusOK {
		t.Fatal("current controller could not reconcile state")
	}
	stop := httptest.NewRecorder()
	s.Handler().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	if stop.Code != http.StatusOK || s.stopSequence.Load() != 1 {
		t.Fatal("public Stop waited for ordinary capacity")
	}
}

func TestReservedLaneDoesNotAuthenticateAnEndedSession(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	r := httptest.NewRequest(http.MethodPost, "/api/controller/heartbeat", nil)
	r.Header.Set(controllerHeaderName, "test-controller")
	r.AddCookie(cookie)
	if s.requestLane(r) != livenessLane {
		t.Fatal("owner had no heartbeat reservation")
	}
	if err := store.RevokeSession(t.Context(), cookie.Value); err != nil {
		t.Fatal(err)
	}
	// The cached owner may still match until the watchdog runs. Eligibility
	// must never replace the real account/session check below it.
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("reservation bypassed session validation: %d", w.Code)
	}
}

func TestGatewayDeliveryReservationSurvivesOrdinaryRequestPressure(t *testing.T) {
	s, store, admin, cookie, gateway := newGatewaySessionFixture(t)
	other := newAdmissionIdentity(t, store, admin.ID, "gateway-pressure-observer", false)
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for i := range requestLaneLimits[ordinaryLane].global {
		release, ok := s.requestAdmission.enter(ordinaryLane, fmt.Sprintf("peer-%d", i/64))
		if !ok {
			t.Fatal("could not occupy ordinary capacity")
		}
		releases = append(releases, release)
	}
	queueGatewayFixture(t, s, "hsp/state")
	const route = "/api/transport/bluetooth/commands?client_id=device-browser&wait=0"
	for _, test := range []struct {
		cookie *http.Cookie
		status int
	}{{other.cookie, http.StatusTooManyRequests}, {cookie, http.StatusOK}} {
		response := serveGatewayFixture(s, gatewayFixtureRequest(test.cookie, gatewayTestTab, http.MethodGet, route, "", gateway))
		if response.Code != test.status {
			t.Fatalf("gateway reservation: status=%d want=%d", response.Code, test.status)
		}
	}
}
