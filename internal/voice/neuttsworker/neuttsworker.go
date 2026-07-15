// Package neuttsworker adapts the non-Python neutts-rs stream_pcm runner to
// MagicHandy's voice worker protocol. Model artifacts remain external to the
// core; child processes request Hugging Face offline mode where supported.
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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

const (
	providerName      = "neutts-air"
	providerVersion   = "1.0.0"
	defaultBackbone   = "neuphonic/neutts-air-q4-gguf"
	defaultGGUFFile   = "neutts-air-Q4_0.gguf"
	modelProbeTimeout = 5 * time.Second
	queueCapacity     = 8
	maxPCMBytes       = 32 << 20
	audioChunkBytes   = 32 << 10
	runnerDiagnostic  = "NeuCodec decoder:"
)

var errOversizedPCM = errors.New("NeuTTS runner returned oversized PCM")

// Options configure the neutts-rs stream_pcm adapter.
type Options struct {
	RunnerPath     string
	ReferenceCodes string
	ReferenceText  string
	ReferenceWAV   string // provenance only until neutts-rs ships an encoder
	Backbone       string
	GGUFFile       string
	ChunkTokens    int
}

// Run speaks the worker protocol over reader/writer until EOF or shutdown.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	if options.ChunkTokens == 0 {
		options.ChunkTokens = 25
	}
	if options.Backbone == "" {
		options.Backbone = defaultBackbone
	}
	if options.GGUFFile == "" && options.Backbone == defaultBackbone {
		options.GGUFFile = defaultGGUFFile
	}
	if options.GGUFFile == "" {
		options.GGUFFile = cachedBackboneFile(options.Backbone, "")
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
	s.cancelLoad()
	s.loadWG.Wait()
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

	mu         sync.Mutex
	loaded     bool
	loading    bool
	loadSeq    uint64
	loadCancel context.CancelFunc
	loadWG     sync.WaitGroup
	pending    int
	cancels    map[string]context.CancelFunc
	canceled   map[string]bool
	queue      chan protocol.Request
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
			s.cancelLoad()
			s.loadWG.Wait()
			s.setLoaded(false)
			s.cancelAll()
			s.send(s.health(request.ID))
		case protocol.RequestCancel:
			s.markCanceled(request.TargetID)
		case protocol.RequestShutdown:
			s.cancelLoad()
			s.loadWG.Wait()
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
		Capabilities: []string{"cancel", "load", "unload", "local"},
	})
}

func (s *session) load(id string) {
	s.cancelLoad()
	s.loadWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), modelProbeTimeout)
	s.mu.Lock()
	s.loadSeq++
	sequence := s.loadSeq
	s.loaded = false
	s.loading = true
	s.loadCancel = cancel
	s.mu.Unlock()
	s.loadWG.Add(1)
	go func() {
		defer s.loadWG.Done()
		defer cancel()
		err := validateOptions(s.options)
		if err == nil {
			err = probeRunner(ctx, s.options)
		}
		s.mu.Lock()
		current := sequence == s.loadSeq && s.loading
		if current {
			s.loading = false
			s.loadCancel = nil
			s.loaded = err == nil
		}
		s.mu.Unlock()
		if !current || errors.Is(ctx.Err(), context.Canceled) {
			s.sendError(id, protocol.ErrorCodeCanceled, "NeuTTS model load was canceled", false)
			return
		}
		if err != nil {
			s.sendError(id, protocol.ErrorCodeMissingDependency, err.Error(), false)
			return
		}
		s.send(s.health(id))
	}()
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
	if runnerWorkingDirectory(options.RunnerPath) == "" {
		return errors.New("NeuTTS decoder weights are unavailable; place models/neucodec_decoder.safetensors in the stream_pcm project directory")
	}
	if options.GGUFFile == "" || !backboneCached(options.Backbone, options.GGUFFile) {
		return fmt.Errorf("NeuTTS backbone is not cached: %s/%s; run stream_pcm directly once to prepare and verify the model", options.Backbone, options.GGUFFile)
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

	args := runnerArgs(s.options, request.Text)
	// #nosec G204 -- both executable and arguments are explicit local settings,
	// invoked directly without shell expansion.
	command := exec.CommandContext(ctx, s.options.RunnerPath, args...)
	command.Dir = runnerWorkingDirectory(s.options.RunnerPath)
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
	audioReader, err := runnerPCMReader(stdout)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		s.sendError(request.ID, protocol.ErrorCodeInternal, err.Error(), false)
		return
	}
	total, readErr := s.streamPCM(request.ID, audioReader)
	if errors.Is(readErr, errOversizedPCM) {
		_ = command.Process.Kill()
		_ = command.Wait()
		s.sendError(request.ID, protocol.ErrorCodeInternal, errOversizedPCM.Error(), false)
		return
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = command.Process.Kill()
	}
	err = command.Wait()
	if errors.Is(ctx.Err(), context.Canceled) {
		s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		return
	}
	if err != nil || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		message := "NeuTTS synthesis failed"
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			message += ": " + detail
		} else if err == nil {
			message += ": " + readErr.Error()
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

func (s *session) streamPCM(requestID string, audioReader io.Reader) (int, error) {
	buffer := make([]byte, audioChunkBytes)
	total := 0
	seq := 0
	for {
		n, err := audioReader.Read(buffer)
		if n > 0 {
			total += n
			if total > maxPCMBytes {
				return total, errOversizedPCM
			}
			s.send(protocol.Response{
				Type:        protocol.ResponseAudioChunk,
				RequestID:   requestID,
				Seq:         seq,
				AudioB64:    base64.StdEncoding.EncodeToString(buffer[:n]),
				AudioFormat: "pcm_s16le_24000",
			})
			seq++
		}
		if err != nil {
			return total, err
		}
	}
}

func runnerPCMReader(stdout io.Reader) (io.Reader, error) {
	reader := bufio.NewReaderSize(stdout, 512)
	prefix, err := reader.Peek(len(runnerDiagnostic))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return reader, nil
		}
		return nil, fmt.Errorf("NeuTTS runner output is unavailable: %w", err)
	}
	if string(prefix) != runnerDiagnostic {
		return reader, nil
	}
	if _, err = reader.ReadSlice('\n'); err != nil {
		return nil, errors.New("NeuTTS runner returned a malformed stdout diagnostic")
	}
	return reader, nil
}

