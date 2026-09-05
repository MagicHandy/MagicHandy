package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLayeredAutopilotJitterHonorsMinimumQuietTime(t *testing.T) {
	options := labConversationSession{Method: "layered", IntervalSeconds: 20}
	for range 100 {
		delay := labAutopilotDelay(options)
		if delay < 20*time.Second || delay >= 30*time.Second {
			t.Fatal("continuation violated quiet-time bounds")
		}
	}
	options.Method = "edits"
	if labAutopilotDelay(options) != 20*time.Second {
		t.Fatal("historical comparison timing changed")
	}
}

func TestLayeredChatStartEvolveModeChangeAndStopUseOneEngine(t *testing.T) {
	fake := transport.NewFake()
	traces := diagnostics.NewTraceRing(2048)
	provider := &scriptedLLMProvider{responses: []string{
		`{"edits":{"stroke_width":{"min_percent":20,"max_percent":90},"controls":{"anchor_percent":100},"layers":[{"axis":"range","amount_percent":100,"period_cycles":8,"shape":"alternate"}],"remove_layers":["center"]},"reply":"Starting broad and tip strokes."}`,
		`{"edits":{"evolve":true},"reply":"Fresh variation."}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider, Traces: traces})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.MotionGenerationMode = config.LLMMotionModeLayered
		return s
	})
	started := postChatStream(t, server, `{"message":"Start moving, alternating broad and short tip strokes."}`)
	if !strings.Contains(started, "event: motion") {
		t.Fatalf("missing start: %s", started)
	}
	engine := server.currentMotionEngine()
	before := engine.Snapshot()
	if before.Target.Flow == nil || before.Target.Dynamic != nil || before.Target.Pattern != nil {
		t.Fatal("wrong motion path")
	}
	oldSeed := before.Target.Flow.Seed
	before.Target.Flow.Layers[0].AmountPercent = 0
	if engine.Snapshot().Target.Flow.Layers[0].AmountPercent == 0 {
		t.Fatal("snapshot aliased active score")
	}
	updated := postChatStream(t, server, `{"message":"Keep varying the motion."}`)
	if !strings.Contains(updated, `"action":"update"`) || engine != server.currentMotionEngine() || engine.Snapshot().Target.Flow.Seed == oldSeed {
		t.Fatalf("failed evolution: %s", updated)
	}
	plays := 0
	for _, c := range fake.Commands() {
		if c.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 {
		t.Fatal("evolution restarted transport")
	}
	command := &chat.MotionCommand{Action: chat.MotionActionUpdate, Layered: motion.CloneFlowSpec(engine.Snapshot().Target.Flow)}
	for _, mode := range []string{config.LLMMotionModeOff, config.LLMMotionModeDynamic, config.LLMMotionModePattern} {
		saveSettings(t, server.store, func(s config.Settings) config.Settings { s.LLM.MotionGenerationMode = mode; return s })
		if _, err := server.dispatchChatMotion(t.Context(), command); err == nil {
			t.Fatal("late Layered response crossed mode change")
		}
	}
	if _, err := server.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionStop}); err != nil || engine.Snapshot().Running {
		t.Fatalf("Stop failed: %v", err)
	}
	exportLabExperimentCapture(t, map[string]any{"scenario": "Production Layered start, evolve on the same stream, reject late mode changes, Stop", "transport": "captured fake transport; no physical device", "commands": fake.Commands(), "trace_rows": traces.Rows()})
}

func TestLayeredAutopilotMapsFlowAndPreservesHumanRequests(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{`{"edits":{"evolve":true},"reply":"Fresh details."}`}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.MotionGenerationMode = config.LLMMotionModeLayered
		return s
	})
	_, err := server.chatLog.Append(chat.MessageRoleUser, "Alternate full strokes and short tip strokes, with ongoing variation.", "test")
	if err != nil {
		t.Fatal(err)
	}
	for range 24 {
		if _, err := server.chatLog.Append(chat.MessageRoleAssistant, "Still here.", ""); err != nil {
			t.Fatal(err)
		}
	}
	score := chat.DefaultLayeredScore(25)
	decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: &score, CurrentSpeed: 25, SpeedMinPercent: 1, SpeedMaxPercent: 100})
	if err != nil || decision.Hold || decision.Segment.Flow == nil || decision.Segment.Flow.Seed == score.Seed || decision.Segment.PatternID != "" || decision.Segment.Dynamic != nil || decision.Variability != modes.VariabilitySettled {
		t.Fatalf("decision %+v err=%v", decision, err)
	}
	if provider.callCount() != 1 {
		t.Fatal("unexpected fallback/repair")
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if !strings.Contains(provider.requests[0].Messages[0].Content, "Alternate full strokes and short tip strokes") {
		t.Fatal("automatic replies displaced human intent")
	}
}

func TestLayeredLabHumanHistorySurvivesAutomaticDisplayRetention(t *testing.T) {
	server := newEnabledLabServer(t, Runtime{})
	t.Cleanup(server.Close)
	state := server.labState()
	prompt := state.Prompts["layered"]
	for i := 0; i < 26; i++ {
		automatic := i > 0
		trial := chat.LLMLabTrial{Autopilot: automatic, Method: "layered", Prompt: prompt, Message: "Keep tip anchored and keep varying.", Raw: `{"edits":{},"reply":"Holding."}`, Valid: true, Before: state.Current, After: state.Current}
		var err error
		state, err = server.recordLabTrial(t.Context(), 0, trial, state)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(state.Turns) != 20 || len(state.DirectiveTurns) != 1 {
		t.Fatal("incorrect bounded retention")
	}
	history := labConversationHistory(state.DirectiveTurns, labChatRequest{Method: "layered", Prompt: prompt})
	if len(history) != 2 || !strings.Contains(history[0].Content, "tip anchored") {
		t.Fatal("human request fell out of history")
	}
	response := labObservationRequest(t, server, http.MethodPost, "/api/labs/llm/reset", map[string]any{"method": "layered"}, true)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body)
	}
	var reset llmLabState
	if err := json.Unmarshal(response.Body.Bytes(), &reset); err != nil {
		t.Fatal(err)
	}
	expected := chat.DefaultLayeredScore(25)
	expected.Seed = reset.Current.Seed
	if !reflect.DeepEqual(reset.Current, expected) || len(server.labState().DirectiveTurns) > 0 {
		t.Fatal("reset did not clear separate human history")
	}
}
