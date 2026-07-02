package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// CurrentSettingsVersion is the latest on-disk settings schema version.
	CurrentSettingsVersion = 1

	// DefaultServerPort is the local HTTP port used by fresh settings.
	DefaultServerPort = 49717
)

const (
	// DispatchOwnerCloudREST is the initial HSP dispatch owner placeholder.
	DispatchOwnerCloudREST = "cloud_rest"
	// DispatchOwnerBrowserBluetooth is the future browser-owned BLE dispatch owner.
	DispatchOwnerBrowserBluetooth = "browser_bluetooth"

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

	// DefaultLlamaCPPBaseURL is the default llama-server OpenAI-compatible URL.
	DefaultLlamaCPPBaseURL = "http://127.0.0.1:8080"
	// DefaultOllamaBaseURL is the default local Ollama daemon URL.
	DefaultOllamaBaseURL = "http://127.0.0.1:11434"
	// DefaultLLMModel is a placeholder model name users replace with a local model.
	DefaultLLMModel = "local-model"
	// DefaultLLMRequestTimeoutMillis caps one chat or repair pass.
	DefaultLLMRequestTimeoutMillis = 120000
)

// Settings is the versioned on-disk application settings schema.
type Settings struct {
	Version     int                 `json:"version"`
	Server      ServerSettings      `json:"server"`
	Device      DeviceSettings      `json:"device"`
	Motion      MotionSettings      `json:"motion"`
	LLM         LLMSettings         `json:"llm"`
	Diagnostics DiagnosticsSettings `json:"diagnostics"`
}

// ServerSettings contains local HTTP server settings.
type ServerSettings struct {
	Port int `json:"port"`
}

// DeviceSettings contains Handy transport prerequisites and placeholders.
type DeviceSettings struct {
	HSPDispatchOwner         string `json:"hsp_dispatch_owner"`
	FirmwareAPIRequirement   string `json:"firmware_api_requirement"`
	APIApplicationIDSource   string `json:"api_application_id_source"`
	APIApplicationIDOverride string `json:"api_application_id_override,omitempty"`
	HandyConnectionKey       string `json:"handy_connection_key,omitempty"`
}

// MotionSettings contains transport-neutral motion control defaults.
type MotionSettings struct {
	SpeedMinPercent  int  `json:"speed_min_percent"`
	SpeedMaxPercent  int  `json:"speed_max_percent"`
	StrokeMinPercent int  `json:"stroke_min_percent"`
	StrokeMaxPercent int  `json:"stroke_max_percent"`
	ReverseDirection bool `json:"reverse_direction"`
}

// LLMSettings contains local model provider settings.
type LLMSettings struct {
	Provider             string `json:"provider"`
	LlamaCPPMode         string `json:"llama_cpp_mode"`
	LlamaCPPBaseURL      string `json:"llama_cpp_base_url"`
	LlamaCPPRunnerPath   string `json:"llama_cpp_runner_path,omitempty"`
	LlamaCPPModelPath    string `json:"llama_cpp_model_path,omitempty"`
	OllamaBaseURL        string `json:"ollama_base_url"`
	Model                string `json:"model"`
	PromptSet            string `json:"prompt_set"`
	RequestTimeoutMillis int    `json:"request_timeout_ms"`
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
	Diagnostics DiagnosticsSettings       `json:"diagnostics"`
	Options     PublicSettingsOptionHints `json:"options"`
}

// PublicDeviceSettings is the API-safe device settings view.
type PublicDeviceSettings struct {
	HSPDispatchOwner         string `json:"hsp_dispatch_owner"`
	FirmwareAPIRequirement   string `json:"firmware_api_requirement"`
	APIApplicationIDSource   string `json:"api_application_id_source"`
	APIApplicationIDOverride string `json:"api_application_id_override,omitempty"`
	ConnectionKeySet         bool   `json:"connection_key_set"`
}

// PublicSettingsOptionHints exposes valid option values to the static UI.
type PublicSettingsOptionHints struct {
	HSPDispatchOwners       []string `json:"hsp_dispatch_owners"`
	APIApplicationIDSources []string `json:"api_application_id_sources"`
	DiagnosticsVerbosities  []string `json:"diagnostics_verbosities"`
	LLMProviders            []string `json:"llm_providers"`
	LlamaCPPModes           []string `json:"llama_cpp_modes"`
	PromptSets              []string `json:"prompt_sets"`
}

// SettingsUpdate is the payload accepted by the settings API.
type SettingsUpdate struct {
	Server             ServerSettings      `json:"server"`
	Device             DeviceUpdate        `json:"device"`
	Motion             MotionSettings      `json:"motion"`
	LLM                LLMSettings         `json:"llm"`
	Diagnostics        DiagnosticsSettings `json:"diagnostics"`
	ClearConnectionKey bool                `json:"clear_connection_key"`
}

