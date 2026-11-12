//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Export every captured model result, including rejected selections, through
// the production tempo sampler. This fixture creates no transport or device.
func TestExportTempoReview(t *testing.T) {
	directory := os.Getenv("MAGICHANDY_TEMPO_REVIEW")
	if directory == "" {
		t.Skip("opt-in tempo atlas export")
	}
	paths, err := filepath.Glob(filepath.Join(directory, "tempo-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	add := func(row map[string]any, spec *TempoSpec, start float64) {
		if spec != nil && spec.Validate() == nil && spec.Mode != TempoModeFollow {
			resolved := spec.Clone()
			if resolved.Mode == TempoModeRoutine && resolved.Seed == 0 {
				resolved.Seed = 12345
			}
			row["rendered_spec"] = resolved
			plan := newTempoPlan(resolved, 0, start, 1)
			levels := make([]float64, 1201)
			for i := range levels {
				levels[i] = plan.LevelAt(int64(i * 100))
			}
			row["levels"] = levels
		}
		rows = append(rows, row)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var captured struct {
			Turns []map[string]any `json:"turns"`
		}
		if err := json.Unmarshal(data, &captured); err != nil {
			t.Fatal(err)
		}
		for _, row := range captured.Turns {
			row["capture"] = filepath.Base(path)
			encoded, _ := json.Marshal(row["command"])
			var command struct {
				Tempo *TempoSpec `json:"tempo"`
			}
			_ = json.Unmarshal(encoded, &command)
			start, known := row["start_level"].(float64)
			if !known {
				start, _ = row["level_at_decision"].(float64)
			}
			add(row, command.Tempo, start)
		}
	}
	for _, routine := range TempoRoutines() {
		for _, seed := range []uint64{1, 42, 12345} {
			spec := TempoSpec{Mode: TempoModeRoutine, Routine: routine.ID, Seed: seed}
			add(map[string]any{"name": routine.ID, "model": "built-in", "seed": seed, "intent_pass": true}, &spec, 0)
		}
	}
	data, err := json.MarshalIndent(map[string]any{"sample_ms": 100, "rows": rows}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "atlas.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("retained %d results and routine samples", len(rows))
}
