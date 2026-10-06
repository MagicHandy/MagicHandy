package httpapi

import (
	"context"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func (s *Server) assessSelectedSetupChat(ctx context.Context, assessment *setupAssessment, settings config.LLMSettings, nvidia bool, vram int) (setupRequirement, bool) {
	selected, err := s.models.Model(ctx, settings.Model)
	if err != nil || selected.State != "ready" {
		return setupRequirement{}, false
	}
	assessment.ModelInstalled, assessment.ModelName = selected.ID, selected.DisplayName
	requirement := setupRequirement{Status: setupRequirementPartial, Reason: "model_unknown"}
	for _, model := range llm.CatalogModels() {
		if !strings.EqualFold(model.SHA256, selected.SHA256) || settings.LlamaCPPContextSize > config.DefaultLlamaCPPContextSize {
			continue
		}
		assessment.ModelID = model.ID
		requirement.VRAMMiB, requirement.MinVRAMMiB = model.VRAMMiB, model.MinVRAMMiB
		switch {
		case nvidia && vram >= model.MinVRAMMiB:
			requirement.Status, requirement.Reason = setupRequirementMet, "gpu_fits"
		case nvidia && vram <= 0:
			requirement.Reason = "vram_unknown"
		default:
			requirement.Reason = "selected_vram_below"
		}
		break
	}
	requirement.Bytes = s.setupRuntimeBytes(assessment.RuntimeBackend)
	return requirement, true
}
