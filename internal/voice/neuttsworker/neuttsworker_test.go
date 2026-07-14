package neuttsworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

func TestLoadRequiresPreencodedCodes(t *testing.T) {
	runner := buildPCMRunner(t)
	err := validateOptions(Options{RunnerPath: runner, ReferenceWAV: "voice.wav", ReferenceText: "transcript"})
	if err == nil || !strings.Contains(err.Error(), "pre-encoded") || !strings.Contains(err.Error(), "cannot encode") {
		t.Fatalf("raw WAV limitation must be explicit, got %v", err)
	}
}

func TestCachedBackboneFileDiscoversCustomRepoWithoutNetwork(t *testing.T) {
	hfHome := t.TempDir()
	t.Setenv("HF_HOME", hfHome)
	repoRoot := filepath.Join(hfHome, "hub", "models--example--custom-neutts")
	if err := os.MkdirAll(filepath.Join(repoRoot, "refs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "refs", "main"), []byte("revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(repoRoot, "snapshots", "revision")
	if err := os.MkdirAll(snapshot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "custom-q8.gguf"), []byte("gguf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := cachedBackboneFile("example/custom-neutts", ""); got != "custom-q8.gguf" {
		t.Fatalf("custom cached GGUF = %q", got)
	}
	if got := cachedBackboneFile(`..\escape/repo`, ""); got != "" {
		t.Fatalf("invalid repo escaped cache root: %q", got)
	}
}

func TestWorkerStreamsCompletePCMAndRequestsOfflineMode(t *testing.T) {
	t.Setenv("HF_HUB_OFFLINE", "0")
	runner := buildPCMRunner(t)
	codes := filepath.Join(t.TempDir(), "voice.npy")
	if err := os.WriteFile(codes, []byte("codes"), 0o600); err != nil {
		t.Fatal(err)
	}

	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- Run(inReader, outWriter, Options{RunnerPath: runner, ReferenceCodes: codes, ReferenceText: "Reference transcript."})
	}()
	frames := make(chan protocol.Response, 16)
	go func() {
		scanner := bufio.NewScanner(outReader)
		for scanner.Scan() {
			var response protocol.Response
			if json.Unmarshal(scanner.Bytes(), &response) == nil {
				frames <- response
			}
		}
	}()
	send := func(request protocol.Request) {
		data, _ := json.Marshal(request)
		_, _ = inWriter.Write(append(data, '\n'))
	}
	next := func() protocol.Response {
		select {
		case response := <-frames:
			return response
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for worker")
			return protocol.Response{}
		}
	}

	send(protocol.Request{Type: protocol.RequestHello, ID: "h", ProtocolVersion: protocol.Version})
	if hello := next(); hello.Provider != providerName || hello.Role != protocol.RoleTTS {
		t.Fatalf("hello = %+v", hello)
	}
	send(protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	if loaded := next(); loaded.ModelState != protocol.ModelStateReady {
		t.Fatalf("load = %+v", loaded)
	}
	send(protocol.Request{Type: protocol.RequestSpeak, ID: "s", Text: "The final sentence must complete."})
	var audio []byte
	for {
		response := next()
		if response.Type == protocol.ResponseAudioChunk {
			chunk, err := base64.StdEncoding.DecodeString(response.AudioB64)
			if err != nil {
				t.Fatal(err)
			}
			audio = append(audio, chunk...)
			continue
		}
		if response.Type != protocol.ResponseDone {
			t.Fatalf("terminal = %+v", response)
		}
		break
	}
	if len(audio) != 8 || string(audio) != "12345678" {
		t.Fatalf("streamed PCM tail was cut off: %d bytes %q", len(audio), audio)
	}

	send(protocol.Request{Type: protocol.RequestShutdown, ID: "x"})
	_ = next()
	_ = inWriter.Close()
	_ = outWriter.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestShutdownCancelsReadinessProbe(t *testing.T) {
	startedPath := filepath.Join(t.TempDir(), "probe-started")
	t.Setenv("MAGICHANDY_TEST_BLOCK", "1")
	t.Setenv("MAGICHANDY_TEST_STARTED", startedPath)
	runner := buildPCMRunner(t)
	codes := filepath.Join(t.TempDir(), "voice.npy")
	if err := os.WriteFile(codes, []byte("codes"), 0o600); err != nil {
		t.Fatal(err)
	}
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- Run(inReader, outWriter, Options{RunnerPath: runner, ReferenceCodes: codes, ReferenceText: "Reference transcript."})
	}()
	frames := make(chan protocol.Response, 4)
	go func() {
		scanner := bufio.NewScanner(outReader)
		for scanner.Scan() {
			var response protocol.Response
			if json.Unmarshal(scanner.Bytes(), &response) == nil {
				frames <- response
			}
		}
	}()
	send := func(request protocol.Request) {
		data, _ := json.Marshal(request)
		_, _ = inWriter.Write(append(data, '\n'))
	}
	send(protocol.Request{Type: protocol.RequestLoad, ID: "load"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("readiness probe did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	send(protocol.Request{Type: protocol.RequestShutdown, ID: "shutdown"})
	for {
		select {
		case response := <-frames:
			if response.Type == protocol.ResponseDone && response.RequestID == "shutdown" {
				goto stopped
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown did not cancel the readiness probe")
		}
	}

stopped:
	_ = inWriter.Close()
	_ = outWriter.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not exit after readiness cancellation")
	}
}

func TestQueuedRequestCanBeCanceledBeforeRunnerStarts(t *testing.T) {
	var output bytes.Buffer
	s := &session{
		writer:   &output,
		loaded:   true,
		pending:  1,
		queue:    make(chan protocol.Request, 1),
		cancels:  make(map[string]context.CancelFunc),
		canceled: make(map[string]bool),
	}
	s.queue <- protocol.Request{Type: protocol.RequestSpeak, ID: "queued", Text: "Do not run."}
	close(s.queue)
	s.markCanceled("queued")
	s.workLoop()

	var response protocol.Response
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Type != protocol.ResponseCanceled || response.RequestID != "queued" {
		t.Fatalf("response = %+v, want queued cancellation", response)
	}
}

func buildPCMRunner(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	hfHome := t.TempDir()
	t.Setenv("HF_HOME", hfHome)
	source := `package main
import ("fmt"; "os"; "time")
func main() {
  if os.Getenv("HF_HUB_OFFLINE") != "1" { fmt.Fprintln(os.Stderr, "offline mode missing"); os.Exit(2) }
  if _, err := os.Stat("models/neucodec_decoder.safetensors"); err != nil { fmt.Fprintln(os.Stderr, "runner working directory is wrong"); os.Exit(3) }
  if os.Getenv("MAGICHANDY_TEST_BLOCK") == "1" { _ = os.WriteFile(os.Getenv("MAGICHANDY_TEST_STARTED"), []byte("started"), 0600); time.Sleep(30 * time.Second) }
  _, _ = os.Stdout.Write([]byte("12345678"))
}`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	modelDir := filepath.Join(dir, "models")
	if err := os.MkdirAll(modelDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "neucodec_decoder.safetensors"), []byte("decoder"), 0o600); err != nil {
		t.Fatal(err)
	}
	backboneRoot := filepath.Join(hfHome, "hub", "models--neuphonic--neutts-air-q4-gguf")
	if err := os.MkdirAll(filepath.Join(backboneRoot, "refs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backboneRoot, "refs", "main"), []byte("test-revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(backboneRoot, "snapshots", "test-revision")
	if err := os.MkdirAll(snapshot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, defaultGGUFFile), []byte("gguf"), 0o600); err != nil {
		t.Fatal(err)
	}
	name := "pcm-runner"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	// #nosec G204 -- the executable is the fixed Go tool and all paths are
	// test-owned values under t.TempDir, passed without shell expansion.
	command := exec.Command("go", "build", "-o", path, "main.go")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build runner: %v: %s", err, output)
	}
	return path
}
