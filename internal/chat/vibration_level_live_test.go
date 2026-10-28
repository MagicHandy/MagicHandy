//go:build liveeval

package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// TestVibrationLevelLive evaluates the ADR 0061 vocabulary through the
// production prompt, schema and parser. Simple and Intermediate are judged by
// vibration_level; Advanced by its envelope in percent of Max. Intent checks
// read what actually changed, including preservation of the linear score.
func TestVibrationLevelLive(t *testing.T) {
	provider, model, advanced := naturalLiveProvider(t)
	simple, intermediate := naturalSimple(), naturalIntermediate()
	if !advanced && !simple && !intermediate {
		t.Skip("set MAGICHANDY_NATURAL_LEVEL (with MAGICHANDY_SIMPLE_PRODUCTION=1 for Simple), or use a hosted model")
	}
	limits := config.DefaultSettings().Motion
	budget := naturalPromptBudget()
	if simple {
		budget = PromptBudgetSettings{ContextSize: config.MinimumLocalContext(config.ModelCapabilitySimple), MaxOutputTokens: naturalMaxTokens()}
	}
	type check func(command *MotionCommand, before motion.FlowSpec, level int) bool
	stronger := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got > level && linearUnchanged(command, before)
	}
	gentler := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got < level && got > 0 && linearUnchanged(command, before)
	}
	off := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got == 0 && linearUnchanged(command, before)
	}
	full := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got >= 95 && linearUnchanged(command, before)
	}
	slowerStronger := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got > level && command.Layered != nil && command.Layered.SpeedPercent < before.SpeedPercent
	}
	fasterOnly := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return (!ok || got == level) && command != nil && command.Layered != nil && command.Layered.SpeedPercent > before.SpeedPercent
	}
	unchanged := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return (!ok || got == level) && linearUnchanged(command, before)
	}
	// Starting requires a chosen level (ADR 0061); before one, vibration
	// follows The Handy at the full Max.
	softStart := func(command *MotionCommand, _ motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got > 0 && got <= 50 && command.Action == MotionActionStart
	}
	plainStart := func(command *MotionCommand, _ motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got > 0 && command.Action == MotionActionStart
	}
	strongStart := func(command *MotionCommand, _ motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		// This request names vibration, not strokes. Advanced may start an
		// independent envelope with action none, without moving The Handy.
		independentStart := advanced && command != nil && command.Action == MotionActionNone && command.Vibration != nil &&
			command.Vibration.Mode != "follow" && command.Vibration.Mode != "off"
		return ok && got >= 70 && (command.Action == MotionActionStart || independentStart)
	}
	littleMore := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got > level && got <= level+40 && linearUnchanged(command, before)
	}
	backOn := func(command *MotionCommand, before motion.FlowSpec, level int) bool {
		got, ok := chosenLevel(command, advanced, level)
		return ok && got > 0 && linearUnchanged(command, before)
	}
	startHistory := []llm.Message{{Role: "user", Content: "Start slowly, with gentle vibration."}, {Role: "assistant", Content: "Starting a slow, gentle motion with low vibration."}}
	cases := []struct {
		name, message string
		running       bool
		level         int
		check         check
		history       []llm.Message
	}{
		{"stronger", "Make the vibration stronger.", true, 50, stronger, nil},
		{"gentler", "Make the vibration a bit gentler.", true, 70, gentler, nil},
		{"off_keep_strokes", "Turn off the vibration but keep the strokes exactly as they are.", true, 60, off, nil},
		{"full", "Vibrate as strong as I allow.", true, 40, full, nil},
		{"slower_stronger", "Slow the strokes down but make the vibration stronger.", true, 40, slowerStronger, nil},
		{"faster_only", "Go a little faster.", true, 50, fasterOnly, nil},
		{"question", "How strong is the vibration right now?", true, 50, unchanged, nil},
		{"soft_start", "Start slowly, with gentle vibration.", false, 100, softStart, nil},
		{"plain_start", "Start.", false, 100, plainStart, nil},
		{"strong_start", "Start with strong vibration.", false, 100, strongStart, nil},
		{"little_more", "A little more vibration, please.", true, 30, littleMore, nil},
		{"back_on", "Turn the vibration back on.", true, 0, backOn, nil},
		// The review app's conversation: a slow start, then stronger vibration.
		{"stronger_after_start", "Make the vibration stronger.", true, 30, stronger, startHistory},
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, tc := range cases {
			name := fmt.Sprintf("%s/%s", mode, tc.name)
			t.Run(name, func(t *testing.T) {
				score := DefaultLayeredScore(45)
				if tc.history != nil {
					score = DefaultLayeredScore(20)
				}
				if mode == MotionModeCreativeV2 {
					score = FreshCreativeV2Score(45)
				}
				score.Seed = 20261028
				before := *motion.CloneFlowSpec(&score)
				state := naturalState(&score, mode, limits)
				state.Running = tc.running
				// The baseline is alpha.76's local contract: no level vocabulary.
				baseline := os.Getenv("MAGICHANDY_VIBRATION_LEVEL_BASELINE") == "1"
				vibrator := &VibrationContext{Independent: advanced, LevelControl: !advanced && !baseline, LevelPercent: tc.level, Ready: true, Follow: "speed", Motors: 1}
				// Before a start no level was chosen, so vibration follows The
				// Handy, as the backend reports it.
				vibrator.LevelFollowsHandy = !tc.running
				if tc.level == 0 {
					// Turned off from 60, as the backend reports after an off.
					vibrator.PreviousLevelPercent = 60
				}
				if advanced && tc.level == 0 {
					vibrator.LevelPercent, vibrator.Current = 50, &motion.VibrationSpec{Mode: "off"}
				}
				state.Outputs = &OutputContext{Strokers: 1, Vibrator: vibrator}
				caps := Capabilities{Motion: true, MotionMode: mode, SimpleMotion: simple, IntermediateMotion: intermediate, AdvancedMotion: advanced, PreserveConversationText: advanced}
				service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, MaxTokens: naturalMaxTokens(), PromptBudget: budget, ReasoningMode: "off"}
				started, calls := time.Now(), provider.calls
				history := tc.history
				if history == nil {
					history = []llm.Message{}
				}
				result, err := service.Complete(t.Context(), Request{Message: tc.message, History: history}, nil)
				command := result.Response.Motion
				pass := err == nil && !result.Repaired && !result.SemanticFallback && result.Response.Reply != "" && provider.calls-calls == 1 && tc.check(command, before, vibrator.LevelPercent)
				got, chosen := chosenLevel(command, advanced, vibrator.LevelPercent)
				encoded, _ := json.Marshal(command)
				rows = append(rows, map[string]any{
					"before": before, "limits": limits, "running_before": tc.running,
					"case": name, "model": model, "message": tc.message, "level_before": tc.level, "level_chosen": got, "level_set": chosen,
					"pass": pass, "error": fmt.Sprint(err), "reply": result.Response.Reply, "raw_attempts": append([]string(nil), provider.raws[calls:]...),
					"command": json.RawMessage(encoded), "elapsed_ms": time.Since(started).Milliseconds(), "repaired": result.Repaired, "fallback": result.SemanticFallback,
					"seed": os.Getenv("MAGICHANDY_NATURAL_SEED"),
				})
				saveHostedLayeredRows(t, rows)
				t.Logf("pass=%t level %d->%d(set=%t) elapsed_ms=%d", pass, tc.level, got, chosen, time.Since(started).Milliseconds())
				if !pass {
					t.Errorf("intent failed; retained for review")
				}
			})
		}
	}
}

