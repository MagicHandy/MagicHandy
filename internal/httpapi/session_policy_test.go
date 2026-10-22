package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func TestSessionPolicyRoutesRequireAdministratorAndPersistExplicitChoices(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	read := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/auth/session-policy", "observer", "")
	var policy accounts.SessionPolicy
	if err := json.Unmarshal(read.Body.Bytes(), &policy); err != nil || read.Code != http.StatusOK || policy.IdleMinutes != 480 {
		t.Fatalf("default policy: %d %s", read.Code, read.Body.String())
	}
	for _, body := range []string{`{}`, `{"idle_minutes":null}`, `{"idle_minutes":-1}`, `{"idle_minutes":1}`} {
		response := authenticatedControlRequest(s, cookie, http.MethodPut, "/api/auth/session-policy", "observer", body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("accepted %s: %d", body, response.Code)
		}
	}
	updated := authenticatedControlRequest(s, cookie, http.MethodPut, "/api/auth/session-policy", "observer", `{"idle_minutes":0}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("save: %d %s", updated.Code, updated.Body.String())
	}
	session, err := store.InspectSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	setSessionIdleFixture(t, s, session.Key, time.Now().Add(-9*time.Hour))
	if _, err := store.InspectSession(t.Context(), cookie.Value); err != nil {
		t.Fatal("idle off did not apply")
	}
	operator, err := store.Create(t.Context(), "observer", "synthetic observer passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := authenticatedControlRequest(s, testSessionCookie(token), method, "/api/auth/session-policy", "observer", `{"idle_minutes":30}`)
		if response.Code != http.StatusForbidden {
			t.Fatalf("operator policy access: %d", response.Code)
		}
	}
}

func TestIdleSignOutOffDoesNotDisableControllerDeadman(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	response := authenticatedControlRequest(s, cookie, http.MethodPut, "/api/auth/session-policy", "test-controller", `{"idle_minutes":0}`)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	s.controller.mu.Lock()
	s.controller.lastSeenAt = time.Now().Add(-controllerLeaseTTL - time.Second)
	s.controller.mu.Unlock()
	if !s.controller.BeginLoss("", true) {
		t.Fatal("idle policy disabled heartbeat expiry")
	}
	s.controller.FinishLoss()
	response = authenticatedControlRequest(s, cookie, http.MethodPost, "/api/controller/heartbeat", "test-controller", `{}`)
	var snapshot controllerSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil || snapshot.Active {
		t.Fatal("heartbeat revived expired controller")
	}
}
