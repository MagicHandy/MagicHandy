// Package stubworker implements the ADR 0003 worker protocol with no models
// attached. It backs cmd/voice-stub-worker and the protocol tests: TTS
// returns a few tiny silent PCM chunks, ASR returns a canned transcript, and
// both honor load state, delays, cancellation, and crash injection.
package stubworker

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

// CrashText makes a speak/transcribe request crash the stub process, so
// supervisor crash handling can be exercised deliberately.
const CrashText = "__stub_crash__"

// ErrCrashRequested is returned by Run when a request asked the stub to
// crash; the command wrapper converts it to a nonzero exit.
var ErrCrashRequested = errors.New("stub worker crash requested")

const (
	speakChunks   = 3
	queueCapacity = 8
)

// Options configure one stub worker session.
type Options struct {
	Role            protocol.Role
	Provider        string
	ProviderVersion string
	StartLoaded     bool

	// AdvertiseProtocolVersion overrides the protocol version reported at
	// hello (0 means the real one) so core-side mismatch rejection can be
	// tested against a live process.
	AdvertiseProtocolVersion int
}

// Run speaks the worker protocol over reader/writer until EOF, a shutdown
// request, or an injected crash. It is transport-agnostic so tests can drive
// it over in-process pipes.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	if options.Provider == "" {
		options.Provider = "magichandy-stub-" + string(options.Role)
	}
	if options.ProviderVersion == "" {
		options.ProviderVersion = "0.1.0"
	}
	if options.AdvertiseProtocolVersion == 0 {
		options.AdvertiseProtocolVersion = protocol.Version
	}

	s := &session{
		options: options,
		writer:  writer,
		loaded:  options.StartLoaded,
		queue:   make(chan protocol.Request, queueCapacity),
		crash:   make(chan struct{}),
		cancels: make(map[string]bool),
	}

	workDone := make(chan struct{})
	go func() {
		defer close(workDone)
		s.workLoop()
	}()

	// The read loop runs aside so an injected crash exits immediately even
	// while stdin is still open; the process wrapper turns that into a
	// nonzero exit, which is the behavior the supervisor tests need.
	readDone := make(chan error, 1)
	go func() { readDone <- s.readLoop(reader) }()

	var readErr error
	select {
	case <-s.crash:
		return ErrCrashRequested
	case readErr = <-readDone:
	}

	close(s.queue)
	select {
	case <-s.crash:
		return ErrCrashRequested
	case <-workDone:
	}
	return readErr
}

type session struct {
	options Options

	writeMu sync.Mutex
	writer  io.Writer

	mu      sync.Mutex
	loaded  bool
	cancels map[string]bool
	pending int

	queue     chan protocol.Request
	crash     chan struct{}
	crashOnce sync.Once
}

// readLoop handles control frames inline and queues work frames so cancel
// and health stay responsive while a speak/transcribe request runs.
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
			s.setLoaded(true)
			s.send(s.healthResponse(request.ID))
		case protocol.RequestUnload:
			s.setLoaded(false)
			s.send(s.healthResponse(request.ID))
		case protocol.RequestCancel:
			s.markCanceled(request.TargetID)
		case protocol.RequestShutdown:
			s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
			return nil
		case protocol.RequestSpeak, protocol.RequestTranscribe:
			s.enqueue(request)
		default:
			s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type), false)
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
		ProtocolVersion: s.options.AdvertiseProtocolVersion,
		Provider:        s.options.Provider,
		ProviderVersion: s.options.ProviderVersion,
		Role:            s.options.Role,
		Capabilities:    []string{"cancel", "load", "unload"},
	})
}

func (s *session) enqueue(request protocol.Request) {
	expected := protocol.RequestSpeak
	if s.options.Role == protocol.RoleASR {
		expected = protocol.RequestTranscribe
	}
	if request.Type != expected {
		message := fmt.Sprintf("%s worker cannot handle %q requests", s.options.Role, request.Type)
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, message, false)
		return
	}

	s.mu.Lock()
	s.pending++
	s.mu.Unlock()
	select {
	case s.queue <- request:
	default:
		s.mu.Lock()
		s.pending--
		s.mu.Unlock()
		s.sendError(request.ID, protocol.ErrorCodeInternal, "stub worker queue is full", true)
	}
}

func (s *session) workLoop() {
	for request := range s.queue {
		s.mu.Lock()
		s.pending--
		canceled := s.cancels[request.ID]
		loaded := s.loaded
		s.mu.Unlock()

		switch {
		case canceled:
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		case request.Text == CrashText || request.AudioRef == CrashText:
			s.crashOnce.Do(func() { close(s.crash) })
			return
		case !loaded:
			s.sendError(request.ID, protocol.ErrorCodeModelNotLoaded, "stub model is not loaded", true)
		case request.Type == protocol.RequestSpeak:
			s.speak(request)
		default:
			s.transcribe(request)
		}
		s.mu.Lock()
		delete(s.cancels, request.ID)
		s.mu.Unlock()
	}
}

func (s *session) speak(request protocol.Request) {
	chunk := base64.StdEncoding.EncodeToString(silentPCM())
	stepDelay := time.Duration(request.DelayMillis) * time.Millisecond / speakChunks
	for seq := 0; seq < speakChunks; seq++ {
		if s.waitOrCanceled(request.ID, stepDelay) {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
			return
		}
		s.send(protocol.Response{
			Type:        protocol.ResponseAudioChunk,
			RequestID:   request.ID,
			Seq:         seq,
			AudioB64:    chunk,
			AudioFormat: "pcm_s16le_24000",
		})
	}
	s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
}

func (s *session) transcribe(request protocol.Request) {
	if s.waitOrCanceled(request.ID, time.Duration(request.DelayMillis)*time.Millisecond) {
		s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		return
	}
	if request.AudioB64 == "" && request.AudioRef == "" {
		// ADR 0003: no-speech audio is rejected, never an empty transcript.
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
			{Text: "stub transcript", Confidence: 0.95},
		},
	})
}

// waitOrCanceled sleeps in small slices so cancellation lands quickly; it
// reports whether the request was canceled.
func (s *session) waitOrCanceled(id string, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		s.mu.Lock()
		canceled := s.cancels[id]
		s.mu.Unlock()
		if canceled {
			return true
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		slice := 10 * time.Millisecond
		if remaining < slice {
			slice = remaining
		}
		time.Sleep(slice)
	}
}

func (s *session) markCanceled(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	s.cancels[id] = true
	s.mu.Unlock()
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

// silentPCM returns a small even-length mono 16-bit PCM segment. The core
// concatenates chunks and adds one WAV header at the playback boundary.
func silentPCM() []byte {
	const samples = 16
	return make([]byte, samples*2)
}
