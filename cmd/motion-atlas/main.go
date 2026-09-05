//go:build magichandy_labs

// motion-atlas exports inert shared-engine output for visual regression review.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func main() {
	output := flag.String("output", ".scratch/motion-atlas.json", "JSON output path")
	speeds := flag.String("speeds", "10,25,43", "comma-separated comparison speeds")
	legacy := flag.Bool("legacy", true, "include deprecated built-ins")
	catalog := flag.Bool("catalog", true, "include built-in library patterns")
	experiments := flag.Bool("experiments", false, "include the guided flow experiment roster and a maximum-softness case")
	creativeV2 := flag.Bool("creative-v2", false, "include the native gesture matrix at 10/45/85 across all device profiles, plus original Creative comparisons")
	reports := flag.String("llm", "", "comma-separated live LLM report paths")
	sessions := flag.String("sessions", "", "comma-separated full-app Autopilot captures")
	flag.Parse()
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 10, 43
	entries := []motion.Review{}
	comparisonSpeeds := []int{}
	for _, text := range strings.Split(*speeds, ",") {
		speed, err := strconv.Atoi(text)
		must(err)
		if speed < 10 || speed > 43 {
			must(fmt.Errorf("comparison speed must be inside 10–43"))
		}
		comparisonSpeeds = append(comparisonSpeeds, speed)
	}
	for _, definition := range motion.BuiltinPatternDefinitions() {
		deprecated := slices.Contains(definition.Tags, motion.TagDeprecated)
		if !*catalog || (deprecated && !*legacy) {
			continue
		}
		for _, speed := range comparisonSpeeds {
			entry := motion.ReviewMotionOutput(motion.MotionTarget{PatternID: definition.ID, Pattern: &definition, SpeedPercent: speed}, settings)
			entry.Group = "new-library"
			if deprecated {
				entry.Group = "legacy-library"
			}
			entry.Description = definition.Description
			entries = append(entries, entry)
		}
	}
	if *experiments {
		entries = append(entries, renderExperiments(comparisonSpeeds, settings)...)
	}
	if *creativeV2 {
		entries = append(entries, renderCreativeV2Matrix()...)
	}
	for _, path := range strings.Split(*reports, ",") {
		if path == "" {
			continue
		}
		entries = append(entries, readTrials(path, settings)...)
	}
	for _, path := range strings.Split(*sessions, ",") {
		if path != "" {
			entries = append(entries, readAutopilotSessions(path)...)
		}
	}
	data, err := json.MarshalIndent(map[string]any{"schema": "magichandy.motion-atlas.v1", "settings": settings, "entries": entries}, "", "  ")
	must(err)
	must(os.WriteFile(*output, data, 0600))
	fmt.Printf("Exported %d motion cases to %s; no transport or playback loop was created.\n", len(entries), *output)
}

func renderExperiments(speeds []int, settings config.MotionSettings) []motion.Review {
	entries := []motion.Review{}
	for _, speed := range speeds {
		base := motion.DefaultFlowSpec()
		base.SpeedPercent = speed
		roster := motion.FlowExperiments(base)
		maximum := roster[len(roster)-1]
		maximum.ID, maximum.Name = "maximum-softness", "Maximum softness and drift"
		maximum.Spec.TurnSoftnessPercent = 100
		maximum.Description = "Check the most concentrated middle-stroke travel and the longer turnarounds."
		roster = append(roster, maximum)
		for _, experiment := range roster {
			target, err := motion.FlowTarget(experiment.Spec, settings)
			must(err)
			entry := motion.ReviewMotionOutput(target, settings)
			entry.ID, entry.Name, entry.Group, entry.Description = experiment.ID, experiment.Name, "flow-experiments", experiment.Description
			entries = append(entries, entry)
		}
	}
	return entries
}

func readTrials(path string, settings config.MotionSettings) []motion.Review {
	data, err := os.ReadFile(path) // #nosec G304 -- explicit local CLI input; this development tool has no server or device access.
	must(err)
	data, captured := unwrapTrials(data, path)
	if captured != nil {
		return captured
	}
	return renderTrials(data, settings)
}

