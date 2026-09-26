package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestGrantedOperatorCanAdjustSemanticLimitsButNotDeviceProfile(t *testing.T) {
	s, store, admin, _ := newControllerSessionFixture(t)
	operator := newAdmissionIdentity(t, store, admin.ID, "operator", true)
	claimAuthenticatedController(t, s, operator.cookie)
	before, _ := s.store.Snapshot()
	denied := authenticatedControlRequest(s, operator.cookie, http.MethodPost, "/api/motion/quick", "test-controller", `{"handy_model":"handy_2_pro","speed_min_percent":19}`)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("device profile update: %d %s", denied.Code, denied.Body.String())
	}
	after, _ := s.store.Snapshot()
	if after.Motion != before.Motion {
		t.Fatal("denied profile update partly changed motion settings")
	}
	accepted := authenticatedControlRequest(s, operator.cookie, http.MethodPost, "/api/motion/quick", "test-controller", `{"speed_min_percent":19,"speed_max_percent":29}`)
	if accepted.Code != http.StatusOK {
		t.Fatalf("semantic update: %d %s", accepted.Code, accepted.Body.String())
	}
	after, _ = s.store.Snapshot()
	if after.Motion.SpeedMinPercent != 19 || after.Motion.SpeedMaxPercent != 29 || after.Motion.HandyModel != before.Motion.HandyModel {
		t.Fatal("granted semantic update did not preserve the physical device profile")
	}
}

func TestGrantedChatSharesTextAndRecoveryWithoutHostDiagnostics(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{`{"reply":"Shared conversation reply.","motion":{"action":"none"}}`}}
	s, store, admin, adminCookie := newControllerSessionFixture(t, provider)
	if _, _, err := s.store.Update(func(settings config.Settings) (config.Settings, error) {
		settings.LLM.Model = "private-host-model-fixture"
		// The scripted reply uses the Creative contract.
		settings.LLM.MotionGenerationMode = config.LLMMotionModeDynamic
		return settings, nil
	}); err != nil {
		t.Fatal(err)
	}
	operator := newAdmissionIdentity(t, store, admin.ID, "operator", true)
	claimAuthenticatedController(t, s, operator.cookie)
	w := authenticatedControlRequest(s, operator.cookie, http.MethodPost, "/api/chat/stream", "test-controller", `{"message":"Hello without motion."}`, func(r *http.Request) {
		r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"reply":"Shared conversation reply."`) || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatalf("shared chat stream: %d %s", w.Code, w.Body.String())
	}
	assertNoHostChatDiagnostics(t, w.Body.String())
	if provider.callCount() != 1 {
		t.Fatal("valid operator reply required repair or fallback")
	}
	for _, identity := range []struct {
		cookie *http.Cookie
		host   bool
	}{{operator.cookie, false}, {adminCookie, true}} {
		page := authenticatedControlRequest(s, identity.cookie, http.MethodGet, "/api/chat/messages", "test-controller", "")
		body := page.Body.String()
		if page.Code != http.StatusOK || !strings.Contains(body, "Shared conversation reply.") || !strings.Contains(body, `"revision"`) {
			t.Fatalf("shared committed history: %d %s", page.Code, body)
		}
		if identity.host {
			if !strings.Contains(body, "private-host-model-fixture") {
				t.Fatal("administrator lost inference diagnostics")
			}
		} else {
			assertNoHostChatDiagnostics(t, body)
		}
	}
}

func assertNoHostChatDiagnostics(t *testing.T, body string) {
	t.Helper()
	for _, marker := range []string{"private-host-model-fixture", `"diagnostics"`, `"prompt_set"`, `"client_id"`} {
		if strings.Contains(body, marker) {
			t.Fatalf("shared chat disclosed %s", marker)
		}
	}
}
