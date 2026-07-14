package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

// voiceHealthTimeout bounds the live health probe on the status endpoint so
// a wedged worker cannot stall the settings page.
const voiceHealthTimeout = 3 * time.Second

// voiceModelLoadTimeout bounds the automatic load that follows a
// user-initiated start; managed providers may boot a local server inside it.
const voiceModelLoadTimeout = 30 * time.Second

const (
	maxVoiceAudioBytes       = 32 << 20
	maxVoiceAudioBase64Bytes = ((maxVoiceAudioBytes + 2) / 3) * 4
	maxVoiceRequestBytes     = maxVoiceAudioBase64Bytes + 1024
	managedNeuTTSSource      = "ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f"
	managedNeuTTSRust        = "1.94.0-x86_64-pc-windows-msvc"
	managedNeuTTSBackbone    = "008555972590ff2c599dd43736ba31c81df3f0bf"
	managedNeuTTSBackboneSHA = "bf66dc21b7588fe720cbdfeac1595e7b7c780515f8d8f1ff9a29062e4ac9119e"
	managedNeuTTSCodec       = "30c1fdd19e68aee65d542cf043750d4c0165893e"
	managedNeuTTSCodecSHA    = "30c3ea13ceeb2de693c56e5e33a1b7e00d44c95dcdd08a4ed0d552d0bf59ebdf"
)

func newVoiceManager(settings config.VoiceSettings, executablePath, dataDir string) (*voice.Manager, error) {
	manager := voice.NewManager()
	if err := manager.PrepareTranscriptionStaging(dataDir); err != nil {
		return nil, err
	}
	manager.Configure(voiceManagerConfig(settings, executablePath, dataDir))
	return manager, nil
}

