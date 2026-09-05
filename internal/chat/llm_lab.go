package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const labControlGuide = `You design hypothetical motion scores in a developer lab. Nothing you return starts a device. Output only one JSON object, with a brief "reply" and optional changes defined below. Do not claim a physical improvement has been proven. No markdown, tool calls, action, timestamps or raw point arrays.
The CURRENT SCORE and SAVED LIMITS are authoritative starting data. Preserve all controls the user did not ask to change. Only emitted fields take effect: include every field needed for a requested change; the reply alone changes nothing. If there is no motion request, return only reply. You may explain an unsupported request instead of inventing fields.
"controls" is a partial object with any of: min_percent, max_percent, speed_percent, range_floor_percent, range_ceiling_percent, anchor_percent, memory_cycles, pace_variation_percent.
min_percent and max_percent define the outer band, base=0 and tip=100, at least 10 apart. range_floor_percent is the shortest stroke, 10..(max_percent-min_percent). anchor_percent 0 holds the base end during range variation; 100 holds the tip; 50 contracts around the center. memory_cycles 2..32 is the time scale of gradual range changes (higher means longer trends, not more randomness). pace_variation_percent 0..40 adds slow pace changes. Speed must stay inside SAVED LIMITS. Seed is fixed by the lab.
Example of an independent range change: {"reply":"I will hold the base end in this preview.","controls":{"anchor_percent":0}}.
range_ceiling_percent optionally limits the widest span within the outer band: zero uses the whole band, otherwise floor..outer width. Floor and ceiling do not move the outer band; min_percent and max_percent do. Change a span ceiling only if the user requests a widest-stroke limit. Equal floor and ceiling keep width fixed while a center layer moves the window. Omit controls entirely when changing only steps or layers. loop_cycles and seed are fixed by the lab. When sections exist, change their ranges using the sequence interface; global range edits alone are rejected. A global speed change also updates every existing section's speed unless a new sequence is supplied.
Optional lab controls: variation_mode is "waves" (the original smooth spectral envelope) or "drift" (seeded correlated irregular variation). Neither changes the band or speed; both eventually repeat with the score. turn_softness_percent 0..100 redistributes travel symmetrically, from the original cosine at 0 to longer, gentler turnarounds at 100. cadence_hold_percent 0..100 moves from compensating stroke length to keeping a steadier beat as length changes. A higher cadence hold may reduce effective mean pace; do not change speed to compensate unless asked. These are hypotheses, not proven physical improvements. Omitted controls remain unchanged.
Always preserve the distinction between range, pace, and timing of variation.`

// LLMLabPrompts isolates three control interfaces from production prompting.
func LLMLabPrompts() map[string]string {
	return map[string]string{
		"library":             libraryLabPrompt("library"),
		"library_descriptive": libraryLabPrompt("library_descriptive"),
		"library_actions":     libraryLabPrompt("library_actions"),
		"edits":               labControlGuide + labEditsGuide,
		"controls":            labControlGuide + `\nThis interface accepts only reply and controls. No steps or layers.`,
		"sequence": labControlGuide + `\nThis interface also accepts "steps": an ordered array of 1..4 complete sections. Each section has exactly min_percent, max_percent, speed_percent and cycles (2..12). A new steps array replaces the old sequence; [] clears it. Sections blend over the first 0.8 cycle and then hold their band/pace. The sequence repeats until the user stops it. Order and cycle counts matter. No layers.
Example: {"reply":"Four broad cycles, then two upper cycles.","steps":[{"min_percent":0,"max_percent":100,"speed_percent":25,"cycles":4},{"min_percent":60,"max_percent":100,"speed_percent":25,"cycles":2}]}.`,
		"layers": labControlGuide + `\nThis interface also accepts "layers": 0..3 modulation layers on a single continuous carrier. Each layer has exactly axis (range, center or pace), amount_percent (0..100), period_cycles (2..32) and phase_percent (0..100). Each axis appears at most once. A new layers array replaces the old stack; [] clears it. Layers act together, not sequentially. Range contracts toward the floor, center shifts the anchor within the outer band, pace adds a gradual slowdown. Larger period_cycles means slower modulation. Phase offsets a wave's starting point. No steps and no direct addition of position patterns.
Example: {"reply":"A slow range swell over the carrier.","layers":[{"axis":"range","amount_percent":40,"period_cycles":8,"phase_percent":0}]}.`,
	}
}

