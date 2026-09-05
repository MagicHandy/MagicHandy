package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestStateDisclosesMotionSimulation(t *testing.T) {
	for _, simulated := range []bool{false, true} {
		name := "selected device"
		if simulated {
			name = "simulator"
		}
		t.Run(name, func(t *testing.T) {
			// The general diagnostics fallback is fake even in production;
			// the public flag must describe the actual motion runtime.
			fake := transport.NewFake()
			runtime := Runtime{Transport: fake, MotionSimulated: simulated}
			if simulated {
				runtime.MotionTransport = fake
			}
			server := newTestServerWithRuntime(t, runtime)
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
			var body struct {
				Simulated *bool `json:"motion_simulated"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || body.Simulated == nil || *body.Simulated != simulated {
				t.Fatalf("simulation disclosure: status=%d, value=%v, want %v", recorder.Code, body.Simulated, simulated)
			}
		})
	}
}
