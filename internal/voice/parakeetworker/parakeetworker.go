// Package parakeetworker implements the ADR 0003 worker protocol for ASR by
// proxying to an OpenAI-compatible transcription server — per ADR 0007,
// Parakeet-TDT through achetronic/parakeet is the recommended engine, but
// any server exposing POST /v1/audio/transcriptions works (sherpa-onnx,
// speaches, whisper servers). Pure Go, no Python, no CGo: the model runtime
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

	requestTimeout = 60 * time.Second
	queueCapacity  = 8
	maxAudioBytes  = 32 << 20 // refuse absurd uploads before they hit the server
)

// Options configure one worker session.
type Options struct {
	// BaseURL of the OpenAI-compatible server (e.g. achetronic/parakeet).
	// Required: there is no safe default port to guess.
	BaseURL string
	// Model is the server-side model name; empty omits the field (most
	// local servers have exactly one model loaded).
	Model      string
	HTTPClient *http.Client
}

// Run speaks the worker protocol over reader/writer until EOF or shutdown.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: requestTimeout}
	}

	s := &session{
		options: options,
		writer:  writer,
		queue:   make(chan protocol.Request, queueCapacity),
		cancels: make(map[string]context.CancelFunc),
	}

	workDone := make(chan struct{})
	go func() {
		defer close(workDone)
		s.workLoop()
	}()

	readErr := s.readLoop(reader)
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
			s.setLoaded(false)
			s.send(s.healthResponse(request.ID))
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
	if s.options.BaseURL == "" {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency,
			"no ASR server configured; pass -base-url pointing at an OpenAI-compatible transcription server", false)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, s.options.BaseURL+"/v1/models", nil)
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInternal, "build server check request", false)
		return
	}
	response, err := s.options.HTTPClient.Do(httpRequest)
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency,
			"ASR server is unreachable at "+s.options.BaseURL+": "+err.Error(), true)
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 500 {
		s.sendError(request.ID, protocol.ErrorCodeInternal,
			fmt.Sprintf("ASR server check failed with status %d", response.StatusCode), true)
		return
	}

	s.setLoaded(true)
	s.send(s.healthResponse(request.ID))
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
