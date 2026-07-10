// Package neuttsworker adapts the non-Python neutts-rs stream_pcm runner to
// MagicHandy's voice worker protocol. Model artifacts must be installed
// explicitly: child processes run with Hugging Face offline mode enabled.
package neuttsworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

const (
	providerName    = "neutts-air"
	providerVersion = "1.0.0"
	queueCapacity   = 8
	maxPCMBytes     = 32 << 20
	audioChunkBytes = 32 << 10
)

// Options configure the neutts-rs stream_pcm adapter.
type Options struct {
	RunnerPath     string
	ReferenceCodes string
	ReferenceText  string
	ReferenceWAV   string // provenance only until neutts-rs ships an encoder
	Backbone       string
	ChunkTokens    int
}

// Run speaks the worker protocol over reader/writer until EOF or shutdown.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	if options.ChunkTokens == 0 {
		options.ChunkTokens = 25
	}
	s := &session{
		options:  options,
		writer:   writer,
		queue:    make(chan protocol.Request, queueCapacity),
		cancels:  make(map[string]context.CancelFunc),
		canceled: make(map[string]bool),
	}
	done := make(chan struct{})
	go func() { defer close(done); s.workLoop() }()
	err := s.readLoop(reader)
	s.setLoaded(false)
	s.cancelAll()
	close(s.queue)
	<-done
	return err
}

type session struct {
	options Options
	writer  io.Writer
	writeMu sync.Mutex

	mu       sync.Mutex
	loaded   bool
	pending  int
	cancels  map[string]context.CancelFunc
	canceled map[string]bool
	queue    chan protocol.Request
}

func (s *session) readLoop(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var request protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			s.sendError("", protocol.ErrorCodeInvalidRequest, "request is not valid JSON", false)
			continue
		}
		switch request.Type {
		case protocol.RequestHello:
			s.hello(request)
		case protocol.RequestHealth:
			s.send(s.health(request.ID))
		case protocol.RequestLoad:
			s.load(request.ID)
		case protocol.RequestUnload:
			s.setLoaded(false)
			s.cancelAll()
			s.send(s.health(request.ID))
		case protocol.RequestCancel:
			s.markCanceled(request.TargetID)
		case protocol.RequestShutdown:
			s.setLoaded(false)
			s.cancelAll()
			s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
			return nil
		case protocol.RequestSpeak:
			s.enqueue(request)
		default:
			s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, fmt.Sprintf("NeuTTS worker cannot handle %q requests", request.Type), false)
		}
	}
	return scanner.Err()
}

func (s *session) hello(request protocol.Request) {
	if request.ProtocolVersion != protocol.Version {
		s.sendError(request.ID, protocol.ErrorCodeProtocolMismatch, fmt.Sprintf("protocol version %d is not supported", request.ProtocolVersion), false)
		return
	}
	s.send(protocol.Response{
		Type: protocol.ResponseHello, RequestID: request.ID,
		ProtocolVersion: protocol.Version, Provider: providerName,
		ProviderVersion: providerVersion, Role: protocol.RoleTTS,
		Capabilities: []string{"cancel", "load", "unload", "local", "offline"},
	})
}

func (s *session) load(id string) {
	if err := validateOptions(s.options); err != nil {
		s.sendError(id, protocol.ErrorCodeMissingDependency, err.Error(), false)
		return
	}
	s.setLoaded(true)
	s.send(s.health(id))
}

func validateOptions(options Options) error {
	if strings.TrimSpace(options.RunnerPath) == "" {
		return errors.New("NeuTTS stream_pcm runner path is required")
	}
	if info, err := os.Stat(options.RunnerPath); err != nil || info.IsDir() {
		return fmt.Errorf("NeuTTS stream_pcm runner is unavailable: %s", options.RunnerPath)
	}
	if strings.TrimSpace(options.ReferenceCodes) == "" {
		return errors.New("pre-encoded NeuCodec .npy reference codes are required; the current non-Python port cannot encode a reference WAV")
	}
	if info, err := os.Stat(options.ReferenceCodes); err != nil || info.IsDir() {
		return fmt.Errorf("NeuTTS reference codes are unavailable: %s", options.ReferenceCodes)
	}
	if strings.TrimSpace(options.ReferenceText) == "" {
		return errors.New("NeuTTS reference transcript is required")
	}
	return nil
}

func (s *session) enqueue(request protocol.Request) {
	if strings.TrimSpace(request.Text) == "" {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, "speak text is required", false)
		return
	}
	if !s.isLoaded() {
		s.sendError(request.ID, protocol.ErrorCodeModelNotLoaded, "NeuTTS model is not loaded", false)
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
		s.sendError(request.ID, protocol.ErrorCodeInternal, "NeuTTS queue is full", true)
	}
}