// chosenLevel reads the level a reply set: vibration_level for the local tiers,
// the envelope's strength for Advanced (0 for off).
func chosenLevel(command *MotionCommand, advanced bool, retainedLevel int) (int, bool) {
	if command == nil {
		return 0, false
	}
	if !advanced {
		if command.VibrationLevel == nil {
			return 0, false
		}
		return *command.VibrationLevel, true
	}
	if command.Vibration == nil {
		return 0, false
	}
	if command.Vibration.Mode == "off" {
		return 0, true
	}
	// All-zero follow is the legacy "keep the owner's level" command, not
	// silence. Judge it against the backend-reported retained level just as the
	// transport does; explicit off/steady-zero remain zero.
	if command.Vibration.Mode == "follow" && command.Vibration.MinPercent == 0 && command.Vibration.MaxPercent == 0 {
		return retainedLevel, true
	}
	return command.Vibration.MaxPercent, true
}

func TestChosenVibrationLevelUsesFollowSemantics(t *testing.T) {
	for _, tc := range []struct {
		mode                     string
		min, max, retained, want int
	}{
		{"follow", 0, 0, 70, 70}, {"follow", 0, 0, 0, 0},
		{"follow", 10, 60, 70, 60}, {"off", 0, 0, 70, 0},
		{"steady", 0, 0, 70, 0}, {"steady", 30, 30, 70, 30},
	} {
		command := &MotionCommand{Vibration: &motion.VibrationSpec{Mode: tc.mode, MinPercent: tc.min, MaxPercent: tc.max}}
		got, set := chosenLevel(command, true, tc.retained)
		if !set || got != tc.want {
			t.Fatalf("%+v: got %d (set %t)", tc, got, set)
		}
	}
}

func linearUnchanged(command *MotionCommand, before motion.FlowSpec) bool {
	if command == nil || command.Action == "" || command.Action == MotionActionNone || command.Layered == nil {
		return true
	}
	after, _ := json.Marshal(command.Layered)
	original, _ := json.Marshal(before)
	return string(after) == string(original)
}
