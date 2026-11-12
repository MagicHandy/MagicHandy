package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// TempoContext is what a model may know about a stroker that makes its own
// strokes, such as the VacuGlide (ADR 0071): its tempo and whether it may
// change the air. It never names a motor speed or the person's Tempo range.
type TempoContext struct {
	// Leads is true when no position stroker plays, so tempo is the motion.
	Leads    bool              `json:"tempo_leads"`
	Ready    bool              `json:"connected"`
	Running  bool              `json:"playing"`
	Current  *motion.TempoSpec `json:"current_tempo,omitempty"`
	Previous *motion.TempoSpec `json:"previous_tempo,omitempty"`
	Level    float64           `json:"level_percent_now"`
	// AirAllowed is the person's permission for motion to change the air.
	AirAllowed bool `json:"air_changes_allowed"`
	AirSteps   int  `json:"air_steps_this_session"`
}

// tempoTier is the vocabulary a capability level gets (ADR 0071).
type tempoTier int

const (
	tempoSimple tempoTier = iota
	tempoIntermediate
	tempoAdvanced
)

const tempoMaxAirSteps = 2

func tempoTierOf(caps Capabilities) tempoTier {
	switch {
	case caps.AdvancedMotion:
		return tempoAdvanced
	case caps.IntermediateMotion:
		return tempoIntermediate
	default:
		return tempoSimple
	}
}

func (t tempoTier) modes() []string {
	modes := []string{motion.TempoModeSteady, motion.TempoModePause, motion.TempoModeRoutine}
	if t >= tempoIntermediate {
		modes = append(modes, motion.TempoModeRamp, motion.TempoModeWave, motion.TempoModePulse)
	}
	if t == tempoAdvanced {
		modes = append(modes, motion.TempoModeSequence)
	}
	return modes
}

// tempoLeadsContext reports a turn whose motion is tempo: the VacuGlide plays
// and no position stroker does.
func tempoLeadsContext(state *MotionContext) bool {
	return state != nil && state.Outputs != nil && state.Outputs.Tempo != nil && state.Outputs.Tempo.Leads
}

func tempoAirAvailable(tier tempoTier, state *MotionContext) bool {
	return tier >= tempoIntermediate && tempoLeadsContext(state) && state.Outputs.Tempo.Ready &&
		state.Outputs.Tempo.Running && !state.Paused && state.Outputs.Tempo.AirAllowed
}

// Resume is a semantic reference to a saved plan, including its routine seed.
// The model chooses whether the request is a continuation; no text matching is
// involved. A stale response is still subject to the ordinary Stop admission.
func resumableTempo(state *MotionContext) *motion.TempoSpec {
	if !tempoLeadsContext(state) || state.Paused || !state.Outputs.Tempo.Ready || state.Outputs.Tempo.Running {
		return nil
	}
	previous := state.Outputs.Tempo.Previous
	if previous == nil || previous.Mode == motion.TempoModeFollow || previous.Mode == motion.TempoModePause || previous.Validate() != nil {
		return nil
	}
	return previous
}

func tempoRoutineGuide() string {
	var lines []string
	for _, routine := range motion.TempoRoutines() {
		lines = append(lines, fmt.Sprintf("- %s: %s", routine.ID, routine.Description))
	}
	return strings.Join(lines, "\n")
}

func tempoContract(tier tempoTier, state *MotionContext) string {
	return `RESPONSE CONTRACT: Reply with one JSON object: {"reply": string, "tempo": object or null, "air_steps": integer or null, "stop": boolean}.
` + tempoDeviceGuide(tier, state) + `
stop true ends the session and stops the VacuGlide; use it only when the user wants to stop or finish. Otherwise stop is false. With stop true, tempo and air_steps are null.`
}

