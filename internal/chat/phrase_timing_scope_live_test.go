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

const previousPhraseTiming = "Existing materialized pace timing persists independently; phrase pace allows edit/remove, not follow. Count/reach-only edits omit controls/layers/evolve and preserve existing group rates."
const scopedPhraseTiming = "Phrase pace allows edit/remove, not follow. When replacing a regional field with a new counted phrase, reassess pace previously coupled to range/center layers now removed: retaining it makes an unrelated whole-phrase speed cycle. Explicitly remove an obsolete coupling, or translate a still-requested regional contrast into group rates. Preserve independently requested pace variation and the selected base speed. Count/reach-only edits within an existing phrase preserve its group rates and independent pace; omit unrelated controls/layers/evolve."

type phraseTimingScopeProvider struct {
	llm.Provider
	variant string
}

func (p phraseTimingScopeProvider) StreamChat(ctx context.Context, r llm.ChatRequest, emit func(string) error) (string, error) {
	r.Messages = slices.Clone(r.Messages)
	for i := range r.Messages {
		if r.Messages[i].Role != "system" {
			continue
		}
		r.Messages[i].Content = strings.ReplaceAll(r.Messages[i].Content, hostedPhraseTimingGuide, previousPhraseTiming)
		if p.variant == "replacement" {
			r.Messages[i].Content = strings.ReplaceAll(r.Messages[i].Content, previousPhraseTiming, hostedPhraseTimingGuide)
		} else if p.variant == "scoped" {
			r.Messages[i].Content = strings.ReplaceAll(r.Messages[i].Content, previousPhraseTiming, scopedPhraseTiming)
		} else {
			r.Messages[i].Content = strings.ReplaceAll(r.Messages[i].Content, scopedPhraseTiming, previousPhraseTiming)
		}
		break
	}
	return p.Provider.StreamChat(ctx, r, emit)
}

// Provider/parser/compiler only: no engine, transport or real-device access.
func TestHostedPhraseTimingScope(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	variants := strings.Split(os.Getenv("MAGICHANDY_TIMING_VARIANTS"), ",")
	if variants[0] == "" {
		variants = []string{"replacement"}
	}
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	for _, variant := range variants {
		for repeat := range max(1, repeats) {
			for _, name := range []string{"replace_coupled", "replace_lower", "preserve_independent", "count_existing", "slow_sweep", "question"} {
				t.Run(variant+"/"+name+"/"+strconv.Itoa(repeat), func(t *testing.T) {
					before := DefaultLayeredScore(58)
					before.MinPercent, before.MaxPercent, before.RangeFloorPercent, before.RangeCeilingPercent = 0, 100, 28, 28
					before.PaceVariationPercent = 0
					before.Layers = []motion.FlowLayer{
						{Axis: "center", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate"},
						{Axis: "pace", AmountPercent: 100, PeriodCycles: 8, Shape: "alternate", LowRatePercent: 25},
					}
					request := "Double stroke the tip, then bounce off the bottom and all the way up and repeat."
					history := []llm.Message{{Role: "user", Content: "Alternate quick upper-end strokes with much slower strokes near the lower end."}, {Role: "assistant", Content: "The upper-region strokes are quick and the lower-region strokes are slower."}}
					switch name {
					case "replace_lower":
						request = "Change to three short strokes at the lower end, then one full sweep to the top and back, repeating."
					case "preserve_independent":
						history = []llm.Message{{Role: "user", Content: "Let the overall speed cycle between slow and fast independently of stroke location. Keep that speed cycle as we change patterns."}}
						request += " Keep that overall slow/fast cycle exactly as it is."
					case "count_existing":
						before.Phrase = &motion.FlowPhrase{Strokes: []motion.PhraseStroke{{BottomPercent: 72, TopPercent: 100, Count: 2, RatePercent: 100}, {BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 65}}}
						before.RangeCeilingPercent, before.AnchorPercent = 100, 100
						before.Layers = before.Layers[1:]
						history = nil
						request = "Make the two short upper strokes into three. Keep everything else, including both group rates and the existing slow/fast cycle."
					case "slow_sweep":
						request = "Replace this alternation with two quick upper-end strokes followed by one slower full sweep to the base and back. Keep the current base speed; the slower travel belongs only to the full sweep, with no separate whole-pattern speed cycling."
					case "question":
						request = "Explain what the pace layer is doing; leave the motion unchanged."
					}
					plan := motion.NewMotionPlan("before", motion.MotionTarget{Flow: &before, SpeedPercent: before.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
					state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: before.SpeedPercent, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits), CommandedSummary: &plan.Perceptual}
					caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true}
					svc := Service{Provider: phraseTimingScopeProvider{provider, variant}, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 1024}
					started, calls := time.Now(), provider.calls
					result, err := svc.Complete(t.Context(), Request{Message: request, History: history}, nil)
					valid := err == nil && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback && provider.calls-calls == 1
					after := before
					if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
						after = *result.Response.Motion.Layered
					}
					intent := phraseTimingScopeIntent(name, before, after)
					row := hostedLayeredLiveRow(variant+"/"+name, model, request, result.Raw, before, result.Response.Motion, limits, started, err, valid && intent)
					row["variant"], row["repeat"], row["provider_calls"], row["reply"] = variant, repeat+1, provider.calls-calls, result.Response.Reply
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					t.Logf("valid=%t intent=%t elapsed=%v", valid, intent, row["elapsed_ms"])
					if !valid || !intent {
						t.Error("generation or timing intent failed; retained for visual review")
					}
				})
			}
		}
	}
}

func phraseTimingScopeIntent(name string, before, after motion.FlowSpec) bool {
	if name == "question" {
		return reflect.DeepEqual(before, after)
	}
	if after.SpeedPercent != before.SpeedPercent || after.Phrase == nil || len(after.Phrase.Strokes) != 2 {
		return false
	}
	groups := after.Phrase.Strokes
	if name == "count_existing" {
		return groups[0].Count == 3 && groups[0].RatePercent == 100 && groups[1].RatePercent == 65 && samePaceLayerValue(before, after)
	}
	count := 2
	if name == "replace_lower" {
		count = 3
	}
	if groups[0].Count != count || groups[1].Count != 1 || groups[1].BottomPercent != 0 || groups[1].TopPercent != 100 {
		return false
	}
	if name == "preserve_independent" {
		return samePaceLayerValue(before, after)
	}
	for _, layer := range after.Layers {
		if layer.Axis == "pace" && layer.AmountPercent != 0 {
			return false
		}
	}
	if name == "replace_coupled" || name == "replace_lower" {
		return groups[0].RatePercent == groups[1].RatePercent
	}
	return name != "slow_sweep" || groups[1].RatePercent < groups[0].RatePercent
}
