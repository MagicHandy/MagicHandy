//go:build liveeval

package motion

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Export every accepted model envelope with the engine's actual sampler. This
// produces inert review data; it never constructs a transport or playback loop.
func TestCapturedVibrationEnvelopes(t *testing.T) {
	paths, output := os.Getenv("MAGICHANDY_VIBRATION_CAPTURE_FILES"), os.Getenv("MAGICHANDY_VIBRATION_ATLAS")
	if paths == "" || output == "" {
		t.Skip("set capture paths and atlas output")
	}
	rows := []map[string]any{}
	for _, path := range strings.Split(paths, ",") {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var source struct {
			Turns []map[string]any `json:"turns"`
		}
		if err := json.Unmarshal(data, &source); err != nil {
			t.Fatal(err)
		}
		for _, row := range source.Turns {
			row["capture_file"] = filepath.Base(path)
			if value := row["vibration"]; value != nil && row["valid"] == true {
				encoded, _ := json.Marshal(value)
				var spec VibrationSpec
				if err := json.Unmarshal(encoded, &spec); err != nil {
					t.Fatal(err)
				}
				if err := spec.Validate(); err != nil {
					t.Fatal(err)
				}
				plan := vibrationPlan{spec: spec}
				samples := make([][2]float64, 0, 1501)
				for at := int64(0); at <= 30_000; at += 20 {
					samples = append(samples, [2]float64{float64(at) / 1000, plan.LevelAt(at)})
				}
				row["vibration_samples"] = samples
			}
			rows = append(rows, row)
		}
	}
	encoded, err := json.Marshal(map[string]any{"turns": rows, "signal": "engine relative drive before human intensity, quantization and startup fade"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("captured %d rows; rejected and no-change results retained", len(rows))
}
