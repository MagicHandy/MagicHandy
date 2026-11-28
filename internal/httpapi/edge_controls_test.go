package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestEdgeNeverAddsAnIdleLinearOutput(t *testing.T) {
	s := independentVibrationServer(t)
	level := motion.VibrationSpec{Mode: "steady", MinPercent: 50, MaxPercent: 50}
	result, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &level})
	if err != nil || !result.Engine.Target.VibrationOnly {
		t.Fatal("could not start vibration alone", err)
	}
	engine := s.currentMotionEngine()
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "test_cleanup") })
	before := engine.Snapshot()
	if before.EdgeAllowed {
		t.Fatal("vibration-only Edge advertised")
	}
	session, _, err := s.chatLog.EdgeChatAnchor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	turn, finish, err := s.chatWorkspace.BeginTurn(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	for _, url := range []string{"/api/motion/edge", "/api/motion/edge?style=still", "/api/motion/edge/resume", "/api/motion/edge/resume?style=still"} {
		r := personalizationRequest(t, s, http.MethodPost, url, `{}`)
		if r.Code != http.StatusConflict {
			t.Fatalf("%s accepted: %d", url, r.Code)
		}
	}
	after := engine.Snapshot()
	if turn.Err() != nil || !after.Target.VibrationOnly || after.StreamID != before.StreamID || s.intifaceSnapshot().Status.PlaybackState == "playing" {
		t.Fatalf("ineligible Edge changed vibration, linear playback, or chat: %+v %v", after.Target, turn.Err())
	}
}

func TestEdgeFeedbackUsesQualifiedPlanningRoleAndConversationPolicy(t *testing.T) {
	settings := config.DefaultSettings().LLM
	for _, tc := range []struct {
		model, provider, level string
		want                   bool
	}{
		{"gpt-6.1-terra", config.LLMProviderChatGPT, config.ModelCapabilityAdvanced, true},
		{"gpt-6.1-terra", config.LLMProviderChatGPT, config.ModelCapabilityAdvancedNoReasoning, false},
		{"gpt-6.1-terra", config.LLMProviderChatGPT, config.ModelCapabilityIntermediate, false},
		{"custom-reasoning-model", config.LLMProviderCompatible, config.ModelCapabilityAdvanced, true},
		{"other/advanced-model", config.LLMProviderOpenRouter, config.ModelCapabilityAdvanced, true},
		{"gpt-6.1-sol", config.LLMProviderChatGPT, config.ModelCapabilityAdvanced, true},
		{"inkling-2-mini", config.LLMProviderCompatible, config.ModelCapabilityIntermediate, false},
	} {
		conn := config.ModelConnection{ID: "test", Provider: tc.provider, Model: tc.model, CapabilityLevel: tc.level}
		if got := edgeFeedbackEnabled(settings.WithConnection(conn)); got != tc.want {
			t.Errorf("%s %s %s => %t", tc.model, tc.provider, tc.level, got)
		}
	}
	local := settings
	local.Provider, local.Model = config.LLMProviderLlamaCPP, "user-selected-local-model"
	for _, level := range config.ModelCapabilityLevels() {
		local.LocalCapabilityLevel = level
		if edgeFeedbackEnabled(local) != (level == config.ModelCapabilityAdvanced) {
			t.Fatalf("wrong local capability gate for %s", level)
		}
	}
	settings.Connections = []config.ModelConnection{{ID: "terra", Provider: config.LLMProviderChatGPT, Model: "gpt-6.1-terra", CapabilityLevel: config.ModelCapabilityAdvanced}}
	settings.ConversationConnectionID = "terra"
	settings.MotionPlanner.Provider = config.MotionPlannerLocal
	if edgePlannerUsesFeedback(settings) {
		t.Fatal("Gemma planner inherited the conversation's capability")
	}
	settings.ConversationConnectionID = ""
	settings.MotionPlanner.Provider = "connection"
	settings.MotionPlanner.ConnectionID = "terra"
	settings.MotionPlanner.ContextPolicy = config.ContextConversation
	if !edgePlannerUsesFeedback(settings) {
		t.Fatal("separately selected Terra planner lost its conversation policy")
	}
	settings.MotionPlanner.ContextPolicy = config.ContextTechnical
	if edgePlannerUsesFeedback(settings) {
		t.Fatal("technical planner gained user remarks")
	}
}

