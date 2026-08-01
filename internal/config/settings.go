package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	// CurrentSettingsVersion is the latest on-disk settings schema version.
	CurrentSettingsVersion = 2

	// DefaultServerPort is the local HTTP port used by fresh settings.
	DefaultServerPort = 49717
)

const (
	// DispatchOwnerCloudREST selects backend-owned Handy Cloud REST dispatch.
	DispatchOwnerCloudREST = "cloud_rest"
	// DispatchOwnerBrowserBluetooth selects browser-owned Bluetooth dispatch.
	DispatchOwnerBrowserBluetooth = "browser_bluetooth"
	// DispatchOwnerIntiface selects backend-owned Intiface dispatch.
	DispatchOwnerIntiface = "intiface"

	// DefaultIntifaceServerAddress is the default local Intiface WebSocket endpoint.
	DefaultIntifaceServerAddress = "ws://127.0.0.1:12345"

	// FirmwareAPIRequirementRequired records the firmware v4/API v3 requirement.
	FirmwareAPIRequirementRequired = "firmware_v4_api_v3_required"

	// ApplicationIDSourceBundled uses MagicHandy's bundled API v3 application ID.
	ApplicationIDSourceBundled = "bundled_app_id"
	// ApplicationIDSourceDeveloperOverride uses a developer-supplied application ID.
	ApplicationIDSourceDeveloperOverride = "developer_override"

	// BundledAPIApplicationID is MagicHandy's public API v3 application identifier.
	BundledAPIApplicationID = "rQoTWeMPrklUYcfdSXYYhS_9z.jAVNwy"

	// DiagnosticsVerbosityNormal records ordinary diagnostics output.
	DiagnosticsVerbosityNormal = "normal"
	// DiagnosticsVerbosityDebug records verbose diagnostics output.
	DiagnosticsVerbosityDebug = "debug"
	// DiagnosticsVerbosityTrace records the most verbose diagnostics output.
	DiagnosticsVerbosityTrace = "trace"

	// ChatStartupPrevious restores the most recently active retained chat.
	ChatStartupPrevious = "previous"
	// ChatStartupNew starts each process with a new unsaved chat.
	ChatStartupNew = "new"
)

const (
	// LLMProviderLlamaCPP is the primary managed Windows/NVIDIA local LLM path.
	LLMProviderLlamaCPP = "llama_cpp"
	// LLMProviderOllama is the secondary externally managed local LLM path.
	LLMProviderOllama = "ollama"

	// LlamaCPPModeManaged starts and owns a configured llama-server process.
	LlamaCPPModeManaged = "managed"
	// LlamaCPPModeExternal connects to a user-managed llama-server process.
	LlamaCPPModeExternal = "external"

	// PromptSetMagicHandyMotionV1 is the default chat and motion JSON contract.
	PromptSetMagicHandyMotionV1 = "magichandy_motion_v1"
	// PromptSetMagicHandyMotionV1ES is the built-in Spanish prompt set.
	PromptSetMagicHandyMotionV1ES = "magichandy_motion_v1_es"
	// PromptSetMagicHandyMotionV1PTBR is the built-in Brazilian Portuguese prompt set.
	PromptSetMagicHandyMotionV1PTBR = "magichandy_motion_v1_pt_br"
	// PromptSetMagicHandyMotionV1ZHHans is the built-in Simplified Chinese prompt set.
	PromptSetMagicHandyMotionV1ZHHans = "magichandy_motion_v1_zh_hans"
	// PromptSetMagicHandyMotionV1JA is the built-in Japanese prompt set.
	PromptSetMagicHandyMotionV1JA = "magichandy_motion_v1_ja"

	// LLMChatVoiceUtility keeps the original neutral motion-assistant register.
	LLMChatVoiceUtility = "utility"
	// LLMChatVoiceWarm is a flirtatious companion voice, suggestive at most.
	LLMChatVoiceWarm = "warm"
	// LLMChatVoiceIntimate is a first-person partner voice with sensual language.
	LLMChatVoiceIntimate = "intimate"
	// LLMChatVoiceExplicit permits direct erotic language (STGPT-RV parity).
	LLMChatVoiceExplicit = "explicit"

	// LLMReactionStyleNeutral composes no style block at all, so a persona left
	// on the default produces a byte-identical prompt to having no persona.
	LLMReactionStyleNeutral = "neutral"
	// LLMReactionStylePlayful teases lightly and initiates.
	LLMReactionStylePlayful = "playful"
	// LLMReactionStyleTender is attentive, reassuring, unhurried.
	LLMReactionStyleTender = "tender"
	// LLMReactionStyleDominant leads and says what happens next. It shapes reply
	// wording only: no style may claim authority over the device.
	LLMReactionStyleDominant = "dominant"
	// LLMReactionStyleSubmissive follows, asks, and defers.
	LLMReactionStyleSubmissive = "submissive"
	// LLMReactionStyleTeasing withholds and draws things out.
	LLMReactionStyleTeasing = "teasing"

	// LLMUserAnatomyPenis selects penis-specific prompt vocabulary.
	LLMUserAnatomyPenis = "penis"
	// LLMUserAnatomyVagina selects vagina/vulva-specific prompt vocabulary.
	LLMUserAnatomyVagina = "vagina"
	// LLMUserAnatomyCustom uses the separately saved custom wording.
	LLMUserAnatomyCustom = "custom"
	// MaxLLMCustomAnatomyChars matches the reviewed STGPT-RV prompt setting.
	MaxLLMCustomAnatomyChars = 120
	// MaxLLMPersonaDescriptionChars bounds user-authored prompt context.
	MaxLLMPersonaDescriptionChars = 500

	// DefaultLlamaCPPBaseURL is the default llama-server OpenAI-compatible URL.
	DefaultLlamaCPPBaseURL = "http://127.0.0.1:8080"
	// DefaultOllamaBaseURL is the default local Ollama daemon URL.
	DefaultOllamaBaseURL = "http://127.0.0.1:11434"
	// DefaultLLMModel is a placeholder model name users replace with a local model.
	DefaultLLMModel = "local-model"
	// DefaultLLMRequestTimeoutMillis caps one chat or repair pass.
	DefaultLLMRequestTimeoutMillis = 120000
	// DefaultLLMMaxOutputTokens bounds compact intent JSON generation.
	DefaultLLMMaxOutputTokens = 256
	// DefaultLlamaCPPContextSize balances the measured prompt size against
	// managed-runner RAM and VRAM allocation.
	DefaultLlamaCPPContextSize = 32768

	// LLMReasoningAuto leaves thinking behavior to the provider and model.
	LLMReasoningAuto = "auto"
	// LLMReasoningOff disables thinking when the provider/template supports it.
	LLMReasoningOff = "off"
)

