// Package elevenlabsworker implements the ADR 0003 worker protocol for the
// ElevenLabs cloud TTS API (ADR 0007's premium non-Python path). It is pure
// Go HTTP — no Python, no CGo — and runs as a separate worker process so a
// cloud outage or bad key can never take the core app down. The API key is
// a private credential: it arrives only via the ELEVENLABS_API_KEY
// environment variable, is sent only as the xi-api-key header, and never
// appears in frames, logs, or errors.
package elevenlabsworker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

// Defaults for the ElevenLabs API. Voice and model are deliberately
// overridable worker arguments — the app's settings surface stays minimal.
const (
	DefaultBaseURL      = "https://api.elevenlabs.io"
	DefaultVoiceID      = "21m00Tcm4TlvDq8ikWAM" // "Rachel", a stock voice
	DefaultModelID      = "eleven_multilingual_v2"
	DefaultOutputFormat = "mp3_44100_128"

	providerName    = "elevenlabs"
	providerVersion = "1.0.0"

	requestTimeout = 30 * time.Second
	chunkBytes     = 32 * 1024
	queueCapacity  = 8
	maxSpeechBytes = 32 << 10
	maxAudioBytes  = 32 << 20
)

// Options configure one worker session.
type Options struct {
	APIKey       string
	VoiceID      string
	ModelID      string
	BaseURL      string
	OutputFormat string
	HTTPClient   *http.Client
}

// Run speaks the worker protocol over reader/writer until EOF or shutdown.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	if options.VoiceID == "" {
		options.VoiceID = DefaultVoiceID
	}
	if options.ModelID == "" {
		options.ModelID = DefaultModelID
	}
	if options.BaseURL == "" {
		options.BaseURL = DefaultBaseURL
	}
	options.BaseURL = strings.TrimRight(options.BaseURL, "/")
	if options.OutputFormat == "" {
		options.OutputFormat = DefaultOutputFormat
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	s := &session{
		options:  options,
		writer:   writer,
		queue:    make(chan protocol.Request, queueCapacity),
		cancels:  make(map[string]context.CancelFunc),
		setupErr: validateOptions(options),
	}

	workDone := make(chan struct{})
	go func() {
		defer close(workDone)
		s.workLoop()
	}()

	readErr := s.readLoop(reader)
	s.cancelAll()
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
	setupErr error

	queue chan protocol.Request
}

func (s *session) readLoop(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

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
			s.cancelAll()
			s.send(s.healthResponse(request.ID))
		case protocol.RequestCancel:
			s.markCanceled(request.TargetID)
		case protocol.RequestShutdown:
			s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
			return nil
		case protocol.RequestSpeak:
			s.enqueue(request)
		default:
			message := fmt.Sprintf("elevenlabs worker cannot handle %q requests", request.Type)
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
		Role:            protocol.RoleTTS,
		Capabilities:    []string{"cancel", "load", "unload", "cloud"},
	})
}

// handleLoad validates the API key against the account endpoint so a bad or
// missing key is a clear, immediate error instead of a failed first speak.
func (s *session) handleLoad(request protocol.Request) {
	if s.setupErr != nil {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, s.setupErr.Error(), false)
		return
	}
	if s.options.APIKey == "" {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency,
			"ELEVENLABS_API_KEY is not set; add the API key in Settings → Voice", false)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, s.options.BaseURL+"/v1/user", nil)
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInternal, "build key check request", false)
		return
	}
	httpRequest.Header.Set("xi-api-key", s.options.APIKey)

	response, err := s.options.HTTPClient.Do(httpRequest)
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInternal,
			"ElevenLabs is unreachable: "+sanitize(err.Error(), s.options.APIKey), true)
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency,
			"ElevenLabs rejected the API key (401); check the key in Settings → Voice", false)
		return
	}
	if response.StatusCode >= 400 {
		s.sendError(request.ID, protocol.ErrorCodeInternal,
			fmt.Sprintf("ElevenLabs key check failed with status %d", response.StatusCode), true)
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
		s.sendError(request.ID, protocol.ErrorCodeInternal, "elevenlabs worker queue is full", true)
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
				"the ElevenLabs key has not been validated; send load first", true)
		default:
			s.speak(ctx, request)
		}

		s.mu.Lock()
		if cancel != nil {
			cancel()
			delete(s.cancels, request.ID)
		}
		delete(s.canceled, request.ID)
		s.mu.Unlock()
	}
}

// speak streams one sentence at a time so first audio arrives quickly and
// cancellation lands between (or inside) sentence requests.
func (s *session) speak(ctx context.Context, request protocol.Request) {
	if len(request.Text) > maxSpeechBytes {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest,
			fmt.Sprintf("speak text exceeds %d KiB", maxSpeechBytes>>10), false)
		return
	}
	sentences := SplitSentences(request.Text)
	if len(sentences) == 0 {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, "speak text is empty", false)
		return
	}

	seq := 0
	total := 0
	for _, sentence := range sentences {
		if s.isCanceled(request.ID) || ctx.Err() != nil {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
			return
		}
		ok := s.streamSentence(ctx, request.ID, sentence, &seq, &total)
		if !ok {
			return
		}
	}
	s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
}

