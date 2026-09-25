//go:build magichandy_labs

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Continuous session reports keep every accepted score with its seed and every
// failed selection. Compile each changed score again through the shared engine;
// a held stretch repeats the previous card, and a failure keeps its record
// without a moving plot.
func readContinuousSessions(path string) []motion.Review {
	data, err := os.ReadFile(path) // #nosec G304 -- explicit development CLI input.
	must(err)
	var runs []struct {
		Mode, Model string
		Settings    *config.MotionSettings
		Requests    map[int]string
		Turns       []struct {
			Turn             int
			Kind, Raw, Error string
			Held             bool
			Score            *motion.FlowSpec
		}
	}
	must(json.Unmarshal(data, &runs))
	name := filepath.Base(path)
	entries := []motion.Review{}
	for r, run := range runs {
		// Reports before the limits were recorded used these harness values.
		settings := config.DefaultSettings().Motion
		settings.SpeedMinPercent, settings.SpeedMaxPercent = 15, 54
		settings.HandyModel = config.HandyModelOriginal
		if run.Settings != nil {
			settings = *run.Settings
		}
		request := "Autopilot with no human motion request"
		for _, turn := range run.Turns {
			if text, ok := run.Requests[turn.Turn]; ok && turn.Kind == "chat" {
				request = "Latest human line: " + text
			}
			if turn.Kind != "motion" {
				continue
			}
			label := fmt.Sprintf("%s · %s · run %d · decision %d", name, run.Mode, r+1, turn.Turn+1)
			id := fmt.Sprintf("%s-%d-%d", name, r+1, turn.Turn+1)
			switch {
			case turn.Error != "":
				entries = append(entries, motion.Review{ID: id, Name: label, Group: "llm-output", Model: run.Model, Request: request,
					Raw: turn.Raw, Error: turn.Error, Outcome: "Failed autonomous selection retained; previous motion held"})
			case !turn.Held && turn.Score != nil:
				entry := motion.ReviewMotionOutput(motion.MotionTarget{Label: "Autopilot", Source: "autopilot",
					SpeedPercent: turn.Score.SpeedPercent, Flow: motion.CloneFlowSpec(turn.Score)}, settings)
				entry.ID, entry.Name, entry.Group, entry.Model, entry.Request, entry.Raw = id, label, "llm-output", run.Model, request, turn.Raw
				entry.Outcome = "Accepted Autopilot score; the session report keeps the stretch as played"
				entries = append(entries, entry)
			}
		}
	}
	return entries
}
