//go:build liveeval

package chat

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const rangeEvaluationGuide = `Preserve the dimensions not requested in a conversational follow-up. Resolve a brief follow-up from the immediately preceding human request and the current score; do not interpret a request for more pace as permission for new geometry. This does not make earlier requests permanent when a person later changes direction. Outer endpoints are permission bounds, not proof of actual coverage: anchoring and width/center layers determine which endpoints are reached. If a person asks to reach the base, choose geometry that actually visits that endpoint. Upper-end coverage should be a meaningful stroke region chosen for the request, not automatically the narrowest allowed band or a named geometry's example width. Use manual composition for custom coverage and preserve an explicitly requested narrow width. Keep pace independent. Counted rhythmic requests need the stated counts and order; do not describe a loose alternating envelope as an exact counted sequence.`

type rangeEvaluationProvider struct {
	llm.Provider
	variant string
}

func (p rangeEvaluationProvider) StreamChat(ctx context.Context, r llm.ChatRequest, emit func(string) error) (string, error) {
	if p.variant == "guidance" {
		r.Messages = slices.Clone(r.Messages)
		for i := range r.Messages {
			if r.Messages[i].Role == "system" {
				r.Messages[i].Content += "\n" + rangeEvaluationGuide
				break
			}
		}
	}
	return p.Provider.StreamChat(ctx, r, emit)
}

