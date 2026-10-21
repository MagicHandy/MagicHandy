package httpapi

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const reviewPaceCorrection = `{"verdict":"correct","confidence":"high","reason":"wrong_pace","feedback":"The request retained slow motion.","correction":{"reply":"Restored pace.","edits":[{"speed_percent":18}]}}`

type reviewFixtureProvider struct {
	local   scriptedLLMProvider
	entered chan llm.ChatRequest
	release chan struct{}
	reviews atomic.Int32
	reply   string
}

func (p *reviewFixtureProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	if request.Model != "review-model" {
		return p.local.StreamChat(ctx, request, delta)
	}
	p.reviews.Add(1)
	p.entered <- request
	// Deliberately ignore cancellation: stale validation must reject even a
	// provider that returns a plausible edit after its context was canceled.
	<-p.release
	return p.reply, nil
}
func (p *reviewFixtureProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Available: true}
}

func reviewFixture(t *testing.T) (*Server, *transport.Fake, *reviewFixtureProvider) {
	t.Helper()
	provider := &reviewFixtureProvider{entered: make(chan llm.ChatRequest, 8), release: make(chan struct{}), reply: reviewPaceCorrection}
	provider.local.responses = []string{
		`{"action":"start","reply":"Moving slowly.","edits":[{"speed_percent":18}]}`,
		`{"action":"update","reply":"Still slow.","edits":[{"speed_percent":40}]}`,
		`{"action":"update","reply":"New direction.","edits":[{"speed_percent":28}]}`,
	}
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(func() {
		select {
		case <-provider.release:
		default:
			close(provider.release)
		}
	})
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		settings.LLM.Model = "local-model"
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 10, 60
		settings.LLM.Connections = []config.ModelConnection{{ID: "review", Name: "Reviewer", Provider: config.LLMProviderCompatible, BaseURL: "http://127.0.0.1:1", Model: "review-model", NoAuthentication: true}}
		return settings
	})
	started := postChatStream(t, s, `{"message":"Start slowly at 18."}`)
	if !strings.Contains(started, `"ok":true`) || s.currentMotionEngine() == nil {
		t.Fatalf("start: %s", started)
	}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionReviewer = config.MotionReviewerSettings{Enabled: true, ConnectionID: "review"}
		return settings
	})
	return s, fake, provider
}

