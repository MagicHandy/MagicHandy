package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

// voiceHealthTimeout bounds the live health probe on the status endpoint so
// a wedged worker cannot stall the settings page.
const voiceHealthTimeout = 3 * time.Second

// voiceModelLoadTimeout bounds startup autoload and the automatic load that
// follows a user-initiated start; managed providers may boot a local server.
const voiceModelLoadTimeout = 15 * time.Minute

const (
	maxVoiceAudioBytes       = 32 << 20
	maxVoiceAudioBase64Bytes = ((maxVoiceAudioBytes + 2) / 3) * 4
	maxVoiceRequestBytes     = maxVoiceAudioBase64Bytes + 1024
)

func newVoiceManager(settings config.VoiceSettings, executablePath, dataDir string) *voice.Manager {
	manager := voice.NewManager()
	manager.Configure(voiceManagerConfig(settings, executablePath, dataDir))
	return manager
}

func voiceManagerConfig(settings config.VoiceSettings, executablePath, dataDir string) voice.Config {
	// Provider credentials travel to the worker process privately via its
	// environment — never on the command line (visible in process listings)
	// and never through any status or protocol frame.
	tts := voice.WorkerConfig{Enabled: settings.Enabled && settings.TTSProvider != config.VoiceProviderNone}
	switch settings.TTSProvider {
	case config.VoiceTTSProviderElevenLabs:
		tts.Command = resolveWorkerBinary(settings.TTSWorkerPath, executablePath, dataDir, "voice-elevenlabs-worker")
		tts.Args = []string{"-voice-id", settings.ElevenLabsVoiceID, "-model-id", settings.ElevenLabsModelID}
		if settings.ElevenLabsAPIKey != "" {
			tts.Env = map[string]string{"ELEVENLABS_API_KEY": settings.ElevenLabsAPIKey}
		}
	case config.VoiceTTSProviderFasterQwen,
		config.VoiceTTSProviderChatterbox,
		config.VoiceTTSProviderOpenAICompat:
		tts = openAITTSWorkerConfig(settings, executablePath, dataDir)
	case config.VoiceProviderCustom:
		tts.Command, tts.Args = settings.TTSWorkerPath, settings.TTSWorkerArgs
	}

	asr := voice.WorkerConfig{Enabled: settings.Enabled && settings.ASRProvider != config.VoiceProviderNone}
	switch settings.ASRProvider {
	case config.VoiceASRProviderParakeet:
		serverPath, modelPath := settings.ParakeetServerPath, settings.ParakeetModelPath
		if settings.ParakeetSource == config.ParakeetSourceApp {
			serverPath, modelPath = parakeetAppPaths(dataDir)
			if !isRegularFile(serverPath) || !isRegularFile(modelPath) {
				break
			}
		}
		command := resolveWorkerBinary(settings.ASRWorkerPath, executablePath, dataDir, "voice-parakeet-worker")
		if command != "" && serverPath != "" && modelPath != "" {
			asr.Command = command
			asr.Args = []string{"-server-path", serverPath, "-server-model", modelPath, "-server-port", strconv.Itoa(settings.ParakeetServerPort)}
		}
	case config.VoiceASRProviderOpenAICompat:
		asr.Command = resolveWorkerBinary(settings.ASRWorkerPath, executablePath, dataDir, "voice-parakeet-worker")
		asr.Args = []string{"-base-url", settings.ASRBaseURL}
		if settings.ASRModel != "" {
			asr.Args = append(asr.Args, "-model", settings.ASRModel)
		}
	case config.VoiceProviderCustom:
		asr.Command, asr.Args = settings.ASRWorkerPath, settings.ASRWorkerArgs
	}
	return voice.Config{TTS: tts, ASR: asr}
}

func openAITTSWorkerConfig(settings config.VoiceSettings, executablePath, dataDir string) voice.WorkerConfig {
	command := resolveWorkerBinary(settings.TTSWorkerPath, executablePath, dataDir, "voice-openai-tts-worker")
	if settings.TTSProvider == config.VoiceTTSProviderFasterQwen ||
		settings.TTSProvider == config.VoiceTTSProviderChatterbox {
		command = resolveFirstPartyWorkerBinary(settings.TTSWorkerPath, executablePath, dataDir, "voice-openai-tts-worker")
	}
	worker := voice.WorkerConfig{
		Enabled:    settings.Enabled,
		JobTimeout: 10 * time.Minute,
		Command:    command,
		Args: []string{
			"-base-url", settings.TTSBaseURL,
			"-model", settings.TTSModel,
			"-voice", settings.TTSVoice,
			"-response-format", settings.TTSResponseFormat,
			"-health-path", settings.TTSHealthPath,
		},
	}
	if settings.TTSProvider == config.VoiceTTSProviderOpenAICompat && settings.OpenAITTSAPIKey != "" {
		worker.Env = map[string]string{"OPENAI_TTS_API_KEY": settings.OpenAITTSAPIKey}
	}
	if settings.TTSProvider == config.VoiceTTSProviderChatterbox {
		worker.Args = append(worker.Args, "-health-ready-field", "loaded")
	}
	if settings.TTSProvider == config.VoiceTTSProviderOpenAICompat || !settings.TTSAutoLaunch {
		return worker
	}

	managed, ready := managedTTSCommand(settings, dataDir)
	if !ready {
		worker.Command = ""
		worker.Args = nil
		return worker
	}
	worker.Args = append(worker.Args,
		"-server-command", managed.command,
		"-server-dir", managed.directory,
		"-server-port", strconv.Itoa(settings.TTSServerPort),
	)
	for _, argument := range managed.args {
		worker.Args = append(worker.Args, "-server-arg", argument)
	}
	worker.JobTimeout = voiceModelLoadTimeout
	moduleRoot := ttsModuleRoot(settings, dataDir)
	if moduleRoot != "" {
		if worker.Env == nil {
			worker.Env = make(map[string]string)
		}
		worker.Env["HF_HOME"] = filepath.Join(moduleRoot, "model-cache")
		worker.Env["HF_HUB_OFFLINE"] = "1"
		worker.Env["TRANSFORMERS_OFFLINE"] = "1"
		worker.Env["NUMBA_CACHE_DIR"] = filepath.Join(moduleRoot, "runtime-cache", "numba")
	}
	return worker
}

