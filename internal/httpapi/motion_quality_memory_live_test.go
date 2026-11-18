//go:build liveeval && magichandy_labs

package httpapi

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const qualityClarification = "Which endpoints and speed should I use?"

var qualityNumerals = regexp.MustCompile(`\b[0-9]+\b`)

func qualityMemoryTurn(t *testing.T, s *Server, p *qualityProvider, scenario string, tc qualityTurn, pairs int, mirror bool, profile *qualityDefaultProfile) qualityRow {
	t.Helper()
	if scenario == "restated" {
		if tc.id == "new_definition" {
			qualityFillHistory(t, s, pairs)
		}
		row := qualityRunTurn(t, s, p, scenario, tc)
		if tc.id == "read_again" || tc.id == "apply" {
			row.checkDefinitionVisible(true)
		}
		if tc.id == "read_again" {
			_, lo, hi, speed := qualityRestatedCue(mirror)
			row.checkReadback(lo, hi, speed)
		}
		row.IntentPass = len(row.Failures) == 0
		return row
	}
	if tc.id != "recall" {
		return qualityRunTurn(t, s, p, scenario, tc)
	}
	qualityFillHistory(t, s, pairs)
	name, lo, hi, speed := qualityCue(mirror)
	if scenario == "long_unrelated" {
		tc.request = "Raise just the current overall speed by five percentage points, preserving all other controls."
		tc.check = qualitySpeedDelta(5)
		return qualityRunTurn(t, s, p, scenario, tc)
	}
	// The corrected user definition is the third of the six real messages
	// before the filler. Its distance from the newest prior message is 2N+4.
	missing := profile != nil && 2*pairs+4 > profile.messages
	switch {
	case missing:
		tc.action, tc.check = "none", nil
		tc.request = fmt.Sprintf("Switch to the corrected %s if you have its exact settings. If its definition is missing, keep the current pattern and ask me with these words: %s", name, qualityClarification)
		if scenario == "recall_readback" {
			tc.request = fmt.Sprintf("What are the corrected endpoints and speed of %s? If its definition is missing, ask me with these words: %s Leave the current motion alone.", name, qualityClarification)
		}
	case scenario == "recall_outside":
		tc.action, tc.check = "none", nil
	case scenario == "recall_readback":
		tc.action, tc.check = "none", nil
		tc.request = fmt.Sprintf("What does %s mean in its corrected form? Give the lower endpoint, upper endpoint and overall speed as three numerals, in that order. Answer only and leave the current movement alone.", name)
	}
	row := qualityRunTurn(t, s, p, scenario, tc)
	if profile != nil {
		row.checkDefinitionVisible(!missing)
	}
	if missing && !strings.Contains(strings.ToLower(row.Events.Reply), strings.ToLower(strings.TrimSuffix(qualityClarification, "?"))) {
		row.Failures = append(row.Failures, "requested clarification of missing settings")
	}
	if scenario == "recall_readback" && !missing {
		row.checkReadback(lo, hi, speed)
	}
	row.IntentPass = len(row.Failures) == 0
	return row
}

func (r *qualityRow) checkDefinitionVisible(want bool) {
	if len(r.Attempts) == 0 || r.Attempts[0].CorrectionPresent != want {
		r.Failures = append(r.Failures, "expected correction visibility at default history window")
	}
}

func (r *qualityRow) checkReadback(lo, hi, speed int) {
	want := []string{strconv.Itoa(lo), strconv.Itoa(hi), strconv.Itoa(speed)}
	if !slices.Equal(qualityNumerals.FindAllString(r.Events.Reply, -1), want) {
		r.Failures = append(r.Failures, "spoken corrected endpoints and speed")
	}
}

func qualityRestatedCue(mirror bool) (string, int, int, int) {
	if mirror {
		return "Willow", 18, 61, 35
	}
	return "Cedar", 34, 79, 48
}

// After 112 pairs of unrelated dialogue the human restates the definition.
// A model must distinguish fresh information from a reference it can no longer see.
func qualityRestatedCases(mirror bool) ([]qualityTurn, string) {
	name, lo, hi, speed := qualityRestatedCue(mirror)
	correction := fmt.Sprintf("One note for the next time I ask for %s: each full pass should run from %d up to %d at overall speed %d. This replaces my old note. Just acknowledge it and keep the current movement exactly as it is.", name, lo, hi, speed)
	return []qualityTurn{
		{"old_definition", fmt.Sprintf("Please remember the name %s for later. It means full travel from 15 to 90 at speed 55. For now just note it and leave the simulator idle.", name), "none", nil},
		{"different", "Start moving along the whole slider now, touching both ends at overall speed 42.", "start", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.SpeedPercent == 42 && qualityBounds(r, 0, 100)
		}},
		{"new_definition", correction, "none", nil},
		{"read_again", fmt.Sprintf("Read the updated note for %s back to me. Give just its low mark, high mark and speed in that order as numbers. Keep the current motion playing unchanged.", name), "none", nil},
		{"apply", fmt.Sprintf("I'd like to use that amended %s now. Make every pass touch both marks.", name), "update", func(_, a *motion.FlowSpec, r *motion.Review) bool {
			return a != nil && a.SpeedPercent == speed && qualityBounds(r, lo, hi) && r.Summary.MeanStrokePercent >= float64(hi-lo-3)
		}},
		{"follow_up", "Back off only the overall pace by four percentage points.", "update", qualitySpeedDelta(-4)},
		{"stop", "Stop.", "stop", nil},
	}, correction
}
