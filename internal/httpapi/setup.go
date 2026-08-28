package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const setupJobOutputLimit = 24 * 1024

const (
	setupJobQueued    = "queued"
	setupJobRunning   = "running"
	setupJobComplete  = "complete"
	setupJobFailed    = "failed"
	setupJobCancelled = "cancelled"
)

type setupVoiceModule struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Provider             string   `json:"provider"`
	Summary              string   `json:"summary"`
	License              string   `json:"license"`
	Model                string   `json:"model"`
	ModelLicense         string   `json:"model_license"`
	PythonVersion        string   `json:"python_version"`
	DiskEstimate         string   `json:"disk_estimate"`
	SupportedDevices     []string `json:"supported_devices"`
	RecommendedForNVIDIA bool     `json:"recommended_for_nvidia"`
	ReferenceRequirement string   `json:"reference_requirement"`
	SourceURL            string   `json:"source_url"`
	SourceRevision       string   `json:"source_revision"`
	Port                 int      `json:"port"`
}

var setupVoiceModules = []setupVoiceModule{
	{
		ID: "faster-qwen3-tts", Name: "Faster Qwen3-TTS", Provider: config.VoiceTTSProviderFasterQwen,
		Summary: "Fast local voice cloning for a compatible NVIDIA GPU.",
		License: "MIT", Model: config.DefaultFasterQwenModel, ModelLicense: "Apache-2.0",
		PythonVersion: "3.11", DiskEstimate: "Several GiB for Python, CUDA PyTorch, dependencies, and model cache.",
		SupportedDevices: []string{config.TTSDeviceCUDA}, RecommendedForNVIDIA: true,
		ReferenceRequirement: "Add a reference WAV and its exact transcript in Voice settings after installation.",
		SourceURL:            "https://github.com/andimarafioti/faster-qwen3-tts.git",
		SourceRevision:       "a70afc0f81f7f5f8801c3227968f1102f43f211c", Port: 8991,
	},
	{
		ID: "chatterbox", Name: "Chatterbox Turbo", Provider: config.VoiceTTSProviderChatterbox,
		Summary: "Local voice cloning with CPU fallback and broad NVIDIA support.",
		License: "MIT", Model: "ResembleAI/chatterbox-turbo", ModelLicense: "MIT",
		PythonVersion: "3.10", DiskEstimate: "Several GiB for Python, PyTorch, dependencies, and model cache.",
		SupportedDevices:     []string{config.TTSDeviceCPU, config.TTSDeviceCUDA},
		ReferenceRequirement: "The included Emily voice works immediately; a local WAV can be selected later.",
		SourceURL:            "https://github.com/devnen/Chatterbox-TTS-Server.git",
		SourceRevision:       "915ae289340e10c6047f27f47e22eae9bf350c32", Port: 8992,
	},
}

type setupVoiceInstallRequest struct {
	Module     string `json:"module"`
	Device     string `json:"device"`
	AutoLaunch bool   `json:"auto_launch"`
}

type setupPreferencesRequest struct {
	UILocale      string              `json:"ui_locale"`
	ChatLocale    string              `json:"chat_locale"`
	DeviceOwner   string              `json:"device_owner"`
	ConnectionKey *string             `json:"connection_key,omitempty"`
	LLM           *config.LLMSettings `json:"llm,omitempty"`
}

type setupCompleteRequest struct {
	AllowUnreadyLLM bool `json:"allow_unready_llm"`
}

type setupVoiceInstallResult struct {
	Module     setupVoiceModule
	Device     string
	AutoLaunch bool
	Root       string
}

type setupParakeetInstallResult struct {
	ServerPath string
	ModelPath  string
}