func voiceSpeechRequest(settings config.VoiceSettings, text string) voice.Request {
	request := voice.Request{Type: voice.RequestSpeak, Text: text}
	if settings.TTSProvider != config.VoiceTTSProviderFasterQwen {
		return request
	}
	seed := settings.TTSSeed
	request.Seed = &seed
	request.RandomizeSeed = settings.TTSSeedMode == config.TTSSeedModeVaried
	request.Instruct = config.ResolveTTSTonePrompt(settings)
	return request
}

type managedTTSLaunch struct {
	command   string
	directory string
	args      []string
}

func managedTTSCommand(settings config.VoiceSettings, dataDir string) (managedTTSLaunch, bool) {
	root := ttsModuleRoot(settings, dataDir)
	if root == "" || !managedTTSRuntimeInstalled(settings, dataDir) {
		return managedTTSLaunch{}, false
	}
	python := filepath.Join(root, ".venv", "Scripts", "python.exe")
	if runtime.GOOS != "windows" {
		python = filepath.Join(root, ".venv", "bin", "python")
	}
	source := filepath.Join(root, "source")

	switch settings.TTSProvider {
	case config.VoiceTTSProviderFasterQwen:
		server := filepath.Join(source, "examples", "openai_server.py")
		launcher := filepath.Join(root, "magichandy-faster-qwen-server.py")
		if !fasterQwenReferenceConfigured(settings) {
			return managedTTSLaunch{}, false
		}
		modelPath, err := fasterQwenModelPath(settings, dataDir)
		if err != nil {
			return managedTTSLaunch{}, false
		}
		device := settings.TTSDevice
		if device == config.TTSDeviceAuto {
			device = config.TTSDeviceCUDA
		}
		return managedTTSLaunch{
			command:   python,
			directory: source,
			args: []string{
				launcher,
				server,
				"--model", modelPath,
				"--ref-audio", settings.TTSReferenceWAV,
				"--ref-text", settings.TTSReferenceText,
				"--language", settings.TTSLanguage,
				"--host", "127.0.0.1",
				"--port", strconv.Itoa(settings.TTSServerPort),
				"--device", device,
			},
		}, true
	case config.VoiceTTSProviderChatterbox:
		server := filepath.Join(source, "server.py")
		launcher := filepath.Join(root, "magichandy-chatterbox-server.py")
		runtimeDir := filepath.Join(root, "runtime")
		return managedTTSLaunch{
			command:   python,
			directory: runtimeDir,
			args:      []string{launcher, server},
		}, true
	default:
		return managedTTSLaunch{}, false
	}
}

func managedTTSRuntimeInstalled(settings config.VoiceSettings, dataDir string) bool {
	return managedTTSRuntimeError(settings, dataDir) == nil
}

func managedTTSRuntimeError(settings config.VoiceSettings, dataDir string) error {
	root := ttsModuleRoot(settings, dataDir)
	if root == "" {
		return errors.New("managed TTS module root is unavailable")
	}
	python := filepath.Join(root, ".venv", "Scripts", "python.exe")
	if runtime.GOOS != "windows" {
		python = filepath.Join(root, ".venv", "bin", "python")
	}
	if !isRegularFile(python) {
		return fmt.Errorf("managed Python executable is missing: %s", python)
	}
	source := filepath.Join(root, "source")
	switch settings.TTSProvider {
	case config.VoiceTTSProviderFasterQwen:
		server := filepath.Join(source, "examples", "openai_server.py")
		if !isRegularFile(server) {
			return fmt.Errorf("faster Qwen3-TTS server entry point is missing: %s", server)
		}
		adapter := filepath.Join(root, "magichandy-faster-qwen-server.py")
		if !isRegularFile(adapter) {
			return fmt.Errorf("missing MagicHandy Faster Qwen3-TTS adapter: %s", adapter)
		}
		if _, err := fasterQwenModelPath(settings, dataDir); err != nil {
			return err
		}
		return nil
	case config.VoiceTTSProviderChatterbox:
		runtimeDir := filepath.Join(root, "runtime")
		required := []struct {
			label string
			path  string
		}{
			{label: "Chatterbox server entry point", path: filepath.Join(source, "server.py")},
			{label: "MagicHandy Chatterbox adapter", path: filepath.Join(root, "magichandy-chatterbox-server.py")},
			{label: "Chatterbox runtime configuration", path: filepath.Join(runtimeDir, "config.yaml")},
			{label: "selected Chatterbox voice", path: filepath.Join(runtimeDir, "voices", settings.TTSVoice)},
		}
		for _, file := range required {
			if !isRegularFile(file.path) {
				return fmt.Errorf("%s is missing: %s", file.label, file.path)
			}
		}
		return nil
	default:
		return fmt.Errorf("provider %q is not an app-managed TTS runtime", settings.TTSProvider)
	}
}