func unwrapTrials(data []byte, path string) ([]byte, []motion.Review) {
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		var report struct {
			Model string
			Turns json.RawMessage
		}
		must(json.Unmarshal(data, &report))
		if len(report.Turns) == 0 {
			must(fmt.Errorf("%s: expected a trial array or an exported object containing turns", path))
		}
		var turns []struct {
			Request, Response string
			Motion            *motion.Review
			IntentPass        *bool `json:"intent_pass"`
		}
		must(json.Unmarshal(report.Turns, &turns))
		if len(turns) > 0 && turns[0].Motion != nil {
			entries := []motion.Review{}
			for _, turn := range turns {
				if turn.Motion == nil {
					entries = append(entries, motion.Review{Group: "llm-output", Model: report.Model, Request: turn.Request, Raw: turn.Response,
						Name: "Uncaptured model turn", Error: "No accepted motion target captured for this turn; inspect the original response.", Outcome: "Failed live selection retained"})
					continue
				}
				entry := *turn.Motion
				entry.Group, entry.Model, entry.Request, entry.Raw = "llm-output", report.Model, turn.Request, turn.Response
				entry.Outcome = "Live app path: captured target; inspect the response and fixture result for intent"
				if turn.IntentPass != nil {
					if *turn.IntentPass {
						entry.Outcome = "Intent pass: live app target and captured transport"
					} else {
						entry.Outcome = "Intent failure: accepted live target retained for review"
					}
				}
				entries = append(entries, entry)
			}
			return nil, entries
		}
		data = report.Turns // The LLM Lab's Export comparison wraps trials in a state object.
	}
	return data, nil
}

func renderTrials(data []byte, settings config.MotionSettings) []motion.Review {
	var rows []struct {
		Model, Method, Message, Request, Raw, Error, Selected, Expected string
		RecipeName                                                      string `json:"recipe_name"`
		ExpectedRecipe                                                  string `json:"expected_recipe"`
		IntentPass                                                      *bool  `json:"intent_pass"`
		Intent                                                          bool
		Command                                                         *struct {
			Area         string `json:"area"`
			SpeedPercent *int   `json:"speed_percent"`
		}
		Valid  bool
		Limits *config.MotionSettings
		After  *motion.FlowSpec
		Output motion.PerceptualSummary
	}
	must(json.Unmarshal(data, &rows))
	entries := make([]motion.Review, 0, len(rows))
	for index, row := range rows {
		if row.Expected == "" {
			row.Expected = row.ExpectedRecipe
		}
		if row.IntentPass != nil {
			row.Intent = *row.IntentPass
		}
		entry := motion.Review{ID: fmt.Sprintf("trial-%d", index+1), Name: fmt.Sprintf("LLM trial %d", index+1)}
		switch {
		case !row.Valid:
			entry.Error = row.Error
			if entry.Error == "" {
				entry.Error = "Rejected model response; no motion compiled"
			}
		case row.After != nil:
			trialLimits := settings
			if row.Limits != nil {
				trialLimits = *row.Limits
			}
			target, err := motion.FlowTarget(*row.After, trialLimits)
			if err != nil {
				entry.Error = err.Error()
			} else {
				entry = motion.ReviewMotionOutput(target, trialLimits)
			}
		case row.Selected == "stop":
			entry.Name = "Stop"
			entry.Error = "Stop action: no moving output"
		case row.Selected != "":
			target := motion.MotionTarget{PatternID: motion.PatternID(row.Selected), SpeedPercent: row.Output.Pace.RequestedPercent}
			if row.Command != nil {
				switch row.Command.Area {
				case "base":
					target.AreaFocus = &motion.AreaFocus{MinPercent: 0, MaxPercent: 34}
				case "shaft":
					target.AreaFocus = &motion.AreaFocus{MinPercent: 33, MaxPercent: 67}
				case "tip":
					target.AreaFocus = &motion.AreaFocus{MinPercent: 66, MaxPercent: 100}
				}
			}
			entry = motion.ReviewMotionOutput(target, settings)
		default:
			entry.Error = "No compiled action recorded"
		}
		entry.Group, entry.Model, entry.Raw = "llm-output", row.Model, row.Raw
		if row.RecipeName != "" {
			entry.Name = row.RecipeName
		}
		if row.Expected != "" {
			entry.Outcome = "Intent mismatch; expected " + row.Expected
			if row.Intent {
				entry.Outcome = "Intent check passed"
			}
		}
		if row.Method != "" {
			entry.Outcome = row.Method + ": " + entry.Outcome
		}
		if !row.Valid {
			entry.Outcome = "Rejected structure"
		}
		entry.Request = row.Request
		if entry.Request == "" {
			entry.Request = row.Message
		}
		entries = append(entries, entry)
	}
	return entries
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