// Compare actual compiled proposals, retaining failures and loose approximations.
// This fixture has no transport or playback engine and never touches hardware.
func TestHostedLayeredRangeComparison(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	cases := []struct {
		name, request string
		history       []llm.Message
	}{
		{"more_pace", "More.", []llm.Message{{Role: "user", Content: "Increase the overall pace and the lower-end speed."}, {Role: "assistant", Content: "Increasing the overall pace and making the lower phase faster."}}},
		{"base_correction", "You aren't going all the way down to the base. Fix the reach and keep the pace.", nil},
		{"double_tip", "Double stroke the tip, then bounce off the bottom and all the way up and repeat.", nil},
		{"triple_base", "Three short strokes at the base, then one complete sweep up to the tip and back. Repeat that flowing sequence at the current pace.", nil},
		{"tip_coverage", "Work the tip with a flowing rhythm and meaningful coverage, keeping the pace.", nil},
		{"narrow_explicit", "Use exactly 90 to 100 for every stroke. Keep the current pace.", nil},
		{"mixed_regions", "Repeat two short strokes at the tip, then two at the base, then one full sweep of the device, in that order. Keep the current base speed.", nil},
		{"slower_sweep", "Repeat two tip strokes followed by one full sweep to the base and back. Make the full sweep slower than the tip work without raising the current base speed.", nil},
		{"slower_base_sweep", "Repeat three short strokes at the base, then one full sweep to the tip and back. Make both directions of that full sweep slower than the base work without raising the current base speed.", nil},
		{"restricted_phrase", "Keep all travel within 20 to 85. Repeat two shorter strokes near the upper end of that band, then one sweep covering that entire restricted band. Keep the pace.", nil},
		{"uncounted", "Mix full sweeps with some shorter upper strokes and let it develop naturally, without a counted sequence. Keep the current pace.", nil},
		{"question", "What does the range layer do? Only explain, leave motion unchanged.", nil},
	}
	variants := strings.Split(os.Getenv("MAGICHANDY_RANGE_VARIANTS"), ",")
	if variants[0] == "" {
		variants = []string{"baseline", "guidance"}
	}
	for _, variant := range variants {
		repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
		for repeat := range max(1, repeats) {
			for _, tc := range cases {
				t.Run(variant+"/"+tc.name+"/"+strconv.Itoa(repeat+1), func(t *testing.T) {
					before := DefaultLayeredScore(32)
					before.MinPercent, before.MaxPercent, before.Seed = 0, 100, 841
					_ = applyLayeredGeometry(&before, "alternate_ends")
					state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: 32, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
					if os.Getenv("MAGICHANDY_RANGE_SUMMARY") == "1" {
						plan := motion.NewMotionPlan("review", motion.MotionTarget{Flow: &before, SpeedPercent: before.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
						state.CommandedSummary = &plan.Perceptual
					}
					caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
					svc := Service{Provider: rangeEvaluationProvider{Provider: provider, variant: variant}, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 2048}
					started, calls := time.Now(), provider.calls
					result, err := svc.Complete(t.Context(), Request{Message: tc.request, History: tc.history}, nil)
					pass := err == nil && result.Response.Reply != "" && provider.calls-calls == 1 && !result.Repaired && !result.SemanticFallback
					row := hostedLayeredLiveRow(variant+"/"+tc.name, model, tc.request, result.Raw, before, result.Response.Motion, limits, started, err, pass)
					row["variant"], row["reply"], row["provider_calls"] = variant, result.Response.Reply, provider.calls-calls
					after := before
					if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
						after = *result.Response.Motion.Layered
					}
					intent := rangeCaseIntent(tc.name, before, after)
					row["intent_pass"], row["repeat"] = pass && intent, repeat+1
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					t.Logf("valid=%t intent=%t elapsed=%v reply=%s", pass, pass && intent, row["elapsed_ms"], result.Response.Reply)
					if !pass || !intent {
						t.Error("generation or intent failed; retained for review")
					}
				})
			}
		}
	}
}

func rangeCaseIntent(name string, before, after motion.FlowSpec) bool {
	switch name {
	case "more_pace":
		a, b := before, after
		a.SpeedPercent, b.SpeedPercent = 0, 0
		a.Layers = slices.DeleteFunc(slices.Clone(a.Layers), func(l motion.FlowLayer) bool { return l.Axis == "pace" })
		b.Layers = slices.DeleteFunc(slices.Clone(b.Layers), func(l motion.FlowLayer) bool { return l.Axis == "pace" })
		return after.SpeedPercent > before.SpeedPercent && reflect.DeepEqual(a, b)
	case "double_tip":
		return phraseCountIntent(after, 100, 2) && after.SpeedPercent == before.SpeedPercent && samePaceLayerValue(before, after)
	case "triple_base":
		return phraseCountIntent(after, 0, 3) && after.SpeedPercent == before.SpeedPercent && samePaceLayerValue(before, after)
	case "narrow_explicit":
		return after.MinPercent == 90 && after.MaxPercent == 100 && after.RangeFloorPercent == 10 && after.SpeedPercent == before.SpeedPercent
	case "tip_coverage":
		return after.AnchorPercent == 100 && after.RangeCeilingPercent > 30 && after.SpeedPercent == before.SpeedPercent && samePaceLayerValue(before, after)
	case "base_correction":
		return after.MinPercent == 0 && after.SpeedPercent == before.SpeedPercent
	case "mixed_regions":
		p := after.Phrase
		return p != nil && len(p.Strokes) == 3 && p.Strokes[0].Count == 2 && p.Strokes[1].Count == 2 && p.Strokes[2].Count == 1 && p.Strokes[0].BottomPercent > p.Strokes[1].TopPercent && p.Strokes[2].BottomPercent == 0 && p.Strokes[2].TopPercent == 100
	case "slower_sweep":
		return phraseCountIntent(after, 100, 2) && after.Phrase.Strokes[1].RatePercent < after.Phrase.Strokes[0].RatePercent && after.SpeedPercent == before.SpeedPercent
	case "slower_base_sweep":
		return phraseCountIntent(after, 0, 3) && after.Phrase.Strokes[1].RatePercent < after.Phrase.Strokes[0].RatePercent && after.SpeedPercent == before.SpeedPercent
	case "restricted_phrase":
		return after.Phrase != nil && len(after.Phrase.Strokes) == 2 && after.Phrase.Strokes[0].Count == 2 && after.Phrase.Strokes[1].Count == 1 && after.MinPercent == 20 && after.MaxPercent == 85 && after.Phrase.Strokes[1].BottomPercent == 20 && after.Phrase.Strokes[1].TopPercent == 85
	case "uncounted":
		return after.Phrase == nil && !reflect.DeepEqual(before, after) && after.SpeedPercent == before.SpeedPercent
	case "question":
		return reflect.DeepEqual(before, after)
	}
	return true
}

func phraseCountIntent(score motion.FlowSpec, anchor, count int) bool {
	p := score.Phrase
	return p != nil && len(p.Strokes) == 2 && p.Strokes[0].Count == count && p.Strokes[1].Count == 1 && p.Strokes[1].BottomPercent == 0 && p.Strokes[1].TopPercent == 100 && ((anchor == 100 && p.Strokes[0].TopPercent == 100 && p.Strokes[0].BottomPercent > 0) || (anchor == 0 && p.Strokes[0].BottomPercent == 0 && p.Strokes[0].TopPercent < 100))
}

func TestHostedLayeredPhraseContinuation(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	score := DefaultLayeredScore(32)
	history := []llm.Message{}
	for i, request := range []string{
		"Double stroke the tip, then bounce off the bottom and all the way up and repeat.",
		"Use more of the upper region for those two short strokes. Keep the counts, full return and pace.",
		"Increase the pace moderately; keep exactly this geometry and rhythm.",
		"More.",
		"Drop that counted pattern now. Let the reach and location wander freely instead, keeping the pace.",
		"Keep this exact motion repeating unchanged.",
	} {
		before := *motion.CloneFlowSpec(&score)
		caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
		plan := motion.NewMotionPlan("current", motion.MotionTarget{Flow: &score, SpeedPercent: score.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
		state := MotionContext{Running: true, Layered: &score, MotionMode: MotionModeLayered, SpeedPercent: score.SpeedPercent, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits), CommandedSummary: &plan.Perceptual}
		svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, MaxTokens: 1536, ReasoningMode: "off"}
		started, calls := time.Now(), provider.calls
		result, err := svc.Complete(t.Context(), Request{Message: request, History: history}, nil)
		if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
			score = *motion.CloneFlowSpec(result.Response.Motion.Layered)
		}
		pass := err == nil && result.Response.Reply != "" && provider.calls-calls == 1 && !result.Repaired && !result.SemanticFallback
		switch i {
		case 0:
			pass = pass && phraseCountIntent(score, 100, 2)
		case 1:
			pass = pass && phraseCountIntent(score, 100, 2) && before.Phrase != nil && score.Phrase.Strokes[0].BottomPercent < before.Phrase.Strokes[0].BottomPercent && score.SpeedPercent == before.SpeedPercent
		case 2, 3:
			pass = pass && reflect.DeepEqual(score.Phrase, before.Phrase) && score.SpeedPercent > before.SpeedPercent
		case 4:
			pass = pass && score.Phrase == nil && score.SpeedPercent == before.SpeedPercent && !reflect.DeepEqual(score, before)
		case 5:
			pass = pass && reflect.DeepEqual(score, before)
		}
		row := hostedLayeredLiveRow("continuation/"+strconv.Itoa(i+1), model, request, result.Raw, before, result.Response.Motion, limits, started, err, pass)
		row["reply"], row["provider_calls"] = result.Response.Reply, provider.calls-calls
		rows = append(rows, row)
		saveHostedLayeredRows(t, rows)
		history = append(history, llm.Message{Role: "user", Content: request}, llm.Message{Role: "assistant", Content: result.Response.Reply})
		t.Logf("turn=%d intent=%t elapsed=%v err=%v", i+1, pass, row["elapsed_ms"], err)
		if !pass {
			t.Error("intent failure retained for review")
		}
	}
}