// DeviceUpdate is the API write payload for device settings.
type DeviceUpdate struct {
	HSPDispatchOwner         string  `json:"hsp_dispatch_owner"`
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
			FirmwareAPIRequirement: FirmwareAPIRequirementRequired,
			APIApplicationIDSource: ApplicationIDSourceBundled,
		},
		Motion: MotionSettings{
			SpeedMinPercent:  20,
			SpeedMaxPercent:  80,
			StrokeMinPercent: 0,
			StrokeMaxPercent: 100,
		},
		LLM: LLMSettings{
			Provider:             LLMProviderLlamaCPP,
			LlamaCPPMode:         LlamaCPPModeManaged,
			LlamaCPPBaseURL:      DefaultLlamaCPPBaseURL,
			OllamaBaseURL:        DefaultOllamaBaseURL,
			Model:                DefaultLLMModel,
			PromptSet:            PromptSetMagicHandyMotionV1,
			RequestTimeoutMillis: DefaultLLMRequestTimeoutMillis,
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
			FirmwareAPIRequirement:   s.Device.FirmwareAPIRequirement,
			APIApplicationIDSource:   s.Device.APIApplicationIDSource,
			APIApplicationIDOverride: s.Device.APIApplicationIDOverride,
			ConnectionKeySet:         s.Device.HandyConnectionKey != "",
		},
		Motion:      s.Motion,
		LLM:         s.LLM,
		Diagnostics: s.Diagnostics,
		Options: PublicSettingsOptionHints{
			HSPDispatchOwners: []string{
				DispatchOwnerCloudREST,
				DispatchOwnerBrowserBluetooth,
			},
			APIApplicationIDSources: []string{
				ApplicationIDSourceBundled,
				ApplicationIDSourceDeveloperOverride,
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
			PromptSets: []string{
				PromptSetMagicHandyMotionV1,
			},
		},
	}
}

// ApplyUpdate merges a settings API payload into the current settings.
func (s Settings) ApplyUpdate(update SettingsUpdate) (Settings, error) {
	next := s
	next.Version = CurrentSettingsVersion
	next.Server = update.Server
	next.Device.HSPDispatchOwner = update.Device.HSPDispatchOwner
	next.Device.FirmwareAPIRequirement = update.Device.FirmwareAPIRequirement
	next.Device.APIApplicationIDSource = update.Device.APIApplicationIDSource
	next.Device.APIApplicationIDOverride = strings.TrimSpace(update.Device.APIApplicationIDOverride)
	next.Motion = update.Motion
	next.LLM = normalizeLLMStrings(update.LLM)
	next.Diagnostics = update.Diagnostics

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
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return Settings{}, false, err
	}

	settings := DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, false, err
	}

	return MigrateSettings(settings, header.Version)
}

func writeSettingsFile(path string, settings Settings) error {
	settings, err := NormalizeSettings(settings)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary settings file: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary settings file: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("secure temporary settings file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary settings file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary settings file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace settings file: %w", err)
	}

	return nil
}

func validateSettings(settings Settings) error {
	if settings.Server.Port < 1 || settings.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535")
	}
	if !oneOf(settings.Device.HSPDispatchOwner, DispatchOwnerCloudREST, DispatchOwnerBrowserBluetooth) {
		return fmt.Errorf("unknown HSP dispatch owner %q", settings.Device.HSPDispatchOwner)
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
	return validateLLMSettings(settings.LLM)
}

func applyMissingDefaults(settings Settings) Settings {
	defaults := DefaultSettings()
	if settings.Server.Port == 0 {
		settings.Server.Port = defaults.Server.Port
	}
	if settings.Device.HSPDispatchOwner == "" {
		settings.Device.HSPDispatchOwner = defaults.Device.HSPDispatchOwner
	}
	if settings.Device.FirmwareAPIRequirement == "" {
		settings.Device.FirmwareAPIRequirement = defaults.Device.FirmwareAPIRequirement
	}
	if settings.Device.APIApplicationIDSource == "" {
		settings.Device.APIApplicationIDSource = defaults.Device.APIApplicationIDSource
	}
	if settings.Motion.SpeedMinPercent == 0 {
		settings.Motion.SpeedMinPercent = defaults.Motion.SpeedMinPercent
	}
	if settings.Motion.SpeedMaxPercent == 0 {
		settings.Motion.SpeedMaxPercent = defaults.Motion.SpeedMaxPercent
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
	settings.LLM = normalizeLLMStrings(settings.LLM)
	if settings.Diagnostics.Verbosity == "" {
		settings.Diagnostics.Verbosity = defaults.Diagnostics.Verbosity
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
		return errors.New("Ollama base URL is required")
	}
	if settings.Model == "" {
		return errors.New("LLM model is required")
	}
	if !oneOf(settings.PromptSet, PromptSetMagicHandyMotionV1) {
		return fmt.Errorf("unknown prompt set %q", settings.PromptSet)
	}
	if settings.RequestTimeoutMillis < 1000 || settings.RequestTimeoutMillis > 300000 {
		return errors.New("LLM request timeout must be between 1000 and 300000 milliseconds")
	}
	return nil
}

func normalizeLLMStrings(settings LLMSettings) LLMSettings {
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.LlamaCPPMode = strings.TrimSpace(settings.LlamaCPPMode)
	settings.LlamaCPPBaseURL = strings.TrimRight(strings.TrimSpace(settings.LlamaCPPBaseURL), "/")
	settings.LlamaCPPRunnerPath = strings.TrimSpace(settings.LlamaCPPRunnerPath)
	settings.LlamaCPPModelPath = strings.TrimSpace(settings.LlamaCPPModelPath)
	settings.OllamaBaseURL = strings.TrimRight(strings.TrimSpace(settings.OllamaBaseURL), "/")
	settings.Model = strings.TrimSpace(settings.Model)
	settings.PromptSet = strings.TrimSpace(settings.PromptSet)
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