func TestEdgeFeedbackUsesSelectedRolesInThePlannerRequest(t *testing.T) {
	for _, tc := range []struct {
		name, chatLevel, plannerLevel, policy  string
		switchChat, wantObservation, wantQuote bool
	}{
		{"advanced planner with simple chat", config.ModelCapabilitySimple, config.ModelCapabilityAdvanced, config.ContextConversation, false, true, true},
		{"advanced chat with technical planner", config.ModelCapabilityAdvanced, config.ModelCapabilityAdvanced, config.ContextTechnical, false, true, false},
		{"advanced chat with intermediate planner", config.ModelCapabilityAdvanced, config.ModelCapabilityIntermediate, config.ContextConversation, false, true, false},
		{"advanced without reasoning", config.ModelCapabilitySimple, config.ModelCapabilityAdvancedNoReasoning, config.ContextConversation, false, false, false},
		{"active conversation changed", config.ModelCapabilityAdvanced, config.ModelCapabilityAdvanced, config.ContextConversation, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &scriptedLLMProvider{responses: []string{`{"hold_seconds":76,"resume_speed_percent":24,"resume_style":"gentle"}`}}
			s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.LocalCapabilityLevel = tc.chatLevel
				settings.LLM.LlamaCPPContextSize = settings.LLM.LocalContextSize()
				settings.LLM.MaxOutputTokens = max(settings.LLM.MaxOutputTokens, config.MinimumLocalOutput(tc.chatLevel))
				settings.LLM.Connections = []config.ModelConnection{{ID: "planner", Name: "My model", Provider: config.LLMProviderCompatible,
					BaseURL: "http://127.0.0.1:8080/v1", NoAuthentication: true, Model: "user-selected-model", CapabilityLevel: tc.plannerLevel}}
				settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "connection", ConnectionID: "planner", ContextPolicy: tc.policy}
				return settings
			})
			ctx := t.Context()
			at := time.Now()
			session, seq := s.edgeChatAnchor(ctx)
			if (session != "") != tc.wantObservation {
				t.Fatalf("press observation eligibility: %q", session)
			}
			s.recordEdge(ctx, session, at, seq, "moving", 20)
			const feedback = "Give me 76 seconds and return at 24 percent on my next Edge."
			if _, err := s.chatLog.Append(chat.MessageRoleUser, feedback, "test"); err != nil {
				t.Fatal(err)
			}
			session, seq = s.edgeChatAnchor(ctx)
			next := at.Add(time.Second)
			s.recordEdge(ctx, session, next, seq, "moving", 20)
			if tc.switchChat {
				if _, err := s.chatLog.SaveSession(session); err != nil {
					t.Fatal(err)
				}
				if _, err := s.chatLog.CreateSession(false); err != nil {
					t.Fatal(err)
				}
			}
			result, err := s.decideEdge(ctx, modes.EdgeInput{ChatSessionID: session, PressedAt: next, AfterSeq: seq,
				PreviousSpeed: 50, HoldingSpeed: 20, SpeedMin: 20, SpeedMax: 50, RangeMax: 100})
			if err != nil || result.HoldSeconds != 76 || result.ResumeSpeed != 24 || len(provider.requests) != 1 {
				t.Fatalf("planner result: %+v %v", result, err)
			}
			req := provider.requests[0]
			wantLimit := 192
			if tc.plannerLevel == config.ModelCapabilityAdvanced {
				wantLimit = 1024
			}
			if req.MaxTokens != wantLimit {
				t.Fatalf("planner output limit %d, want %d", req.MaxTokens, wantLimit)
			}
			if req.Model != "user-selected-model" || strings.Contains(req.Messages[1].Content, feedback) != tc.wantQuote {
				t.Fatalf("unexpected model or feedback in request: %+v", req.Messages)
			}
		})
	}
}

func TestStillEdgeRemoteRequiresExplicitReturnAndStopWins(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	startRemoteFixtureMotion(t, s, desktop)
	do := func(path string, want int) {
		t.Helper()
		res := authenticatedRemoteRequest(s, phone, http.MethodPost, path, "phone-tab", `{}`)
		if res.Code != want {
			t.Fatalf("%s => %d %s", path, res.Code, res.Body.String())
		}
	}
	engine := s.currentMotionEngine()
	before := engine.Snapshot()
	do("/api/remote/edge", 200)
	do("/api/remote/edge?style=still", 200)
	if state := engine.Snapshot(); !state.Paused || !state.StillHold || state.Running || state.EdgeAllowed {
		t.Fatalf("still state %+v", state)
	}
	do("/api/remote/edge/resume", 409)
	checkPausedEdgeKeepsChat(t, s, func() { do("/api/remote/edge", 409) })
	do("/api/remote/edge/resume?style=still", 200)
	// Hold still paused the moving hold, which has its own manual Return.
	do("/api/remote/edge/resume", 200)
	if state := engine.Snapshot(); !state.Running || state.StillHold || state.Target.SpeedPercent != before.Target.SpeedPercent {
		t.Fatal("explicit still return lost the paused target")
	}
	do("/api/remote/edge?style=still", 200)
	if _, err := s.emergencyStop(t.Context(), "test_stop"); err != nil {
		t.Fatal(err)
	}
	do("/api/remote/edge/resume?style=still", 409)
	if engine.Snapshot().Running {
		t.Fatal("stale return restarted motion")
	}
}

func TestPausedMovingEdgeRejectsStaleDesktopPress(t *testing.T) {
	s := newTestServer(t)
	for _, step := range []struct{ path, body string }{
		{"/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":35}`},
		{"/api/motion/edge", "{}"},
		{"/api/motion/edge?style=still", "{}"},
	} {
		if got := personalizationRequest(t, s, http.MethodPost, step.path, step.body); got.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", step.path, got.Code, got.Body.String())
		}
	}
	checkPausedEdgeKeepsChat(t, s, func() {
		if got := personalizationRequest(t, s, http.MethodPost, "/api/motion/edge", "{}"); got.Code != http.StatusConflict {
			t.Fatalf("paused Edge: %d", got.Code)
		}
	})
}

func checkPausedEdgeKeepsChat(t *testing.T, s *Server, press func()) {
	t.Helper()
	id, err := s.chatLog.ActiveSessionIDContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	turn, finish, err := s.chatWorkspace.BeginTurn(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	press()
	if state := s.currentMotionEngine().Snapshot(); turn.Err() != nil || !state.StillHold || state.Running {
		t.Fatalf("rejected press disturbed chat or pause: chat=%v still=%t running=%t", turn.Err(), state.StillHold, state.Running)
	}
}
