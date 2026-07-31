package openaittsworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

const testAPIKey = "tts-secret-test-key"

type workerDriver struct {
	stdin  *io.PipeWriter
	frames chan protocol.Response

	mu  sync.Mutex
	raw []string
}

func startWorker(t *testing.T, options Options) *workerDriver {
	t.Helper()

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	driver := &workerDriver{
		stdin:  stdinWriter,
		frames: make(chan protocol.Response, 256),
	}

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
			driver.mu.Lock()
			driver.raw = append(driver.raw, line)
			driver.mu.Unlock()

			var response protocol.Response
			if json.Unmarshal([]byte(line), &response) == nil {
				driver.frames <- response
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

		driver.mu.Lock()
		defer driver.mu.Unlock()
		for _, line := range driver.raw {
			if strings.Contains(line, testAPIKey) {
				t.Errorf("API key leaked into a protocol frame: %s", line)
			}
		}
	})
	return driver
}

func (d *workerDriver) send(t *testing.T, request protocol.Request) {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if _, err := d.stdin.Write(append(data, '\n')); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func (d *workerDriver) next(t *testing.T, timeout time.Duration) protocol.Response {
	t.Helper()
	select {
	case response := <-d.frames:
		return response
	case <-time.After(timeout):
		t.Fatal("timed out waiting for worker response")
		return protocol.Response{}
	}
}

func (d *workerDriver) terminal(t *testing.T) (protocol.Response, []byte) {
	t.Helper()
	var audio []byte
	for {
		response := d.next(t, 5*time.Second)
		if response.Type == protocol.ResponseAudioChunk {
			chunk, err := base64.StdEncoding.DecodeString(response.AudioB64)
			if err != nil {
				t.Fatalf("decode audio chunk: %v", err)
			}
			audio = append(audio, chunk...)
			continue
		}
		if response.Terminal() {
			return response, audio
		}
	}
}

func loadWorker(t *testing.T, serverURL string) *workerDriver {
	t.Helper()
	driver := startWorker(t, Options{
		BaseURL:        serverURL,
		APIKey:         testAPIKey,
		Model:          "Qwen/Qwen3-TTS-12Hz-0.6B-Base",
		Voice:          "default",
		ResponseFormat: "wav",
	})
	driver.send(t, protocol.Request{
		Type:            protocol.RequestHello,
		ID:              "hello",
		ProtocolVersion: protocol.Version,
	})
	hello := driver.next(t, 5*time.Second)
	if hello.Type != protocol.ResponseHello || hello.Provider != providerName || hello.Role != protocol.RoleTTS {
		t.Fatalf("unexpected hello response: %+v", hello)
	}
	driver.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "load"})
	loaded := driver.next(t, 5*time.Second)
	if loaded.Type != protocol.ResponseHealth || loaded.ModelState != protocol.ModelStateReady {
		t.Fatalf("worker did not become ready: %+v", loaded)
	}
	return driver
}

func TestHealthReadyFieldRequiresTrueBoolean(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		field   string
		wantErr bool
	}{
		{name: "ready", body: `{"loaded":true}`, field: "loaded"},
		{name: "nested ready", body: `{"model":{"loaded":true}}`, field: "model.loaded"},
		{name: "false", body: `{"loaded":false}`, field: "loaded", wantErr: true},
		{name: "missing", body: `{"ready":true}`, field: "loaded", wantErr: true},
		{name: "wrong type", body: `{"loaded":"true"}`, field: "loaded", wantErr: true},
		{name: "invalid JSON", body: `{`, field: "loaded", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.body)
			}))
			t.Cleanup(server.Close)

			options := normalizeOptions(Options{
				BaseURL:          server.URL,
				HealthPath:       "/api/model-info",
				HealthReadyField: test.field,
				HTTPClient:       server.Client(),
			})
			worker := session{options: options}
			err := worker.probe(context.Background())
			if test.wantErr {
				if !errors.Is(err, errHealthNotReady) {
					t.Fatalf("probe error = %v, want health-not-ready", err)
				}
			} else if err != nil {
				t.Fatalf("probe: %v", err)
			}
		})
	}
}

