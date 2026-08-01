package config

import (
	"strings"
	"testing"
)

func TestLlamaCPPContextSizeCatalogAndUpdateValidation(t *testing.T) {
	want := []int{16384, 32768, 65536, 131072}
	options := DefaultSettings().Public().Options.LlamaCPPContextSizes
	if len(options) != len(want) {
		t.Fatalf("llama.cpp context size options = %v, want %v", options, want)
	}
	for index, size := range want {
		if options[index] != size {
			t.Fatalf("llama.cpp context size option %d = %d, want %d", index, options[index], size)
		}
		settings := DefaultSettings()
		settings.LLM.LlamaCPPContextSize = size
		if _, err := NormalizeSettings(settings); err != nil {
			t.Errorf("NormalizeSettings context size %d: %v", size, err)
		}
	}
	options[0] = 1
	if LlamaCPPContextSizes()[0] != want[0] {
		t.Fatal("public context-size options alias config catalog storage")
	}

	current := DefaultSettings()
	current.LLM.LlamaCPPContextSize = 65536
	update := SettingsUpdate{
		Server:      current.Server,
		Device:      DeviceUpdate{HSPDispatchOwner: current.Device.HSPDispatchOwner, IntifaceServerAddress: current.Device.IntifaceServerAddress, FirmwareAPIRequirement: current.Device.FirmwareAPIRequirement, APIApplicationIDSource: current.Device.APIApplicationIDSource},
		Motion:      current.Motion,
		LLM:         LLMUpdateFromSettings(current.LLM),
		Diagnostics: current.Diagnostics,
	}
	update.LLM.LlamaCPPContextSize = nil
	preserved, err := current.ApplyUpdate(update)
	if err != nil {
		t.Fatalf("ApplyUpdate omitted context size: %v", err)
	}
	if preserved.LLM.LlamaCPPContextSize != 65536 {
		t.Fatalf("omitted context size = %d, want preserved 65536", preserved.LLM.LlamaCPPContextSize)
	}

	invalid := 8192
	update.LLM.LlamaCPPContextSize = &invalid
	if _, err := current.ApplyUpdate(update); err == nil || !strings.Contains(err.Error(), "unsupported managed llama.cpp context size") {
		t.Fatalf("invalid context-size update error = %v", err)
	}
	settings := DefaultSettings()
	settings.LLM.LlamaCPPContextSize = invalid
	if _, err := NormalizeSettings(settings); err == nil || !strings.Contains(err.Error(), "unsupported managed llama.cpp context size") {
		t.Fatalf("invalid persisted context-size error = %v", err)
	}
}
