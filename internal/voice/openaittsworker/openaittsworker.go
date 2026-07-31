// Package openaittsworker adapts an OpenAI-compatible text-to-speech HTTP
// service to MagicHandy's ADR 0003 worker protocol. The model runtime remains
// outside the pure-Go core. A worker may either connect to a user-owned server
// or own one direct child process started on loopback.
package openaittsworker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

const (
	// DefaultResponseFormat is the adapter's broadly playable audio format.
	DefaultResponseFormat = "wav"
	// DefaultHealthPath is used by compatible servers without a custom probe.
	DefaultHealthPath = "/health"
	// DefaultServerPort is the first managed local TTS port.
	DefaultServerPort = 8991

	providerName    = "openai-compatible-tts"
	providerVersion = "1.0.0"

	requestTimeout       = 5 * time.Minute
	externalLoadTimeout  = 15 * time.Second
	managedLoadTimeout   = 15 * time.Minute
	managedProbeInterval = 250 * time.Millisecond
	queueCapacity        = 8
	chunkBytes           = 32 * 1024
	maxSpeechBytes       = 32 << 10
	maxAudioBytes        = 32 << 20
	maxHealthBytes       = 64 << 10
	maxErrorBytes        = 2 << 10
)

var errHealthNotReady = errors.New("health endpoint reports not ready")

// Options configures one worker session. ServerCommand selects managed mode;
// otherwise BaseURL points at an external server that this worker never stops.
type Options struct {
	BaseURL          string
	APIKey           string
	Model            string
	Voice            string
	ResponseFormat   string
	HealthPath       string
	HealthReadyField string
	HTTPClient       *http.Client
	Seed             *uint32
	RandomizeSeed    bool

	ServerCommand string
	ServerArgs    []string
	ServerDir     string
	ServerPort    int
	ServerEnv     map[string]string
}

// Run speaks the worker protocol over reader/writer until EOF or shutdown.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	options = normalizeOptions(options)
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	var server *managedServer
	if options.ServerCommand != "" {
		options.ServerEnv = managedChildEnvironment(options.ServerEnv)
		server = newManagedServer(
			options.ServerCommand,
			options.ServerArgs,
			options.ServerDir,
			options.ServerPort,
			options.ServerEnv,
		)
		options.BaseURL = server.BaseURL()
	}

	s := &session{
		options:  options,
		writer:   writer,
		queue:    make(chan protocol.Request, queueCapacity),
		canceled: make(map[string]bool),
		cancels:  make(map[string]context.CancelFunc),
		server:   server,
		setupErr: validateOptions(options),
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

	server   *managedServer
	setupErr error
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
			s.handleUnload(request)
		case protocol.RequestCancel:
			s.markCanceled(request.TargetID)
		case protocol.RequestShutdown:
			s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
			return nil
		case protocol.RequestSpeak:
			s.enqueue(request)
		default:
			s.sendError(request.ID, protocol.ErrorCodeInvalidRequest,
				fmt.Sprintf("OpenAI-compatible TTS worker cannot handle %q requests", request.Type), false)
		}
	}
	return scanner.Err()
}

func (s *session) handleHello(request protocol.Request) {
	if request.ProtocolVersion != protocol.Version {
		s.sendError(request.ID, protocol.ErrorCodeProtocolMismatch,
			fmt.Sprintf("protocol version %d is not supported; this worker speaks %d",
				request.ProtocolVersion, protocol.Version), false)
		return
	}
	capabilities := []string{"cancel", "load", "unload", "openai-compatible"}
	if s.server != nil {
		capabilities = append(capabilities, "managed-server")
	}
	s.send(protocol.Response{
		Type:            protocol.ResponseHello,
		RequestID:       request.ID,
		ProtocolVersion: protocol.Version,
		Provider:        providerName,
		ProviderVersion: providerVersion,
		Role:            protocol.RoleTTS,
		Capabilities:    capabilities,
	})
}

