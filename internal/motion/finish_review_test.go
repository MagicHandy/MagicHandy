//go:build magichandy_labs

package motion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Retain every position proposal in the finish-target experiments and compile
// it through the actual Cloud sampler. The session timeline is separately
// rendered from its recorded tempo samples; these are geometry inspections.
func TestFinishTargetVisualReview(t *testing.T) {
	directory := os.Getenv("MAGICHANDY_FINISH_REVIEW")
	if directory == "" {
		t.Skip("opt-in finish target visual review")
	}
	paths, err := filepath.Glob(filepath.Join(directory, "finish-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 10, 100
	var entries []Review
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var capture struct {
			Turns []struct {
				Device, Condition, Model, Error string
				Elapsed                         int      `json:"elapsed_s"`
				Raw                             []string `json:"raw_attempts"`
				Command                         struct {
					Layered *FlowSpec `json:"layered"`
				} `json:"command"`
			} `json:"turns"`
		}
		if err := json.Unmarshal(data, &capture); err != nil {
			t.Fatal(err)
		}
		for _, row := range capture.Turns {
			if row.Device != "positions" {
				continue
			}
			id := fmt.Sprintf("%s-%s-%d", filepath.Base(path), row.Condition, row.Elapsed)
			entry := Review{ID: id, Name: id, Model: row.Model, Error: row.Error, Group: "finish-target"}
			if row.Command.Layered != nil {
				target, err := FlowTarget(*row.Command.Layered, settings)
				if err == nil {
					entry = speedSupportCloudReview(target, settings)
					entry.ID, entry.Name, entry.Model, entry.Group = id, id, row.Model, "finish-target"
				} else {
					entry.Error = err.Error()
				}
			} else if entry.Error == "" {
				entry.Outcome = "hold"
			}
			if len(row.Raw) > 0 {
				entry.Raw = row.Raw[len(row.Raw)-1]
			}
			entries = append(entries, entry)
			if entry.Error != "" {
				t.Errorf("%s: %s", id, entry.Error)
			}
		}
	}
	writeSpeedReviewReport(t, filepath.Join(directory, "position-atlas.json"), map[string]any{"schema": "magichandy.motion-atlas.v1", "entries": entries})
}
