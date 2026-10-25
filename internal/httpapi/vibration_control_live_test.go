//go:build liveeval

package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type vibrationAppCapture struct {
	llm.Provider
	calls int
	raw   string
}

func (p *vibrationAppCapture) StreamChat(ctx context.Context, req llm.ChatRequest, delta func(string) error) (string, error) {
	p.calls++
	raw, err := p.Provider.StreamChat(ctx, req, delta)
	p.raw = raw
	return raw, err
}

// Real model -> HTTP chat -> parser -> engine -> simulated Buttplug. The host
// credential store is read in place; no keys or live-device settings are copied.
func TestIndependentVibrationProductionChatLive(t *testing.T) {
	if os.Getenv("MAGICHANDY_VIBRATION_EVAL") != "1" || os.Getenv("MAGICHANDY_HOSTED_DATA_DIR") == "" {
		t.Skip("opt-in hosted simulation")
	}
	provider, model, hosted := timingLiveProvider(t)
	if !hosted {
		t.Fatal("this evaluation requires the hosted provider")
	}
	capture := &vibrationAppCapture{Provider: provider}
	s := independentVibrationServer(t)
	s.llm.provider = capture
	s.traces = diagnostics.NewTraceRing(4096)
	rows := []map[string]any{}
	t.Cleanup(func() {
		if path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"); path != "" {
			data, _ := json.MarshalIndent(map[string]any{"model": model, "transport": "in-process Buttplug simulator; no real device", "turns": rows, "trace": s.traces.Rows()}, "", "  ")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Error(err)
			}
		}
	})
	for i, request := range []string{
		"Start the vibrator pulsing between 20 and 60 percent of my armed intensity every 3 seconds. Keep The Handy stopped.",
		"Now start The Handy with full-device sweeps at 50 percent speed. Keep the existing vibration pulse unchanged.",
		"Keep The Handy exactly unchanged. Change only vibration to a smooth six-second wave from 10 to 50 percent of my armed intensity.",
		"Turn off only the vibrator. Keep The Handy moving exactly as it is.",
		"Bring the vibrator back with a gentle eight-second wave of your choosing. Leave The Handy exactly unchanged.",
		"Keep that vibration unchanged. On The Handy repeat two shorter strokes at the tip followed by one full-device sweep, flowing between them without pauses. Keep the current base speed.",
		"Make just the full sweep slower than those two short strokes. Preserve the exact counts, reach, base speed and vibration wave.",
		"Keep The Handy exactly unchanged. Soften only the vibration wave's peak; preserve its low level and cycle length.",
	} {
		var before motion.ActiveMotionState
		if e := s.currentMotionEngine(); e != nil {
			before = e.Snapshot()
		}
		calls, started := capture.calls, time.Now()
		body, _ := json.Marshal(map[string]string{"message": request})
		response := postChatStream(t, s, string(body))
		engine := s.currentMotionEngine()
		var after motion.ActiveMotionState
		if engine != nil {
			after = engine.Snapshot()
		}
		pass := !strings.Contains(response, "event: error") && !strings.Contains(response, `"repaired":true`) && !strings.Contains(response, `"semantic_fallback":true`) && capture.calls-calls == 1 && after.Running
		spec := after.Target.Vibration
		switch i {
		case 0:
			pass = pass && after.Target.VibrationOnly && s.intifaceSnapshot().Status.PlaybackState != "playing" && spec != nil && spec.Mode == "pulse" && spec.MinPercent == 20 && spec.MaxPercent == 60 && spec.PeriodSeconds == 3
		case 1:
			pass = pass && !after.Target.VibrationOnly && after.Target.SpeedPercent == 50 && reflect.DeepEqual(spec, before.Target.Vibration)
		case 2:
			pass = pass && before.PlanID == after.PlanID && reflect.DeepEqual(before.Target.Flow, after.Target.Flow) && spec != nil && spec.Mode == "wave" && spec.MinPercent == 10 && spec.MaxPercent == 50 && spec.PeriodSeconds == 6
		case 3:
			pass = pass && before.PlanID == after.PlanID && spec != nil && spec.Mode == "off"
		case 4:
			pass = pass && before.PlanID == after.PlanID && reflect.DeepEqual(before.Target.Flow, after.Target.Flow) && spec != nil && spec.Mode == "wave" && spec.MinPercent < spec.MaxPercent && spec.PeriodSeconds == 8
		case 5:
			pass = pass && reflect.DeepEqual(spec, before.Target.Vibration) && after.Target.SpeedPercent == before.Target.SpeedPercent
			flow := after.Target.Flow
			pass = pass && flow != nil && flow.Phrase != nil && len(flow.Phrase.Strokes) == 2
			if pass {
				strokes := flow.Phrase.Strokes
				pass = strokes[0].Count == 2 && strokes[0].BottomPercent > 0 && strokes[0].TopPercent == 100 && strokes[1].Count == 1 && strokes[1].BottomPercent == 0 && strokes[1].TopPercent == 100
			}
		case 6:
			pass = pass && reflect.DeepEqual(spec, before.Target.Vibration) && after.Target.SpeedPercent == before.Target.SpeedPercent
			oldFlow, flow := before.Target.Flow, after.Target.Flow
			pass = pass && oldFlow != nil && flow != nil && flow.Phrase != nil && oldFlow.Phrase != nil && len(flow.Phrase.Strokes) == 2 && len(oldFlow.Phrase.Strokes) == 2
			if pass {
				oldStrokes, strokes := oldFlow.Phrase.Strokes, flow.Phrase.Strokes
				oldSweep, sweep := oldStrokes[1], strokes[1]
				oldSweep.RatePercent, sweep.RatePercent = 0, 0
				pass = oldStrokes[0] == strokes[0] && oldSweep == sweep && strokes[1].RatePercent < strokes[0].RatePercent
			}
		case 7:
			old := before.Target.Vibration
			pass = pass && before.PlanID == after.PlanID && reflect.DeepEqual(before.Target.Flow, after.Target.Flow) && old != nil && spec != nil
			if pass {
				pass = spec.Mode == old.Mode && spec.MinPercent == old.MinPercent && spec.MaxPercent < old.MaxPercent && spec.MaxPercent > spec.MinPercent && spec.PeriodSeconds == old.PeriodSeconds
			}
		}
		rows = append(rows, map[string]any{"method": "layered", "recipe_name": "production_chat", "message": request, "raw": capture.raw, "sse": response, "intent_pass": pass,
			"before": before.Target.Flow, "after": after.Target.Flow, "engine_before": before, "engine_after": after, "limits": config.DefaultSettings().Motion,
			"vibration": spec, "current_vibration": before.Target.Vibration, "provider_calls": capture.calls - calls, "elapsed_ms": time.Since(started).Milliseconds(), "vibrator": s.intifaceSnapshot().Vibrator})
		if !pass {
			t.Fatalf("turn %d failed; capture retains response and engine state", i+1)
		}
	}
	if _, err := s.emergencyStop(t.Context(), "live_review_finished"); err != nil {
		t.Fatal(err)
	}
	if s.currentMotionEngine().Snapshot().Running || s.intifaceSnapshot().Vibrator.IntensityPercent != 0 {
		t.Fatal("global Stop did not stop and zero intensity")
	}
}
