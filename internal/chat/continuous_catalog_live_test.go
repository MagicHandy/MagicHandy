//go:build liveeval

package chat

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type catalogTrial struct {
	Model, Naming, Request, Raw, Error, Selected, Expected, Prompt string
	Valid, Intent                                                  bool
	Millis                                                         int64
	PromptBytes                                                    int
	Output                                                         motion.PerceptualSummary
	Command                                                        *MotionCommand
}

var actionHandles = map[motion.PatternID]string{
	motion.PatternFullSweeps:    "sweep_full_range",
	"flow-lower-strokes":        "stroke_lower_region",
	"flow-middle-strokes":       "stroke_middle_region",
	"flow-upper-strokes":        "stroke_upper_region",
	motion.PatternBaseVariation: "vary_reach_from_base",
	"flow-tip-anchored":         "vary_reach_from_tip",
	"flow-centered-variety":     "vary_width_around_middle",
	"flow-traveling-window":     "move_fixed_width_window",
	"flow-wide-narrow":          "alternate_full_and_middle",
	motion.PatternPaceWave:      "wave_pace_keep_full_range",
}

func TestContinuousCatalogLiveNaming(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_LAB_MODELS"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_LAB_MODELS")
	}
	variants := strings.Split(os.Getenv("MAGICHANDY_CATALOG_NAMING"), "|")
	if variants[0] == "" {
		variants = []string{"opaque", "descriptive", "actions"}
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	tests := []struct {
		request  string
		expected motion.PatternID
		speed    int
		current  motion.PatternID
	}{
		{"Use smooth strokes through the entire length, from base to tip, with an even pace. Keep speed 25.", motion.PatternFullSweeps, 25, "flow-middle-strokes"},
		{"Use only the lower region near the base, with a fixed stroke width. Keep speed 25.", "flow-lower-strokes", 25, motion.PatternFullSweeps},
		{"Stay around the middle with fixed-width strokes. Keep speed 25.", "flow-middle-strokes", 25, motion.PatternFullSweeps},
		{"Stay near the tip with fixed-width strokes. Keep speed 25.", "flow-upper-strokes", 25, motion.PatternFullSweeps},
		{"Return to the base on every stroke, but gradually vary how far each stroke reaches. Keep speed 25.", motion.PatternBaseVariation, 25, motion.PatternFullSweeps},
		{"Keep returning to the tip while the stroke length gradually varies. Keep speed 25.", "flow-tip-anchored", 25, motion.PatternFullSweeps},
		{"Vary the stroke width around the middle, moving both endpoints equally. Do not anchor either end. Keep speed 25.", "flow-centered-variety", 25, motion.PatternFullSweeps},
		{"Keep stroke width constant while the whole window travels from lower to middle to upper and back. Keep speed 25.", "flow-traveling-window", 25, motion.PatternFullSweeps},
		{"Arrange four full-length cycles followed by four narrower middle cycles, repeating that order at speed 25.", "flow-wide-narrow", 25, motion.PatternFullSweeps},
		{"Gradually vary the pace in a wave while keeping every stroke full-length. Use speed 25 as the pace setting.", motion.PatternPaceWave, 25, motion.PatternFullSweeps},
		{"Increase speed to 35. Keep the exact same movement shape and range.", motion.PatternBaseVariation, 35, motion.PatternBaseVariation},
		{"Keep the exact same motion and pace. Just confirm that you understand.", motion.PatternBaseVariation, 25, motion.PatternBaseVariation},
		{"Stop motion now.", "stop", 25, motion.PatternFullSweeps},
	}
	records := []catalogTrial{}
	failures := 0
	for _, model := range models {
		provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		for _, variant := range variants {
			choices, handles, ids := catalogNamingChoices(variant)
			for repeat := 0; repeat < 2; repeat++ {
				for _, test := range tests {
					state := MotionContext{Running: true, PatternID: handles[test.current], SpeedPercent: 25, Area: AreaZoneFull, SpeedMinPercent: 10, SpeedMaxPercent: 43, MotionMode: MotionModePattern}
					prompt := composeSystem(PromptSet{System: "Explain the requested movement briefly and accurately. Keep the reply technical and concise."}, nil, choices, FullCapabilities(), &state, nil)
					for _, handle := range handles {
						prompt = strings.ReplaceAll(prompt, modelPatternID(handle), handle)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
					start := time.Now()
					request := llm.ChatRequest{Model: model, Temperature: 0.1, MaxTokens: 768, ReasoningMode: "off", Messages: []llm.Message{{Role: "system", Content: prompt}, {Role: "user", Content: test.request}}}
					if os.Getenv("MAGICHANDY_CATALOG_SCHEMA") == "1" {
						schema := string(PatternResponseSchema(choices, FullCapabilities(), &state))
						for _, handle := range handles {
							schema = strings.ReplaceAll(schema, modelPatternID(handle), handle)
						}
						request.JSONSchema = json.RawMessage(schema)
					}
					raw, callErr := provider.StreamChat(ctx, request, func(string) error { return nil })
					cancel()
					trial := catalogTrial{Model: model, Naming: variant, Request: test.request, Raw: raw, Expected: string(test.expected), Prompt: prompt, Millis: time.Since(start).Milliseconds(), PromptBytes: len(prompt)}
					if callErr != nil {
						trial.Error = callErr.Error()
					} else {
						response, parseErr := parseAssistantResponseForCapabilities(raw, choices, FullCapabilities(), &state)
						if parseErr != nil {
							trial.Error = parseErr.Error()
						} else {
							trial.Valid = true
							trial.Command = response.Motion
							selected, speed := test.current, 25
							if response.Motion != nil {
								if response.Motion.PatternID != "" {
									selected = ids[response.Motion.PatternID]
								}
								if response.Motion.SpeedPercent != nil {
									speed = *response.Motion.SpeedPercent
								}
								if response.Motion.Action == MotionActionStop {
									selected = "stop"
								}
							}
							trial.Selected = string(selected)
							trial.Intent = selected == test.expected && speed == test.speed
							if selected != "stop" {
								target := motion.MotionTarget{PatternID: selected, SpeedPercent: speed}
								if response.Motion != nil {
									target.AreaFocus = catalogEvalFocus(response.Motion.Area)
								}
								plan := motion.NewMotionPlan("live-library-eval", target, limits, 0, 0, time.Unix(0, 0))
								trial.Output = plan.Perceptual
								if !catalogGeometryMatches(test.expected, plan.Perceptual) {
									trial.Intent = false
									trial.Error = "compiled range does not match the requested behavior"
								}
								if plan.Target.PatternName == "" || plan.Perceptual.CommandedMeanTravelPerSecond <= 0 {
									trial.Intent = false
									trial.Error = "selected action did not compile continuous motion"
								}
							}
						}
					}
					if !trial.Valid || !trial.Intent {
						failures++
						t.Logf("FAIL %s %s %s -> %s %s", model, variant, test.expected, trial.Selected, trial.Error)
					}
					records = append(records, trial)
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
	t.Logf("catalog mapping passed %d/%d", len(records)-failures, len(records))
	if failures > 0 {
		t.Errorf("%d model intent/structure failures; raw evidence retained", failures)
	}
}

func catalogEvalFocus(area string) *motion.AreaFocus {
	switch area {
	case AreaZoneTip:
		return &motion.AreaFocus{MinPercent: 66, MaxPercent: 100}
	case AreaZoneShaft:
		return &motion.AreaFocus{MinPercent: 33, MaxPercent: 67}
	case AreaZoneBase:
		return &motion.AreaFocus{MinPercent: 0, MaxPercent: 34}
	}
	return nil
}

func catalogGeometryMatches(id motion.PatternID, summary motion.PerceptualSummary) bool {
	lo, hi := summary.PositionMinPercent, summary.PositionMaxPercent
	switch id {
	case motion.PatternFullSweeps, motion.PatternPaceWave:
		return lo <= 1 && hi >= 99
	case "flow-lower-strokes":
		return lo <= 1 && hi <= 40.01 && hi-lo >= 30
	case "flow-middle-strokes":
		return lo >= 29.99 && hi <= 70.01 && hi-lo >= 30
	case "flow-upper-strokes":
		return lo >= 59.99 && hi >= 99 && hi-lo >= 30
	case motion.PatternBaseVariation:
		return lo <= 1 && hi >= 80 && summary.StrokeLengthCV > .08
	case "flow-tip-anchored":
		return hi >= 99 && lo <= 20 && summary.StrokeLengthCV > .08
	case "flow-centered-variety":
		return lo < 20 && hi > 80 && summary.StrokeLengthCV > .08
	case "flow-traveling-window":
		return lo < 5 && hi > 95
	case "flow-wide-narrow":
		return lo < 1 && hi > 99 && summary.StrokeLengthCV > .08
	}
	return true
}

func catalogNamingChoices(variant string) ([]PatternChoice, map[motion.PatternID]string, map[string]motion.PatternID) {
	choices := []PatternChoice{}
	handles, ids := map[motion.PatternID]string{}, map[string]motion.PatternID{}
	for _, recipe := range motion.ContinuousRecipes(25) {
		handle := string(recipe.ID)
		switch variant {
		case "opaque":
			digest := sha256.Sum256([]byte(recipe.ID))
			handle = fmt.Sprintf("p-%x", digest[:6])
		case "actions":
			handle = actionHandles[recipe.ID]
		}
		handles[recipe.ID], ids[handle] = handle, recipe.ID
		choices = append(choices, PatternChoice{ID: handle, Name: recipe.Name, Description: recipe.Description, Tags: []string{"continuous", "smooth"}, Weight: 1})
	}
	return choices, handles, ids
}
