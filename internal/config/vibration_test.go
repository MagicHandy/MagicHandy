package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestVibrationDefaultsStartLowAndFollowSpeed(t *testing.T) {
	settings, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Vibration != DefaultVibrationSettings() || settings.Vibration.CeilingPercent != 40 || settings.Vibration.Follow != VibrationFollowSpeed {
		t.Fatalf("defaults = %+v", settings.Vibration)
	}
	// Settings saved before the vibrator existed gain the defaults; a stored
	// 0% ceiling is a deliberate choice and stays.
	older := settings
	older.Vibration = VibrationSettings{}
	if migrated, err := NormalizeSettings(cloneSettings(older)); err != nil || migrated.Vibration != DefaultVibrationSettings() {
		t.Fatalf("missing block = %+v, %v", migrated.Vibration, err)
	}
	silent := settings
	silent.Vibration.CeilingPercent = 0
	if kept, err := NormalizeSettings(cloneSettings(silent)); err != nil || kept.Vibration.CeilingPercent != 0 {
		t.Fatalf("0%% ceiling = %+v, %v", kept.Vibration, err)
	}
}

func TestVibrationSettingsRejectInvalidRanges(t *testing.T) {
	for _, change := range []func(*VibrationSettings){
		func(v *VibrationSettings) { v.Follow = "thrust" },
		func(v *VibrationSettings) { v.DepthEnd = "middle" },
		func(v *VibrationSettings) { v.CeilingPercent = 101 },
		func(v *VibrationSettings) { v.MinimumPercent = 50; v.CeilingPercent = 40 },
		func(v *VibrationSettings) { v.TimingOffsetMillis = 301 },
	} {
		settings := DefaultSettings()
		change(&settings.Vibration)
		if _, err := NormalizeSettings(settings); err == nil {
			t.Fatalf("accepted %+v", settings.Vibration)
		}
	}
}

func TestLegacyOutputWriteKeepsTheVibratorPlaying(t *testing.T) {
	settings, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	settings.Device, _ = settings.Device.ChangeConnection(DeviceVibrator, "add")
	settings.Device, _ = settings.Device.ChangeConnection(DeviceVibrator, "include")
	if !reflect.DeepEqual(settings.Device.PlaybackDevices, []string{DeviceOutputHandy, DeviceVibrator}) || settings.Device.Output != DeviceOutputHandy {
		t.Fatalf("vibrator included: %+v", settings.Device)
	}
	// An older client only knows the Handy and the Coyote.
	update := settingsUpdateFrom(t, settings)
	update.Device.Output = DeviceOutputCoyote
	settings.Device, _ = settings.Device.ChangeConnection(DeviceOutputCoyote, "add")
	next, err := settings.ApplyUpdate(update)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next.Device.PlaybackDevices, []string{DeviceOutputCoyote, DeviceVibrator}) {
		t.Fatalf("legacy write lost the vibrator: %v", next.Device.PlaybackDevices)
	}
	data, _ := json.Marshal(next.Public().Device)
	var public map[string]any
	_ = json.Unmarshal(data, &public)
	if public["output"] != DeviceOutputCoyote {
		t.Fatalf("legacy projection = %v", public["output"])
	}
}

func TestVibratorOnlyPlaybackIsValid(t *testing.T) {
	settings, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	settings.Device, _ = settings.Device.ChangeConnection(DeviceVibrator, "add")
	settings.Device, _ = settings.Device.ChangeConnection(DeviceVibrator, "include")
	settings.Device, _ = settings.Device.ChangeConnection(DeviceOutputHandy, "exclude")
	normalized, err := NormalizeSettings(cloneSettings(settings))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized.Device.PlaybackSet(), []string{DeviceVibrator}) || normalized.Device.Output != DeviceOutputNone {
		t.Fatalf("vibrator only: %+v", normalized.Device)
	}
}

func TestVibrationLevelControlDefaultsToTheModelAndValidates(t *testing.T) {
	defaults := DefaultVibrationSettings()
	if defaults.LevelControl != VibrationLevelModel || !defaults.ModelSetsLevel() {
		t.Fatalf("defaults = %+v", defaults)
	}
	// A block saved before ADR 0061 keeps its values and gains model control.
	stored := VibrationSettings{Follow: VibrationFollowDepth, DepthEnd: VibrationDepthTip, MinimumPercent: 5, CeilingPercent: 30}
	if migrated := applyMissingVibrationDefaults(stored); migrated.LevelControl != VibrationLevelModel || migrated.CeilingPercent != 30 || migrated.Follow != VibrationFollowDepth {
		t.Fatalf("migrated = %+v", migrated)
	}
	manual := defaults
	manual.LevelControl = VibrationLevelManual
	if err := validateVibrationSettings(manual); err != nil || manual.ModelSetsLevel() {
		t.Fatalf("manual control: %v", err)
	}
	manual.LevelControl = "autopilot"
	if err := validateVibrationSettings(manual); err == nil {
		t.Fatal("accepted an unknown level control")
	}
}

func TestOriginalCreativeModeIsHiddenUntilRestored(t *testing.T) {
	settings := DefaultSettings()
	if settings.LLM.OriginalCreativeMode || settings.LLM.MotionGenerationMode != LLMMotionModeLayered {
		t.Fatalf("defaults = %q original=%t", settings.LLM.MotionGenerationMode, settings.LLM.OriginalCreativeMode)
	}
	// A chat already on the original Creative mode keeps it offered.
	settings.LLM.MotionGenerationMode = LLMMotionModeDynamic
	normalized, err := NormalizeSettings(settings)
	if err != nil || !normalized.LLM.OriginalCreativeMode {
		t.Fatalf("saved dynamic: %v original=%t", err, normalized.LLM.OriginalCreativeMode)
	}
	// Hiding it moves that chat to the default mode.
	off := false
	next, err := applyLLMUpdate(normalized.LLM, LLMUpdate{OriginalCreativeMode: &off})
	if err != nil || next.OriginalCreativeMode || next.MotionGenerationMode != LLMMotionModeLayered {
		t.Fatalf("hidden: %v mode=%q original=%t", err, next.MotionGenerationMode, next.OriginalCreativeMode)
	}
	on := true
	if restored, err := applyLLMUpdate(next, LLMUpdate{OriginalCreativeMode: &on}); err != nil || !restored.OriginalCreativeMode || restored.MotionGenerationMode != LLMMotionModeLayered {
		t.Fatalf("restored: %v %+v", err, restored.MotionGenerationMode)
	}
	// Settings from before the selector still adopt Creative, and keep it offered.
	legacy, _, err := loadSettingsFromBytes([]byte(`{"version":2,"llm":{"model":"local"}}`))
	if err != nil || legacy.LLM.MotionGenerationMode != LLMMotionModeDynamic || !legacy.LLM.OriginalCreativeMode {
		t.Fatalf("pre-selector: %v mode=%q original=%t", err, legacy.LLM.MotionGenerationMode, legacy.LLM.OriginalCreativeMode)
	}
}
