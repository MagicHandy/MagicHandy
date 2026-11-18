//go:build liveeval && magichandy_labs

package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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
	source := struct {
		Motion      config.MotionSettings
		HandyModels []string `json:"handy_models"`
		SourceID    string   `json:"source_id"`
		InferStops  bool     `json:"infer_stops"`
		Sessions    []historyReplaySession
	}{Motion: config.DefaultSettings().Motion, InferStops: true}
	if err := json.NewDecoder(os.Stdin).Decode(&source); err != nil {
		t.Fatal("invalid replay input")
	}
	if len(source.Sessions) == 0 {
		t.Fatal("invalid replay sessions")
	}
	settings := config.DefaultSettings()
	settings.Motion = source.Motion.WithHandyCalibration(source.HandyModels)
	settings, err := config.NormalizeSettings(settings)
	if err != nil {
		t.Fatalf("invalid replay calibration: %v", err)
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
	active := profile.settings.ConversationSettings()
	header := map[string]any{"kind": "profile", "schema": "chat_history_replay.v2", "model": active.Model,
		"capability": active.ModelCapabilityLevel(), "context_size": profile.window, "settings": settings.Motion,
		"binary_sha256": historyReplayBuildDigest(t), "source_id": source.SourceID, "infer_stops": source.InferStops,
		"provider": active.Provider, "seed_mode": "provider_default"}
	if active.IsHosted() {
		header["provider"] = active.ActiveConnection.Provider
		header["reasoning_effort"] = active.ActiveConnection.ReasoningEffort
	} else {
		header["gguf_sha256"], header["template_fix"], header["max_output_tokens"] = profile.digest, profile.template, active.MaxOutputTokens
	}
	if err := writer.Encode(header); err != nil {
		t.Fatal(err)
	}
	for _, session := range source.Sessions {
		t.Run(fmt.Sprintf("session_%d", session.Number), func(t *testing.T) {
			replayHistorySession(t, writer, profile, session, settings.Motion)
		})
	}
}

func historyReplayBuildDigest(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenInRoot(filepath.Dir(executable), filepath.Base(executable))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func replayHistorySession(t *testing.T, writer *json.Encoder, profile *qualityDefaultProfile, session historyReplaySession, calibration config.MotionSettings) {
	t.Helper()
	active := profile.settings.ConversationSettings()
	server, provider, fake := qualityServer(t, active.Model, config.LLMMotionModeLayered, "", profile.window, active.IsHosted(), profile)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion = calibration
		return settings
	})
	saved, _ := server.store.Snapshot()
	if saved.EffectiveMotion() != calibration {
		t.Fatal("simulator calibration differs from the replay header")
	}
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
			insertedStop := event.Reset && qualityState(server).Running
			if insertedStop {
				req := withController(httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader("{}")))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				server.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusOK || qualityState(server).Running {
					t.Fatal("could not reconstruct a stop before a later recorded start")
				}
			}
			step++
			recordHistoryReplayTurn(t, writer, server, provider, event, session.Number, step, insertedStop)
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

func recordHistoryReplayTurn(t *testing.T, writer *json.Encoder, server *Server, provider *qualityProvider, event historyReplayEvent, session, step int, insertedStop bool) {
	t.Helper()
	before := qualityState(server)
	provider.attempts = nil
	start := time.Now()
	events, _ := qualityChat(t, server, event.Content)
	elapsed := time.Since(start)
	after := qualityState(server)
	calls := len(provider.attempts)
	outcome := historyReplayOutcome(events, calls, isChatStopMessage(event.Content))
	record := map[string]any{
		"kind": "turn", "session": session, "step": step, "elapsed_ms": elapsed.Milliseconds(),
		"outcome": outcome, "first_pass": outcome == "first_pass",
		"reply_emitted": strings.TrimSpace(events.Reply) != "", "inferred_restart": event.Reset, "inserted_stop": insertedStop,
		"action": events.Action, "calls": calls, "repaired": events.Repaired, "fallback": events.Fallback,
		"first_token_ms": events.Diagnostics.FirstTokenMillis, "prefill_ms": events.Diagnostics.PromptEvalMillis,
		"prompt_tokens": events.Diagnostics.PromptTokens, "cached_tokens": events.Diagnostics.CachedPromptTokens,
		"generated_tokens": events.Diagnostics.GeneratedTokens, "was_running": before.Running, "running": after.Running,
		"plan_changed": before.PlanID != after.PlanID, "before": before.Target.Flow, "after": after.Target.Flow,
	}
	if calls > 0 {
		record["requested_sampling"] = provider.attempts[0].Sampling
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

func historyReplayOutcome(events qualityEvents, calls int, stop bool) string {
	switch {
	case !events.OK || strings.TrimSpace(events.Reply) == "":
		return "failed"
	case events.Diagnostics.RefusalFallbackFrom != "":
		return "refusal_fallback"
	case events.Fallback:
		return "fallback"
	case events.Repaired:
		return "repaired"
	case (stop && calls == 0) || (!stop && calls == 1):
		return "first_pass"
	case !stop && calls > 1:
		return "retried"
	default:
		return "unexpected_calls"
	}
}

func TestHistoryReplayOutcomes(t *testing.T) {
	for _, tc := range []struct {
		want   string
		events qualityEvents
		calls  int
		stop   bool
	}{
		{"first_pass", qualityEvents{OK: true, Reply: "Ready."}, 1, false},
		{"first_pass", qualityEvents{OK: true, Reply: "Stopped."}, 0, true},
		{"repaired", qualityEvents{OK: true, Reply: "Ready.", Repaired: true}, 2, false},
		{"retried", qualityEvents{OK: true, Reply: "Ready."}, 2, false},
		{"fallback", qualityEvents{OK: true, Reply: "Ready.", Fallback: true}, 2, false},
		{"refusal_fallback", qualityEvents{OK: true, Reply: "Ready.", Diagnostics: chat.MessageDiagnostics{RefusalFallbackFrom: "original"}}, 2, false},
		{"failed", qualityEvents{Reply: "Reply emitted before a dispatch failure."}, 1, false},
		{"failed", qualityEvents{OK: true}, 1, false},
		{"unexpected_calls", qualityEvents{OK: true, Reply: "Stopped."}, 1, true},
	} {
		if got := historyReplayOutcome(tc.events, tc.calls, tc.stop); got != tc.want {
			t.Errorf("got %s, want %s (calls %d, stop %t)", got, tc.want, tc.calls, tc.stop)
		}
	}
}