const (
	// VoiceProviderNone disables one voice role.
	VoiceProviderNone = "none"
	// VoiceProviderCustom runs the stored raw worker command unchanged.
	VoiceProviderCustom = "custom"
	// VoiceTTSProviderElevenLabs selects the bundled ElevenLabs worker.
	VoiceTTSProviderElevenLabs = "elevenlabs"
	// VoiceTTSProviderFasterQwen selects the scripted Faster Qwen3-TTS module.
	VoiceTTSProviderFasterQwen = "faster_qwen3_tts"
	// VoiceTTSProviderChatterbox selects the scripted Chatterbox TTS module.
	VoiceTTSProviderChatterbox = "chatterbox_tts"
	// VoiceTTSProviderOpenAICompat selects an external compatible TTS server.
	VoiceTTSProviderOpenAICompat = "openai_compatible"
	// VoiceASRProviderParakeet selects managed local Parakeet.
	VoiceASRProviderParakeet = "parakeet_managed"
	// VoiceASRProviderOpenAICompat selects an external compatible ASR server.
	VoiceASRProviderOpenAICompat = "openai_compatible"
	// DefaultParakeetServerPort is the managed local ASR port.
	DefaultParakeetServerPort = 8990
	// ParakeetSourceApp uses the runner and model installed by MagicHandy.
	ParakeetSourceApp = "app_managed"
	// ParakeetSourceCustom uses user-supplied local runner and model paths.
	ParakeetSourceCustom = "custom_local"
	// DefaultElevenLabsVoiceID is the stock Rachel voice.
	DefaultElevenLabsVoiceID = "21m00Tcm4TlvDq8ikWAM"
	// DefaultElevenLabsModelID is the default multilingual model.
	DefaultElevenLabsModelID = "eleven_multilingual_v2"
	// DefaultTTSServerPort is the first managed local TTS port.
	DefaultTTSServerPort = 8991
	// DefaultTTSBaseURL is the loopback endpoint used by the first TTS module.
	DefaultTTSBaseURL = "http://127.0.0.1:8991"
	// DefaultTTSResponseFormat is broadly playable and supports header repair.
	DefaultTTSResponseFormat = "wav"
	// DefaultTTSHealthPath is used by Faster Qwen and most compatible servers.
	DefaultTTSHealthPath = "/health"
	// DefaultChatterboxHealthPath reports whether the model actually loaded.
	DefaultChatterboxHealthPath = "/api/model-info"
	// DefaultFasterQwenModel limits VRAM relative to the 1.7B model.
	DefaultFasterQwenModel = "Qwen/Qwen3-TTS-12Hz-0.6B-Base"
	// DefaultFasterQwenVoice names the single reference configured by the script.
	DefaultFasterQwenVoice = "default"
	// DefaultFasterQwenSeed matches the pinned runner's documented examples.
	DefaultFasterQwenSeed uint32 = 1337
	// TTSSeedModeFixed reuses the saved seed for repeatable synthesis.
	TTSSeedModeFixed = "fixed"
	// TTSSeedModeVaried selects a fresh seed for each synthesis request.
	TTSSeedModeVaried = "varied"
	// DefaultChatterboxModel selects the reviewed Turbo model.
	DefaultChatterboxModel = "chatterbox-turbo"
	// DefaultChatterboxVoice is bundled by the pinned server checkout.
	DefaultChatterboxVoice = "Emily.wav"
	// TTSDeviceAuto lets a scripted server select its best available backend.
	TTSDeviceAuto = "auto"
	// TTSDeviceCUDA selects an NVIDIA CUDA runtime.
	TTSDeviceCUDA = "cuda"
	// TTSDeviceCPU selects a compatibility CPU runtime.
	TTSDeviceCPU = "cpu"
	// VoiceInputModeHandsFree keeps listening and segments phrases at silence.
	VoiceInputModeHandsFree = "hands_free"
	// VoiceInputModeHold records only while the microphone control is held.
	VoiceInputModeHold = "hold"
	// DefaultVoiceInputSensitivity balances quiet speech against room noise.
	DefaultVoiceInputSensitivity = 55
	// DefaultVoiceInputSilenceMillis closes a phrase after this quiet period.
	DefaultVoiceInputSilenceMillis = 900
)

// Settings is the versioned on-disk application settings schema.
type Settings struct {
	Version     int                 `json:"version"`
	Server      ServerSettings      `json:"server"`
	UI          UISettings          `json:"ui"`
	Media       MediaSettings       `json:"media"`
	Device      DeviceSettings      `json:"device"`
	Motion      MotionSettings      `json:"motion"`
	Autopilot   AutopilotSettings   `json:"autopilot"`
	LLM         LLMSettings         `json:"llm"`
	Voice       VoiceSettings       `json:"voice"`
	Chat        ChatSettings        `json:"chat"`
	Diagnostics DiagnosticsSettings `json:"diagnostics"`
}

// ServerSettings contains local HTTP server settings.
type ServerSettings struct {
	Port int `json:"port"`
}

