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

func TestPersistentRunnerLoadsOnceAndServesMultipleRequests(t *testing.T) {
	startLog := filepath.Join(t.TempDir(), "starts.log")
	t.Setenv("MAGICHANDY_TEST_STARTS", startLog)
	runner := buildPersistentPCMRunner(t)
	codes := filepath.Join(t.TempDir(), "voice.npy")
	if err := os.WriteFile(codes, []byte("codes"), 0o600); err != nil {
		t.Fatal(err)
	}

	worker := startPersistentTestWorker(t, Options{RunnerPath: runner, ReferenceCodes: codes, ReferenceText: "Reference transcript."})

	worker.send(protocol.Request{Type: protocol.RequestLoad, ID: "load"})
	if loaded := worker.next(t); loaded.ModelState != protocol.ModelStateReady {
		t.Fatalf("load = %+v", loaded)
	}
	for _, id := range []string{"first", "second"} {
		assertPersistentSpeech(t, worker, id, "Speak without reloading.")
	}

	worker.send(protocol.Request{Type: protocol.RequestSpeak, ID: "blocked", Text: "block"})
	if response := worker.next(t); response.RequestID != "blocked" || response.Type != protocol.ResponseAudioChunk {
		t.Fatalf("cancel fixture first response = %+v", response)
	}
	worker.send(protocol.Request{Type: protocol.RequestCancel, ID: "cancel", TargetID: "blocked"})
	if response := worker.next(t); response.RequestID != "blocked" || response.Type != protocol.ResponseCanceled {
		t.Fatalf("cancel fixture terminal response = %+v", response)
	}
	assertPersistentSpeech(t, worker, "recovery", "Speak after cancellation.")
	// #nosec G304 -- startLog is a test-owned path under t.TempDir.
	if starts, err := os.ReadFile(startLog); err != nil || string(starts) != "start\n" {
		t.Fatalf("persistent starts = %q, %v; requests and cancellation recovery must reuse the model process", starts, err)
	}

	worker.send(protocol.Request{Type: protocol.RequestUnload, ID: "unload"})
	if unloaded := worker.next(t); unloaded.ModelState != protocol.ModelStateUnloaded {
		t.Fatalf("unload = %+v", unloaded)
	}
	worker.shutdown(t)
}

func TestPersistentRunnerDoesNotCancelCompletedRequest(t *testing.T) {
	var commands bytes.Buffer
	runner := &persistentRunner{stdin: nopWriteCloser{Writer: &commands}}
	ctx, cancel := context.WithCancel(context.Background())
	requestDone := make(chan struct{})
	close(requestDone)
	cancel()

	// Both signals are deliberately ready. The old single select could choose
	// ctx.Done and inject a stale cancel into the following request.
	for range 1000 {
		runner.forwardCancellation(ctx, requestDone, "finished")
	}
	if commands.Len() != 0 {
		t.Fatalf("completed request emitted stale runner command: %q", commands.String())
	}
}

