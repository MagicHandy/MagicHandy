package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Leave room beside the model for the store's metadata and ordinary disk use.
const modelDownloadSpaceSlack = int64(512 << 20)

type catalogModelView struct {
	llm.CatalogModel
	Fit              string `json:"fit"`
	InstalledModelID string `json:"installed_model_id,omitempty"`
	PartialBytes     int64  `json:"partial_bytes,omitempty"`
}

type catalogHardwareView struct {
	NVIDIA  bool   `json:"nvidia"`
	GPUName string `json:"gpu_name,omitempty"`
	VRAMMiB int    `json:"vram_mib,omitempty"`
}

func (s *Server) handleLLMCatalog(w http.ResponseWriter, r *http.Request) {
	hardware := s.catalogHardware()
	models := llm.CatalogModels()
	views := make([]catalogModelView, 0, len(models))
	for _, model := range models {
		installed, err := s.models.CatalogModelInstalled(r.Context(), model)
		if err != nil {
			s.writeModelManagerStorageError(w, err)
			return
		}
		views = append(views, catalogModelView{
			CatalogModel:     model,
			Fit:              llm.CatalogFit(model, llm.CatalogHardware{NVIDIA: hardware.NVIDIA, VRAMMiB: hardware.VRAMMiB}),
			InstalledModelID: installed,
			PartialBytes:     s.models.CatalogPartialBytes(model),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": views, "hardware": hardware})
}

func (s *Server) handleCatalogDownload(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request struct {
		ID string `json:"id"`
	}
	if !decodeModelManagerRequest(w, r, &request) {
		return
	}
	model, ok := llm.FindCatalogModel(request.ID)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown catalog model %q", strings.TrimSpace(request.ID)))
		return
	}
	if err := s.checkModelDownloadSpace(model); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	job, err := s.models.StartCatalogDownload(r.Context(), model)
	if err != nil {
		if errors.Is(err, llm.ErrModelInventoryUnavailable) {
			s.writeModelManagerStorageError(w, err)
			return
		}
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"import": job})
}

func (s *Server) catalogHardware() catalogHardwareView {
	snapshot := s.setup.HardwareSnapshot()
	view := catalogHardwareView{}
	view.NVIDIA, _ = snapshot["nvidia"].(bool)
	view.GPUName, _ = snapshot["gpu_name"].(string)
	if text, ok := snapshot["vram_mib"].(string); ok {
		if value, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && value > 0 {
			view.VRAMMiB = value
		}
	}
	return view
}

// checkModelDownloadSpace refuses a download that cannot fit, before any bytes
// move. A resumable partial already on disk counts toward the model.
func (s *Server) checkModelDownloadSpace(model llm.CatalogModel) error {
	available, err := setupAvailableBytes(s.models.DownloadsDir())
	if err != nil {
		return fmt.Errorf("check free space for %s: %w", model.DisplayName, err)
	}
	required := model.SizeBytes - s.models.CatalogPartialBytes(model) + modelDownloadSpaceSlack
	if required > 0 && available < uint64(required) { //nolint:gosec // The positive guard makes this conversion safe.
		return fmt.Errorf(
			"not enough free disk space for %s: %.1f GiB available, about %.1f GiB needed",
			model.DisplayName, float64(available)/(1<<30), float64(required)/(1<<30),
		)
	}
	return nil
}

// downloadCatalogModelForSetup runs one catalog download inside the setup
// plan, mirrors its progress onto the setup job, and selects the model once
// it is in the store. Cancelling the plan cancels the download and keeps its
// partial for resume.
func (s *Server) downloadCatalogModelForSetup(ctx context.Context, setupJobID string, model llm.CatalogModel) error {
	if err := s.checkModelDownloadSpace(model); err != nil {
		return err
	}
	start := fmt.Sprintf("Downloading %s (%.2f GiB) from %s.", model.DisplayName, float64(model.SizeBytes)/(1<<30), downloadHost(model.DownloadURL))
	if saved := s.models.CatalogPartialBytes(model); saved > 0 {
		start += fmt.Sprintf(" Resuming after %.2f GiB already saved.", float64(saved)/(1<<30))
	}
	s.setup.updateJob(setupJobID, setupJobRunning, "", start+"\n")
	job, err := s.models.StartCatalogDownload(ctx, model)
	if err != nil {
		return err
	}
	label := "Downloading " + model.DisplayName
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := s.models.Import(job.ID)
		if err != nil {
			return err
		}
		switch current.Status {
		case llm.ImportStatusComplete:
			s.setup.updateJobProgress(setupJobID, label, current.TotalBytes, current.TotalBytes)
			s.setup.updateJob(setupJobID, setupJobRunning, "", fmt.Sprintf("Verified SHA-256 %s and added %s to the model store.\n", model.SHA256, model.DisplayName))
			return s.selectManagedModel(ctx, current.ModelID)
		case llm.ImportStatusFailed:
			return errors.New(current.Error)
		case llm.ImportStatusCancelled:
			return context.Canceled
		}
		s.setup.updateJobProgress(setupJobID, label, current.BytesCopied, current.TotalBytes)
		select {
		case <-ctx.Done():
			_, _ = s.models.CancelImport(job.ID)
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// selectManagedModel makes a newly downloaded model the active chat model.
// A runner that cannot start yet does not undo the download; the model stays
// selected and Settings reports the runtime state.
func (s *Server) selectManagedModel(ctx context.Context, modelID string) error {
	_, _, saveErr, runtimeErr := s.updateSettingsAndRuntime(context.WithoutCancel(ctx), func(current config.Settings) (config.Settings, error) {
		current.LLM.Provider = config.LLMProviderLlamaCPP
		current.LLM.LlamaCPPMode = config.LlamaCPPModeManaged
		current.LLM.Model = modelID
		return current, nil
	})
	if saveErr != nil {
		return fmt.Errorf("the model downloaded but could not be selected: %w", saveErr)
	}
	if runtimeErr != nil {
		s.logger.Warn("downloaded model selected; runtime not ready yet", "model", modelID, "error", runtimeErr)
	}
	return nil
}

func downloadHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "the model registry"
	}
	return parsed.Host
}