// DeviceSettings contains device transport configuration.
type DeviceSettings struct {
	HSPDispatchOwner         string `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string `json:"intiface_server_address"`
	FirmwareAPIRequirement   string `json:"firmware_api_requirement"`
	APIApplicationIDSource   string `json:"api_application_id_source"`
	APIApplicationIDOverride string `json:"api_application_id_override,omitempty"`
	HandyConnectionKey       string `json:"handy_connection_key,omitempty"`
}

// Motion style preferences bias the deterministic mode planners directly
// (never only prompt text): pattern weights, speed bias, and segment pacing.
const (
	// MotionStyleGentle favors slow full strokes and longer segments.
	MotionStyleGentle = "gentle"
	// MotionStyleBalanced is the default mixed profile.
	MotionStyleBalanced = "balanced"
	// MotionStyleIntense favors pulse patterns, higher speeds, faster changes.
	MotionStyleIntense = "intense"
)

// MotionSettings contains transport-neutral motion control defaults.
type MotionSettings struct {
	SpeedMinPercent      int    `json:"speed_min_percent"`
	SpeedMaxPercent      int    `json:"speed_max_percent"`
	StrokeMinPercent     int    `json:"stroke_min_percent"`
	StrokeMaxPercent     int    `json:"stroke_max_percent"`
	ReverseDirection     bool   `json:"reverse_direction"`
	ApplyVideoSpeedLimit bool   `json:"apply_video_speed_limit"`
	Style                string `json:"style"`
}

// LLMSettings contains local model provider settings.
type LLMSettings struct {
	Provider             string `json:"provider"`
	LlamaCPPMode         string `json:"llama_cpp_mode"`
	LlamaCPPBaseURL      string `json:"llama_cpp_base_url"`
	LlamaCPPContextSize  int    `json:"llama_cpp_context_size"`
	OllamaBaseURL        string `json:"ollama_base_url"`
	OllamaModelsPath     string `json:"ollama_models_path,omitempty"`
	Model                string `json:"model"`
	PromptSet            string `json:"prompt_set"`
	RequestTimeoutMillis int    `json:"request_timeout_ms"`
	MaxOutputTokens      int    `json:"max_output_tokens"`
	ReasoningMode        string `json:"reasoning_mode"`
	// ChatVoice selects how sexual the model's reply register may be. It only
	// shapes prompt composition; the motion contract and every motion safety
	// gate are identical at every level.
	ChatVoice string `json:"chat_voice"`
	// UserAnatomy controls code-owned vocabulary independently of the partner
	// persona. CustomAnatomy and PersonaDescription are quoted as data when
	// composed into a non-utility chat prompt.
	UserAnatomy        string `json:"user_anatomy"`
	CustomAnatomy      string `json:"custom_anatomy"`
	PersonaDescription string `json:"persona_description"`
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

// VoiceSettings configures the optional voice worker processes (ADR 0003).
// Voice is off by default; first-party and custom workers all speak the
// versioned protocol. Paths are not secrets — the same trust model as other
// optional local worker executables.
type VoiceSettings struct {
	Enabled     bool   `json:"enabled"`
	TTSProvider string `json:"tts_provider"`
	ASRProvider string `json:"asr_provider"`

	// The raw command fields remain the lossless custom-provider escape hatch
	// and act as explicit worker-binary overrides for first-party providers.
	TTSWorkerPath string   `json:"tts_worker_path,omitempty"`
	TTSWorkerArgs []string `json:"tts_worker_args,omitempty"`
	ASRWorkerPath string   `json:"asr_worker_path,omitempty"`
	ASRWorkerArgs []string `json:"asr_worker_args,omitempty"`

	ElevenLabsVoiceID string `json:"elevenlabs_voice_id,omitempty"`
	ElevenLabsModelID string `json:"elevenlabs_model_id,omitempty"`

	TTSAutoLaunch     bool   `json:"tts_auto_launch"`
	TTSBaseURL        string `json:"tts_base_url,omitempty"`
	TTSModel          string `json:"tts_model,omitempty"`
	TTSVoice          string `json:"tts_voice,omitempty"`
	TTSResponseFormat string `json:"tts_response_format,omitempty"`
	TTSHealthPath     string `json:"tts_health_path,omitempty"`
	TTSModuleRoot     string `json:"tts_module_root,omitempty"`
	TTSServerPort     int    `json:"tts_server_port,omitempty"`
	TTSReferenceWAV   string `json:"tts_reference_wav,omitempty"`
	TTSReferenceText  string `json:"tts_reference_text,omitempty"`
	TTSLanguage       string `json:"tts_language,omitempty"`
	TTSDevice         string `json:"tts_device,omitempty"`
	TTSSeed           uint32 `json:"tts_seed"`
	TTSSeedMode       string `json:"tts_seed_mode"`

	ParakeetServerPath string `json:"parakeet_server_path,omitempty"`
	ParakeetModelPath  string `json:"parakeet_model_path,omitempty"`
	ParakeetServerPort int    `json:"parakeet_port,omitempty"`
	ParakeetSource     string `json:"parakeet_source"`
	ASRBaseURL         string `json:"asr_base_url,omitempty"`
	ASRModel           string `json:"asr_model,omitempty"`
	InputMode          string `json:"input_mode"`
	InputSensitivity   int    `json:"input_sensitivity"`
	InputSilenceMillis int    `json:"input_silence_ms"`
	InputNoiseSuppress bool   `json:"input_noise_suppression"`

	// SpeakReplies enqueues each displayed chat reply to the running TTS
	// worker in lockstep (ADR 0003: a spoken reply is always also shown).
	SpeakReplies bool `json:"speak_replies"`
	// ElevenLabsAPIKey is a private credential like the Handy connection
	// key: stored at rest, handed to the TTS worker process only via a
	// private environment variable, never returned by any read API.
	ElevenLabsAPIKey string `json:"elevenlabs_api_key,omitempty"`
	// OpenAITTSAPIKey is an optional bearer credential for a compatible
	// endpoint. Managed loopback modules leave it empty.
	OpenAITTSAPIKey string `json:"openai_tts_api_key,omitempty"`
}

// PublicVoiceSettings is the API-safe voice view; the ElevenLabs key is
// reduced to a set/unset flag.
type PublicVoiceSettings struct {
	Enabled            bool     `json:"enabled"`
	TTSProvider        string   `json:"tts_provider"`
	ASRProvider        string   `json:"asr_provider"`
	TTSWorkerPath      string   `json:"tts_worker_path,omitempty"`
	TTSWorkerArgs      []string `json:"tts_worker_args,omitempty"`
	ASRWorkerPath      string   `json:"asr_worker_path,omitempty"`
	ASRWorkerArgs      []string `json:"asr_worker_args,omitempty"`
	SpeakReplies       bool     `json:"speak_replies"`
	ElevenLabsVoiceID  string   `json:"elevenlabs_voice_id,omitempty"`
	ElevenLabsModelID  string   `json:"elevenlabs_model_id,omitempty"`
	TTSAutoLaunch      bool     `json:"tts_auto_launch"`
	TTSBaseURL         string   `json:"tts_base_url,omitempty"`
	TTSModel           string   `json:"tts_model,omitempty"`
	TTSVoice           string   `json:"tts_voice,omitempty"`
	TTSResponseFormat  string   `json:"tts_response_format,omitempty"`
	TTSHealthPath      string   `json:"tts_health_path,omitempty"`
	TTSModuleRoot      string   `json:"tts_module_root,omitempty"`
	TTSServerPort      int      `json:"tts_server_port,omitempty"`
	TTSReferenceWAV    string   `json:"tts_reference_wav,omitempty"`
	TTSReferenceText   string   `json:"tts_reference_text,omitempty"`
	TTSLanguage        string   `json:"tts_language,omitempty"`
	TTSDevice          string   `json:"tts_device,omitempty"`
	TTSSeed            uint32   `json:"tts_seed"`
	TTSSeedMode        string   `json:"tts_seed_mode"`
	ParakeetServerPath string   `json:"parakeet_server_path,omitempty"`
	ParakeetModelPath  string   `json:"parakeet_model_path,omitempty"`
	ParakeetServerPort int      `json:"parakeet_port,omitempty"`
	ParakeetSource     string   `json:"parakeet_source"`
	ASRBaseURL         string   `json:"asr_base_url,omitempty"`
	ASRModel           string   `json:"asr_model,omitempty"`
	InputMode          string   `json:"input_mode"`
	InputSensitivity   int      `json:"input_sensitivity"`
	InputSilenceMillis int      `json:"input_silence_ms"`
	InputNoiseSuppress bool     `json:"input_noise_suppression"`
	ElevenLabsKeySet   bool     `json:"elevenlabs_key_set"`
	OpenAITTSKeySet    bool     `json:"openai_tts_key_set"`
}

// VoiceUpdate is the API write payload for voice settings. A nil API key
// keeps the stored secret; ClearElevenLabsKey removes it.
type VoiceUpdate struct {
	Enabled            bool     `json:"enabled"`
	TTSProvider        string   `json:"tts_provider"`
	ASRProvider        string   `json:"asr_provider"`
	TTSWorkerPath      string   `json:"tts_worker_path"`
	TTSWorkerArgs      []string `json:"tts_worker_args"`
	ASRWorkerPath      string   `json:"asr_worker_path"`
	ASRWorkerArgs      []string `json:"asr_worker_args"`
	SpeakReplies       bool     `json:"speak_replies"`
	ElevenLabsVoiceID  string   `json:"elevenlabs_voice_id"`
	ElevenLabsModelID  string   `json:"elevenlabs_model_id"`
	TTSAutoLaunch      *bool    `json:"tts_auto_launch,omitempty"`
	TTSBaseURL         string   `json:"tts_base_url"`
	TTSModel           string   `json:"tts_model"`
	TTSVoice           string   `json:"tts_voice"`
	TTSResponseFormat  string   `json:"tts_response_format"`
	TTSHealthPath      string   `json:"tts_health_path"`
	TTSModuleRoot      string   `json:"tts_module_root"`
	TTSServerPort      int      `json:"tts_server_port"`
	TTSReferenceWAV    string   `json:"tts_reference_wav"`
	TTSReferenceText   string   `json:"tts_reference_text"`
	TTSLanguage        string   `json:"tts_language"`
	TTSDevice          string   `json:"tts_device"`
	TTSSeed            *uint32  `json:"tts_seed,omitempty"`
	TTSSeedMode        *string  `json:"tts_seed_mode,omitempty"`
	ParakeetServerPath string   `json:"parakeet_server_path"`
	ParakeetModelPath  string   `json:"parakeet_model_path"`
	ParakeetServerPort int      `json:"parakeet_port"`
	ParakeetSource     *string  `json:"parakeet_source,omitempty"`
	ASRBaseURL         string   `json:"asr_base_url"`
	ASRModel           string   `json:"asr_model"`
	InputMode          *string  `json:"input_mode,omitempty"`
	InputSensitivity   *int     `json:"input_sensitivity,omitempty"`
	InputSilenceMillis *int     `json:"input_silence_ms,omitempty"`
	InputNoiseSuppress *bool    `json:"input_noise_suppression,omitempty"`
	ElevenLabsAPIKey   *string  `json:"elevenlabs_api_key,omitempty"`
	ClearElevenLabsKey bool     `json:"clear_elevenlabs_key"`
	OpenAITTSAPIKey    *string  `json:"openai_tts_api_key,omitempty"`
	ClearOpenAITTSKey  bool     `json:"clear_openai_tts_key"`
}

// DiagnosticsSettings contains logging and diagnostics verbosity settings.
type DiagnosticsSettings struct {
	Verbosity string `json:"verbosity"`
}

// ChatSettings controls process-start restoration. Saved chats are always
// retained; preserving an unsaved current chat is an explicit privacy choice.
type ChatSettings struct {
	StartupBehavior   string `json:"startup_behavior"`
	KeepUnsavedOnExit bool   `json:"keep_unsaved_on_exit"`
}

// PublicSettings is the API-safe settings view. It intentionally omits secrets.
type PublicSettings struct {
	Version     int                       `json:"version"`
	Server      ServerSettings            `json:"server"`
	UI          UISettings                `json:"ui"`
	Media       MediaSettings             `json:"media"`
	Device      PublicDeviceSettings      `json:"device"`
	Motion      MotionSettings            `json:"motion"`
	Autopilot   AutopilotSettings         `json:"autopilot"`
	LLM         LLMSettings               `json:"llm"`
	Voice       PublicVoiceSettings       `json:"voice"`
	Chat        ChatSettings              `json:"chat"`
	Diagnostics DiagnosticsSettings       `json:"diagnostics"`
	Options     PublicSettingsOptionHints `json:"options"`
}

// PublicDeviceSettings is the API-safe device settings view.
type PublicDeviceSettings struct {
	HSPDispatchOwner         string `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string `json:"intiface_server_address"`
	FirmwareAPIRequirement   string `json:"firmware_api_requirement"`
	APIApplicationIDSource   string `json:"api_application_id_source"`
	APIApplicationIDOverride string `json:"api_application_id_override,omitempty"`
	ConnectionKeySet         bool   `json:"connection_key_set"`
}

