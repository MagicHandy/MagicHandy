package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

var libraryActionHandles = map[motion.PatternID]string{
	motion.PatternFullSweeps: "sweep_full_range", "flow-lower-strokes": "stroke_lower_region",
	"flow-middle-strokes": "stroke_middle_region", "flow-upper-strokes": "stroke_upper_region",
	motion.PatternBaseVariation: "vary_reach_from_base", "flow-tip-anchored": "vary_reach_from_tip",
	"flow-centered-variety": "vary_width_around_middle", "flow-traveling-window": "move_fixed_width_window",
	"flow-wide-narrow": "alternate_full_and_middle", motion.PatternPaceWave: "wave_pace_keep_full_range",
	"flow-base-drift": "drift_reach_from_base", "flow-tip-drift": "drift_reach_from_tip",
	"flow-centered-drift": "drift_width_around_middle", "flow-soft-sweeps": "soften_full_range_turns",
	"flow-even-beat": "vary_width_keep_even_beat", "flow-zone-tour": "tour_lower_middle_upper",
	"flow-breathing-window": "move_and_resize_window",
}

func libraryLabHandle(method string, id motion.PatternID) string {
	switch method {
	case "library_descriptive":
		return string(id)
	case "library_actions":
		return libraryActionHandles[id]
	default:
		return modelPatternID(string(id))
	}
}

func libraryLabPrompt(method string) string {
	var prompt strings.Builder
	prompt.WriteString(labPlanningContextGuide)
	fmt.Fprintf(&prompt, `First decide whether the user asks to change movement shape. If yes, copy the single best catalog ID into recipe_id BEFORE writing reply. An ID in reply does NOT select any motion. Do not set speed_percent for a request that says keep pace unchanged.
Select the most specific complete recipe: soft turnarounds already include full sweeps; irregular drift differs from repeating width waves; a fixed-width traveling window differs from a varying-width breathing window. Do not combine recipe names.
Complete shape-change example: {"recipe_id":"%s","reply":"Switching to fixed middle strokes."}
Complete no-change example: {"reply":"Keeping the current motion."}
Now follow the complete contract and choose from the catalog below.
`, libraryLabHandle(method, "flow-middle-strokes"))
	prompt.WriteString(`You are the conversational assistant in a motion testing session. Reply naturally to the user, including questions that need no motion change. The app may apply accepted semantic changes during a live test. Return one JSON object with a brief "reply", optional "recipe_id", and optional "speed_percent". No action, controls, steps, layers, raw points or extra fields.
Choose a recipe only when a movement change is requested. Omitted recipe_id preserves the CURRENT SCORE. A pace-only request uses speed_percent alone and preserves the movement shape. Omitted speed_percent preserves the current pace setting. Speed stays inside SAVED LIMITS. If there is no requested change, return only reply.
Fixed-region motion contains both endpoints in one zone. Varying anchored reach holds one endpoint while length changes. A traveling window moves both endpoints together. These describe different movements; pace is independent.
The catalog rows are id | name | behavior. Copy exactly one supplied id when selecting a recipe.
`)
	for _, recipe := range motion.ContinuousRecipes(25) {
		fmt.Fprintf(&prompt, "%s | %s | %s\n", libraryLabHandle(method, recipe.ID), recipe.Name, recipe.Description)
	}
	prompt.WriteString(`Complete response for a pace-only edit: {"reply":"Changing only the pace setting.","speed_percent":25}. For a shape change include recipe_id in that same root object. Only emitted fields take effect; make reply describe the accepted change.`)
	return prompt.String()
}

func libraryLabSchema(method string, limits config.MotionSettings) json.RawMessage {
	ids := []string{}
	for _, recipe := range motion.ContinuousRecipes(25) {
		ids = append(ids, libraryLabHandle(method, recipe.ID))
	}
	encoded, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"reply"}, "additionalProperties": false,
		"properties": map[string]any{"reply": map[string]any{"type": "string"},
			"recipe_id":     map[string]any{"type": "string", "enum": ids},
			"speed_percent": map[string]any{"type": "integer", "minimum": limits.SpeedMinPercent, "maximum": limits.SpeedMaxPercent}}})
	return encoded
}

func parseLibraryLab(raw, method string, current motion.FlowSpec, limits config.MotionSettings) (string, motion.FlowSpec, []string, error) {
	var proposal struct {
		Reply    string  `json:"reply"`
		RecipeID *string `json:"recipe_id"`
		Speed    *int    `json:"speed_percent"`
	}
	if err := decodeLabObject(raw, &proposal); err != nil {
		return "", current, nil, err
	}
	if strings.TrimSpace(proposal.Reply) == "" || len(proposal.Reply) > 2000 {
		return "", current, nil, errors.New("a brief reply is required")
	}
	next := current
	if proposal.RecipeID != nil {
		found := false
		for _, recipe := range motion.ContinuousRecipes(current.SpeedPercent) {
			if *proposal.RecipeID == libraryLabHandle(method, recipe.ID) {
				next = recipe.Spec
				next.Seed = current.Seed
				found = true
				break
			}
		}
		if !found {
			return "", current, nil, errors.New("choose a recipe ID from the selected naming interface")
		}
	}
	if proposal.Speed != nil {
		next.SpeedPercent = *proposal.Speed
		next.Steps = append([]motion.FlowStep(nil), next.Steps...)
		for index := range next.Steps {
			next.Steps[index].SpeedPercent = *proposal.Speed
		}
	}
	if err := next.Validate(limits); err != nil {
		return "", current, nil, err
	}
	return proposal.Reply, next, labChangedControls(current, next), nil
}

func labCurrentRecipe(method string, current motion.FlowSpec) (string, string) {
	encoded, _ := json.Marshal(current)
	for _, recipe := range motion.ContinuousRecipes(current.SpeedPercent) {
		spec := recipe.Spec
		spec.Seed = current.Seed
		candidate, _ := json.Marshal(spec)
		if string(candidate) == string(encoded) {
			return libraryLabHandle(method, recipe.ID), recipe.Name
		}
	}
	return "custom", ""
}
