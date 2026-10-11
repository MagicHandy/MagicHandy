//go:build liveeval && magichandy_labs

package httpapi

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type liveReviewProviders struct {
	local, reviewer         llm.Provider
	localModel, reviewModel string
	beforeReview            func()
}

func (p liveReviewProviders) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	if request.Model == "review-model" {
		p.beforeReview()
		request.Model = p.reviewModel
		return p.reviewer.StreamChat(ctx, request, delta)
	}
	request.Model = p.localModel
	return p.local.StreamChat(ctx, request, delta)
}
func (p liveReviewProviders) Status(ctx context.Context) llm.ProviderStatus {
	return p.local.Status(ctx)
}

// Real local and ChatGPT completions through the app's chat and scheduled
// Autopilot paths. The only transport is Fake; account credentials are read
// through the existing profile's token closure and never copied into the app.
func TestMotionReviewLiveApp(t *testing.T) {
	model, localModel := os.Getenv("MAGICHANDY_LIVE_MODEL"), os.Getenv("MAGICHANDY_LOCAL_MODEL")
	if model == "" || localModel == "" || os.Getenv("MAGICHANDY_HOSTED_DATA_DIR") == "" {
		t.Skip("set local/reviewer models and existing account profile")
	}
	reviewer, _ := creativeReviewProvider(t, model)
	local, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: localModel, Timeout: 45 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	s, fake, _ := reviewFixture(t)
	var previewMu sync.Mutex
	previews := []map[string]any{}
	s.llm.provider = liveReviewProviders{local: local, reviewer: reviewer, localModel: localModel, reviewModel: model, beforeReview: func() {
		state := s.currentMotionEngine().Snapshot()
		settings, _ := s.store.Snapshot()
		output := motion.ReviewMotionOutput(state.Target, settings.Motion)
		previewMu.Lock()
		previews = append(previews, map[string]any{"request": "Local output BEFORE the second opinion", "motion": output})
		previewMu.Unlock()
	}}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.ChatVoice = "utility"
		settings.LLM.ReasoningMode = "off"
		return settings
	})
	rows := []map[string]any{}
	defer func() {
		s.motionReview.invalidate()
		s.motionReview.wg.Wait()
		previewMu.Lock()
		defer previewMu.Unlock()
		exportLabExperimentCapture(t, map[string]any{"scenario": "Real Gemma + ChatGPT asynchronous second opinion, simulated motion", "turns": append(rows, previews...), "commands": fake.Commands(), "trace": s.traces.Export()})
	}()
	finish := func() motion.ActiveMotionState {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			status := s.motionReview.snapshot()
			if status.State == "kept" || status.State == "corrected" {
				return s.currentMotionEngine().Snapshot()
			}
			if status.State == "skipped" || status.State == "unavailable" {
				t.Fatalf("live review: %+v", status)
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("live reviewer did not complete")
		return motion.ActiveMotionState{}
	}
	for _, request := range []string{
		`{"message":"Use full strokes from 0 to 100, with repeated shorter base rebounds mixed in. Base focus 0, width 45, mix 55. Three rebounds, retaining 75 percent width. Keep the pace unchanged."}`,
		`{"message":"Remove all rebounds, keep everything else unchanged."}`,
		`{"message":"Keep the pace at 18 until I ask otherwise. You may develop the other motion controls naturally."}`,
	} {
		started := time.Now()
		stream := postChatStream(t, s, request)
		localMillis := time.Since(started).Milliseconds()
		if !strings.Contains(stream, `"ok":true`) || strings.Contains(stream, `event: malformed`) {
			t.Fatalf("local chat: %s", stream)
		}
		state := finish()
		settings, _ := s.store.Snapshot()
		rows = append(rows, map[string]any{"request": request, "stream": stream, "local_ms": localMillis, "review": s.motionReview.snapshot(), "motion": motion.ReviewMotionOutput(state.Target, settings.Motion)})
	}
	prior := s.motionReview.snapshot().Revision
	if _, err := s.modes.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(45 * time.Second)
	for time.Now().Before(until) && s.motionReview.snapshot().Revision <= prior {
		time.Sleep(20 * time.Millisecond)
	}
	if s.motionReview.snapshot().Revision <= prior {
		t.Fatal("no scheduled Autopilot review")
	}
	state := finish()
	settings, _ := s.store.Snapshot()
	rows = append(rows, map[string]any{"request": "Scheduled local Autopilot continuation", "review": s.motionReview.snapshot(), "motion": motion.ReviewMotionOutput(state.Target, settings.Motion)})
	stop := postChatStream(t, s, `{"message":"Stop motion now."}`)
	if state.Target.Flow == nil || s.currentMotionEngine().Snapshot().Running || !strings.Contains(stop, `"action":"stop"`) {
		t.Fatal("shared-engine Stop failed")
	}
}