func (s *session) workLoop() {
	for request := range s.queue {
		s.mu.Lock()
		s.pending--
		canceled := s.canceled[request.ID]
		loaded := s.loaded
		s.mu.Unlock()
		switch {
		case canceled:
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		case !loaded:
			s.sendError(request.ID, protocol.ErrorCodeModelNotLoaded, "NeuTTS model is not loaded", true)
		default:
			s.speak(request)
		}
	}
}

func (s *session) speak(request protocol.Request) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.canceled[request.ID] {
		s.mu.Unlock()
		cancel()
		s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		return
	}
	s.cancels[request.ID] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.cancels, request.ID)
		s.mu.Unlock()
	}()

	args := []string{
		"--codes", s.options.ReferenceCodes,
		"--ref-text", s.options.ReferenceText,
		"--text", request.Text,
		"--chunk", fmt.Sprintf("%d", s.options.ChunkTokens),
	}
	if s.options.Backbone != "" {
		args = append(args, "--backbone", s.options.Backbone)
	}
	// #nosec G204 -- both executable and arguments are explicit local settings,
	// invoked directly without shell expansion.
	command := exec.CommandContext(ctx, s.options.RunnerPath, args...)
	command.Env = offlineEnvironment()
	var stderr tailBuffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		s.sendError(request.ID, protocol.ErrorCodeInternal, "NeuTTS runner stdout is unavailable", false)
		return
	}
	if err = command.Start(); err != nil {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency, "NeuTTS runner could not start: "+err.Error(), true)
		return
	}
	buffer := make([]byte, audioChunkBytes)
	total := 0
	seq := 0
	for {
		n, readErr := stdout.Read(buffer)
		if n > 0 {
			total += n
			if total > maxPCMBytes {
				_ = command.Process.Kill()
				_ = command.Wait()
				s.sendError(request.ID, protocol.ErrorCodeInternal, "NeuTTS runner returned oversized PCM", false)
				return
			}
			s.send(protocol.Response{Type: protocol.ResponseAudioChunk, RequestID: request.ID, Seq: seq, AudioB64: base64.StdEncoding.EncodeToString(buffer[:n]), AudioFormat: "pcm_s16le_24000"})
			seq++
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				_ = command.Process.Kill()
			}
			break
		}
	}
	err = command.Wait()
	if errors.Is(ctx.Err(), context.Canceled) {
		s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		return
	}
	if err != nil {
		message := "NeuTTS synthesis failed"
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			message += ": " + detail
		}
		s.sendError(request.ID, protocol.ErrorCodeInternal, message, true)
		return
	}
	if total == 0 || total%2 != 0 {
		s.sendError(request.ID, protocol.ErrorCodeInternal, "NeuTTS runner returned invalid or oversized PCM", false)
		return
	}
	s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
}

func offlineEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "HF_HUB_OFFLINE") || strings.EqualFold(key, "HF_HUB_DISABLE_PROGRESS_BARS") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HF_HUB_OFFLINE=1", "HF_HUB_DISABLE_PROGRESS_BARS=1")
}

func (s *session) health(id string) protocol.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := protocol.ModelStateUnloaded
	if s.loaded {
		state = protocol.ModelStateReady
	}
	return protocol.Response{Type: protocol.ResponseHealth, RequestID: id, ModelState: state, QueueDepth: s.pending}
}

func (s *session) setLoaded(loaded bool) { s.mu.Lock(); s.loaded = loaded; s.mu.Unlock() }
func (s *session) isLoaded() bool        { s.mu.Lock(); defer s.mu.Unlock(); return s.loaded }

func (s *session) markCanceled(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	if s.canceled == nil {
		s.canceled = make(map[string]bool)
	}
	s.canceled[id] = true
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
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

func (s *session) send(response protocol.Response) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	data, _ := json.Marshal(response)
	_, _ = s.writer.Write(append(data, '\n'))
}

func (s *session) sendError(id, code, message string, retryable bool) {
	s.send(protocol.Response{Type: protocol.ResponseError, RequestID: id, Error: &protocol.WorkerError{Code: code, Message: message, Retryable: retryable}})
}

type tailBuffer struct{ bytes.Buffer }

func (b *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n >= 4096 {
		b.Reset()
		_, _ = b.Buffer.Write(p[n-4096:])
		return n, nil
	}
	if b.Len()+n > 4096 {
		data := append([]byte(nil), b.Bytes()[b.Len()+n-4096:]...)
		b.Reset()
		_, _ = b.Buffer.Write(data)
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
