package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"strings"
)

// One bounded, redacted failure survives retries and restarts. Its retained
// terminal tail stays out of the persisted job and frequent status responses.
type setupFailureReport struct {
	SchemaVersion int      `json:"schema_version"`
	Version       string   `json:"app_version"`
	Commit        string   `json:"app_commit"`
	Platform      string   `json:"platform"`
	GoVersion     string   `json:"go_version"`
	CPUThreads    int      `json:"cpu_threads"`
	GPUName       string   `json:"gpu_name,omitempty"`
	VRAMMiB       string   `json:"vram_mib,omitempty"`
	Installation  setupJob `json:"installation"`
	Notes         string   `json:"notes"`
}

func (s *Server) configureSetupManager() {
	s.setup.prepareParakeet = s.prepareParakeetRepair
	s.setup.restoreParakeet = s.restoreParakeetAfterRepair
	s.setup.reportVersion = s.version
	s.setup.reportSecrets = func() []string {
		settings, _ := s.store.Snapshot()
		return []string{settings.Device.HandyConnectionKey, settings.Voice.ElevenLabsAPIKey, settings.Voice.OpenAITTSAPIKey}
	}
}

func (m *setupManager) newFailureReport(job setupJob) *setupFailureReport {
	redact := m.setupReportRedactor()
	output := job.Output
	if job.OutputTruncated {
		// The bounded live log may start halfway through a credential or path.
		_, output, _ = strings.Cut(output, "\n")
	}
	job.Output = ""
	job = sanitizePersistedSetupJob(redactReportJob(cloneSetupJob(job), redact))
	job.Output = redact(output)
	if len(job.Output) > 16*1024 {
		job.Output = completeSetupTail(job.Output, 16*1024)
		job.OutputTruncated = true
	}
	report := &setupFailureReport{
		SchemaVersion: 1, Version: m.reportVersion.Version, Commit: m.reportVersion.Commit,
		Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), CPUThreads: runtime.NumCPU(),
		Installation: job,
		Notes:        "Share this report with the MagicHandy developer to help diagnose the installation failure. Common credentials, URLs and local paths are redacted. Review before sharing. No settings, chat history, audio or environment dump is included.",
	}
	if job.Output == "" {
		report.Notes += " Installer output is unavailable; older app versions did not retain it after a restart."
	}
	m.hardwareMu.Lock()
	defer m.hardwareMu.Unlock()
	if gpu, ok := m.hardware["gpu_name"].(string); ok {
		report.GPUName = sanitizeSetupText(redact(gpu))
	}
	if vram, ok := m.hardware["vram_mib"].(string); ok {
		report.VRAMMiB = sanitizeSetupText(redact(vram))
	}
	return report
}

func completeSetupTail(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	_, tail, _ := strings.Cut(value[len(value)-limit:], "\n")
	return tail
}

func (s *Server) handleSetupFailureReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	m := s.setup
	m.reportMu.Lock()
	var report *setupFailureReport
	if m.lastFailureReport != nil && m.lastFailureReport.Installation.ID == id {
		cloned := *m.lastFailureReport
		cloned.Installation = cloneSetupJob(cloned.Installation)
		report = &cloned
	}
	m.reportMu.Unlock()
	if report == nil {
		job := m.Snapshot()
		if job == nil || job.ID != id || job.Status != setupJobFailed {
			writeError(w, http.StatusNotFound, errors.New("the requested installation failure report is no longer available"))
			return
		}
		report = m.newFailureReport(*job)
	}
	// Reapply redaction on download so old reports also respect current keys.
	// Redact field values rather than serialized JSON to preserve valid JSON.
	redact := m.setupReportRedactor()
	report.Installation = redactReportJob(report.Installation, redact)
	report.Version, report.Commit = redact(report.Version), redact(report.Commit)
	report.GPUName, report.VRAMMiB = redact(report.GPUName), redact(report.VRAMMiB)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("the installation failure report could not be generated"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="magichandy-install-failure.json"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(data, '\n'))
}

func redactReportJob(job setupJob, redact func(string) string) setupJob {
	// The opaque app-generated job ID is also the lookup key for this report.
	job.Kind, job.Module, job.Device = redact(job.Kind), redact(job.Module), redact(job.Device)
	job.Message, job.Output = redact(job.Message), redact(job.Output)
	for index := range job.Steps {
		job.Steps[index].ID = redact(job.Steps[index].ID)
		job.Steps[index].Label = redact(job.Steps[index].Label)
		job.Steps[index].Message = redact(job.Steps[index].Message)
	}
	return job
}
