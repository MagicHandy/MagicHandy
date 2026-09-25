package httpapi

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// The model reads a standing wish from the conversation; the host remembers
// its answer. Planning turns alone let such a wish lapse after an unrelated
// remark, so while it stands Autopilot holds without asking the model at all.
func TestStandingHoldFollowsTheModelsReadingOfChat(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"action":"start","stay_unchanged":true,"edits":[{"speed_percent":30}],"reply":"Starting slowly."}`,
		`{"edits":[],"reply":"Staying with this for now."}`,
		`{"action":"none","stay_unchanged":true,"edits":[],"reply":"Keeping it exactly like this."}`,
		`{"action":"none","stay_unchanged":true,"edits":[],"reply":"Mm, I love hearing that."}`,
		`{"action":"none","stay_unchanged":false,"edits":[],"reply":"Then let me surprise you."}`,
		`{"edits":[{"evolve":true}],"reply":"Fresh variation."}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		return s
	})
	decide := func() modes.Decision {
		t.Helper()
		state := server.currentMotionEngine().Snapshot()
		decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: state.Target.Flow,
			CurrentSpeed: state.Target.SpeedPercent, SpeedMinPercent: 1, SpeedMaxPercent: 100})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}
	calls := func() int {
		provider.mu.Lock()
		defer provider.mu.Unlock()
		return len(provider.requests)
	}
	// Without Autopilot a wish has nothing to hold, so it is neither offered
	// nor remembered, even from a reply that ignores the grammar.
	postChatStream(t, server, `{"message":"Start slowly."}`)
	if offersStandingWish(t, provider, 0) {
		t.Fatal("chat without Autopilot was offered a standing wish")
	}
	startScheduledHoldAutopilot(t, server)
	if decision := decide(); decision.Requested {
		t.Fatal("a wish declared outside Autopilot was remembered")
	}

	postChatStream(t, server, `{"message":"Keep it exactly like this, no changes from now on."}`)
	if !offersStandingWish(t, provider, 2) {
		t.Fatal("live chat during Autopilot was not asked about a standing wish")
	}
	before := calls()
	for range 2 {
		if decision := decide(); !decision.Hold || !decision.Requested {
			t.Fatalf("standing wish did not hold Autopilot: %+v", decision)
		}
	}
	// An unrelated remark leaves the wish standing, as the model read it.
	postChatStream(t, server, `{"message":"That feels amazing."}`)
	if system := provider.requests[3].Messages[0].Content; !strings.Contains(system, "holding the motion unchanged at the user's request") {
		t.Fatal("chat was not told that Autopilot is holding")
	}
	if decision := decide(); !decision.Requested || calls() != before+1 {
		t.Fatalf("the hold lapsed or consulted the model: %+v calls=%d", decision, calls()-before)
	}
	postChatStream(t, server, `{"message":"Okay, now surprise me."}`)
	if decision := decide(); decision.Requested || decision.Hold || decision.Segment.Flow == nil {
		t.Fatalf("releasing the wish did not return planning to the model: %+v", decision)
	}
}

func TestStandingHoldIsScopedToItsSessionAndFreshRuns(t *testing.T) {
	var hold autopilotStandingHold
	hold.record("chat-a", true)
	if !hold.holds("chat-a") || hold.holds("chat-b") || hold.holds("") {
		t.Fatal("a standing wish leaked across conversations")
	}
	hold.record("chat-b", false)
	if !hold.holds("chat-a") {
		t.Fatal("another conversation released this one's wish")
	}
	hold.release()
	if hold.holds("chat-a") {
		t.Fatal("a fresh Autopilot run inherited an old wish")
	}
}

// startScheduledHoldAutopilot runs a real Autopilot mode whose own scheduled
// decisions hold, so a test can drive planning boundaries directly without
// racing the scheduler for scripted model replies.
func startScheduledHoldAutopilot(t *testing.T, server *Server) {
	t.Helper()
	manager, err := modes.NewManager(modes.Options{
		Ensure: func(context.Context) (modes.Engine, error) {
			engine, admission, err := server.motionEngineForStart()
			if err != nil {
				return nil, err
			}
			return admittedMotionEngine{Engine: engine, admission: admission}, nil
		},
		Current: func() modes.Engine {
			if engine := server.currentMotionEngine(); engine != nil {
				return engine
			}
			return nil
		},
		Settings: func() config.MotionSettings { s, _ := server.store.Snapshot(); return s.Motion },
		Traces:   server.traces,
		Tick:     5 * time.Millisecond,
		Seed:     7,
		Decide: func(context.Context, modes.DecisionInput) (modes.Decision, error) {
			return modes.Decision{Hold: true, Next: modes.TimingNormal}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server.modes = manager
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatal(err)
	}
}

func offersStandingWish(t *testing.T, provider *scriptedLLMProvider, request int) bool {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	var schema struct {
		OneOf []struct {
			Required []string `json:"required"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(provider.requests[request].JSONSchema, &schema); err != nil || len(schema.OneOf) == 0 {
		t.Fatalf("request %d has no continuous decision grammar: %v", request, err)
	}
	return slices.Contains(schema.OneOf[0].Required, "stay_unchanged")
}
