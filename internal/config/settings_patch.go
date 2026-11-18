package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrSettingsConflict asks a form to reload when another writer changed a field it edited.
var ErrSettingsConflict = errors.New("settings changed since editing began; reload the settings and retry")

// SettingsPatch compares only edited fields, inside the same store transaction as the save.
// A missing optional field compares as JSON null. Voice keys compare their public set flag.
type SettingsPatch struct {
	CredentialRevision string                                `json:"credential_revision,omitempty"`
	Changes            map[string]map[string]json.RawMessage `json:"changes"`
	Expected           map[string]map[string]json.RawMessage `json:"expected"`
}

// ChangesCredential reports a write-only edit that needs the saved revision.
func (p SettingsPatch) ChangesCredential() bool {
	keys := p.Changes["voice"]
	return keys["elevenlabs_api_key"] != nil || keys["openai_tts_api_key"] != nil
}

// ApplyPatch preserves server-owned fields and credentials outside the submitted edits.
func (s Settings) ApplyPatch(patch SettingsPatch) (Settings, error) {
	next := cloneSettings(s)
	llm := LLMUpdateFromSettings(next.LLM)
	targets := map[string]any{"ui": &next.UI, "media": &next.Media, "motion": &next.Motion, "llm": &llm,
		"voice": &next.Voice, "chat": &next.Chat, "autopilot": &next.Autopilot, "diagnostics": &next.Diagnostics}
	public := s.Public()
	current := map[string]any{"ui": public.UI, "media": public.Media, "motion": public.Motion, "llm": public.LLM,
		"voice": public.Voice, "chat": public.Chat, "autopilot": public.Autopilot, "diagnostics": public.Diagnostics}
	for section, fields := range patch.Changes {
		if targets[section] == nil {
			return s, errors.New("unknown settings form section")
		}
		if err := compareSettingsFields(section, current[section], fields, patch.Expected[section]); err != nil {
			return s, err
		}
		data, err := json.Marshal(fields)
		if err != nil {
			return s, err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(targets[section]); err != nil {
			return s, errors.New("invalid settings value")
		}
	}
	if _, changed := patch.Changes["llm"]; changed {
		updated, err := applyLLMUpdate(s.LLM, llm)
		if err != nil {
			return s, err
		}
		next.LLM = updated
	}
	return NormalizeSettings(next)
}

func compareSettingsFields(section string, current any, changes, expected map[string]json.RawMessage) error {
	data, err := json.Marshal(current)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name := range changes {
		publicName := name
		if section == "voice" && (name == "elevenlabs_api_key" || name == "openai_tts_api_key") {
			publicName = strings.TrimSuffix(name, "_api_key") + "_key_set"
		}
		if !editableSettingField(section, name, publicName, reflect.TypeOf(current)) {
			return errors.New("field is outside this settings form")
		}
		before, exists := expected[name]
		var value any
		if !exists || json.Unmarshal(before, &value) != nil {
			return errors.New("missing or invalid expected settings value")
		}
		if !reflect.DeepEqual(fields[publicName], value) {
			return fmt.Errorf("%s.%s: %w", section, name, ErrSettingsConflict)
		}
	}
	return nil
}

func editableSettingField(section, name, publicName string, shape reflect.Type) bool {
	if section == "motion" && name != "apply_video_speed_limit" || section == "media" && (name == "peak_rounding_ms" || name == "script_smoothing_percent") {
		return false
	}
	if section == "voice" && strings.HasSuffix(name, "_key_set") {
		return false
	}
	for field := range shape.Fields() {
		tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if tag == publicName && tag != "" && tag != "-" {
			return true
		}
	}
	return false
}
