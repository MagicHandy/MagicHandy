package config

// LLMSettings contains local defaults, named providers, and model role settings.
type LLMSettings struct {
	RequestRole              string                `json:"-"`
	Connections              []ModelConnection     `json:"connections,omitempty"`
	ConversationConnectionID string                `json:"conversation_connection_id,omitempty"`
	ActiveConnection         *ModelConnection      `json:"-"`
	MotionPlanner            MotionPlannerSettings `json:"motion_planner"`
	Provider                 string                `json:"provider"`
	LlamaCPPMode             string                `json:"llama_cpp_mode"`
	ManagedLoadPolicy        string                `json:"managed_load_policy"`
	LlamaCPPBaseURL          string                `json:"llama_cpp_base_url"`
	LlamaCPPContextSize      int                   `json:"llama_cpp_context_size"`
	OllamaBaseURL            string                `json:"ollama_base_url"`
	OllamaModelsPath         string                `json:"ollama_models_path,omitempty"`
	Model                    string                `json:"model"`
	PromptSet                string                `json:"prompt_set"`
	RequestTimeoutMillis     int                   `json:"request_timeout_ms"`
	MaxOutputTokens          int                   `json:"max_output_tokens"`
	ReasoningMode            string                `json:"reasoning_mode"`
	// ChatVoice selects how sexual the model's reply register may be. It only
	// shapes prompt composition; the motion contract and every motion safety
	// gate are identical at every level.
	ChatVoice string `json:"chat_voice"`
	// ReplyLength steers how long chat replies run; balanced leaves the
	// prompt unchanged.
	ReplyLength string `json:"reply_length"`
	// UserAnatomy controls code-owned vocabulary independently of the partner
	// persona. CustomAnatomy and PersonaDescription are quoted as data when
	// composed into a non-utility chat prompt.
	UserAnatomy        string `json:"user_anatomy"`
	CustomAnatomy      string `json:"custom_anatomy"`
	PersonaDescription string `json:"persona_description"`
	// MotionGenerationMode selects the single model-facing motion vocabulary.
	// Dynamic geometry and pattern IDs are never advertised together.
	MotionGenerationMode string `json:"motion_generation_mode"`
	// MotionCapabilities gates which motion control methods the model may
	// use. A nil pointer means "never saved" and resolves to the defaults, so
	// older payloads keep today's behavior; an explicit all-false is a valid
	// saved choice (chat-only model).
	MotionCapabilities *LLMMotionCapabilities `json:"motion_capabilities,omitempty"`
}

// LLMMotionCapabilities is the user-selected checkbox list of control methods
// the model may use. Enforcement is server-side: disabled methods are neither
// advertised in the prompt nor honored if the model emits them. Stop and all
// user controls are unaffected — these gates only ever restrict the model.
type LLMMotionCapabilities struct {
	// Motion is the master gate: off makes the model chat-only.
	Motion bool `json:"motion"`
	// Patterns lets the model curate enabled library patterns.
	Patterns bool `json:"patterns"`
	// AreaFocus lets the model focus motion on a named zone (tip/shaft/base).
	AreaFocus bool `json:"area_focus"`
	// ExperimentalPatterns includes experimental-tagged patterns in the
	// model's catalog. They stay visible and playable in the library UI
	// regardless — this only gates model access.
	ExperimentalPatterns bool `json:"experimental_patterns"`
}

// DefaultLLMMotionCapabilities matches the pre-gate behavior plus area focus;
// experimental patterns are opt-in.
func DefaultLLMMotionCapabilities() LLMMotionCapabilities {
	return LLMMotionCapabilities{Motion: true, Patterns: true, AreaFocus: true, ExperimentalPatterns: false}
}

// Capabilities resolves the saved motion-capability gates, applying defaults
// for payloads that predate the field.
func (s LLMSettings) Capabilities() LLMMotionCapabilities {
	if s.MotionCapabilities == nil {
		return DefaultLLMMotionCapabilities()
	}
	return *s.MotionCapabilities
}