type setupJob struct {
	ID             string         `json:"id"`
	Kind           string         `json:"kind"`
	Module         string         `json:"module"`
	Device         string         `json:"device"`
	Status         string         `json:"status"`
	Message        string         `json:"message"`
	Output         string         `json:"output,omitempty"`
	Steps          []setupJobStep `json:"steps,omitempty"`
	CompletedSteps int            `json:"completed_steps,omitempty"`
	TotalSteps     int            `json:"total_steps,omitempty"`
	StartedAt      string         `json:"started_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type setupJobStep struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type setupJobState struct {
	setupJob
	cancel  context.CancelFunc
	command *exec.Cmd
}

type setupManager struct {
	ctx            context.Context
	cancel         context.CancelFunc
	dataDir        string
	executablePath string
	logger         *slog.Logger
	onInstalled    func(context.Context, setupVoiceInstallResult) error
	onParakeet     func(context.Context, setupParakeetInstallResult) error
	hardwareMu     sync.Mutex
	hardwareOnce   sync.Once
	hardware       map[string]any

	mu     sync.Mutex
	job    *setupJobState
	closed bool
	wg     sync.WaitGroup
}

func newSetupManager(
	parent context.Context,
	dataDir string,
	executablePath string,
	logger *slog.Logger,
	onInstalled func(context.Context, setupVoiceInstallResult) error,
	onParakeet func(context.Context, setupParakeetInstallResult) error,
) *setupManager {
	ctx, cancel := context.WithCancel(parent)
	return &setupManager{
		ctx: ctx, cancel: cancel, dataDir: dataDir, executablePath: executablePath,
		logger: logger, onInstalled: onInstalled, onParakeet: onParakeet,
		hardware: map[string]any{"platform": runtime.GOOS + "/" + runtime.GOARCH, "nvidia": false, "cuda": false},
	}
}

func (m *setupManager) HardwareSnapshot() map[string]any {
	m.hardwareOnce.Do(func() {
		hardware := setupHardwareSnapshot()
		m.hardwareMu.Lock()
		m.hardware = hardware
		m.hardwareMu.Unlock()
	})
	m.hardwareMu.Lock()
	defer m.hardwareMu.Unlock()
	snapshot := make(map[string]any, len(m.hardware))
	for key, value := range m.hardware {
		snapshot[key] = value
	}
	return snapshot
}

func (m *setupManager) hasNVIDIA() bool {
	hardware := m.HardwareSnapshot()
	available, _ := hardware["nvidia"].(bool)
	return available
}

func (m *setupManager) Snapshot() *setupJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil {
		return nil
	}
	job := cloneSetupJob(m.job.setupJob)
	return &job
}

func cloneSetupJob(job setupJob) setupJob {
	job.Steps = append([]setupJobStep(nil), job.Steps...)
	return job
}

func (m *setupManager) StartVoiceInstall(request setupVoiceInstallRequest) (setupJob, error) {
	module, normalized, err := m.validateVoiceInstall(request)
	if err != nil {
		return setupJob{}, err
	}

	ctx, job, err := m.reserveJob("voice_module", module.ID, normalized.Device, "Voice module installation queued.")
	if err != nil {
		return setupJob{}, err
	}
	m.wg.Add(1)
	go m.runVoiceInstall(ctx, job.ID, module, normalized)
	return job, nil
}

func (m *setupManager) validateVoiceInstall(request setupVoiceInstallRequest) (setupVoiceModule, setupVoiceInstallRequest, error) {
	module, err := findSetupVoiceModule(request.Module)
	if err != nil {
		return setupVoiceModule{}, request, err
	}
	request.Device = strings.ToLower(strings.TrimSpace(request.Device))
	if request.Device == "" || request.Device == config.TTSDeviceAuto {
		if module.ID == "faster-qwen3-tts" {
			request.Device = config.TTSDeviceCUDA
		} else if m.hasNVIDIA() {
			request.Device = config.TTSDeviceCUDA
		} else {
			request.Device = config.TTSDeviceCPU
		}
	}
	if !stringInSlice(request.Device, module.SupportedDevices) {
		return setupVoiceModule{}, request, fmt.Errorf("%s does not support device %q", module.Name, request.Device)
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return setupVoiceModule{}, request, errors.New("managed voice module installation is currently supported on Windows/amd64 only")
	}
	if module.ID == "faster-qwen3-tts" && !m.hasNVIDIA() {
		return setupVoiceModule{}, request, errors.New("an NVIDIA GPU is required for Faster Qwen3-TTS; choose Chatterbox CPU instead")
	}
	return module, request, nil
}

func (m *setupManager) reserveJob(kind, module, device, message string) (context.Context, setupJob, error) {
	id, err := newSetupJobID()
	if err != nil {
		return nil, setupJob{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, setupJob{}, errors.New("setup manager is closed")
	}
	if m.job != nil && (m.job.Status == setupJobQueued || m.job.Status == setupJobRunning) {
		return nil, setupJob{}, errors.New("another setup installation is already running")
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.job = &setupJobState{
		setupJob: setupJob{
			ID: id, Kind: kind, Module: module, Device: device,
			Status: setupJobQueued, Message: message, StartedAt: now, UpdatedAt: now,
		},
		cancel: cancel,
	}
	return ctx, m.job.setupJob, nil
}

func (m *setupManager) Cancel() (setupJob, error) {
	m.mu.Lock()
	if m.job == nil || (m.job.Status != setupJobQueued && m.job.Status != setupJobRunning) {
		m.mu.Unlock()
		return setupJob{}, errors.New("no setup installation is running")
	}
	m.job.cancel()
	command := m.job.command
	job := cloneSetupJob(m.job.setupJob)
	m.mu.Unlock()
	if command != nil {
		_ = killSetupProcess(command)
	}
	return job, nil
}

func (m *setupManager) Close() {
	m.mu.Lock()
	var command *exec.Cmd
	if !m.closed {
		m.closed = true
		m.cancel()
		if m.job != nil && m.job.command != nil {
			command = m.job.command
		}
	}
	m.mu.Unlock()
	if command != nil {
		_ = killSetupProcess(command)
	}
	m.wg.Wait()
}

func (m *setupManager) runVoiceInstall(
	ctx context.Context,
	id string,
	module setupVoiceModule,
	request setupVoiceInstallRequest,
) {
	defer m.wg.Done()
	err := m.installVoice(ctx, id, module, request)
	m.finishJob(ctx, id, err, "Voice module")
}

func (m *setupManager) installVoice(
	ctx context.Context,
	id string,
	module setupVoiceModule,
	request setupVoiceInstallRequest,
) error {
	m.updateJob(id, setupJobRunning, "Preparing the managed voice installer.", "")

	script, err := resolveTTSInstallerScript(m.executablePath)
	if err != nil {
		return err
	}
	powerShell, err := resolveSetupPowerShell()
	if err != nil {
		return err
	}
	folder := "faster-qwen3-tts"
	if module.ID == "chatterbox" {
		folder = "chatterbox-tts"
	}
	root := filepath.Join(m.dataDir, "voice", folder)
	arguments := []string{
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script,
		"-Module", module.ID, "-DataDir", m.dataDir, "-InstallRoot", root,
		"-Device", request.Device, "-Port", fmt.Sprint(module.Port),
		"-Yes", "-SkipAppConfiguration",
	}
	if request.AutoLaunch {
		arguments = append(arguments, "-AutoLaunch")
	}
	command := exec.CommandContext(ctx, powerShell, arguments...) // #nosec G204 -- executable and script are app-discovered; arguments are closed enums and app-owned paths.
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
		err = verifyInstalledVoiceModule(root, module)
	}
	if err == nil {
		worker := resolveFirstPartyWorkerBinary("", m.executablePath, m.dataDir, "voice-openai-tts-worker")
		if !isRegularFile(worker) {
			err = errors.New("voice module install is incomplete: the bundled OpenAI-compatible worker is missing")
		}
	}
	if err == nil && ctx.Err() == nil && m.onInstalled != nil {
		err = m.onInstalled(context.WithoutCancel(ctx), setupVoiceInstallResult{
			Module: module, Device: request.Device, AutoLaunch: request.AutoLaunch, Root: root,
		})
	}
	return err
}

func (m *setupManager) setJobSteps(id string, steps []setupJobStep) setupJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil || m.job.ID != id {
		return setupJob{}
	}
	m.job.Steps = append([]setupJobStep(nil), steps...)
	m.job.TotalSteps = len(steps)
	m.job.CompletedSteps = 0
	return cloneSetupJob(m.job.setupJob)
}

func (m *setupManager) updateJobStep(id, stepID, status, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil || m.job.ID != id {
		return
	}
	for index := range m.job.Steps {
		if m.job.Steps[index].ID != stepID {
			continue
		}
		m.job.Steps[index].Status = status
		m.job.Steps[index].Message = message
		break
	}
	completed := 0
	for _, step := range m.job.Steps {
		if step.Status == setupJobComplete {
			completed++
		}
	}
	m.job.CompletedSteps = completed
	m.job.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

func (m *setupManager) finishJob(ctx context.Context, id string, err error, subject string) {
	switch {
	case ctx.Err() != nil:
		m.updateJob(id, setupJobCancelled, subject+" installation cancelled. Partial downloads were kept for resume.", "")
	case err != nil:
		m.logger.Error("setup installation failed", "kind", subject, "error", err)
		m.updateJob(id, setupJobFailed, fmt.Sprintf("%s installation failed: %v", subject, err), "")
	default:
		message := subject + " installed."
		if subject == "Voice module" || subject == "Parakeet" {
			message += " Voice remains off until you enable and start it."
		}
		m.updateJob(id, setupJobComplete, message, "")
	}
}

func (m *setupManager) updateJob(id, status, message, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil || m.job.ID != id {
		return
	}
	m.job.Status = status
	if message != "" {
		m.job.Message = message
	}
	if output != "" {
		m.job.Output = trimSetupOutput(m.job.Output + output)
		if line := lastSetupOutputLine(output); line != "" {
			m.job.Message = line
		}
	}
	m.job.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
}

type setupOutputWriter struct {
	manager *setupManager
	id      string
}

func (w *setupOutputWriter) Write(data []byte) (int, error) {
	w.manager.updateJob(w.id, setupJobRunning, "", string(data))
	return len(data), nil
}

func trimSetupOutput(output string) string {
	if len(output) <= setupJobOutputLimit {
		return output
	}
	start := len(output) - setupJobOutputLimit
	for start < len(output) && !utf8.RuneStart(output[start]) {
		start++
	}
	return output[start:]
}

func lastSetupOutputLine(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r", ""), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 240 {
			return string(runes[:240])
		}
		return line
	}
	return ""
}

func resolveTTSInstallerScript(executablePath string) (string, error) {
	return resolvePackagedSetupScript(executablePath, "install-tts-module.ps1")
}

func resolvePackagedSetupScript(executablePath, name string) (string, error) {
	safeNames := map[string]string{
		"install-llama-runtime.ps1":   "install-llama-runtime.ps1",
		"install-parakeet-module.ps1": "install-parakeet-module.ps1",
		"install-tts-module.ps1":      "install-tts-module.ps1",
	}
	safeName, ok := safeNames[name]
	if !ok {
		return "", fmt.Errorf("unsupported setup helper %q", name)
	}
	var candidates []string
	if executablePath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(executablePath), "scripts", safeName))
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(workingDirectory, "scripts", safeName))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return filepath.Abs(candidate)
		}
	}
	return "", fmt.Errorf("setup helper %s is unavailable; repair MagicHandy or use the packaged scripts manually", safeName)
}

func resolveSetupPowerShell() (string, error) {
	for _, name := range []string{"powershell.exe", "pwsh.exe", "pwsh"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("managed voice installation requires Windows PowerShell 5.1 or PowerShell 7")
}

func verifyInstalledVoiceModule(root string, module setupVoiceModule) error {
	stateFile, err := os.Open(filepath.Join(root, "module-state.json")) // #nosec G304 -- fixed beneath the app-owned voice module root.
	if err != nil {
		return fmt.Errorf("voice module install is incomplete: module-state.json is missing: %w", err)
	}
	defer func() { _ = stateFile.Close() }()
	var state struct {
		SchemaVersion int    `json:"schema_version"`
		Module        string `json:"module"`
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		Voice         string `json:"voice"`
	}
	decoder := json.NewDecoder(io.LimitReader(stateFile, 32*1024))
	if err := decoder.Decode(&state); err != nil {
		return fmt.Errorf("read voice module state: %w", err)
	}
	if state.SchemaVersion != 2 || state.Module != module.ID || state.Provider != module.Provider {
		return errors.New("voice module state does not match the requested module")
	}
	settings := config.DefaultSettings().Voice
	settings.TTSProvider = module.Provider
	settings.TTSModuleRoot = root
	settings.TTSModel = strings.TrimSpace(state.Model)
	settings.TTSVoice = strings.TrimSpace(state.Voice)
	if err := managedTTSRuntimeError(settings, ""); err != nil {
		return fmt.Errorf("voice module install is incomplete: %w", err)
	}
	return nil
}

func findSetupVoiceModule(id string) (setupVoiceModule, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, module := range setupVoiceModules {
		if module.ID == id {
			return module, nil
		}
	}
	return setupVoiceModule{}, fmt.Errorf("unknown managed voice module %q", id)
}

func newSetupJobID() (string, error) {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("create setup job ID: %w", err)
	}
	return "setup-" + hex.EncodeToString(data), nil
}

func stringInSlice(value string, values []string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func setupHardwareSnapshot() map[string]any {
	snapshot := map[string]any{
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
		"nvidia":   false,
		"cuda":     false,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits") // #nosec G204 -- fixed diagnostic command.
	configureSetupProcess(command)
	if output, err := command.Output(); err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(output)), ",", 2)
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			snapshot["nvidia"] = true
			snapshot["gpu_name"] = strings.TrimSpace(parts[0])
		}
		if len(parts) == 2 {
			snapshot["vram_mib"] = strings.TrimSpace(parts[1])
		}
	}
	if _, err := exec.LookPath("nvcc"); err == nil {
		snapshot["cuda"] = true
	}
	return snapshot
}

func (s *Server) setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/setup", s.handleSetupStatus)
	mux.HandleFunc("PUT /api/setup/preferences", s.handleSetupPreferences)
	mux.HandleFunc("POST /api/setup/llm/install", s.handleSetupLlamaInstall)
	mux.HandleFunc("POST /api/setup/parakeet/install", s.handleSetupParakeetInstall)
	mux.HandleFunc("POST /api/setup/voice/install", s.handleSetupVoiceInstall)
	mux.HandleFunc("POST /api/setup/install", s.handleSetupInstallPlan)
	mux.HandleFunc("DELETE /api/setup/install", s.handleSetupInstallCancel)
	mux.HandleFunc("POST /api/setup/complete", s.handleSetupComplete)
}

func (s *Server) handleSetupPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request setupPreferencesRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.UILocale = strings.TrimSpace(request.UILocale)
	request.ChatLocale = strings.TrimSpace(request.ChatLocale)
	request.DeviceOwner = strings.TrimSpace(request.DeviceOwner)
	if (request.UILocale == "") != (request.ChatLocale == "") {
		writeError(w, http.StatusBadRequest, errors.New("UI and chat locales must be provided together"))
		return
	}
	if request.UILocale != "" && (!config.IsSupportedLocale(request.UILocale) || !config.IsSupportedLocale(request.ChatLocale)) {
		writeError(w, http.StatusBadRequest, errors.New("setup locale is unsupported"))
		return
	}
	if request.DeviceOwner != "" && !stringInSlice(request.DeviceOwner, []string{
		config.DispatchOwnerCloudREST,
		config.DispatchOwnerBrowserBluetooth,
		config.DispatchOwnerIntiface,
	}) {
		writeError(w, http.StatusBadRequest, errors.New("setup device transport is unsupported"))
		return
	}
	if request.ConnectionKey != nil {
		trimmed := strings.TrimSpace(*request.ConnectionKey)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, errors.New("connection key cannot be empty"))
			return
		}
		request.ConnectionKey = &trimmed
	}

	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		if request.LLM != nil {
			current.LLM = *request.LLM
		}
		if request.UILocale != "" {
			promptSet, _ := config.PromptSetForLocale(request.ChatLocale)
			current.UI.Locale = request.UILocale
			current.LLM.PromptSet = promptSet
		}
		if request.DeviceOwner != "" {
			current.Device.HSPDispatchOwner = request.DeviceOwner
		}
		if request.ConnectionKey != nil {
			current.Device.HandyConnectionKey = *request.ConnectionKey
		}
		return current, nil
	})
	if saveErr != nil {
		writeError(w, http.StatusBadRequest, errors.New("setup preferences could not be saved"))
		return
	}
	payload := map[string]any{"settings": saved.Public()}
	if runtimeErr != nil {
		payload["warning"] = "preferences were saved, but the selected runtime is not ready yet"
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, _ *http.Request) {
	settings, status := s.store.Snapshot()
	helpers := map[string]bool{
		"llama":    setupScriptPresent(s.voiceExecutable, "install-llama-runtime.ps1"),
		"parakeet": setupScriptPresent(s.voiceExecutable, "install-parakeet-module.ps1"),
		"voice":    setupScriptPresent(s.voiceExecutable, "install-tts-module.ps1"),
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"required":        !settings.UI.SetupCompleted,
		"data_dir":        status.DataDir,
		"hardware":        s.setup.HardwareSnapshot(),
		"voice_modules":   setupVoiceModules,
		"llama_runtime":   setupLlamaRuntime,
		"parakeet":        setupParakeet,
		"installation":    s.setup.Snapshot(),
		"scripts_present": helpers["llama"] && helpers["parakeet"] && helpers["voice"],
		"helpers":         helpers,
	})
}

func (s *Server) handleSetupVoiceInstall(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request setupVoiceInstallRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	job, err := s.setup.StartVoiceInstall(request)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"installation": job})
}

func (s *Server) handleSetupInstallCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	job, err := s.setup.Cancel()
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installation": job})
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var request setupCompleteRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, hasSession := authenticatedSession(r)
	signOutAfterFirstCompletion := false
	_, saved, saveErr, runtimeErr := s.updateSettingsAndRuntime(r.Context(), func(current config.Settings) (config.Settings, error) {
		signOutAfterFirstCompletion = !current.UI.SetupCompleted && hasSession
		current.UI.SetupCompleted = true
		return current, nil
	})
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, errors.New("setup completion could not be saved"))
		return
	}
	payload := map[string]any{"settings": saved.Public()}
	status := http.StatusOK
	if runtimeErr != nil {
		if request.AllowUnreadyLLM {
			payload["warning"] = "setup was saved without a ready chat model"
		} else {
			status = http.StatusBadGateway
			payload["error"] = "setup was saved, but an active runtime could not apply the final settings"
		}
	}
	if signOutAfterFirstCompletion {
		if err := s.accounts.RevokeSession(r.Context(), session.token); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("setup was saved, but the temporary setup session could not be revoked"))
			return
		}
		s.clearSessionCookie(w)
		w.Header().Set("Clear-Site-Data", `"cookies"`)
		payload["signed_out"] = true
	}
	writeJSON(w, status, payload)
}

func (s *Server) applyInstalledVoiceModule(ctx context.Context, result setupVoiceInstallResult) error {
	_, _, saveErr, runtimeErr := s.updateSettingsAndRuntime(ctx, func(current config.Settings) (config.Settings, error) {
		current.Voice.Enabled = false
		current.Voice.SpeakReplies = false
		current.Voice.TTSProvider = result.Module.Provider
		current.Voice.TTSWorkerPath = ""
		current.Voice.TTSWorkerArgs = nil
		current.Voice.TTSModuleRoot = result.Root
		current.Voice.TTSBaseURL = fmt.Sprintf("http://127.0.0.1:%d", result.Module.Port)
		current.Voice.TTSServerPort = result.Module.Port
		current.Voice.TTSDevice = result.Device
		current.Voice.TTSAutoLaunch = result.AutoLaunch
		current.Voice.TTSResponseFormat = config.DefaultTTSResponseFormat
		current.Voice.TTSLanguage = "Auto"
		if result.Module.Provider == config.VoiceTTSProviderFasterQwen {
			current.Voice.TTSModel = config.DefaultFasterQwenModel
			current.Voice.TTSVoice = config.DefaultFasterQwenVoice
			current.Voice.TTSHealthPath = config.DefaultTTSHealthPath
		} else {
			current.Voice.TTSModel = config.DefaultChatterboxModel
			current.Voice.TTSVoice = config.DefaultChatterboxVoice
			current.Voice.TTSHealthPath = config.DefaultChatterboxHealthPath
		}
		return current, nil
	})
	return errors.Join(saveErr, runtimeErr)
}

func setupScriptPresent(executablePath, name string) bool {
	_, err := resolvePackagedSetupScript(executablePath, name)
	return err == nil
}