func TestHealthReadyFieldRejectsInvalidPath(t *testing.T) {
	options := normalizeOptions(Options{
		BaseURL:          "http://127.0.0.1:8991",
		HealthReadyField: "model..loaded",
	})
	if err := validateOptions(options); err == nil ||
		!strings.Contains(err.Error(), "dot-separated JSON field path") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestOpenAICompatibleSpeechRequestAndStreamingWAVRepair(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			if r.Header.Get("Authorization") != "Bearer "+testAPIKey {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/audio/speech":
			if r.Header.Get("Authorization") != "Bearer "+testAPIKey {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("Accept"); got != "audio/wav" {
				t.Errorf("Accept = %q, want audio/wav", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Errorf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(streamingTestWAV())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	driver := loadWorker(t, server.URL)
	driver.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "speak", Text: "Hello there."})
	terminal, audio := driver.terminal(t)
	if terminal.Type != protocol.ResponseDone {
		t.Fatalf("speak terminal response = %+v", terminal)
	}
	if requestBody["input"] != "Hello there." ||
		requestBody["model"] != "Qwen/Qwen3-TTS-12Hz-0.6B-Base" ||
		requestBody["voice"] != "default" ||
		requestBody["response_format"] != "wav" {
		t.Fatalf("OpenAI-compatible request = %#v", requestBody)
	}
	if got, want := binary.LittleEndian.Uint32(audio[4:8]), uint32(40); got != want {
		t.Fatalf("RIFF length = %d, want %d", got, want)
	}
	if got, want := binary.LittleEndian.Uint32(audio[40:44]), uint32(4); got != want {
		t.Fatalf("data length = %d, want %d", got, want)
	}
}

func TestSpeakVoiceOverrideAndContentTypeDetection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var requestBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&requestBody)
		if requestBody["voice"] != "session-voice" {
			t.Errorf("voice override = %#v", requestBody["voice"])
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mp3-audio"))
	}))
	t.Cleanup(server.Close)

	driver := loadWorker(t, server.URL)
	driver.send(t, protocol.Request{
		Type:  protocol.RequestSpeak,
		ID:    "speak",
		Text:  "Use this voice.",
		Voice: "session-voice",
	})
	response := driver.next(t, 5*time.Second)
	if response.Type != protocol.ResponseAudioChunk || response.AudioFormat != "mp3" {
		t.Fatalf("audio response = %+v", response)
	}
	if terminal := driver.next(t, 5*time.Second); terminal.Type != protocol.ResponseDone {
		t.Fatalf("terminal response = %+v", terminal)
	}
}

func TestCancelAbortsInFlightHTTPRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		once.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)

	driver := loadWorker(t, server.URL)
	driver.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "slow", Text: "Wait."})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("speech request did not start")
	}
	driver.send(t, protocol.Request{Type: protocol.RequestCancel, ID: "cancel", TargetID: "slow"})
	terminal, _ := driver.terminal(t)
	if terminal.Type != protocol.ResponseCanceled || terminal.RequestID != "slow" {
		t.Fatalf("cancel response = %+v", terminal)
	}
	close(release)
}

func TestServerErrorRedactsBearerKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "provider rejected "+testAPIKey, http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	driver := loadWorker(t, server.URL)
	driver.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "speak", Text: "Hello."})
	response, _ := driver.terminal(t)
	if response.Type != protocol.ResponseError || response.Error == nil {
		t.Fatalf("error response = %+v", response)
	}
	if strings.Contains(response.Error.Message, testAPIKey) || !strings.Contains(response.Error.Message, "[redacted]") {
		t.Fatalf("secret was not redacted: %+v", response.Error)
	}
}

func TestSpeechResponseSizeIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = io.CopyN(w, zeroReader{}, maxAudioBytes+1)
	}))
	t.Cleanup(server.Close)

	driver := loadWorker(t, server.URL)
	driver.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "oversize", Text: "Too much audio."})
	response, _ := driver.terminal(t)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		!strings.Contains(response.Error.Message, "exceeds 32 MiB") {
		t.Fatalf("oversize response = %+v", response)
	}
}

