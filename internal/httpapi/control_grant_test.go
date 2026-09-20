package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

func TestControlGrantRequiresExplicitPermanentChoice(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	operator, err := store.Create(t.Context(), "observer", "synthetic operator passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	claimAuthenticatedController(t, s, cookie)
	route := "/api/accounts/" + operator.ID + "/control-grant"
	for _, body := range []string{`{}`, `{"duration_minutes":0}`, `{"duration_minutes":-1}`, `{"duration_minutes":721}`, `{"permanent":false}`, `{"permanent":true,"duration_minutes":60}`} {
		response := authenticatedControlRequest(s, cookie, http.MethodPut, route, "test-controller", body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid duration %s accepted: %d %s", body, response.Code, response.Body.String())
		}
	}
	response := authenticatedControlRequest(s, cookie, http.MethodPut, route, "test-controller", `{"permanent":true}`)
	var result struct{ Grant *accounts.ControlGrant }
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK || result.Grant == nil || result.Grant.ExpiresAt != nil {
		t.Fatalf("permanent API response: %d %s %v", response.Code, response.Body.String(), err)
	}
	response = authenticatedControlRequest(s, cookie, http.MethodGet, route, "test-controller", "")
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Grant == nil || result.Grant.ExpiresAt != nil {
		t.Fatalf("permanent API read: %s %v", response.Body.String(), err)
	}
}

func TestPermanentControlStillLosesControllerAuthority(t *testing.T) {
	for _, reason := range []string{"revoked", "replaced", "disabled", "signed-out", "heartbeat-expired"} {
		t.Run(reason, func(t *testing.T) { assertPermanentControlLoss(t, reason) })
	}
}

func assertPermanentControlLoss(t *testing.T, reason string) {
	t.Helper()
	s, store, admin, _ := newControllerSessionFixture(t)
	operator, err := store.Create(t.Context(), "observer", "synthetic observer passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GrantPermanentControl(t.Context(), admin.ID, operator.ID); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.NewSession(t.Context(), operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := testSessionCookie(token)
	initial := claimAuthenticatedController(t, s, cookie)
	if owner := s.controller.Owner(); owner.grantID == "" || !owner.grantExpires.IsZero() {
		t.Fatalf("permanent controller binding: %+v", owner)
	}
	switch reason {
	case "revoked":
		err = store.RevokeControl(t.Context(), admin.ID, operator.ID)
	case "replaced":
		_, err = store.GrantControl(t.Context(), admin.ID, operator.ID, time.Hour)
	case "disabled":
		err = store.SetDisabled(t.Context(), operator.ID, true)
	case "signed-out":
		err = store.RevokeSession(t.Context(), token)
	case "heartbeat-expired":
		s.controller.mu.Lock()
		s.controller.lastSeenAt = time.Now().Add(-controllerLeaseTTL - time.Second)
		s.controller.mu.Unlock()
	}
	if err != nil {
		t.Fatal(err)
	}
	s.checkAccessLifetimes()
	deadline := time.Now().Add(3 * time.Second)
	for s.stopSequence.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.stopSequence.Load() == 0 {
		t.Fatal("permanent permission bypassed Stop on control loss")
	}
	s.controller.mu.Lock()
	newGeneration := s.controller.generation
	s.controller.mu.Unlock()
	if newGeneration <= initial.Generation {
		t.Fatal("permanent controller's old generation survived")
	}
}