// tempoDeviceGuide is the tempo vocabulary shared by chat and Autopilot.
func tempoDeviceGuide(tier tempoTier, state *MotionContext) string {
	var b strings.Builder
	b.WriteString(`THE DEVICE: The stroker is a VacuGlide. It makes its own strokes; you choose its TEMPO, how fast it strokes, as level_percent from 1 (its slowest) to 100 (its fastest in the range the user allows). You cannot choose depth, position, tip, base or stroke length: the air sealed in its sleeve sets how long and tight each stroke is. Describe speed and rhythm, never a depth or a spot you chose.
tempo null keeps the current tempo. To start or change it, emit exactly one of:
- {"mode":"steady","level_percent":L} holds level L (1..100).
- {"mode":"pause"} stops the motor until you change it; the session stays on.
- {"mode":"routine","routine":"ID"} plays a built-in routine:
`)
	b.WriteString(tempoRoutineGuide())
	b.WriteString("\nWhen the person names a built-in routine, select mode routine and its ID; a similar wave is a different plan.")
	b.WriteString("\nSession buildup is the optional Autopilot finish target. The person sets its duration and sees a progress bar; Autopilot uses it to plan a gradual build toward the strongest stretch at the end, within existing limits. It is separate from the Slow build routine and does not automatically stop playback. Only explicit Session buildup facts mean that setting is enabled; knowing about the feature is not a request to build up.")
	if resumableTempo(state) != nil {
		b.WriteString("\n- {\"mode\":\"resume\"} starts the saved previous_tempo unchanged, including its exact routine variation. Use for an unchanged continuation after Stop; choose a new tempo for a different request.")
	}
	if tier >= tempoIntermediate {
		b.WriteString(`
- {"mode":"ramp","level_percent":L,"period_seconds":S} glides from the current level to L over S seconds (1..600), then holds L. L may be 0 for a gradual motor stop.
- {"mode":"wave","min_percent":A,"max_percent":B,"period_seconds":S} rises and falls smoothly between A and B, one cycle every S seconds (2..120), with 0<=A<B<=100.
- {"mode":"pulse","min_percent":A,"max_percent":B,"period_seconds":S} sits at A and lifts to B in rounded bursts, one every S seconds (2..120).`)
	}
	if tier == tempoAdvanced {
		b.WriteString(`
- {"mode":"sequence","sections":[...],"loop":boolean} plays up to 12 sections in order. Each section is one of steady, ramp, wave, pulse or pause with the same fields plus "seconds" (1..1800), how long it plays; the last section may omit seconds to hold it, unless loop is true. Use a sequence for a build, an edge or a planned arc.`)
	}
	if tier == tempoIntermediate {
		b.WriteString("\nUse whole seconds for periods and ramp durations.")
	}
	b.WriteString(`
Write only the fields of the shape you choose. A new tempo begins from the current level and blends in over about a second; a pause stops at once. Choose levels for the request: a gentle start is low (10 to 30), faster means higher, 100 is the fastest allowed. Small changes (5 to 15 levels) suit "a little faster"; never jump to the extremes unless asked. Changes reach the device through the internet within about a second.`)
	if tempoAirAvailable(tier, state) {
		b.WriteString(`
air_steps: null keeps the air. 1 or 2 draws air out of the sleeve for a tighter, longer stroke; -1 or -2 lets air in for a looser, shorter one. Use it only when asked for tighter, looser, more or less suction; at most 3 steps either way in one session (see air_steps_this_session). Nothing measures the air, so steps are relative to how the user set the device.`)
	} else {
		b.WriteString(`
air_steps must be null: you cannot change the air. If asked for tighter or looser, say the user can change it on the device or with its stroke buttons.`)
	}
	if tier == tempoSimple {
		b.WriteString(`
Examples of unchanged or unavailable actions:
"Keep this tempo": {"reply":"Keeping this rhythm.","tempo":null,"air_steps":null,"stop":false}.
"Tighten the sleeve": {"reply":"Use the device's stroke buttons to tighten the sleeve; I can choose its tempo.","tempo":null,"air_steps":null,"stop":false}.
"Stay near the tip": {"reply":"I can change tempo, but the VacuGlide cannot choose a position or body region.","tempo":null,"air_steps":null,"stop":false}.`)
	}
	return b.String()
}

