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
