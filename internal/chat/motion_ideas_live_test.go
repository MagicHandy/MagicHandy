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

type motionIdeaCase struct {
	name, baseline, message string
	before, wanted          motion.FlowSpec
}

func motionIdeaCases() []motionIdeaCase {
	base := motion.DefaultFlowSpec()
	makeCase := func(name, method, message string, change func(*motion.FlowSpec)) motionIdeaCase {
		wanted := base
		change(&wanted)
		return motionIdeaCase{name, method, message, base, wanted}
	}
	cases := []motionIdeaCase{
		makeCase("slower", "controls", "Make the speed 5 percentage points slower. Keep everything else exactly as it is.", func(s *motion.FlowSpec) { s.SpeedPercent -= 5 }),
		makeCase("faster", "controls", "Increase speed by exactly 4 percentage points; leave range, anchor and variation alone.", func(s *motion.FlowSpec) { s.SpeedPercent += 4 }),
		makeCase("longer-trends", "controls", "Increase range memory by 4 cycles, without changing speed or the amount of pace variation.", func(s *motion.FlowSpec) { s.MemoryCycles += 4 }),
		makeCase("drift", "controls", "Use correlated drift instead of waves for the variation. Preserve the band, pace and every other control.", func(s *motion.FlowSpec) { s.VariationMode = "drift" }),
		makeCase("soft-turns", "controls", "Set turn softness to 70 percent. Keep the rhythm and range unchanged.", func(s *motion.FlowSpec) { s.TurnSoftnessPercent = 70 }),
		makeCase("steady-beat", "controls", "Set cadence hold to 100 for a steadier beat as reach changes. Keep the speed and pace variation unchanged.", func(s *motion.FlowSpec) { s.CadenceHoldPercent = 100 }),
		makeCase("remove-pace-variation", "controls", "Do not change speed or range. Remove only the pace variation.", func(s *motion.FlowSpec) { s.PaceVariationPercent = 0 }),
		makeCase("chat-only", "controls", "Explain what this preview estimates. Do not change any control, layer or section.", func(*motion.FlowSpec) {}),
	}
	layered := base
	layered.Layers = []motion.FlowLayer{{Axis: "range", AmountPercent: 40, PeriodCycles: 8}, {Axis: "center", AmountPercent: 30, PeriodCycles: 16, PhasePercent: 25}}
	wanted := layered
	wanted.Layers = append(append([]motion.FlowLayer{}, layered.Layers...), motion.FlowLayer{Axis: "pace", AmountPercent: 20, PeriodCycles: 12})
	cases = append(cases, motionIdeaCase{"add-layer", "layers", "Keep the existing range and center layers. Add a pace layer at 20 percent, 12 cycles, phase zero. Preserve all base controls.", layered, wanted})
	wanted = layered
	wanted.Layers = append([]motion.FlowLayer{}, layered.Layers...)
	wanted.Layers[0].AmountPercent = 60
	cases = append(cases, motionIdeaCase{"edit-layer", "layers", "Change only the range layer amount to 60 percent. Preserve its period and phase, the center layer, and all base controls.", layered, wanted})
	wanted = layered
	wanted.Layers = []motion.FlowLayer{layered.Layers[0]}
	cases = append(cases, motionIdeaCase{"remove-layer", "layers", "Remove only the center layer. Keep the range layer and all base controls exactly unchanged.", layered, wanted})
	sectioned := base
	sectioned.Steps = []motion.FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: 20, Cycles: 4}, {MinPercent: 60, MaxPercent: 100, SpeedPercent: 35, Cycles: 2}}
	wanted = sectioned
	wanted.SpeedPercent -= 5
	wanted.Steps = append([]motion.FlowStep{}, sectioned.Steps...)
	wanted.Steps[0].SpeedPercent -= 5
	wanted.Steps[1].SpeedPercent -= 5
	cases = append(cases, motionIdeaCase{"section-relative-pace", "sequence", "Reduce the global speed and each section's speed by exactly 5 points. Preserve the differences between section speeds, their ranges, cycle counts and order.", sectioned, wanted})
	return cases
}