func (s *session) handleLoad(request protocol.Request) {
	if s.setupErr != nil {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency, s.setupErr.Error(), false)
		return
	}

	timeout := externalLoadTimeout
	if s.server != nil {
		timeout = managedLoadTimeout
		if err := s.server.Start(); err != nil {
			s.sendError(request.ID, protocol.ErrorCodeMissingDependency,
				"local TTS server could not start: "+sanitize(err.Error(), s.options.APIKey), true)
			return
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.waitReady(ctx); err != nil {
		if s.server != nil {
			_ = s.server.Stop()
			err = s.server.failureMessage(err)
		}
		code := protocol.ErrorCodeInternal
		if errors.Is(err, context.DeadlineExceeded) {
			code = protocol.ErrorCodeTimeout
		}
		s.sendError(request.ID, code,
			"TTS server did not become ready: "+sanitize(err.Error(), s.options.APIKey), true)
		return
	}

	s.setLoaded(true)
	s.send(s.healthResponse(request.ID))
}

func (s *session) handleUnload(request protocol.Request) {
	s.setLoaded(false)
	s.cancelAll()
	if s.server != nil {
		if err := s.server.Stop(); err != nil {
			s.sendError(request.ID, protocol.ErrorCodeInternal,
				"stop local TTS server: "+sanitize(err.Error(), s.options.APIKey), true)
			return
		}
	}
	s.send(s.healthResponse(request.ID))
}

func (s *session) waitReady(ctx context.Context) error {
	for {
		if s.server != nil && !s.server.Running() {
			return errors.New("local TTS server exited during startup")
		}
		if err := s.probe(ctx); err == nil {
			return nil
		} else if errors.Is(err, errHealthNotReady) {
			return err
		} else if s.server == nil {
			return err
		}

		timer := time.NewTimer(managedProbeInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *session) probe(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.options.BaseURL+s.options.HealthPath, nil)
	if err != nil {
		return err
	}
	s.authorize(request)
	response, err := s.options.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("health endpoint returned status %d", response.StatusCode)
	}
	if s.options.HealthReadyField == "" {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxHealthBytes))
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxHealthBytes+1))
	if err != nil {
		return fmt.Errorf("read health response: %w", err)
	}
	if len(body) > maxHealthBytes {
		return fmt.Errorf("%w: response exceeds %d KiB", errHealthNotReady, maxHealthBytes>>10)
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("%w: response is not valid JSON", errHealthNotReady)
	}
	value := payload
	for _, segment := range strings.Split(s.options.HealthReadyField, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: field %q is missing", errHealthNotReady, s.options.HealthReadyField)
		}
		value, ok = object[segment]
		if !ok {
			return fmt.Errorf("%w: field %q is missing", errHealthNotReady, s.options.HealthReadyField)
		}
	}
	ready, ok := value.(bool)
	if !ok {
		return fmt.Errorf("%w: field %q is not boolean", errHealthNotReady, s.options.HealthReadyField)
	}
	if !ready {
		return fmt.Errorf("%w: field %q is false", errHealthNotReady, s.options.HealthReadyField)
	}
	return nil
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
		s.sendError(request.ID, protocol.ErrorCodeInternal, "TTS worker queue is full", true)
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
				"the TTS server is not loaded; send load first", true)
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

// speechRequestBody encodes the /v1/audio/speech payload. It reports failures
// on the session itself and returns false when the caller should stop, so the
// distinct error code and retryable flag for each failure stay with the check
// that produces it.
func (s *session) speechRequestBody(request protocol.Request, text string) ([]byte, bool) {
	payload := map[string]any{
		"input":           text,
		"response_format": s.options.ResponseFormat,
	}
	if s.options.Model != "" {
		payload["model"] = s.options.Model
	}
	voice := strings.TrimSpace(request.Voice)
	if voice == "" {
		voice = s.options.Voice
	}
	if voice != "" {
		payload["voice"] = voice
	}
	if s.options.Seed != nil {
		seed := *s.options.Seed
		if s.options.RandomizeSeed {
			var randomBytes [4]byte
			if _, err := rand.Read(randomBytes[:]); err != nil {
				s.sendError(request.ID, protocol.ErrorCodeInternal, "generate random TTS seed", true)
				return nil, false
			}
			seed = binary.LittleEndian.Uint32(randomBytes[:])
		}
		payload["seed"] = seed
	}
	body, err := json.Marshal(payload)
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInternal, "encode TTS request", false)
		return nil, false
	}
	return body, true
}

