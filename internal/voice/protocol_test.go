package voice

import (
	"io"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/stubworker"
)

// startStubConn wires a stub worker to a protocol conn over in-process pipes
// so frame semantics are tested without spawning a process.
func startStubConn(t *testing.T, options stubworker.Options) *conn {
	t.Helper()

	coreToStubReader, coreToStubWriter := io.Pipe()
	stubToCoreReader, stubToCoreWriter := io.Pipe()

	stubDone := make(chan struct{})
	go func() {
		defer close(stubDone)
		_ = stubworker.Run(coreToStubReader, stubToCoreWriter, options)
		_ = stubToCoreWriter.Close()
	}()

	c := newConn(coreToStubWriter, stubToCoreReader)
	t.Cleanup(func() {
		_ = coreToStubWriter.Close()
		select {
		case <-stubDone:
		case <-time.After(5 * time.Second):
			t.Error("stub worker did not exit")
		}
		_ = coreToStubReader.Close()
		select {
		case <-c.closedChan():
		case <-time.After(5 * time.Second):
			t.Error("conn read loop did not exit")
		}
	})
	return c
}

// exchange sends a request and returns frames until the terminal one.
func exchange(t *testing.T, c *conn, request Request) []Response {
	t.Helper()

	responses, release, err := c.register(request.ID)
	if err != nil {
		t.Fatalf("register request %q: %v", request.ID, err)
	}
	defer release()
	if err := c.send(request); err != nil {
		t.Fatalf("send %s request: %v", request.Type, err)
	}

	var frames []Response
	deadline := time.After(5 * time.Second)
	for {
		select {
		case response := <-responses:
			frames = append(frames, response)
			if response.Terminal() {
				return frames
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s response; got %d frames", request.Type, len(frames))
		}
	}
}

func terminalFrame(t *testing.T, frames []Response) Response {
	t.Helper()
	if len(frames) == 0 {
		t.Fatal("no response frames")
	}
	return frames[len(frames)-1]
}

func TestHelloNegotiation(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleTTS})

	hello := terminalFrame(t, exchange(t, c, Request{
		Type: RequestHello, ID: "h1", ProtocolVersion: ProtocolVersion,
	}))
	if hello.Type != ResponseHello {
		t.Fatalf("hello response type = %q, want %q", hello.Type, ResponseHello)
	}
	if hello.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocol version = %d, want %d", hello.ProtocolVersion, ProtocolVersion)
	}
	if hello.Provider == "" || hello.ProviderVersion == "" {
		t.Fatalf("hello must identify provider name and version, got %+v", hello)
	}
	if hello.Role != RoleTTS {
		t.Fatalf("hello role = %q, want %q", hello.Role, RoleTTS)
	}
}

func TestHelloRejectsProtocolMismatch(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleTTS})

	response := terminalFrame(t, exchange(t, c, Request{
		Type: RequestHello, ID: "h1", ProtocolVersion: ProtocolVersion + 99,
	}))
	if response.Type != ResponseError || response.Error == nil {
		t.Fatalf("mismatched hello must produce a structured error, got %+v", response)
	}
	if response.Error.Code != ErrorCodeProtocolMismatch {
		t.Fatalf("error code = %q, want %q", response.Error.Code, ErrorCodeProtocolMismatch)
	}
}

func TestHealthAndModelLoadCycle(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleTTS})

	health := terminalFrame(t, exchange(t, c, Request{Type: RequestHealth, ID: "q1"}))
	if health.Type != ResponseHealth || health.ModelState != ModelStateUnloaded {
		t.Fatalf("initial health = %+v, want unloaded", health)
	}
	if health.QueueDepth != 0 {
		t.Fatalf("initial queue depth = %d, want 0", health.QueueDepth)
	}

	loaded := terminalFrame(t, exchange(t, c, Request{Type: RequestLoad, ID: "q2"}))
	if loaded.ModelState != ModelStateReady {
		t.Fatalf("post-load model state = %q, want %q", loaded.ModelState, ModelStateReady)
	}

	unloaded := terminalFrame(t, exchange(t, c, Request{Type: RequestUnload, ID: "q3"}))
	if unloaded.ModelState != ModelStateUnloaded {
		t.Fatalf("post-unload model state = %q, want %q", unloaded.ModelState, ModelStateUnloaded)
	}
}

