package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func getFailureReport(t *testing.T, server *Server, id string, status int) *setupFailureReport {
	t.Helper()
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/setup/install/"+url.PathEscape(id)+"/report", nil))
	if response.Code != status {
		t.Fatalf("report status %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("report may be cached")
	}
	if status != http.StatusOK {
		return nil
	}
	if response.Header().Get("Content-Disposition") != `attachment; filename="magichandy-install-failure.json"` || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("report is not a safe attachment")
	}
	var report setupFailureReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	return &report
}

func TestSetupFailureReportCapturesRedactedDiagnosticsAndSurvivesRestart(t *testing.T) {
	server := newTestServer(t)
	const secret = "private-installed-key/with+symbols"
	t.Setenv("MAGICHANDY_TEST_TOKEN", "private-environment-token")
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.OpenAITTSAPIKey = secret
		return settings
	})
	m := server.setup
	m.reportVersion = VersionInfo{Version: "report-test-version", Commit: "report-test-commit"}
	m.hardwareMu.Lock()
	m.hardware["gpu_name"], m.hardware["vram_mib"] = "Fixture NVIDIA GPU", "8192"
	m.hardwareMu.Unlock()
	ctx, job, err := m.reserveJob("voice_module", "chatterbox", "cuda", "queued")
	if err != nil {
		t.Fatal(err)
	}
	getFailureReport(t, server, job.ID, http.StatusNotFound)
	output := "Collecting torch==2.5.1\n" +
		"secret echoed without a label: " + secret + "\n" +
		"encoded " + url.QueryEscape(secret) + "\n" +
		"private-environment-token\n" +
		"private-environment-\n   token\n" +
		"Authorization: Bearer very-private-auth\n" +
		"Downloading https://user:pass@private.example/package?token=private-value\n" +
		"File \"C:\\Users\\private-person\\voice\\engine.py\", line 42\n" +
		"File \"/home/private-person/engine.py\", line 42\n" +
		"FullyQualifiedErrorId: failed 'C:\\Users\\private-\n   person\\private-project\\voice.py'. Try again.\n" +
		"ERROR: DLL load failed: missing native library\n"
	m.updateJob(job.ID, setupJobRunning, "", output)
	m.setJobSteps(job.ID, []setupJobStep{{ID: "voice_module", Label: "Chatterbox", Status: setupJobFailed, Message: "failed " + secret}})
	m.finishJob(ctx, job.ID, errors.New("Python voice runtime verification failed"), "Chatterbox Turbo")
	report := getFailureReport(t, server, job.ID, http.StatusOK)
	if report.Version != "report-test-version" || report.GPUName != "Fixture NVIDIA GPU" || report.VRAMMiB != "8192" || report.Installation.Status != setupJobFailed {
		t.Fatalf("incomplete report: %+v", report)
	}
	if !strings.Contains(report.Installation.Output, "DLL load failed") || !strings.Contains(report.Installation.Output, "torch==2.5.1") {
		t.Fatal("useful installer diagnostics were lost")
	}
	data, err := os.ReadFile(m.setupResultPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, url.QueryEscape(secret), "private-environment-", "very-private-auth", "private.example", "private-person", "private-project", "private-value"} {
		encoded, _ := json.Marshal(report)
		if strings.Contains(string(data), forbidden) || strings.Contains(string(encoded), forbidden) {
			t.Fatalf("sensitive detail was retained: %q", forbidden)
		}
	}
	m.Close()
	server.setup = newSetupManager(context.Background(), m.dataDir, "", m.logger, nil, nil)
	server.setup.reportVersion = VersionInfo{Version: "newer-app"}
	restored := getFailureReport(t, server, job.ID, http.StatusOK)
	if restored.Version != report.Version || restored.Installation.Output != report.Installation.Output {
		t.Fatal("restart lost the captured version or installer output")
	}
	if server.setup.Snapshot().Output != "" {
		t.Fatal("persisted report output leaked into status polling")
	}
	ctx, next, err := server.setup.reserveJob("parakeet", "parakeet", "cpu", "retry")
	if err != nil {
		t.Fatal(err)
	}
	getFailureReport(t, server, job.ID, http.StatusOK)
	getFailureReport(t, server, next.ID, http.StatusNotFound)
	server.setup.finishJob(ctx, next.ID, errors.New("download interrupted"), "Parakeet")
	getFailureReport(t, server, job.ID, http.StatusNotFound)
	getFailureReport(t, server, next.ID, http.StatusOK)
}

func TestSetupFailureReportBoundsNoisyOutputAndDropsPartialFirstLine(t *testing.T) {
	server := newTestServer(t)
	m := server.setup
	ctx, job, err := m.reserveJob("install_plan", "", "", "queued")
	if err != nil {
		t.Fatal(err)
	}
	output := strings.Repeat("sensitive-truncated-fragment", 1500) + "\n" + strings.Repeat("<noisy & output>\n", 1200) + "ERROR: disk full\n"
	m.updateJob(job.ID, setupJobRunning, "", output)
	m.finishJob(ctx, job.ID, errors.New("download failed"), "Selected components")
	report := getFailureReport(t, server, job.ID, http.StatusOK)
	if !report.Installation.OutputTruncated || !strings.HasSuffix(report.Installation.Output, "ERROR: disk full\n") || strings.Contains(report.Installation.Output, "sensitive-truncated-fragment") {
		t.Fatal("report tail was not safely bounded")
	}
	info, err := os.Stat(m.setupResultPath())
	if err != nil || info.Size() > setupResultFileLimit {
		t.Fatalf("report persistence exceeded limit: %v, %v", info, err)
	}
}

func TestSetupFailureReportSupportsLegacyFailuresAndRequiresAuthentication(t *testing.T) {
	server := newTestServer(t)
	server.setup.job = &setupJobState{setupJob: setupJob{ID: "legacy-failure", Status: setupJobFailed, Message: "exit status 1"}}
	report := getFailureReport(t, server, "legacy-failure", http.StatusOK)
	if !strings.Contains(report.Notes, "output is unavailable") {
		t.Fatal("legacy report implied that logs were retained")
	}
	server.auth.requireAuthentication()
	getFailureReport(t, server, "legacy-failure", http.StatusUnauthorized)
}

func TestSetupReportRedactsBeforeMetadataTruncation(t *testing.T) {
	m := &setupManager{reportSecrets: func() []string { return []string{"sensitive-secret-spanning-the-truncation-boundary"} }}
	report := m.newFailureReport(setupJob{ID: "job", Status: setupJobFailed, Message: strings.Repeat(".", 500) + "sensitive-secret-spanning-the-truncation-boundary"})
	if strings.Contains(report.Installation.Message, "sensitive") {
		t.Fatal("truncation exposed part of a credential")
	}
	redact := m.setupReportRedactor()
	for _, value := range []string{"hf_abcdefghijklmno", "sk-proj-abcdefghijklmno", `password = "private-password"`, `api_key: private-api-key`, `\\private-host\share\secret.txt`} {
		if redact(value) == value {
			t.Fatalf("expected redaction for %q", value)
		}
	}
}