func (s *session) speak(ctx context.Context, request protocol.Request) {
	text := strings.TrimSpace(request.Text)
	if text == "" {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, "speak text is empty", false)
		return
	}
	if len(text) > maxSpeechBytes {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest,
			fmt.Sprintf("speak text exceeds %d KiB", maxSpeechBytes>>10), false)
		return
	}

	body, ok := s.speechRequestBody(request, text)
	if !ok {
		return
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.options.BaseURL+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInternal, "build TTS request", false)
		return
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", acceptedContentType(s.options.ResponseFormat))
	s.authorize(httpRequest)

	response, err := s.options.HTTPClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil || s.isCanceled(request.ID) {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
			return
		}
		s.sendError(request.ID, protocol.ErrorCodeInternal,
			"TTS request failed: "+sanitize(err.Error(), s.options.APIKey), true)
		return
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBytes))
		s.sendError(request.ID, protocol.ErrorCodeInternal,
			fmt.Sprintf("TTS server returned status %d: %s", response.StatusCode,
				sanitize(strings.TrimSpace(string(detail)), s.options.APIKey)),
			response.StatusCode >= http.StatusInternalServerError)
		return
	}

	audio, err := io.ReadAll(io.LimitReader(response.Body, maxAudioBytes+1))
	if err != nil {
		if ctx.Err() != nil || s.isCanceled(request.ID) {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
			return
		}
		s.sendError(request.ID, protocol.ErrorCodeInternal,
			"TTS audio stream was interrupted: "+sanitize(err.Error(), s.options.APIKey), true)
		return
	}
	if len(audio) > maxAudioBytes {
		s.sendError(request.ID, protocol.ErrorCodeInternal,
			fmt.Sprintf("TTS audio exceeds %d MiB", maxAudioBytes>>20), false)
		return
	}
	if len(audio) == 0 {
		s.sendError(request.ID, protocol.ErrorCodeInternal, "TTS server returned no audio", true)
		return
	}

	format, err := responseAudioFormat(response.Header.Get("Content-Type"), s.options.ResponseFormat)
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInternal, err.Error(), false)
		return
	}
	s.sendAudio(request.ID, audio, format)
}

func (s *session) sendAudio(requestID string, audio []byte, format string) {
	if format == "wav" {
		audio = RepairWAVLengths(audio)
	}
	for seq, offset := 0, 0; offset < len(audio); seq++ {
		end := min(offset+chunkBytes, len(audio))
		s.send(protocol.Response{
			Type:        protocol.ResponseAudioChunk,
			RequestID:   requestID,
			Seq:         seq,
			AudioB64:    base64.StdEncoding.EncodeToString(audio[offset:end]),
			AudioFormat: format,
		})
		offset = end
	}
	s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: requestID})
}

func (s *session) authorize(request *http.Request) {
	if s.options.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+s.options.APIKey)
	}
}

func (s *session) shutdown() {
	s.setLoaded(false)
	s.cancelAll()
	if s.server != nil {
		_ = s.server.Stop()
	}
}

func normalizeOptions(options Options) Options {
	options.BaseURL = strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	options.APIKey = strings.TrimSpace(options.APIKey)
	options.Model = strings.TrimSpace(options.Model)
	options.Voice = strings.TrimSpace(options.Voice)
	options.ResponseFormat = strings.ToLower(strings.TrimSpace(options.ResponseFormat))
	if options.ResponseFormat == "" {
		options.ResponseFormat = DefaultResponseFormat
	}
	options.HealthPath = strings.TrimSpace(options.HealthPath)
	if options.HealthPath == "" {
		options.HealthPath = DefaultHealthPath
	}
	if !strings.HasPrefix(options.HealthPath, "/") {
		options.HealthPath = "/" + options.HealthPath
	}
	options.HealthReadyField = strings.TrimSpace(options.HealthReadyField)
	options.ServerCommand = strings.TrimSpace(options.ServerCommand)
	options.ServerDir = strings.TrimSpace(options.ServerDir)
	if options.ServerPort == 0 {
		options.ServerPort = DefaultServerPort
	}
	return options
}

func managedChildEnvironment(environment map[string]string) map[string]string {
	environment = cloneEnvironment(environment)
	if environment == nil {
		environment = make(map[string]string)
	}
	// The bearer key authenticates the adapter to an external endpoint. The
	// app-managed loopback servers do not use it and must not inherit it.
	environment["OPENAI_TTS_API_KEY"] = ""
	return environment
}

func validateOptions(options Options) error {
	if err := validateServerEndpoint(options); err != nil {
		return err
	}
	return validateRequestOptions(options)
}

