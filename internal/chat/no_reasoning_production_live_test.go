//go:build liveeval

package chat

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Exercise the real capability -> schema -> parser path at the connection's
// actual 1,024-token answer floor. Unlike withLiveLevel, nothing rewrites the
// request or strips the note at the provider boundary. Each turn gets a fresh
// provider, as the application's prepared provider does. No transport exists.
func TestNoReasoningProductionContractLive(t *testing.T) {
	note := os.Getenv("MAGICHANDY_PRODUCTION_PLAN_NOTE") == "1"
	if os.Getenv("MAGICHANDY_LIVE_LEVEL") != "" {
		t.Fatal("unset MAGICHANDY_LIVE_LEVEL: this test uses production capabilities")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 1, 100
	var rows []map[string]any
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		for _, tc := range []struct {
			name, message   string
			stopped, repair bool
		}{
			{"pace_only", "Increase the speed by five percentage points. Keep all reach and pattern details unchanged.", false, false},
			{"resume", "Continue the motion where we left off, using the same pattern and speed.", true, false},
			{"explanation", "What does the speed setting mean? Just explain; leave the device stopped.", true, false},
			{"variation", "Increase the timing variation so the slower stretches and brief quicker flurries are more distinct. Keep the base speed unchanged.", false, false},
			{"counted", "Repeat two short upper-end strokes followed by one full-range sweep. Keep that exact order and count. Make it flow smoothly.", false, false},
			{"bounded_counted", "Keep all travel within 20 to 80 percent. Repeat two short strokes at the upper end of that band, then one sweep filling that entire band. Keep that exact order and count.", false, false},
			{"repair", "Increase the speed by five percentage points. Keep all reach and pattern details unchanged.", false, true},
		} {
			if mode == MotionModeCreativeV2 && (tc.name == "counted" || tc.name == "bounded_counted" || tc.repair) {
				continue
			}
			t.Run(string(mode)+"/"+tc.name, func(t *testing.T) {
				provider, model := hostedLayeredLiveProvider(t)
				before := DefaultLayeredScore(35)
				if mode == MotionModeCreativeV2 {
					before = FreshCreativeV2Score(35)
				}
				before.Seed = 915
				state := MotionContext{Running: !tc.stopped, MotionMode: mode, Layered: &before, SpeedPercent: 35, SpeedMinPercent: 1, SpeedMaxPercent: 100, Envelope: motion.CurrentPlanningEnvelope(limits)}
				caps := Capabilities{Motion: true, MotionMode: mode, AdvancedMotion: true, PreserveConversationText: true, ChatPlanNote: note}
				svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: config.MinimumLocalOutput(config.ModelCapabilityAdvanced), HostedResponseRepair: tc.repair}
				if tc.repair {
					svc.Provider = &rejectedFirstProvider{Provider: provider, raw: `{"action":"update","edits":{"composition":"manual","controls":{"speed_percent":{"op":"set","op":"adjust","value":5}}},"reply":"Faster."}`}
				}
				started := time.Now()
				result, err := svc.Complete(t.Context(), Request{Message: tc.message}, nil)
				command := result.Response.Motion
				pass := err == nil && result.Response.Reply != "" && !result.SemanticFallback && result.Repaired == tc.repair
				if tc.name == "explanation" {
					pass = pass && command == nil
				} else {
					pass = pass && command != nil && command.Layered != nil
				}
				if pass && command != nil {
					after := *command.Layered
					switch tc.name {
					case "pace_only", "repair":
						pass = after.SpeedPercent == before.SpeedPercent+5
						after.SpeedPercent = before.SpeedPercent
						pass = pass && reflect.DeepEqual(after, before)
					case "resume":
						pass = reflect.DeepEqual(after, before)
					case "variation":
						pass = after.SpeedPercent == before.SpeedPercent && (after.PaceVariationPercent > before.PaceVariationPercent || pacePeriod(&after) != pacePeriod(&before) || (after.Gesture != nil && after.Gesture.VariationPercent > before.Gesture.VariationPercent))
					case "counted":
						// "Upper-end" does not demand an exact endpoint for the
						// short strokes; only the full-range sweep does.
						pass = after.Phrase != nil && len(after.Phrase.Strokes) == 2 && after.Phrase.Strokes[0].Count == 2 && after.Phrase.Strokes[0].BottomPercent > 50 && after.Phrase.Strokes[0].TopPercent >= 90 && after.Phrase.Strokes[1].Count == 1 && after.Phrase.Strokes[1].BottomPercent == 0 && after.Phrase.Strokes[1].TopPercent == 100
					case "bounded_counted":
						pass = after.Phrase != nil && len(after.Phrase.Strokes) == 2 && after.Phrase.Strokes[0].Count == 2 && after.Phrase.Strokes[0].BottomPercent > 50 && after.Phrase.Strokes[0].TopPercent >= 70 && after.Phrase.Strokes[0].TopPercent <= 80 && after.Phrase.Strokes[1].Count == 1 && after.Phrase.Strokes[1].BottomPercent == 20 && after.Phrase.Strokes[1].TopPercent == 80
					}
				}
				row := hostedLayeredLiveRow(string(mode)+"/"+tc.name, model, tc.message, result.Raw, before, command, limits, started, err, pass)
				row["method"], row["plan_note"], row["reply"], row["repair_raw"], row["repaired"], row["provider_calls"] = mode, note, result.Response.Reply, result.RepairRaw, result.Repaired, provider.calls
				rows = append(rows, row)
				t.Logf("intent=%t elapsed_ms=%v calls=%d error=%v", pass, row["elapsed_ms"], provider.calls, err)
				if !pass {
					t.Error("production intent check failed; captured for review")
				}
			})
		}
	}
}
