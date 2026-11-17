package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type retiredStopOwner struct{ *transport.Fake }

func (*retiredStopOwner) Stop(context.Context, transport.StopCommand) (transport.CommandResult, error) {
	return transport.CommandResult{OK: true, Status: "connection_retired"}, nil
}

func TestLoopbackStopRequiresPhysicalConfirmation(t *testing.T) {
	owner := &retiredStopOwner{transport.NewFake()}
	server := newTestServerWithRuntime(t, Runtime{Transport: owner, MotionTransport: owner})
	t.Cleanup(server.Close)
	callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":30}`)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), `"transport_stop_confirmed":false`) || server.currentMotionEngine().Snapshot().Running {
		t.Fatalf("retired owner Stop acknowledgement = %d %s", w.Code, w.Body.String())
	}
}

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

func TestStopOutcomeMatrixAcrossClientSurfaces(t *testing.T) {
	local := newTestServer(t)
	protected, _, _, _ := newControllerSessionFixture(t)
	for _, tc := range []struct {
		name    string
		outcome emergencyStopResult
		err     error
		status  int
	}{
		{"confirmed", emergencyStopResult{transportAvailable: true, transportResult: transport.CommandResult{Kind: transport.CommandKindStop, OK: true}}, nil, 200},
		{"unconfirmed", emergencyStopResult{transportAvailable: true, transportResult: transport.CommandResult{OK: true, Status: "connection_retired"}}, nil, 502},
		{"pending", emergencyStopResult{pending: true}, errStopConfirmationPending, 503},
		{"no_device", emergencyStopResult{transportResult: transport.CommandResult{OK: true, Status: "no_devices"}}, nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			console, _ := local.consoleStopFeedback(tc.outcome, tc.err)
			if console.Status != tc.name || console.Confirmed != (tc.name == "confirmed") {
				t.Fatalf("console: %+v", console)
			}
			for _, s := range []*Server{local, protected} {
				w := httptest.NewRecorder()
				s.writeEmergencyStopReply(w, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil), tc.outcome, tc.err)
				var body map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if w.Code != tc.status || body["stop_status"] != tc.name || body["transport_stop_confirmed"] != console.Confirmed {
					t.Fatalf("HTTP %d, %+v", w.Code, body)
				}
				if s == protected && (body["engine"] != nil || body["transport_result"] != nil) {
					t.Fatal("protected Stop included host diagnostics")
				}
			}
		})
	}
}
