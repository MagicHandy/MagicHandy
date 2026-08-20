package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
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
			// Alternate enabled builtins so each scripted check-in is a real
			// curation rather than an intentional no-op Hold.
			patternID := "pulse"
			intensity := 33
			if input.SegmentIndex%2 == 1 {
				patternID = "stroke"
				intensity = 35
			}
			result := chat.Result{Response: chat.AssistantResponse{
				Motion: &chat.MotionCommand{Action: chat.MotionActionTarget, PatternID: patternID, Intensity: intPtr(intensity)},
			}}
			return server.mapAutopilotResult(result, input)
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
		`{"motion":{"action":"none"},"next":"normal","variability":"settled"}`,
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
		Style:            config.MotionStyleBalanced,
		SpeedMinPercent:  20,
		SpeedMaxPercent:  80,
		CurrentPatternID: motion.PatternStroke,
		CurrentSpeed:     30,
		CurrentAreaFocus: &motion.AreaFocus{MinPercent: 0, MaxPercent: 34},
	})
	if err != nil {
		t.Fatalf("autopilotDecide: %v", err)
	}
	if !decision.Hold || decision.Say != "" || decision.Next != modes.TimingNormal {
		t.Fatalf("decision = %+v, want silent motion hold", decision)
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
	for _, want := range []string{"Use the slower pattern next.", "I will keep that in mind.", "Autopilot motion decision", "an enabled catalog pattern at 30% speed in area \"base\""} {
		if !strings.Contains(contextText.String(), want) {
			t.Fatalf("provider context missing %q:\n%s", want, contextText.String())
		}
	}
}

func TestAutopilotDecisionCanCurateMotionDespiteStopProhibition(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"motion":{"action":"target","pattern_id":"stroke","intensity":45},"next":"soon","variability":"normal"}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)

	decision, err := server.autopilotDecide(t.Context(), modes.DecisionInput{
		Style:           config.MotionStyleBalanced,
		SpeedMinPercent: 20,
		SpeedMaxPercent: 80,
	})
	if err != nil {
		t.Fatalf("autopilotDecide: %v", err)
	}
	if decision.Hold || decision.Pattern == nil || decision.Segment.PatternID != motion.PatternStroke || decision.Segment.SpeedPercent != 45 {
		t.Fatalf("Autopilot target was stripped: %+v", decision)
	}
}

func TestAutopilotSpeechUsesIndependentChatOnlyContractByDefault(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Still here with you.","motion":{"action":"target","speed_percent":45},"next":"later"}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)

	decision, err := server.autopilotDecideSpeech(t.Context(), modes.DecisionInput{
		Style:            config.MotionStyleBalanced,
		SpeedMinPercent:  20,
		SpeedMaxPercent:  80,
		CurrentPatternID: motion.PatternStroke,
		CurrentSpeed:     30,
	})
	if err != nil {
		t.Fatalf("autopilotDecideSpeech: %v", err)
	}
	if !decision.Hold || decision.Say != "Still here with you." || decision.Next != modes.TimingLater {
		t.Fatalf("chat-only speech decision = %+v", decision)
	}
}

