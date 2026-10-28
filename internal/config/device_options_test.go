package config

import (
	"encoding/json"
	"testing"
)

func patchFromJSON(t *testing.T, body string) DeviceOptionsPatch {
	t.Helper()
	var patch DeviceOptionsPatch
	if err := json.Unmarshal([]byte(body), &patch); err != nil {
		t.Fatal(err)
	}
	return patch
}

func TestDeviceOptionsPatchChangesOnlyTheNamedFields(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	s.Estim.RememberedAddress = "AA:BB:CC:DD:EE:FF"
	next, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, `{"estim":{"max_strength_a":45,"swap_channels":true},"vibration":{"ceiling_percent":55}}`))
	if err != nil {
		t.Fatal(err)
	}
	if next.Estim.MaxStrengthA != 45 || !next.Estim.SwapChannels || next.Vibration.CeilingPercent != 55 {
		t.Fatalf("patch not applied: %+v %+v", next.Estim, next.Vibration)
	}
	if next.Estim.MaxStrengthB != s.Estim.MaxStrengthB || next.Estim.Follow != s.Estim.Follow || next.Vibration.Follow != s.Vibration.Follow {
		t.Fatalf("unnamed fields changed: %+v %+v", next.Estim, next.Vibration)
	}
	if next.Estim.RememberedAddress != s.Estim.RememberedAddress || next.Device.HandyConnectionKey != s.Device.HandyConnectionKey {
		t.Fatal("the patch touched a field it cannot name")
	}
	if s.Estim.MaxStrengthA == 45 {
		t.Fatal("the patch mutated its input")
	}
}

func TestDeviceOptionsPatchForgetsTheCoyoteAndClearsABundledOverride(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	s.Estim.RememberedAddress = "AA:BB:CC:DD:EE:FF"
	s.Device.APIApplicationIDSource = ApplicationIDSourceDeveloperOverride
	s.Device.APIApplicationIDOverride = "developer-id"
	next, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, `{"estim":{"forget_device":true},"api_application_id_source":"bundled_app_id"}`))
	if err != nil {
		t.Fatal(err)
	}
	if next.Estim.RememberedAddress != "" {
		t.Fatal("forget kept the remembered Coyote")
	}
	if next.Device.APIApplicationIDSource != ApplicationIDSourceBundled || next.Device.APIApplicationIDOverride != "" {
		t.Fatalf("bundled source kept an override: %+v", next.Device)
	}
}

func TestDeviceOptionsPatchForgetsTheConnectionKeyOnlyWhenAsked(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	s.Device.HandyConnectionKey = "private-key"
	kept, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, `{"linked_failure_policy":"continue_healthy"}`))
	if err != nil || kept.Device.HandyConnectionKey != "private-key" {
		t.Fatalf("an unrelated change dropped the key: %v", err)
	}
	cleared, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, `{"clear_connection_key":true}`))
	if err != nil || cleared.Device.HandyConnectionKey != "" {
		t.Fatalf("clear kept the key: %v", err)
	}
}

func TestDeviceOptionsPatchRejectsInvalidAndEmptyChanges(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{}`,
		`{"estim":{}}`,
		`{"linked_failure_policy":"ignore"}`,
		`{"intiface_server_address":"http://127.0.0.1:12345"}`,
		`{"estim":{"max_strength_a":201}}`,
		`{"estim":{"frequency_min_hz":90,"frequency_max_hz":80}}`,
		`{"estim":{"follow":"tickle"}}`,
		`{"vibration":{"minimum_percent":50,"ceiling_percent":40}}`,
		`{"vibration":{"timing_offset_ms":301}}`,
		`{"api_application_id_source":"borrowed"}`,
	} {
		next, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, body))
		if err == nil {
			t.Fatalf("%s was accepted: %+v", body, next.Device)
		}
	}
}

func TestDeviceOptionsPatchTrimsTheIntifaceAddressAndPolicy(t *testing.T) {
	s, err := NormalizeSettings(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, `{"intiface_server_address":"  ws://192.168.1.20:12345  ","linked_failure_policy":" continue_healthy "}`))
	if err != nil {
		t.Fatal(err)
	}
	if next.Device.IntifaceServerAddress != "ws://192.168.1.20:12345" || next.Device.LinkedFailurePolicy != LinkedFailureContinueHealthy {
		t.Fatalf("not trimmed: %+v", next.Device)
	}
}

func TestDeviceOptionsPatchTurnsModelControlOff(t *testing.T) {
	s := DefaultSettings()
	next, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, `{"vibration":{"level_control":"manual"}}`))
	if err != nil || next.Vibration.LevelControl != VibrationLevelManual || next.Vibration.CeilingPercent != s.Vibration.CeilingPercent {
		t.Fatalf("patched = %+v %v", next.Vibration, err)
	}
	if _, err := s.ApplyDeviceOptionsPatch(patchFromJSON(t, `{"vibration":{"level_control":"sometimes"}}`)); err == nil {
		t.Fatal("accepted an unknown level control")
	}
}
