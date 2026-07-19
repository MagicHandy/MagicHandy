package httpapi

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

func TestAutopilotDrivesRealEngineWithCuratedDecisions(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Traces:          diagnostics.NewTraceRing(2048),
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	var mu sync.Mutex
	var says []string
	manager, err := modes.NewManager(modes.Options{
		Ensure: func(context.Context) (modes.Engine, error) {
			engine, admission, err := server.motionEngineForStart()
			if err != nil {
				return nil, err
			}
			return admittedMotionEngine{Engine: engine, admission: admission}, nil
		},
		Current: func() modes.Engine {
			engine := server.currentMotionEngine()
			if engine == nil {
				return nil
			}
			return engine
		},
		Settings:           func() config.MotionSettings { s, _ := server.store.Snapshot(); return s.Motion },
		Traces:             server.traces,
		Tick:               5 * time.Millisecond,
		Seed:               42,
		MaxSegmentDuration: 80 * time.Millisecond,
		Decide: func(_ context.Context, input modes.DecisionInput) (modes.Decision, error) {
			// A scripted curation: pick an enabled builtin with an in-band
			// intensity and speak once at the first check-in.
			say := ""
			if input.SegmentIndex == 0 {
				say = "Starting a steady pulse."
			}
			result := chat.Result{Response: chat.AssistantResponse{
				Reply:  say,
				Motion: &chat.MotionCommand{Action: chat.MotionActionTarget, PatternID: "pulse", Intensity: intPtr(33)},
			}}
			return server.mapAutopilotResult(result)
		},
		Announce: func(_ context.Context, say string) {
			mu.Lock()
			says = append(says, say)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	server.modes = manager
	t.Cleanup(manager.Shutdown)

	if _, err := manager.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.Status()
		if status.SegmentIndex >= 2 && status.DecisionSource == "model" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	status := manager.Status()
	if status.Mode != modes.ModeAutopilot || status.SegmentIndex < 2 {
		t.Fatalf("autopilot did not cross segment boundaries: %+v", status)
	}
	if status.DecisionSource != "model" {
		t.Fatalf("decision source = %q, want model", status.DecisionSource)
	}

	mu.Lock()
	announced := append([]string(nil), says...)
	mu.Unlock()
	if len(announced) == 0 || announced[0] != "Starting a steady pulse." {
		t.Fatalf("announced lines = %v, want the first check-in line", announced)
	}

	// The curated decisions must ride the shared engine as retargets, never
	// per-segment restarts: exactly one play on the wire.
	plays := 0
	for _, command := range fake.Commands() {
		if command.PointsPlay != nil {
			plays++
		}
	}
	if plays != 1 {
		t.Fatalf("wire plays = %d, want exactly 1 continuous stream", plays)
	}
}

func TestAutopilotFallsBackWithoutConfiguredLLM(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Traces:          diagnostics.NewTraceRing(1024),
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	// The default server wiring injects the real LLM-backed decision step; with
	// no provider configured it must fail closed into the deterministic
	// planner, keeping motion alive and reporting the fallback honestly.
	if _, err := server.modes.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatalf("start autopilot: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if server.modes.Status().DecisionSource == "fallback" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	status := server.modes.Status()
	if status.DecisionSource != "fallback" {
		t.Fatalf("decision source = %q, want fallback without an LLM", status.DecisionSource)
	}
	if status.SegmentIndex < 1 {
		t.Fatalf("fallback did not arm a segment: %+v", status)
	}
}

func TestAutopilotDecisionIncludesRecentConversation(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"I remember.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	if _, err := server.chatLog.Append(chat.MessageRoleUser, "Use the slower pattern next.", "client"); err != nil {
		t.Fatalf("append user context: %v", err)
	}
	if _, err := server.chatLog.Append(chat.MessageRoleAssistant, "I will keep that in mind.", ""); err != nil {
		t.Fatalf("append assistant context: %v", err)
	}

	decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{
		Style:           config.MotionStyleBalanced,
		SpeedMinPercent: 20,
		SpeedMaxPercent: 80,
	})
	if err != nil {
		t.Fatalf("autopilotDecide: %v", err)
	}
	if !decision.Hold || decision.Say != "I remember." {
		t.Fatalf("decision = %+v, want conversational hold", decision)
	}

	provider.mu.Lock()
	requests := append([]llm.ChatRequest(nil), provider.requests...)
	provider.mu.Unlock()
	if len(requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(requests))
	}
	var contextText strings.Builder
	for _, message := range requests[0].Messages {
		contextText.WriteString(message.Content)
		contextText.WriteByte('\n')
	}
	for _, want := range []string{"Use the slower pattern next.", "I will keep that in mind.", "Autopilot check-in"} {
		if !strings.Contains(contextText.String(), want) {
			t.Fatalf("provider context missing %q:\n%s", want, contextText.String())
		}
	}
}

func TestAutopilotAnnouncementIsDiscoverableByChatPlayback(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	startSpeakingTTS(t, server, true)

	server.autopilotAnnounce(t.Context(), "Shown and spoken autonomously.")
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 1 || messages[0].Content != "Shown and spoken autonomously." {
		t.Fatalf("autopilot message missing from chat: %+v", messages)
	}
	requestID := messages[0].SpeechRequestID
	if requestID == "" {
		t.Fatal("autopilot message has no speech request ID for browser playback")
	}
	pending, ok := server.voice.Request(requestID)
	if !ok || pending.Text() != messages[0].Content {
		t.Fatalf("speech request %q does not match displayed message", requestID)
	}
}

func TestAutopilotAnnouncementDoesNotDeepenSpeechBacklog(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	startSpeakingTTS(t, server, true)

	pending, err := server.voice.Submit(voice.RoleTTS, voice.Request{
		Type:        voice.RequestSpeak,
		Text:        "Already speaking.",
		DelayMillis: 1000,
	})
	if err != nil {
		t.Fatalf("submit existing speech: %v", err)
	}
	t.Cleanup(func() { server.voice.Worker(voice.RoleTTS).Cancel(pending) })
	deadline := time.Now().Add(time.Second)
	for !autopilotSpeechBacklogged(server.voice.Worker(voice.RoleTTS).Status()) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !autopilotSpeechBacklogged(server.voice.Worker(voice.RoleTTS).Status()) {
		t.Fatal("existing speech never became active or queued")
	}

	server.autopilotAnnounce(t.Context(), "Visible while speech is busy.")
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 1 || messages[0].Content != "Visible while speech is busy." {
		t.Fatalf("autopilot message missing from chat: %+v", messages)
	}
	if messages[0].SpeechRequestID != "" {
		t.Fatalf("busy speech queue received another request: %+v", messages[0])
	}
	if requests := server.voice.Requests(); len(requests) != 1 || requests[0].ID != pending.ID {
		t.Fatalf("voice requests = %+v, want only existing request %q", requests, pending.ID)
	}
}

func TestCanceledAutopilotAnnouncementNeverEntersChat(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	server.autopilotAnnounce(ctx, "Do not publish this.")
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 0 {
		t.Fatalf("canceled announcement entered chat: %+v", messages)
	}
}

func TestMapAutopilotResult(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	hold, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{Reply: "just talking"}})
	if err != nil || !hold.Hold || hold.Say != "just talking" {
		t.Fatalf("nil motion => %+v, %v; want hold with say", hold, err)
	}

	stop, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Reply:  "stopping",
		Motion: &chat.MotionCommand{Action: chat.MotionActionStop},
	}})
	if err != nil || !stop.Hold {
		t.Fatalf("model stop => %+v, %v; want hold (user owns stop)", stop, err)
	}

	curated, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Reply:  "picking up",
		Motion: &chat.MotionCommand{Action: chat.MotionActionTarget, PatternID: "stroke", Intensity: intPtr(45)},
	}})
	if err != nil {
		t.Fatalf("curated: %v", err)
	}
	if curated.Hold || string(curated.Segment.PatternID) != "stroke" || curated.Segment.SpeedPercent != 45 || curated.Pattern == nil {
		t.Fatalf("curated => %+v; want resolved stroke segment", curated)
	}

	disabled, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Reply:  "hm",
		Motion: &chat.MotionCommand{Action: chat.MotionActionTarget, PatternID: "not-a-pattern", Intensity: intPtr(45)},
	}})
	if err != nil || !disabled.Hold {
		t.Fatalf("unknown pattern => %+v, %v; want hold", disabled, err)
	}
}

func TestAutopilotDecisionMessageFramesTheContract(t *testing.T) {
	message := chat.AutopilotDecisionMessage(chat.AutopilotContext{
		Style:            "balanced",
		SegmentIndex:     2,
		RecentPatternIDs: []string{"stroke", "pulse"},
		SpeedMinPercent:  20,
		SpeedMaxPercent:  80,
		LastSay:          "previous line",
	})
	for _, want := range []string{
		"Autopilot check-in 3",
		"balanced",
		"20-80%",
		"stroke, pulse",
		"previous line",
		"Never use action \"stop\"",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("decision message missing %q:\n%s", want, message)
		}
	}
}

func intPtr(value int) *int { return &value }
