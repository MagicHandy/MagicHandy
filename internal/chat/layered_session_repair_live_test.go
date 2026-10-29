//go:build liveeval && magichandy_labs

package chat

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Replay rejected inert evaluations, then ask the actual hosted model for the
// one production repair. This never reads user sessions or connects hardware.
func TestHostedLayeredSessionRepairLive(t *testing.T) {
	path := os.Getenv("MAGICHANDY_REPAIR_CASES")
	if path == "" {
		t.Skip("set an inert evaluation capture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Turns []struct {
			Name    string          `json:"recipe_name"`
			Message string          `json:"message"`
			Raw     string          `json:"raw"`
			Before  motion.FlowSpec `json:"before"`
			Valid   bool            `json:"valid"`
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	provider, model := hostedLayeredLiveProvider(t)
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range cases.Turns {
		if tc.Valid {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			p := &rejectedFirstProvider{Provider: provider, raw: tc.Raw}
			state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &tc.Before, SpeedMinPercent: 1, SpeedMaxPercent: 100}
			service := Service{Provider: p, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", HostedResponseRepair: true}
			started := time.Now()
			result, err := service.Complete(t.Context(), Request{Message: tc.Message}, nil)
			pass := err == nil && result.Repaired && p.calls == 2 && sessionPhraseMatches(result.Response.Motion, []motion.PhraseStroke{{BottomPercent: 35, TopPercent: 65, Count: 2}, {BottomPercent: 70, TopPercent: 100, Count: 3}, {BottomPercent: 0, TopPercent: 30, Count: 2}})
			row := hostedLayeredLiveRow(tc.Name+"-repair", model, tc.Message, result.RepairRaw, tc.Before, result.Response.Motion, limits, started, err, pass)
			row["initial_raw"], row["initial_error"], row["provider_calls"] = result.Raw, result.InitialError, p.calls
			rows = append(rows, row)
			t.Logf("repair pass=%t elapsed_ms=%v error=%v", pass, row["elapsed_ms"], err)
			if !pass {
				t.Error("repair did not preserve requested complete cycles")
			}
		})
	}
}