func tempoStateSection(state *MotionContext) string {
	tempo := TempoContext{}
	if tempoLeadsContext(state) {
		tempo = *state.Outputs.Tempo
	}
	encoded, _ := json.Marshal(tempo)
	timeline := ""
	if state != nil && len(state.UserRequests) > 0 {
		requests, _ := json.Marshal(timedUserRequests(*state))
		timeline = "\nRecent human directions (oldest first): " + string(requests) + ". Recorded seconds_ago is the age of that request, not the session's age. For a timed direction, use the time remaining from when it was said; later conversation does not restart that interval."
	}
	companions := ""
	if state != nil && state.Outputs != nil && (state.Outputs.Vibrator != nil || state.Outputs.Estim != nil) {
		companions = " This native tempo session controls the VacuGlide only. A selected vibrator or Coyote remains idle; do not claim it follows the tempo or that you can change it here."
	}
	return "TEMPO CONTEXT: " + string(encoded) + "\nThese backend facts describe now; older chat may describe earlier runs. The reply must not claim a tempo or air change that this response does not make. " +
		"When connected and stopped, a new request to start or continue may start a tempo; previous_tempo is the saved plan for an unchanged continuation, not current playback. A question leaves playback as it is. " +
		"When disconnected, tempo and air_steps must be null. When playback is paused by the user, only the user can resume it." + companions + timeline
}

const tempoOutputGuard = `FINAL OUTPUT GUARD: Output only the JSON object with reply, tempo, air_steps and stop.`

// tempoSystemPrompt keeps the persona's own sections and replaces the
// position contract with the tempo contract (ADR 0071).
func (s Service) tempoSystemPrompt(caps Capabilities, tier tempoTier) string {
	return tempoSystem(s.Prompt, s.Memories, s.ConversationContext, s.PromptBudget, s.MotionContext, caps,
		tempoContract(tier, s.MotionContext), tempoOutputGuard)
}

func tempoSystem(prompt PromptSet, memories []string, conversation *ConversationContext, budget PromptBudgetSettings,
	state *MotionContext, caps Capabilities, contract, guard string) string {
	if strings.TrimSpace(prompt.ID) == "" {
		prompt, _ = BuiltinPromptSetByID(DefaultPromptSetID)
	}
	chatOnly := caps
	chatOnly.Motion, chatOnly.Patterns, chatOnly.AreaFocus = false, false, false
	composition := composePrompt(prompt, memories, nil, chatOnly, nil, conversation, budget)
	texts := make([]string, 0, len(composition.Sections)+2)
	for _, section := range composition.Sections {
		switch section.ID {
		case "response_contract":
			texts = append(texts, contract)
		case "playback_outputs", "video_motion", "motion_context", "depth_frame", "pattern_catalog":
		case "output_guard":
			texts = append(texts, tempoStateSection(state), guard)
		default:
			texts = append(texts, section.Text)
		}
	}
	return strings.Join(texts, "\n\n")
}