func TestPersistentFrameFitsCoreProtocolRelay(t *testing.T) {
	const coreFrameLimit = 1 << 20
	if encoded := base64.StdEncoding.EncodedLen(persistentFrameMax) + 1024; encoded > coreFrameLimit {
		t.Fatalf("persistent frame relay can exceed core protocol: %d > %d", encoded, coreFrameLimit)
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type persistentTestWorker struct {
	inWriter  *io.PipeWriter
	outWriter *io.PipeWriter
	done      chan error
	frames    chan protocol.Response
}

func startPersistentTestWorker(t *testing.T, options Options) *persistentTestWorker {
	t.Helper()
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	worker := &persistentTestWorker{
		inWriter: inWriter, outWriter: outWriter,
		done: make(chan error, 1), frames: make(chan protocol.Response, 16),
	}
	go func() { worker.done <- Run(inReader, outWriter, options) }()
	go func() {
		scanner := bufio.NewScanner(outReader)
		for scanner.Scan() {
			var response protocol.Response
			if json.Unmarshal(scanner.Bytes(), &response) == nil {
				worker.frames <- response
			}
		}
	}()
	return worker
}

func (w *persistentTestWorker) send(request protocol.Request) {
	data, _ := json.Marshal(request)
	_, _ = w.inWriter.Write(append(data, '\n'))
}

func (w *persistentTestWorker) next(t *testing.T) protocol.Response {
	t.Helper()
	select {
	case response := <-w.frames:
		return response
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for persistent worker")
		return protocol.Response{}
	}
}

func (w *persistentTestWorker) shutdown(t *testing.T) {
	t.Helper()
	w.send(protocol.Request{Type: protocol.RequestShutdown, ID: "shutdown"})
	_ = w.next(t)
	_ = w.inWriter.Close()
	_ = w.outWriter.Close()
	select {
	case err := <-w.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}

func assertPersistentSpeech(t *testing.T, worker *persistentTestWorker, id, text string) {
	t.Helper()
	worker.send(protocol.Request{Type: protocol.RequestSpeak, ID: id, Text: text})
	var audio []byte
	for {
		response := worker.next(t)
		if response.RequestID != id {
			t.Fatalf("response request = %q, want %q", response.RequestID, id)
		}
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
	if string(audio) != "12345678" {
		t.Fatalf("audio = %q", audio)
	}
}

func TestInstalledRunnerCompatibility(t *testing.T) {
	runner := os.Getenv("MAGICHANDY_NEUTTS_RUNNER")
	if runner == "" {
		t.Skip("set MAGICHANDY_NEUTTS_RUNNER to an installed stream_pcm binary")
	}
	ctx, cancel := context.WithTimeout(t.Context(), modelProbeTimeout)
	defer cancel()
	if err := probeRunner(ctx, Options{RunnerPath: runner}); err != nil {
		t.Fatalf("installed runner compatibility: %v", err)
	}
}

func TestRunnerPCMReaderStripsOnlyKnownDiagnostic(t *testing.T) {
	input := runnerDiagnostic + " hidden=1024, depth=12\n" + "12345678"
	reader, err := runnerPCMReader(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := io.ReadAll(reader)
	if err != nil || string(pcm) != "12345678" {
		t.Fatalf("PCM = %q, %v", pcm, err)
	}

	reader, err = runnerPCMReader(strings.NewReader("ordinary pcm bytes"))
	if err != nil {
		t.Fatal(err)
	}
	pcm, _ = io.ReadAll(reader)
	if string(pcm) != "ordinary pcm bytes" {
		t.Fatalf("non-diagnostic prefix was removed: %q", pcm)
	}
}

func TestLoadDoesNotRunSynthesis(t *testing.T) {
	t.Setenv("MAGICHANDY_TEST_BLOCK", "1")
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
	select {
	case response := <-frames:
		if response.ModelState != protocol.ModelStateReady {
			t.Fatalf("load = %+v, want quick compatibility-only readiness", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("load attempted synthesis instead of the compatibility check")
	}
	send(protocol.Request{Type: protocol.RequestShutdown, ID: "shutdown"})
	for {
		select {
		case response := <-frames:
			if response.Type == protocol.ResponseDone && response.RequestID == "shutdown" {
				goto stopped
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown did not finish")
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
		t.Fatal("worker did not exit")
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
  if len(os.Args) == 2 && os.Args[1] == "--help" { fmt.Println("stream_pcm --codes FILE --ref-text TEXT"); return }
	if os.Getenv("MAGICHANDY_TEST_BLOCK") == "1" { time.Sleep(30 * time.Second) }
	_, _ = fmt.Fprintln(os.Stdout, "NeuCodec decoder: hidden=1024, depth=12")
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

func buildPersistentPCMRunner(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	hfHome := t.TempDir()
	t.Setenv("HF_HOME", hfHome)
	source := `package main
import ("bufio"; "encoding/binary"; "encoding/json"; "fmt"; "os")
type command struct { Type string ` + "`json:\"type\"`" + `; ID string ` + "`json:\"id\"`" + `; Text string ` + "`json:\"text\"`" + ` }
func frame(kind byte, payload []byte) {
  _, _ = os.Stdout.Write([]byte{'M','H','T','S',kind})
  var size [4]byte; binary.LittleEndian.PutUint32(size[:], uint32(len(payload))); _, _ = os.Stdout.Write(size[:])
  _, _ = os.Stdout.Write(payload)
}
func main() {
  if len(os.Args) == 2 && os.Args[1] == "--help" { fmt.Println("MAGICHANDY_NEUTTS_STREAM_V1 --serve --codes FILE --ref-text TEXT"); return }
  if os.Getenv("HF_HUB_OFFLINE") != "1" { os.Exit(2) }
  starts := os.Getenv("MAGICHANDY_TEST_STARTS"); file, _ := os.OpenFile(starts, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); _, _ = file.WriteString("start\n"); _ = file.Close()
  fmt.Println("NeuCodec decoder: test fixture")
  fmt.Println("NeuCodec: using Burn wgpu (GPU) backend")
  fmt.Println("NeuCodec: burn/wgpu (GPU) backend ready in 0.01 s")
  frame(1, []byte(` + "`{\"protocol\":1,\"codec\":\"test\"}`" + `))
  commands := make(chan command)
  go func() {
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() { var request command; if json.Unmarshal(scanner.Bytes(), &request) == nil { commands <- request } }
    close(commands)
  }()
  for request := range commands {
    switch request.Type {
    case "speak":
      frame(2, []byte("12345678"))
      if request.Text == "block" {
        for next := range commands {
          if next.Type == "cancel" && next.ID == request.ID { frame(5, nil); break }
          if next.Type == "shutdown" { return }
        }
      } else { frame(3, nil) }
    case "cancel": frame(5, nil)
    case "shutdown": return
    }
  }
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
	name := "persistent-pcm-runner"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	command := exec.Command("go", "build", "-o", path, "main.go") // #nosec G204 -- test-owned paths.
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build persistent runner: %v: %s", err, output)
	}
	return path
}
