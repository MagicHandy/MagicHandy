//go:build liveeval

package chat

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// The person's deadline is relative to their message, not session start or the
// latest check-in. Every result is retained and sampled through the production
// envelope. This does not contact a transport or classify language in runtime.
func TestTempoTimedRequestsLive(t *testing.T) {
	if os.Getenv("MAGICHANDY_TEMPO_TIMED_EVAL") != "1" {
		t.Skip("opt-in timed tempo review")
	}
	provider, model, hosted := naturalLiveProvider(t)
	level := os.Getenv("MAGICHANDY_NATURAL_LEVEL")
	advanced := hosted || level == config.ModelCapabilityAdvanced
	intermediate := !advanced && level == config.ModelCapabilityIntermediate
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: advanced,
		IntermediateMotion: intermediate, SimpleMotion: !advanced && !intermediate, PreserveConversationText: hosted,
		AutoTiming: advanced, AutoTimingFloor: 10}
	request := "Over the next two minutes, gradually build from this gentle 20% tempo to about 80%, reaching that level at the end. Then keep flowing around that pace; don't stop."
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name     string
		now, due int
		start    float64
		messages []string
		at       []int
		bar      bool
		want     float64
	}{
		{"mid_session_target", 150, 240, 20, []string{request}, []int{120}, false, 80},
		{"later_praise_does_not_reset", 190, 240, 45, []string{request, "That feels good, thanks."}, []int{120, 180}, false, 80},
		{"revised_deadline", 200, 310, 45, []string{request, "Actually, take another two minutes from now to reach that 80%, then keep flowing there."}, []int{120, 190}, false, 80},
		{"elapsed_gentle_interval", 200, 215, 20, []string{"Stay gentle at 20% for the next minute, then return to about 60% and keep flowing."}, []int{120}, false, 60},
		{"full_bar_new_gentle_direction", 480, 485, 80, []string{"Forget the buildup target; keep the tempo gentle at about 20% from here and continue."}, []int{475}, true, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := motion.TempoSpec{Mode: motion.TempoModeSteady, LevelPercent: int(tc.start)}
			state := MotionContext{Running: true, Autopilot: true, Outputs: &OutputContext{Strokers: 1, PaceOnly: 1,
				Tempo: &TempoContext{Leads: true, Ready: true, Running: true, Current: &current, Level: tc.start}}}
			var history []llm.Message
			var historyAt []time.Time
			for i, message := range tc.messages {
				history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: "Understood."})
				historyAt = append(historyAt, time.Unix(int64(tc.at[i]), 0), time.Unix(int64(tc.at[i]), 0))
			}
			sample, sampleErr := motion.TempoReviewSampler(current, int64(tc.now)*1000, tc.start)
			if sampleErr != nil {
				t.Fatal(sampleErr)
			}
			pass, started, firstRow := true, time.Now(), len(rows)
			next, previous := tc.now, tc.now
			var err error
			for turn := 0; next < tc.due && turn < 16; turn++ {
				var samples [][2]float64
				for at := previous; at < next; at++ {
					samples = append(samples, [2]float64{float64(at), sample(int64(at) * 1000)})
				}
				previous = next
				state.Outputs.Tempo.Level = sample(int64(next) * 1000)
				state.Outputs.Tempo.Current = motion.CloneTempoSpec(&current)
				state.UserRequests, state.UserRequestSecondsAgo = nil, nil
				for i, message := range tc.messages {
					state.UserRequests = append(state.UserRequests, message)
					state.UserRequestSecondsAgo = append(state.UserRequestSecondsAgo, next-tc.at[i])
				}
				marked := history
				if !advanced {
					marked = PlanningHistory(history, historyAt, time.Unix(int64(next), 0))
				}
				age := state.UserRequestSecondsAgo[len(state.UserRequestSecondsAgo)-1]
				context := AutopilotContext{TempoOnly: true, Hosted: advanced, Intermediate: intermediate, MotionMode: MotionModeLayered,
					SessionTracking: true, SessionSeconds: next, LastHumanSecondsAgo: &age, MotionChangeLevel: 4,
					MotionMinSeconds: 20, MotionMaxSeconds: 60, AutoTiming: advanced, AutoTimingFloor: 10,
					ArcEnabled: tc.bar, ArcPercent: 100, ArcMinutes: 8, SegmentIndex: turn}
				message := AutopilotMotionMessage(context)
				before, callStart := provider.calls, time.Now()
				response, callErr := (AutopilotService{Provider: provider, Model: model, Capabilities: caps, MotionContext: &state,
					ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}).Complete(t.Context(), AutopilotKindMotion, Request{Message: message, History: marked})
				err = callErr
				if response.Motion != nil && response.Motion.Tempo != nil && err == nil {
					current = response.Motion.Tempo.Clone()
					sample, sampleErr = motion.TempoReviewSampler(current, int64(next)*1000, state.Outputs.Tempo.Level)
					if sampleErr != nil {
						t.Fatal(sampleErr)
					}
				}
				pass = pass && err == nil && provider.calls-before == 1
				row := map[string]any{"name": tc.name, "model": model, "level": level, "prompt": message, "intent": response.Intent,
					"command": response.Motion, "elapsed_s": next, "next_check_seconds": response.NextCheckSeconds,
					"raw_attempts": append([]string(nil), provider.raws[before:]...), "elapsed_ms": time.Since(callStart).Milliseconds(),
					"deadline": tc.due, "samples": samples, "start_level": state.Outputs.Tempo.Level}
				if err != nil {
					row["error"] = err.Error()
				}
				rows = append(rows, row)
				saveHostedLayeredRows(t, rows)
				// The scheduler tests cover randomized bounds. Here use each
				// category's measured mean, or the advanced planner's horizon,
				// so comparisons exercise repeat decisions reproducibly.
				delay := 40
				if response.Next == AutopilotTimingSoon {
					delay = 27
				}
				if response.Next == AutopilotTimingLater {
					delay = 53
				}
				if advanced && response.NextCheckSeconds > 0 {
					delay = min(300, max(10, response.NextCheckSeconds))
				}
				next += delay
			}
			actual := sample(int64(tc.due) * 1000)
			var tail [][2]float64
			for at := previous; at <= tc.due+15; at++ {
				tail = append(tail, [2]float64{float64(at), sample(int64(at) * 1000)})
			}
			pass = pass && math.Abs(actual-tc.want) <= 8
			// Preserve deadline/goal grading on the last row; preceding rows
			// are decisions, not independent successes.
			last := rows[len(rows)-1]
			last["tail_samples"], last["level_at_deadline"], last["intent_pass"] = tail, actual, pass
			last["decision_count"] = len(rows) - firstRow
			saveHostedLayeredRows(t, rows)
			elapsed := time.Since(started).Milliseconds()
			t.Logf("intent=%v deadline=%ds level=%.1f decisions=%d ms=%d error=%v", pass, tc.due, actual, len(rows)-firstRow, elapsed, err)
			if !pass {
				t.Error("timed request mismatch; raw and actual engine samples retained")
			}
		})
	}
}