func awaitReview(t *testing.T, s *Server, state string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.motionReview.snapshot().State == state {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("review status=%+v want=%s", s.motionReview.snapshot(), state)
}

func TestMotionReviewChatAppliesLocalFirstThenCorrectsOneStream(t *testing.T) {
	s, fake, provider := reviewFixture(t)
	stream := postChatStream(t, s, `{"message":"Keep the slow pace, please."}`)
	if !strings.Contains(stream, `"ok":true`) || s.currentMotionEngine().Snapshot().Target.Flow.SpeedPercent != 40 {
		t.Fatalf("local choice was delayed: %s", stream)
	}
	select {
	case request := <-provider.entered:
		if !strings.Contains(request.Messages[1].Content, "Keep the slow pace") {
			t.Fatal("review lost user intent")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("review not called")
	}
	close(provider.release)
	awaitReview(t, s, "corrected")
	if got := s.currentMotionEngine().Snapshot().Target.Flow.SpeedPercent; got != 18 {
		t.Fatalf("pace=%d", got)
	}
	plays := 0
	for _, command := range fake.Commands() {
		if command.Kind == transport.CommandKindPointsPlay {
			plays++
		}
	}
	if plays != 1 || provider.local.callCount() != 2 || provider.reviews.Load() != 1 {
		t.Fatalf("plays=%d local=%d reviews=%d", plays, provider.local.callCount(), provider.reviews.Load())
	}
}

func TestMotionReviewLateCorrectionsCannotOverrideNewIntent(t *testing.T) {
	for _, change := range []string{"stop", "pause", "target", "settings", "new_chat", "session", "credentials", "expired"} {
		t.Run(change, func(t *testing.T) {
			s, _, provider := reviewFixture(t)
			postChatStream(t, s, `{"message":"Keep the slow pace."}`)
			select {
			case <-provider.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("no review")
			}
			switch change {
			case "stop":
				finish, _ := s.invalidateWorkForStop("test_review_stop")
				finish()
				_, _ = s.currentMotionEngine().Stop(t.Context(), "test")
			case "pause":
				_, _ = s.currentMotionEngine().Pause(t.Context(), "test")
			case "target":
				target := s.currentMotionEngine().Snapshot().Target
				target.Flow.SpeedPercent = 27
				_, _ = s.currentMotionEngine().ApplyTarget(t.Context(), target, "new_target")
			case "settings":
				saveSettings(t, s.store, func(settings config.Settings) config.Settings { settings.Motion.SpeedMaxPercent--; return settings })
			case "new_chat":
				postChatStream(t, s, `{"message":"Use 28 now.","motion_owner":"off"}`)
			case "session":
				_, _ = s.chatLog.CreateSession(true)
			case "credentials":
				s.invalidateCloudPlanning()
			case "expired":
				s.motionReview.mu.Lock()
				s.motionReview.cancel()
				s.motionReview.mu.Unlock()
			}
			before := s.currentMotionEngine().Snapshot()
			close(provider.release)
			awaitReview(t, s, "skipped")
			s.motionReview.wg.Wait()
			after := s.currentMotionEngine().Snapshot()
			if after.PlanID != before.PlanID || after.Running != before.Running || after.Paused != before.Paused {
				t.Fatalf("late review changed state: before=%+v after=%+v", before.Target, after.Target)
			}
		})
	}
}

func TestMotionReviewAutopilotObservationAndAdoption(t *testing.T) {
	s, _, provider := reviewFixture(t)
	engine := s.currentMotionEngine()
	before := engine.Snapshot().Target
	// Drive the same accepted-choice boundary as the real mode loop, without
	// waiting for its randomly sampled cadence or making a physical connection.
	decide := func(context.Context, modes.DecisionInput) (modes.Decision, error) {
		next := motion.CloneFlowSpec(before.Flow)
		next.SpeedPercent = 40
		return modes.Decision{Segment: modes.Segment{Flow: next, SpeedPercent: 40}, Next: modes.TimingLater, Variability: modes.VariabilitySettled}, nil
	}
	manager, err := modes.NewManager(modes.Options{
		Ensure:               func(context.Context) (modes.Engine, error) { return engine, nil },
		Current:              func() modes.Engine { return engine },
		Settings:             func() config.MotionSettings { settings, _ := s.store.Snapshot(); return settings.Motion },
		MotionGenerationMode: func() string { return config.LLMMotionModeCreativeV2 },
		Decide:               decide, ObserveChoice: s.reviewAutopilotChoice,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.modes = manager
	if _, err := s.modes.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Autopilot did not submit a review")
	}
	if engine.Snapshot().Target.Flow.SpeedPercent != 40 {
		t.Fatal("Autopilot local choice not immediate")
	}
	close(provider.release)
	awaitReview(t, s, "corrected")
	if engine.Snapshot().Target.Flow.SpeedPercent != 18 || s.modes.Status().DecisionSource != "second_opinion" {
		t.Fatal("Autopilot did not adopt correction")
	}
}

func TestMotionReviewOffKeepsLocalCallsAndPrompts(t *testing.T) {
	s, _, provider := reviewFixture(t)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionReviewer.Enabled = false
		return settings
	})
	postChatStream(t, s, `{"message":"Keep the slow pace."}`)
	if provider.reviews.Load() != 0 || provider.local.callCount() != 2 {
		t.Fatal("disabled reviewer did work")
	}
	for _, request := range provider.local.requests {
		for _, message := range request.Messages {
			if strings.Contains(message.Content, "second opinion") {
				t.Fatal("review instructions entered local prompt")
			}
		}
	}
	if got := s.currentMotionEngine().Snapshot().Target.Flow.SpeedPercent; got != 40 {
		t.Fatalf("pace=%d", got)
	}
}
