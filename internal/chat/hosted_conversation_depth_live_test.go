//go:build liveeval

package chat

import (
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Carry actual accepted scores and replies through successive edits. Isolated
// one-turn cases miss scope drift, stale restrictions and unsupported promises.
// No engine, transport or installed-session mutation is involved.
func TestHostedConversationDepthLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	for repeat := range max(1, repeats) {
		score := DefaultLayeredScore(58)
		score.MinPercent, score.MaxPercent = 0, 100
		score.RangeFloorPercent, score.RangeCeilingPercent = 28, 28
		score.PaceVariationPercent, score.Seed = 0, 914
		score.Layers = []motion.FlowLayer{
			{Axis: "center", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate"},
			{Axis: "pace", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate", LowRatePercent: 25},
		}
		history := []llm.Message{
			{Role: "user", Content: "Alternate quick upper-end strokes with much slower lower-end strokes."},
			{Role: "assistant", Content: "Alternating those regions, with quicker upper-end travel."},
		}
		for _, tc := range conversationDepthCases() {
			t.Run(tc.name+"/"+strconv.Itoa(repeat+1), func(t *testing.T) {
				if tc.stopped {
					history = append(history, llm.Message{Role: "user", Content: "Stop."}, llm.Message{Role: "assistant", Content: "Stopped."})
				}
				before := *motion.CloneFlowSpec(&score)
				plan := motion.NewMotionPlan("before", motion.MotionTarget{Flow: &before, SpeedPercent: before.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
				state := MotionContext{Running: !tc.stopped, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: before.SpeedPercent,
					SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits), CommandedSummary: &plan.Perceptual}
				svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, MaxTokens: 1024, ReasoningMode: "off"}
				started, calls := time.Now(), provider.calls
				result, err := svc.Complete(t.Context(), Request{Message: tc.message, History: history}, nil)
				command := result.Response.Motion
				valid := err == nil && !result.Malformed && !result.Repaired && !result.SemanticFallback && result.Response.Reply != "" && provider.calls-calls == 1
				if err == nil && command != nil && command.Layered != nil {
					score = *motion.CloneFlowSpec(command.Layered)
				}
				intent := tc.check(before, score)
				if tc.question {
					intent = intent && command == nil
				} else if tc.stopped {
					intent = intent && command != nil && command.Action == MotionActionStart
				} else {
					intent = intent && command != nil && command.Action == MotionActionUpdate
				}
				_, compileErr := motion.FlowTarget(score, limits)
				row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, command, limits, started, err, valid && intent && compileErr == nil)
				row["repeat"], row["reply"], row["provider_calls"], row["running"] = repeat+1, result.Response.Reply, provider.calls-calls, !tc.stopped
				row["history"], row["compiled"] = history, compileErr == nil
				if compileErr != nil {
					row["compile_error"] = compileErr.Error()
				}
				rows = append(rows, row)
				saveHostedLayeredRows(t, rows)
				history = append(history, llm.Message{Role: "user", Content: tc.message})
				if err == nil && result.Response.Reply != "" {
					history = append(history, llm.Message{Role: "assistant", Content: result.Response.Reply})
				}
				if row["intent_pass"] != true {
					t.Error("conversation intent failed; raw response and accepted score retained")
				}
			})
		}
	}
}

type conversationDepthCase struct {
	name, message     string
	stopped, question bool
	check             func(motion.FlowSpec, motion.FlowSpec) bool
}

func conversationDepthCases() []conversationDepthCase {
	groups := func(s motion.FlowSpec) bool { return s.Phrase != nil && len(s.Phrase.Strokes) == 2 }
	same := func(a, b motion.FlowSpec) bool { return reflect.DeepEqual(a, b) }
	preservePace := func(a, b motion.FlowSpec) bool {
		return a.SpeedPercent == b.SpeedPercent && a.PaceVariationPercent == b.PaceVariationPercent && samePaceLayerValue(a, b)
	}
	return []conversationDepthCase{
		{name: "replace_regional", message: "Replace that alternation with two short upper-end strokes, then one full-device sweep down to the base and back. Repeat smoothly at the current base speed; no separate slow/fast cycle.", check: func(a, b motion.FlowSpec) bool {
			return groups(b) && b.Phrase.Strokes[0].Count == 2 && b.Phrase.Strokes[1].Count == 1 && b.Phrase.Strokes[0].BottomPercent > 0 &&
				b.Phrase.Strokes[1].BottomPercent == 0 && b.Phrase.Strokes[1].TopPercent == 100 && b.Phrase.Strokes[0].RatePercent == b.Phrase.Strokes[1].RatePercent && a.SpeedPercent == b.SpeedPercent && pacePeriod(&b) == 0
		}},
		{name: "count_only", message: "Make that three upper-end strokes instead of two. Keep the full sweep, both travel rates, and every other setting.", check: func(a, b motion.FlowSpec) bool {
			if !groups(a) || !groups(b) || !preservePace(a, b) {
				return false
			}
			want := *motion.CloneFlowSpec(&a)
			want.Phrase.Strokes[0].Count = 3
			return reflect.DeepEqual(want.Phrase, b.Phrase)
		}},
		{name: "slower_sweep", message: "Make only the full sweep take twice as long in each direction. Leave the upper strokes, counts, base speed and all coverage unchanged.", check: func(a, b motion.FlowSpec) bool {
			if !groups(a) || !groups(b) || !preservePace(a, b) {
				return false
			}
			want := *motion.CloneFlowSpec(&a)
			want.Phrase.Strokes[1].RatePercent /= 2
			return reflect.DeepEqual(want.Phrase, b.Phrase)
		}},
		{name: "explicit_narrow", message: "For just the upper strokes, use exactly 90 to 100. Keep the full sweep, counts and timing unchanged.", check: func(a, b motion.FlowSpec) bool {
			if !groups(a) || !groups(b) || !preservePace(a, b) {
				return false
			}
			want := *motion.CloneFlowSpec(&a)
			want.Phrase.Strokes[0].BottomPercent, want.Phrase.Strokes[0].TopPercent = 90, 100
			return reflect.DeepEqual(want.Phrase, b.Phrase)
		}},
		{name: "base_speed_only", message: "Raise the base speed by seven percentage points. Preserve both group-rate multipliers, counts and endpoints.", check: func(a, b motion.FlowSpec) bool {
			return groups(b) && b.SpeedPercent == a.SpeedPercent+7 && reflect.DeepEqual(a.Phrase, b.Phrase) && samePaceLayerValue(a, b)
		}},
		{name: "replace_coverage", message: "Change the whole motif's coverage: upper strokes from 60 to 80, and the broad sweep from 20 to 80. Keep counts, group rates and base speed unchanged.", check: func(a, b motion.FlowSpec) bool {
			if !groups(a) || !groups(b) || !preservePace(a, b) {
				return false
			}
			want := *motion.CloneFlowSpec(&a)
			want.Phrase.Strokes[0].BottomPercent, want.Phrase.Strokes[0].TopPercent = 60, 80
			want.Phrase.Strokes[1].BottomPercent, want.Phrase.Strokes[1].TopPercent = 20, 80
			return reflect.DeepEqual(want.Phrase, b.Phrase)
		}},
		{name: "release_old_band", message: "Use the full device again for the broad sweep, from 0 to 100. Put the upper strokes between 70 and 100. Keep the counts and all timing as they are.", check: func(a, b motion.FlowSpec) bool {
			if !groups(a) || !groups(b) || !preservePace(a, b) {
				return false
			}
			want := *motion.CloneFlowSpec(&a)
			want.Phrase.Strokes[0].BottomPercent, want.Phrase.Strokes[0].TopPercent = 70, 100
			want.Phrase.Strokes[1].BottomPercent, want.Phrase.Strokes[1].TopPercent = 0, 100
			return reflect.DeepEqual(want.Phrase, b.Phrase)
		}},
		{name: "stopped_question", stopped: true, question: true, message: "How do the two groups differ in travel speed? Just explain, leave it stopped.", check: same},
		{name: "restart_motif", stopped: true, message: "Use that same three-plus-one motif again, unchanged.", check: same},
		{name: "release_counts", message: "Drop the counted motif and its regional timing. Make it flow with irregularly varying stroke lengths and locations across the full device. Keep only the current base speed; choose fresh coverage and variation.", check: func(a, b motion.FlowSpec) bool {
			return b.Phrase == nil && b.MinPercent == 0 && b.MaxPercent == 100 && a.SpeedPercent == b.SpeedPercent && b.RangeFloorPercent < b.RangeCeilingPercent
		}},
	}
}

// A failed turn leaves its user message in history but applies no motion.
// Later partial edits must preserve the actual score, while an explicit retry
// can still ask for the outstanding change. No provider failure is hidden.
func TestHostedUnappliedHistoryLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	for repeat := range max(1, repeats) {
		for _, name := range []string{"failed_slowing", "stale_reply", "explicit_retry", "failed_range"} {
			t.Run(name+"/"+strconv.Itoa(repeat+1), func(t *testing.T) {
				before := DefaultLayeredScore(58)
				before.Seed, before.PaceVariationPercent = 914, 0
				before.MinPercent, before.MaxPercent = 0, 100
				before.RangeFloorPercent, before.RangeCeilingPercent, before.AnchorPercent = 30, 100, 100
				before.Layers = nil
				before.Phrase = &motion.FlowPhrase{Strokes: []motion.PhraseStroke{{BottomPercent: 70, TopPercent: 100, Count: 3, RatePercent: 100}, {BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100}}}
				history := []llm.Message{
					{Role: "user", Content: "Use three short upper-end strokes and one full sweep, at equal travel rates."},
					{Role: "assistant", Content: "Repeating that three-plus-one motif."},
					{Role: "user", Content: "Make only the full sweep take twice as long, with everything else unchanged."},
				}
				message := "Raise only the base speed by seven percentage points. Keep the current counts, endpoints and group-rate multipliers unchanged."
				want := *motion.CloneFlowSpec(&before)
				want.SpeedPercent += 7
				if name == "stale_reply" {
					history = append(history, llm.Message{Role: "assistant", Content: "The full sweep is now slower, at half its old travel rate."})
				}
				if name == "explicit_retry" {
					message = "That last slowing request failed. Please try it again now, keeping everything else unchanged."
					want.SpeedPercent, want.Phrase.Strokes[1].RatePercent = before.SpeedPercent, 50
				}
				if name == "failed_range" {
					before.Phrase = nil
					before.MinPercent, before.MaxPercent = 20, 70
					before.RangeFloorPercent, before.RangeCeilingPercent = 50, 50
					history = []llm.Message{{Role: "user", Content: "Change to full-device sweeps from 0 to 100."}}
					message = "Raise only the base speed by seven points, preserving the current coverage and timing."
					want = *motion.CloneFlowSpec(&before)
					want.SpeedPercent += 7
				}
				state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: before.SpeedPercent, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
				svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, MaxTokens: 1024, ReasoningMode: "off"}
				started, calls := time.Now(), provider.calls
				result, err := svc.Complete(t.Context(), Request{Message: message, History: history}, nil)
				command := result.Response.Motion
				pass := err == nil && !result.Repaired && !result.SemanticFallback && provider.calls-calls == 1 && result.Response.Reply != "" && command != nil && command.Layered != nil && reflect.DeepEqual(*command.Layered, want)
				row := hostedLayeredLiveRow(name, model, message, result.Raw, before, command, limits, started, err, pass)
				row["history"], row["reply"], row["repeat"], row["provider_calls"] = history, result.Response.Reply, repeat+1, provider.calls-calls
				rows = append(rows, row)
				saveHostedLayeredRows(t, rows)
				if !pass {
					t.Error("current-state preservation or explicit retry failed; response retained")
				}
			})
		}
	}
}
