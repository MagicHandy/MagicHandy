//go:build magichandy_labs

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// The replay records commanded scores and calibration without copying dialogue.
// Recompile the score still running after a failed or unchanged turn as well.
func readHistoryReplay(path string) []motion.Review {
	dir, err := os.OpenRoot(filepath.Dir(path))
	must(err)
	defer func() { must(dir.Close()) }()
	file, err := dir.Open(filepath.Base(path))
	must(err)
	defer func() { must(file.Close()) }()
	decoder := json.NewDecoder(file)
	var header struct {
		Schema, Model string
		Settings      config.MotionSettings
	}
	must(decoder.Decode(&header))
	if header.Schema != "chat_history_replay.v1" && header.Schema != "chat_history_replay.v2" {
		must(fmt.Errorf("%s: unrecognized chat replay schema", path))
	}
	var entries []motion.Review
	for {
		var row struct {
			Kind, Action, Outcome string
			Session, Step         int
			Running               bool
			FirstPass             bool `json:"first_pass"`
			After                 *motion.FlowSpec
		}
		err := decoder.Decode(&row)
		if errors.Is(err, io.EOF) {
			return entries
		}
		must(err)
		var entry motion.Review
		name := fmt.Sprintf("Session %d, turn %d", row.Session, row.Step)
		switch row.Kind {
		case "turn":
			if row.Running && row.After != nil {
				entry = motion.ReviewMotionOutput(motion.MotionTarget{Source: "chat", Label: name,
					SpeedPercent: row.After.SpeedPercent, Flow: row.After}, header.Settings)
			}
			entry.Outcome = fmt.Sprintf("First-pass completion %t. Applied action %s.", row.FirstPass, row.Action)
			if header.Schema == "chat_history_replay.v2" {
				if row.Outcome == "" {
					must(fmt.Errorf("%s: missing outcome for %s", path, name))
				}
				entry.Outcome = fmt.Sprintf("Completion %s. Applied action %s.", strings.ReplaceAll(row.Outcome, "_", " "), row.Action)
			}
		case "stop":
			name, entry.Outcome = fmt.Sprintf("Session %d, Stop", row.Session), "Simulator acknowledged Stop."
		default:
			must(fmt.Errorf("%s: unknown replay row %q", path, row.Kind))
		}
		entry.ID, entry.Name = filepath.Base(path)+"/"+name, name
		entry.Group, entry.Model, entry.Request = "llm-output", header.Model, name
		entries = append(entries, entry)
	}
}
