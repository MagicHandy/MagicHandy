// Package parakeetworker implements the ADR 0003 worker protocol for ASR by
// proxying to an OpenAI-compatible transcription server. Managed mode runs
// parakeet.cpp; external mode remains compatible with other servers exposing
// POST /v1/audio/transcriptions. Pure Go, no Python, no CGo: the model runtime
// stays in the external server, exactly like the llama.cpp LLM runner.
package parakeetworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

const (
	providerName    = "parakeet-openai-asr"
	providerVersion = "1.0.0"

	requestTimeout       = 60 * time.Second
	loadTimeout          = 30 * time.Second
	managedProbeInterval = 100 * time.Millisecond
	queueCapacity        = 8
	maxAudioBytes        = 32 << 20 // refuse absurd uploads before they hit the server
)

// Options configure one worker session. Exactly one server mode is used:
// external (BaseURL points at an existing OpenAI-compatible server) or managed
// (ServerPath + ServerModel starts the bundled parakeet.cpp server on loopback).
type Options struct {
	// BaseURL of an externally managed OpenAI-compatible server.
	BaseURL string
	// Model is the server-side model name; empty omits the form field. Most
	// local servers have one model loaded, including parakeet.cpp.
	Model      string
	HTTPClient *http.Client

	// ServerPath is the parakeet-server executable for managed mode.
	ServerPath string
	// ServerModel is the local GGUF file loaded by a managed server. Downloads
	// stay an explicit installer action; a worker never fetches an alias.
	ServerModel string
	// ServerPort is the managed server's loopback port. Zero uses
	// DefaultServerPort.
	ServerPort int
}

// Run speaks the worker protocol over reader/writer until EOF or shutdown.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: requestTimeout}
	}
	setupErr := validateOptions(options)

	var runner *managedServer
	if usesManagedServer(options) {
		runner = newManagedServer(options.ServerPath, options.ServerModel, options.ServerPort)
		options.BaseURL = runner.BaseURL()
	}

	s := &session{
		options:  options,
		writer:   writer,
		queue:    make(chan protocol.Request, queueCapacity),
		cancels:  make(map[string]context.CancelFunc),
		runner:   runner,
		setupErr: setupErr,
	}

	workDone := make(chan struct{})
	go func() {
		defer close(workDone)
		s.workLoop()
	}()

	readErr := s.readLoop(reader)
	s.shutdown()
	close(s.queue)
	<-workDone
	return readErr
}

type session struct {
	options Options

	writeMu sync.Mutex
	writer  io.Writer

	mu       sync.Mutex
	loaded   bool
	pending  int
	canceled map[string]bool
	cancels  map[string]context.CancelFunc

	queue chan protocol.Request

	runner   *managedServer
	setupErr error
}

func (s *session) readLoop(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var request protocol.Request
		if err := json.Unmarshal(line, &request); err != nil {
			s.sendError("", protocol.ErrorCodeInvalidRequest, "request is not valid JSON", false)
			continue
		}

		switch request.Type {
		case protocol.RequestHello:
			s.handleHello(request)
		case protocol.RequestHealth:
			s.send(s.healthResponse(request.ID))
		case protocol.RequestLoad:
			s.handleLoad(request)
		case protocol.RequestUnload:
			s.handleUnload(request)
		case protocol.RequestCancel:
			s.markCanceled(request.TargetID)
		case protocol.RequestShutdown:
			s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
			return nil
		case protocol.RequestTranscribe:
			s.enqueue(request)
		default:
			message := fmt.Sprintf("parakeet worker cannot handle %q requests", request.Type)
			s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, message, false)
		}
	}
	return scanner.Err()
}

func (s *session) handleHello(request protocol.Request) {
	if request.ProtocolVersion != protocol.Version {
		message := fmt.Sprintf("protocol version %d is not supported; this worker speaks %d",
			request.ProtocolVersion, protocol.Version)
		s.sendError(request.ID, protocol.ErrorCodeProtocolMismatch, message, false)
		return
	}
	s.send(protocol.Response{
		Type:            protocol.ResponseHello,
		RequestID:       request.ID,
		ProtocolVersion: protocol.Version,
		Provider:        providerName,
		ProviderVersion: providerVersion,
		Role:            protocol.RoleASR,
		Capabilities:    []string{"cancel", "load", "unload"},
	})
}

// handleLoad checks the transcription server is reachable so "server not
// running" is an immediate, clear state instead of a failed first dictation.
func (s *session) handleLoad(request protocol.Request) {
	if s.setupErr != nil {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency,
			s.setupErr.Error(), false)
		return
	}

	if s.runner != nil {
		if err := s.runner.Start(); err != nil {
			s.sendError(request.ID, protocol.ErrorCodeMissingDependency, err.Error(), true)
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	defer cancel()
	if err := s.waitForServer(ctx); err != nil {
		if s.runner != nil {
			_ = s.runner.Stop()
		}
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency, err.Error(), true)
		return
	}

	s.setLoaded(true)
	s.send(s.healthResponse(request.ID))
}

