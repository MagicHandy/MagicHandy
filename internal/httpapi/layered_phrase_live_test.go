//go:build liveeval && magichandy_labs

package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// Real hosted generations through production HTTP orchestration, with one fake
// dispatch owner. Uses the existing credential store without copying secrets.
func TestHostedLayeredPhraseLiveHTTP(t *testing.T) {
	directory, model := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR"), os.Getenv("MAGICHANDY_LIVE_MODEL")
	if directory == "" || model == "" {
		t.Skip("set hosted review profile and model")
	}
	auth, err := openaiauth.Open(openaiauth.Options{DataDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	token, err := auth.ActiveTokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	native, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{Model: model, PlanUsage: true, ReasoningEffort: "none", ContextSize: 131072, Token: token, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	provider := &libraryLiveProvider{Provider: native}
	fake, trace := transport.NewFake(), diagnostics.NewTraceRing(4096)
	s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider, Transport: fake, MotionTransport: fake, Traces: trace})
	t.Cleanup(s.Close)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		connection := config.ModelConnection{ID: "phrase-review", Name: "Phrase review", Provider: config.LLMProviderChatGPT, Model: model, ReasoningEffort: "none", ContextWindow: 131072}.Normalize()
		settings.LLM.Connections, settings.LLM.ConversationConnectionID = []config.ModelConnection{connection}, connection.ID
		settings.LLM.Provider, settings.LLM.Model = config.LLMProviderOllama, model
		settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "conversation", ContextPolicy: config.ContextConversation}
		settings.LLM.MotionGenerationMode, settings.LLM.ChatVoice = config.LLMMotionModeLayered, "utility"
		settings.LLM.ReasoningMode, settings.LLM.MaxOutputTokens = "off", 1024
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 1, 83
		return settings
	})
	rows := []map[string]any{}
	var textProbe, stop string
	defer func() {
		if engine := s.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
			_, _ = engine.Stop(t.Context(), "review complete")
		}
		exportLabExperimentCapture(t, map[string]any{"model": model, "transport": "fake; no hardware", "scenario": "Counted Layered phrase, wider coverage, pace follow-up and release through production HTTP", "text_probe": textProbe, "stop": stop, "turns": rows, "commands": fake.Commands(), "trace_rows": trace.Rows()})
	}()
	textProbe = postChatStream(t, s, `{"message":"Briefly greet the reviewer. Leave motion stopped."}`)
	if !strings.Contains(textProbe, "event: done") || strings.Contains(textProbe, "event: motion") || len(fake.Commands()) != 0 {
		t.Fatal("text-only probe failed or issued motion")
	}
	var previous *motion.FlowSpec
	for i, request := range []string{
		"Start a repeating pattern of two short tip strokes, then one full stroke all the way to the base and back to the tip.",
		"Keep the two-to-one rhythm and full stroke. Give the two tip strokes more coverage, keeping the pace.",
		"Increase only the speed; keep exactly this pattern and reach.",
		"Let go of the counted pattern now. Use uncounted wandering reach and location instead, keeping the pace.",
	} {
		calls := provider.calls
		body, _ := json.Marshal(map[string]string{"message": request})
		response := postChatStream(t, s, string(body))
		row := map[string]any{"request": request, "response": response, "provider_calls": provider.calls - calls, "intent_pass": false}
		rows = append(rows, row)
		if !strings.Contains(response, "event: motion") || strings.Contains(response, `"repaired":true`) || strings.Contains(response, `"semantic_fallback":true`) || provider.calls-calls != 1 {
			t.Fatalf("production turn failed: %s", response)
		}
		state := s.currentMotionEngine().Snapshot()
		settings, _ := s.store.Snapshot()
		row["motion"] = motion.ReviewMotionOutput(state.Target, settings.Motion)
		if state.Target.Flow == nil || !state.Running {
			t.Fatal("missing active shared-engine score")
		}
		score := state.Target.Flow
		if i < 3 {
			p := score.Phrase
			if p == nil || len(p.Strokes) != 2 || p.Strokes[0].Count != 2 || p.Strokes[1].Count != 1 || p.Strokes[1].BottomPercent != 0 || p.Strokes[1].TopPercent != 100 {
				t.Fatal("lost counted full-range rhythm")
			}
			if i == 1 && (p.Strokes[0].BottomPercent >= previous.Phrase.Strokes[0].BottomPercent || score.SpeedPercent != previous.SpeedPercent) {
				t.Fatal("wider-coverage edit missed intent")
			}
			if i == 2 && (!reflect.DeepEqual(p, previous.Phrase) || score.SpeedPercent <= previous.SpeedPercent) {
				t.Fatal("pace edit changed phrase")
			}
		} else if score.Phrase != nil || score.SpeedPercent != previous.SpeedPercent {
			t.Fatal("new request did not release the counted pattern")
		}
		previous = motion.CloneFlowSpec(score)
		row["intent_pass"] = true
		time.Sleep(1200 * time.Millisecond)
	}
	stop = postChatStream(t, s, `{"message":"Stop motion now."}`)
	if s.currentMotionEngine().Snapshot().Running || !strings.Contains(stop, `"action":"stop"`) {
		t.Fatal("Stop failed")
	}
	plays := 0
	for _, command := range fake.Commands() {
		if command.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 {
		t.Fatalf("retarget restarted playback %d times", plays)
	}
}
