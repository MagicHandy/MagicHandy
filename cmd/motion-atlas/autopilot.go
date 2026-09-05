//go:build magichandy_labs

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Full-app captures retain the accepted semantic target, not a reconstruction
// from chat prose. Compile it again through the same engine for inert review.
func readAutopilotSessions(path string) []motion.Review {
	data, err := os.ReadFile(path) // #nosec G304 -- explicit development CLI input.
	must(err)
	var report struct {
		Sessions []struct {
			Mode, Model string
			Settings    config.MotionSettings
			Targets     []struct {
				At     float64
				Target motion.MotionTarget
			}
			Trace struct {
				Rows []struct {
					Reason  string
					Planner struct{ Note string }
				}
			}
		}
	}
	must(json.Unmarshal(data, &report))
	entries := []motion.Review{}
	for _, session := range report.Sessions {
		for i, captured := range session.Targets {
			entry := motion.ReviewMotionOutput(captured.Target, session.Settings)
			entry.ID = fmt.Sprintf("%s-%s-%d", filepath.Base(path), session.Mode, i+1)
			entry.Name = fmt.Sprintf("%s · %s · %.1fs", filepath.Base(path), session.Mode, captured.At)
			entry.Group, entry.Model = "llm-output", session.Model
			entry.Request = "Autopilot with no human motion request"
			entry.Outcome = "Accepted full-app target; assess character and speech against the session capture"
			entries = append(entries, entry)
		}
		// The trace ring can span several runs. Retain only this final run's
		// failures instead of attributing an earlier mode's failure to this one.
		start := 0
		for i, row := range session.Trace.Rows {
			if row.Reason == "mode_started" {
				start = i
			}
		}
		for _, row := range session.Trace.Rows[start:] {
			note := row.Planner.Note
			if !strings.Contains(note, "rejected") && !strings.Contains(note, "failed") && !strings.Contains(note, "error") {
				continue
			}
			entries = append(entries, motion.Review{Name: session.Mode + " · rejected decision", Group: "llm-output", Model: session.Model, Error: note, Outcome: "Failed autonomous selection retained; previous motion held"})
		}
	}
	return entries
}