func (s *session) handleUnload(request protocol.Request) {
	s.setLoaded(false)
	s.cancelAll()
	if s.runner != nil {
		if err := s.runner.Stop(); err != nil {
			s.sendError(request.ID, protocol.ErrorCodeInternal,
				"stop managed ASR server: "+err.Error(), true)
			return
		}
	}
	s.send(s.healthResponse(request.ID))
}

// waitForServer waits only for a server this worker owns. External servers are
// checked once so Load stays an immediate configuration/readiness signal.
func (s *session) waitForServer(ctx context.Context) error {
	if s.runner == nil {
		return s.checkServer(ctx)
	}

	var lastErr error
	for {
		probeCtx, cancel := context.WithTimeout(ctx, time.Second)
		err := s.checkServer(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if !s.runner.Running() {
			return s.runner.failureMessage(err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("managed ASR server did not become ready: %w", lastErr)
		case <-time.After(managedProbeInterval):
		}
	}
}

// checkServer accepts the two small readiness surfaces used by the supported
// runners. parakeet.cpp exposes /health; many other OpenAI-compatible servers
// expose /v1/models instead. A transcription request is deliberately never
// used as a health probe because it could create a false chat input.
func (s *session) checkServer(ctx context.Context) error {
	healthStatus, err := s.endpointStatus(ctx, "/health")
	if err != nil {
		return fmt.Errorf("ASR server is unreachable at %s: %w", s.options.BaseURL, err)
	}
	if isSuccessStatus(healthStatus) {
		return nil
	}
	if healthStatus != http.StatusNotFound && healthStatus != http.StatusMethodNotAllowed {
		return fmt.Errorf("ASR server health check failed with status %d", healthStatus)
	}

	modelsStatus, err := s.endpointStatus(ctx, "/v1/models")
	if err != nil {
		return fmt.Errorf("ASR server is unreachable at %s: %w", s.options.BaseURL, err)
	}
	if isSuccessStatus(modelsStatus) {
		return nil
	}
	return fmt.Errorf("ASR server at %s does not expose a ready /health or /v1/models endpoint (statuses %d and %d)",
		s.options.BaseURL, healthStatus, modelsStatus)
}

func (s *session) endpointStatus(ctx context.Context, endpoint string) (int, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, s.options.BaseURL+endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build readiness request: %w", err)
	}
	response, err := s.options.HTTPClient.Do(httpRequest)
	if err != nil {
		return 0, err
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode, nil
}

func isSuccessStatus(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func (s *session) enqueue(request protocol.Request) {
	s.mu.Lock()
	s.pending++
	s.mu.Unlock()
	select {
	case s.queue <- request:
	default:
		s.mu.Lock()
		s.pending--
		s.mu.Unlock()
		s.sendError(request.ID, protocol.ErrorCodeInternal, "parakeet worker queue is full", true)
	}
}

func (s *session) workLoop() {
	for request := range s.queue {
		s.mu.Lock()
		s.pending--
		canceled := s.canceledLocked(request.ID)
		loaded := s.loaded
		var ctx context.Context
		var cancel context.CancelFunc
		if !canceled {
			ctx, cancel = context.WithCancel(context.Background())
			s.cancels[request.ID] = cancel
		}
		s.mu.Unlock()

		switch {
		case canceled:
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		case !loaded:
			s.sendError(request.ID, protocol.ErrorCodeModelNotLoaded,
				"the ASR server has not been checked; send load first", true)
		default:
			s.transcribe(ctx, request)
		}

		if cancel != nil {
			cancel()
			s.mu.Lock()
			delete(s.cancels, request.ID)
			s.mu.Unlock()
		}
	}
}

func (s *session) transcribe(ctx context.Context, request protocol.Request) {
	audio, err := s.loadAudio(request)
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, err.Error(), false)
		return
	}
	if len(audio) == 0 {
		// ADR 0003: no audio is a rejection, never an empty transcript.
		s.send(protocol.Response{
			Type:      protocol.ResponseTranscript,
			RequestID: request.ID,
			Rejected:  protocol.RejectedNoSpeech,
		})
		return
	}

	text, err := s.callServer(ctx, audio, request.AudioFormat)
	if err != nil {
		if ctx.Err() != nil || s.isCanceled(request.ID) {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
			return
		}
		s.sendError(request.ID, protocol.ErrorCodeInternal, err.Error(), true)
		return
	}
	if s.isCanceled(request.ID) {
		s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		return
	}

	text = strings.TrimSpace(text)
	if text == "" {
		// Silence or no speech: reject rather than injecting an empty
		// transcript into chat (the exact old-app failure ADR 0003 bans).
		s.send(protocol.Response{
			Type:      protocol.ResponseTranscript,
			RequestID: request.ID,
			Rejected:  protocol.RejectedNoSpeech,
		})
		return
	}
	s.send(protocol.Response{
		Type:      protocol.ResponseTranscript,
		RequestID: request.ID,
		Candidates: []protocol.TranscriptCandidate{
			// OpenAI-compatible servers return one hypothesis without a
			// confidence; report 1.0 and let rejection handle silence.
			{Text: text, Confidence: 1.0},
		},
	})
}

func (s *session) loadAudio(request protocol.Request) ([]byte, error) {
	if request.AudioB64 != "" {
		audio, err := base64.StdEncoding.DecodeString(request.AudioB64)
		if err != nil {
			return nil, fmt.Errorf("audio_b64 is not valid base64")
		}
		if len(audio) > maxAudioBytes {
			return nil, fmt.Errorf("audio_b64 exceeds %d MiB", maxAudioBytes>>20)
		}
		return audio, nil
	}
	if request.AudioRef != "" {
		info, err := os.Stat(request.AudioRef)
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("audio_ref file is unavailable: %s", request.AudioRef)
		}
		if info.Size() > maxAudioBytes {
			return nil, fmt.Errorf("audio_ref file exceeds %d MiB", maxAudioBytes>>20)
		}
		// #nosec G304 -- audio_ref comes from the core over the private
		// stdio protocol, same trust domain as the process itself.
		return os.ReadFile(request.AudioRef)
	}
	return nil, nil
}

