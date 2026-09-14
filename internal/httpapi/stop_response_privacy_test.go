package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestProtectedStopDoesNotDisclosePrivateMotionSnapshot(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternHardAndRegular, Label: "private prior activity", SpeedPercent: 20}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || engine.Snapshot().Running {
		t.Fatal("anonymous Stop did not stop the simulated engine")
	}
	for _, private := range []string{"private prior activity", `"engine"`, `"target"`, `"settings"`} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("public Stop disclosed protected motion state")
		}
	}
}

func TestPublicStopFailureKeepsConfirmationHonestWithoutPrivateDetails(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	w := httptest.NewRecorder()
	s.writePublicStopResult(w, emergencyStopResult{
		engineAvailable: true,
		state: motion.ActiveMotionState{
			LastError:  "private transport failure",
			LastResult: &transport.CommandResult{Kind: transport.CommandKindStop, OK: false, Error: "private transport failure"},
		},
	}, errors.New("private transport failure"))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), `"transport_stop_confirmed":false`) || !strings.Contains(w.Body.String(), `"stopped":true`) {
		t.Fatalf("failed Stop acknowledgement: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private transport failure") || !strings.Contains(w.Body.String(), "Stop is unconfirmed") {
		t.Fatal("Stop failure did not preserve its public response contract")
	}
}

func TestPublicStopRejectsAnUnconfirmedResultEvenWithoutAnError(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	w := httptest.NewRecorder()
	s.writePublicStopResult(w, emergencyStopResult{transportAvailable: true, transportResult: transport.CommandResult{Kind: transport.CommandKindStop, OK: false}}, nil)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Stop is unconfirmed") {
		t.Fatal("an unconfirmed transport result became a successful UI Stop")
	}
}
