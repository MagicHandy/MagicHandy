package httpapi

import "github.com/mapledaemon/MagicHandy/internal/config"

// Planning allowances for the pinned default voice models, not measured peaks
// or guarantees. CUDA graphs, reference length and other GPU users vary. See
// docs/decisions/0037-easy-setup.md for the estimate and its limits.
const (
	setupQwenVRAMMiB       = 4096
	setupChatterboxVRAMMiB = 2048
	setupDesktopVRAMMiB    = 1024
)

type setupVoiceMemory struct {
	Status       string `json:"status"` // fits, insufficient, unknown, cpu
	LLMKnown     bool   `json:"llm_known"`
	LLMVRAMMiB   int    `json:"llm_vram_mib"`
	VoiceVRAMMiB int    `json:"voice_vram_mib"`
	ReserveMiB   int    `json:"reserve_mib"`
	RequiredMiB  int    `json:"required_mib"`
	AvailableMiB int    `json:"available_mib"`
}

type setupVoiceOption struct {
	Module      string           `json:"module"`
	Device      string           `json:"device"`
	Requirement setupRequirement `json:"requirement"`
	Memory      setupVoiceMemory `json:"memory"`
}

func setupVoiceBudget(chat setupRequirement, voiceMiB, availableMiB int, nvidia bool) setupVoiceMemory {
	if chat.Status == setupRequirementUnmet {
		// A skipped chat model consumes no GPU memory; MinVRAMMiB may still
		// explain why local chat was unavailable on this machine.
		chat.VRAMMiB, chat.MinVRAMMiB = 0, 0
	}
	known := chat.Status == setupRequirementUnmet || chat.VRAMMiB > 0
	memory := setupVoiceMemory{
		Status: "fits", LLMKnown: known, LLMVRAMMiB: chat.VRAMMiB,
		VoiceVRAMMiB: voiceMiB, ReserveMiB: setupDesktopVRAMMiB,
		AvailableMiB: availableMiB,
	}
	memory.ReserveMiB = max(memory.ReserveMiB, chat.MinVRAMMiB-chat.VRAMMiB-voiceMiB)
	memory.RequiredMiB = chat.VRAMMiB + voiceMiB + memory.ReserveMiB
	switch {
	case !nvidia && voiceMiB == 0:
		memory.Status = "cpu"
	case availableMiB <= 0:
		memory.Status = "unknown"
	case memory.RequiredMiB > availableMiB:
		memory.Status = "insufficient"
	case !known:
		memory.Status = "unknown"
	}
	return memory
}

// Qwen remains selectable below the estimate. Only an unavailable installer or
// unsupported hardware disables it. Chatterbox uses CPU if its own GPU budget
// would also exceed the card, leaving the selected LLM room to run.
func assessSetupVoice(chat setupRequirement, available, nvidia bool, vram int) []setupVoiceOption {
	qwen := setupVoiceOption{
		Module: "faster-qwen3-tts", Device: config.TTSDeviceCUDA,
		Requirement: setupRequirement{Status: setupRequirementMet, Reason: "qwen_gpu", Bytes: setupQwenBytes},
		Memory:      setupVoiceBudget(chat, setupQwenVRAMMiB, vram, nvidia),
	}
	chatterbox := setupVoiceOption{
		Module: "chatterbox", Device: config.TTSDeviceCUDA,
		Requirement: setupRequirement{Status: setupRequirementMet, Reason: "gpu", Bytes: setupChatterboxBytes},
		Memory:      setupVoiceBudget(chat, setupChatterboxVRAMMiB, vram, nvidia),
	}
	if qwen.Memory.Status == "insufficient" || qwen.Memory.Status == "unknown" {
		qwen.Requirement.Status, qwen.Requirement.Reason = setupRequirementPartial, "qwen_"+qwen.Memory.Status
	}
	if !nvidia || chatterbox.Memory.Status == "insufficient" {
		chatterbox.Device = config.TTSDeviceCPU
		chatterbox.Requirement.Status, chatterbox.Requirement.Reason = setupRequirementPartial, "cpu"
		chatterbox.Memory = setupVoiceBudget(chat, 0, vram, nvidia)
	} else if chatterbox.Memory.Status == "unknown" {
		chatterbox.Requirement.Status = setupRequirementPartial
	}
	if !available {
		qwen.Requirement = setupRequirement{Status: setupRequirementUnmet, Reason: "unavailable"}
		chatterbox.Requirement = qwen.Requirement
	} else if !nvidia {
		qwen.Requirement = setupRequirement{Status: setupRequirementUnmet, Reason: "no_nvidia"}
	}
	return []setupVoiceOption{qwen, chatterbox}
}
