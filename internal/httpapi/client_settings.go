package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const administratorDetails = "Details are available to the administrator."

func (s *Server) clientRuntimeError(r *http.Request, err error) string {
	if s.capabilities(r).ConfigureHost {
		return err.Error()
	}
	return administratorDetails
}

// PublicSettings removes stored credentials, but still contains host paths,
// process arguments and private service addresses. Remote observers get only
// explicitly selected operational fields; new configuration fields default to
// withheld until their audience is reviewed.
func (s *Server) clientSettings(r *http.Request, settings config.PublicSettings) config.PublicSettings {
	if s.capabilities(r).ConfigureHost {
		return settings
	}
	caps := settings.LLM.Capabilities()
	return config.PublicSettings{
		Version:   settings.Version,
		UI:        settings.UI,
		Device:    config.PublicDeviceSettings{HSPDispatchOwner: settings.Device.HSPDispatchOwner},
		Motion:    settings.Motion,
		Autopilot: settings.Autopilot,
		Media:     clientMediaPreferences(settings.Media),
		LLM: config.LLMSettings{
			Provider:             settings.LLM.Provider,
			MotionGenerationMode: settings.LLM.MotionGenerationMode,
			MotionCapabilities:   &caps,
		},
		Voice: config.PublicVoiceSettings{
			Enabled:            settings.Voice.Enabled,
			TTSProvider:        settings.Voice.TTSProvider,
			ASRProvider:        settings.Voice.ASRProvider,
			SpeakReplies:       settings.Voice.SpeakReplies,
			ChatSpeechPolicy:   settings.Voice.ChatSpeechPolicy,
			InputMode:          settings.Voice.InputMode,
			InputSensitivity:   settings.Voice.InputSensitivity,
			InputSilenceMillis: settings.Voice.InputSilenceMillis,
			InputNoiseSuppress: settings.Voice.InputNoiseSuppress,
		},
		Options: config.PublicSettingsOptionHints{HandyModels: settings.Options.HandyModels, MotionStyles: settings.Options.MotionStyles, LLMMotionModes: settings.Options.LLMMotionModes},
	}
}

func clientMediaPreferences(settings config.MediaSettings) config.MediaSettings {
	return config.MediaSettings{
		ScriptOffsetMillis:     settings.ScriptOffsetMillis,
		ScriptSmoothingPercent: settings.ScriptSmoothingPercent,
		PeakRoundingMillis:     settings.PeakRoundingMillis,
	}
}

func (s *Server) clientLoadStatus(r *http.Request, status config.LoadStatus) config.LoadStatus {
	if s.capabilities(r).ConfigureHost {
		return status
	}
	return config.LoadStatus{
		UsingDefaults: status.UsingDefaults, Recovered: status.Recovered,
		Migrated: status.Migrated, Imported: status.Imported, LoadedAt: status.LoadedAt,
	}
}

func (s *Server) clientLLMState(r *http.Request) any {
	state := s.llmState(r.Context())
	if s.capabilities(r).ConfigureHost {
		return state
	}
	values, _ := state.(map[string]any)
	return map[string]any{"provider": values["provider"], "managed_runtime": values["managed_runtime"], "managed_ready": values["managed_ready"]}
}

func (s *Server) clientProviderStatus(r *http.Request, status llm.ProviderStatus) llm.ProviderStatus {
	if s.capabilities(r).ConfigureHost {
		return status
	}
	settings, _ := s.store.Snapshot()
	view := llm.ProviderStatus{Provider: settings.LLM.Provider, Available: status.Available, ModelAvailable: status.ModelAvailable, Managed: status.Managed, Loaded: status.Loaded, Loading: status.Loading}
	if !status.Available || !status.ModelAvailable {
		view.Message = administratorDetails
	}
	return view
}