func TestManagedServerStopTerminatesOwnedChild(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	server := newManagedServer(
		executable,
		[]string{"-test.run=^TestManagedServerHelperProcess$"},
		"",
		port,
		map[string]string{"MAGICHANDY_TTS_HELPER_PROCESS": "1"},
	)
	t.Cleanup(func() { _ = server.Stop() })
	if err := server.Start(); err != nil {
		t.Fatalf("start managed server helper: %v", err)
	}
	if !server.Running() {
		t.Fatal("managed server helper exited before Stop")
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("stop managed server helper: %v", err)
	}
	if server.Running() {
		t.Fatal("managed server still reports running after Stop")
	}
}

func TestManagedServerHelperProcess(_ *testing.T) {
	if os.Getenv("MAGICHANDY_TTS_HELPER_PROCESS") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestLoadRejectsUnsafeBaseURL(t *testing.T) {
	driver := startWorker(t, Options{BaseURL: "file:///tmp/tts"})
	driver.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "load"})
	response := driver.next(t, 5*time.Second)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		!strings.Contains(response.Error.Message, "absolute HTTP URL") {
		t.Fatalf("unsafe URL response = %+v", response)
	}
}

func TestAppendEnvironmentReplacesExistingValuesCaseInsensitively(t *testing.T) {
	got := appendEnvironment(
		[]string{"Path=original", "HF_HOME=old-cache", "KEEP=value"},
		map[string]string{
			"hf_home":                 "new-cache",
			"OPENAI_TTS_API_KEY":      "",
			"INVALID=ENVIRONMENT_KEY": "ignored",
		},
	)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "HF_HOME=old-cache") ||
		!strings.Contains(joined, "hf_home=new-cache") ||
		!strings.Contains(joined, "OPENAI_TTS_API_KEY=") ||
		!strings.Contains(joined, "KEEP=value") {
		t.Fatalf("merged environment = %q", got)
	}
}

func TestManagedChildEnvironmentClearsAdapterBearerWithoutMutatingCaller(t *testing.T) {
	original := map[string]string{"HF_HOME": "model-cache"}
	got := managedChildEnvironment(original)
	if got["OPENAI_TTS_API_KEY"] != "" || got["HF_HOME"] != "model-cache" {
		t.Fatalf("managed child environment = %#v", got)
	}
	if _, changed := original["OPENAI_TTS_API_KEY"]; changed {
		t.Fatalf("caller environment was mutated: %#v", original)
	}
}

func TestRepairWAVLengthsLeavesNonWAVUntouched(t *testing.T) {
	audio := []byte("not-a-wave")
	repaired := RepairWAVLengths(audio)
	if !bytes.Equal(repaired, audio) {
		t.Fatalf("non-WAV bytes changed: %q", repaired)
	}
}

func TestResponseAudioFormatRejectsNonAudioContent(t *testing.T) {
	if _, err := responseAudioFormat("image/png", "wav"); err == nil ||
		!strings.Contains(err.Error(), "instead of audio") {
		t.Fatalf("responseAudioFormat error = %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}

func streamingTestWAV() []byte {
	audio := make([]byte, 48)
	copy(audio[0:4], "RIFF")
	binary.LittleEndian.PutUint32(audio[4:8], ^uint32(0))
	copy(audio[8:12], "WAVE")
	copy(audio[12:16], "fmt ")
	binary.LittleEndian.PutUint32(audio[16:20], 16)
	binary.LittleEndian.PutUint16(audio[20:22], 1)
	binary.LittleEndian.PutUint16(audio[22:24], 1)
	binary.LittleEndian.PutUint32(audio[24:28], 24000)
	binary.LittleEndian.PutUint32(audio[28:32], 48000)
	binary.LittleEndian.PutUint16(audio[32:34], 2)
	binary.LittleEndian.PutUint16(audio[34:36], 16)
	copy(audio[36:40], "data")
	binary.LittleEndian.PutUint32(audio[40:44], ^uint32(0))
	copy(audio[44:], []byte{1, 2, 3, 4})
	return audio
}