func validateServerEndpoint(options Options) error {
	managed := options.ServerCommand != ""
	if managed {
		if options.ServerPort < 1 || options.ServerPort > 65535 {
			return fmt.Errorf("managed TTS port must be between 1 and 65535")
		}
	} else if options.BaseURL == "" {
		return errors.New("TTS base URL is required for an external server")
	}

	parsed, err := url.Parse(options.BaseURL)
	if err != nil || parsed.Host == "" || !parsed.IsAbs() {
		return errors.New("TTS base URL must be an absolute HTTP URL with a host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("TTS base URL scheme must be http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return errors.New("TTS base URL must not contain credentials, a query, or a fragment")
	}
	if managed && parsed.Scheme != "http" {
		return errors.New("managed TTS server must use loopback HTTP")
	}
	return nil
}

func validateRequestOptions(options Options) error {
	if options.RandomizeSeed && options.Seed == nil {
		return errors.New("randomized TTS seed mode requires a configured seed")
	}
	for label, value := range map[string]string{
		"model": options.Model, "voice": options.Voice, "response format": options.ResponseFormat,
		"health path": options.HealthPath, "health ready field": options.HealthReadyField,
	} {
		if len(value) > 512 || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("TTS %s is invalid", label)
		}
	}
	switch options.ResponseFormat {
	case "wav", "mp3", "opus", "aac", "flac":
	default:
		return fmt.Errorf("unsupported TTS response format %q", options.ResponseFormat)
	}
	if strings.ContainsAny(options.HealthPath, "?#") {
		return errors.New("TTS health path must not contain a query or fragment")
	}
	if strings.HasPrefix(options.HealthReadyField, ".") ||
		strings.HasSuffix(options.HealthReadyField, ".") ||
		strings.Contains(options.HealthReadyField, "..") {
		return errors.New("TTS health ready field must be a dot-separated JSON field path")
	}
	return nil
}

func responseAudioFormat(contentType, fallback string) (string, error) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch strings.ToLower(mediaType) {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return "wav", nil
	case "audio/mpeg", "audio/mp3":
		return "mp3", nil
	case "audio/ogg", "audio/opus":
		return "opus", nil
	case "audio/aac":
		return "aac", nil
	case "audio/flac", "audio/x-flac":
		return "flac", nil
	case "", "application/octet-stream":
		return fallback, nil
	case "application/json", "text/plain", "text/html":
		return "", fmt.Errorf("TTS server returned %s instead of audio", mediaType)
	default:
		if strings.HasPrefix(strings.ToLower(mediaType), "audio/") {
			return fallback, nil
		}
		return "", fmt.Errorf("TTS server returned %s instead of audio", mediaType)
	}
}

func acceptedContentType(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}

// RepairWAVLengths replaces streaming sentinel lengths with the actual bounded
// response size. Faster Qwen's streaming WAV uses 0xffffffff because its final
// size is unknown when the first bytes are sent; browsers reject that header
// after MagicHandy has already retained the complete clip.
func RepairWAVLengths(audio []byte) []byte {
	if len(audio) < 12 || len(audio) > maxAudioBytes ||
		string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return audio
	}
	repaired := append([]byte(nil), audio...)
	// #nosec G115 -- audio is bounded to maxAudioBytes above.
	binary.LittleEndian.PutUint32(repaired[4:8], uint32(len(repaired)-8))

	for offset := 12; offset+8 <= len(repaired); {
		size := binary.LittleEndian.Uint32(repaired[offset+4 : offset+8])
		remaining := len(repaired) - (offset + 8)
		// #nosec G115 -- remaining is bounded to maxAudioBytes above.
		remainingSize := uint32(remaining)
		if string(repaired[offset:offset+4]) == "data" {
			if size == ^uint32(0) || size > remainingSize {
				binary.LittleEndian.PutUint32(repaired[offset+4:offset+8], remainingSize)
			}
			break
		}
		if size > remainingSize {
			break
		}
		next := offset + 8 + int(size)
		if size%2 != 0 {
			next++
		}
		offset = next
	}
	return repaired
}

func (s *session) markCanceled(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	s.canceled[id] = true
	if cancel, ok := s.cancels[id]; ok {
		cancel()
	}
	s.mu.Unlock()
}

func (s *session) cancelAll() {
	s.mu.Lock()
	for id, cancel := range s.cancels {
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
	return s.canceled[id]
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

func (s *session) sendError(requestID, code, message string, retryable bool) {
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

func sanitize(text, key string) string {
	if key == "" {
		return text
	}
	return strings.ReplaceAll(text, key, "[redacted]")
}
