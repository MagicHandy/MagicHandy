package parakeetworker

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

func TestMain(m *testing.M) {
	if os.Getenv("MAGICHANDY_TEST_PARAKEET_SERVER") == "1" {
		runManagedServerHelper()
		return
	}
	os.Exit(m.Run())
}

func TestValidateOptionsRejectsUnsafeExternalURL(t *testing.T) {
	for _, baseURL := range []string{"file:///tmp/asr", "http://user:password@127.0.0.1:8990"} {
		if err := validateOptions(Options{BaseURL: baseURL}); err == nil {
			t.Fatalf("unsafe ASR URL %q was accepted", baseURL)
		}
	}
}

// runManagedServerHelper accepts the same launcher arguments as
// parakeet-server but implements only the endpoints needed to prove that the
// worker owns start, readiness, and teardown. It runs in a child test process.
func runManagedServerHelper() {
	host, port, ok := managedServerAddress(os.Args[1:])
	if !ok {
		os.Exit(2)
	}
	if countPath := os.Getenv("MAGICHANDY_TEST_PARAKEET_SERVER_COUNT"); countPath != "" {
		file, err := os.OpenFile(countPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304,G703 -- test helper writes a parent-owned temp path.
		if err == nil {
			_, _ = file.WriteString("start\n")
			_ = file.Close()
		}
	}

	listener, err := net.Listen("tcp4", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /v1/audio/transcriptions", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"text":"managed response"}`))
	})
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Second,
		IdleTimeout:       time.Second,
	}
	_ = server.Serve(listener)
}

func managedServerAddress(args []string) (string, int, bool) {
	host := "127.0.0.1"
	port := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			if i+1 >= len(args) {
				return "", 0, false
			}
			i++
			host = args[i]
		case "--port":
			if i+1 >= len(args) {
				return "", 0, false
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return "", 0, false
			}
			port = value
		}
	}
	return host, port, port > 0
}

type driver struct {
	stdin  *io.PipeWriter
	frames chan protocol.Response
}

func startWorker(t *testing.T, options Options) *driver {
	t.Helper()

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	d := &driver{stdin: stdinWriter, frames: make(chan protocol.Response, 256)}

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = Run(stdinReader, stdoutWriter, options)
		_ = stdoutWriter.Close()
	}()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		scanner := bufio.NewScanner(stdoutReader)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		for scanner.Scan() {
			var response protocol.Response
			if json.Unmarshal(scanner.Bytes(), &response) == nil {
				d.frames <- response
			}
		}
	}()
	t.Cleanup(func() {
		_ = stdinWriter.Close()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
			t.Error("worker did not exit")
		}
		_ = stdoutReader.Close()
		<-readDone
	})
	return d
}

