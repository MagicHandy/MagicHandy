package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type setupLlamaInstallRequest struct {
	Backend string `json:"backend"`
}

type setupLlamaRuntimeCatalog struct {
	Name              string   `json:"name"`
	Summary           string   `json:"summary"`
	License           string   `json:"license"`
	SourceVersion     string   `json:"source_version"`
	DiskEstimate      string   `json:"disk_estimate"`
	BuildDependencies []string `json:"build_dependencies"`
	Backends          []string `json:"backends"`
}

var setupLlamaRuntime = setupLlamaRuntimeCatalog{
	Name:          "Managed llama.cpp",
	Summary:       "A pinned app-owned local LLM runner. Building it avoids a separate Ollama runtime and model copy.",
	License:       "MIT",
	SourceVersion: "b9966 (c749cb0)",
	DiskEstimate:  "Several GiB of temporary compiler tooling and build files; CUDA requires the NVIDIA toolkit.",
	BuildDependencies: []string{
		"Git for Windows",
		"CMake",
		"Visual Studio C++ Build Tools and Windows SDK",
	},
	Backends: []string{"auto", "cpu", "cuda"},
}

type setupParakeetCatalog struct {
	Name          string `json:"name"`
	Summary       string `json:"summary"`
	RunnerLicense string `json:"runner_license"`
	ModelLicense  string `json:"model_license"`
	DownloadSize  string `json:"download_size"`
	RunnerVersion string `json:"runner_version"`
	Model         string `json:"model"`
}

var setupParakeet = setupParakeetCatalog{
	Name:          "Parakeet",
	Summary:       "Local speech recognition for microphone input. It runs outside the pure-Go core in an app-owned worker.",
	RunnerLicense: "MIT",
	ModelLicense:  "CC-BY-4.0",
	DownloadSize:  "About 646 MiB",
	RunnerVersion: "parakeet.cpp v0.4.0",
	Model:         "TDT 0.6B v3 Q4_K",
}

func (m *setupManager) StartLlamaInstall(request setupLlamaInstallRequest) (setupJob, error) {
	request.Backend = strings.ToLower(strings.TrimSpace(request.Backend))
	if request.Backend == "" {
		request.Backend = "auto"
	}
	if !stringInSlice(request.Backend, setupLlamaRuntime.Backends) {
		return setupJob{}, fmt.Errorf("unknown managed llama.cpp backend %q", request.Backend)
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return setupJob{}, errors.New("managed llama.cpp installation is currently supported on Windows/amd64 only")
	}
	if request.Backend == "cuda" && !m.hasNVIDIA() {
		return setupJob{}, errors.New("the CUDA backend requires a detected NVIDIA GPU; choose CPU instead")
	}
	ctx, job, err := m.reserveJob("llama_runtime", "llama.cpp", request.Backend, "Managed llama.cpp installation queued.")
	if err != nil {
		return setupJob{}, err
	}
	m.wg.Add(1)
	go m.runLlamaInstall(ctx, job.ID, request.Backend)
	return job, nil
}

func (m *setupManager) runLlamaInstall(ctx context.Context, id, backend string) {
	defer m.wg.Done()
	m.updateJob(id, setupJobRunning, "Preparing managed llama.cpp prerequisites.", "")
	script, err := resolvePackagedSetupScript(m.executablePath, "install-llama-runtime.ps1")
	if err != nil {
		m.finishJob(ctx, id, err, "Managed llama.cpp")
		return
	}
	powerShell, err := resolveSetupPowerShell()
	if err != nil {
		m.finishJob(ctx, id, err, "Managed llama.cpp")
		return
	}
	command := exec.CommandContext(ctx, powerShell, // #nosec G204 -- helper is app-discovered and arguments are app-owned paths or closed enums.
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script,
		"-DataDir", m.dataDir, "-Backend", backend, "-Yes",
	)
	configureSetupProcess(command)
	command.Cancel = func() error { return killSetupProcess(command) }
	command.WaitDelay = 15 * time.Second
	writer := &setupOutputWriter{manager: m, id: id}
	command.Stdout = writer
	command.Stderr = writer
	if !m.attachCommand(id, command) {
		m.finishJob(ctx, id, context.Canceled, "Managed llama.cpp")
		return
	}
	err = command.Run()
	if err == nil {
		status := llm.InspectManagedLlamaRuntime(m.dataDir)
		if !status.Installed || !status.Current {
			err = fmt.Errorf("runtime verification failed: %s", status.Message)
		}
	}
	m.finishJob(ctx, id, err, "Managed llama.cpp")
}

