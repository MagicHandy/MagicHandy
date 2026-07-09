package parakeetworker

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

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