// streamSentence performs one TTS HTTP call and relays its byte stream as
// audio_chunk frames; false means the request already terminated (error or
// cancel frame sent).
func (s *session) streamSentence(ctx context.Context, requestID string, sentence string, seq, total *int) bool {
	body, err := json.Marshal(map[string]any{
		"text":     sentence,
		"model_id": s.options.ModelID,
	})
	if err != nil {
		s.sendError(requestID, protocol.ErrorCodeInternal, "encode TTS request", false)
		return false
	}

	endpoint := fmt.Sprintf("%s/v1/text-to-speech/%s/stream?output_format=%s",
		s.options.BaseURL, url.PathEscape(s.options.VoiceID), url.QueryEscape(s.options.OutputFormat))
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		s.sendError(requestID, protocol.ErrorCodeInternal, "build TTS request", false)
		return false
	}
	httpRequest.Header.Set("xi-api-key", s.options.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := s.options.HTTPClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: requestID})
			return false
		}
		s.sendError(requestID, protocol.ErrorCodeInternal,
			"ElevenLabs request failed: "+sanitize(err.Error(), s.options.APIKey), true)
		return false
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		code := protocol.ErrorCodeInternal
		if response.StatusCode == http.StatusUnauthorized {
			code = protocol.ErrorCodeMissingDependency
		}
		s.sendError(requestID, code, fmt.Sprintf("ElevenLabs returned status %d: %s",
			response.StatusCode, sanitize(string(detail), s.options.APIKey)), response.StatusCode >= 500)
		return false
	}

	buffer := make([]byte, chunkBytes)
	format := formatLabel(s.options.OutputFormat)
	for {
		if s.isCanceled(requestID) {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: requestID})
			return false
		}
		n, err := response.Body.Read(buffer)
		if n > 0 {
			*total += n
			if *total > maxAudioBytes {
				s.sendError(requestID, protocol.ErrorCodeInternal,
					fmt.Sprintf("ElevenLabs audio exceeds %d MiB", maxAudioBytes>>20), false)
				return false
			}
			s.send(protocol.Response{
				Type:        protocol.ResponseAudioChunk,
				RequestID:   requestID,
				Seq:         *seq,
				AudioB64:    base64.StdEncoding.EncodeToString(buffer[:n]),
				AudioFormat: format,
			})
			*seq++
		}
		if err == io.EOF {
			return true
		}
		if err != nil {
			if ctx.Err() != nil {
				s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: requestID})
				return false
			}
			s.sendError(requestID, protocol.ErrorCodeInternal,
				"ElevenLabs stream interrupted: "+sanitize(err.Error(), s.options.APIKey), true)
			return false
		}
	}
}

func validateOptions(options Options) error {
	parsed, err := url.Parse(options.BaseURL)
	if err != nil || parsed.Host == "" || !parsed.IsAbs() {
		return fmt.Errorf("ElevenLabs base URL must be an absolute HTTP URL with a host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("ElevenLabs base URL scheme must be http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return fmt.Errorf("ElevenLabs base URL must not contain credentials, a query, or a fragment")
	}
	for label, value := range map[string]string{
		"voice ID": options.VoiceID, "model ID": options.ModelID, "output format": options.OutputFormat,
	} {
		if strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("ElevenLabs %s is invalid", label)
		}
	}
	return nil
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
	for id, cancel := range s.cancels {
		if s.canceled == nil {
			s.canceled = make(map[string]bool)
		}
		s.canceled[id] = true
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

// sanitize scrubs the API key from any text that could reach a frame or log.
func sanitize(text string, key string) string {
	if key == "" {
		return text
	}
	return strings.ReplaceAll(text, key, "[redacted]")
}

func formatLabel(outputFormat string) string {
	switch {
	case strings.HasPrefix(outputFormat, "mp3"):
		return "mp3"
	case strings.HasPrefix(outputFormat, "opus"):
		return "opus"
	case strings.HasPrefix(outputFormat, "pcm"):
		return "pcm"
	default:
		return outputFormat
	}
}

// SplitSentences breaks text on sentence punctuation so synthesis can start
// on the first sentence while later ones render. Very short fragments merge
// forward to avoid choppy one-word requests.
func SplitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var sentences []string
	var current strings.Builder
	for _, r := range text {
		current.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '…' {
			sentence := strings.TrimSpace(current.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			current.Reset()
		}
	}
	if tail := strings.TrimSpace(current.String()); tail != "" {
		sentences = append(sentences, tail)
	}

	// Merge fragments under ~20 chars into their successor so abbreviations
	// and interjections do not become separate API calls.
	merged := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		if len(merged) > 0 && len(merged[len(merged)-1]) < 20 {
			merged[len(merged)-1] += " " + sentence
			continue
		}
		merged = append(merged, sentence)
	}
	return merged
}
