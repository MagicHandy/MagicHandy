//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type labEvaluationTrial struct {
	LLMLabTrial
	Expected string
	Intent   bool
}

func TestLLMLabLiveInterfaces(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_LAB_MODELS"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_LAB_MODELS to installed Ollama model names")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: models[0]})
	if err != nil {
		t.Fatal(err)
	}
	type scenario struct {
		name, method, prompt string
		accepts              func(motion.FlowSpec) bool
	}
	cases := []scenario{
		{"anchor", "controls", "Keep the speed and outer band unchanged. Hold the tip end while the stroke length varies.", func(s motion.FlowSpec) bool {
			return s.AnchorPercent == 100 && s.SpeedPercent == 25 && s.MinPercent == 5 && s.MaxPercent == 95
		}},
		{"full-band", "controls", "Use the full outer band, exactly 0 to 100. Keep the speed and shortest stroke unchanged.", func(s motion.FlowSpec) bool {
			return s.MinPercent == 0 && s.MaxPercent == 100 && s.SpeedPercent == 25 && s.RangeFloorPercent == 25
		}},
		{"memory", "controls", "Make the range evolve over longer trends: set memory to 20 cycles. Keep pace unchanged.", func(s motion.FlowSpec) bool { return s.MemoryCycles == 20 && s.SpeedPercent == 25 }},
		{"negation", "controls", "Do not make it faster. Remove pace variation, keeping the same speed and range.", func(s motion.FlowSpec) bool {
			return s.PaceVariationPercent == 0 && s.SpeedPercent == 25 && s.MinPercent == 5 && s.MaxPercent == 95
		}},
		{"chat-only", "controls", "Explain what the preview can tell me. Leave every control unchanged.", func(s motion.FlowSpec) bool {
			return s.AnchorPercent == 0 && s.SpeedPercent == 25 && s.MemoryCycles == 8 && s.RangeFloorPercent == 25
		}},
		{"two-sections", "sequence", "Arrange four cycles from 0 to 100 at speed 25, then two cycles from 60 to 100 at speed 35. Repeat that order.", func(s motion.FlowSpec) bool {
			return len(s.Steps) == 2 && s.Steps[0] == (motion.FlowStep{MinPercent: 0, MaxPercent: 100, SpeedPercent: 25, Cycles: 4}) && s.Steps[1] == (motion.FlowStep{MinPercent: 60, MaxPercent: 100, SpeedPercent: 35, Cycles: 2})
		}},
		{"three-sections", "sequence", "Arrange 3 cycles in 10–40 at speed 20, 4 cycles in 0–100 at speed 30, then 2 cycles in 30–80 at speed 25.", func(s motion.FlowSpec) bool {
			return len(s.Steps) == 3 && s.Steps[0].Cycles == 3 && s.Steps[0].MinPercent == 10 && s.Steps[0].MaxPercent == 40 && s.Steps[1].Cycles == 4 && s.Steps[1].SpeedPercent == 30 && s.Steps[2].Cycles == 2 && s.Steps[2].MinPercent == 30 && s.Steps[2].MaxPercent == 80
		}},
		{"two-layers", "layers", "Keep the carrier band and speed. Layer a 40-percent range swell over 8 cycles and a 30-percent pace modulation over 12 cycles. Both phases zero.", func(s motion.FlowSpec) bool {
			return labHasLayer(s, "range", 40, 8, 0) && labHasLayer(s, "pace", 30, 12, 0) && len(s.Layers) == 2 && s.SpeedPercent == 25
		}},
		{"three-layers", "layers", "Use three layers at once: range 60 percent, period 6 cycles, phase 0; center 30 percent, period 16 cycles, phase 25; pace 20 percent, period 8 cycles, phase 50. Keep base controls unchanged.", func(s motion.FlowSpec) bool {
			return len(s.Layers) == 3 && labHasLayer(s, "range", 60, 6, 0) && labHasLayer(s, "center", 30, 16, 25) && labHasLayer(s, "pace", 20, 8, 50)
		}},
	}
	records := []labEvaluationTrial{}
	failures := 0
	for _, model := range models {
		for repeat := 0; repeat < 2; repeat++ {
			for _, test := range cases {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				guided := os.Getenv("MAGICHANDY_LAB_SCHEMA") == "1" || (os.Getenv("MAGICHANDY_LAB_SCHEMA") == "auto" && test.method != "controls")
				trial := RunLLMLab(ctx, provider, model, test.method, LLMLabPrompts()[test.method], test.prompt, motion.DefaultFlowSpec(), limits, nil, guided)
				cancel()
				accepted := trial.Valid && test.accepts(trial.After) && labTrialPreservesUnrequested(test.name, trial.Changed)
				if accepted {
					_, err := motion.FlowTarget(trial.After, limits)
					accepted = err == nil
				}
				if !accepted {
					failures++
				}
				records = append(records, labEvaluationTrial{LLMLabTrial: trial, Expected: test.name, Intent: accepted})
				t.Logf("model=%s repeat=%d case=%s valid=%t semantic=%t elapsed=%d error=%s raw=%s", model, repeat, test.name, trial.Valid, accepted, trial.ElapsedMillis, trial.Error, trial.Raw)
			}
		}
	}
	if report := os.Getenv("MAGICHANDY_LAB_REPORT"); report != "" {
		encoded, _ := json.MarshalIndent(records, "", "  ")
		if err := os.WriteFile(report, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if failures > 0 {
		t.Errorf("%d/%d trials failed strict syntax, semantic intent or compilation; raw evidence retained", failures, len(records))
	}
}

func labTrialPreservesUnrequested(name string, changed []string) bool {
	allowed := map[string][]string{
		"anchor": {"anchor_percent"}, "full-band": {"min_percent", "max_percent"},
		"memory": {"memory_cycles"}, "negation": {"pace_variation_percent"}, "chat-only": {},
		"two-sections": {"steps"}, "three-sections": {"steps"}, "two-layers": {"layers"}, "three-layers": {"layers"},
	}
	for _, field := range changed {
		if !slices.Contains(allowed[name], field) {
			return false
		}
	}
	return true
}

func labHasLayer(spec motion.FlowSpec, axis string, amount, period, phase int) bool {
	for _, layer := range spec.Layers {
		if layer.Axis == axis && layer.AmountPercent == amount && layer.PeriodCycles == period && layer.PhasePercent == phase {
			return true
		}
	}
	return false
}

func TestLLMLabLiveContinuations(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_LAB_MODELS"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_LAB_MODELS")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: models[0]})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		for _, method := range []string{"controls", "sequence", "layers"} {
			current := motion.DefaultFlowSpec()
			wanted := motion.DefaultFlowSpec()
			history := []llm.Message{}
			requests := []string{"Hold the tip end while varying stroke length. Keep speed and the outer band unchanged.", "Now set speed to 35. Preserve every other control, including the anchor.", "Keep this exact score unchanged and briefly explain it."}
			if method == "sequence" {
				requests = []string{"Arrange four cycles from 0 to 100 at speed 25, then two cycles from 60 to 100 at speed 35.", "Keep that sequence unchanged. Set the range anchor to 100.", "Keep this exact score unchanged and briefly explain it."}
			}
			if method == "layers" {
				requests = []string{"Layer a range modulation of 40 percent over 8 cycles at phase zero. Keep the base controls.", "Keep that range layer and base controls. Add a pace layer: 30 percent, 12 cycles, phase zero.", "Keep this exact score unchanged and briefly explain it."}
			}
			for index, message := range requests {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				trial := RunLLMLab(ctx, provider, model, method, LLMLabPrompts()[method], message, current, limits, history, method != "controls")
				cancel()
				wanted = labContinuationWanted(wanted, method, index)
				okay := trial.Valid && len(labChangedControls(wanted, trial.After)) == 0
				t.Logf("model=%s method=%s turn=%d okay=%t valid=%t elapsed=%d raw=%s", model, method, index, okay, trial.Valid, trial.ElapsedMillis, trial.Raw)
				if !okay {
					t.Errorf("continuation failed: %s/%s/%d: %s", model, method, index, trial.Error)
				}
				if trial.Valid {
					current = trial.After
				}
				history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: trial.Raw})
			}
		}
	}
}

func labContinuationWanted(wanted motion.FlowSpec, method string, index int) motion.FlowSpec {
	if index == 2 {
		return wanted
	}
	switch method {
	case "controls":
		wanted.AnchorPercent = 100
		if index == 1 {
			wanted.SpeedPercent = 35
		}
	case "sequence":
		wanted.Steps = []motion.FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: 25, Cycles: 4}, {MinPercent: 60, MaxPercent: 100, SpeedPercent: 35, Cycles: 2}}
		if index == 1 {
			wanted.AnchorPercent = 100
		}
	case "layers":
		wanted.Layers = []motion.FlowLayer{{Axis: "range", AmountPercent: 40, PeriodCycles: 8}}
		if index == 1 {
			wanted.Layers = append(wanted.Layers, motion.FlowLayer{Axis: "pace", AmountPercent: 30, PeriodCycles: 12})
		}
	}
	return wanted
}