// PublicSettingsOptionHints lists valid option values for settings clients.
type PublicSettingsOptionHints struct {
	HSPDispatchOwners       []string `json:"hsp_dispatch_owners"`
	APIApplicationIDSources []string `json:"api_application_id_sources"`
	DiagnosticsVerbosities  []string `json:"diagnostics_verbosities"`
	MotionStyles            []string `json:"motion_styles"`
	AutopilotSpeechCadences []string `json:"autopilot_speech_cadences"`
	AutopilotMotionCadences []string `json:"autopilot_motion_cadences"`
	AutopilotAuthorities    []string `json:"autopilot_speech_motion_authorities"`
	LLMProviders            []string `json:"llm_providers"`
	LlamaCPPModes           []string `json:"llama_cpp_modes"`
	LlamaCPPContextSizes    []int    `json:"llama_cpp_context_sizes"`
	LLMReasoningModes       []string `json:"llm_reasoning_modes"`
	LLMMaxOutputTokens      []int    `json:"llm_max_output_tokens"`
	LLMChatVoices           []string `json:"llm_chat_voices"`
	LLMUserAnatomies        []string `json:"llm_user_anatomies"`
	LLMReactionStyles       []string `json:"llm_reaction_styles"`
	PromptSets              []string `json:"prompt_sets"`
	TTSProviders            []string `json:"tts_providers"`
	ASRProviders            []string `json:"asr_providers"`
	ParakeetSources         []string `json:"parakeet_sources"`
	TTSDevices              []string `json:"tts_devices"`
	ChatStartupBehaviors    []string `json:"chat_startup_behaviors"`
	Locales                 []string `json:"locales"`
	Themes                  []string `json:"themes"`
}

// LLMUpdate is the settings API write shape. New tuning fields are pointers so
// older clients that omit them preserve the current persisted values.
type LLMUpdate struct {
	Provider             string  `json:"provider"`
	LlamaCPPMode         string  `json:"llama_cpp_mode"`
	LlamaCPPBaseURL      string  `json:"llama_cpp_base_url"`
	LlamaCPPContextSize  *int    `json:"llama_cpp_context_size,omitempty"`
	OllamaBaseURL        string  `json:"ollama_base_url"`
	OllamaModelsPath     string  `json:"ollama_models_path,omitempty"`
	Model                string  `json:"model"`
	PromptSet            string  `json:"prompt_set"`
	RequestTimeoutMillis int     `json:"request_timeout_ms"`
	MaxOutputTokens      *int    `json:"max_output_tokens,omitempty"`
	ReasoningMode        *string `json:"reasoning_mode,omitempty"`
	// ChatVoice replaces the saved voice when present; omitted preserves the
	// current persisted value (older clients keep working).
	ChatVoice *string `json:"chat_voice,omitempty"`
	// UserAnatomy, CustomAnatomy, and PersonaDescription preserve saved values
	// when omitted by an older settings client.
	UserAnatomy        *string `json:"user_anatomy,omitempty"`
	CustomAnatomy      *string `json:"custom_anatomy,omitempty"`
	PersonaDescription *string `json:"persona_description,omitempty"`
	// MotionCapabilities replaces the saved gates when present; omitted
	// preserves the current persisted values (older clients keep working).
	MotionCapabilities *LLMMotionCapabilities `json:"motion_capabilities,omitempty"`
}

// LLMUpdateFromSettings creates a complete write payload from a settings view.
func LLMUpdateFromSettings(settings LLMSettings) LLMUpdate {
	return LLMUpdate{
		Provider:             settings.Provider,
		LlamaCPPMode:         settings.LlamaCPPMode,
		LlamaCPPBaseURL:      settings.LlamaCPPBaseURL,
		LlamaCPPContextSize:  &settings.LlamaCPPContextSize,
		OllamaBaseURL:        settings.OllamaBaseURL,
		OllamaModelsPath:     settings.OllamaModelsPath,
		Model:                settings.Model,
		PromptSet:            settings.PromptSet,
		RequestTimeoutMillis: settings.RequestTimeoutMillis,
		MaxOutputTokens:      &settings.MaxOutputTokens,
		ReasoningMode:        &settings.ReasoningMode,
		ChatVoice:            &settings.ChatVoice,
		UserAnatomy:          &settings.UserAnatomy,
		CustomAnatomy:        &settings.CustomAnatomy,
		PersonaDescription:   &settings.PersonaDescription,
		MotionCapabilities:   settings.MotionCapabilities,
	}
}

// SettingsUpdate is the payload accepted by the settings API.
type SettingsUpdate struct {
	Server             ServerSettings      `json:"server"`
	UI                 *UISettings         `json:"ui,omitempty"`
	Media              *MediaUpdate        `json:"media,omitempty"`
	Device             DeviceUpdate        `json:"device"`
	Motion             MotionSettings      `json:"motion"`
	Autopilot          *AutopilotSettings  `json:"autopilot,omitempty"`
	LLM                LLMUpdate           `json:"llm"`
	Voice              VoiceUpdate         `json:"voice"`
	Chat               *ChatSettings       `json:"chat,omitempty"`
	Diagnostics        DiagnosticsSettings `json:"diagnostics"`
	ClearConnectionKey bool                `json:"clear_connection_key"`
}

// DeviceUpdate is the API write payload for device settings.
type DeviceUpdate struct {
	HSPDispatchOwner         string  `json:"hsp_dispatch_owner"`
	IntifaceServerAddress    string  `json:"intiface_server_address"`
	FirmwareAPIRequirement   string  `json:"firmware_api_requirement"`
	APIApplicationIDSource   string  `json:"api_application_id_source"`
	APIApplicationIDOverride string  `json:"api_application_id_override"`
	HandyConnectionKey       *string `json:"handy_connection_key,omitempty"`
}

