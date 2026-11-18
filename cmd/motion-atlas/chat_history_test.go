//go:build magichandy_labs

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestHistoryReplayAtlasUsesRecordedCalibration(t *testing.T) {
	settings := config.DefaultSettings().Motion
	settings.HandyModel, settings.StrokeMinPercent, settings.StrokeMaxPercent = config.HandyModel2Standard, 19, 77
	settings.ReverseDirection = true
	score := motion.DefaultFlowSpec()
	score.Seed = 479
	score.MinPercent, score.MaxPercent = 0, 100
	score.RangeFloorPercent, score.RangeCeilingPercent = 100, 100
	score.Layers = nil
	target := motion.MotionTarget{Flow: &score, SpeedPercent: score.SpeedPercent}
	recorded := motion.ReviewMotionOutput(target, settings)
	if recorded.Error != "" {
		t.Fatal(recorded.Error)
	}
	if recorded.Summary.PositionMinPercent > 2 || recorded.Summary.PositionMaxPercent < 98 {
		t.Fatalf("full logical travel inside the reversed 19–77 device window: %+v", recorded.Summary)
	}
	for _, schema := range []string{"chat_history_replay.v1", "chat_history_replay.v2"} {
		t.Run(schema, func(t *testing.T) {
			f, err := os.Create(filepath.Join(t.TempDir(), "replay.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			enc := json.NewEncoder(f)
			row := map[string]any{"kind": "turn", "session": 1, "step": 1, "running": true, "action": "update",
				"after": score, "first_pass": false, "trajectory": recorded.Summary}
			if schema == "chat_history_replay.v2" {
				row["outcome"] = "repaired"
			}
			for _, item := range []any{map[string]any{"kind": "profile", "schema": schema, "settings": settings}, row} {
				if err := enc.Encode(item); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			results := readHistoryReplay(f.Name())
			if len(results) != 1 || results[0].HandyModel != settings.HandyModel || !reflect.DeepEqual(results[0].Summary, recorded.Summary) {
				t.Fatal("the exported trajectory differs from the recorded simulator trajectory")
			}
			if strings.Contains(results[0].Outcome, "repaired") != (schema == "chat_history_replay.v2") {
				t.Fatal("the atlas lost a recorded outcome or inferred one from a v1 capture")
			}
		})
	}
}