func TestAutopilotAnnouncementIsDiscoverableByChatPlayback(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	startSpeakingTTS(t, server, true)

	announcement := server.autopilotAnnounce(t.Context(), "Shown and spoken autonomously.")
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 1 || messages[0].Content != "Shown and spoken autonomously." {
		t.Fatalf("autopilot message missing from chat: %+v", messages)
	}
	requestID := messages[0].SpeechRequestID
	if requestID == "" {
		t.Fatal("autopilot message has no speech request ID for browser playback")
	}
	if !announcement.Published || !announcement.AwaitPlayback || announcement.RequestID != requestID {
		t.Fatalf("announcement = %+v, request = %q", announcement, requestID)
	}
	pending, ok := server.voice.Request(requestID)
	if !ok || pending.Text() != messages[0].Content {
		t.Fatalf("speech request %q does not match displayed message", requestID)
	}
	waitForVoiceRequestDone(t, server, requestID)
	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(
		http.MethodPost,
		"/api/voice/requests/"+requestID+"/played",
		nil,
	))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"acknowledged":false`) {
		t.Fatalf("playback acknowledgement = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAutopilotPreferencesEndpointPersistsValidatedSettings(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	preferences := config.DefaultAutopilotSettings()
	preferences.MotionCadence = config.AutopilotMotionSteady
	preferences.SpeechCadence = config.AutopilotSpeechQuiet
	preferences.SpeechMotionAuthority = config.AutopilotSpeechMotionStyle
	body, err := json.Marshal(preferences)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request := withController(httptest.NewRequest(
		http.MethodPut,
		"/api/modes/autopilot/preferences",
		strings.NewReader(string(body)),
	))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	saved, _ := server.store.Snapshot()
	if saved.Autopilot != preferences {
		t.Fatalf("saved preferences = %+v", saved.Autopilot)
	}

	preferences.MotionMinSeconds = 7
	body, _ = json.Marshal(preferences)
	request = withController(httptest.NewRequest(
		http.MethodPut,
		"/api/modes/autopilot/preferences",
		strings.NewReader(string(body)),
	))
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAutopilotAnnouncementCarriesSessionMood(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := server.chatLog.AppendTo(sessionID, chat.MessageRoleAssistant, "Prior reply.", "", &chat.MessageDiagnostics{Mood: chat.MoodLoving}); err != nil {
		t.Fatalf("seed mood: %v", err)
	}

	server.autopilotAnnounce(t.Context(), "Autonomous follow-up.")
	messages, _, _ := getChatMessages(t, server, "")
	last := messages[len(messages)-1]
	if last.Diagnostics == nil || last.Diagnostics.Mood != chat.MoodLoving {
		t.Fatalf("autopilot diagnostics = %+v, want carried mood", last.Diagnostics)
	}
}

func TestAutopilotAnnouncementCarriesPersonaProvenance(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	set, err := server.personalization.prompts.Create("Autopilot persona", "Keep this persona voice.")
	if err != nil {
		t.Fatalf("create prompt set: %v", err)
	}
	created := createPersonaVia(t, server, "Rowan")
	recorder, _ := personaRequest(t, server, http.MethodPatch, "/api/personas/"+created.ID,
		map[string]any{"prompt_set_id": set.ID})
	if recorder.Code != http.StatusOK {
		t.Fatalf("configure persona: status %d body %s", recorder.Code, recorder.Body.String())
	}
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	recorder, _ = personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})
	if recorder.Code != http.StatusOK {
		t.Fatalf("bind persona: status %d body %s", recorder.Code, recorder.Body.String())
	}

	server.autopilotAnnounce(t.Context(), "Autonomous follow-up.")
	messages, _, _ := getChatMessages(t, server, "")
	last := messages[len(messages)-1]
	if last.Diagnostics == nil ||
		last.Diagnostics.PersonaID != created.ID ||
		last.Diagnostics.PersonaName != "Rowan" ||
		last.Diagnostics.PromptSet != set.ID {
		t.Fatalf("autopilot persona diagnostics = %+v", last.Diagnostics)
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

func autopilotMapInput() modes.DecisionInput {
	return modes.DecisionInput{
		CurrentPatternID: motion.PatternStroke,
		CurrentSpeed:     30,
		CurrentAreaFocus: &motion.AreaFocus{MinPercent: 0, MaxPercent: 34},
	}
}

func TestMapAutopilotResultHoldsNonChanges(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	input := autopilotMapInput()

	hold, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{Reply: "just talking"}}, input)
	if err != nil || !hold.Hold || hold.Say != "just talking" {
		t.Fatalf("nil motion => %+v, %v; want hold with say", hold, err)
	}

	stop, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Reply:  "stopping",
		Motion: &chat.MotionCommand{Action: chat.MotionActionStop},
	}}, input)
	if err != nil || !stop.Hold {
		t.Fatalf("model stop => %+v, %v; want hold (user owns stop)", stop, err)
	}

	noOp, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Motion: &chat.MotionCommand{Action: chat.MotionActionTarget},
	}}, input)
	if err != nil || !noOp.Hold {
		t.Fatalf("unchanged target => %+v, %v; want hold", noOp, err)
	}

	disabled, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Reply:  "hm",
		Motion: &chat.MotionCommand{Action: chat.MotionActionTarget, PatternID: "not-a-pattern", Intensity: intPtr(45)},
	}}, input)
	if err != nil || !disabled.Hold {
		t.Fatalf("unknown pattern => %+v, %v; want hold", disabled, err)
	}
}

func TestMapAutopilotResultAppliesPartialChanges(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	input := autopilotMapInput()

	curated, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Reply:  "picking up",
		Motion: &chat.MotionCommand{Action: chat.MotionActionTarget, PatternID: "stroke", Intensity: intPtr(45)},
	}}, input)
	if err != nil {
		t.Fatalf("curated: %v", err)
	}
	if curated.Hold || string(curated.Segment.PatternID) != "stroke" || curated.Segment.SpeedPercent != 45 ||
		curated.Pattern == nil || !sameAreaFocus(curated.Segment.AreaFocus, input.CurrentAreaFocus) {
		t.Fatalf("curated => %+v; want resolved stroke segment preserving the live area", curated)
	}

	areaOnly, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Reply:  "opening up",
		Motion: &chat.MotionCommand{Action: chat.MotionActionTarget, Area: chat.AreaZoneFull},
	}}, input)
	if err != nil || areaOnly.Hold || areaOnly.Segment.PatternID != motion.PatternStroke ||
		areaOnly.Segment.SpeedPercent != 30 || areaOnly.Segment.AreaFocus != nil {
		t.Fatalf("area-only result => %+v, %v; want current pattern and speed at full area", areaOnly, err)
	}
}

func TestMapAutopilotResultBuildsDynamicSegmentWithoutPatternFallback(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = config.LLMMotionModeDynamic
		return settings
	})
	current := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{
		CenterPercent: 50, SpanPercent: 70, VariationPercent: 10, SegmentSeconds: 12,
	})
	input := modes.DecisionInput{
		CurrentSpeed: 30, CurrentDynamic: &current, MotionMinSeconds: 4, MotionMaxSeconds: 60,
	}
	center, span, variation, seconds := 35, 50, 30, 18
	decision, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Motion: &chat.MotionCommand{
			Action: chat.MotionActionUpdate, CenterPercent: &center, SpanPercent: &span,
			VariationPercent: &variation, SegmentSeconds: &seconds,
		},
	}}, input)
	if err != nil || decision.Hold || decision.Segment.PatternID != "" || decision.Pattern != nil || decision.Segment.Dynamic == nil {
		t.Fatalf("dynamic decision = %+v, %v", decision, err)
	}
	if dynamic := decision.Segment.Dynamic; dynamic.CenterPercent != center || dynamic.SpanPercent != span ||
		dynamic.VariationPercent != variation || dynamic.SegmentSeconds != seconds {
		t.Fatalf("dynamic segment = %+v", dynamic)
	}

	hold, err := server.mapAutopilotResult(chat.Result{Response: chat.AssistantResponse{
		Motion: &chat.MotionCommand{Action: chat.MotionActionUpdate},
	}}, input)
	if err != nil || !hold.Hold {
		t.Fatalf("unchanged dynamic target = %+v, %v; want hold", hold, err)
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
		CurrentPatternID: "stroke",
		CurrentSpeed:     30,
		CurrentArea:      chat.AreaZoneBase,
		AreaFocusEnabled: true,
	})
	for _, want := range []string{
		"Autopilot motion decision 3",
		"balanced",
		"20-80%",
		"current catalog may omit recently played patterns",
		"Current motion: a recently played catalog pattern at 30% speed in area \"base\"",
		"Area changes available now: tip, shaft, full",
		"Suggested spatial contrast for this stretch:",
		"omit area to deliberately keep \"base\"",
		"Never use action \"start\" or \"stop\"",
		"Set next to soon, normal, or later",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("decision message missing %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "stroke, pulse") || strings.Contains(message, `pattern "stroke"`) {
		t.Fatalf("temporarily unavailable pattern IDs leaked into the motion prompt:\n%s", message)
	}
}

func TestAutopilotDecisionMessageOffersFocusedAlternativesFromFull(t *testing.T) {
	message := chat.AutopilotDecisionMessage(chat.AutopilotContext{
		CurrentPatternID: "stroke",
		CurrentSpeed:     30,
		CurrentArea:      chat.AreaZoneFull,
		AreaFocusEnabled: true,
	})
	if !strings.Contains(message, "Area changes available now: tip, shaft, base") ||
		strings.Contains(message, "Area changes available now: tip, shaft, base, full") {
		t.Fatalf("full-area alternatives are not explicit:\n%s", message)
	}
}

func intPtr(value int) *int { return &value }