// callServer posts multipart audio to /v1/audio/transcriptions and returns
// the transcript text.
func (s *session) callServer(ctx context.Context, audio []byte, format string) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	name := "audio.wav"
	if format != "" && format != "wav" {
		name = "audio." + format
	}
	part, err := form.CreateFormFile("file", name)
	if err != nil {
		return "", fmt.Errorf("build transcription upload: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return "", fmt.Errorf("write transcription upload: %w", err)
	}
	if s.options.Model != "" {
		if err := form.WriteField("model", s.options.Model); err != nil {
			return "", fmt.Errorf("write model field: %w", err)
		}
	}
	if err := form.Close(); err != nil {
		return "", fmt.Errorf("finish transcription upload: %w", err)
	}

	url := s.options.BaseURL + "/v1/audio/transcriptions"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", form.FormDataContentType())

	response, err := s.options.HTTPClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("ASR server request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 400 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return "", fmt.Errorf("ASR server returned status %d: %s", response.StatusCode, detail)
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode transcription response: %w", err)
	}
	return payload.Text, nil
}

func (s *session) markCanceled(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	if s.canceled == nil {
		s.canceled = make(map[string]bool)
	}
	s.canceled[id] = true
	if cancel, ok := s.cancels[id]; ok {
		cancel()
	}
	s.mu.Unlock()
}

func (s *session) cancelAll() {
	s.mu.Lock()
	if s.canceled == nil {
		s.canceled = make(map[string]bool)
	}
	for id, cancel := range s.cancels {
		s.canceled[id] = true
		cancel()
	}
	s.mu.Unlock()
}

func (s *session) shutdown() {
	s.setLoaded(false)
	s.cancelAll()
	if s.runner != nil {
		_ = s.runner.Stop()
	}
}

func (s *session) isCanceled(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canceledLocked(id)
}

func (s *session) canceledLocked(id string) bool {
	return s.canceled != nil && s.canceled[id]
}

func (s *session) setLoaded(loaded bool) {
	s.mu.Lock()
	s.loaded = loaded
	s.mu.Unlock()
}

func (s *session) healthResponse(requestID string) protocol.Response {
	s.mu.Lock()
	loaded := s.loaded
	depth := s.pending
	s.mu.Unlock()

	state := protocol.ModelStateUnloaded
	if loaded {
		state = protocol.ModelStateReady
	}
	return protocol.Response{
		Type:       protocol.ResponseHealth,
		RequestID:  requestID,
		ModelState: state,
		QueueDepth: depth,
	}
}

func (s *session) sendError(requestID string, code string, message string, retryable bool) {
	s.send(protocol.Response{
		Type:      protocol.ResponseError,
		RequestID: requestID,
		Error: &protocol.WorkerError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	})
}

func (s *session) send(response protocol.Response) {
	data, err := json.Marshal(response)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.writer.Write(data)
}

func usesManagedServer(options Options) bool {
	return strings.TrimSpace(options.ServerPath) != "" || strings.TrimSpace(options.ServerModel) != ""
}

func validateOptions(options Options) error {
	managed := usesManagedServer(options)
	if managed && strings.TrimSpace(options.BaseURL) != "" {
		return fmt.Errorf("choose either -base-url or -server-path with -server-model, not both")
	}
	if managed {
		if strings.TrimSpace(options.ServerPath) == "" {
			return fmt.Errorf("managed ASR requires -server-path for parakeet-server")
		}
		if strings.TrimSpace(options.ServerModel) == "" {
			return fmt.Errorf("managed ASR requires -server-model pointing at a local GGUF file")
		}
		if options.ServerPort < 0 || options.ServerPort > 65535 {
			return fmt.Errorf("-server-port must be between 1 and 65535")
		}
		return nil
	}
	if strings.TrimSpace(options.BaseURL) == "" {
		return fmt.Errorf("no ASR server configured; pass -base-url or -server-path with -server-model")
	}
	return nil
}
