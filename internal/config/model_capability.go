package config

// Capability is chosen per model, independently of transport. The local model
// starts with the simple contract; a connection saved without a level keeps the
// advanced contract it had before levels existed.
const (
	ModelCapabilitySimple       = "simple"
	ModelCapabilityIntermediate = "intermediate"
	// ModelCapabilityAdvancedNoReasoning is the Advanced contract plus a
	// private planning note in the same generation, for strong models run
	// without reasoning (ADR 0065). Every Advanced feature applies to it.
	ModelCapabilityAdvancedNoReasoning = "advanced_no_reasoning"
	ModelCapabilityAdvanced            = "advanced"
)

// ModelCapabilityLevels lists the user-selectable contracts.
func ModelCapabilityLevels() []string {
	return []string{ModelCapabilitySimple, ModelCapabilityIntermediate, ModelCapabilityAdvancedNoReasoning, ModelCapabilityAdvanced}
}

// ModelCapabilityLevel resolves a role's contract without changing its provider.
func (s LLMSettings) ModelCapabilityLevel() string {
	if s.ActiveConnection != nil {
		return s.ActiveConnection.ModelCapabilityLevel()
	}
	if s.IsHosted() {
		return ModelCapabilityAdvanced
	}
	switch s.LocalCapabilityLevel {
	case ModelCapabilityIntermediate, ModelCapabilityAdvancedNoReasoning, ModelCapabilityAdvanced:
		return s.LocalCapabilityLevel
	default:
		return ModelCapabilitySimple
	}
}

// ModelCapabilityLevel resolves a connection's contract. A server such as LM
// Studio may run a small model, so the level is chosen with the connection.
func (c ModelConnection) ModelCapabilityLevel() string {
	switch c.CapabilityLevel {
	case ModelCapabilitySimple, ModelCapabilityIntermediate, ModelCapabilityAdvancedNoReasoning:
		return c.CapabilityLevel
	default:
		return ModelCapabilityAdvanced
	}
}

// AdvancedMotion enables the complete model-facing motion toolset.
func (s LLMSettings) AdvancedMotion() bool {
	level := s.ModelCapabilityLevel()
	return level == ModelCapabilityAdvanced || level == ModelCapabilityAdvancedNoReasoning
}

// ChatPlanNote adds the private planning note to the Advanced chat contract.
func (s LLMSettings) ChatPlanNote() bool {
	return s.ModelCapabilityLevel() == ModelCapabilityAdvancedNoReasoning
}

// IntermediateMotion enables the expanded local contract after explicit opt-in.
func (s LLMSettings) IntermediateMotion() bool {
	return s.ModelCapabilityLevel() == ModelCapabilityIntermediate
}

// ConnectionOutputFloor is a connection's per-request output floor for its
// capability. The saved Max output stays the local model's budget, so a
// Simple local default cannot truncate an Advanced connection's structured
// replies. ChatGPT plan usage sends no output cap at all.
func (s LLMSettings) ConnectionOutputFloor() int {
	if s.ActiveConnection == nil || s.ModelCapabilityLevel() == ModelCapabilitySimple {
		return 0
	}
	return MinimumLocalOutput(s.ModelCapabilityLevel())
}

// MinimumLocalContext reserves space for the selected contract and its schema.
// Connections use the same floors for their request budget.
func MinimumLocalContext(level string) int {
	switch level {
	case ModelCapabilityAdvanced, ModelCapabilityAdvancedNoReasoning:
		return MinimumExternalContext
	case ModelCapabilityIntermediate:
		return 32768
	default:
		return 16384
	}
}

// MinimumLocalOutput leaves room to finish a rich structured reply. It raises
// only explicitly selected local tiers' saved limit; legacy/simple output
// limits are preserved. Connections apply it per request instead.
func MinimumLocalOutput(level string) int {
	switch level {
	case ModelCapabilityAdvanced, ModelCapabilityAdvancedNoReasoning:
		return 1024
	case ModelCapabilityIntermediate:
		return 512
	default:
		return 64
	}
}

// ModelCapabilityOption supplies the settings UI with backend allocation floors.
type ModelCapabilityOption struct {
	Level          string `json:"level"`
	MinimumContext int    `json:"minimum_context"`
	MinimumOutput  int    `json:"minimum_output"`
}

// ModelCapabilityOptions describes valid choices for settings clients. Output
// floors raise the local model's saved limit; connections apply them per
// request (ConnectionOutputFloor).
func ModelCapabilityOptions() []ModelCapabilityOption {
	options := make([]ModelCapabilityOption, 0, 4)
	for _, level := range ModelCapabilityLevels() {
		options = append(options, ModelCapabilityOption{level, MinimumLocalContext(level), MinimumLocalOutput(level)})
	}
	return options
}

// LocalContextSize is also an admission budget for self-hosted endpoints. Only
// managed llama.cpp and Ollama can have their allocation set by this app.
func (s LLMSettings) LocalContextSize() int {
	return max(s.LlamaCPPContextSize, MinimumLocalContext(s.LocalCapabilityLevel))
}

// ModelFeatures is backend-owned UI capability data, not connection readiness
// or permission to raise a user's Max. It never changes provider routing.
type ModelFeatures struct {
	Level                string `json:"level"`
	MinimumContext       int    `json:"minimum_context"`
	AutoTiming           bool   `json:"auto_timing"`
	PrepareDecisions     bool   `json:"prepare_decisions"`
	IntermediatePlanning bool   `json:"intermediate_planning"`
	IndependentVibration bool   `json:"independent_vibration"`
	// VibrationLevel is the Simple/Intermediate vocabulary: one level under
	// Max while vibration follows movement (ADR 0061).
	VibrationLevel bool `json:"vibration_level"`
	ResponseRepair bool `json:"response_repair"`
}

// ModelFeatures projects the effective capabilities of a resolved model role.
func (s LLMSettings) ModelFeatures() ModelFeatures {
	advanced := s.AdvancedMotion()
	continuous := s.MotionGenerationMode == LLMMotionModeLayered || s.MotionGenerationMode == LLMMotionModeCreativeV2
	return ModelFeatures{Level: s.ModelCapabilityLevel(), MinimumContext: MinimumLocalContext(s.ModelCapabilityLevel()),
		AutoTiming: advanced, PrepareDecisions: advanced, ResponseRepair: advanced,
		IntermediatePlanning: s.IntermediateMotion() && continuous,
		IndependentVibration: advanced && continuous,
		// E4B kept strokes and chose levels well in Layered (18/20) but
		// overran its output copying Creative v2 scores (8/20), so Simple
		// Creative v2 follows at Max (ADR 0061).
		VibrationLevel: !advanced && (s.MotionGenerationMode == LLMMotionModeLayered ||
			(s.IntermediateMotion() && s.MotionGenerationMode == LLMMotionModeCreativeV2))}
}