func fasterQwenModelPath(settings config.VoiceSettings, dataDir string) (string, error) {
	model := strings.TrimSpace(settings.TTSModel)
	if model == "" {
		return "", errors.New("faster Qwen3-TTS model is not configured")
	}
	if info, err := os.Stat(model); err == nil && info.IsDir() {
		return validateFasterQwenModelDirectory(model)
	}
	if filepath.IsAbs(model) || strings.Contains(model, `\`) {
		return "", fmt.Errorf("faster Qwen3-TTS model directory is unavailable: %q", model)
	}

	parts := strings.Split(model, "/")
	if len(parts) != 2 || !validHuggingFaceRepositoryPart(parts[0]) ||
		!validHuggingFaceRepositoryPart(parts[1]) {
		return "", fmt.Errorf("faster Qwen3-TTS model must be a local directory or Hugging Face repository ID: %q", model)
	}
	root := ttsModuleRoot(settings, dataDir)
	if root == "" {
		return "", errors.New("faster Qwen3-TTS module root is unavailable")
	}
	repositoryCache := filepath.Join(root, "model-cache", "hub", "models--"+strings.Join(parts, "--"))
	snapshots := filepath.Join(repositoryCache, "snapshots")

	// #nosec G304 -- the module root is an explicit local setting and the
	// repository components are validated before this fixed cache path is read.
	ref, err := os.ReadFile(filepath.Join(repositoryCache, "refs", "main"))
	if err == nil {
		revision := strings.TrimSpace(string(ref))
		if !validSnapshotRevision(revision) {
			return "", errors.New("faster Qwen3-TTS model cache has an invalid refs/main revision")
		}
		return validateFasterQwenModelDirectory(filepath.Join(snapshots, revision))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read Faster Qwen3-TTS model cache revision: %w", err)
	}

	entries, err := os.ReadDir(snapshots)
	if err != nil {
		return "", fmt.Errorf("read Faster Qwen3-TTS model snapshots: %w", err)
	}
	candidates := make([]string, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() && validSnapshotRevision(entry.Name()) {
			candidates = append(candidates, filepath.Join(snapshots, entry.Name()))
		}
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("faster Qwen3-TTS model cache has %d snapshots and no refs/main revision", len(candidates))
	}
	return validateFasterQwenModelDirectory(candidates[0])
}

func validHuggingFaceRepositoryPart(part string) bool {
	return part != "" && part != "." && part != ".." &&
		!strings.ContainsAny(part, `/\:`)
}

func validSnapshotRevision(revision string) bool {
	return revision != "" && revision != "." && revision != ".." &&
		!strings.ContainsAny(revision, `/\:`)
}

func validateFasterQwenModelDirectory(directory string) (string, error) {
	required := []string{
		filepath.Join(directory, "config.json"),
		filepath.Join(directory, "tokenizer_config.json"),
		filepath.Join(directory, "speech_tokenizer", "config.json"),
		filepath.Join(directory, "speech_tokenizer", "model.safetensors"),
	}
	for _, path := range required {
		if !isRegularFile(path) {
			return "", fmt.Errorf("faster Qwen3-TTS model cache is incomplete: %s is missing", path)
		}
	}
	if !isRegularFile(filepath.Join(directory, "model.safetensors")) &&
		!isRegularFile(filepath.Join(directory, "model.safetensors.index.json")) {
		return "", fmt.Errorf("faster Qwen3-TTS model cache is incomplete: model weights are missing from %s", directory)
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve Faster Qwen3-TTS model directory: %w", err)
	}
	return filepath.Clean(abs), nil
}

func fasterQwenReferenceConfigured(settings config.VoiceSettings) bool {
	return isRegularFile(settings.TTSReferenceWAV) &&
		strings.TrimSpace(settings.TTSReferenceText) != ""
}

func ttsModuleRoot(settings config.VoiceSettings, dataDir string) string {
	if root := strings.TrimSpace(settings.TTSModuleRoot); root != "" {
		return root
	}
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	switch settings.TTSProvider {
	case config.VoiceTTSProviderFasterQwen:
		return filepath.Join(dataDir, "voice", "faster-qwen3-tts")
	case config.VoiceTTSProviderChatterbox:
		return filepath.Join(dataDir, "voice", "chatterbox-tts")
	default:
		return ""
	}
}

func parakeetAppPaths(dataDir string) (string, string) {
	if strings.TrimSpace(dataDir) == "" {
		return "", ""
	}
	root := filepath.Join(dataDir, "voice", "parakeet")
	return filepath.Join(root, "runner", "parakeet-server.exe"), filepath.Join(root, "tdt-0.6b-v3-q4_k.gguf")
}

func isRegularFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	// #nosec G703 -- this checks an explicit local settings path without reading
	// content; provider processes receive the same path after validation.
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

type voiceModuleStatus struct {
	State            string `json:"state"`
	Installed        bool   `json:"installed"`
	WorkerInstalled  bool   `json:"worker_installed"`
	RuntimeInstalled bool   `json:"runtime_installed"`
	RuntimeBackend   string `json:"runtime_backend,omitempty"`
	Message          string `json:"message"`
}

func inspectParakeetAppModule(workerOverride, executablePath, dataDir string) voiceModuleStatus {
	worker := resolveWorkerBinary(workerOverride, executablePath, dataDir, "voice-parakeet-worker")
	serverPath, modelPath := parakeetAppPaths(dataDir)
	workerInstalled := isRegularFile(worker)
	runtimeInstalled := isRegularFile(serverPath) && isRegularFile(modelPath)
	status := voiceModuleStatus{
		State:            "missing",
		WorkerInstalled:  workerInstalled,
		RuntimeInstalled: runtimeInstalled,
		Message:          "Parakeet is not installed by MagicHandy. Rerun update.ps1 with Parakeet enabled.",
	}
	if workerInstalled && runtimeInstalled {
		status.State = "ready"
		status.Installed = true
		status.Message = "MagicHandy's Parakeet worker, runner, and model are installed."
		return status
	}
	if workerInstalled || runtimeInstalled {
		status.State = "incomplete"
		status.Message = "The MagicHandy Parakeet module is incomplete. Rerun update.ps1 with Parakeet enabled."
	}
	return status
}

func inspectTTSModule(settings config.VoiceSettings, executablePath, dataDir string) voiceModuleStatus {
	worker := resolveFirstPartyWorkerBinary(settings.TTSWorkerPath, executablePath, dataDir, "voice-openai-tts-worker")
	runtimeErr := managedTTSRuntimeError(settings, dataDir)
	runtimeInstalled := runtimeErr == nil
	name := "Local TTS"
	switch settings.TTSProvider {
	case config.VoiceTTSProviderFasterQwen:
		name = "Faster Qwen3-TTS"
	case config.VoiceTTSProviderChatterbox:
		name = "Chatterbox TTS"
	}
	status := voiceModuleStatus{
		State:            "missing",
		WorkerInstalled:  isRegularFile(worker),
		RuntimeInstalled: runtimeInstalled,
		RuntimeBackend:   settings.TTSDevice,
		Message:          name + " is not installed. Run scripts/install-tts-module.ps1.",
	}
	if status.WorkerInstalled && runtimeInstalled {
		if settings.TTSProvider == config.VoiceTTSProviderFasterQwen &&
			!fasterQwenReferenceConfigured(settings) {
			status.State = "incomplete"
			status.Message = "Faster Qwen3-TTS is installed. Add a reference WAV and its exact transcript in Voice settings, then save."
			return status
		}
		status.State = "ready"
		status.Installed = true
		status.Message = name + " and the OpenAI-compatible adapter are installed."
		return status
	}
	if status.WorkerInstalled || runtimeInstalled {
		status.State = "incomplete"
		switch {
		case !status.WorkerInstalled:
			status.Message = name + " cannot find the bundled OpenAI-compatible worker. Repair or reinstall MagicHandy."
		case runtimeErr != nil:
			status.Message = name + " is incomplete: " + runtimeErr.Error() + ". Rerun scripts/update-tts-module.ps1 or retry from Setup."
		}
	}
	return status
}

func resolveFirstPartyWorkerBinary(explicit, executablePath, dataDir, name string) string {
	explicit = strings.TrimSpace(explicit)
	if explicit == "" || workerCommandAvailable(explicit) {
		return resolveWorkerBinary(explicit, executablePath, dataDir, name)
	}
	if bundled := resolveWorkerBinary("", executablePath, dataDir, name); bundled != "" {
		return bundled
	}
	return explicit
}

func workerCommandAvailable(command string) bool {
	if isRegularFile(command) {
		return true
	}
	resolved, err := exec.LookPath(command)
	return err == nil && isRegularFile(resolved)
}

func resolveWorkerBinary(explicit, executablePath, dataDir, name string) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if executablePath == "" {
		executablePath, _ = os.Executable()
	}
	candidates := []string{}
	if executablePath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(executablePath), name))
	}
	if dataDir != "" {
		candidates = append(candidates, filepath.Join(dataDir, "tools", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (s *Server) voiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/voice/status", s.handleVoiceStatus)
	mux.HandleFunc("POST /api/voice/workers/{role}/start", s.handleVoiceWorkerStart)
	mux.HandleFunc("POST /api/voice/workers/{role}/stop", s.handleVoiceWorkerStop)
	mux.HandleFunc("POST /api/voice/workers/{role}/restart", s.handleVoiceWorkerRestart)
	mux.HandleFunc("POST /api/voice/workers/{role}/model", s.handleVoiceWorkerModel)
	mux.HandleFunc("POST /api/voice/workers/{role}/test", s.handleVoiceWorkerTest)
	mux.HandleFunc("GET /api/voice/requests/{id}", s.handleVoiceRequestGet)
	mux.HandleFunc("GET /api/voice/requests/{id}/audio", s.handleVoiceRequestAudio)
	mux.HandleFunc("POST /api/voice/requests/{id}/cancel", s.handleVoiceRequestCancel)
	mux.HandleFunc("POST /api/voice/requests/{id}/played", s.handleVoiceRequestPlayed)
	mux.HandleFunc("POST /api/voice/transcriptions", s.handleVoiceTranscription)
	mux.HandleFunc("PUT /api/voice/preferences", s.handleVoicePreferences)
	mux.HandleFunc("PUT /api/voice/input-preferences", s.handleVoiceInputPreferences)
}

// voiceState is the /api/state block: lifecycle snapshots only, no live IPC
// on the polling path.
func (s *Server) voiceState() map[string]any {
	settings, _ := s.store.Snapshot()
	status := s.voice.Status()
	modules := map[string]any{
		"parakeet": inspectParakeetAppModule(settings.Voice.ASRWorkerPath, s.voiceExecutable, s.voiceDataDir),
	}
	if settings.Voice.TTSProvider == config.VoiceTTSProviderFasterQwen ||
		settings.Voice.TTSProvider == config.VoiceTTSProviderChatterbox {
		modules["tts"] = inspectTTSModule(settings.Voice, s.voiceExecutable, s.voiceDataDir)
	}
	return map[string]any{
		"enabled":          settings.Voice.Enabled,
		"protocol_version": voice.ProtocolVersion,
		"workers":          status,
		"modules":          modules,
	}
}

// handleVoiceStatus returns both workers with a live health probe for
// running ones (model state and worker queue depth stay fresh).
func (s *Server) handleVoiceStatus(w http.ResponseWriter, r *http.Request) {
	for _, role := range voice.Roles() {
		worker := s.voice.Worker(role)
		if worker.Status().State == voice.StateRunning {
			ctx, cancel := context.WithTimeout(r.Context(), voiceHealthTimeout)
			_, _ = worker.Health(ctx)
			cancel()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"voice":    s.voiceState(),
		"requests": s.voice.Requests(),
	})
}

func (s *Server) voiceWorkerFromPath(w http.ResponseWriter, r *http.Request) (*voice.Supervisor, bool) {
	role, err := voice.ParseRole(r.PathValue("role"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return nil, false
	}
	return s.voice.Worker(role), true
}

func (s *Server) handleVoiceWorkerStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	worker, ok := s.voiceWorkerFromPath(w, r)
	if !ok {
		return
	}
	if err := worker.Start(r.Context()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  err.Error(),
			"worker": worker.Status(),
		})
		return
	}
	s.writeStartedWorker(w, worker)
}

// writeStartedWorker follows every user-initiated start with a model load.
// Startup autoload uses the same ready-state contract through
// ensureVoiceWorkerReady.
func (s *Server) writeStartedWorker(w http.ResponseWriter, worker *voice.Supervisor) {
	// Detached context: an impatient client disconnect must not abort the load.
	ctx, cancel := context.WithTimeout(context.Background(), voiceModelLoadTimeout)
	defer cancel()
	if err := ensureVoiceWorkerReady(ctx, worker); err != nil {
		s.logger.Warn("voice model auto-load failed", "role", worker.Status().Role, "error", err)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), voiceHealthTimeout)
		_ = worker.Stop(stopCtx)
		stopCancel()
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  fmt.Sprintf("voice worker could not become ready: %s", err),
			"worker": worker.Status(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"worker": worker.Status()})
}

func ensureVoiceWorkerReady(ctx context.Context, worker *voice.Supervisor) error {
	status := worker.Status()
	if status.State == voice.StateRunning && status.ModelState == voice.ModelStateReady {
		response, err := worker.Health(ctx)
		if err == nil && response.ModelState == voice.ModelStateReady {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if err := worker.Start(ctx); err != nil {
		return err
	}
	status = worker.Status()
	if status.ModelState == voice.ModelStateReady {
		return nil
	}
	_, err := worker.SetModelLoaded(ctx, true)
	return err
}

func (s *Server) startVoiceAutoload(settings config.VoiceSettings) {
	roles := voiceAutoloadRoles(settings)
	if len(roles) == 0 || s.voice == nil {
		return
	}
	parent := s.lifecycleCtx
	if parent == nil {
		parent = context.Background()
	}

	s.voiceAutoloadMu.Lock()
	if s.voiceAutoloadCancel != nil {
		s.voiceAutoloadMu.Unlock()
		return
	}
	s.voiceAutoloadID++
	autoloadID := s.voiceAutoloadID
	ctx, cancel := context.WithCancel(parent)
	s.voiceAutoloadCancel = cancel
	s.voiceAutoloadWG.Add(1)
	s.voiceAutoloadMu.Unlock()

	go func() {
		defer func() {
			cancel()
			s.voiceAutoloadMu.Lock()
			if s.voiceAutoloadID == autoloadID {
				s.voiceAutoloadCancel = nil
			}
			s.voiceAutoloadMu.Unlock()
			s.voiceAutoloadWG.Done()
		}()

		var rolesWG sync.WaitGroup
		for _, role := range roles {
			rolesWG.Add(1)
			go func(role voice.Role) {
				defer rolesWG.Done()
				s.autoloadVoiceWorker(ctx, role)
			}(role)
		}
		rolesWG.Wait()
	}()
}

func (s *Server) autoloadVoiceWorker(ctx context.Context, role voice.Role) {
	worker := s.voice.Worker(role)
	status := worker.Status()
	if !status.Configured {
		s.logger.Warn("voice worker autoload skipped", "role", role, "reason", "worker is not configured")
		return
	}
	loadCtx, loadCancel := context.WithTimeout(ctx, voiceModelLoadTimeout)
	defer loadCancel()
	if err := ensureVoiceWorkerReady(loadCtx, worker); err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("voice worker autoload failed", "role", role, "error", err)
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), voiceHealthTimeout)
		_ = worker.Stop(stopCtx)
		stopCancel()
		return
	}
	s.logger.Info("voice worker autoloaded", "role", role, "model_state", worker.Status().ModelState)
}

func voiceAutoloadRoles(settings config.VoiceSettings) []voice.Role {
	if !settings.Enabled {
		return nil
	}
	roles := make([]voice.Role, 0, 2)
	if (settings.TTSAutoLaunch || settings.SpeakReplies) && settings.TTSProvider != config.VoiceProviderNone {
		roles = append(roles, voice.RoleTTS)
	}
	if settings.ASRProvider != config.VoiceProviderNone {
		roles = append(roles, voice.RoleASR)
	}
	return roles
}

func (s *Server) stopVoiceAutoload() {
	s.voiceAutoloadMu.Lock()
	cancel := s.voiceAutoloadCancel
	s.voiceAutoloadCancel = nil
	s.voiceAutoloadID++
	s.voiceAutoloadMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.voiceAutoloadWG.Wait()
}

func (s *Server) handleVoiceWorkerStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	worker, ok := s.voiceWorkerFromPath(w, r)
	if !ok {
		return
	}
	if err := worker.Stop(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"worker": worker.Status()})
}

func (s *Server) handleVoiceWorkerRestart(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	worker, ok := s.voiceWorkerFromPath(w, r)
	if !ok {
		return
	}
	if err := worker.Restart(r.Context()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  err.Error(),
			"worker": worker.Status(),
		})
		return
	}
	s.writeStartedWorker(w, worker)
}

func (s *Server) handleVoiceWorkerModel(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	worker, ok := s.voiceWorkerFromPath(w, r)
	if !ok {
		return
	}
	var body struct {
		Loaded bool `json:"loaded"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), voiceModelActionTimeout(body.Loaded))
	defer cancel()
	health, err := worker.SetModelLoaded(ctx, body.Loaded)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  err.Error(),
			"worker": worker.Status(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model_state": health.ModelState,
		"worker":      worker.Status(),
	})
}

