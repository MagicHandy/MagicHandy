package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func lifecycleStartFixture(mode string) string {
	switch mode {
	case config.LLMMotionModeLayered:
		return `{"action":"start","edits":{"controls":{"speed_percent":{"op":"set","value":30}}},"reply":"Playing the new rhythm."}`
	case config.LLMMotionModeCreativeV2:
		return `{"action":"start","edits":[{"speed_percent":30}],"reply":"Playing the new rhythm."}`
	case config.LLMMotionModeDynamic:
		return `{"reply":"Playing the new rhythm.","motion":{"action":"start","speed_percent":30,"center_percent":50,"span_percent":80,"variation_percent":30,"segment_seconds":20}}`
	default:
		return `{"reply":"Playing the new rhythm.","motion":{"action":"start","speed_percent":30}}`
	}
}

func TestHostedFreshMotionAfterStopUsesSharedEngineWithoutAutopilot(t *testing.T) {
	for _, mode := range []string{config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2, config.LLMMotionModeDynamic, config.LLMMotionModePattern} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeHostedFixtureReply(w, lifecycleStartFixture(mode))
			})
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = mode
				return settings
			})
			for run := range 2 {
				body := postChatStream(t, s, `{"message":"Use syncopation."}`)
				engine := s.currentMotionEngine()
				if engine == nil || !engine.Snapshot().Running || !strings.Contains(body, `"applied":true`) || calls.Load() != int32(run+1) {
					t.Fatalf("fresh motion did not start through the normal path: %s", body)
				}
				if s.modes.Status().Mode == modes.ModeAutopilot {
					t.Fatal("a chat motion request started Autopilot")
				}
				postChatStream(t, s, `{"message":"Stop motion now."}`)
				if engine.Snapshot().Running {
					t.Fatal("Stop failed")
				}
			}
			plays := 0
			for _, command := range fake.Commands() {
				if command.Kind == transport.CommandKindPointsPlay {
					plays++
				}
			}
			if plays != 2 || calls.Load() != 2 {
				t.Fatalf("unexpected start or model call: plays=%d calls=%d", plays, calls.Load())
			}
		})
	}
}

func TestHostedStartCannotCrossPausedOffOrScriptOwnership(t *testing.T) {
	for _, tc := range []struct{ name, mode, owner string }{
		{"paused layered", config.LLMMotionModeLayered, ""},
		{"paused creative v2", config.LLMMotionModeCreativeV2, ""},
		{"paused dynamic", config.LLMMotionModeDynamic, ""},
		{"paused pattern", config.LLMMotionModePattern, ""},
		{"LLM Off", config.LLMMotionModeOff, ""},
		{"script owner", config.LLMMotionModePattern, "script"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeHostedFixtureReply(w, lifecycleStartFixture(tc.mode))
			})
			configureResponseRepair(t, s, false)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = tc.mode
				return settings
			})
			if strings.HasPrefix(tc.name, "paused") {
				callMotion(t, s, http.MethodPost, "/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":30}`)
				callMotion(t, s, http.MethodPost, "/api/motion/pause", `{}`)
			}
			before := len(fake.Commands())
			body := postChatStream(t, s, fmt.Sprintf(`{"message":"Use syncopation.","motion_owner":%q}`, tc.owner))
			if strings.Contains(body, "event: motion") || len(fake.Commands()) != before || calls.Load() != 1 {
				t.Fatalf("forbidden start had effects: calls=%d %s", calls.Load(), body)
			}
			if engine := s.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
				t.Fatal("model started motion without lifecycle authority")
			}
		})
	}
}

func TestFreshHostedMotionDoesNotRevivePreStopRepair(t *testing.T) {
	var calls atomic.Int32
	s, _, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeHostedFixtureReply(w, `{"action":"start","edits":{"unknown":true},"reply":"Rejected."}`)
			return
		}
		writeHostedFixtureReply(w, lifecycleStartFixture(config.LLMMotionModeLayered))
	})
	configureResponseRepair(t, s, false)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = config.LLMMotionModeLayered
		return settings
	})
	session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Use a flowing rhythm."}`))
	callMotion(t, s, http.MethodPost, "/api/motion/stop", `{}`)
	body := postChatStream(t, s, `{"message":"Use syncopation."}`)
	if !strings.Contains(body, `"applied":true`) {
		t.Fatalf("fresh start was blocked by the old rejected request: %s", body)
	}
	before := s.currentMotionEngine().Snapshot()
	if response := postRepair(s, session, id); response.Code != http.StatusConflict || calls.Load() != 2 {
		t.Fatalf("pre-Stop repair revived: %d %s", response.Code, response.Body.String())
	}
	after := s.currentMotionEngine().Snapshot()
	if !after.Running || before.PlanID != after.PlanID || before.Generation != after.Generation {
		t.Fatal("old repair changed the fresh motion")
	}
	callMotion(t, s, http.MethodPost, "/api/motion/stop", `{}`)
}

func TestLocalPausedChatUsesOneInertResponseThroughApp(t *testing.T) {
	for _, mode := range []string{config.LLMMotionModeDynamic, config.LLMMotionModePattern} {
		t.Run(mode, func(t *testing.T) {
			fake := transport.NewFake()
			provider := &scriptedLLMProvider{responses: []string{`{"reply":"Resume using the app before changing the rhythm."}`}}
			s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
			t.Cleanup(s.Close)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.MotionGenerationMode = mode
				return settings
			})
			callMotion(t, s, http.MethodPost, "/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":30}`)
			callMotion(t, s, http.MethodPost, "/api/motion/pause", `{}`)
			before := len(fake.Commands())
			body := postChatStream(t, s, `{"message":"Use syncopation."}`)
			if !strings.Contains(body, "event: message") || strings.Contains(body, "event: malformed") || strings.Contains(body, "event: motion") || provider.callCount() != 1 || len(fake.Commands()) != before {
				t.Fatalf("paused local chat retried or applied motion: %s", body)
			}
			if state := s.currentMotionEngine().Snapshot(); state.Running || !state.Paused {
				t.Fatal("inert local response changed the paused lifecycle")
			}
			provider.mu.Lock()
			schema := string(provider.requests[0].JSONSchema)
			provider.mu.Unlock()
			if schema == "" || strings.Contains(schema, `"start"`) || strings.Contains(schema, `"target"`) || strings.Contains(schema, `"update"`) {
				t.Fatalf("paused local request advertised active motion: %s", schema)
			}
		})
	}
}
