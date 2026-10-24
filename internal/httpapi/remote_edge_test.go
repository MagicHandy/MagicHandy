package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/audit"
	"github.com/mapledaemon/MagicHandy/internal/remote"
)

func remoteEdgeSession(t *testing.T, body []byte) remote.Session {
	t.Helper()
	var payload struct {
		Session *remote.Session `json:"session"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Session == nil {
		t.Fatalf("edge response %s: %v", body, err)
	}
	return *payload.Session
}

// The phone's Edge reaches the session directly, so it works while the desktop
// tab is hidden and has withdrawn its presence, and Return restores the motion.
func TestRemoteEdgeHoldsAndReturnsAHiddenDesktopSession(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	if response := authenticatedRemoteRequest(s, phone, http.MethodPost, "/api/remote/edge", "phone-tab", `{}`); response.Code != http.StatusConflict {
		t.Fatalf("Edge without motion: %d %s", response.Code, response.Body.String())
	}
	startRemoteFixtureMotion(t, s, desktop)
	if response := authenticatedRemoteRequest(s, desktop, http.MethodDelete, "/api/remote/presence", "test-controller", ""); response.Code != http.StatusOK {
		t.Fatalf("withdraw: %d", response.Code)
	}
	before := s.currentMotionEngine().Snapshot()
	response := authenticatedRemoteRequest(s, phone, http.MethodPost, "/api/remote/edge", "phone-tab", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("remote Edge: %d %s", response.Code, response.Body.String())
	}
	if session := remoteEdgeSession(t, response.Body.Bytes()); session.Edge == nil || !session.Edge.Active {
		t.Fatalf("Edge response session = %+v", session)
	}
	after := s.currentMotionEngine().Snapshot()
	if after.StreamID != before.StreamID || after.Target.Source != "edge" || after.Target.SpeedPercent >= before.Target.SpeedPercent {
		t.Fatal("remote Edge did not hold the existing stream")
	}
	if response := authenticatedRemoteRequest(s, phone, http.MethodPost, "/api/remote/edge/resume", "phone-tab", `{}`); response.Code != http.StatusOK {
		t.Fatalf("remote Return: %d %s", response.Code, response.Body.String())
	}
	if s.currentMotionEngine().Snapshot().Target.SpeedPercent != 35 {
		t.Fatal("remote Return lost the saved target")
	}
}

func TestRemoteEdgeIsFencedByStopAndAccount(t *testing.T) {
	s, store, admin, desktop, phone := newRemoteFixture(t)
	startRemoteFixtureMotion(t, s, desktop)
	stale := func(r *http.Request) {
		r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load()+1, 10))
	}
	if response := authenticatedRemoteRequest(s, phone, http.MethodPost, "/api/remote/edge", "phone-tab", `{}`, stale); response.Code != http.StatusConflict {
		t.Fatalf("Edge with a stale Stop sequence: %d", response.Code)
	}
	operator := newAdmissionIdentity(t, store, admin.ID, "operator", true)
	response := authenticatedRemoteRequest(s, operator.cookie, http.MethodPost, "/api/remote/edge", "operator-tab", `{}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), errRemoteNoSession.Error()) {
		t.Fatalf("another account held the session: %d %s", response.Code, response.Body.String())
	}
	if s.currentMotionEngine().Snapshot().Target.Source == "edge" {
		t.Fatal("a refused Edge changed motion")
	}
	if response := authenticatedRemoteRequest(s, desktop, http.MethodPost, "/api/motion/stop", "test-controller", `{}`); response.Code != http.StatusOK {
		t.Fatalf("stop: %d", response.Code)
	}
	if response := authenticatedRemoteRequest(s, phone, http.MethodPost, "/api/remote/edge/resume", "phone-tab", `{}`); response.Code != http.StatusConflict {
		t.Fatalf("Return after Stop: %d", response.Code)
	}
	if s.currentMotionEngine().Snapshot().Running {
		t.Fatal("Return after Stop started motion")
	}
}

// A remote-only login reaches Edge on the remote listener, and the press is
// audited under the phone's own login.
func TestRemoteListenerOffersEdgeAndAuditsIt(t *testing.T) {
	s, store, admin, desktop, _ := newRemoteFixture(t)
	startRemoteFixtureMotion(t, s, desktop)
	account, cookie := remoteOnlyLogin(t, s, store, admin, desktop)
	if response := remotePortalRequest(s, cookie, http.MethodPost, "/api/remote/keepalive", `{}`); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"renewed":true`) {
		t.Fatalf("remote keepalive: %d %s", response.Code, response.Body.String())
	}
	if response := remotePortalRequest(s, cookie, http.MethodPost, "/api/remote/edge", `{}`); response.Code != http.StatusOK {
		t.Fatalf("remote Edge: %d %s", response.Code, response.Body.String())
	}
	s.accessAudit.Close()
	w := authenticatedControlRequest(s, desktop, http.MethodGet, "/api/audit/export", "audit-observer", "")
	var page audit.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("audit export %d: %v", w.Code, err)
	}
	for _, event := range page.Events {
		if event.Kind == audit.CommandFinished && event.Operation == "motion" && event.Outcome == "success" && event.Actor.AccountID == account.ID {
			return
		}
	}
	t.Fatal("the remote Edge press was not audited under the phone's login")
}
