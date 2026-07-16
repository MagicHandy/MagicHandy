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
	CurrentSettingsVersion = 1

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
	// VoiceTTSProviderNeuTTSAir selects the bundled NeuTTS adapter.
	VoiceTTSProviderNeuTTSAir = "neutts_air"
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
	// DefaultNeuTTSBackbone is the reviewed Q4 local runner model.
	DefaultNeuTTSBackbone = "neuphonic/neutts-air-q4-gguf"
	// NeuTTSSamplingFixed reuses one seed for repeatable speech and PCM caching.
	NeuTTSSamplingFixed = "fixed"
	// NeuTTSSamplingRandom asks the runner to choose a new seed per request.
	NeuTTSSamplingRandom = "random"
	// DefaultNeuTTSSamplerSeed passed the mixed-sentence intelligibility corpus.
	DefaultNeuTTSSamplerSeed uint32 = 3
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
	Device      DeviceSettings      `json:"device"`
	Motion      MotionSettings      `json:"motion"`
	LLM         LLMSettings         `json:"llm"`
	Voice       VoiceSettings       `json:"voice"`
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
	SpeedMinPercent  int    `json:"speed_min_percent"`
	SpeedMaxPercent  int    `json:"speed_max_percent"`
	StrokeMinPercent int    `json:"stroke_min_percent"`
	StrokeMaxPercent int    `json:"stroke_max_percent"`
	ReverseDirection bool   `json:"reverse_direction"`
	Style            string `json:"style"`
}

// LLMSettings contains local model provider settings.
type LLMSettings struct {
	Provider             string `json:"provider"`
	LlamaCPPMode         string `json:"llama_cpp_mode"`
	LlamaCPPBaseURL      string `json:"llama_cpp_base_url"`
	OllamaBaseURL        string `json:"ollama_base_url"`
	OllamaModelsPath     string `json:"ollama_models_path,omitempty"`
	Model                string `json:"model"`
	PromptSet            string `json:"prompt_set"`
	RequestTimeoutMillis int    `json:"request_timeout_ms"`
	MaxOutputTokens      int    `json:"max_output_tokens"`
	ReasoningMode        string `json:"reasoning_mode"`
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

	NeuTTSRunnerPath     string `json:"neutts_runner_path,omitempty"`
	NeuTTSReferenceWAV   string `json:"neutts_reference_wav,omitempty"`
	NeuTTSReferenceCodes string `json:"neutts_reference_codes,omitempty"`
	NeuTTSReferenceText  string `json:"neutts_reference_text,omitempty"`
	NeuTTSBackbone       string `json:"neutts_backbone,omitempty"`
	NeuTTSSamplingMode   string `json:"neutts_sampling_mode"`
	NeuTTSSamplerSeed    uint32 `json:"neutts_sampler_seed"`
	// SpeakReplies enqueues each displayed chat reply to the running TTS
	// worker in lockstep (ADR 0003: a spoken reply is always also shown).
	SpeakReplies bool `json:"speak_replies"`
	// ElevenLabsAPIKey is a private credential like the Handy connection
	// key: stored at rest, handed to the TTS worker process only via a
	// private environment variable, never returned by any read API.
	ElevenLabsAPIKey string `json:"elevenlabs_api_key,omitempty"`
}

