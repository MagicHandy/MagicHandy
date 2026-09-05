package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLabMotionExperimentsCaptureAndSharedAudition(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(512)
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, Traces: traces})
	state := server.labState()
	base := state.Current
	base.AnchorPercent = 100
	base.Layers = []motion.FlowLayer{{Axis: "pace", AmountPercent: 15, PeriodCycles: 12}}
	target := observationRequest{Source: "motion", Method: "flow", Spec: &base, SettingsKey: state.SettingsKey}
	view := createTestSequence(t, server, labTestCreateRequest{Preset: "motion_experiments", Target: &target})
	wanted := motion.FlowExperiments(base)
	if len(view.Run.Steps) != len(wanted) || len(fake.Commands()) != 0 || view.NextIndex != 0 {
		t.Fatal("comparison did not capture five inert rounds")
	}
	for index, step := range view.Run.Steps {
		if step.Preview == nil || !reflect.DeepEqual(step.Preview.Spec, wanted[index].Spec) || len(step.Preview.Candidates) != 1 || step.Preview.Candidates[0].Method != "flow" {
			t.Fatalf("round %d changed the captured score or generator", index)
		}
		response := labObservationRequest(t, server, http.MethodPost, "/api/motion/start", motionRequest{Lab: &motion.LabStart{Method: "flow", Flow: &step.Preview.Spec, SettingsKey: step.Preview.SettingsKey}}, true)
		if response.Code != http.StatusOK || !server.currentMotionEngine().Snapshot().Running {
			t.Fatalf("round %d shared audition: %s", index, response.Body)
		}
		response = labObservationRequest(t, server, http.MethodPost, "/api/motion/stop", nil, false)
		if response.Code != http.StatusOK || server.currentMotionEngine().Snapshot().Running {
			t.Fatal("Stop failed for a non-controller client")
		}
		count := len(fake.Commands())
		time.Sleep(30 * time.Millisecond)
		if len(fake.Commands()) != count {
			t.Fatal("motion command appeared after Stop returned")
		}
	}
	adds, plays := 0, 0
	for _, command := range fake.Commands() {
		if command.Kind == transport.CommandKindPointsAdd {
			adds++
		}
		if command.Kind == transport.CommandKindPointsPlay {
			plays++
		}
	}
	if adds < 5 || plays != 5 {
		t.Fatalf("missing shared sampler dispatch: %d adds, %d plays", adds, plays)
	}
	exportLabExperimentCapture(t, map[string]any{"scenario": "Five separate guided flow auditions, each followed by Stop", "speed": "25", "transport": "captured fake transport; no physical device", "commands": fake.Commands(), "trace_rows": traces.Rows(), "run": view.Run})
}

func exportLabExperimentCapture(t *testing.T, capture map[string]any) {
	t.Helper()
	path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE")
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(capture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- Explicit opt-in local test artifact path, never an HTTP request or runtime setting.
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
