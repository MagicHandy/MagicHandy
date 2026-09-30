package httpapi

import (
	"context"
	"runtime"
	"strconv"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Requirement verdicts for one feature on the machine running setup.
const (
	setupRequirementMet     = "met"
	setupRequirementPartial = "partial"
	setupRequirementUnmet   = "unmet"
)

// Disk estimates for what Easy Setup installs. They are generous so the
// check errs toward warning before a long install rather than after it.
const (
	setupCUDARuntimeBytes    = 1280 << 20
	setupCPURuntimeBytes     = 64 << 20
	setupChatterboxBytes     = 6 << 30
	setupParakeetBytes       = 800 << 20
	setupModelDownloadMargin = 512 << 20
)

// setupRequirement is one feature's verdict. Reason is a stable code the
// browser words in the user's language; the numbers explain it.
type setupRequirement struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
	// Bytes is the free disk space the feature still needs.
	Bytes uint64 `json:"bytes"`
	// VRAMMiB and MinVRAMMiB describe the chat model's graphics memory.
	VRAMMiB    int `json:"vram_mib,omitempty"`
	MinVRAMMiB int `json:"min_vram_mib,omitempty"`
}

// setupAssessment is what Easy Setup shows and installs: a verdict for chat,
// voice output and voice input, and the choices it made for this machine.
type setupAssessment struct {
	FreeDiskBytes  uint64           `json:"free_disk_bytes"`
	Chat           setupRequirement `json:"chat"`
	VoiceOutput    setupRequirement `json:"voice_output"`
	VoiceInput     setupRequirement `json:"voice_input"`
	ModelID        string           `json:"model_id,omitempty"`
	ModelInstalled string           `json:"model_installed_id,omitempty"`
	RuntimeBackend string           `json:"runtime_backend"`
	VoiceModule    string           `json:"voice_module"`
	VoiceDevice    string           `json:"voice_device"`
}

// assessSetup judges this machine for Easy Setup. Local chat needs an NVIDIA
// GPU that holds a curated model: CPU inference is too slow for chat, so a
// machine without one is told so rather than offered a CPU model.
func (s *Server) assessSetup(ctx context.Context, hardware map[string]any, helpers map[string]bool, dataDir string, parakeetInstalled bool) setupAssessment {
	nvidia, _ := hardware["nvidia"].(bool)
	vram := 0
	if text, ok := hardware["vram_mib"].(string); ok {
		vram, _ = strconv.Atoi(text)
	}
	windows := runtime.GOOS == "windows" && runtime.GOARCH == "amd64"
	assessment := setupAssessment{RuntimeBackend: config.TTSDeviceCPU, VoiceModule: "chatterbox", VoiceDevice: config.TTSDeviceCPU}
	if nvidia {
		assessment.RuntimeBackend, assessment.VoiceDevice = config.TTSDeviceCUDA, config.TTSDeviceCUDA
	}
	if free, err := setupAvailableBytes(dataDir); err == nil {
		assessment.FreeDiskBytes = free
	}
	assessment.Chat = s.assessSetupChat(ctx, &assessment, windows, nvidia, vram)

	switch {
	case !windows || !helpers["voice"]:
		assessment.VoiceOutput = setupRequirement{Status: setupRequirementUnmet, Reason: "unavailable"}
	case nvidia:
		assessment.VoiceOutput = setupRequirement{Status: setupRequirementMet, Reason: "gpu", Bytes: setupChatterboxBytes}
	default:
		assessment.VoiceOutput = setupRequirement{Status: setupRequirementPartial, Reason: "cpu", Bytes: setupChatterboxBytes}
	}
	switch {
	case !windows || !helpers["parakeet"]:
		assessment.VoiceInput = setupRequirement{Status: setupRequirementUnmet, Reason: "unavailable"}
	case parakeetInstalled:
		assessment.VoiceInput = setupRequirement{Status: setupRequirementMet, Reason: "installed"}
	default:
		assessment.VoiceInput = setupRequirement{Status: setupRequirementMet, Reason: "cpu", Bytes: setupParakeetBytes}
	}
	return assessment
}

func (s *Server) assessSetupChat(ctx context.Context, assessment *setupAssessment, windows, nvidia bool, vram int) setupRequirement {
	switch {
	case !windows:
		return setupRequirement{Status: setupRequirementUnmet, Reason: "platform"}
	case !nvidia:
		return setupRequirement{Status: setupRequirementUnmet, Reason: "no_nvidia"}
	}
	models := llm.CatalogModels()
	hardware := llm.CatalogHardware{NVIDIA: true, VRAMMiB: vram}
	var chosen *llm.CatalogModel
	smallest := 0
	for index := range models {
		model := models[index]
		if smallest == 0 || model.MinVRAMMiB < smallest {
			smallest = model.MinVRAMMiB
		}
		fit := llm.CatalogFit(model, hardware)
		if chosen == nil && (fit == llm.CatalogFitRecommended || (fit == llm.CatalogFitUnknownVRAM && model.Default)) {
			chosen = &models[index]
		}
	}
	if chosen == nil {
		return setupRequirement{Status: setupRequirementUnmet, Reason: "vram_below", MinVRAMMiB: smallest}
	}
	assessment.ModelID = chosen.ID
	requirement := setupRequirement{Status: setupRequirementMet, Reason: "gpu_fits", VRAMMiB: chosen.VRAMMiB, MinVRAMMiB: chosen.MinVRAMMiB}
	if vram <= 0 {
		requirement.Status, requirement.Reason = setupRequirementPartial, "vram_unknown"
	}
	if runtimeStatus := s.managedLLM.Snapshot().Runtime; !runtimeStatus.Installed || !runtimeStatus.Current {
		requirement.Bytes = setupCPURuntimeBytes
		if assessment.RuntimeBackend == config.TTSDeviceCUDA {
			requirement.Bytes = setupCUDARuntimeBytes
		}
	}
	installed, err := s.models.CatalogModelInstalled(ctx, *chosen)
	if err == nil && installed != "" {
		assessment.ModelInstalled = installed
		return requirement
	}
	if remaining := chosen.SizeBytes - s.models.CatalogPartialBytes(*chosen); remaining > 0 {
		requirement.Bytes += uint64(remaining) + setupModelDownloadMargin //nolint:gosec // remaining is positive.
	}
	return requirement
}