func runnerArgs(options Options, text string) []string {
	args := []string{
		"--codes", options.ReferenceCodes,
		"--ref-text", options.ReferenceText,
		"--text", text,
		"--chunk", fmt.Sprintf("%d", options.ChunkTokens),
	}
	if options.Backbone != "" {
		args = append(args, "--backbone", options.Backbone)
	}
	if options.GGUFFile != "" {
		args = append(args, "--gguf-file", options.GGUFFile)
	}
	return args
}

func probeRunner(ctx context.Context, options Options) error {
	// #nosec G204 -- executable and arguments are explicit local settings,
	// invoked directly without shell expansion.
	command := exec.CommandContext(ctx, options.RunnerPath, "--help")
	command.Dir = runnerWorkingDirectory(options.RunnerPath)
	command.Env = offlineEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("NeuTTS runner compatibility check timed out")
		}
		message := "NeuTTS runner compatibility check failed"
		if detail := strings.TrimSpace(string(output)); detail != "" {
			message += ": " + detail
		}
		return errors.New(message)
	}
	help := strings.ToLower(string(output))
	if !strings.Contains(help, "stream_pcm") || !strings.Contains(help, "--codes") || !strings.Contains(help, "--ref-text") {
		return errors.New("NeuTTS runner is incompatible: stream_pcm --help did not advertise the required reference-code options")
	}
	return nil
}

func runnerWorkingDirectory(runnerPath string) string {
	directory := filepath.Dir(runnerPath)
	for {
		decoder := filepath.Join(directory, "models", "neucodec_decoder.safetensors")
		if info, err := os.Stat(decoder); err == nil && info.Mode().IsRegular() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}

func backboneCached(repo, filename string) bool {
	return cachedBackboneFile(repo, filename) != ""
}

func cachedBackboneFile(repo, filename string) string {
	if !validHuggingFaceRepo(repo) {
		return ""
	}
	root := strings.TrimSpace(os.Getenv("HF_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		root = filepath.Join(home, ".cache", "huggingface")
	}
	hub := filepath.Join(root, "hub")
	repoRoot := filepath.Join(hub, "models--"+strings.ReplaceAll(repo, "/", "--"))
	// #nosec G304,G703 -- repo is constrained to an ASCII owner/name pair and the
	// resulting path remains under the Hugging Face cache root.
	revision, err := os.ReadFile(filepath.Join(repoRoot, "refs", "main"))
	if err != nil {
		return ""
	}
	commit := strings.TrimSpace(string(revision))
	if commit == "" || strings.ContainsAny(commit, `/\`) {
		return ""
	}
	snapshot := filepath.Join(repoRoot, "snapshots", commit)
	if filename != "" {
		if filepath.Base(filename) != filename || !strings.EqualFold(filepath.Ext(filename), ".gguf") {
			return ""
		}
		// #nosec G703 -- filename is a basename with a .gguf extension and snapshot
		// is rooted in the validated local Hugging Face cache.
		if info, err := os.Stat(filepath.Join(snapshot, filename)); err == nil && info.Mode().IsRegular() {
			return filename
		}
		return ""
	}
	entries, err := os.ReadDir(snapshot)
	if err != nil {
		return ""
	}
	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".gguf") {
			candidates = append(candidates, entry.Name())
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func validHuggingFaceRepo(repo string) bool {
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
	if s.loading {
		state = protocol.ModelStateLoading
	} else if s.loaded {
		state = protocol.ModelStateReady
	}
	return protocol.Response{Type: protocol.ResponseHealth, RequestID: id, ModelState: state, QueueDepth: s.pending}
}

func (s *session) setLoaded(loaded bool) { s.mu.Lock(); s.loaded = loaded; s.mu.Unlock() }
func (s *session) isLoaded() bool        { s.mu.Lock(); defer s.mu.Unlock(); return s.loaded }

func (s *session) cancelLoad() {
	s.mu.Lock()
	s.loadSeq++
	s.loading = false
	s.loaded = false
	cancel := s.loadCancel
	s.loadCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
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
