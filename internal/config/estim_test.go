package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEstimDefaultsStartLowAndDisarmed(t *testing.T) {
	settings := DefaultSettings()
	if settings.Device.Output != DeviceOutputHandy {
		t.Fatalf("existing installs must keep driving the Handy, got %q", settings.Device.Output)
	}
	estim := settings.Estim
	if estim.MaxStrengthA != 30 || estim.MaxStrengthB != 30 || estim.SafetyAcknowledged {
		t.Fatalf("Coyote defaults = %+v", estim)
	}
	if estim.FrequencyBalanceA != 160 || estim.IntensityBalanceA != 0 || estim.Follow != EstimFollowStroke {
		t.Fatalf("Coyote device defaults = %+v", estim)
	}
}

func TestEstimBlockMissingFromOlderSettingsGetsDefaults(t *testing.T) {
	older := DefaultSettings()
	older.Estim = EstimSettings{}
	older.Device.Output = ""
	normalized, err := NormalizeSettings(older)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Device.Output != DeviceOutputHandy || normalized.Estim != DefaultEstimSettings() {
		t.Fatalf("normalized = %+v / %+v", normalized.Device, normalized.Estim)
	}
	saved := DefaultSettings()
	saved.Estim.MaxStrengthA = 0
	normalized, err = NormalizeSettings(saved)
	if err != nil || normalized.Estim.MaxStrengthA != 0 {
		t.Fatalf("a saved zero ceiling is a user choice and must stay: %+v, %v", normalized.Estim, err)
	}
}

func TestEstimValidationRejectsOutOfRangeValues(t *testing.T) {
	cases := map[string]func(*Settings){
		"output":          func(s *Settings) { s.Device.Output = "vibrator" },
		"ceiling":         func(s *Settings) { s.Estim.MaxStrengthB = 201 },
		"negative":        func(s *Settings) { s.Estim.MaxStrengthA = -1 },
		"profile":         func(s *Settings) { s.Estim.Follow = "position_to_strength" },
		"rate order":      func(s *Settings) { s.Estim.FrequencyMinHz, s.Estim.FrequencyMaxHz = 80, 40 },
		"rate range":      func(s *Settings) { s.Estim.FrequencyMaxHz = 101 },
		"balance":         func(s *Settings) { s.Estim.Balance = 101 },
		"offset":          func(s *Settings) { s.Estim.TimingOffsetMillis = -301 },
		"device balance":  func(s *Settings) { s.Estim.FrequencyBalanceB = 256 },
		"address":         func(s *Settings) { s.Estim.RememberedAddress = strings.Repeat("A", 65) },
		"intensity range": func(s *Settings) { s.Estim.IntensityBalanceA = -1 },
	}
	for name, mutate := range cases {
		settings := DefaultSettings()
		mutate(&settings)
		if _, err := NormalizeSettings(settings); err == nil {
			t.Errorf("%s: invalid settings were accepted", name)
		}
	}
}

func TestEstimUpdateKeepsTheRememberedDeviceUntilForgotten(t *testing.T) {
	settings := DefaultSettings()
	settings.Estim.RememberedAddress = "AA:BB:CC:DD:EE:FF"
	update := settingsUpdateFrom(t, settings)
	update.Estim = &EstimUpdate{
		SafetyAcknowledged: true, MaxStrengthA: 25, MaxStrengthB: 40, Follow: EstimFollowSpeed,
		FrequencyMinHz: 20, FrequencyMaxHz: 90, Balance: -20, FrequencyBalanceA: 160, FrequencyBalanceB: 160,
	}
	next, err := settings.ApplyUpdate(update)
	if err != nil {
		t.Fatal(err)
	}
	if next.Estim.RememberedAddress != "AA:BB:CC:DD:EE:FF" || next.Estim.MaxStrengthB != 40 || !next.Estim.SafetyAcknowledged {
		t.Fatalf("after update = %+v", next.Estim)
	}
	update.Estim.ForgetDevice = true
	next, err = next.ApplyUpdate(update)
	if err != nil || next.Estim.RememberedAddress != "" {
		t.Fatalf("ForgetDevice = %+v, %v", next.Estim, err)
	}
}

func TestDeviceOutputIsPreservedWhenAnUpdateOmitsIt(t *testing.T) {
	settings := DefaultSettings()
	settings.Device.Output = DeviceOutputCoyote
	update := settingsUpdateFrom(t, settings)
	update.Device.Output = ""
	next, err := settings.ApplyUpdate(update)
	if err != nil || next.Device.Output != DeviceOutputCoyote {
		t.Fatalf("output after an update without it = %q, %v", next.Device.Output, err)
	}
	update.Device.Output = DeviceOutputHandyCoyote
	next, err = settings.ApplyUpdate(update)
	if err != nil || next.Device.Output != DeviceOutputHandyCoyote {
		t.Fatalf("output change = %q, %v", next.Device.Output, err)
	}
}

func TestPublicEstimSettingsHideTheAddress(t *testing.T) {
	settings := DefaultSettings()
	settings.Estim.RememberedAddress = "AA:BB:CC:DD:EE:FF"
	public := settings.Public()
	data, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "AA:BB") || !public.Estim.RememberedDevice {
		t.Fatalf("public settings leaked the address or lost the flag: %s", data)
	}
	if len(public.Options.DeviceOutputs) != 4 || len(public.Options.EstimFollowModes) != 3 {
		t.Fatalf("option hints = %+v", public.Options)
	}
}

func TestOutputHelpers(t *testing.T) {
	if !UsesCoyote(DeviceOutputCoyote) || !UsesCoyote(DeviceOutputHandyCoyote) || UsesCoyote(DeviceOutputHandy) {
		t.Fatal("UsesCoyote is wrong")
	}
	if !UsesHandy(DeviceOutputHandy) || !UsesHandy(DeviceOutputHandyCoyote) || UsesHandy(DeviceOutputCoyote) {
		t.Fatal("UsesHandy is wrong")
	}
}

// settingsUpdateFrom builds the full update a settings client sends.
func settingsUpdateFrom(t *testing.T, settings Settings) SettingsUpdate {
	t.Helper()
	return SettingsUpdate{
		Server: settings.Server,
		Device: DeviceUpdate{
			Output:                 settings.Device.Output,
			HSPDispatchOwner:       settings.Device.HSPDispatchOwner,
			IntifaceServerAddress:  settings.Device.IntifaceServerAddress,
			FirmwareAPIRequirement: settings.Device.FirmwareAPIRequirement,
			APIApplicationIDSource: settings.Device.APIApplicationIDSource,
		},
		Motion:      settings.Motion,
		LLM:         LLMUpdateFromSettings(settings.LLM),
		Diagnostics: settings.Diagnostics,
	}
}
