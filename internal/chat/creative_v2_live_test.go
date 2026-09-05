//go:build liveeval

package chat

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Run explicitly against installed local models. Every selection, including
// rejected proposals, is retained in the atlas-compatible report.
func TestCreativeV2LiveMapping(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_LAB_MODELS"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_LAB_MODELS")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: models[0], Timeout: 90 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	defer func() {
		if path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"); path != "" {
			encoded, _ := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Error(err)
			} // #nosec G703 -- explicit local evaluation output
		}
	}()
	type scenario struct {
		name, message string
		check         func(motion.FlowSpec, motion.FlowSpec) bool
	}
	sequence := []scenario{
		{"tip-sweep", "Work the upper end with quick travel toward the tip and a slower return. Keep the pace setting.", func(a, b motion.FlowSpec) bool {
			return b.Gesture.FocusPercent == 100 && b.Gesture.FocusMixPercent == 100 && b.Gesture.FocusWidthPercent <= 35 && b.Gesture.FasterDirection == "tip" && b.Gesture.ContrastPercent >= 40 && b.SpeedPercent == a.SpeedPercent
		}},
		{"base-bounce-and-full", "Now bounce at the lower end, shrink each rebound, then go back to full strokes. Mix these up naturally.", func(a, b motion.FlowSpec) bool {
			return b.Gesture.FocusPercent == 0 && b.Gesture.FocusMixPercent > 0 && b.Gesture.FocusMixPercent < 100 && b.Gesture.ReboundCount >= 2 && b.Gesture.FocusWidthPercent >= 35 && b.SpeedPercent == a.SpeedPercent
		}},
		{"late-crest", "Make the travel build momentum later within each stroke. Keep everything else.", func(a, b motion.FlowSpec) bool {
			return b.Gesture.InertiaPercent > a.Gesture.InertiaPercent && b.Gesture.ReboundCount == a.Gesture.ReboundCount && b.SpeedPercent == a.SpeedPercent
		}},
		{"remove-bounce", "Remove the bounce and preserve the rest of this motion.", func(a, b motion.FlowSpec) bool {
			copy := *b.Gesture
			copy.ReboundCount = a.Gesture.ReboundCount
			return b.Gesture.ReboundCount == 0 && copy == *a.Gesture && a.SpeedPercent == b.SpeedPercent
		}},
		{"full-only", "Return to only full strokes across the entire slider.", func(a, b motion.FlowSpec) bool {
			return b.Gesture.FocusMixPercent == 0 && b.MinPercent == 0 && b.MaxPercent == 100 && a.SpeedPercent == b.SpeedPercent
		}},
		{"question", "Explain how this preview differs from measured motion. Do not change any controls.", func(a, b motion.FlowSpec) bool { return reflect.DeepEqual(a, b) }},
		{"fresh", "Keep varying within this same character.", func(a, b motion.FlowSpec) bool {
			return a.Seed != b.Seed && *a.Gesture == *b.Gesture && a.SpeedPercent == b.SpeedPercent
		}},
		{"autopilot", CreativeV2ContinuationMessage(nil), func(a, b motion.FlowSpec) bool {
			return a.Seed != b.Seed && *a.Gesture == *b.Gesture && a.MinPercent == b.MinPercent && a.MaxPercent == b.MaxPercent && a.SpeedPercent == b.SpeedPercent
		}},
		{"hold", CreativeV2ContinuationMessage([]string{"Keep this exact score unchanged."}), func(a, b motion.FlowSpec) bool { return reflect.DeepEqual(a, b) }},
	}
	independent := []scenario{
		{"reverse-bias", "Make downward travel noticeably faster than upward travel, keeping the band and speed setting.", func(a, b motion.FlowSpec) bool {
			return b.Gesture.FasterDirection == "base" && b.Gesture.ContrastPercent >= 40 && a.SpeedPercent == b.SpeedPercent && a.MinPercent == b.MinPercent && a.MaxPercent == b.MaxPercent
		}},
		{"numeric-compound", "Anchor at the base. Use 45 percent local width, mix 60, three rebounds retaining 75 percent width each, and inertia 70. Keep speed.", func(a, b motion.FlowSpec) bool {
			g := b.Gesture
			return g.FocusPercent == 0 && g.FocusWidthPercent == 45 && g.FocusMixPercent == 60 && g.ReboundCount == 3 && g.ReboundDecayPercent == 75 && g.InertiaPercent == 70 && a.SpeedPercent == b.SpeedPercent
		}},
		{"middle-excursions", "Mix broad travel with short strokes centered in the middle. No bounces.", func(a, b motion.FlowSpec) bool {
			return b.Gesture.FocusPercent == 50 && b.Gesture.FocusWidthPercent <= 35 && b.Gesture.FocusMixPercent > 0 && b.Gesture.FocusMixPercent < 100 && b.Gesture.ReboundCount == 0
		}},
		{"slower", "Slow the overall speed setting by exactly five points. Keep the character unchanged.", func(a, b motion.FlowSpec) bool { return b.SpeedPercent == a.SpeedPercent-5 && *a.Gesture == *b.Gesture }},
		{"gentle", "Make it gentler without changing the reach.", func(a, b motion.FlowSpec) bool {
			return b.SpeedPercent < a.SpeedPercent && a.MinPercent == b.MinPercent && a.MaxPercent == b.MaxPercent
		}},
		{"full-reset", "Use only full strokes. Make travel timing equal in both directions and remove inertia and rebounds.", func(a, b motion.FlowSpec) bool {
			return b.Gesture.FocusMixPercent == 0 && b.Gesture.InertiaPercent == 0 && b.Gesture.ReboundCount == 0 && (b.Gesture.FasterDirection == "even" || b.Gesture.ContrastPercent == 0)
		}},
		{"small-band", "Use the outer band 30 to 60 and local width 15, staying at the same speed.", func(a, b motion.FlowSpec) bool {
			return b.MinPercent == 30 && b.MaxPercent == 60 && b.Gesture.FocusWidthPercent == 15 && b.SpeedPercent == a.SpeedPercent
		}},
	}
	for _, model := range models {
		current := FreshCreativeV2Score(25)
		history := []llm.Message{}
		for index, sc := range append(sequence, independent...) {
			if index >= len(sequence) {
				current = FreshCreativeV2Score(25)
				history = nil
			}
			trial := RunLLMLab(t.Context(), provider, model, "creative_v2", LLMLabPrompts()["creative_v2"], sc.message, current, limits, history, true)
			intent := trial.Valid && sc.check(current, trial.After)
			encoded, _ := json.Marshal(trial)
			row := map[string]any{}
			_ = json.Unmarshal(encoded, &row)
			row["intent_pass"], row["expected_recipe"], row["evaluation"] = intent, sc.name, sc.name
			rows = append(rows, row)
			t.Logf("%s %s valid=%v intent=%v raw=%s error=%s", model, sc.name, trial.Valid, intent, trial.Raw, trial.Error)
			if trial.Valid {
				current = trial.After
			}
			history = append(history, llm.Message{Role: "user", Content: sc.message}, llm.Message{Role: "assistant", Content: trial.Raw})
		}
	}
}
