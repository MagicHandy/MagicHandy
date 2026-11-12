//go:build liveeval

package chat

import (
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestTempoLiveContinuationAndSpeech(t *testing.T) {
	if os.Getenv("MAGICHANDY_TEMPO_EVAL") != "1" {
		t.Skip("opt-in native tempo evaluation")
	}
	provider, model, hosted := naturalLiveProvider(t)
	level := os.Getenv("MAGICHANDY_NATURAL_LEVEL")
	advanced := hosted || level == config.ModelCapabilityAdvanced
	intermediate := !advanced && level == config.ModelCapabilityIntermediate
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: advanced,
		IntermediateMotion: intermediate, SimpleMotion: !advanced && !intermediate, PreserveConversationText: hosted}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name, message                    string
		speech, authority, quiet, resume bool
		want                             int
	}{
		{name: "continue_after_stop", message: "Continue exactly where we left off with the same Wandering routine. Leave the air alone.", authority: true, resume: true},
		{name: "question_after_stop", message: "What was that last routine? Tell me without starting it.", authority: true},
		{name: "new_plan_after_stop", message: "Instead of the old routine, start steady at 55 percent now.", authority: true, want: 55},
		{name: "quiet_checkin", message: "The person asked for quiet while this phrase unfolds. There is no new question or change to report. Keep the current tempo unchanged and stay quiet.", speech: true, authority: true, quiet: true},
		{name: "spoken_tempo", message: "The person just asked you to ease the tempo to a steady 25 percent and briefly explain the change. Fulfill that request now and leave the air unchanged.", speech: true, authority: true, want: 25},
		{name: "speech_without_authority", message: "The person asks how a wave differs from a pulse. Explain without changing the current motion.", speech: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := tempoTurnContext(tc.speech, false)
			saved := &motion.TempoSpec{Mode: motion.TempoModeRoutine, Routine: "wandering", Seed: 12345}
			if tc.speech {
				state.Outputs.Tempo.Current = saved
				state.Outputs.Tempo.Level = 40
			} else {
				state.Outputs.Tempo.Previous = saved
			}
			request := Request{Message: tc.message, History: []llm.Message{{Role: "user", Content: "Play Wandering."}, {Role: "assistant", Content: "Wandering is playing."}}}
			before, started := provider.calls, time.Now()
			var command *MotionCommand
			var reply string
			var err error
			pass := true
			if tc.speech {
				autoCaps := caps
				autoCaps.Motion = tc.authority
				result, resultErr := (AutopilotService{Provider: provider, Model: model, Capabilities: autoCaps, MotionContext: state, ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}).Complete(t.Context(), AutopilotKindSpeech, request)
				command, reply, err = result.Motion, result.Reply, resultErr
				pass = (tc.quiet && reply == "" && result.Next == AutopilotTimingLater) || (!tc.quiet && reply != "")
			} else {
				result, resultErr := (Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: state, ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}).Complete(t.Context(), request, nil)
				command, reply, err = result.Response.Motion, result.Response.Reply, resultErr
				pass = reply != "" && !result.Repaired && !result.SemanticFallback
			}
			if tc.resume {
				pass = pass && command != nil && command.Tempo != nil && command.Tempo.Equal(*saved)
			} else if tc.want > 0 {
				pass = pass && tempoLiveIntent(command, "steady", tc.want)
			} else {
				pass = pass && command == nil
			}
			pass = pass && err == nil && provider.calls-before == 1
			row := map[string]any{"name": tc.name, "model": model, "level": level, "kind": "continuation_or_speech", "message": tc.message, "context": state.Outputs.Tempo, "raw_attempts": append([]string(nil), provider.raws[before:]...), "reply": reply, "intent_pass": pass, "elapsed_ms": time.Since(started).Milliseconds(), "calls": provider.calls - before, "command": command, "start_level": state.Outputs.Tempo.Level}
			if err != nil {
				row["error"] = err.Error()
			}
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("intent=%v calls=%d elapsed_ms=%v error=%v", pass, provider.calls-before, row["elapsed_ms"], err)
			if !pass {
				t.Error("native continuation/check-in mismatch; retained in capture")
			}
		})
	}
}