func voiceManagerConfig(settings config.VoiceSettings, executablePath, dataDir string) voice.Config {
	// Provider credentials travel to the worker process privately via its
	// environment — never on the command line (visible in process listings)
	// and never through any status or protocol frame.
	var ttsEnv map[string]string
	if settings.ElevenLabsAPIKey != "" {
		ttsEnv = map[string]string{"ELEVENLABS_API_KEY": settings.ElevenLabsAPIKey}
	}
	tts := voice.WorkerConfig{Enabled: settings.Enabled && settings.TTSProvider != config.VoiceProviderNone}
	switch settings.TTSProvider {
	case config.VoiceTTSProviderElevenLabs:
		tts.Command = resolveWorkerBinary(settings.TTSWorkerPath, executablePath, dataDir, "voice-elevenlabs-worker")
		tts.Args = []string{"-voice-id", settings.ElevenLabsVoiceID, "-model-id", settings.ElevenLabsModelID}
		tts.Env = ttsEnv
	case config.VoiceTTSProviderNeuTTSAir:
		command := resolveWorkerBinary(settings.TTSWorkerPath, executablePath, dataDir, "voice-neutts-worker")
		runner, hfHome := resolveNeuTTSRuntime(settings, dataDir)
		tts.Env = managedNeuTTSEnvironment(settings, hfHome)
		if neuttsRuntimeReady(settings, command, runner, hfHome, dataDir) {
			tts.Command = command
			tts.Args = []string{"-runner", runner, "-ref-text", settings.NeuTTSReferenceText, "-backbone", settings.NeuTTSBackbone, "-ref-codes", settings.NeuTTSReferenceCodes}
			if settings.NeuTTSReferenceWAV != "" {
				tts.Args = append(tts.Args, "-ref-audio", settings.NeuTTSReferenceWAV)
			}
		}
	case config.VoiceProviderCustom:
		tts.Command, tts.Args = settings.TTSWorkerPath, settings.TTSWorkerArgs
		tts.Env = ttsEnv
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

func parakeetAppPaths(dataDir string) (string, string) {
	if strings.TrimSpace(dataDir) == "" {
		return "", ""
	}
	root := filepath.Join(dataDir, "voice", "parakeet")
	return filepath.Join(root, "runner", "parakeet-server.exe"), filepath.Join(root, "tdt-0.6b-v3-q4_k.gguf")
}

func neuttsAppPaths(dataDir string) (string, string) {
	if strings.TrimSpace(dataDir) == "" {
		return "", ""
	}
	root := filepath.Join(dataDir, "voice", "neutts", "active")
	return filepath.Join(root, "runtime", "stream_pcm.exe"), filepath.Join(root, "hf")
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

func resolveNeuTTSRuntime(settings config.VoiceSettings, dataDir string) (string, string) {
	if runner := strings.TrimSpace(settings.NeuTTSRunnerPath); runner != "" {
		return runner, strings.TrimSpace(os.Getenv("HF_HOME"))
	}
	return neuttsAppPaths(dataDir)
}

func managedNeuTTSEnvironment(settings config.VoiceSettings, hfHome string) map[string]string {
	if strings.TrimSpace(settings.NeuTTSRunnerPath) == "" && hfHome != "" {
		return map[string]string{"HF_HOME": hfHome}
	}
	return nil
}

func neuttsRuntimeInstalled(runner, hfHome, backbone string) bool {
	return isRegularFile(runner) &&
		neuttsWorkingDirectory(runner) != "" &&
		neuttsBackboneCached(backbone, hfHome)
}

func fileSHA256(path string) (string, bool) {
	// #nosec G304 -- callers provide server-owned app data paths.
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", false
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), true
}

var managedNeuTTSFileSHA256 = fileSHA256

type managedNeuTTSManifest struct {
	SchemaVersion     int    `json:"schema_version"`
	SourceCommit      string `json:"source_commit"`
	RustToolchain     string `json:"rust_toolchain"`
	BackboneRevision  string `json:"backbone_revision"`
	CodecRevision     string `json:"codec_revision"`
	RunnerSHA256      string `json:"runner_sha256"`
	DecoderSHA256     string `json:"decoder_sha256"`
	BackboneSHA256    string `json:"backbone_sha256"`
	CodecSourceSHA256 string `json:"codec_checkpoint_sha256"`
}

func readManagedNeuTTSManifest(dataDir string) (managedNeuTTSManifest, bool) {
	var manifest managedNeuTTSManifest
	if strings.TrimSpace(dataDir) == "" {
		return manifest, false
	}
	path := filepath.Join(dataDir, "voice", "neutts", "active", "runtime", "runtime.json")
	// #nosec G304 -- dataDir is the server-owned application data root.
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, false
	}
	if json.Unmarshal(data, &manifest) != nil {
		return manifest, false
	}
	valid := manifest.SchemaVersion == 1 &&
		manifest.SourceCommit == managedNeuTTSSource &&
		manifest.RustToolchain == managedNeuTTSRust &&
		manifest.BackboneRevision == managedNeuTTSBackbone &&
		manifest.CodecRevision == managedNeuTTSCodec &&
		manifest.BackboneSHA256 == managedNeuTTSBackboneSHA &&
		manifest.CodecSourceSHA256 == managedNeuTTSCodecSHA &&
		manifest.RunnerSHA256 != "" && manifest.DecoderSHA256 != ""
	return manifest, valid
}

func managedNeuTTSManifestReady(dataDir string, verifyRuntimeHashes bool) bool {
	manifest, valid := readManagedNeuTTSManifest(dataDir)
	if !valid {
		return false
	}
	active := filepath.Join(dataDir, "voice", "neutts", "active")
	backboneRepo := filepath.Join(active, "hf", "hub", "models--neuphonic--neutts-air-q4-gguf")
	// #nosec G304 -- the cache path is rooted in the server-owned app data directory.
	backboneRef, err := os.ReadFile(filepath.Join(backboneRepo, "refs", "main"))
	backbonePath := filepath.Join(backboneRepo, "snapshots", managedNeuTTSBackbone, "neutts-air-Q4_0.gguf")
	if err != nil || strings.TrimSpace(string(backboneRef)) != managedNeuTTSBackbone || !isRegularFile(backbonePath) {
		return false
	}
	if !verifyRuntimeHashes {
		return true
	}
	runtime := filepath.Join(active, "runtime")
	runnerHash, runnerOK := managedNeuTTSFileSHA256(filepath.Join(runtime, "stream_pcm.exe"))
	decoderHash, decoderOK := managedNeuTTSFileSHA256(filepath.Join(runtime, "models", "neucodec_decoder.safetensors"))
	backboneHash, backboneOK := managedNeuTTSFileSHA256(backbonePath)
	return runnerOK && decoderOK && backboneOK && runnerHash == manifest.RunnerSHA256 &&
		decoderHash == manifest.DecoderSHA256 && backboneHash == managedNeuTTSBackboneSHA
}

func neuttsRuntimeReady(settings config.VoiceSettings, command, runner, hfHome, dataDir string) bool {
	customRunner := strings.TrimSpace(settings.NeuTTSRunnerPath) != ""
	managedReady := customRunner || (settings.NeuTTSBackbone == config.DefaultNeuTTSBackbone && managedNeuTTSManifestReady(dataDir, true))
	return managedReady && isRegularFile(command) &&
		neuttsRuntimeInstalled(runner, hfHome, settings.NeuTTSBackbone) &&
		isRegularFile(settings.NeuTTSReferenceCodes) &&
		strings.TrimSpace(settings.NeuTTSReferenceText) != ""
}

type voiceModuleStatus struct {
	State            string `json:"state"`
	Installed        bool   `json:"installed"`
	WorkerInstalled  bool   `json:"worker_installed"`
	RuntimeInstalled bool   `json:"runtime_installed"`
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

func inspectNeuTTSModule(settings config.VoiceSettings, adapterInstalled, configured bool, dataDir string) voiceModuleStatus {
	runner, hfHome := resolveNeuTTSRuntime(settings, dataDir)
	runtimeInstalled := neuttsRuntimeInstalled(runner, hfHome, settings.NeuTTSBackbone)
	if strings.TrimSpace(settings.NeuTTSRunnerPath) == "" {
		runtimeInstalled = runtimeInstalled && settings.NeuTTSBackbone == config.DefaultNeuTTSBackbone && managedNeuTTSManifestReady(dataDir, false)
	}
	status := voiceModuleStatus{
		State:            "missing",
		WorkerInstalled:  adapterInstalled,
		RuntimeInstalled: runtimeInstalled,
		Message:          "NeuTTS is not installed. Rerun update.ps1 with managed llama.cpp enabled; skipping llama.cpp also skips NeuTTS.",
	}
	if adapterInstalled && runtimeInstalled && configured {
		status.State = "ready"
		status.Installed = true
		status.Message = "The NeuTTS adapter, app-managed runtime, reference codes, and transcript are configured."
		return status
	}
	if !adapterInstalled && !runtimeInstalled {
		return status
	}
	status.State = "incomplete"
	switch {
	case !runtimeInstalled && strings.TrimSpace(settings.NeuTTSRunnerPath) == "":
		status.Message = "The app-managed NeuTTS runtime is incomplete. Rerun update.ps1 with managed llama.cpp enabled; skipping llama.cpp also skips NeuTTS."
	case !runtimeInstalled:
		status.Message = "The selected custom NeuTTS runner, decoder, or exact backbone cache entry is unavailable. Re-select the files and save settings."
	case strings.TrimSpace(settings.NeuTTSReferenceCodes) == "":
		status.Message = "The NeuTTS runtime is installed. Add licensed pre-encoded NeuCodec .npy reference codes."
	case strings.TrimSpace(settings.NeuTTSReferenceText) == "":
		status.Message = "The NeuTTS runtime is installed. Add the exact transcript for the selected reference codes."
	default:
		status.Message = "The NeuTTS runtime is installed, but a selected reference file is unavailable. Re-select it and save settings."
	}
	return status
}

func neuttsWorkingDirectory(runnerPath string) string {
	directory := filepath.Dir(runnerPath)
	for {
		if isRegularFile(filepath.Join(directory, "models", "neucodec_decoder.safetensors")) {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}

func neuttsBackboneCached(backbone, hfHome string) bool {
	const filename = "neutts-air-Q4_0.gguf"
	if !validHuggingFaceRepoID(backbone) {
		return false
	}
	root := strings.TrimSpace(hfHome)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("HF_HOME"))
	}
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		root = filepath.Join(home, ".cache", "huggingface")
	}
	repoRoot := filepath.Join(root, "hub", "models--"+strings.ReplaceAll(backbone, "/", "--"))
	// #nosec G304,G703 -- backbone is constrained to an ASCII owner/name pair and the
	// resulting path remains under the Hugging Face cache root.
	revision, err := os.ReadFile(filepath.Join(repoRoot, "refs", "main"))
	if err != nil {
		return false
	}
	commit := strings.TrimSpace(string(revision))
	if commit == "" || strings.ContainsAny(commit, `/\`) {
		return false
	}
	snapshot := filepath.Join(repoRoot, "snapshots", commit)
	if backbone == config.DefaultNeuTTSBackbone {
		return isRegularFile(filepath.Join(snapshot, filename))
	}
	entries, err := os.ReadDir(snapshot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".gguf") {
			return true
		}
	}
	return false
}

func validHuggingFaceRepoID(repo string) bool {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, character := range part {
			switch {
			case character >= 'a' && character <= 'z':
			case character >= 'A' && character <= 'Z':
			case character >= '0' && character <= '9':
			case strings.ContainsRune("-_.", character):
			default:
				return false
			}
		}
	}
	return true
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
	mux.HandleFunc("POST /api/voice/transcriptions", s.handleVoiceTranscription)
	mux.HandleFunc("PUT /api/voice/preferences", s.handleVoicePreferences)
}

// voiceState is the /api/state block: lifecycle snapshots only, no live IPC
// on the polling path.
func (s *Server) voiceState() map[string]any {
	settings, _ := s.store.Snapshot()
	status := s.voice.Status()
	modules := map[string]any{
		"parakeet": inspectParakeetAppModule(settings.Voice.ASRWorkerPath, s.voiceExecutable, s.voiceDataDir),
	}
	if settings.Voice.TTSProvider == config.VoiceTTSProviderNeuTTSAir {
		modules["neutts"] = inspectNeuTTSModule(settings.Voice, s.neuttsAdapterInstalled.Load(), status[voice.RoleTTS].Configured, s.voiceDataDir)
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

// writeStartedWorker follows every user-initiated start with a model load:
// starting a role means making it ready to serve, so replies never fail
// silently with model_not_loaded. Workers are still never started
// implicitly. A load failure stops the just-started worker and fails the
// action, so a successful Start always means the role is ready to serve.
func (s *Server) writeStartedWorker(w http.ResponseWriter, worker *voice.Supervisor) {
	// Detached context: an impatient client disconnect must not abort the load.
	ctx, cancel := context.WithTimeout(context.Background(), voiceModelLoadTimeout)
	defer cancel()
	if _, err := worker.SetModelLoaded(ctx, true); err != nil {
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
		request.Type = voice.RequestSpeak
		request.Text = body.Text
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

// applyVoiceSettingsTransition reconfigures workers after a settings save.
// A changed command or a disable stops the affected worker; nothing starts
// implicitly. Unchanged configs are a no-op inside SetConfig.
func (s *Server) applyVoiceSettingsTransition(next config.Settings) {
	s.neuttsAdapterInstalled.Store(isRegularFile(resolveWorkerBinary(next.Voice.TTSWorkerPath, s.voiceExecutable, s.voiceDataDir, "voice-neutts-worker")))
	s.voice.Configure(voiceManagerConfig(next.Voice, s.voiceExecutable, s.voiceDataDir))
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
	settings, _ := s.store.Snapshot()
	settings.Voice.SpeakReplies = body.SpeakReplies
	if _, err := s.store.Save(settings); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("voice preference could not be saved"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"speak_replies": body.SpeakReplies})
}
