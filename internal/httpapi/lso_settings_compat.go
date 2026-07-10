package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const lsoUIHeader = "X-MagicHandy-UI"

func isLsoUIRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get(lsoUIHeader)), "lso")
}

func (s *Server) handleLsoGetSettings(w http.ResponseWriter, _ *http.Request) {
	settings, _ := s.store.Snapshot()
	writeJSON(w, http.StatusOK, lsoSettingsFromConfig(settings))
}

func (s *Server) handleLsoPutSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}

	var body struct {
		Updates map[string]json.RawMessage `json:"updates"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current, _ := s.store.Snapshot()
	next, err := applyLsoSettingsUpdate(current, body.Updates)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	saved, err := s.store.Save(next)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("settings could not be saved"))
		return
	}

	if raw, ok := body.Updates["app"]; ok {
		s.applyLsoAppStateUpdate(r.Context(), raw)
	}

	s.applySettingsRuntimeTransition(r.Context(), current, next)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"settings": lsoSettingsFromConfig(saved),
	})
}

func lsoSettingsFromConfig(settings config.Settings) map[string]any {
	return map[string]any{
		"motion": map[string]any{
			"speed_min_percent":      settings.Motion.SpeedMinPercent,
			"speed_max_percent":      settings.Motion.SpeedMaxPercent,
			"stroke_min_percent":     settings.Motion.StrokeMinPercent,
			"stroke_max_percent":     settings.Motion.StrokeMaxPercent,
			"reverse_direction":      settings.Motion.ReverseDirection,
			"style":                  settings.Motion.Style,
			"motion_generation_mode": settings.Motion.MotionGenerationMode,
			"hardware_safety_lock":   settings.Motion.HardwareSafetyLock,
		},
		"handy": map[string]any{
			"transport":      mapOwnerToDeviceTransport(settings.Device.HSPDispatchOwner),
			"connection_key": settings.Device.HandyConnectionKey,
			"base_url":       defaultHandyCloudBaseURL(),
		},
		"intiface": map[string]any{
			"server_url": settings.Device.IntifaceURL,
		},
		"llm": map[string]any{
			"provider":       settings.LLM.Provider,
			"llama_cpp_mode": settings.LLM.LlamaCPPMode,
			"base_url":       selectedLLMBaseURL(settings.LLM),
			"model":          settings.LLM.Model,
		},
		"ollama": map[string]any{
			"base_url": settings.LLM.LlamaCPPBaseURL,
			"model":    settings.LLM.Model,
		},
		"diagnostics": map[string]any{
			"verbosity": settings.Diagnostics.Verbosity,
		},
		"autodom": map[string]any{
			"allow_dominatrix":        settings.AutoDom.AllowDominatrix,
			"dominatrix_ramp_minutes": settings.AutoDom.DominatrixRampMinutes,
		},
	}
}

func applyLsoSettingsUpdate(current config.Settings, updates map[string]json.RawMessage) (config.Settings, error) {
	next := current
	handlers := map[string]func(config.Settings, json.RawMessage) (config.Settings, error){
		"handy": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoHandySettingsUpdate(s, raw)
		},
		"intiface": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoIntifaceSettingsUpdate(s, raw)
		},
		"motion": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoMotionSettingsUpdate(s, raw)
		},
		"ollama": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoOllamaSettingsUpdate(s, raw)
		},
		"diagnostics": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoDiagnosticsSettingsUpdate(s, raw)
		},
		"autodom": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoAutoDomSettingsUpdate(s, raw)
		},
	}

	for key, raw := range updates {
		if handler, ok := handlers[key]; ok {
			updated, err := handler(next, raw)
			if err != nil {
				return config.Settings{}, err
			}
			next = updated
		}
	}
	return config.NormalizeSettings(next)
}

func applyLsoHandySettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	var handy map[string]any
	if err := json.Unmarshal(raw, &handy); err != nil {
		return config.Settings{}, err
	}
	if transport, ok := handy["transport"].(string); ok && strings.TrimSpace(transport) != "" {
		owner, err := mapDeviceTransportToOwner(transport)
		if err != nil {
			return config.Settings{}, err
		}
		next.Device.HSPDispatchOwner = owner
	}
	if connectionKey, ok := handy["connection_key"].(string); ok {
		connectionKey = strings.TrimSpace(connectionKey)
		if connectionKey != "" {
			next.Device.HandyConnectionKey = connectionKey
		}
	}
	return next, nil
}

func applyLsoIntifaceSettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	var intiface map[string]any
	if err := json.Unmarshal(raw, &intiface); err != nil {
		return config.Settings{}, err
	}
	if serverURL, ok := intiface["server_url"].(string); ok {
		serverURL = strings.TrimSpace(serverURL)
		if serverURL != "" {
			next.Device.IntifaceURL = serverURL
		}
	}
	return next, nil
}

func applyLsoMotionSettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	var motion config.MotionSettings
	if err := json.Unmarshal(raw, &motion); err != nil {
		return config.Settings{}, err
	}
	next.Motion = motion
	return next, nil
}

func applyLsoOllamaSettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	var ollama map[string]any
	if err := json.Unmarshal(raw, &ollama); err != nil {
		return config.Settings{}, err
	}
	if baseURL, ok := ollama["base_url"].(string); ok {
		baseURL = strings.TrimSpace(baseURL)
		if baseURL != "" {
			next.LLM.LlamaCPPBaseURL = baseURL
		}
	}
	if model, ok := ollama["model"].(string); ok {
		model = strings.TrimSpace(model)
		if model != "" {
			next.LLM.Model = model
		}
	}
	return next, nil
}

func applyLsoDiagnosticsSettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	var diagnostics config.DiagnosticsSettings
	if err := json.Unmarshal(raw, &diagnostics); err != nil {
		return config.Settings{}, err
	}
	next.Diagnostics = diagnostics
	return next, nil
}

func applyLsoAutoDomSettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	var patch map[string]any
	if err := json.Unmarshal(raw, &patch); err != nil {
		return config.Settings{}, err
	}
	if allow, ok := patch["allow_dominatrix"].(bool); ok {
		next.AutoDom.AllowDominatrix = allow
	}
	if ramp, ok := patch["dominatrix_ramp_minutes"].(float64); ok {
		next.AutoDom.DominatrixRampMinutes = int(ramp)
	}
	return next, nil
}

func defaultHandyCloudBaseURL() string {
	return "https://www.handyfeeling.com/api/handy-rest/v3/"
}

func (s *Server) applyLsoAppStateUpdate(ctx context.Context, raw json.RawMessage) {
	var app map[string]any
	if err := json.Unmarshal(raw, &app); err != nil {
		return
	}
	state, err := s.store.DB().LoadAppState()
	if err != nil {
		return
	}
	previousMode := state.OperationMode
	if mode, ok := app["operation_mode"].(string); ok && strings.TrimSpace(mode) != "" {
		state.OperationMode = strings.ToLower(strings.TrimSpace(mode))
	}
	if _, err := s.store.DB().SaveAppState(state); err != nil {
		return
	}
	if state.OperationMode == "auto" && previousMode != "auto" {
		s.startChatAutoLoop(ctx)
	}
	if state.OperationMode != "auto" && previousMode == "auto" {
		s.stopChatAutoLoop(ctx)
	}
}