func voiceModelActionTimeout(loaded bool) time.Duration {
	if loaded {
		return voiceModelLoadTimeout
	}
	return voiceHealthTimeout
}

// handleVoiceWorkerTest submits a small valid request so the queue,
// cancellation, and error paths can be exercised without touching chat or
// motion (ADR 0003). ASR gets a valid silent WAV because real ASR servers
// reject the old arbitrary-byte stub before their model path is exercised.
func (s *Server) handleVoiceWorkerTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	worker, ok := s.voiceWorkerFromPath(w, r)
	if !ok {
		return
	}
	var body struct {
		Text        string `json:"text"`
		DelayMillis int    `json:"delay_ms"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	request := voice.Request{DelayMillis: body.DelayMillis}
	if worker.Status().Role == voice.RoleASR {
		request.Type = voice.RequestTranscribe
		// A non-empty test request checks format handling and inference. Empty
		// remains a no-audio rejection check, matching the worker contract.
		if strings.TrimSpace(body.Text) != "" {
			request.AudioB64 = silentTestWAVBase64()
			request.AudioFormat = "wav"
		}
	} else {
		settings, _ := s.store.Snapshot()
		request = voiceSpeechRequest(settings.Voice, body.Text)
		request.DelayMillis = body.DelayMillis
	}

	pending, err := s.voice.Submit(worker.Status().Role, request)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  err.Error(),
			"worker": worker.Status(),
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"request": pending.Snapshot()})
}

func silentTestWAVBase64() string {
	// 100 ms of 16 kHz mono PCM silence. The compact fixture is valid WAV and
	// intentionally contains no spoken content that could enter a transcript.
	const sampleRate = 16000
	const sampleCount = sampleRate / 10
	const bytesPerSample = 2
	dataSize := sampleCount * bytesPerSample
	data := make([]byte, 44+dataSize)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(36+dataSize))
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], sampleRate)
	binary.LittleEndian.PutUint32(data[28:32], sampleRate*bytesPerSample)
	binary.LittleEndian.PutUint16(data[32:34], bytesPerSample)
	binary.LittleEndian.PutUint16(data[34:36], bytesPerSample*8)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(dataSize))
	return base64.StdEncoding.EncodeToString(data)
}

func (s *Server) handleVoiceRequestGet(w http.ResponseWriter, r *http.Request) {
	pending, ok := s.voice.Request(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("unknown voice request"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"request": pending.Snapshot()})
}

// handleVoiceRequestAudio serves retained speak audio. The single-owner
// audio lease rides the controller lease (ADR 0003): only the active
// controller can fetch a clip, so two tabs never speak the same audio.
func (s *Server) handleVoiceRequestAudio(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	pending, ok := s.voice.Request(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("unknown voice request"))
		return
	}
	if pending.Snapshot().State != voice.RequestStateDone {
		writeError(w, http.StatusConflict, errors.New("voice audio is not available for a canceled or incomplete request"))
		return
	}
	audio, format := pending.Audio()
	if len(audio) == 0 {
		writeError(w, http.StatusNotFound, errors.New("no audio is retained for this request"))
		return
	}
	w.Header().Set("Content-Type", audioContentType(format))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- binary audio from the local worker process, served with
	// an explicit audio/* content type and nosniff; never rendered as HTML.
	_, _ = w.Write(audio)
}

func audioContentType(format string) string {
	switch strings.ToLower(format) {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "ogg", "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}

func (s *Server) handleVoiceRequestCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	pending, ok := s.voice.Request(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("unknown voice request"))
		return
	}
	s.voice.Worker(pending.Role).Cancel(pending)
	writeJSON(w, http.StatusOK, map[string]any{"request": pending.Snapshot()})
}

// handleVoiceRequestPlayed acknowledges completed browser playback. Inference
// completion is not audible completion, so Autopilot starts its next speech
// interval only from this controller-owned signal.
func (s *Server) handleVoiceRequestPlayed(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	pending, ok := s.voice.Request(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("unknown voice request"))
		return
	}
	snapshot := pending.Snapshot()
	// Done and failed both end the turn as far as the speech clock is concerned:
	// a failed synthesis produces no audio, so waiting out the playback fallback
	// would stall autonomous speech for two minutes over a request that already
	// finished. Canceled is deliberately excluded — the backend cancels, and it
	// reschedules that case itself.
	terminal := snapshot.State == voice.RequestStateDone || snapshot.State == voice.RequestStateFailed
	if snapshot.Role != voice.RoleTTS || snapshot.Type != voice.RequestSpeak || !terminal {
		writeError(w, http.StatusConflict, errors.New("only a finished speech request can be acknowledged"))
		return
	}
	acknowledged := s.modes.NotifySpeechPlaybackComplete(pending.ID)
	writeJSON(w, http.StatusOK, map[string]any{"acknowledged": acknowledged})
}

// applyVoiceSettingsTransition reconfigures workers after a settings save and
// restores every role whose persisted policy says it should be ready. Common
// Qwen seed/tone edits ride each speech request, so they do not churn the
// process or discard its loaded model and reference cache.
func (s *Server) applyVoiceSettingsTransition(previous, next config.Settings) {
	previousConfig := voiceManagerConfig(previous.Voice, s.voiceExecutable, s.voiceDataDir)
	nextConfig := voiceManagerConfig(next.Voice, s.voiceExecutable, s.voiceDataDir)
	previousRoles := voiceAutoloadRoles(previous.Voice)
	nextRoles := voiceAutoloadRoles(next.Voice)
	if previousConfig.Equal(nextConfig) && slices.Equal(previousRoles, nextRoles) {
		return
	}

	s.stopVoiceAutoload()
	s.voice.Configure(nextConfig)
	s.startVoiceAutoload(next.Voice)
}

func (s *Server) handleVoiceTranscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	stopSequence, err := s.requestStopSequence(r)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	var audio []byte
	var format string
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if strings.HasPrefix(contentType, "audio/") {
		format = strings.TrimPrefix(contentType, "audio/")
		r.Body = http.MaxBytesReader(w, r.Body, maxVoiceAudioBytes)
		var err error
		audio, err = io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, errors.New("recorded audio exceeds 32 MiB"))
				return
			}
			writeError(w, http.StatusBadRequest, fmt.Errorf("read recorded audio: %w", err))
			return
		}
	} else {
		var body struct {
			AudioB64    string `json:"audio_b64"`
			AudioFormat string `json:"audio_format"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxVoiceRequestBytes)
		if err := decodeVoiceTranscriptionJSON(r, &body); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, errors.New("recorded audio exceeds 32 MiB"))
				return
			}
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if len(body.AudioB64) > maxVoiceAudioBase64Bytes {
			writeError(w, http.StatusRequestEntityTooLarge, errors.New("recorded audio exceeds 32 MiB"))
			return
		}
		var err error
		audio, err = base64.StdEncoding.DecodeString(body.AudioB64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("recorded audio is not valid base64"))
			return
		}
		if len(audio) > maxVoiceAudioBytes {
			writeError(w, http.StatusRequestEntityTooLarge, errors.New("recorded audio exceeds 32 MiB"))
			return
		}
		format = strings.ToLower(strings.TrimSpace(body.AudioFormat))
	}
	if len(audio) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("recorded audio is required"))
		return
	}
	switch format {
	case "webm", "ogg", "wav":
		// Supported by MediaRecorder or the silent worker test.
	default:
		writeError(w, http.StatusBadRequest, errors.New("recorded audio format must be webm, ogg, or wav"))
		return
	}
	settings, _ := s.store.Snapshot()
	if settings.Voice.ASRProvider == config.VoiceASRProviderParakeet {
		if format != "wav" || !hasCanonicalASRWAV(audio) {
			writeError(w, http.StatusBadRequest, errors.New("managed Parakeet requires a WAV recording; refresh the MagicHandy page and record again"))
			return
		}
	}
	if s.stopSequence.Load() != stopSequence {
		writeError(w, http.StatusConflict, errors.New("recorded audio was invalidated by Emergency Stop"))
		return
	}
	pending, err := s.voice.SubmitTranscription(audio, format, s.voiceDataDir)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	if s.stopSequence.Load() != stopSequence {
		s.voice.Worker(voice.RoleASR).Cancel(pending)
		writeError(w, http.StatusConflict, errors.New("recorded audio was invalidated by Emergency Stop"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"request": pending.Snapshot()})
}