// PublicVoiceSettings is the API-safe voice view; the ElevenLabs key is
// reduced to a set/unset flag.
type PublicVoiceSettings struct {
	Enabled              bool     `json:"enabled"`
	TTSProvider          string   `json:"tts_provider"`
	ASRProvider          string   `json:"asr_provider"`
	TTSWorkerPath        string   `json:"tts_worker_path,omitempty"`
	TTSWorkerArgs        []string `json:"tts_worker_args,omitempty"`
	ASRWorkerPath        string   `json:"asr_worker_path,omitempty"`
	ASRWorkerArgs        []string `json:"asr_worker_args,omitempty"`
	SpeakReplies         bool     `json:"speak_replies"`
	ElevenLabsVoiceID    string   `json:"elevenlabs_voice_id,omitempty"`
	ElevenLabsModelID    string   `json:"elevenlabs_model_id,omitempty"`
	ParakeetServerPath   string   `json:"parakeet_server_path,omitempty"`
	ParakeetModelPath    string   `json:"parakeet_model_path,omitempty"`
	ParakeetServerPort   int      `json:"parakeet_port,omitempty"`
	ParakeetSource       string   `json:"parakeet_source"`
	ASRBaseURL           string   `json:"asr_base_url,omitempty"`
	ASRModel             string   `json:"asr_model,omitempty"`
	InputMode            string   `json:"input_mode"`
	InputSensitivity     int      `json:"input_sensitivity"`
	InputSilenceMillis   int      `json:"input_silence_ms"`
	InputNoiseSuppress   bool     `json:"input_noise_suppression"`
	NeuTTSRunnerPath     string   `json:"neutts_runner_path,omitempty"`
	NeuTTSReferenceWAV   string   `json:"neutts_reference_wav,omitempty"`
	NeuTTSReferenceCodes string   `json:"neutts_reference_codes,omitempty"`
	NeuTTSReferenceText  string   `json:"neutts_reference_text,omitempty"`
	NeuTTSBackbone       string   `json:"neutts_backbone,omitempty"`
	NeuTTSSamplingMode   string   `json:"neutts_sampling_mode"`
	NeuTTSSamplerSeed    uint32   `json:"neutts_sampler_seed"`
	ElevenLabsKeySet     bool     `json:"elevenlabs_key_set"`
}

// VoiceUpdate is the API write payload for voice settings. A nil API key
// keeps the stored secret; ClearElevenLabsKey removes it.
type VoiceUpdate struct {
	Enabled              bool     `json:"enabled"`
	TTSProvider          string   `json:"tts_provider"`
	ASRProvider          string   `json:"asr_provider"`
	TTSWorkerPath        string   `json:"tts_worker_path"`
	TTSWorkerArgs        []string `json:"tts_worker_args"`
	ASRWorkerPath        string   `json:"asr_worker_path"`
	ASRWorkerArgs        []string `json:"asr_worker_args"`
	SpeakReplies         bool     `json:"speak_replies"`
	ElevenLabsVoiceID    string   `json:"elevenlabs_voice_id"`
	ElevenLabsModelID    string   `json:"elevenlabs_model_id"`
	ParakeetServerPath   string   `json:"parakeet_server_path"`
	ParakeetModelPath    string   `json:"parakeet_model_path"`
	ParakeetServerPort   int      `json:"parakeet_port"`
	ParakeetSource       *string  `json:"parakeet_source,omitempty"`
	ASRBaseURL           string   `json:"asr_base_url"`
	ASRModel             string   `json:"asr_model"`
	InputMode            *string  `json:"input_mode,omitempty"`
	InputSensitivity     *int     `json:"input_sensitivity,omitempty"`
	InputSilenceMillis   *int     `json:"input_silence_ms,omitempty"`
	InputNoiseSuppress   *bool    `json:"input_noise_suppression,omitempty"`
	NeuTTSRunnerPath     string   `json:"neutts_runner_path"`
	NeuTTSReferenceWAV   string   `json:"neutts_reference_wav"`
	NeuTTSReferenceCodes string   `json:"neutts_reference_codes"`
	NeuTTSReferenceText  string   `json:"neutts_reference_text"`
	NeuTTSBackbone       string   `json:"neutts_backbone"`
	NeuTTSSamplingMode   *string  `json:"neutts_sampling_mode,omitempty"`
	NeuTTSSamplerSeed    *uint32  `json:"neutts_sampler_seed,omitempty"`
	ElevenLabsAPIKey     *string  `json:"elevenlabs_api_key,omitempty"`
	ClearElevenLabsKey   bool     `json:"clear_elevenlabs_key"`
}

// DiagnosticsSettings contains logging and diagnostics verbosity settings.
type DiagnosticsSettings struct {
	Verbosity string `json:"verbosity"`
}

// PublicSettings is the API-safe settings view. It intentionally omits secrets.
type PublicSettings struct {
	Version     int                       `json:"version"`
	Server      ServerSettings            `json:"server"`
	Device      PublicDeviceSettings      `json:"device"`
	Motion      MotionSettings            `json:"motion"`
	LLM         LLMSettings               `json:"llm"`
	Voice       PublicVoiceSettings       `json:"voice"`
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
	LLMProviders            []string `json:"llm_providers"`
	LlamaCPPModes           []string `json:"llama_cpp_modes"`
	LLMReasoningModes       []string `json:"llm_reasoning_modes"`
	LLMMaxOutputTokens      []int    `json:"llm_max_output_tokens"`
	PromptSets              []string `json:"prompt_sets"`
	TTSProviders            []string `json:"tts_providers"`
	ASRProviders            []string `json:"asr_providers"`
	ParakeetSources         []string `json:"parakeet_sources"`
	NeuTTSSamplingModes     []string `json:"neutts_sampling_modes"`
}