func TestMotionIdeasLive(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_LAB_MODELS"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_LAB_MODELS to installed Ollama models")
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: models[0]})
	if err != nil {
		t.Fatal(err)
	}
	records := []labEvaluationTrial{}
	failures := 0
	for _, model := range models {
		for repeat := 0; repeat < 2; repeat++ {
			for _, test := range motionIdeaCases() {
				formats := []struct {
					method string
					guided bool
				}{{test.baseline, test.baseline != "controls"}, {"edits", true}}
				if test.baseline == "controls" {
					formats = append(formats, struct {
						method string
						guided bool
					}{"controls", true})
				}
				for _, format := range formats {
					method, guided := format.method, format.guided
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					trial := RunLLMLab(ctx, provider, model, method, LLMLabPrompts()[method], test.message, test.before, limits, nil, guided)
					cancel()
					accepted := trial.Valid && len(labChangedControls(test.wanted, trial.After)) == 0
					if accepted {
						_, compileErr := motion.FlowTarget(trial.After, limits)
						accepted = compileErr == nil
					}
					if !accepted {
						failures++
					}
					records = append(records, labEvaluationTrial{LLMLabTrial: trial, Expected: test.name, Intent: accepted})
					t.Logf("model=%s method=%s schema=%t repeat=%d case=%s valid=%t intent=%t time=%d raw=%s", model, method, guided, repeat, test.name, trial.Valid, accepted, trial.ElapsedMillis, trial.Raw)
				}
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
		t.Errorf("%d/%d motion-idea trials failed exact intent/preservation; all raw results retained", failures, len(records))
	}
}

// A separate continuation scenario uses different wording, values and history
// after the default prompt is frozen. It feeds accepted output forward exactly
// as the Lab does, and scores each edit against that actual starting state.
func TestMotionIdeasConversationLive(t *testing.T) {
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
	records := []labEvaluationTrial{}
	for _, model := range models {
		current := motion.DefaultFlowSpec()
		current.SpeedPercent, current.MemoryCycles, current.AnchorPercent = 30, 11, 35
		current.Layers = []motion.FlowLayer{{Axis: "range", AmountPercent: 35, PeriodCycles: 10, PhasePercent: 37}, {Axis: "center", AmountPercent: 45, PeriodCycles: 18, PhasePercent: 60}}
		history := []llm.Message{}
		for _, test := range []struct {
			name, message string
			change        func(*motion.FlowSpec)
		}{
			{"holdout-slower", "Take seven points off the current speed; leave all other settings alone.", func(s *motion.FlowSpec) { s.SpeedPercent -= 7 }},
			{"holdout-layer", "Give the range layer a strength of 55. Don't alter its cycle length or phase; keep the other layer.", func(s *motion.FlowSpec) {
				for i := range s.Layers {
					if s.Layers[i].Axis == "range" {
						s.Layers[i].AmountPercent = 55
					}
				}
			}},
			{"holdout-remove", "Drop the center modulation, but keep the range modulation.", func(s *motion.FlowSpec) {
				layers := []motion.FlowLayer{}
				for _, l := range s.Layers {
					if l.Axis != "center" {
						layers = append(layers, l)
					}
				}
				s.Layers = layers
			}},
			{"holdout-two-controls", "Use drift and 55 for turn softness. Hold every other field fixed.", func(s *motion.FlowSpec) { s.VariationMode = "drift"; s.TurnSoftnessPercent = 55 }},
			{"holdout-memory", "I want three more cycles of memory; keep the speed the same.", func(s *motion.FlowSpec) { s.MemoryCycles += 3 }},
			{"holdout-question", "Tell me what you changed. Keep the current score as-is.", func(*motion.FlowSpec) {}},
		} {
			encoded, _ := json.Marshal(current)
			var wanted motion.FlowSpec
			_ = json.Unmarshal(encoded, &wanted)
			test.change(&wanted)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			trial := RunLLMLab(ctx, provider, model, "edits", LLMLabPrompts()["edits"], test.message, current, limits, history, true)
			cancel()
			accepted := trial.Valid && len(labChangedControls(wanted, trial.After)) == 0
			if accepted {
				_, err = motion.FlowTarget(trial.After, limits)
				accepted = err == nil
			}
			records = append(records, labEvaluationTrial{LLMLabTrial: trial, Expected: test.name, Intent: accepted})
			t.Logf("model=%s case=%s valid=%t intent=%t raw=%s", model, test.name, trial.Valid, accepted, trial.Raw)
			if !accepted {
				t.Errorf("continuation failed: %s / %s", model, test.name)
			}
			if trial.Valid {
				current = trial.After
			}
			history = append(history, llm.Message{Role: "user", Content: test.message}, llm.Message{Role: "assistant", Content: trial.Raw})
		}
	}
	if report := os.Getenv("MAGICHANDY_LAB_REPORT"); report != "" {
		encoded, _ := json.MarshalIndent(records, "", "  ")
		if err := os.WriteFile(report, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
