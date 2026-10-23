//go:build liveeval && magichandy_labs

package httpapi

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type lifecycleCaptureRow struct {
	Model, Method, Message, Raw, Error string
	Name                               string              `json:"recipe_name"`
	Command                            *chat.MotionCommand `json:"motion_command"`
	IntentPass                         bool                `json:"intent_pass"`
	Valid                              bool
	Limits                             config.MotionSettings
}

// Recompile captured commands through production chat target mapping and the
// shared sampler. It never starts an engine or invokes a provider or transport.
// The resulting report contains real Review objects for motion-atlas, including
// unchanged and rejected turns instead of unrelated default Flow scores.
func TestHostedLifecycleCompiledCapture(t *testing.T) {
	path := os.Getenv("MAGICHANDY_LIFECYCLE_CAPTURE")
	if path == "" {
		t.Skip("set MAGICHANDY_LIFECYCLE_CAPTURE to a hosted lifecycle evaluation report")
	}
	data, err := os.ReadFile(path) // #nosec G703 -- explicit development-only capture input
	if err != nil {
		t.Fatal(err)
	}
	var report struct{ Turns []lifecycleCaptureRow }
	if err := json.Unmarshal(data, &report); err != nil || len(report.Turns) == 0 {
		t.Fatalf("invalid lifecycle capture: %v", err)
	}
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(s.Close)
	turns := []map[string]any{}
	for _, row := range report.Turns {
		review := compileLifecycleCapture(t, s, row)
		review.Name = row.Method + "/" + row.Name
		turns = append(turns, map[string]any{"request": row.Message, "response": row.Raw, "motion": review, "intent_pass": row.IntentPass})
	}
	encoded, err := json.MarshalIndent(map[string]any{"model": report.Turns[0].Model, "turns": turns}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".compiled.json", encoded, 0600); err != nil { // #nosec G703 -- explicit development-only capture output
		t.Fatal(err)
	}
	if len(fake.Commands()) != 0 || s.currentMotionEngine() != nil {
		t.Fatal("capture compilation started an engine or used a transport")
	}
}

func compileLifecycleCapture(t *testing.T, s *Server, row lifecycleCaptureRow) motion.Review {
	t.Helper()
	if !row.Valid {
		return motion.Review{Error: row.Error, Outcome: "Rejected response retained without motion"}
	}
	if row.Command == nil || row.Command.Action == chat.MotionActionNone {
		return motion.Review{Error: "Motion intentionally unchanged", Outcome: "No moving output"}
	}
	target, err := s.chatMotionTarget(row.Command, motion.ActiveMotionState{})
	if err != nil {
		t.Errorf("%s/%s target mapping: %v", row.Method, row.Name, err)
		return motion.Review{Error: err.Error()}
	}
	if row.Command.PatternID != "" && string(target.PatternID) != row.Command.PatternID {
		t.Errorf("%s/%s selected a pattern absent from the production catalog", row.Method, row.Name)
		return motion.Review{Error: "Selected pattern absent from the production catalog"}
	}
	review := motion.ReviewMotionOutput(target, row.Limits)
	if review.Error != "" {
		t.Errorf("%s/%s compilation: %s", row.Method, row.Name, review.Error)
	}
	return review
}