// LLMUpdate is the settings API write shape. New tuning fields are pointers so
// older clients that omit them preserve the current persisted values.
type LLMUpdate struct {
	Provider             string  `json:"provider"`
	LlamaCPPMode         string  `json:"llama_cpp_mode"`
	LlamaCPPBaseURL      string  `json:"llama_cpp_base_url"`
	OllamaBaseURL        string  `json:"ollama_base_url"`
	OllamaModelsPath     string  `json:"ollama_models_path,omitempty"`
	Model                string  `json:"model"`
	PromptSet            string  `json:"prompt_set"`
	RequestTimeoutMillis int     `json:"request_timeout_ms"`
	MaxOutputTokens      *int    `json:"max_output_tokens,omitempty"`
	ReasoningMode        *string `json:"reasoning_mode,omitempty"`
}

// LLMUpdateFromSettings creates a complete write payload from a settings view.
func LLMUpdateFromSettings(settings LLMSettings) LLMUpdate {
	return LLMUpdate{
		Provider:             settings.Provider,
		LlamaCPPMode:         settings.LlamaCPPMode,
		LlamaCPPBaseURL:      settings.LlamaCPPBaseURL,
		OllamaBaseURL:        settings.OllamaBaseURL,
		OllamaModelsPath:     settings.OllamaModelsPath,
		Model:                settings.Model,
		PromptSet:            settings.PromptSet,
		RequestTimeoutMillis: settings.RequestTimeoutMillis,
		MaxOutputTokens:      &settings.MaxOutputTokens,
		ReasoningMode:        &settings.ReasoningMode,
	}
}

// SettingsUpdate is the payload accepted by the settings API.
type SettingsUpdate struct {
	Server             ServerSettings      `json:"server"`
	Device             DeviceUpdate        `json:"device"`
	Motion             MotionSettings      `json:"motion"`
	LLM                LLMUpdate           `json:"llm"`
	Voice              VoiceUpdate         `json:"voice"`
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
		LLM: LLMSettings{
			Provider:             LLMProviderLlamaCPP,
			LlamaCPPMode:         LlamaCPPModeManaged,
			LlamaCPPBaseURL:      DefaultLlamaCPPBaseURL,
			OllamaBaseURL:        DefaultOllamaBaseURL,
			Model:                DefaultLLMModel,
			PromptSet:            PromptSetMagicHandyMotionV1,
			RequestTimeoutMillis: DefaultLLMRequestTimeoutMillis,
			MaxOutputTokens:      DefaultLLMMaxOutputTokens,
			ReasoningMode:        LLMReasoningOff,
		},
		Voice: VoiceSettings{
			TTSProvider:        VoiceProviderNone,
			ASRProvider:        VoiceProviderNone,
			ElevenLabsVoiceID:  DefaultElevenLabsVoiceID,
			ElevenLabsModelID:  DefaultElevenLabsModelID,
			ParakeetServerPort: DefaultParakeetServerPort,
			ParakeetSource:     ParakeetSourceApp,
			InputMode:          VoiceInputModeHandsFree,
			InputSensitivity:   DefaultVoiceInputSensitivity,
			InputSilenceMillis: DefaultVoiceInputSilenceMillis,
			InputNoiseSuppress: true,
			NeuTTSBackbone:     DefaultNeuTTSBackbone,
			NeuTTSSamplingMode: NeuTTSSamplingFixed,
			NeuTTSSamplerSeed:  DefaultNeuTTSSamplerSeed,
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
		Device: PublicDeviceSettings{
			HSPDispatchOwner:         s.Device.HSPDispatchOwner,
			IntifaceServerAddress:    s.Device.IntifaceServerAddress,
			FirmwareAPIRequirement:   s.Device.FirmwareAPIRequirement,
			APIApplicationIDSource:   s.Device.APIApplicationIDSource,
			APIApplicationIDOverride: s.Device.APIApplicationIDOverride,
			ConnectionKeySet:         s.Device.HandyConnectionKey != "",
		},
		Motion:      s.Motion,
		LLM:         s.LLM,
		Voice:       publicVoiceSettings(s.Voice),
		Diagnostics: s.Diagnostics,
		Options: PublicSettingsOptionHints{
			HSPDispatchOwners: []string{
				DispatchOwnerCloudREST,
				DispatchOwnerBrowserBluetooth,
				DispatchOwnerIntiface,
			},
			APIApplicationIDSources: []string{
				ApplicationIDSourceBundled,
				ApplicationIDSourceDeveloperOverride,
			},
			MotionStyles: []string{
				MotionStyleGentle,
				MotionStyleBalanced,
				MotionStyleIntense,
			},
			DiagnosticsVerbosities: []string{
				DiagnosticsVerbosityNormal,
				DiagnosticsVerbosityDebug,
				DiagnosticsVerbosityTrace,
			},
			LLMProviders: []string{
				LLMProviderLlamaCPP,
				LLMProviderOllama,
			},
			LlamaCPPModes: []string{
				LlamaCPPModeManaged,
				LlamaCPPModeExternal,
			},
			LLMReasoningModes: []string{
				LLMReasoningOff,
				LLMReasoningAuto,
			},
			LLMMaxOutputTokens: []int{128, 256, 512, 1024},
			PromptSets: []string{
				PromptSetMagicHandyMotionV1,
				PromptSetMagicHandyMotionV1ES,
				PromptSetMagicHandyMotionV1PTBR,
				PromptSetMagicHandyMotionV1ZHHans,
				PromptSetMagicHandyMotionV1JA,
			},
			TTSProviders: []string{
				VoiceProviderNone,
				VoiceTTSProviderElevenLabs,
				VoiceTTSProviderNeuTTSAir,
				VoiceProviderCustom,
			},
			ASRProviders: []string{
				VoiceProviderNone,
				VoiceASRProviderParakeet,
				VoiceASRProviderOpenAICompat,
				VoiceProviderCustom,
			},
			ParakeetSources: []string{
				ParakeetSourceApp,
				ParakeetSourceCustom,
			},
			NeuTTSSamplingModes: []string{
				NeuTTSSamplingFixed,
				NeuTTSSamplingRandom,
			},
		},
	}
}

