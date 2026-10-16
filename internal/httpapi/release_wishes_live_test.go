//go:build liveeval && magichandy_labs

package httpapi

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Real model decisions, without constructing a transport or starting playback.
func TestReleaseLiveStandingWishes(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	if model == "" || os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE") == "" {
		t.Skip("set a live model and an explicit ignored capture path")
	}
	provider, _ := creativeReviewProvider(t, model)
	rows := []map[string]any{}
	defer func() { exportLabExperimentCapture(t, map[string]any{"model": model, "turns": rows}) }()
	for _, mode := range []chat.MotionMode{chat.MotionModeLayered, chat.MotionModeCreativeV2} {
		for _, tc := range []struct {
			name, message string
			holding, want bool
			speed         int
		}{
			{"hold", "Keep this exact motion unchanged from now on.", false, true, 40},
			{"enjoy", "That feels good. Keep going.", false, false, 40},
			{"still-hold", "That feels good. Keep going.", true, true, 40},
			{"release", "Take the lead again and vary the motion.", true, false, 0},
			{"edit-and-hold", "Slow to 20 percent and keep that exact motion unchanged from now on.", false, true, 20},
		} {
			t.Run(string(mode)+"/"+tc.name, func(t *testing.T) {
				before := chat.DefaultLayeredScore(40)
				before.RangeCeilingPercent = before.MaxPercent - before.MinPercent
				if mode == chat.MotionModeCreativeV2 {
					before = chat.FreshCreativeV2Score(40)
				}
				before.Seed = 123456
				state := chat.MotionContext{MotionMode: mode, Running: true, Autopilot: true, StandingHold: tc.holding, Layered: &before, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80}
				caps := chat.Capabilities{Motion: true, MotionMode: mode, Voice: chat.VoiceUtility}
				service := chat.Service{Provider: provider, Model: model, MotionContext: &state, Capabilities: &caps, MaxTokens: 1200, ReasoningMode: "off"}
				started := time.Now()
				result, err := service.Complete(t.Context(), chat.Request{Message: tc.message}, nil)
				after := before
				if result.Response.Motion != nil {
					after = *result.Response.Motion.Layered
				}
				valid := err == nil && !result.InitialMalformed && !result.Repaired && !result.SemanticFallback
				intent := valid && result.Response.StayUnchanged != nil && *result.Response.StayUnchanged == tc.want
				if tc.speed != 0 {
					want := *motion.CloneFlowSpec(&before)
					want.SpeedPercent = tc.speed
					intent = intent && reflect.DeepEqual(want, after)
				}
				limits := config.DefaultSettings().Motion
				limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
				target, compileErr := motion.FlowTarget(after, limits)
				row := map[string]any{"mode": mode, "request": tc.message, "raw": result.Raw, "before": before, "after": after, "valid": valid, "intent_pass": intent, "elapsed_ms": time.Since(started).Milliseconds()}
				if err != nil {
					row["error"] = err.Error()
				}
				if compileErr == nil {
					row["motion"] = motion.ReviewMotionOutput(target, limits)
				}
				rows = append(rows, row)
				if !intent {
					t.Fatal("standing wish or requested motion was not preserved; see retained capture")
				}
			})
		}
	}
}
