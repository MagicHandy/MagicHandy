package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

// voiceHealthTimeout bounds the live health probe on the status endpoint so
// a wedged worker cannot stall the settings page.
const voiceHealthTimeout = 3 * time.Second

func newVoiceManager(settings config.VoiceSettings) *voice.Manager {
	manager := voice.NewManager()
	manager.Configure(voiceManagerConfig(settings))
	return manager
}

func voiceManagerConfig(settings config.VoiceSettings) voice.Config {
	// Provider credentials travel to the worker process privately via its
	// environment — never on the command line (visible in process listings)
	// and never through any status or protocol frame.
	var ttsEnv map[string]string
	if settings.ElevenLabsAPIKey != "" {
		ttsEnv = map[string]string{"ELEVENLABS_API_KEY": settings.ElevenLabsAPIKey}
	}
	return voice.Config{
		TTS: voice.WorkerConfig{
			Enabled: settings.Enabled,
			Command: settings.TTSWorkerPath,
			Args:    settings.TTSWorkerArgs,
			Env:     ttsEnv,
		},
		ASR: voice.WorkerConfig{
			Enabled: settings.Enabled,
			Command: settings.ASRWorkerPath,
			Args:    settings.ASRWorkerArgs,
		},
	}
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
	writeJSON(w, http.StatusOK, map[string]any{"worker": worker.Status()})
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
	ctx, cancel := context.WithTimeout(r.Context(), voiceHealthTimeout)
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
	s.voice.Configure(voiceManagerConfig(next.Voice))
}
