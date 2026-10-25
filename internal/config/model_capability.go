package config

// Capability is chosen independently of transport. External connections retain
// their existing advanced contract; local models start with the simple one.
const (
	ModelCapabilitySimple       = "simple"
	ModelCapabilityIntermediate = "intermediate"
	ModelCapabilityAdvanced     = "advanced"
)

// ModelCapabilityLevels lists the user-selectable local contracts.
func ModelCapabilityLevels() []string {
	return []string{ModelCapabilitySimple, ModelCapabilityIntermediate, ModelCapabilityAdvanced}
}

// ModelCapabilityLevel resolves a role's contract without changing its provider.
func (s LLMSettings) ModelCapabilityLevel() string {
	if s.IsHosted() {
		return ModelCapabilityAdvanced
	}
	switch s.LocalCapabilityLevel {
	case ModelCapabilityIntermediate, ModelCapabilityAdvanced:
		return s.LocalCapabilityLevel
	default:
		return ModelCapabilitySimple
	}
}

// AdvancedMotion enables the complete model-facing motion toolset.
func (s LLMSettings) AdvancedMotion() bool {
	return s.ModelCapabilityLevel() == ModelCapabilityAdvanced
}

// IntermediateMotion enables the expanded local contract after explicit opt-in.
func (s LLMSettings) IntermediateMotion() bool {
	return s.ModelCapabilityLevel() == ModelCapabilityIntermediate
}

// MinimumLocalContext reserves space for the selected contract and its schema.
func MinimumLocalContext(level string) int {
	switch level {
	case ModelCapabilityAdvanced:
		return MinimumExternalContext
	case ModelCapabilityIntermediate:
		return 32768
	default:
		return 16384
	}
}

// MinimumLocalOutput leaves room to finish a rich structured reply. It changes only
// explicitly selected local tiers; legacy/simple output limits are preserved.
func MinimumLocalOutput(level string) int {
	switch level {
	case ModelCapabilityAdvanced:
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

// ModelCapabilityOptions describes valid local choices for settings clients.
func ModelCapabilityOptions() []ModelCapabilityOption {
	options := make([]ModelCapabilityOption, 0, 3)
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
// or permission to raise output intensity. It never changes provider routing.
type ModelFeatures struct {
	Level                string `json:"level"`
	MinimumContext       int    `json:"minimum_context"`
	AutoTiming           bool   `json:"auto_timing"`
	PrepareDecisions     bool   `json:"prepare_decisions"`
	IndependentVibration bool   `json:"independent_vibration"`
	ResponseRepair       bool   `json:"response_repair"`
}

// ModelFeatures projects the effective capabilities of a resolved model role.
func (s LLMSettings) ModelFeatures() ModelFeatures {
	advanced := s.AdvancedMotion()
	return ModelFeatures{Level: s.ModelCapabilityLevel(), MinimumContext: MinimumLocalContext(s.ModelCapabilityLevel()),
		AutoTiming: advanced, PrepareDecisions: advanced, ResponseRepair: advanced,
		IndependentVibration: advanced && (s.MotionGenerationMode == LLMMotionModeLayered || s.MotionGenerationMode == LLMMotionModeCreativeV2)}
}