func (m *setupManager) StartParakeetInstall() (setupJob, error) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return setupJob{}, errors.New("managed Parakeet installation is currently supported on Windows/amd64 only")
	}
	ctx, job, err := m.reserveJob("parakeet", "parakeet", "cpu", "Parakeet installation queued.")
	if err != nil {
		return setupJob{}, err
	}
	m.wg.Add(1)
	go m.runParakeetInstall(ctx, job.ID)
	return job, nil
}

func (m *setupManager) runParakeetInstall(ctx context.Context, id string) {
	defer m.wg.Done()
	m.updateJob(id, setupJobRunning, "Preparing the verified Parakeet download.", "")
	script, err := resolvePackagedSetupScript(m.executablePath, "install-parakeet-module.ps1")
	if err != nil {
		m.finishJob(ctx, id, err, "Parakeet")
		return
	}
	powerShell, err := resolveSetupPowerShell()
	if err != nil {
		m.finishJob(ctx, id, err, "Parakeet")
		return
	}
	command := exec.CommandContext(ctx, powerShell, // #nosec G204 -- helper is app-discovered and arguments are app-owned paths.
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script,
		"-DataDir", m.dataDir, "-Yes",
	)
	configureSetupProcess(command)
	command.Cancel = func() error { return killSetupProcess(command) }
	command.WaitDelay = 15 * time.Second
	writer := &setupOutputWriter{manager: m, id: id}
	command.Stdout = writer
	command.Stderr = writer
	if !m.attachCommand(id, command) {
		m.finishJob(ctx, id, context.Canceled, "Parakeet")
		return
	}
	err = command.Run()
	result := setupParakeetInstallResult{
		ServerPath: filepath.Join(m.dataDir, "voice", "parakeet", "runner", "parakeet-server.exe"),
		ModelPath:  filepath.Join(m.dataDir, "voice", "parakeet", "tdt-0.6b-v3-q4_k.gguf"),
	}
	if err == nil {
		err = verifyRegularFiles(result.ServerPath, result.ModelPath)
	}
	if err == nil && ctx.Err() == nil && m.onParakeet != nil {
		err = m.onParakeet(context.WithoutCancel(ctx), result)
	}
	m.finishJob(ctx, id, err, "Parakeet")
}

func (m *setupManager) attachCommand(id string, command *exec.Cmd) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.job == nil || m.job.ID != id {
		return false
	}
	m.job.command = command
	return true
}

func verifyRegularFiles(paths ...string) error {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("installed setup asset is missing: %s", filepath.Base(path))
		}
	}
	return nil
}

func (s *Server) handleSetupLlamaInstall(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request setupLlamaInstallRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, err := s.setup.StartLlamaInstall(request)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"installation": job})
}

func (s *Server) handleSetupParakeetInstall(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	job, err := s.setup.StartParakeetInstall()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"installation": job})
}

func (s *Server) applyInstalledParakeet(ctx context.Context, result setupParakeetInstallResult) error {
	_, _, saveErr, runtimeErr := s.updateSettingsAndRuntime(ctx, func(current config.Settings) (config.Settings, error) {
		current.Voice.ASRProvider = config.VoiceASRProviderParakeet
		current.Voice.ParakeetSource = config.ParakeetSourceApp
		current.Voice.ParakeetServerPath = result.ServerPath
		current.Voice.ParakeetModelPath = result.ModelPath
		current.Voice.ParakeetServerPort = config.DefaultParakeetServerPort
		return current, nil
	})
	return errors.Join(saveErr, runtimeErr)
}
