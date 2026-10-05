package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestSetupVoiceSharesMemoryWithChat(t *testing.T) {
	large := setupRequirement{Status: setupRequirementMet, VRAMMiB: 8400, MinVRAMMiB: 10240}
	small := setupRequirement{Status: setupRequirementMet, VRAMMiB: 3446, MinVRAMMiB: 6144}
	for _, test := range []struct {
		name                       string
		chat                       setupRequirement
		vram                       int
		nvidia                     bool
		module, device, qwenStatus string
	}{
		{"16 GiB", large, 16384, true, "faster-qwen3-tts", "cuda", "fits"},
		{"12 GiB", large, 12288, true, "chatterbox", "cuda", "insufficient"},
		{"8 GiB", small, 8192, true, "chatterbox", "cuda", "insufficient"},
		{"6 GiB", small, 6144, true, "chatterbox", "cpu", "insufficient"},
		{"exact boundary", large, 8400 + setupQwenVRAMMiB + setupDesktopVRAMMiB, true, "faster-qwen3-tts", "cuda", "fits"},
		{"one MiB below", large, 8400 + setupQwenVRAMMiB + setupDesktopVRAMMiB - 1, true, "chatterbox", "cuda", "insufficient"},
		{"unknown GPU", large, 0, true, "faster-qwen3-tts", "cuda", "unknown"},
		{"unmeasured LLM", setupRequirement{Status: setupRequirementPartial, Reason: "model_unknown"}, 16384, true, "faster-qwen3-tts", "cuda", "unknown"},
		{"voice alone exceeds GPU", setupRequirement{Status: setupRequirementPartial, Reason: "model_unknown"}, 4096, true, "chatterbox", "cuda", "insufficient"},
		{"chat skipped", setupRequirement{Status: setupRequirementUnmet}, 8192, true, "faster-qwen3-tts", "cuda", "fits"},
		{"CPU only", setupRequirement{Status: setupRequirementUnmet}, 0, false, "chatterbox", "cpu", "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := assessSetupVoice(test.chat, true, test.nvidia, test.vram)
			qwen, chosen := options[0], options[0]
			if chosen.Requirement.Status == setupRequirementUnmet || chosen.Memory.Status == "insufficient" {
				chosen = options[1]
			}
			if chosen.Module != test.module || chosen.Device != test.device || qwen.Memory.Status != test.qwenStatus {
				t.Fatalf("chosen=%+v, Qwen=%+v", chosen, qwen)
			}
			if test.nvidia && qwen.Requirement.Status == setupRequirementUnmet {
				t.Fatal("a memory warning must not prevent forcing Qwen installation")
			}
			if qwen.Memory.RequiredMiB != max(test.chat.MinVRAMMiB, test.chat.VRAMMiB+setupQwenVRAMMiB+setupDesktopVRAMMiB) {
				t.Fatalf("combined memory omits the LLM or reserve: %+v", qwen.Memory)
			}
			if chosen.Device == config.TTSDeviceCPU && chosen.Memory.VoiceVRAMMiB != 0 {
				t.Fatal("CPU voice must not reserve graphics memory")
			}
		})
	}
}

func TestSetupVoiceRequiresInstallerAndNVIDIAForQwen(t *testing.T) {
	for _, option := range assessSetupVoice(setupRequirement{Status: setupRequirementUnmet}, false, true, 16384) {
		if option.Requirement.Status != setupRequirementUnmet || option.Requirement.Bytes != 0 {
			t.Fatalf("missing installer remains available: %+v", option)
		}
	}
	qwen := assessSetupVoice(setupRequirement{Status: setupRequirementUnmet}, true, false, 0)[0]
	if qwen.Requirement.Status != setupRequirementUnmet || qwen.Requirement.Reason != "no_nvidia" {
		t.Fatalf("Qwen must still require supported hardware: %+v", qwen)
	}
}