// DefaultSettings returns the current built-in settings.
func DefaultSettings() Settings {
	return Settings{
		Version: CurrentSettingsVersion,
		Server: ServerSettings{
			Port: DefaultServerPort,
		},
		UI: UISettings{
			Locale: LocaleEnglish,
			Theme:  ThemeSteelAzure,
		},
		Media: MediaSettings{
			RemoveMissingOnScan: true,
			ReencodeCodec:       ReencodeCodecH264,
			ReencodeCRFH264:     DefaultReencodeCRFH264,
			ReencodeCRFH265:     DefaultReencodeCRFH265,
			ReencodePreset:      DefaultReencodePreset,
			ReencodeAudioKbps:   DefaultReencodeAudioKbps,
		},
		Device: DeviceSettings{
			HSPDispatchOwner:       DispatchOwnerCloudREST,
			IntifaceServerAddress:  DefaultIntifaceServerAddress,
			FirmwareAPIRequirement: FirmwareAPIRequirementRequired,
			APIApplicationIDSource: ApplicationIDSourceBundled,
		},
		Motion: MotionSettings{
			SpeedMinPercent:  20,
			SpeedMaxPercent:  80,
			StrokeMinPercent: 0,
			StrokeMaxPercent: 100,
			Style:            MotionStyleBalanced,
		},
		Autopilot: DefaultAutopilotSettings(),
		LLM: LLMSettings{
			Provider:             LLMProviderLlamaCPP,
			LlamaCPPMode:         LlamaCPPModeManaged,
			LlamaCPPBaseURL:      DefaultLlamaCPPBaseURL,
			LlamaCPPContextSize:  DefaultLlamaCPPContextSize,
			OllamaBaseURL:        DefaultOllamaBaseURL,
			Model:                DefaultLLMModel,
			PromptSet:            PromptSetMagicHandyMotionV1,
			RequestTimeoutMillis: DefaultLLMRequestTimeoutMillis,
			MaxOutputTokens:      DefaultLLMMaxOutputTokens,
			ReasoningMode:        LLMReasoningOff,
			ChatVoice:            LLMChatVoiceUtility,
			UserAnatomy:          LLMUserAnatomyPenis,
		},
		Voice: VoiceSettings{
			TTSProvider:        VoiceProviderNone,
			ASRProvider:        VoiceProviderNone,
			ElevenLabsVoiceID:  DefaultElevenLabsVoiceID,
			ElevenLabsModelID:  DefaultElevenLabsModelID,
			TTSBaseURL:         DefaultTTSBaseURL,
			TTSResponseFormat:  DefaultTTSResponseFormat,
			TTSHealthPath:      DefaultTTSHealthPath,
			TTSServerPort:      DefaultTTSServerPort,
			TTSLanguage:        "Auto",
			TTSDevice:          TTSDeviceAuto,
			TTSSeed:            DefaultFasterQwenSeed,
			TTSSeedMode:        TTSSeedModeFixed,
			ParakeetServerPort: DefaultParakeetServerPort,
			ParakeetSource:     ParakeetSourceApp,
			InputMode:          VoiceInputModeHandsFree,
			InputSensitivity:   DefaultVoiceInputSensitivity,
			InputSilenceMillis: DefaultVoiceInputSilenceMillis,
			InputNoiseSuppress: true,
		},
		Chat: ChatSettings{
			StartupBehavior: ChatStartupPrevious,
		},
		Diagnostics: DiagnosticsSettings{
			Verbosity: DiagnosticsVerbosityNormal,
		},
	}
}

// Public returns an API-safe settings view with secrets redacted.
func (s Settings) Public() PublicSettings {
	return PublicSettings{
		Version: s.Version,
		Server:  s.Server,
		UI:      s.UI,
		// Every MediaSettings field has to be copied explicitly here. A field
		// added to the struct but missed in this projection saves correctly and
		// then reads back as its zero value, which unit tests over ApplyUpdate
		// and NormalizeSettings cannot see.
		Media: MediaSettings{
			ScriptOffsetMillis:          s.Media.ScriptOffsetMillis,
			ScriptSmoothingPercent:      s.Media.ScriptSmoothingPercent,
			PeakRoundingMillis:          s.Media.PeakRoundingMillis,
			LibraryPaths:                append([]string{}, s.Media.LibraryPaths...),
			AutoScanOnStartup:           s.Media.AutoScanOnStartup,
			RemoveMissingOnScan:         s.Media.RemoveMissingOnScan,
			FFmpegPath:                  s.Media.FFmpegPath,
			ConvertH265ForCompatibility: s.Media.ConvertH265ForCompatibility,
			ReencodeCodec:               s.Media.ReencodeCodec,
			ReencodeCRFH264:             s.Media.ReencodeCRFH264,
			ReencodeCRFH265:             s.Media.ReencodeCRFH265,
			ReencodePreset:              s.Media.ReencodePreset,
			ReencodeAudioKbps:           s.Media.ReencodeAudioKbps,
			GenerateThumbnailsOnScan:    s.Media.GenerateThumbnailsOnScan,
			ConvertIncompatibleOnScan:   s.Media.ConvertIncompatibleOnScan,
			ShowSupersededOriginals:     s.Media.ShowSupersededOriginals,
		},
		Device: PublicDeviceSettings{
			HSPDispatchOwner:         s.Device.HSPDispatchOwner,
			IntifaceServerAddress:    s.Device.IntifaceServerAddress,
			FirmwareAPIRequirement:   s.Device.FirmwareAPIRequirement,
			APIApplicationIDSource:   s.Device.APIApplicationIDSource,
			APIApplicationIDOverride: s.Device.APIApplicationIDOverride,
			ConnectionKeySet:         s.Device.HandyConnectionKey != "",
		},
		Motion:      s.Motion,
		Autopilot:   s.Autopilot,
		LLM:         s.LLM,
		Voice:       publicVoiceSettings(s.Voice),
		Chat:        s.Chat,
		Diagnostics: s.Diagnostics,
		Options:     publicSettingsOptionHints(),
	}
}

func publicVoiceSettings(settings VoiceSettings) PublicVoiceSettings {
	return PublicVoiceSettings{
		Enabled:            settings.Enabled,
		TTSProvider:        settings.TTSProvider,
		ASRProvider:        settings.ASRProvider,
		TTSWorkerPath:      settings.TTSWorkerPath,
		TTSWorkerArgs:      cloneStrings(settings.TTSWorkerArgs),
		ASRWorkerPath:      settings.ASRWorkerPath,
		ASRWorkerArgs:      cloneStrings(settings.ASRWorkerArgs),
		SpeakReplies:       settings.SpeakReplies,
		ElevenLabsVoiceID:  settings.ElevenLabsVoiceID,
		ElevenLabsModelID:  settings.ElevenLabsModelID,
		TTSAutoLaunch:      settings.TTSAutoLaunch,
		TTSBaseURL:         settings.TTSBaseURL,
		TTSModel:           settings.TTSModel,
		TTSVoice:           settings.TTSVoice,
		TTSResponseFormat:  settings.TTSResponseFormat,
		TTSHealthPath:      settings.TTSHealthPath,
		TTSModuleRoot:      settings.TTSModuleRoot,
		TTSServerPort:      settings.TTSServerPort,
		TTSReferenceWAV:    settings.TTSReferenceWAV,
		TTSReferenceText:   settings.TTSReferenceText,
		TTSLanguage:        settings.TTSLanguage,
		TTSDevice:          settings.TTSDevice,
		TTSSeed:            settings.TTSSeed,
		TTSSeedMode:        settings.TTSSeedMode,
		ParakeetServerPath: settings.ParakeetServerPath,
		ParakeetModelPath:  settings.ParakeetModelPath,
		ParakeetServerPort: settings.ParakeetServerPort,
		ParakeetSource:     settings.ParakeetSource,
		ASRBaseURL:         settings.ASRBaseURL,
		ASRModel:           settings.ASRModel,
		InputMode:          settings.InputMode,
		InputSensitivity:   settings.InputSensitivity,
		InputSilenceMillis: settings.InputSilenceMillis,
		InputNoiseSuppress: settings.InputNoiseSuppress,
		ElevenLabsKeySet:   settings.ElevenLabsAPIKey != "",
		OpenAITTSKeySet:    settings.OpenAITTSAPIKey != "",
	}
}

