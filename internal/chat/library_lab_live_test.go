//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLibraryLabLive(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_LAB_MODELS"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_LAB_MODELS")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	records := []labEvaluationTrial{}
	failures := 0
	for _, model := range models {
		provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{"library", "library_descriptive", "library_actions"} {
			for repeat := 0; repeat < 2; repeat++ {
				initial, _ := motion.ContinuousRecipeByID(motion.PatternFullSweeps, 25)
				current := initial.Spec
				history := []llm.Message{}
				for _, test := range []struct {
					message string
					id      motion.PatternID
					speed   int
				}{
					{"Return to base with slowly changing irregular reach and long variation memory. Keep pace.", "flow-base-drift", 25},
					{"Return to tip with slowly changing irregular reach and long variation memory. Keep pace.", "flow-tip-drift", 25},
					{"Keep the center fixed while stroke width wanders irregularly. Keep pace.", "flow-centered-drift", 25},
					{"Use full-length strokes with longer softer turnarounds at both ends. Keep pace.", "flow-soft-sweeps", 25},
					{"Vary width irregularly around the middle but maintain an even cycle beat. Keep pace.", "flow-even-beat", 25},
					{"Four lower cycles, four middle, four upper, four middle, repeating. Keep pace.", "flow-zone-tour", 25},
					{"Move a varying-width window through the band. Width wanders between 20 and 65. Keep pace.", "flow-breathing-window", 25},
					{"Gradually vary stroke length while returning to the tip on every stroke. Keep the pace setting.", "flow-tip-anchored", 25},
					{"Increase speed to 35. Preserve this exact movement shape.", "flow-tip-anchored", 35},
					{"Keep stroke width fixed and move the whole window gradually between lower and upper regions. Keep speed.", "flow-traveling-window", 35},
					{"Switch to full-length sweeps, with pace rising and falling in a wave. Keep the pace setting.", motion.PatternPaceWave, 35},
					{"Keep every setting exactly as it is. Just confirm.", motion.PatternPaceWave, 35},
				} {
					trial := RunLLMLab(context.Background(), provider, model, method, LLMLabPrompts()[method], test.message, current, limits, history, true)
					want, _ := motion.ContinuousRecipeByID(test.id, test.speed)
					_, wantName := labCurrentRecipe(method, want.Spec)
					_, gotName := labCurrentRecipe(method, trial.After)
					accepted := trial.Valid && gotName == wantName && trial.After.SpeedPercent == test.speed
					if !accepted {
						failures++
						t.Logf("FAIL %s %s expected %s@%d got %s@%d: %s", model, method, wantName, test.speed, gotName, trial.After.SpeedPercent, trial.Error)
					}
					if trial.Valid {
						if _, err := motion.FlowTarget(trial.After, limits); err != nil {
							if accepted {
								failures++
							}
							accepted = false
							t.Logf("compile: %v", err)
						}
					}
					records = append(records, labEvaluationTrial{LLMLabTrial: trial, Expected: wantName, Intent: accepted})
					if trial.Valid {
						current = trial.After
					}
					history = append(history, llm.Message{Role: "user", Content: test.message}, llm.Message{Role: "assistant", Content: trial.Raw})
				}
			}
		}
	}
	if path := os.Getenv("MAGICHANDY_LAB_REPORT"); path != "" {
		encoded, _ := json.MarshalIndent(records, "", "  ")
		if err := os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("Library Lab continuation checks: %d/%d", len(records)-failures, len(records))
	if failures > 0 {
		t.Errorf("%d live library-lab failures; raw evidence retained", failures)
	}
}