func publicVoiceSettings(settings VoiceSettings) PublicVoiceSettings {
	return PublicVoiceSettings{
		Enabled:              settings.Enabled,
		TTSProvider:          settings.TTSProvider,
		ASRProvider:          settings.ASRProvider,
		TTSWorkerPath:        settings.TTSWorkerPath,
		TTSWorkerArgs:        cloneStrings(settings.TTSWorkerArgs),
		ASRWorkerPath:        settings.ASRWorkerPath,
		ASRWorkerArgs:        cloneStrings(settings.ASRWorkerArgs),
		SpeakReplies:         settings.SpeakReplies,
		ElevenLabsVoiceID:    settings.ElevenLabsVoiceID,
		ElevenLabsModelID:    settings.ElevenLabsModelID,
		ParakeetServerPath:   settings.ParakeetServerPath,
		ParakeetModelPath:    settings.ParakeetModelPath,
		ParakeetServerPort:   settings.ParakeetServerPort,
		ParakeetSource:       settings.ParakeetSource,
		ASRBaseURL:           settings.ASRBaseURL,
		ASRModel:             settings.ASRModel,
		InputMode:            settings.InputMode,
		InputSensitivity:     settings.InputSensitivity,
		InputSilenceMillis:   settings.InputSilenceMillis,
		InputNoiseSuppress:   settings.InputNoiseSuppress,
		NeuTTSRunnerPath:     settings.NeuTTSRunnerPath,
		NeuTTSReferenceWAV:   settings.NeuTTSReferenceWAV,
		NeuTTSReferenceCodes: settings.NeuTTSReferenceCodes,
		NeuTTSReferenceText:  settings.NeuTTSReferenceText,
		NeuTTSBackbone:       settings.NeuTTSBackbone,
		NeuTTSSamplingMode:   settings.NeuTTSSamplingMode,
		NeuTTSSamplerSeed:    settings.NeuTTSSamplerSeed,
		ElevenLabsKeySet:     settings.ElevenLabsAPIKey != "",
	}
}