func tempoResponseSchema(tier tempoTier, state *MotionContext) json.RawMessage {
	nullable := func(value map[string]any) map[string]any {
		return map[string]any{"anyOf": []any{map[string]any{"type": "null"}, value}}
	}
	section := func(modes []string, withSeconds bool) map[string]any {
		periodType := "number"
		if tier != tempoAdvanced {
			periodType = "integer"
		}
		properties := map[string]any{
			"mode":           map[string]any{"type": "string", "enum": modes},
			"level_percent":  nullable(map[string]any{"type": "integer", "minimum": 0, "maximum": 100}),
			"min_percent":    nullable(map[string]any{"type": "integer", "minimum": 0, "maximum": 100}),
			"max_percent":    nullable(map[string]any{"type": "integer", "minimum": 0, "maximum": 100}),
			"period_seconds": nullable(map[string]any{"type": periodType, "minimum": 0, "maximum": 600}),
		}
		required := []string{"mode", "level_percent", "min_percent", "max_percent", "period_seconds"}
		if withSeconds {
			properties["seconds"] = nullable(map[string]any{"type": "number", "minimum": 0, "maximum": 1800})
			required = append(required, "seconds")
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	modes := tier.modes()
	if resumableTempo(state) != nil {
		modes = append(modes, "resume")
	}
	tempo := section(modes, false)
	properties := tempo["properties"].(map[string]any)
	var routines []string
	for _, routine := range motion.TempoRoutines() {
		routines = append(routines, routine.ID)
	}
	properties["routine"] = nullable(map[string]any{"type": "string", "enum": routines})
	required := append(tempo["required"].([]string), "routine")
	if tier == tempoAdvanced {
		sectionModes := []string{motion.TempoModeSteady, motion.TempoModeRamp, motion.TempoModeWave, motion.TempoModePulse, motion.TempoModePause}
		properties["sections"] = nullable(map[string]any{"type": "array", "maxItems": 12, "items": section(sectionModes, true)})
		properties["loop"] = nullable(map[string]any{"type": "boolean"})
		required = append(required, "sections", "loop")
	}
	tempo["required"] = required
	air := map[string]any{"type": "null"}
	if tempoAirAvailable(tier, state) {
		air = nullable(map[string]any{"type": "integer", "minimum": -tempoMaxAirSteps, "maximum": tempoMaxAirSteps})
	}
	tempoValue := nullable(tempo)
	if tier != tempoAdvanced {
		tempoValue = localTempoSchema(tier, state)
	}
	if state != nil && (state.Paused || (tempoLeadsContext(state) && !state.Outputs.Tempo.Ready)) {
		tempoValue = map[string]any{"type": "null"}
	}
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reply":     map[string]any{"type": "string"},
			"tempo":     tempoValue,
			"air_steps": air,
			"stop":      map[string]any{"type": "boolean"},
		},
		"required":             []string{"reply", "tempo", "air_steps", "stop"},
		"additionalProperties": false,
	}
	encoded, _ := json.Marshal(root)
	return orderedTempoSchema(encoded, tier)
}

// tempoWire is a tempo object as a strict-schema provider writes it: every
// field present, null where unused.
type tempoWire struct {
	Mode          string             `json:"mode"`
	LevelPercent  *int               `json:"level_percent"`
	MinPercent    *int               `json:"min_percent"`
	MaxPercent    *int               `json:"max_percent"`
	PeriodSeconds *float64           `json:"period_seconds"`
	Seconds       *float64           `json:"seconds"`
	Routine       *string            `json:"routine"`
	Sections      []tempoSectionWire `json:"sections"`
	Loop          *bool              `json:"loop"`
}

