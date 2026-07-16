package elevenlabsworker

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

const testKey = "el-secret-test-key-123"

// driver runs one worker session over in-process pipes and captures every
// raw outbound frame so tests can assert the API key never leaks.
type driver struct {
	stdin  *io.PipeWriter
	frames chan protocol.Response

	mu  sync.Mutex
	raw []string
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
			line := scanner.Text()
			d.mu.Lock()
			d.raw = append(d.raw, line)
			d.mu.Unlock()
			var response protocol.Response
			if json.Unmarshal([]byte(line), &response) == nil {
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

		// The key must never appear in any outbound frame.
		d.mu.Lock()
		defer d.mu.Unlock()
		for _, line := range d.raw {
			if strings.Contains(line, testKey) {
				t.Errorf("API key leaked into a protocol frame: %s", line)
			}
		}
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

func (d *driver) next(t *testing.T, timeout time.Duration) protocol.Response {
	t.Helper()
	select {
	case response := <-d.frames:
		return response
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a worker frame")
		return protocol.Response{}
	}
}

func (d *driver) nextTerminal(t *testing.T) (protocol.Response, int) {
	t.Helper()
	chunks := 0
	for {
		response := d.next(t, 5*time.Second)
		if response.Type == protocol.ResponseAudioChunk {
			chunks++
			continue
		}
		if response.Terminal() {
			return response, chunks
		}
	}
}

// mockAPI is a minimal ElevenLabs stand-in: /v1/user validates the key;
// the TTS stream endpoint returns deterministic bytes per sentence call.
type mockAPI struct {
	mu        sync.Mutex
	ttsBodies []string
	block     chan struct{} // when set, TTS handlers block until ctx cancel
}

func (m *mockAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /v1/text-to-speech/{voice}/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.ttsBodies = append(m.ttsBodies, string(body))
		block := m.block
		m.mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("AUDIO-BYTES-"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if block != nil {
			// Simulate a long stream: hold until the client cancels.
			select {
			case <-r.Context().Done():
				return
			case <-block:
			}
		}
		_, _ = w.Write([]byte("MORE-AUDIO"))
	})
	return mux
}

func (m *mockAPI) bodies() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.ttsBodies...)
}

func newLoadedWorker(t *testing.T, mock *mockAPI) *driver {
	t.Helper()
	server := httptest.NewServer(mock.handler())
	t.Cleanup(server.Close)

	d := startWorker(t, Options{APIKey: testKey, BaseURL: server.URL})
	d.send(t, protocol.Request{Type: protocol.RequestHello, ID: "h", ProtocolVersion: protocol.Version})
	if hello := d.next(t, 5*time.Second); hello.Provider != "elevenlabs" || hello.Role != protocol.RoleTTS {
		t.Fatalf("unexpected hello: %+v", hello)
	}
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	if loaded := d.next(t, 5*time.Second); loaded.Type != protocol.ResponseHealth || loaded.ModelState != protocol.ModelStateReady {
		t.Fatalf("load did not reach ready: %+v", loaded)
	}
	return d
}

func TestLoadWithoutKeyIsAClearError(t *testing.T) {
	d := startWorker(t, Options{})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	response := d.next(t, 5*time.Second)
	if response.Type != protocol.ResponseError || response.Error == nil {
		t.Fatalf("expected structured error, got %+v", response)
	}
	if response.Error.Code != protocol.ErrorCodeMissingDependency {
		t.Fatalf("error code = %q, want %q", response.Error.Code, protocol.ErrorCodeMissingDependency)
	}
	if !strings.Contains(response.Error.Message, "ELEVENLABS_API_KEY") {
		t.Fatalf("error should name the missing key setting: %+v", response.Error)
	}
}

func TestLoadRejectsUnsafeBaseURL(t *testing.T) {
	d := startWorker(t, Options{APIKey: testKey, BaseURL: "file:///tmp/elevenlabs"})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	response := d.next(t, 5*time.Second)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		!strings.Contains(response.Error.Message, "absolute HTTP URL") {
		t.Fatalf("unsafe base URL response = %+v", response)
	}
}

