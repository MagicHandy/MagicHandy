package httpapi

import (
	"context"
	"runtime"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestSetupAssessmentPicksTheBestModelEachMachineHolds(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("managed features are assessed as unavailable off Windows/amd64")
	}
	catalog := llm.CatalogModels()
	all := map[string]bool{"llama": true, "parakeet": true, "voice": true}
	for name, test := range map[string]struct {
		hardware    map[string]any
		helpers     map[string]bool
		chat        string
		reason      string
		model       string
		voiceOutput string
		voiceInput  string
	}{
		"16 GB NVIDIA": {map[string]any{"nvidia": true, "vram_mib": "16303"}, all, setupRequirementMet, "gpu_fits", catalog[0].ID, setupRequirementMet, setupRequirementMet},
		"8 GB NVIDIA":  {map[string]any{"nvidia": true, "vram_mib": "8192"}, all, setupRequirementMet, "gpu_fits", catalog[2].ID, setupRequirementMet, setupRequirementMet},
		"4 GB NVIDIA":  {map[string]any{"nvidia": true, "vram_mib": "4096"}, all, setupRequirementUnmet, "vram_below", "", setupRequirementMet, setupRequirementMet},
		"unknown VRAM": {map[string]any{"nvidia": true}, all, setupRequirementPartial, "vram_unknown", catalog[0].ID, setupRequirementPartial, setupRequirementMet},
		"no NVIDIA":    {map[string]any{"nvidia": false}, all, setupRequirementUnmet, "no_nvidia", "", setupRequirementPartial, setupRequirementMet},
		"no helpers":   {map[string]any{"nvidia": true, "vram_mib": "16303"}, map[string]bool{}, setupRequirementMet, "gpu_fits", catalog[0].ID, setupRequirementUnmet, setupRequirementUnmet},
	} {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t)
			got := server.assessSetup(context.Background(), test.hardware, test.helpers, server.store.DataDir(), false)
			if got.Chat.Status != test.chat || got.Chat.Reason != test.reason || got.ModelID != test.model {
				t.Fatalf("chat = %+v, model %q; want %s/%s/%q", got.Chat, got.ModelID, test.chat, test.reason, test.model)
			}
			if got.VoiceOutput.Status != test.voiceOutput || got.VoiceInput.Status != test.voiceInput {
				t.Fatalf("voice output %+v, input %+v", got.VoiceOutput, got.VoiceInput)
			}
			if got.Chat.Status == setupRequirementMet && got.Chat.Bytes <= uint64(catalogModelSize(test.model)) { //nolint:gosec // catalog sizes are positive.
				t.Fatalf("chat needs %d bytes, less than its model download", got.Chat.Bytes)
			}
			if got.FreeDiskBytes == 0 {
				t.Fatal("free disk space was not measured")
			}
		})
	}
}

func TestSetupAssessmentKeepsSelectedUnmeasuredModel(t *testing.T) {
	server := newTestServer(t)
	selected := importHTTPAPIModel(t, server, httpAPITestGGUF())
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.Model = selected.ID
		return settings
	})
	got := server.assessSetup(context.Background(), map[string]any{"nvidia": true, "vram_mib": "16384"}, map[string]bool{"voice": true}, server.store.DataDir(), false)
	if got.ModelInstalled != selected.ID || got.ModelName != selected.DisplayName || got.ModelID != "" || got.Chat.Reason != "model_unknown" {
		t.Fatalf("assessment substitutes a measured catalog model: %+v", got)
	}
	if got.VoiceOptions[0].Memory.LLMKnown || got.VoiceOptions[0].Memory.Status != "unknown" {
		t.Fatalf("unmeasured model is claimed to fit: %+v", got.VoiceOptions[0])
	}
}

func TestSetupAssessmentDoesNotUseDefaultContextMeasurementForLargerContext(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("managed catalog selection requires Windows/amd64")
	}
	server := newTestServer(t)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.LlamaCPPContextSize = 65536
		return settings
	})
	got := server.assessSetup(context.Background(), map[string]any{"nvidia": true, "vram_mib": "16384"}, map[string]bool{"voice": true}, server.store.DataDir(), false)
	if got.Chat.VRAMMiB != 0 || got.VoiceOptions[0].Memory.Status != "unknown" {
		t.Fatalf("large context wrongly rated against the default-context measurement: %+v", got)
	}
}

func catalogModelSize(id string) int64 {
	if model, ok := llm.FindCatalogModel(id); ok {
		return model.SizeBytes
	}
	return 0
}