// ApplyUpdate merges a settings API payload into the current settings.
func (s Settings) ApplyUpdate(update SettingsUpdate) (Settings, error) {
	next := s
	next.Version = CurrentSettingsVersion
	next.Server = update.Server
	if update.UI != nil {
		next.UI.Locale = strings.TrimSpace(update.UI.Locale)
		theme := strings.TrimSpace(update.UI.Theme)
		if theme != "" {
			next.UI.Theme = theme
		}
	}
	if update.Media != nil {
		next.Media = normalizeMediaSettings(mergeMediaUpdate(next.Media, *update.Media))
	}
	next.Device.HSPDispatchOwner = update.Device.HSPDispatchOwner
	next.Device.IntifaceServerAddress = strings.TrimSpace(update.Device.IntifaceServerAddress)
	next.Device.FirmwareAPIRequirement = update.Device.FirmwareAPIRequirement
	next.Device.APIApplicationIDSource = update.Device.APIApplicationIDSource
	next.Device.APIApplicationIDOverride = strings.TrimSpace(update.Device.APIApplicationIDOverride)
	next.Motion = update.Motion
	if update.Autopilot != nil {
		next.Autopilot = *update.Autopilot
	}
	nextLLM, err := applyLLMUpdate(s.LLM, update.LLM)
	if err != nil {
		return Settings{}, err
	}
	next.LLM = nextLLM
	next.Voice = applyVoiceUpdate(s.Voice, update.Voice)
	if update.Chat != nil {
		next.Chat = *update.Chat
	}
	next.Diagnostics = update.Diagnostics

	if update.Voice.ClearElevenLabsKey {
		next.Voice.ElevenLabsAPIKey = ""
	} else if update.Voice.ElevenLabsAPIKey != nil {
		next.Voice.ElevenLabsAPIKey = strings.TrimSpace(*update.Voice.ElevenLabsAPIKey)
	}
	if update.Voice.ClearOpenAITTSKey {
		next.Voice.OpenAITTSAPIKey = ""
	} else if update.Voice.OpenAITTSAPIKey != nil {
		next.Voice.OpenAITTSAPIKey = strings.TrimSpace(*update.Voice.OpenAITTSAPIKey)
	}

	if update.ClearConnectionKey {
		next.Device.HandyConnectionKey = ""
	} else if update.Device.HandyConnectionKey != nil {
		next.Device.HandyConnectionKey = strings.TrimSpace(*update.Device.HandyConnectionKey)
	}

	if next.Device.APIApplicationIDSource == ApplicationIDSourceBundled {
		next.Device.APIApplicationIDOverride = ""
	}

	return NormalizeSettings(next)
}

func applyVoiceUpdate(current VoiceSettings, update VoiceUpdate) VoiceSettings {
	ttsAutoLaunch := current.TTSAutoLaunch
	if update.TTSAutoLaunch != nil {
		ttsAutoLaunch = *update.TTSAutoLaunch
	}
	parakeetSource := current.ParakeetSource
	if update.ParakeetSource != nil {
		parakeetSource = *update.ParakeetSource
	}
	inputMode := current.InputMode
	if update.InputMode != nil {
		inputMode = *update.InputMode
	}
	inputSensitivity := current.InputSensitivity
	if update.InputSensitivity != nil {
		inputSensitivity = *update.InputSensitivity
	}
	inputSilenceMillis := current.InputSilenceMillis
	if update.InputSilenceMillis != nil {
		inputSilenceMillis = *update.InputSilenceMillis
	}
	inputNoiseSuppress := current.InputNoiseSuppress
	if update.InputNoiseSuppress != nil {
		inputNoiseSuppress = *update.InputNoiseSuppress
	}
	ttsSeed := current.TTSSeed
	if update.TTSSeed != nil {
		ttsSeed = *update.TTSSeed
	}
	ttsSeedMode := current.TTSSeedMode
	if update.TTSSeedMode != nil {
		ttsSeedMode = *update.TTSSeedMode
	}
	return normalizeVoiceStrings(VoiceSettings{
		Enabled:            update.Enabled,
		TTSProvider:        update.TTSProvider,
		ASRProvider:        update.ASRProvider,
		TTSWorkerPath:      update.TTSWorkerPath,
		TTSWorkerArgs:      update.TTSWorkerArgs,
		ASRWorkerPath:      update.ASRWorkerPath,
		ASRWorkerArgs:      update.ASRWorkerArgs,
		SpeakReplies:       update.SpeakReplies,
		ElevenLabsVoiceID:  update.ElevenLabsVoiceID,
		ElevenLabsModelID:  update.ElevenLabsModelID,
		TTSAutoLaunch:      ttsAutoLaunch,
		TTSBaseURL:         update.TTSBaseURL,
		TTSModel:           update.TTSModel,
		TTSVoice:           update.TTSVoice,
		TTSResponseFormat:  update.TTSResponseFormat,
		TTSHealthPath:      update.TTSHealthPath,
		TTSModuleRoot:      update.TTSModuleRoot,
		TTSServerPort:      update.TTSServerPort,
		TTSReferenceWAV:    update.TTSReferenceWAV,
		TTSReferenceText:   update.TTSReferenceText,
		TTSLanguage:        update.TTSLanguage,
		TTSDevice:          update.TTSDevice,
		TTSSeed:            ttsSeed,
		TTSSeedMode:        ttsSeedMode,
		ParakeetServerPath: update.ParakeetServerPath,
		ParakeetModelPath:  update.ParakeetModelPath,
		ParakeetServerPort: update.ParakeetServerPort,
		ParakeetSource:     parakeetSource,
		ASRBaseURL:         update.ASRBaseURL,
		ASRModel:           update.ASRModel,
		InputMode:          inputMode,
		InputSensitivity:   inputSensitivity,
		InputSilenceMillis: inputSilenceMillis,
		InputNoiseSuppress: inputNoiseSuppress,
		// The stored key survives unless explicitly replaced or cleared.
		ElevenLabsAPIKey: current.ElevenLabsAPIKey,
		OpenAITTSAPIKey:  current.OpenAITTSAPIKey,
	})
}

// NormalizeSettings validates settings and fills any invalid version metadata.
func NormalizeSettings(settings Settings) (Settings, error) {
	settings = applyMissingDefaults(settings)
	if settings.Version == 0 {
		settings.Version = CurrentSettingsVersion
	}
	if settings.Version > CurrentSettingsVersion {
		return Settings{}, fmt.Errorf("unsupported settings version %d", settings.Version)
	}
	if err := validateSettings(settings); err != nil {
		return Settings{}, err
	}
	settings.Version = CurrentSettingsVersion
	return settings, nil
}

// MigrateSettings moves older settings schema versions to the current version.
func MigrateSettings(settings Settings, sourceVersion int) (Settings, bool, error) {
	if sourceVersion > CurrentSettingsVersion {
		return Settings{}, false, fmt.Errorf("unsupported settings version %d", sourceVersion)
	}
	if sourceVersion == CurrentSettingsVersion {
		normalized, err := NormalizeSettings(settings)
		return normalized, false, err
	}

	// NeuTTS was removed in schema 2 after failing release quality acceptance.
	// Do not leave an upgraded app repeatedly trying to start a worker that no
	// longer ships, and do not silently select a replacement with different
	// runtime and privacy implications.
	if sourceVersion < 2 && settings.Voice.TTSProvider == "neutts_air" {
		settings.Voice.TTSProvider = VoiceProviderNone
		settings.Voice.SpeakReplies = false
		settings.Voice.TTSAutoLaunch = false
	}
	settings.Version = CurrentSettingsVersion
	normalized, err := NormalizeSettings(settings)
	return normalized, true, err
}