func TestSpeakRequiresLoadedModel(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleTTS})

	response := terminalFrame(t, exchange(t, c, Request{
		Type: RequestSpeak, ID: "s1", Text: "hello",
	}))
	if response.Type != ResponseError || response.Error == nil {
		t.Fatalf("speak on unloaded model must error, got %+v", response)
	}
	if response.Error.Code != ErrorCodeModelNotLoaded || !response.Error.Retryable {
		t.Fatalf("error = %+v, want retryable %s", response.Error, ErrorCodeModelNotLoaded)
	}
}

func TestSpeakStreamsChunksThenDone(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleTTS, StartLoaded: true})

	frames := exchange(t, c, Request{Type: RequestSpeak, ID: "s1", Text: "hello"})
	var chunks int
	for _, frame := range frames {
		if frame.Type == ResponseAudioChunk {
			if frame.AudioB64 == "" || frame.AudioFormat == "" {
				t.Fatalf("audio chunk missing payload metadata: %+v", frame)
			}
			chunks++
		}
	}
	if chunks == 0 {
		t.Fatal("speak produced no audio chunks")
	}
	if final := terminalFrame(t, frames); final.Type != ResponseDone {
		t.Fatalf("speak terminal frame = %q, want %q", final.Type, ResponseDone)
	}
}

func TestCancelInterruptsSpeak(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleTTS, StartLoaded: true})

	responses, release, err := c.register("s1")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer release()
	if err := c.send(Request{Type: RequestSpeak, ID: "s1", Text: "hello", DelayMillis: 5000}); err != nil {
		t.Fatalf("send speak: %v", err)
	}
	if err := c.send(Request{Type: RequestCancel, ID: "c1", TargetID: "s1"}); err != nil {
		t.Fatalf("send cancel: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case response := <-responses:
			if !response.Terminal() {
				continue
			}
			if response.Type != ResponseCanceled {
				t.Fatalf("terminal frame = %q, want %q", response.Type, ResponseCanceled)
			}
			return
		case <-deadline:
			t.Fatal("cancellation did not interrupt the speak request quickly")
		}
	}
}

func TestTranscribeReturnsCandidates(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleASR, StartLoaded: true})

	transcript := terminalFrame(t, exchange(t, c, Request{
		Type: RequestTranscribe, ID: "t1", AudioB64: "AAAA", AudioFormat: "wav",
	}))
	if transcript.Type != ResponseTranscript {
		t.Fatalf("terminal frame = %q, want %q", transcript.Type, ResponseTranscript)
	}
	if len(transcript.Candidates) == 0 || transcript.Candidates[0].Text == "" {
		t.Fatalf("transcript has no candidates: %+v", transcript)
	}
	if transcript.Candidates[0].Confidence <= 0 {
		t.Fatalf("candidate confidence must be reported, got %+v", transcript.Candidates[0])
	}
}

func TestTranscribeRejectsSilenceWithoutEmptyTranscript(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleASR, StartLoaded: true})

	transcript := terminalFrame(t, exchange(t, c, Request{Type: RequestTranscribe, ID: "t1"}))
	if transcript.Type != ResponseTranscript {
		t.Fatalf("terminal frame = %q, want %q", transcript.Type, ResponseTranscript)
	}
	if transcript.Rejected != RejectedNoSpeech {
		t.Fatalf("rejected = %q, want %q", transcript.Rejected, RejectedNoSpeech)
	}
	if len(transcript.Candidates) != 0 {
		t.Fatalf("rejected audio must not carry transcript candidates, got %+v", transcript.Candidates)
	}
}

func TestWrongRoleRequestIsRejected(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleASR, StartLoaded: true})

	response := terminalFrame(t, exchange(t, c, Request{Type: RequestSpeak, ID: "x1", Text: "hi"}))
	if response.Type != ResponseError || response.Error == nil || response.Error.Code != ErrorCodeInvalidRequest {
		t.Fatalf("speak sent to an ASR worker must be invalid_request, got %+v", response)
	}
}

func TestUnknownRequestTypeIsRejected(t *testing.T) {
	c := startStubConn(t, stubworker.Options{Role: RoleTTS})

	response := terminalFrame(t, exchange(t, c, Request{Type: "bogus", ID: "b1"}))
	if response.Type != ResponseError || response.Error == nil || response.Error.Code != ErrorCodeInvalidRequest {
		t.Fatalf("unknown request type must be invalid_request, got %+v", response)
	}
}
