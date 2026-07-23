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
	motion := map[string]any{
		"speed_min_percent":      settings.Motion.SpeedMinPercent,
		"speed_max_percent":      settings.Motion.SpeedMaxPercent,
		"stroke_min_percent":     settings.Motion.StrokeMinPercent,
		"stroke_max_percent":     settings.Motion.StrokeMaxPercent,
		"reverse_direction":      settings.Motion.ReverseDirection,
		"style":                  settings.Motion.Style,
		"motion_generation_mode": settings.Motion.MotionGenerationMode,
		"hardware_safety_lock":   settings.Motion.HardwareSafetyLock,
	}
	if len(settings.Motion.MotionPreferences) > 0 {
		var prefs any
		if err := json.Unmarshal(settings.Motion.MotionPreferences, &prefs); err == nil {
			motion["motion_preferences"] = prefs
		}
	}
	payload := map[string]any{
		"motion": motion,
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
			"prompt_set":     settings.LLM.PromptSet,
			"director_mode":  settings.LLM.DirectorMode,
		},
		"ollama": map[string]any{
			"base_url": settings.LLM.LlamaCPPBaseURL,
			"model":    settings.LLM.Model,
		},
		"diagnostics": map[string]any{
			"verbosity":                settings.Diagnostics.Verbosity,
			"log_handy_motion":         settings.Diagnostics.ShouldLogHandyMotion(),
			"log_handy_motion_verbose": settings.Diagnostics.VerboseHandyMotion(),
		},
		"autodom": map[string]any{
			"allow_dominatrix":         settings.AutoDom.AllowDominatrix,
			"dominatrix_ramp_minutes":  settings.AutoDom.DominatrixRampMinutes,
			"wait_for_user_message":    settings.AutoDom.ShouldWaitForUserMessage(),
			"segment_duration_min_sec": settings.AutoDom.SegmentDurationMinSec,
			"segment_duration_max_sec": settings.AutoDom.SegmentDurationMaxSec,
			"prefetch_lead_seconds":    settings.AutoDom.PrefetchLeadSeconds,
		},
		"user_profile": map[string]any{
			"gender":             settings.UserProfile.Gender,
			"sexual_orientation": settings.UserProfile.SexualOrientation,
			"about_me":           settings.UserProfile.AboutMe,
		},
	}
	return mergeLsoUISections(payload, settings.UISections)
}

func applyLsoSettingsUpdate(current config.Settings, updates map[string]json.RawMessage) (config.Settings, error) {
	next := current
	if next.UISections == nil {
		next.UISections = map[string]json.RawMessage{}
	}
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
		"llm": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoLLMSettingsUpdate(s, raw)
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
		"user_profile": func(s config.Settings, raw json.RawMessage) (config.Settings, error) {
			return applyLsoUserProfileSettingsUpdate(s, raw)
		},
	}

	for key, raw := range updates {
		if handler, ok := handlers[key]; ok {
			updated, err := handler(next, raw)
			if err != nil {
				return config.Settings{}, err
			}
			next = updated
			if key == "ollama" {
				next.UISections["ollama"] = raw
			}
			continue
		}
		if key == "app" {
			continue
		}
		next.UISections[key] = raw
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
	motion, err := mergeJSONSection(current.Motion, raw)
	if err != nil {
		return config.Settings{}, err
	}
	next.Motion = motion
	return next, nil
}

func applyLsoLLMSettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	llm, err := mergeJSONSection(current.LLM, raw)
	if err != nil {
		return config.Settings{}, err
	}
	next.LLM = llm
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
	var patch map[string]any
	if err := json.Unmarshal(raw, &patch); err != nil {
		return config.Settings{}, err
	}
	if verbosity, ok := patch["verbosity"].(string); ok {
		next.Diagnostics.Verbosity = verbosity
	}
	if enabled, ok := patch["log_handy_motion"].(bool); ok {
		enabledCopy := enabled
		next.Diagnostics.LogHandyMotion = &enabledCopy
	}
	if verbose, ok := patch["log_handy_motion_verbose"].(bool); ok {
		verboseCopy := verbose
		next.Diagnostics.LogHandyMotionVerbose = &verboseCopy
	}
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
	if wait, ok := patch["wait_for_user_message"].(bool); ok {
		waitCopy := wait
		next.AutoDom.WaitForUserMessage = &waitCopy
	}
	if minSec, ok := patch["segment_duration_min_sec"].(float64); ok {
		next.AutoDom.SegmentDurationMinSec = int(minSec)
	}
	if maxSec, ok := patch["segment_duration_max_sec"].(float64); ok {
		next.AutoDom.SegmentDurationMaxSec = int(maxSec)
	}
	if lead, ok := patch["prefetch_lead_seconds"].(float64); ok {
		next.AutoDom.PrefetchLeadSeconds = int(lead)
	}
	return next, nil
}

func applyLsoUserProfileSettingsUpdate(current config.Settings, raw json.RawMessage) (config.Settings, error) {
	next := current
	profile, err := mergeJSONSection(current.UserProfile, raw)
	if err != nil {
		return config.Settings{}, err
	}
	next.UserProfile = profile
	return next, nil
}

func mergeJSONSection[T any](current T, raw json.RawMessage) (T, error) {
	var out T
	base, err := json.Marshal(current)
	if err != nil {
		return out, err
	}
	var baseMap map[string]json.RawMessage
	if err := json.Unmarshal(base, &baseMap); err != nil {
		return out, err
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(raw, &patch); err != nil {
		return out, err
	}
	for key, value := range patch {
		baseMap[key] = value
	}
	merged, err := json.Marshal(baseMap)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(merged, &out); err != nil {
		return out, err
	}
	return out, nil
}

func mergeLsoUISections(payload map[string]any, sections map[string]json.RawMessage) map[string]any {
	if len(sections) == 0 {
		return payload
	}
	for key, raw := range sections {
		var section any
		if err := json.Unmarshal(raw, &section); err != nil {
			continue
		}
		if existing, ok := payload[key].(map[string]any); ok {
			if patch, ok := section.(map[string]any); ok {
				for patchKey, patchValue := range patch {
					existing[patchKey] = patchValue
				}
				continue
			}
		}
		payload[key] = section
	}
	return payload
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