type tempoSectionWire struct {
	Mode          string   `json:"mode"`
	LevelPercent  *int     `json:"level_percent"`
	MinPercent    *int     `json:"min_percent"`
	MaxPercent    *int     `json:"max_percent"`
	PeriodSeconds *float64 `json:"period_seconds"`
	Seconds       *float64 `json:"seconds"`
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func (w tempoSectionWire) section() motion.TempoSection {
	return motion.TempoSection{Mode: w.Mode, LevelPercent: intValue(w.LevelPercent), MinPercent: intValue(w.MinPercent),
		MaxPercent: intValue(w.MaxPercent), PeriodSeconds: floatValue(w.PeriodSeconds), Seconds: floatValue(w.Seconds)}
}

// spec converts the wire object and drops fields the chosen shape does not
// use, so a strict provider's placeholder values never change the meaning.
func (w tempoWire) spec(tier tempoTier) (motion.TempoSpec, error) {
	if !slices.Contains(tier.modes(), w.Mode) {
		return motion.TempoSpec{}, fmt.Errorf("tempo mode %q is not offered here", w.Mode)
	}
	section := tempoSectionWire{Mode: w.Mode, LevelPercent: w.LevelPercent, MinPercent: w.MinPercent, MaxPercent: w.MaxPercent, PeriodSeconds: w.PeriodSeconds}.section()
	spec := motion.TempoSpec{Mode: w.Mode}
	switch w.Mode {
	case motion.TempoModeSteady:
		spec.LevelPercent = section.LevelPercent
	case motion.TempoModeRamp:
		spec.LevelPercent, spec.PeriodSeconds = section.LevelPercent, section.PeriodSeconds
	case motion.TempoModeWave, motion.TempoModePulse:
		spec.MinPercent, spec.MaxPercent, spec.PeriodSeconds = section.MinPercent, section.MaxPercent, section.PeriodSeconds
	case motion.TempoModeRoutine:
		if w.Routine == nil {
			return spec, errors.New("a tempo routine needs its routine id")
		}
		spec.Routine = *w.Routine
	case motion.TempoModeSequence:
		for _, item := range w.Sections {
			next := item.section()
			trimmed := motion.TempoSection{Mode: next.Mode, Seconds: next.Seconds}
			switch next.Mode {
			case motion.TempoModeSteady:
				trimmed.LevelPercent = next.LevelPercent
			case motion.TempoModeRamp:
				trimmed.LevelPercent, trimmed.PeriodSeconds = next.LevelPercent, next.PeriodSeconds
			case motion.TempoModeWave, motion.TempoModePulse:
				trimmed.MinPercent, trimmed.MaxPercent, trimmed.PeriodSeconds = next.MinPercent, next.MaxPercent, next.PeriodSeconds
			}
			spec.Sections = append(spec.Sections, trimmed)
		}
		spec.Loop = w.Loop != nil && *w.Loop
	}
	return spec, spec.Validate()
}

// parseTempoResponse validates one tempo-contract reply into the ordinary
// semantic command: a tempo, air steps, or Stop.
func parseTempoResponse(raw string, tier tempoTier, state *MotionContext) (AssistantResponse, error) {
	root, err := uniqueLayeredObject(json.RawMessage(strings.TrimSpace(raw)))
	if err != nil {
		return AssistantResponse{}, err
	}
	var reply string
	if err := json.Unmarshal(root["reply"], &reply); err != nil || strings.TrimSpace(reply) == "" {
		return AssistantResponse{}, errors.New("reply must be a non-empty string")
	}
	response := AssistantResponse{Reply: strings.TrimSpace(reply)}
	stop, err := parseTempoStop(root["stop"])
	if err != nil {
		return response, err
	}
	command := &MotionCommand{Action: MotionActionNone}
	if command.Tempo, err = parseTempoField(root["tempo"], tier, state); err != nil {
		return response, err
	}
	if command.AirSteps, err = parseTempoAir(root["air_steps"], tier, state); err != nil {
		return response, err
	}
	if stop {
		if command.Tempo != nil || command.AirSteps != nil {
			return response, errors.New("stop true takes tempo null and air_steps null")
		}
		command.Action = MotionActionStop
	}
	if command.Action != MotionActionNone || command.Tempo != nil || command.AirSteps != nil {
		response.Motion = command
	}
	return response, nil
}

func parseTempoStop(value json.RawMessage) (bool, error) {
	var stop bool
	if len(value) == 0 || string(value) == "null" {
		return false, nil
	}
	if err := json.Unmarshal(value, &stop); err != nil {
		return false, errors.New("stop must be true or false")
	}
	return stop, nil
}

func parseTempoField(value json.RawMessage, tier tempoTier, state *MotionContext) (*motion.TempoSpec, error) {
	if len(value) == 0 || string(value) == "null" {
		return nil, nil
	}
	if state != nil && state.Paused {
		return nil, errors.New("playback is paused; only the user can resume it, so tempo must be null")
	}
	if tempoLeadsContext(state) && !state.Outputs.Tempo.Ready {
		return nil, errors.New("the VacuGlide is not available; tempo must be null")
	}
	var wire tempoWire
	if err := json.Unmarshal(value, &wire); err != nil {
		return nil, fmt.Errorf("tempo: %w", err)
	}
	if wire.Mode == "resume" {
		if previous := resumableTempo(state); previous != nil {
			return motion.CloneTempoSpec(previous), nil
		}
		return nil, errors.New("there is no saved tempo to resume; choose a new tempo")
	}
	spec, err := wire.spec(tier)
	if err != nil {
		return nil, fmt.Errorf("tempo: %w", err)
	}
	return &spec, nil
}

func parseTempoAir(value json.RawMessage, tier tempoTier, state *MotionContext) (*int, error) {
	if len(value) == 0 || string(value) == "null" {
		return nil, nil
	}
	var steps int
	if err := json.Unmarshal(value, &steps); err != nil || steps < -tempoMaxAirSteps || steps > tempoMaxAirSteps {
		return nil, fmt.Errorf("air_steps must be an integer from -%d to %d, or null", tempoMaxAirSteps, tempoMaxAirSteps)
	}
	if steps == 0 {
		return nil, nil
	}
	if !tempoAirAvailable(tier, state) {
		return nil, errors.New("air_steps must be null: changing the air is not allowed")
	}
	return &steps, nil
}

// completeTempo is the chat turn when tempo is the motion (ADR 0071).
func (s Service) completeTempo(ctx context.Context, request Request, emit func(StreamEvent) error) (Result, error) {
	caps := s.capabilities()
	tier := tempoTierOf(caps)
	messages := capabilityMessages(s.tempoSystemPrompt(caps, tier), request.History, request.Message, caps)
	schema := tempoResponseSchema(tier, s.MotionContext)
	input := llm.ChatRequest{
		Messages:               messages,
		HistoryMessagesDropped: request.HistoryMessagesDropped + hostedHistoryDropped(request.History, messages, caps),
		Model:                  s.Model,
		Temperature:            chatTemperature,
		TopP:                   chatTopP,
		RepeatPenalty:          chatRepeatPenalty,
		RepeatLastN:            chatRepeatLastN,
		MaxTokens:              s.MaxTokens,
		ReasoningMode:          s.ReasoningMode,
		ReasoningBudgetTokens:  s.ReasoningBudgetTokens,
		JSONSchema:             schema,
	}
	raw, err := s.Provider.StreamChat(ctx, input, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "delta", Phase: "initial", Text: text})
	})
	truncated := errors.Is(err, llm.ErrOutputTruncated)
	if err != nil && !truncated {
		return Result{Raw: raw, Input: &input}, err
	}
	response, parseErr := parseTempoResponse(raw, tier, s.MotionContext)
	if parseErr == nil {
		if err := ctx.Err(); err != nil {
			return Result{Raw: raw, Input: &input}, err
		}
		return Result{Response: response, Raw: raw, Input: &input}, nil
	}
	result := Result{Raw: raw, Input: &input, InitialMalformed: true, Malformed: true, MalformedError: parseErr.Error()}
	if s.SingleAttempt {
		return result, parseErr
	}
	if caps.PreserveConversationText {
		if truncated {
			return result, err
		}
		return s.rejectHostedResponse(ctx, input, raw, parseErr, func(value string) (AssistantResponse, error) {
			return parseTempoResponse(value, tier, s.MotionContext)
		}, emit)
	}
	result.RepairAttempted = true
	if err := emitEvent(emit, StreamEvent{Type: "malformed", Phase: "initial", Error: parseErr.Error()}); err != nil {
		return result, err
	}
	repairMessages := append(append([]llm.Message(nil), messages...),
		llm.Message{Role: "assistant", Content: truncateUTF8Bytes(strings.TrimSpace(raw), 4096)},
		llm.Message{Role: "user", Content: "That response was not valid: " + parseErr.Error() + ". Reply again with only the corrected JSON object for the same request."})
	repairRaw, repairErr := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages: repairMessages, PreserveTailMessages: 3, Model: s.Model, Temperature: 0, MaxTokens: s.MaxTokens,
		ReasoningMode: "off", JSONSchema: schema,
	}, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "repair_delta", Phase: "repair", Text: text})
	})
	result.RepairRaw = repairRaw
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if repairErr != nil && !errors.Is(repairErr, llm.ErrOutputTruncated) {
		return result, repairErr
	}
	if repairErr == nil || errors.Is(repairErr, llm.ErrOutputTruncated) {
		if repaired, err := parseTempoResponse(repairRaw, tier, s.MotionContext); err == nil {
			result.Response, result.Malformed, result.Repaired = repaired, false, true
			return result, nil
		}
	}
	// Invalid tempo objects can contain claims about unapplied changes. Keep
	// the rejected response in diagnostics rather than presenting it as success.
	return result, fmt.Errorf("the tempo reply was not valid: %w", parseErr)
}
