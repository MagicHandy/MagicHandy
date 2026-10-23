//go:build liveeval

package chat

import (
	"context"
	_ "embed"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

//go:embed testdata/hosted-phrase-alpha69.txt
var alpha69PhraseGuide string

type phraseJoinEvaluationProvider struct {
	llm.Provider
	variant string
}

func (p phraseJoinEvaluationProvider) StreamChat(ctx context.Context, r llm.ChatRequest, emit func(string) error) (string, error) {
	if p.variant == "baseline" {
		r.Messages = slices.Clone(r.Messages)
		for i := range r.Messages {
			if r.Messages[i].Role == "system" {
				r.Messages[i].Content = strings.ReplaceAll(r.Messages[i].Content, "\n"+hostedLayeredRegionGuide, "")
				r.Messages[i].Content = strings.ReplaceAll(r.Messages[i].Content, hostedLayeredPhraseGuide, alpha69PhraseGuide)
				break
			}
		}
	}
	return p.Provider.StreamChat(ctx, r, emit)
}

// Inert: only provider, parser and shared compiler. No engine or transport.
func TestHostedPhraseJoinChoices(t *testing.T) {
	p, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	variants := strings.Split(os.Getenv("MAGICHANDY_JOIN_VARIANTS"), ",")
	if variants[0] == "" {
		variants = []string{"baseline", "guidance"}
	}
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	for _, variant := range variants {
		for repeat := range max(1, repeats) {
			for _, tc := range []struct {
				name, request string
				counted       bool
				reverse       bool
			}{
				{"upper_fast", "Alternate quick strokes near the upper end with slow strokes near the lower end, at a moderate overall pace.", false, false},
				{"lower_fast", "Alternate quick lower-end strokes with slower upper-end strokes. Use a moderate overall pace.", false, true},
				{"separate_regions", "Use flowing alternation between vigorous upper-region strokes and gentle lower-region strokes, with a moderate overall speed and no fixed counts.", false, false},
				{"counted_bridge", "Repeat two short upper-end strokes, then two short lower-end strokes, then one full sweep to the top and back, in that order. Keep the base speed.", true, false},
				{"counted_regions", "Repeat four short upper-end strokes, then two short lower-end strokes. Move smoothly between the regions, keeping those counts and the current base speed.", true, false},
			} {
				t.Run(variant+"/"+tc.name+"/"+strconv.Itoa(repeat+1), func(t *testing.T) {
					before := DefaultLayeredScore(100)
					before.MinPercent, before.MaxPercent, before.AnchorPercent, before.RangeFloorPercent, before.RangeCeilingPercent, before.Seed = 0, 100, 100, 30, 100, 841
					before.Phrase = &motion.FlowPhrase{Strokes: []motion.PhraseStroke{{BottomPercent: 70, TopPercent: 100, Count: 4, RatePercent: 100}, {BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100}}}
					before.Layers = slices.DeleteFunc(before.Layers, func(layer motion.FlowLayer) bool { return layer.Axis != "pace" })
					if err := before.Validate(limits); err != nil {
						t.Fatalf("invalid evaluation starting score: %v", err)
					}
					plan := motion.NewMotionPlan("before", motion.MotionTarget{Flow: &before, SpeedPercent: 100}, limits, 0, 0, time.Unix(0, 0))
					state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: 100, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits), CommandedSummary: &plan.Perceptual}
					caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
					svc := Service{Provider: phraseJoinEvaluationProvider{p, variant}, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 512}
					started, calls := time.Now(), p.calls
					result, err := svc.Complete(t.Context(), Request{Message: tc.request}, nil)
					pass := err == nil && result.Response.Motion != nil && result.Response.Motion.Layered != nil && !result.Repaired
					if pass {
						after := *result.Response.Motion.Layered
						if tc.name == "counted_regions" {
							groups := after.Phrase
							pass = groups != nil && len(groups.Strokes) == 3 && after.SpeedPercent == before.SpeedPercent
							if pass {
								a, b, join := groups.Strokes[0], groups.Strokes[1], groups.Strokes[2]
								pass = a.Count == 4 && b.Count == 2 && join.Count == 1 && a.BottomPercent > b.TopPercent && join.TopPercent >= a.BottomPercent+10 && join.BottomPercent <= b.TopPercent-10
							}
						} else if tc.counted {
							pass = rangeCaseIntent("mixed_regions", before, after) && after.SpeedPercent == before.SpeedPercent
						} else {
							pass = after.Phrase == nil && after.SpeedPercent < before.SpeedPercent
							var center, pace motion.FlowLayer
							for _, l := range after.Layers {
								if l.Axis == "center" {
									center = l
								}
								if l.Axis == "pace" {
									pace = l
								}
							}
							pass = pass && hasCompiledRegionalTurns(after, limits) && center.AmountPercent > 0 && center.Shape != "drift" && pace.AmountPercent > 0 && pace.LowRatePercent < 100 && pace.PeriodCycles == center.PeriodCycles && pace.PhasePercent == center.PhasePercent && pace.Shape == center.Shape && pace.Inverted == (center.Inverted != tc.reverse)
						}
					}
					row := hostedLayeredLiveRow(variant+"/"+tc.name, model, tc.request, result.Raw, before, result.Response.Motion, limits, started, err, pass)
					row["variant"], row["repeat"], row["provider_calls"], row["reply"] = variant, repeat+1, p.calls-calls, result.Response.Reply
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					t.Logf("valid=%t intent=%t elapsed=%v error=%v", err == nil, pass, row["elapsed_ms"], err)
					if !pass {
						t.Error("choice or intent failed; retained for review")
					}
				})
			}
		}
	}
}
