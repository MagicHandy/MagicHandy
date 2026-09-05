package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func startConversationTest(t *testing.T, server *Server, live, autopilot bool) llmLabState {
	t.Helper()
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/session", map[string]any{
		"method": "edits", "live": live, "autopilot": autopilot, "schema_guided": true, "interval_seconds": 5,
	}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("start %d: %s", response.Code, response.Body)
	}
	var state llmLabState
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func dueLabAutopilot(server *Server) {
	server.lab.mu.Lock()
	server.lab.lastActivity = time.Now().Add(-time.Minute)
	server.lab.mu.Unlock()
}

func awaitLabTrial(t *testing.T, server *Server, count int) llmLabState {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state := server.labState()
		if len(state.Turns) >= count && !state.Busy && (state.Turns[len(state.Turns)-1].Valid || !state.Session.Autopilot) {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("lab trial did not finish")
	return llmLabState{}
}

func TestLabConversationRetargetsOneSharedRun(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(512)
	provider := &scriptedLLMProvider{responses: []string{`{"reply":"Shorter reach, same pace.","controls":{"range_floor_percent":15}}`}}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider, Traces: traces})
	t.Cleanup(server.Close)
	state := startConversationTest(t, server, true, false)
	engine := server.currentMotionEngine()
	if engine == nil || !engine.Snapshot().Running {
		t.Fatal("live session did not start shared engine")
	}
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/chat", map[string]any{
		"message": "Shorten the minimum reach", "revision": state.Revision, "method": "controls",
	}, true)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	state = server.labState()
	if len(state.Turns) != 1 || !state.Turns[0].MotionApplied || state.Turns[0].Method != "edits" || state.Current.RangeFloorPercent != 15 {
		t.Fatalf("live reply: %+v", state)
	}
	plays := 0
	for _, command := range fake.Commands() {
		if command.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 || server.currentMotionEngine() != engine {
		t.Fatal("reply restarted or bypassed shared motion")
	}
	setLabsForTest(t, server, false)
	if engine.Snapshot().Running || server.labState().Session.Active {
		t.Fatal("disabling Labs left a live session")
	}
	exportLabExperimentCapture(t, map[string]any{"scenario": "Live Lab start, accepted conversational retarget, disable Labs and Stop", "transport": "captured fake transport; no physical device", "speed": "25", "commands": fake.Commands(), "trace_rows": traces.Rows(), "turns": state.Turns})
}

func TestLabAutopilotPreviewPreservesFailuresAndStops(t *testing.T) {
	for _, raw := range []string{`{"reply":"Keep the same motion."}`, `{"reply":"Faster.","change_by":{"speed_percent":1}}`, `not JSON`} {
		t.Run(raw, func(t *testing.T) {
			fake := transport.NewFake()
			provider := &scriptedLLMProvider{responses: []string{raw}}
			server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
			t.Cleanup(server.Close)
			startConversationTest(t, server, false, true)
			dueLabAutopilot(server)
			state := awaitLabTrial(t, server, 1)
			trial := state.Turns[0]
			if !trial.Autopilot || trial.Raw != raw || trial.ProviderCalls != 1 || len(fake.Commands()) != 0 || server.currentMotionEngine() != nil {
				t.Fatal("autopilot repaired output or started a device in preview")
			}
			if raw != `{"reply":"Keep the same motion."}` && (trial.Valid || state.Session.Autopilot || state.Current.SpeedPercent != 25) {
				t.Fatal("failed/increasing proposal was accepted or automatically retried")
			}
			if _, err := server.emergencyStop(t.Context(), "lab_test_stop"); err != nil {
				t.Fatal(err)
			}
			if server.labState().Session.Active {
				t.Fatal("Stop left Autopilot active")
			}
		})
	}
}

type labInterruptProvider struct {
	calls   atomic.Int32
	started chan struct{}
}

func (p *labInterruptProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Available: true}
}
func (p *labInterruptProvider) StreamChat(ctx context.Context, _ llm.ChatRequest, _ func(string) error) (string, error) {
	if p.calls.Add(1) == 1 {
		close(p.started)
		<-ctx.Done()
		return `{"reply":"Late.","controls":{"anchor_percent":100}}`, nil
	}
	return `{"reply":"Your message takes priority.","controls":{"anchor_percent":50}}`, nil
}

func TestLabMessageInterruptsAutopilotAndDiscardsLateOutput(t *testing.T) {
	provider := &labInterruptProvider{started: make(chan struct{})}
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(server.Close)
	startConversationTest(t, server, false, true)
	dueLabAutopilot(server)
	select {
	case <-provider.started:
	case <-time.After(3 * time.Second):
		t.Fatal("autopilot did not start")
	}
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/chat", map[string]any{"message": "Center the anchor", "revision": 0}, true)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	state := server.labState()
	if len(state.Turns) != 1 || state.Turns[0].Autopilot || state.Current.AnchorPercent != 50 || provider.calls.Load() != 2 {
		t.Fatal("manual interruption retained or applied the canceled reply")
	}
	if len(fake.Commands()) != 0 {
		t.Fatal("preview dispatched device commands")
	}
}

func TestLabAutopilotStopCancelsLivePendingReply(t *testing.T) {
	provider := &labInterruptProvider{started: make(chan struct{})}
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(server.Close)
	startConversationTest(t, server, true, true)
	engine := server.currentMotionEngine()
	dueLabAutopilot(server)
	select {
	case <-provider.started:
	case <-time.After(3 * time.Second):
		t.Fatal("autopilot did not start")
	}
	if _, err := server.emergencyStop(t.Context(), "lab_stop"); err != nil {
		t.Fatal(err)
	}
	if engine.Snapshot().Running || server.labState().Session.Active || len(server.labState().Turns) != 0 {
		t.Fatal("late autopilot motion survived Stop")
	}
}

func TestLegacyManualMotionCannotStart(t *testing.T) {
	_, err := (motionRequest{Pattern: string(motion.PatternStroke), SpeedPercent: 25}).target(config.DefaultSettings().Motion)
	if err == nil {
		t.Fatal("legacy manual start accepted")
	}
}
