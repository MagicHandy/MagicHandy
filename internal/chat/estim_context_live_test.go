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

// TestEstimFactsLive evaluates the ADR 0063 Coyote facts through the
// production prompt, schema and parser. The Coyote's strength is the user's
// alone, so a strength request must keep the strokes; movement requests must
// still move. Replies are retained for review of what they claim.
func TestEstimFactsLive(t *testing.T) {
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
	type check func(command *MotionCommand, before motion.FlowSpec) bool
	keeps := func(command *MotionCommand, before motion.FlowSpec) bool { return linearUnchanged(command, before) }
	starts := func(command *MotionCommand, _ motion.FlowSpec) bool {
		return command != nil && command.Action == MotionActionStart
	}
	faster := func(command *MotionCommand, before motion.FlowSpec) bool {
		return command != nil && command.Layered != nil && command.Layered.SpeedPercent > before.SpeedPercent
	}
	cases := []struct {
		name, message string
		running       bool
		silent        bool
		check         check
	}{
		{"stronger", "Make the e-stim stronger.", true, false, keeps},
		{"off_keep_strokes", "Turn off the e-stim but keep the strokes exactly as they are.", true, false, keeps},
		{"is_it_on", "Is the e-stim on right now?", true, true, keeps},
		{"weaker", "The shocks are too strong, turn them down.", true, false, keeps},
		{"start", "Start slowly.", false, true, starts},
		{"faster", "Go a little faster.", true, false, faster},
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, tc := range cases {
			name := fmt.Sprintf("%s/%s", mode, tc.name)
			t.Run(name, func(t *testing.T) {
				score := DefaultLayeredScore(45)
				if mode == MotionModeCreativeV2 {
					score = FreshCreativeV2Score(45)
				}
				score.Seed = 20261029
				before := *motion.CloneFlowSpec(&score)
				state := naturalState(&score, mode, limits)
				state.Running = tc.running
				state.Outputs = &OutputContext{Strokers: 1, Coyote: true, Estim: &EstimContext{Follows: "stroke", Connected: true, Silent: tc.silent}}
				caps := Capabilities{Motion: true, MotionMode: mode, SimpleMotion: simple, IntermediateMotion: intermediate, AdvancedMotion: advanced, PreserveConversationText: advanced}
				service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, MaxTokens: naturalMaxTokens(), PromptBudget: budget, ReasoningMode: "off"}
				started, calls := time.Now(), provider.calls
				result, err := service.Complete(t.Context(), Request{Message: tc.message, History: []llm.Message{}}, nil)
				command := result.Response.Motion
				pass := err == nil && !result.Repaired && !result.SemanticFallback && result.Response.Reply != "" && provider.calls-calls == 1 && tc.check(command, before)
				encoded, _ := json.Marshal(command)
				rows = append(rows, map[string]any{
					"case": name, "model": model, "message": tc.message, "silent": tc.silent, "pass": pass, "error": fmt.Sprint(err),
					"reply": result.Response.Reply, "raw_attempts": append([]string(nil), provider.raws[calls:]...), "command": json.RawMessage(encoded),
					"elapsed_ms": time.Since(started).Milliseconds(), "repaired": result.Repaired, "fallback": result.SemanticFallback,
					"seed": os.Getenv("MAGICHANDY_NATURAL_SEED"),
				})
				saveHostedLayeredRows(t, rows)
				t.Logf("pass=%t elapsed_ms=%d reply=%q", pass, time.Since(started).Milliseconds(), result.Response.Reply)
				if !pass {
					t.Errorf("intent failed; retained for review")
				}
			})
		}
	}
}