func hasCanonicalASRWAV(audio []byte) bool {
	decodedSize := len(audio)
	if decodedSize <= 44 || (decodedSize-44)%2 != 0 || uint64(decodedSize) > uint64(^uint32(0)) {
		return false
	}
	return string(audio[0:4]) == "RIFF" &&
		binary.LittleEndian.Uint32(audio[4:8]) == uint32(decodedSize-8) &&
		string(audio[8:12]) == "WAVE" && string(audio[12:16]) == "fmt " &&
		binary.LittleEndian.Uint32(audio[16:20]) == 16 &&
		binary.LittleEndian.Uint16(audio[20:22]) == 1 &&
		binary.LittleEndian.Uint16(audio[22:24]) == 1 &&
		binary.LittleEndian.Uint32(audio[24:28]) == 16000 &&
		binary.LittleEndian.Uint32(audio[28:32]) == 32000 &&
		binary.LittleEndian.Uint16(audio[32:34]) == 2 &&
		binary.LittleEndian.Uint16(audio[34:36]) == 16 &&
		string(audio[36:40]) == "data" &&
		binary.LittleEndian.Uint32(audio[40:44]) == uint32(decodedSize-44)
}

func decodeVoiceTranscriptionJSON(r *http.Request, target any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode JSON request: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("decode JSON request: multiple JSON values are not allowed")
	}
	return nil
}