func TestLoadRejectsInvalidKey(t *testing.T) {
	mock := &mockAPI{}
	server := httptest.NewServer(mock.handler())
	t.Cleanup(server.Close)

	d := startWorker(t, Options{APIKey: "wrong-key", BaseURL: server.URL})
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	response := d.next(t, 5*time.Second)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		response.Error.Code != protocol.ErrorCodeMissingDependency {
		t.Fatalf("401 must map to a clear key error, got %+v", response)
	}
}

func TestSpeakStreamsSentencesAsChunks(t *testing.T) {
	mock := &mockAPI{}
	d := newLoadedWorker(t, mock)

	d.send(t, protocol.Request{
		Type: protocol.RequestSpeak, ID: "s1",
		Text: "First sentence is long enough to stand alone. Second sentence also carries plenty of words.",
	})
	terminal, chunks := d.nextTerminal(t)
	if terminal.Type != protocol.ResponseDone {
		t.Fatalf("terminal frame = %+v, want done", terminal)
	}
	if chunks < 2 {
		t.Fatalf("chunks = %d, want at least one per sentence", chunks)
	}

	bodies := mock.bodies()
	if len(bodies) != 2 {
		t.Fatalf("TTS API calls = %d, want 2 (one per sentence): %v", len(bodies), bodies)
	}
	if !strings.Contains(bodies[0], "First sentence") || !strings.Contains(bodies[1], "Second sentence") {
		t.Fatalf("sentences not forwarded in order: %v", bodies)
	}
	if !strings.Contains(bodies[0], DefaultModelID) {
		t.Fatalf("model id missing from request body: %s", bodies[0])
	}
}

func TestSpeakBeforeLoadFails(t *testing.T) {
	mock := &mockAPI{}
	server := httptest.NewServer(mock.handler())
	t.Cleanup(server.Close)

	d := startWorker(t, Options{APIKey: testKey, BaseURL: server.URL})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "hello"})
	response, _ := d.nextTerminal(t)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		response.Error.Code != protocol.ErrorCodeModelNotLoaded {
		t.Fatalf("speak before load must be model_not_loaded, got %+v", response)
	}
}

func TestCancelInterruptsAnActiveStream(t *testing.T) {
	mock := &mockAPI{block: make(chan struct{})}
	d := newLoadedWorker(t, mock)

	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "One very long sentence that streams forever."})
	// Wait for the first chunk so the HTTP stream is live, then cancel.
	first := d.next(t, 5*time.Second)
	if first.Type != protocol.ResponseAudioChunk {
		t.Fatalf("expected first audio chunk, got %+v", first)
	}
	start := time.Now()
	d.send(t, protocol.Request{Type: protocol.RequestCancel, ID: "c1", TargetID: "s1"})

	terminal, _ := d.nextTerminal(t)
	if terminal.Type != protocol.ResponseCanceled {
		t.Fatalf("terminal frame = %+v, want canceled", terminal)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %s; must interrupt the live stream", elapsed)
	}
	close(mock.block)
}

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"One plain sentence with no terminator", 1},
		{"First full sentence here. Second full sentence here!", 2},
		{"Yes. This merges the tiny fragment forward properly.", 1},
		{"A question then more? Certainly, with a longer clause following it.", 2},
	}
	for _, tc := range cases {
		got := SplitSentences(tc.text)
		if len(got) != tc.want {
			t.Errorf("SplitSentences(%q) = %d parts %v, want %d", tc.text, len(got), got, tc.want)
		}
	}
}

func TestHealthReportsQueueDepth(t *testing.T) {
	mock := &mockAPI{}
	d := newLoadedWorker(t, mock)

	d.send(t, protocol.Request{Type: protocol.RequestHealth, ID: "q"})
	health := d.next(t, 5*time.Second)
	if health.Type != protocol.ResponseHealth || health.ModelState != protocol.ModelStateReady {
		t.Fatalf("health = %+v, want ready", health)
	}
	if health.QueueDepth != 0 {
		t.Fatalf("queue depth = %d, want 0", health.QueueDepth)
	}
}
