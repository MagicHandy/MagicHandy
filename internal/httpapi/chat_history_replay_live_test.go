//go:build liveeval && magichandy_labs

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type historyReplayEvent struct {
	Kind, Content string
	Reset         bool
}

type historyReplaySession struct {
	Number int
	Events []historyReplayEvent
}

// Run via scripts/evaluate-chat-history.py. The source transcript arrives on
// stdin and the selected provider receives the turn. Each later user turn sees
// the replies and scores produced in the isolated chat store.
func TestLiveChatHistoryReplay(t *testing.T) {
	path := os.Getenv("MAGICHANDY_REPLAY_OUTPUT")
	if path == "" {
		t.Skip("select a database and model with evaluate-chat-history.py")
	}
	var source struct {
		Min, Max int
		Sessions []historyReplaySession
	}
	if err := json.NewDecoder(os.Stdin).Decode(&source); err != nil {
		t.Fatal("invalid replay input")
	}
	if len(source.Sessions) == 0 || source.Min < 1 || source.Min > source.Max || source.Max > 100 {
		t.Fatal("invalid replay sessions or speed limits")
	}
	profile := loadQualityDefault(t, os.Getenv("MAGICHANDY_QUALITY_PROFILE"))
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dir.Close(); err != nil {
			t.Error(err)
		}
	})
	file, err := dir.OpenFile(filepath.Base(path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	writer := json.NewEncoder(file)
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = source.Min, source.Max
	if err := writer.Encode(map[string]any{"kind": "profile", "schema": "chat_history_replay.v1",
		"model": profile.settings.ConversationSettings().Model, "settings": limits}); err != nil {
		t.Fatal(err)
	}
	for _, session := range source.Sessions {
		t.Run(fmt.Sprintf("session_%d", session.Number), func(t *testing.T) {
			replayHistorySession(t, writer, profile, session, source.Min, source.Max)
		})
	}
}

func replayHistorySession(t *testing.T, writer *json.Encoder, profile *qualityDefaultProfile, session historyReplaySession, speedMin, speedMax int) {
	t.Helper()
	active := profile.settings.ConversationSettings()
	server, provider, fake := qualityServer(t, active.Model, config.LLMMotionModeLayered, "", profile.window, active.IsHosted(), profile)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = speedMin, speedMax
		return settings
	})
	id, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	step := 0
	for _, event := range session.Events {
		switch event.Kind {
		case "autopilot":
			_, err := server.chatLog.AppendTo(id, chat.MessageRoleAssistant, event.Content, "", &chat.MessageDiagnostics{Source: "autopilot"})
			if err != nil {
				t.Fatal("could not append recorded Autopilot remark")
			}
		case "user":
			if event.Reset && qualityState(server).Running {
				req := withController(httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader("{}")))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				server.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusOK || qualityState(server).Running {
					t.Fatal("could not reconstruct a stop before a later recorded start")
				}
			}
			step++
			recordHistoryReplayTurn(t, writer, server, provider, event.Content, session.Number, step)
		default:
			t.Fatal("unknown replay event")
		}
	}
	before := fake.Diagnostics().CommandCount
	qualityChat(t, server, "Stop.")
	diag := fake.Diagnostics()
	if qualityState(server).Running || diag.CommandCount <= before || diag.LastResult == nil || !diag.LastResult.ConfirmsStop() {
		t.Fatal("the simulated owner did not confirm Stop")
	}
	if err := writer.Encode(map[string]any{"kind": "stop", "session": session.Number, "commands": fake.Commands()}); err != nil {
		t.Fatal(err)
	}
}

func recordHistoryReplayTurn(t *testing.T, writer *json.Encoder, server *Server, provider *qualityProvider, message string, session, step int) {
	t.Helper()
	before := qualityState(server)
	provider.attempts = nil
	start := time.Now()
	events, _ := qualityChat(t, server, message)
	elapsed := time.Since(start)
	after := qualityState(server)
	calls := len(provider.attempts)
	expectedCalls := 1
	if isChatStopMessage(message) {
		expectedCalls = 0
	}
	firstPass := events.OK && strings.TrimSpace(events.Reply) != "" && !events.Repaired && !events.Fallback &&
		events.Diagnostics.RefusalFallbackFrom == "" && calls == expectedCalls
	record := map[string]any{
		"kind": "turn", "session": session, "step": step, "elapsed_ms": elapsed.Milliseconds(), "first_pass": firstPass,
		"action": events.Action, "calls": calls, "repaired": events.Repaired, "fallback": events.Fallback,
		"first_token_ms": events.Diagnostics.FirstTokenMillis, "prefill_ms": events.Diagnostics.PromptEvalMillis,
		"prompt_tokens": events.Diagnostics.PromptTokens, "cached_tokens": events.Diagnostics.CachedPromptTokens,
		"generated_tokens": events.Diagnostics.GeneratedTokens, "was_running": before.Running, "running": after.Running,
		"plan_changed": before.PlanID != after.PlanID, "before": before.Target.Flow, "after": after.Target.Flow,
	}
	if after.Running && after.Target.Flow != nil && before.PlanID != after.PlanID {
		settings, _ := server.store.Snapshot()
		review := motion.ReviewMotionOutput(after.Target, settings.EffectiveMotion())
		record["trajectory"], record["render_error"] = review.Summary, review.Error
	}
	if err := writer.Encode(record); err != nil {
		t.Fatal(err)
	}
}
