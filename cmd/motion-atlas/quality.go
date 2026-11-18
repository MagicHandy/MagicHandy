//go:build magichandy_labs

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Quality captures contain the actual state after each request. In particular,
// a rejected edit can leave a previous trajectory running, and an ordinary
// conversational turn can correctly leave an idle device idle.
func qualityEntries(data json.RawMessage, model, path string) []motion.Review {
	var turns []struct {
		Scenario, ID, Request string
		Expected              string `json:"expected_action"`
		Events                struct{ Action, Reply string }
		Attempts              []struct{ Raw string }
		Failures              []string
		IntentPass            bool `json:"intent_pass"`
		Motion                *motion.Review
	}
	must(json.Unmarshal(data, &turns))
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries := make([]motion.Review, 0, len(turns))
	for _, turn := range turns {
		var entry motion.Review
		if turn.Motion != nil {
			entry = *turn.Motion
		}
		entry.ID = name + "/" + turn.Scenario + "/" + turn.ID
		entry.Name = fmt.Sprintf("%s · %s · %s", name, turn.Scenario, turn.ID)
		entry.Model, entry.Request, entry.Group = model, turn.Request, "llm-output"
		outcome := "Intent pass"
		if !turn.IntentPass {
			outcome = "Intent failure: " + strings.Join(turn.Failures, ", ")
		}
		entry.Outcome = fmt.Sprintf("%s. Expected %s, observed %s.", outcome, turn.Expected, turn.Events.Action)
		raw := make([]string, 0, len(turn.Attempts))
		for _, attempt := range turn.Attempts {
			raw = append(raw, attempt.Raw)
		}
		entry.Raw = strings.Join(raw, "\n\n")
		entries = append(entries, entry)
	}
	return entries
}
