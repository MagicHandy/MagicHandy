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

type setupInstallPlanRequest struct {
	Llama    *setupLlamaInstallRequest `json:"llama,omitempty"`
	Voice    *setupVoiceInstallRequest `json:"voice,omitempty"`
	Parakeet bool                      `json:"parakeet"`
}

type setupPlanTask struct {
	step setupJobStep
	run  func(context.Context, string) error
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
		"MSYS2 UCRT64 GCC, CMake, and Ninja for CPU builds",
		"Visual Studio C++ Build Tools, Windows SDK, and CUDA Toolkit for CUDA builds",
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
	backend, err := m.validateLlamaInstall(request)
	if err != nil {
		return setupJob{}, err
	}
	ctx, job, err := m.reserveJob("llama_runtime", "llama.cpp", backend, "Managed llama.cpp installation queued.")
	if err != nil {
		return setupJob{}, err
	}
	m.wg.Add(1)
	go m.runLlamaInstall(ctx, job.ID, backend)
	return job, nil
}

func (m *setupManager) validateLlamaInstall(request setupLlamaInstallRequest) (string, error) {
	request.Backend = strings.ToLower(strings.TrimSpace(request.Backend))
	if request.Backend == "" {
		request.Backend = "auto"
	}
	if !stringInSlice(request.Backend, setupLlamaRuntime.Backends) {
		return "", fmt.Errorf("unknown managed llama.cpp backend %q", request.Backend)
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return "", errors.New("managed llama.cpp installation is currently supported on Windows/amd64 only")
	}
	if request.Backend == "cuda" && !m.hasNVIDIA() {
		return "", errors.New("the CUDA backend requires a detected NVIDIA GPU; choose CPU instead")
	}
	return request.Backend, nil
}

func (m *setupManager) runLlamaInstall(ctx context.Context, id, backend string) {
	defer m.wg.Done()
	err := m.installLlama(ctx, id, backend)
	m.finishJob(ctx, id, err, "Managed llama.cpp")
}

func (m *setupManager) installLlama(ctx context.Context, id, backend string) error {
	m.updateJob(id, setupJobRunning, "Preparing managed llama.cpp prerequisites.", "")
	script, err := resolvePackagedSetupScript(m.executablePath, "install-llama-runtime.ps1")
	if err != nil {
		return err
	}
	powerShell, err := resolveSetupPowerShell()
	if err != nil {
		return err
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
		return context.Canceled
	}
	err = command.Run()
	m.detachCommand(id, command)
	if err == nil {
		status := llm.InspectManagedLlamaRuntime(m.dataDir)
		if !status.Installed || !status.Current {
			err = fmt.Errorf("runtime verification failed: %s", status.Message)
		}
	}
	return err
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
	err := m.installParakeet(ctx, id)
	m.finishJob(ctx, id, err, "Parakeet")
}

func (m *setupManager) installParakeet(ctx context.Context, id string) error {
	m.updateJob(id, setupJobRunning, "Preparing the verified Parakeet download.", "")
	script, err := resolvePackagedSetupScript(m.executablePath, "install-parakeet-module.ps1")
	if err != nil {
		return err
	}
	powerShell, err := resolveSetupPowerShell()
	if err != nil {
		return err
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
		return context.Canceled
	}
	err = command.Run()
	m.detachCommand(id, command)
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
	return err
}

func (m *setupManager) StartInstallPlan(request setupInstallPlanRequest) (setupJob, error) {
	tasks := make([]setupPlanTask, 0, 3)
	if request.Llama != nil {
		backend, err := m.validateLlamaInstall(*request.Llama)
		if err != nil {
			return setupJob{}, err
		}
		tasks = append(tasks, setupPlanTask{
			step: setupJobStep{ID: "llama_runtime", Label: "Managed llama.cpp", Status: setupJobQueued},
			run:  func(ctx context.Context, id string) error { return m.installLlama(ctx, id, backend) },
		})
	}
	if request.Voice != nil {
		module, normalized, err := m.validateVoiceInstall(*request.Voice)
		if err != nil {
			return setupJob{}, err
		}
		tasks = append(tasks, setupPlanTask{
			step: setupJobStep{ID: "voice_module", Label: module.Name, Status: setupJobQueued},
			run:  func(ctx context.Context, id string) error { return m.installVoice(ctx, id, module, normalized) },
		})
	}
	if request.Parakeet {
		if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
			return setupJob{}, errors.New("managed Parakeet installation is currently supported on Windows/amd64 only")
		}
		tasks = append(tasks, setupPlanTask{
			step: setupJobStep{ID: "parakeet", Label: "Parakeet speech input", Status: setupJobQueued},
			run:  m.installParakeet,
		})
	}
	if len(tasks) == 0 {
		return setupJob{}, errors.New("select at least one component to install")
	}

	ctx, job, err := m.reserveJob("install_plan", "selected_components", "", "Selected component installation queued.")
	if err != nil {
		return setupJob{}, err
	}
	steps := make([]setupJobStep, len(tasks))
	for index, task := range tasks {
		steps[index] = task.step
	}
	job = m.setJobSteps(job.ID, steps)
	m.wg.Add(1)
	go m.runInstallPlan(ctx, job.ID, tasks)
	return job, nil
}

func (m *setupManager) runInstallPlan(ctx context.Context, id string, tasks []setupPlanTask) {
	defer m.wg.Done()
	m.updateJob(id, setupJobRunning, "Installing selected components.", "")
	for index, task := range tasks {
		if err := ctx.Err(); err != nil {
			m.cancelPlanSteps(id, tasks[index:])
			m.finishJob(ctx, id, err, "Selected components")
			return
		}
		m.updateJobStep(id, task.step.ID, setupJobRunning, "Installing")
		m.updateJob(id, setupJobRunning, "Installing "+task.step.Label+".", "")
		if err := task.run(ctx, id); err != nil {
			if ctx.Err() != nil {
				m.cancelPlanSteps(id, tasks[index:])
				m.finishJob(ctx, id, err, "Selected components")
				return
			}
			m.updateJobStep(id, task.step.ID, setupJobFailed, err.Error())
			m.finishJob(ctx, id, err, task.step.Label)
			return
		}
		m.updateJobStep(id, task.step.ID, setupJobComplete, "Installed")
	}
	m.updateJob(id, setupJobComplete, "Selected components installed and verified.", "")
}

func (m *setupManager) cancelPlanSteps(id string, tasks []setupPlanTask) {
	for index, task := range tasks {
		message := "Not started"
		if index == 0 {
			message = "Cancelled"
		}
		m.updateJobStep(id, task.step.ID, setupJobCancelled, message)
	}
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

func (m *setupManager) detachCommand(id string, command *exec.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job != nil && m.job.ID == id && m.job.command == command {
		m.job.command = nil
	}
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

func (s *Server) handleSetupInstallPlan(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request setupInstallPlanRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, err := s.setup.StartInstallPlan(request)
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