func (d *driver) send(t *testing.T, request protocol.Request) {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if _, err := d.stdin.Write(append(data, '\n')); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func (d *driver) next(t *testing.T) protocol.Response {
	t.Helper()
	select {
	case response := <-d.frames:
		return response
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a worker frame")
		return protocol.Response{}
	}
}

// mockASR is an OpenAI-compatible transcription stand-in recording uploads.
type mockASR struct {
	mu     sync.Mutex
	files  [][]byte
	models []string
	reply  string
	block  chan struct{}
}

func (m *mockASR) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"parakeet-tdt-0.6b-v3"}]}`))
	})
	mux.HandleFunc("POST /v1/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		// #nosec G120 -- test-only mock server; uploads are tiny fixtures.
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		audio, _ := io.ReadAll(file)
		_ = file.Close()

		m.mu.Lock()
		m.files = append(m.files, audio)
		m.models = append(m.models, r.FormValue("model"))
		reply := m.reply
		block := m.block
		m.mu.Unlock()

		if block != nil {
			select {
			case <-r.Context().Done():
				return
			case <-block:
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": reply})
	})
	return mux
}

func newLoadedWorker(t *testing.T, mock *mockASR, model string) *driver {
	t.Helper()
	server := httptest.NewServer(mock.handler())
	t.Cleanup(server.Close)

	d := startWorker(t, Options{BaseURL: server.URL, Model: model})
	d.send(t, protocol.Request{Type: protocol.RequestHello, ID: "h", ProtocolVersion: protocol.Version})
	if hello := d.next(t); hello.Provider != "parakeet-openai-asr" || hello.Role != protocol.RoleASR {
		t.Fatalf("unexpected hello: %+v", hello)
	}
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	if loaded := d.next(t); loaded.Type != protocol.ResponseHealth || loaded.ModelState != protocol.ModelStateReady {
		t.Fatalf("load did not reach ready: %+v", loaded)
	}
	return d
}

func TestLoadWithoutBaseURLIsAClearError(t *testing.T) {
	d := startWorker(t, Options{})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	response := d.next(t)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		response.Error.Code != protocol.ErrorCodeMissingDependency {
		t.Fatalf("missing base-url must be a clear missing_dependency error, got %+v", response)
	}
	if !strings.Contains(response.Error.Message, "-base-url") {
		t.Fatalf("error should name the flag to set: %+v", response.Error)
	}
}

func TestLoadAcceptsParakeetHealthEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	d := startWorker(t, Options{BaseURL: server.URL})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	response := d.next(t)
	if response.Type != protocol.ResponseHealth || response.ModelState != protocol.ModelStateReady {
		t.Fatalf("parakeet /health must make load ready, got %+v", response)
	}
}

func TestTranscribeForwardsAudioAndReturnsCandidates(t *testing.T) {
	mock := &mockASR{reply: "start something gentle"}
	d := newLoadedWorker(t, mock, "parakeet-tdt-0.6b-v3")

	audio := []byte("RIFF-fake-wav-bytes")
	d.send(t, protocol.Request{
		Type: protocol.RequestTranscribe, ID: "t1",
		AudioB64: base64.StdEncoding.EncodeToString(audio), AudioFormat: "wav",
	})
	transcript := d.next(t)
	if transcript.Type != protocol.ResponseTranscript {
		t.Fatalf("frame = %+v, want transcript", transcript)
	}
	if len(transcript.Candidates) != 1 || transcript.Candidates[0].Text != "start something gentle" {
		t.Fatalf("candidates = %+v", transcript.Candidates)
	}
	if transcript.Candidates[0].Confidence <= 0 {
		t.Fatalf("confidence must be reported: %+v", transcript.Candidates[0])
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.files) != 1 || string(mock.files[0]) != string(audio) {
		t.Fatalf("audio bytes not forwarded intact: %d uploads", len(mock.files))
	}
	if mock.models[0] != "parakeet-tdt-0.6b-v3" {
		t.Fatalf("model field = %q", mock.models[0])
	}
}

func TestTranscribeReadsAudioRefFiles(t *testing.T) {
	mock := &mockASR{reply: "from a file"}
	d := newLoadedWorker(t, mock, "")

	path := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(path, []byte("file-audio-bytes"), 0o600); err != nil {
		t.Fatalf("write clip: %v", err)
	}
	d.send(t, protocol.Request{Type: protocol.RequestTranscribe, ID: "t1", AudioRef: path, AudioFormat: "wav"})
	transcript := d.next(t)
	if transcript.Type != protocol.ResponseTranscript || len(transcript.Candidates) != 1 {
		t.Fatalf("frame = %+v, want transcript with one candidate", transcript)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if string(mock.files[0]) != "file-audio-bytes" {
		t.Fatal("audio_ref file contents not forwarded")
	}
}

func TestSilenceIsRejectedNeverEmptyTranscript(t *testing.T) {
	// Server returns whitespace-only text (what Whisper-family servers do
	// on silence): the worker must reject, not emit an empty transcript.
	mock := &mockASR{reply: "   "}
	d := newLoadedWorker(t, mock, "")

	d.send(t, protocol.Request{
		Type: protocol.RequestTranscribe, ID: "t1",
		AudioB64: base64.StdEncoding.EncodeToString([]byte("quiet")), AudioFormat: "wav",
	})
	transcript := d.next(t)
	if transcript.Type != protocol.ResponseTranscript || transcript.Rejected != protocol.RejectedNoSpeech {
		t.Fatalf("whitespace transcript must be rejected as no_speech, got %+v", transcript)
	}
	if len(transcript.Candidates) != 0 {
		t.Fatalf("rejected transcript must carry no candidates: %+v", transcript.Candidates)
	}

	// No audio at all is also a rejection, without calling the server.
	d.send(t, protocol.Request{Type: protocol.RequestTranscribe, ID: "t2"})
	empty := d.next(t)
	if empty.Rejected != protocol.RejectedNoSpeech {
		t.Fatalf("empty audio must be rejected as no_speech, got %+v", empty)
	}
}

func TestCancelInterruptsAnActiveTranscription(t *testing.T) {
	mock := &mockASR{reply: "too late", block: make(chan struct{})}
	d := newLoadedWorker(t, mock, "")

	d.send(t, protocol.Request{
		Type: protocol.RequestTranscribe, ID: "t1",
		AudioB64: base64.StdEncoding.EncodeToString([]byte("long clip")), AudioFormat: "wav",
	})
	// Give the request a moment to reach the blocking server handler.
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	d.send(t, protocol.Request{Type: protocol.RequestCancel, ID: "c1", TargetID: "t1"})

	response := d.next(t)
	if response.Type != protocol.ResponseCanceled {
		t.Fatalf("frame = %+v, want canceled", response)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %s; must abort the in-flight request", elapsed)
	}
	close(mock.block)
}

func TestTranscribeBeforeLoadFails(t *testing.T) {
	mock := &mockASR{reply: "x"}
	server := httptest.NewServer(mock.handler())
	t.Cleanup(server.Close)

	d := startWorker(t, Options{BaseURL: server.URL})
	d.send(t, protocol.Request{
		Type: protocol.RequestTranscribe, ID: "t1",
		AudioB64: base64.StdEncoding.EncodeToString([]byte("x")),
	})
	response := d.next(t)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		response.Error.Code != protocol.ErrorCodeModelNotLoaded {
		t.Fatalf("transcribe before load must be model_not_loaded, got %+v", response)
	}
}

func TestManagedServerLoadsOnceAndStopsOnUnload(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "parakeet.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	port := freeLoopbackPort(t)
	countPath := filepath.Join(t.TempDir(), "starts.txt")
	t.Setenv("MAGICHANDY_TEST_PARAKEET_SERVER", "1")
	t.Setenv("MAGICHANDY_TEST_PARAKEET_SERVER_COUNT", countPath)

	d := startWorker(t, Options{
		ServerPath:  os.Args[0],
		ServerModel: modelPath,
		ServerPort:  port,
	})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l1"})
	if response := d.next(t); response.Type != protocol.ResponseHealth || response.ModelState != protocol.ModelStateReady {
		t.Fatalf("managed load = %+v, want ready health", response)
	}
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l2"})
	if response := d.next(t); response.Type != protocol.ResponseHealth || response.ModelState != protocol.ModelStateReady {
		t.Fatalf("second managed load = %+v, want ready health", response)
	}
	if starts := waitForStartCount(t, countPath); starts != 1 {
		t.Fatalf("managed parakeet starts = %d, want 1", starts)
	}

	d.send(t, protocol.Request{Type: protocol.RequestUnload, ID: "u"})
	if response := d.next(t); response.Type != protocol.ResponseHealth || response.ModelState != protocol.ModelStateUnloaded {
		t.Fatalf("managed unload = %+v, want unloaded health", response)
	}
	waitForFreeLoopbackPort(t, port)
}

func TestManagedServerStopsOnWorkerShutdown(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "parakeet.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	port := freeLoopbackPort(t)
	t.Setenv("MAGICHANDY_TEST_PARAKEET_SERVER", "1")

	d := startWorker(t, Options{ServerPath: os.Args[0], ServerModel: modelPath, ServerPort: port})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	if response := d.next(t); response.ModelState != protocol.ModelStateReady {
		t.Fatalf("managed load = %+v, want ready health", response)
	}
	d.send(t, protocol.Request{Type: protocol.RequestShutdown, ID: "s"})
	if response := d.next(t); response.Type != protocol.ResponseDone {
		t.Fatalf("shutdown = %+v, want done", response)
	}
	waitForFreeLoopbackPort(t, port)
}

func TestManagedServerReportsBusyPortBeforeStarting(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "parakeet.gguf")
	if err := os.WriteFile(modelPath, []byte("test model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port

	d := startWorker(t, Options{ServerPath: os.Args[0], ServerModel: modelPath, ServerPort: port})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	response := d.next(t)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		response.Error.Code != protocol.ErrorCodeMissingDependency {
		t.Fatalf("busy managed port must be a clear load error, got %+v", response)
	}
	if !strings.Contains(response.Error.Message, "port") {
		t.Fatalf("busy managed port error = %q, want port context", response.Error.Message)
	}
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForFreeLoopbackPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			_ = listener.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("managed server still owns port %d: %v", port, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForStartCount(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(path) // #nosec G304 -- parent test owns this temp path.
		if err == nil {
			count := strings.Count(string(data), "start\n")
			if count > 0 || time.Now().After(deadline) {
				return count
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("read start count: %v", err)
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(25 * time.Millisecond)
	}
}