func (s *Server) handleVoicePreferences(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body struct {
		SpeakReplies bool `json:"speak_replies"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_, _, err := s.store.Update(func(settings config.Settings) (config.Settings, error) {
		settings.Voice.SpeakReplies = body.SpeakReplies
		return settings, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("voice preference could not be saved"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"speak_replies": body.SpeakReplies})
}

type voiceInputPreferencesPatch struct {
	Mode             *string `json:"input_mode"`
	Sensitivity      *int    `json:"input_sensitivity"`
	SilenceMillis    *int    `json:"input_silence_ms"`
	NoiseSuppression *bool   `json:"input_noise_suppression"`
}

func (p voiceInputPreferencesPatch) validate() error {
	if p.Mode == nil && p.Sensitivity == nil && p.SilenceMillis == nil && p.NoiseSuppression == nil {
		return errors.New("at least one voice input preference is required")
	}
	if p.Mode != nil && *p.Mode != config.VoiceInputModeHandsFree && *p.Mode != config.VoiceInputModeHold {
		return errors.New("voice input mode must be hands_free or hold")
	}
	if p.Sensitivity != nil && (*p.Sensitivity < 1 || *p.Sensitivity > 100) {
		return errors.New("voice input sensitivity must be between 1 and 100")
	}
	if p.SilenceMillis != nil && (*p.SilenceMillis < 300 || *p.SilenceMillis > 3000) {
		return errors.New("voice input silence delay must be between 300 and 3000 milliseconds")
	}
	return nil
}

func (p voiceInputPreferencesPatch) apply(settings *config.Settings) {
	if p.Mode != nil {
		settings.Voice.InputMode = *p.Mode
	}
	if p.Sensitivity != nil {
		settings.Voice.InputSensitivity = *p.Sensitivity
	}
	if p.SilenceMillis != nil {
		settings.Voice.InputSilenceMillis = *p.SilenceMillis
	}
	if p.NoiseSuppression != nil {
		settings.Voice.InputNoiseSuppress = *p.NoiseSuppression
	}
}

func (s *Server) handleVoiceInputPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	var body voiceInputPreferencesPatch
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := body.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_, saved, err := s.store.Update(func(settings config.Settings) (config.Settings, error) {
		body.apply(&settings)
		return settings, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("voice input preferences could not be saved"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"input_mode":              saved.Voice.InputMode,
		"input_sensitivity":       saved.Voice.InputSensitivity,
		"input_silence_ms":        saved.Voice.InputSilenceMillis,
		"input_noise_suppression": saved.Voice.InputNoiseSuppress,
	})
}