// LLMLabTrial retains raw failures as evidence, without repair or fallback.
type LLMLabTrial struct {
	Message       string                `json:"message"`
	Reply         string                `json:"reply"`
	Raw           string                `json:"raw"`
	Error         string                `json:"error,omitempty"`
	Valid         bool                  `json:"valid"`
	Changed       []string              `json:"changed"`
	Model         string                `json:"model"`
	Method        string                `json:"method"`
	Prompt        string                `json:"prompt"`
	ElapsedMillis int64                 `json:"elapsed_ms"`
	ProviderCalls int                   `json:"provider_calls"`
	SchemaGuided  bool                  `json:"schema_guided"`
	RecipeName    string                `json:"recipe_name,omitempty"`
	Before        motion.FlowSpec       `json:"before"`
	After         motion.FlowSpec       `json:"after"`
	Limits        config.MotionSettings `json:"limits"`
}

// RunLLMLab uses one inference request and a strict semantic parser. It has no
// engine or transport access and never writes the production conversation.
func RunLLMLab(ctx context.Context, provider llm.Provider, model, method, prompt, message string, current motion.FlowSpec, limits config.MotionSettings, history []llm.Message, schemaGuided bool) LLMLabTrial {
	trial := LLMLabTrial{Message: message, Model: model, Method: method, Prompt: prompt, Before: current, After: current, Limits: limits, Changed: []string{}}
	if _, ok := LLMLabPrompts()[method]; !ok || len(prompt) > 16000 || strings.TrimSpace(prompt) == "" {
		trial.Error = "select a supported interface and a non-empty prompt of at most 16000 characters"
		return trial
	}
	if _, err := ValidateUserMessage(message); err != nil {
		trial.Error = err.Error()
		return trial
	}
	recipeID, _ := labCurrentRecipe(method, current)
	contextJSON, _ := json.Marshal(map[string]any{"current_score": current, "current_recipe": recipeID, "saved_limits": limits})
	messages := []llm.Message{{Role: "system", Content: prompt}}
	if len(history) > 8 {
		history = history[len(history)-8:]
	}
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: string(contextJSON) + "\nRequest: " + message})
	started := time.Now()
	trial.ProviderCalls = 1
	trial.SchemaGuided = schemaGuided
	request := llm.ChatRequest{Model: model, Temperature: 0.1, MaxTokens: 1536, ReasoningMode: "off", Messages: messages}
	if schemaGuided {
		request.JSONSchema = LLMLabSchema(method, limits)
	}
	raw, err := provider.StreamChat(ctx, request, func(string) error { return nil })
	trial.ElapsedMillis = time.Since(started).Milliseconds()
	trial.Raw = raw
	if err != nil {
		trial.Error = err.Error()
		return trial
	}
	trial.Reply, trial.After, trial.Changed, err = ParseLLMLab(raw, method, current, limits)
	if err != nil {
		trial.After = current
		trial.Changed = []string{}
		trial.Error = err.Error()
		return trial
	}
	trial.Valid = true
	_, trial.RecipeName = labCurrentRecipe(method, trial.After)
	return trial
}

