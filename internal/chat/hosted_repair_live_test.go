//go:build liveeval

package chat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type rejectedFirstProvider struct {
	llm.Provider
	raw   string
	calls int
}

func (p *rejectedFirstProvider) StreamChat(ctx context.Context, r llm.ChatRequest, emit func(string) error) (string, error) {
	p.calls++
	if p.calls == 1 {
		return p.raw, nil
	}
	return p.Provider.StreamChat(ctx, r, emit)
}

// The first response is a retained or deliberately malformed structured proposal;
// the repair is a real hosted generation through the production Service. This
// isolates repair effectiveness from the chance of a model making an error.
// There is no engine or transport and the current installed session is untouched.
func TestHostedResponseRepairLive(t *testing.T) {
	p, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	for _, automatic := range []bool{true, false} {
		for _, tc := range []struct {
			name, request, raw string
			counted            bool
		}{
			{"regional_seam", "Alternate quick upper-end strokes with slow lower-end strokes at a moderate overall pace.", `{"action":"update","edits":{"composition":"phrase","controls":{"speed_percent":{"op":"set","value":60}},"layers":{"pace":{"op":"remove"}},"phrase":{"strokes":[{"bottom_percent":78,"count":4,"rate_percent":100,"top_percent":100},{"bottom_percent":0,"count":2,"rate_percent":35,"top_percent":35}]}},"reply":"Alternating upper and lower strokes."}`, false},
			{"duplicate_operation", "Increase the overall speed slightly. Keep the reach, counts and rhythm unchanged.", `{"action":"update","edits":{"composition":"manual","controls":{"speed_percent":{"op":"set","op":"adjust","value":5}}},"reply":"A little faster."}`, false},
			{"invalid_count", "Repeat two short upper-end strokes followed by one full sweep, at the current base speed.", `{"action":"update","edits":{"composition":"phrase","phrase":{"strokes":[{"bottom_percent":70,"count":9,"rate_percent":100,"top_percent":100},{"bottom_percent":0,"count":1,"rate_percent":100,"top_percent":100}]}},"reply":"Two upper strokes then a full sweep."}`, true},
			{"counted_seam", "Repeat four short upper-end strokes and two short lower-end strokes, with smooth travel between the regions. Keep those counts and the base speed.", `{"action":"update","edits":{"composition":"phrase","phrase":{"strokes":[{"bottom_percent":70,"count":4,"rate_percent":100,"top_percent":100},{"bottom_percent":0,"count":2,"rate_percent":100,"top_percent":35},{"bottom_percent":0,"count":1,"rate_percent":100,"top_percent":70}]}},"reply":"Four upper and two lower strokes, with a connecting stroke."}`, true},
			{"counted_seam_explicit", "Repeat four upper-end strokes from 70 to 100 and two lower-end strokes from 0 to 35, with smooth connecting travel between the regions. Keep those endpoints, counts and the base speed.", `{"action":"update","edits":{"composition":"phrase","phrase":{"strokes":[{"bottom_percent":70,"count":4,"rate_percent":100,"top_percent":100},{"bottom_percent":0,"count":2,"rate_percent":100,"top_percent":35},{"bottom_percent":0,"count":1,"rate_percent":100,"top_percent":70}]}},"reply":"Four upper and two lower strokes, with a connecting stroke."}`, true},
			{"wrong_shape_contract", "Switch to short strokes alternating between the upper and lower ends, with the upper region faster. Keep the current base speed.", `{"action":"update","edits":{"composition":"geometry","geometry":"alternate_ends","layers":{"center":{"op":"edit","value":{"amount_percent":100}},"pace":{"op":"follow","value":{"axis":"center","relation":"same","amount_percent":100,"low_rate_percent":35}}}},"reply":"Alternating regions."}`, false},
		} {
			t.Run(fmt.Sprintf("automatic_%t/%s", automatic, tc.name), func(t *testing.T) {
				before := DefaultLayeredScore(40)
				before.MinPercent, before.MaxPercent, before.AnchorPercent, before.RangeFloorPercent, before.RangeCeilingPercent, before.Seed = 0, 100, 100, 30, 100, 841
				before.Phrase = &motion.FlowPhrase{Strokes: []motion.PhraseStroke{{BottomPercent: 70, TopPercent: 100, Count: 4, RatePercent: 100}, {BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100}}}
				before.Layers = slices.DeleteFunc(before.Layers, func(layer motion.FlowLayer) bool { return layer.Axis != "pace" })
				if err := before.Validate(limits); err != nil {
					t.Fatalf("invalid evaluation starting score: %v", err)
				}
				state := MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: 40, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
				caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
				injected := &rejectedFirstProvider{Provider: p, raw: tc.raw}
				svc := Service{Provider: injected, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: 512, HostedResponseRepair: automatic}
				started, calls := time.Now(), p.calls
				result, err := svc.Complete(t.Context(), Request{Message: tc.request}, nil)
				if !automatic {
					if err == nil || !result.CanRepair() || injected.calls != 1 {
						t.Fatal("manual repair was not offered without a model retry")
					}
					result, err = svc.Repair(t.Context(), result, nil)
				}
				pass := err == nil && result.Repaired && !result.CanRepair() && injected.calls == 2 && p.calls-calls == 1 && result.Response.Reply != "" && result.Response.Motion != nil && result.Response.Motion.Layered != nil
				if pass {
					after := result.Response.Motion.Layered
					switch tc.name {
					case "regional_seam":
						pass = after.Phrase == nil && hasCompiledRegionalTurns(*after, limits)
					case "duplicate_operation":
						pass = after.SpeedPercent > before.SpeedPercent && fmt.Sprint(after.Phrase) == fmt.Sprint(before.Phrase)
					case "invalid_count":
						pass = phraseCountIntent(*after, 100, 2) && after.SpeedPercent == before.SpeedPercent
					case "counted_seam", "counted_seam_explicit":
						pass = after.Phrase != nil && len(after.Phrase.Strokes) == 3 && after.SpeedPercent == before.SpeedPercent
						if pass {
							a, b, join := after.Phrase.Strokes[0], after.Phrase.Strokes[1], after.Phrase.Strokes[2]
							// Unrequested coordinates in the rejected JSON are not
							// user constraints. Check regional intent and counts;
							// enforce exact endpoints only in the explicit case.
							pass = a.Count == 4 && b.Count == 2 && join.Count == 1 && a.BottomPercent >= 50 && b.TopPercent <= 50 && join.TopPercent >= a.BottomPercent+10
							if tc.name == "counted_seam_explicit" {
								pass = pass && a.BottomPercent == 70 && a.TopPercent == 100 && b.BottomPercent == 0 && b.TopPercent == 35
							}
						}
					case "wrong_shape_contract":
						pass = after.Phrase == nil && after.SpeedPercent == before.SpeedPercent && hasCompiledRegionalTurns(*after, limits)
					}
				}
				prefix := fmt.Sprintf("repair_%t/%s", automatic, tc.name)
				initial := hostedLayeredLiveRow(prefix+"/initial_rejection", model, tc.request, result.Raw, before, nil, limits, started, fmt.Errorf("%s", result.InitialError), false)
				initial["injected_fixture"] = true
				row := hostedLayeredLiveRow(prefix+"/repaired", model, tc.request, result.RepairRaw, before, result.Response.Motion, limits, started, err, pass)
				row["initial_error"], row["reply"], row["provider_calls"], row["repair_attempted"], row["repaired"] = result.InitialError, result.Response.Reply, injected.calls, result.RepairAttempted, result.Repaired
				rows = append(rows, initial, row)
				saveHostedLayeredRows(t, rows)
				t.Logf("repaired=%t intent=%t elapsed=%v err=%v", result.Repaired, pass, row["elapsed_ms"], err)
				if !pass {
					t.Error("repair or intended change failed; retained for review")
				}
				if !strings.HasPrefix(result.Raw, "{") {
					t.Fatal("original rejected response not retained")
				}
			})
		}
	}
}

// Evaluate actual reversals as well as the selected fields. A center envelope
// can traverse the whole band so quickly that nominally short regional strokes
// disappear. These full-range fixture checks do not constrain runtime choices.
func hasCompiledRegionalTurns(score motion.FlowSpec, limits config.MotionSettings) bool {
	plan := motion.NewMotionPlan("regional-review", motion.MotionTarget{Flow: &score, SpeedPercent: score.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
	turns := plan.Perceptual.Turns
	return turns.LowerPercentiles[2] > 50 && turns.UpperPercentiles[0] < 50
}