// ApplyUpdate merges a settings API payload into the current settings.
func (s Settings) ApplyUpdate(update SettingsUpdate) (Settings, error) {
	next := s
	next.Version = CurrentSettingsVersion
	next.Server = update.Server
	next.Device.HSPDispatchOwner = update.Device.HSPDispatchOwner
	next.Device.IntifaceServerAddress = strings.TrimSpace(update.Device.IntifaceServerAddress)
	next.Device.FirmwareAPIRequirement = update.Device.FirmwareAPIRequirement
	next.Device.APIApplicationIDSource = update.Device.APIApplicationIDSource
	next.Device.APIApplicationIDOverride = strings.TrimSpace(update.Device.APIApplicationIDOverride)
	next.Motion = update.Motion
	maxOutputTokens := s.LLM.MaxOutputTokens
	if update.LLM.MaxOutputTokens != nil {
		maxOutputTokens = *update.LLM.MaxOutputTokens
	}
	reasoningMode := s.LLM.ReasoningMode
	if update.LLM.ReasoningMode != nil {
		reasoningMode = *update.LLM.ReasoningMode
	}
	next.LLM = normalizeLLMStrings(LLMSettings{
		Provider:             update.LLM.Provider,
		LlamaCPPMode:         update.LLM.LlamaCPPMode,
		LlamaCPPBaseURL:      update.LLM.LlamaCPPBaseURL,
		OllamaBaseURL:        update.LLM.OllamaBaseURL,
		OllamaModelsPath:     update.LLM.OllamaModelsPath,
		Model:                update.LLM.Model,
		PromptSet:            update.LLM.PromptSet,
		RequestTimeoutMillis: update.LLM.RequestTimeoutMillis,
		MaxOutputTokens:      maxOutputTokens,
		ReasoningMode:        reasoningMode,
	})
	next.Voice = applyVoiceUpdate(s.Voice, update.Voice)
	next.Diagnostics = update.Diagnostics

	if update.Voice.ClearElevenLabsKey {
		next.Voice.ElevenLabsAPIKey = ""
	} else if update.Voice.ElevenLabsAPIKey != nil {
		next.Voice.ElevenLabsAPIKey = strings.TrimSpace(*update.Voice.ElevenLabsAPIKey)
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
	neuTTSSamplingMode := current.NeuTTSSamplingMode
	if update.NeuTTSSamplingMode != nil {
		neuTTSSamplingMode = *update.NeuTTSSamplingMode
	}
	neuTTSSamplerSeed := current.NeuTTSSamplerSeed
	if update.NeuTTSSamplerSeed != nil {
		neuTTSSamplerSeed = *update.NeuTTSSamplerSeed
	}
	return normalizeVoiceStrings(VoiceSettings{
		Enabled:              update.Enabled,
		TTSProvider:          update.TTSProvider,
		ASRProvider:          update.ASRProvider,
		TTSWorkerPath:        update.TTSWorkerPath,
		TTSWorkerArgs:        update.TTSWorkerArgs,
		ASRWorkerPath:        update.ASRWorkerPath,
		ASRWorkerArgs:        update.ASRWorkerArgs,
		SpeakReplies:         update.SpeakReplies,
		ElevenLabsVoiceID:    update.ElevenLabsVoiceID,
		ElevenLabsModelID:    update.ElevenLabsModelID,
		ParakeetServerPath:   update.ParakeetServerPath,
		ParakeetModelPath:    update.ParakeetModelPath,
		ParakeetServerPort:   update.ParakeetServerPort,
		ParakeetSource:       parakeetSource,
		ASRBaseURL:           update.ASRBaseURL,
		ASRModel:             update.ASRModel,
		InputMode:            inputMode,
		InputSensitivity:     inputSensitivity,
		InputSilenceMillis:   inputSilenceMillis,
		InputNoiseSuppress:   inputNoiseSuppress,
		NeuTTSRunnerPath:     update.NeuTTSRunnerPath,
		NeuTTSReferenceWAV:   update.NeuTTSReferenceWAV,
		NeuTTSReferenceCodes: update.NeuTTSReferenceCodes,
		NeuTTSReferenceText:  update.NeuTTSReferenceText,
		NeuTTSBackbone:       update.NeuTTSBackbone,
		NeuTTSSamplingMode:   neuTTSSamplingMode,
		NeuTTSSamplerSeed:    neuTTSSamplerSeed,
		// The stored key survives unless explicitly replaced or cleared.
		ElevenLabsAPIKey: current.ElevenLabsAPIKey,
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

	settings.Version = CurrentSettingsVersion
	normalized, err := NormalizeSettings(settings)
	return normalized, true, err
}

func loadSettingsFromBytes(data []byte) (Settings, bool, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var header struct {
		Version int                        `json:"version"`
		Voice   map[string]json.RawMessage `json:"voice"`
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

	return MigrateSettings(settings, header.Version)
}

func validateSettings(settings Settings) error {
	if settings.Server.Port < 1 || settings.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
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
	if err := validateMotionSettings(settings.Motion); err != nil {
		return err
	}
	if err := validateLLMSettings(settings.LLM); err != nil {
		return err
	}
	return validateVoiceSettings(settings.Voice)
}

func applyMissingDefaults(settings Settings) Settings {
	defaults := DefaultSettings()
	if settings.Server.Port == 0 {
		settings.Server.Port = defaults.Server.Port
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
	if settings.LLM.Provider == "" {
		settings.LLM.Provider = defaults.LLM.Provider
	}
	if settings.LLM.LlamaCPPMode == "" {
		settings.LLM.LlamaCPPMode = defaults.LLM.LlamaCPPMode
	}
	if settings.LLM.LlamaCPPBaseURL == "" {
		settings.LLM.LlamaCPPBaseURL = defaults.LLM.LlamaCPPBaseURL
	}
	if settings.LLM.OllamaBaseURL == "" {
		settings.LLM.OllamaBaseURL = defaults.LLM.OllamaBaseURL
	}
	if settings.LLM.Model == "" {
		settings.LLM.Model = defaults.LLM.Model
	}
	if settings.LLM.PromptSet == "" {
		settings.LLM.PromptSet = defaults.LLM.PromptSet
	}
	if settings.LLM.RequestTimeoutMillis == 0 {
		settings.LLM.RequestTimeoutMillis = defaults.LLM.RequestTimeoutMillis
	}
	if settings.LLM.MaxOutputTokens == 0 {
		settings.LLM.MaxOutputTokens = defaults.LLM.MaxOutputTokens
	}
	if settings.LLM.ReasoningMode == "" {
		settings.LLM.ReasoningMode = defaults.LLM.ReasoningMode
	}
	settings.Voice = applyMissingVoiceDefaults(settings.Voice, defaults.Voice)
	settings.LLM = normalizeLLMStrings(settings.LLM)
	settings.Voice = normalizeVoiceStrings(settings.Voice)
	if settings.Diagnostics.Verbosity == "" {
		settings.Diagnostics.Verbosity = defaults.Diagnostics.Verbosity
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

func applyMissingVoiceDefaults(settings, defaults VoiceSettings) VoiceSettings {
	// Settings version 1 originally had only raw worker paths and arguments.
	// Preserve those commands exactly by classifying them as custom providers.
	if settings.TTSProvider == "" {
		settings.TTSProvider = defaults.TTSProvider
		if settings.TTSWorkerPath != "" || len(settings.TTSWorkerArgs) > 0 {
			settings.TTSProvider = VoiceProviderCustom
		}
	}
	if settings.ASRProvider == "" {
		settings.ASRProvider = defaults.ASRProvider
		if settings.ASRWorkerPath != "" || len(settings.ASRWorkerArgs) > 0 {
			settings.ASRProvider = VoiceProviderCustom
		}
	}
	if settings.ElevenLabsVoiceID == "" {
		settings.ElevenLabsVoiceID = defaults.ElevenLabsVoiceID
	}
	if settings.ElevenLabsModelID == "" {
		settings.ElevenLabsModelID = defaults.ElevenLabsModelID
	}
	if settings.ParakeetServerPort == 0 {
		settings.ParakeetServerPort = defaults.ParakeetServerPort
	}
	if settings.ParakeetSource == "" {
		settings.ParakeetSource = defaults.ParakeetSource
		if settings.ParakeetServerPath != "" || settings.ParakeetModelPath != "" {
			settings.ParakeetSource = ParakeetSourceCustom
		}
	}
	if settings.NeuTTSBackbone == "" {
		settings.NeuTTSBackbone = defaults.NeuTTSBackbone
	}
	if settings.NeuTTSSamplingMode == "" {
		settings.NeuTTSSamplingMode = defaults.NeuTTSSamplingMode
		settings.NeuTTSSamplerSeed = defaults.NeuTTSSamplerSeed
	}
	if settings.InputMode == "" {
		settings.InputMode = defaults.InputMode
	}
	if settings.InputSensitivity == 0 {
		settings.InputSensitivity = defaults.InputSensitivity
	}
	if settings.InputSilenceMillis == 0 {
		settings.InputSilenceMillis = defaults.InputSilenceMillis
	}
	return settings
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

func normalizeVoiceStrings(settings VoiceSettings) VoiceSettings {
	settings.TTSProvider = strings.TrimSpace(settings.TTSProvider)
	settings.ASRProvider = strings.TrimSpace(settings.ASRProvider)
	settings.TTSWorkerPath = strings.TrimSpace(settings.TTSWorkerPath)
	settings.ASRWorkerPath = strings.TrimSpace(settings.ASRWorkerPath)
	settings.TTSWorkerArgs = trimArgs(settings.TTSWorkerArgs)
	settings.ASRWorkerArgs = trimArgs(settings.ASRWorkerArgs)
	settings.ElevenLabsVoiceID = strings.TrimSpace(settings.ElevenLabsVoiceID)
	settings.ElevenLabsModelID = strings.TrimSpace(settings.ElevenLabsModelID)
	settings.ParakeetServerPath = strings.TrimSpace(settings.ParakeetServerPath)
	settings.ParakeetModelPath = strings.TrimSpace(settings.ParakeetModelPath)
	settings.ParakeetSource = strings.TrimSpace(settings.ParakeetSource)
	settings.ASRBaseURL = strings.TrimRight(strings.TrimSpace(settings.ASRBaseURL), "/")
	settings.ASRModel = strings.TrimSpace(settings.ASRModel)
	settings.InputMode = strings.TrimSpace(settings.InputMode)
	settings.NeuTTSRunnerPath = strings.TrimSpace(settings.NeuTTSRunnerPath)
	settings.NeuTTSReferenceWAV = strings.TrimSpace(settings.NeuTTSReferenceWAV)
	settings.NeuTTSReferenceCodes = strings.TrimSpace(settings.NeuTTSReferenceCodes)
	settings.NeuTTSReferenceText = strings.TrimSpace(settings.NeuTTSReferenceText)
	settings.NeuTTSBackbone = strings.TrimSpace(settings.NeuTTSBackbone)
	settings.NeuTTSSamplingMode = strings.TrimSpace(settings.NeuTTSSamplingMode)
	return settings
}

func validateVoiceSettings(settings VoiceSettings) error {
	if !oneOf(settings.TTSProvider, VoiceProviderNone, VoiceTTSProviderElevenLabs, VoiceTTSProviderNeuTTSAir, VoiceProviderCustom) {
		return fmt.Errorf("unknown TTS provider %q", settings.TTSProvider)
	}
	if !oneOf(settings.ASRProvider, VoiceProviderNone, VoiceASRProviderParakeet, VoiceASRProviderOpenAICompat, VoiceProviderCustom) {
		return fmt.Errorf("unknown ASR provider %q", settings.ASRProvider)
	}
	if settings.ParakeetServerPort < 1 || settings.ParakeetServerPort > 65535 {
		return errors.New("parakeet server port must be between 1 and 65535")
	}
	if !oneOf(settings.ParakeetSource, ParakeetSourceApp, ParakeetSourceCustom) {
		return fmt.Errorf("unknown Parakeet source %q", settings.ParakeetSource)
	}
	if !oneOf(settings.InputMode, VoiceInputModeHandsFree, VoiceInputModeHold) {
		return fmt.Errorf("unknown voice input mode %q", settings.InputMode)
	}
	if settings.InputSensitivity < 1 || settings.InputSensitivity > 100 {
		return errors.New("voice input sensitivity must be between 1 and 100")
	}
	if settings.InputSilenceMillis < 300 || settings.InputSilenceMillis > 3000 {
		return errors.New("voice input silence delay must be between 300 and 3000 milliseconds")
	}
	if !oneOf(settings.NeuTTSSamplingMode, NeuTTSSamplingFixed, NeuTTSSamplingRandom) {
		return fmt.Errorf("unknown NeuTTS sampling mode %q", settings.NeuTTSSamplingMode)
	}
	return nil
}

func trimArgs(args []string) []string {
	trimmed := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			trimmed = append(trimmed, arg)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}
	return trimmed
}

func cloneSettings(settings Settings) Settings {
	settings.Voice.TTSWorkerArgs = cloneStrings(settings.Voice.TTSWorkerArgs)
	settings.Voice.ASRWorkerArgs = cloneStrings(settings.Voice.ASRWorkerArgs)
	return settings
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
	return settings
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
