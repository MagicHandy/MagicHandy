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
	maxVoiceAudioBase64Bytes = 44 << 20
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
		tts.Command = resolveWorkerBinary(settings.TTSWorkerPath, executablePath, dataDir, "voice-neutts-worker")
		tts.Args = []string{"-runner", settings.NeuTTSRunnerPath, "-ref-text", settings.NeuTTSReferenceText, "-backbone", settings.NeuTTSBackbone}
		if settings.NeuTTSReferenceWAV != "" {
			tts.Args = append(tts.Args, "-ref-audio", settings.NeuTTSReferenceWAV)
		}
		if settings.NeuTTSReferenceCodes != "" {
			tts.Args = append(tts.Args, "-ref-codes", settings.NeuTTSReferenceCodes)
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

func isRegularFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
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
	return map[string]any{
		"enabled":          settings.Voice.Enabled,
		"protocol_version": voice.ProtocolVersion,
		"workers":          status,
		"modules": map[string]any{
			"parakeet": inspectParakeetAppModule(settings.Voice.ASRWorkerPath, s.voiceExecutable, s.voiceDataDir),
		},
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

	pending, err := worker.Submit(request)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  err.Error(),
			"worker": worker.Status(),
		})
		return
	}
	s.voice.Track(pending)
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
	s.voice.Configure(voiceManagerConfig(next.Voice, s.voiceExecutable, s.voiceDataDir))
}

func (s *Server) handleVoiceTranscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
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
	if body.AudioB64 == "" {
		writeError(w, http.StatusBadRequest, errors.New("recorded audio is required"))
		return
	}
	if len(body.AudioB64) > maxVoiceAudioBase64Bytes {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("recorded audio exceeds 32 MiB"))
		return
	}
	format := strings.ToLower(strings.TrimSpace(body.AudioFormat))
	switch format {
	case "webm", "ogg", "wav":
		// Supported by MediaRecorder or the silent worker test.
	default:
		writeError(w, http.StatusBadRequest, errors.New("recorded audio format must be webm, ogg, or wav"))
		return
	}
	pending, err := s.voice.Worker(voice.RoleASR).Submit(voice.Request{
		Type: voice.RequestTranscribe, AudioB64: body.AudioB64, AudioFormat: format,
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	s.voice.Track(pending)
	writeJSON(w, http.StatusAccepted, map[string]any{"request": pending.Snapshot()})
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