func loadSettingsFromBytes(data []byte) (Settings, bool, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var header struct {
		Version int                        `json:"version"`
		Voice   map[string]json.RawMessage `json:"voice"`
		LLM     map[string]json.RawMessage `json:"llm"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return Settings{}, false, err
	}

	settings := DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, false, err
	}
	// Defaults are unmarshaled first, so an absent provider discriminator
	// would otherwise look identical to an explicit "none". Clear only absent
	// fields here so applyMissingDefaults can classify legacy raw commands as
	// custom while preserving an intentional none selection with hidden data.
	if _, present := header.Voice["tts_provider"]; !present {
		settings.Voice.TTSProvider = ""
	}
	if _, present := header.Voice["asr_provider"]; !present {
		settings.Voice.ASRProvider = ""
	}
	if _, present := header.Voice["parakeet_source"]; !present {
		settings.Voice.ParakeetSource = ""
	}
	// Existing non-utility users must not silently inherit penis-specific
	// wording merely because their saved document predates anatomy settings.
	// Custom with empty wording is the supported neutral profile state; fresh
	// installs still use the reviewed STGPT-RV penis default.
	if _, present := header.LLM["user_anatomy"]; !present {
		settings.LLM.UserAnatomy = LLMUserAnatomyCustom
		settings.LLM.CustomAnatomy = ""
	}

	localeFallback := false
	if !IsSupportedLocale(strings.TrimSpace(settings.UI.Locale)) {
		settings.UI.Locale = LocaleEnglish
		localeFallback = true
	}
	themeFallback := false
	settings.UI.Theme = strings.TrimSpace(settings.UI.Theme)
	if !IsSupportedUITheme(settings.UI.Theme) {
		settings.UI.Theme = ThemeSteelAzure
		themeFallback = true
	}

	migratedSettings, migrated, err := MigrateSettings(settings, header.Version)
	return migratedSettings, migrated || localeFallback || themeFallback, err
}

func validateSettings(settings Settings) error {
	if settings.Server.Port < 1 || settings.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}
	if !oneOf(settings.UI.Locale, LocaleEnglish, LocaleSpanish, LocalePortugueseBrazil, LocaleSimplifiedChinese, LocaleJapanese) {
		return fmt.Errorf("unknown UI locale %q", settings.UI.Locale)
	}
	if !IsSupportedUITheme(settings.UI.Theme) {
		return fmt.Errorf("unknown UI theme %q", settings.UI.Theme)
	}
	if !oneOf(settings.Device.HSPDispatchOwner, DispatchOwnerCloudREST, DispatchOwnerBrowserBluetooth, DispatchOwnerIntiface) {
		return fmt.Errorf("unknown dispatch owner %q", settings.Device.HSPDispatchOwner)
	}
	if err := validateIntifaceServerAddress(settings.Device.IntifaceServerAddress); err != nil {
		return err
	}
	if settings.Device.FirmwareAPIRequirement != FirmwareAPIRequirementRequired {
		return errors.New("firmware/API requirement must remain firmware_v4_api_v3_required")
	}
	if !oneOf(settings.Device.APIApplicationIDSource, ApplicationIDSourceBundled, ApplicationIDSourceDeveloperOverride) {
		return fmt.Errorf("unknown API application ID source %q", settings.Device.APIApplicationIDSource)
	}
	if !oneOf(settings.Diagnostics.Verbosity, DiagnosticsVerbosityNormal, DiagnosticsVerbosityDebug, DiagnosticsVerbosityTrace) {
		return fmt.Errorf("unknown diagnostics verbosity %q", settings.Diagnostics.Verbosity)
	}
	if !oneOf(settings.Chat.StartupBehavior, ChatStartupPrevious, ChatStartupNew) {
		return fmt.Errorf("unknown chat startup behavior %q", settings.Chat.StartupBehavior)
	}
	if settings.Chat.StartupBehavior == ChatStartupNew && settings.Chat.KeepUnsavedOnExit {
		return errors.New("a new chat at startup cannot also retain the previous unsaved chat")
	}
	if err := validateMotionSettings(settings.Motion); err != nil {
		return err
	}
	if err := validateAutopilotSettings(settings.Autopilot); err != nil {
		return err
	}
	if err := validateLLMSettings(settings.LLM); err != nil {
		return err
	}
	if err := validateMediaSettings(settings.Media); err != nil {
		return err
	}
	return validateVoiceSettings(settings.Voice)
}

func applyMissingDefaults(settings Settings) Settings {
	defaults := DefaultSettings()
	if settings.Server.Port == 0 {
		settings.Server.Port = defaults.Server.Port
	}
	settings.UI.Locale = strings.TrimSpace(settings.UI.Locale)
	if settings.UI.Locale == "" {
		settings.UI.Locale = defaults.UI.Locale
	}
	settings.UI.Theme = strings.TrimSpace(settings.UI.Theme)
	if settings.UI.Theme == "" {
		settings.UI.Theme = defaults.UI.Theme
	}
	if settings.Device.HSPDispatchOwner == "" {
		settings.Device.HSPDispatchOwner = defaults.Device.HSPDispatchOwner
	}
	settings.Device.IntifaceServerAddress = strings.TrimSpace(settings.Device.IntifaceServerAddress)
	if settings.Device.IntifaceServerAddress == "" {
		settings.Device.IntifaceServerAddress = defaults.Device.IntifaceServerAddress
	}
	if settings.Device.FirmwareAPIRequirement == "" {
		settings.Device.FirmwareAPIRequirement = defaults.Device.FirmwareAPIRequirement
	}
	settings.Device.APIApplicationIDSource, settings.Device.APIApplicationIDOverride = normalizeAPIApplicationID(
		settings.Device.APIApplicationIDSource,
		settings.Device.APIApplicationIDOverride,
	)
	if settings.Motion.SpeedMinPercent == 0 {
		settings.Motion.SpeedMinPercent = defaults.Motion.SpeedMinPercent
	}
	if settings.Motion.SpeedMaxPercent == 0 {
		settings.Motion.SpeedMaxPercent = defaults.Motion.SpeedMaxPercent
	}
	if settings.Motion.Style == "" {
		settings.Motion.Style = defaults.Motion.Style
	}
	if settings.Motion.StrokeMaxPercent == 0 {
		settings.Motion.StrokeMaxPercent = defaults.Motion.StrokeMaxPercent
	}
	settings.Autopilot = applyMissingAutopilotDefaults(settings.Autopilot, defaults.Autopilot)
	settings.LLM = applyMissingLLMDefaults(settings.LLM, defaults.LLM)
	settings.Voice = applyMissingVoiceDefaults(settings.Voice, defaults.Voice)
	if settings.Chat.StartupBehavior == "" {
		settings.Chat.StartupBehavior = defaults.Chat.StartupBehavior
	}
	settings.Media = normalizeMediaSettings(settings.Media)
	settings.LLM = normalizeLLMStrings(settings.LLM)
	settings.Voice = normalizeVoiceStrings(settings.Voice)
	if settings.Diagnostics.Verbosity == "" {
		settings.Diagnostics.Verbosity = defaults.Diagnostics.Verbosity
	}
	return settings
}

func applyMissingLLMDefaults(settings LLMSettings, defaults LLMSettings) LLMSettings {
	if settings.Provider == "" {
		settings.Provider = defaults.Provider
	}
	if settings.LlamaCPPMode == "" {
		settings.LlamaCPPMode = defaults.LlamaCPPMode
	}
	if settings.LlamaCPPBaseURL == "" {
		settings.LlamaCPPBaseURL = defaults.LlamaCPPBaseURL
	}
	if settings.LlamaCPPContextSize == 0 {
		settings.LlamaCPPContextSize = defaults.LlamaCPPContextSize
	}
	if settings.OllamaBaseURL == "" {
		settings.OllamaBaseURL = defaults.OllamaBaseURL
	}
	if settings.Model == "" {
		settings.Model = defaults.Model
	}
	if settings.PromptSet == "" {
		settings.PromptSet = defaults.PromptSet
	}
	if settings.ChatVoice == "" {
		settings.ChatVoice = defaults.ChatVoice
	}
	if settings.UserAnatomy == "" {
		settings.UserAnatomy = defaults.UserAnatomy
	}
	if settings.RequestTimeoutMillis == 0 {
		settings.RequestTimeoutMillis = defaults.RequestTimeoutMillis
	}
	if settings.MaxOutputTokens == 0 {
		settings.MaxOutputTokens = defaults.MaxOutputTokens
	}
	if settings.ReasoningMode == "" {
		settings.ReasoningMode = defaults.ReasoningMode
	}
	return settings
}

func normalizeAPIApplicationID(source string, override string) (string, string) {
	override = strings.TrimSpace(override)
	if source == "" || (source == ApplicationIDSourceDeveloperOverride && override == "") {
		source = ApplicationIDSourceBundled
	}
	if source == ApplicationIDSourceBundled {
		override = ""
	}
	return source, override
}

func validateMotionSettings(settings MotionSettings) error {
	if settings.SpeedMinPercent < 1 || settings.SpeedMaxPercent > 100 {
		return errors.New("motion speed bounds must be between 1 and 100")
	}
	if settings.SpeedMinPercent > settings.SpeedMaxPercent {
		return errors.New("motion speed minimum cannot exceed maximum")
	}
	if settings.StrokeMinPercent < 0 || settings.StrokeMaxPercent > 100 {
		return errors.New("stroke bounds must be between 0 and 100")
	}
	if settings.StrokeMinPercent >= settings.StrokeMaxPercent {
		return errors.New("stroke minimum must be lower than maximum")
	}
	if !oneOf(settings.Style, MotionStyleGentle, MotionStyleBalanced, MotionStyleIntense) {
		return fmt.Errorf("unknown motion style %q", settings.Style)
	}
	return nil
}

func validateIntifaceServerAddress(address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("intiface server address must be a valid URL: %w", err)
	}
	if !parsed.IsAbs() || parsed.Hostname() == "" {
		return errors.New("intiface server address must be an absolute ws or wss URL with a host")
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return errors.New("intiface server address scheme must be ws or wss")
	}
	if parsed.User != nil {
		return errors.New("intiface server address must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return errors.New("intiface server address must not include a query")
	}
	if parsed.Fragment != "" || strings.Contains(address, "#") {
		return errors.New("intiface server address must not include a fragment")
	}
	return nil
}

func validateLLMSettings(settings LLMSettings) error {
	if !oneOf(settings.Provider, LLMProviderLlamaCPP, LLMProviderOllama) {
		return fmt.Errorf("unknown LLM provider %q", settings.Provider)
	}
	if !oneOf(settings.LlamaCPPMode, LlamaCPPModeManaged, LlamaCPPModeExternal) {
		return fmt.Errorf("unknown llama.cpp mode %q", settings.LlamaCPPMode)
	}
	if settings.LlamaCPPBaseURL == "" {
		return errors.New("llama.cpp base URL is required")
	}
	if settings.OllamaBaseURL == "" {
		return errors.New("ollama base URL is required")
	}
	if err := validateLLMBaseURL("llama.cpp", settings.LlamaCPPBaseURL); err != nil {
		return err
	}
	if err := validateLLMBaseURL("Ollama", settings.OllamaBaseURL); err != nil {
		return err
	}
	if !oneOfInt(settings.LlamaCPPContextSize, LlamaCPPContextSizes()...) {
		return fmt.Errorf("unsupported managed llama.cpp context size %d", settings.LlamaCPPContextSize)
	}
	if settings.Model == "" {
		return errors.New("LLM model is required")
	}
	// Prompt sets are dynamic (built-in templates plus user-created sets in
	// the prompt library), so config only requires a non-empty identifier;
	// the chat layer falls back to the bundled default if a selection is gone.
	if settings.PromptSet == "" {
		return errors.New("prompt set is required")
	}
	if settings.RequestTimeoutMillis < 1000 || settings.RequestTimeoutMillis > 300000 {
		return errors.New("LLM request timeout must be between 1000 and 300000 milliseconds")
	}
	if settings.MaxOutputTokens < 64 || settings.MaxOutputTokens > 4096 {
		return errors.New("LLM output limit must be between 64 and 4096 tokens")
	}
	if !oneOf(settings.ReasoningMode, LLMReasoningAuto, LLMReasoningOff) {
		return fmt.Errorf("unknown LLM reasoning mode %q", settings.ReasoningMode)
	}
	if !ValidLLMChatVoice(settings.ChatVoice) {
		return fmt.Errorf("unknown LLM chat voice %q", settings.ChatVoice)
	}
	if !oneOf(settings.UserAnatomy, LLMUserAnatomyPenis, LLMUserAnatomyVagina, LLMUserAnatomyCustom) {
		return fmt.Errorf("unknown LLM user anatomy %q", settings.UserAnatomy)
	}
	if len([]rune(settings.CustomAnatomy)) > MaxLLMCustomAnatomyChars {
		return fmt.Errorf("custom anatomy wording must be at most %d characters", MaxLLMCustomAnatomyChars)
	}
	if len([]rune(settings.PersonaDescription)) > MaxLLMPersonaDescriptionChars {
		return fmt.Errorf("persona description must be at most %d characters", MaxLLMPersonaDescriptionChars)
	}
	return nil
}

func validateLLMBaseURL(label, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return fmt.Errorf("%s base URL must be an absolute HTTP URL with a host", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s base URL scheme must be http or https", label)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s base URL must not include userinfo", label)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return fmt.Errorf("%s base URL must not include a query", label)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("%s base URL must not include a fragment", label)
	}
	return nil
}

func cloneSettings(settings Settings) Settings {
	settings.Media.LibraryPaths = append([]string{}, settings.Media.LibraryPaths...)
	if settings.LLM.MotionCapabilities != nil {
		capabilities := *settings.LLM.MotionCapabilities
		settings.LLM.MotionCapabilities = &capabilities
	}
	settings.Voice.TTSWorkerArgs = cloneStrings(settings.Voice.TTSWorkerArgs)
	settings.Voice.ASRWorkerArgs = cloneStrings(settings.Voice.ASRWorkerArgs)
	return settings
}

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func normalizeLLMStrings(settings LLMSettings) LLMSettings {
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.LlamaCPPMode = strings.TrimSpace(settings.LlamaCPPMode)
	settings.LlamaCPPBaseURL = strings.TrimRight(strings.TrimSpace(settings.LlamaCPPBaseURL), "/")
	settings.OllamaBaseURL = strings.TrimRight(strings.TrimSpace(settings.OllamaBaseURL), "/")
	settings.OllamaModelsPath = strings.TrimSpace(settings.OllamaModelsPath)
	settings.Model = strings.TrimSpace(settings.Model)
	settings.PromptSet = strings.TrimSpace(settings.PromptSet)
	settings.ReasoningMode = strings.TrimSpace(settings.ReasoningMode)
	settings.ChatVoice = strings.ToLower(strings.TrimSpace(settings.ChatVoice))
	settings.UserAnatomy = strings.ToLower(strings.TrimSpace(settings.UserAnatomy))
	settings.CustomAnatomy = strings.Join(strings.Fields(settings.CustomAnatomy), " ")
	settings.PersonaDescription = strings.Join(strings.Fields(settings.PersonaDescription), " ")
	return settings
}

// LLMChatVoices lists the reply registers in escalation order. Exported so the
// persona store validates against the same list the settings form offers,
// rather than keeping a second copy that can drift from this one.
func LLMChatVoices() []string {
	return []string{LLMChatVoiceUtility, LLMChatVoiceWarm, LLMChatVoiceIntimate, LLMChatVoiceExplicit}
}

// LLMReactionStyles lists the reaction styles, neutral first because it is the
// default and the only one that composes nothing.
func LLMReactionStyles() []string {
	return []string{
		LLMReactionStyleNeutral, LLMReactionStylePlayful, LLMReactionStyleTender,
		LLMReactionStyleDominant, LLMReactionStyleSubmissive, LLMReactionStyleTeasing,
	}
}

// LlamaCPPContextSizes lists the reviewed managed-runner context allocations.
// Callers receive a new slice so the settings catalog cannot be mutated.
func LlamaCPPContextSizes() []int {
	return []int{16384, 32768, 65536, 131072}
}

// ValidLLMChatVoice reports whether a reply register is one this build composes.
func ValidLLMChatVoice(voice string) bool {
	return oneOf(voice, LLMChatVoices()...)
}

// ValidLLMReactionStyle reports whether a reaction style is one this build
// composes.
func ValidLLMReactionStyle(style string) bool {
	return oneOf(style, LLMReactionStyles()...)
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func oneOfInt(value int, allowed ...int) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