// ParseLLMLab merges only explicit valid changes into the authoritative score.
func ParseLLMLab(raw, method string, current motion.FlowSpec, limits config.MotionSettings) (string, motion.FlowSpec, []string, error) {
	if method == "edits" {
		return parseLabEdits(raw, current, limits)
	}
	if strings.HasPrefix(method, "library") {
		return parseLibraryLab(raw, method, current, limits)
	}
	var proposal struct {
		Reply    string                     `json:"reply"`
		Controls map[string]json.RawMessage `json:"controls"`
		Steps    json.RawMessage            `json:"steps"`
		Layers   json.RawMessage            `json:"layers"`
	}
	if err := decodeLabObject(raw, &proposal); err != nil {
		return "", current, nil, err
	}
	if strings.TrimSpace(proposal.Reply) == "" || len(proposal.Reply) > 2000 {
		return "", current, nil, errors.New("a brief reply is required")
	}
	if (len(proposal.Steps) > 0 && method != "sequence") || (len(proposal.Layers) > 0 && method != "layers") {
		return "", current, nil, errors.New("output used fields outside the selected interface")
	}
	encoded, _ := json.Marshal(current)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if err := mergeLabControls(fields, proposal.Controls); err != nil {
		return "", current, nil, err
	}
	if len(proposal.Steps) > 0 {
		if err := labRequiredItems(proposal.Steps, []string{"min_percent", "max_percent", "speed_percent", "cycles"}); err != nil {
			return "", current, nil, err
		}
		fields["steps"] = proposal.Steps
	}
	if len(proposal.Layers) > 0 {
		if err := labRequiredItems(proposal.Layers, []string{"axis", "amount_percent", "period_cycles", "phase_percent"}); err != nil {
			return "", current, nil, err
		}
		fields["layers"] = proposal.Layers
	}
	merged, _ := json.Marshal(fields)
	var next motion.FlowSpec
	if err := decodeLabObject(string(merged), &next); err != nil {
		return "", current, nil, err
	}
	if len(current.Steps) > 0 && len(proposal.Steps) == 0 {
		if err := applyExistingSectionControls(&next, proposal.Controls); err != nil {
			return "", current, nil, err
		}
	}
	if err := next.Validate(limits); err != nil {
		return "", current, nil, err
	}
	return proposal.Reply, next, labChangedControls(current, next), nil
}

func mergeLabControls(fields, controls map[string]json.RawMessage) error {
	allowed := []string{"min_percent", "max_percent", "speed_percent", "range_floor_percent", "range_ceiling_percent", "anchor_percent", "memory_cycles", "pace_variation_percent", "variation_mode", "turn_softness_percent", "cadence_hold_percent"}
	for key, value := range controls {
		if !slices.Contains(allowed, key) || string(value) == "null" {
			return fmt.Errorf("unsupported control: %s", key)
		}
		fields[key] = value
	}
	return nil
}

func applyExistingSectionControls(next *motion.FlowSpec, controls map[string]json.RawMessage) error {
	_, lower := controls["min_percent"]
	_, upper := controls["max_percent"]
	if lower || upper {
		return errors.New("change section ranges with the sequence interface")
	}
	if _, speed := controls["speed_percent"]; speed {
		for index := range next.Steps {
			next.Steps[index].SpeedPercent = next.SpeedPercent
		}
	}
	return nil
}

func labChangedControls(before, after motion.FlowSpec) []string {
	leftJSON, _ := json.Marshal(before)
	rightJSON, _ := json.Marshal(after)
	var left, right map[string]json.RawMessage
	_ = json.Unmarshal(leftJSON, &left)
	_ = json.Unmarshal(rightJSON, &right)
	changed := []string{}
	for key, value := range right {
		if string(left[key]) != string(value) {
			changed = append(changed, key)
		}
	}
	for key := range left {
		if _, present := right[key]; !present {
			changed = append(changed, key)
		}
	}
	slices.Sort(changed)
	return changed
}

func labRequiredItems(raw json.RawMessage, required []string) error {
	if string(raw) == "null" {
		return errors.New("use an empty array to clear a score section")
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	for _, item := range items {
		for _, key := range required {
			if value, exists := item[key]; !exists || string(value) == "null" {
				return fmt.Errorf("score item requires %s", key)
			}
		}
	}
	return nil
}

func decodeLabObject(raw string, value any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid lab JSON: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("lab output must contain exactly one JSON object")
	}
	return nil
}
