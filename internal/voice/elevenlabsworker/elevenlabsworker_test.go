package elevenlabsworker

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/elevenlabs"
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
	if options.RetrySleep == nil {
		options.RetrySleep = func(context.Context, time.Duration) error { return nil }
	}

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

// nextTerminal returns the terminal frame and the audio chunks before it.
func (d *driver) nextTerminal(t *testing.T) (protocol.Response, []protocol.Response) {
	t.Helper()
	var chunks []protocol.Response
	for {
		response := d.next(t, 5*time.Second)
		if response.Type == protocol.ResponseAudioChunk {
			chunks = append(chunks, response)
			continue
		}
		if response.Terminal() {
			return response, chunks
		}
	}
}

type ttsCall struct {
	voice  string
	format string
	body   map[string]any
}

// mockAPI is a minimal ElevenLabs stand-in with the documented error shapes.
type mockAPI struct {
	mu sync.Mutex
	// models is the /v1/models answer; nil serves the default speech models.
	models []elevenlabs.Model
	// modelsStatus/modelsBody override /v1/models (scoped keys, outages).
	modelsStatus int
	modelsBody   string
	// ttsFailures are served, in order, before TTS succeeds.
	ttsFailures []mockFailure
	calls       []ttsCall
	// block holds a TTS stream open after its first bytes until closed or
	// canceled; stall holds it without ever finishing.
	block chan struct{}
	stall bool
	audio []byte
}

type mockFailure struct {
	status int
	body   string
	header map[string]string
}

func (m *mockAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != testKey {
			writeDetail(w, http.StatusUnauthorized, `{"detail":{"type":"authentication_error","code":"invalid_api_key","message":"Invalid API key","status":"invalid_api_key"}}`)
			return
		}
		m.mu.Lock()
		status, body, models := m.modelsStatus, m.modelsBody, m.models
		m.mu.Unlock()
		if status != 0 {
			writeDetail(w, status, body)
			return
		}
		if models == nil {
			models = []elevenlabs.Model{
				{ID: "eleven_multilingual_v2", CanDoTextToSpeech: true, CanUseStyle: true, CanUseSpeakerBoost: true, MaxTextLength: 10000},
				{ID: "eleven_v4", CanDoTextToSpeech: true, MaxTextLength: 100000},
				{ID: "eleven_v4_turbo", CanDoTextToSpeech: true, MaxTextLength: 100000},
				{ID: "eleven_multilingual_sts_v2", CanDoTextToSpeech: false},
			}
		}
		_ = json.NewEncoder(w).Encode(models)
	})
	mux.HandleFunc("POST /v1/text-to-speech/{voice}/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != testKey {
			writeDetail(w, http.StatusUnauthorized, `{"detail":{"status":"invalid_api_key","message":"Invalid API key"}}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.calls = append(m.calls, ttsCall{voice: r.PathValue("voice"), format: r.URL.Query().Get("output_format"), body: body})
		var failure *mockFailure
		if len(m.ttsFailures) > 0 {
			failure = &m.ttsFailures[0]
			m.ttsFailures = m.ttsFailures[1:]
		}
		block, stall, audio := m.block, m.stall, m.audio
		requestNumber := len(m.calls)
		m.mu.Unlock()
		if failure != nil {
			for key, value := range failure.header {
				w.Header().Set(key, value)
			}
			writeDetail(w, failure.status, failure.body)
			return
		}
		if audio == nil {
			audio = []byte("AUDIO-BYTES-")
		}
		w.Header().Set("request-id", "req-"+strconv.Itoa(requestNumber))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(audio)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if stall {
			<-r.Context().Done()
			return
		}
		if block != nil {
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

func writeDetail(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (m *mockAPI) ttsCalls() []ttsCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ttsCall(nil), m.calls...)
}

func newServer(t *testing.T, mock *mockAPI) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(mock.handler())
	t.Cleanup(server.Close)
	return server
}

func newLoadedWorker(t *testing.T, mock *mockAPI, options Options) *driver {
	t.Helper()
	server := newServer(t, mock)
	options.APIKey = testKey
	options.BaseURL = server.URL
	d := startWorker(t, options)
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

func loadResponse(t *testing.T, mock *mockAPI, options Options) protocol.Response {
	t.Helper()
	server := newServer(t, mock)
	if options.APIKey == "" {
		options.APIKey = testKey
	}
	options.BaseURL = server.URL
	d := startWorker(t, options)
	d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
	return d.next(t, 5*time.Second)
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
	response := loadResponse(t, &mockAPI{}, Options{APIKey: "wrong-key"})
	if response.Type != protocol.ResponseError || response.Error == nil ||
		response.Error.Code != protocol.ErrorCodeMissingDependency || response.Error.Retryable {
		t.Fatalf("401 must map to a clear, non-retryable key error, got %+v", response)
	}
	if !strings.Contains(response.Error.Message, "didn't accept the API key") {
		t.Fatalf("message should explain the key problem: %q", response.Error.Message)
	}
}

// A key restricted to speech cannot list models. ElevenLabs answers 403
// insufficient_permissions today and answered 401 missing_permissions
// before; neither is a bad key, so load must succeed in both cases.
func TestLoadAcceptsSpeechOnlyKeys(t *testing.T) {
	for name, failure := range map[string]mockFailure{
		"current": {status: http.StatusForbidden, body: `{"detail":{"type":"authorization_error","code":"insufficient_permissions","message":"The API key you used is missing the permission models_read to execute this operation."}}`},
		"legacy":  {status: http.StatusUnauthorized, body: `{"detail":{"status":"missing_permissions","message":"The API key you used is missing the permission models_read to execute this operation."}}`},
	} {
		t.Run(name, func(t *testing.T) {
			mock := &mockAPI{modelsStatus: failure.status, modelsBody: failure.body}
			d := newLoadedWorker(t, mock, Options{})
			d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s", Text: "Hello there."})
			if terminal, chunks := d.nextTerminal(t); terminal.Type != protocol.ResponseDone || len(chunks) == 0 {
				t.Fatalf("speech-only key should still speak, got %+v", terminal)
			}
		})
	}
}

func TestLoadNamesAnUnavailableModel(t *testing.T) {
	response := loadResponse(t, &mockAPI{}, Options{ModelID: "eleven_monolingual_v1"})
	if response.Type != protocol.ResponseError || response.Error == nil || response.Error.Code != protocol.ErrorCodeInvalidRequest {
		t.Fatalf("unavailable model must be an invalid request, got %+v", response)
	}
	if !strings.Contains(response.Error.Message, "eleven_monolingual_v1") ||
		!strings.Contains(response.Error.Message, "eleven_v4") ||
		strings.Contains(response.Error.Message, "sts") {
		t.Fatalf("message should name the model and the speech-capable alternatives: %q", response.Error.Message)
	}
}

func TestLoadReportsUnusualActivity(t *testing.T) {
	mock := &mockAPI{modelsStatus: http.StatusUnauthorized, modelsBody: `{"detail":{"status":"detected_unusual_activity","message":"Unusual activity detected. Free Tier usage disabled."}}`}
	response := loadResponse(t, mock, Options{})
	if response.Error == nil || response.Error.Code != protocol.ErrorCodeMissingDependency ||
		!strings.Contains(response.Error.Message, "VPN") {
		t.Fatalf("unusual activity should be explained, not reported as a bad key: %+v", response)
	}
}

func TestSpeakStreamsTheWholeReplyAsProgressivePCM(t *testing.T) {
	mock := &mockAPI{}
	d := newLoadedWorker(t, mock, Options{})

	d.send(t, protocol.Request{
		Type: protocol.RequestSpeak, ID: "s1",
		Text: "First sentence is long enough to stand alone. Second sentence also carries plenty of words.",
	})
	terminal, chunks := d.nextTerminal(t)
	if terminal.Type != protocol.ResponseDone {
		t.Fatalf("terminal frame = %+v, want done", terminal)
	}
	if len(chunks) == 0 || chunks[0].AudioFormat != pcmFormatLabel {
		t.Fatalf("chunks = %+v, want %s audio", chunks, pcmFormatLabel)
	}
	calls := mock.ttsCalls()
	if len(calls) != 1 {
		t.Fatalf("TTS API calls = %d, want one for the whole reply", len(calls))
	}
	if calls[0].format != pcmOutputFormat || calls[0].voice != DefaultVoiceID || calls[0].body["model_id"] != DefaultModelID {
		t.Fatalf("unexpected request: %+v", calls[0])
	}
	if !strings.Contains(calls[0].body["text"].(string), "Second sentence") {
		t.Fatalf("reply text was not sent whole: %v", calls[0].body)
	}
}

func TestLongRepliesUseMP3ToStayInsideTheAudioBound(t *testing.T) {
	mock := &mockAPI{}
	d := newLoadedWorker(t, mock, Options{})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: strings.Repeat("A sentence of filler words. ", 90)})
	if terminal, chunks := d.nextTerminal(t); terminal.Type != protocol.ResponseDone || chunks[0].AudioFormat != "mp3" {
		t.Fatalf("long reply = %+v, want mp3 chunks", terminal)
	}
	if calls := mock.ttsCalls(); calls[0].format != "mp3_44100_128" {
		t.Fatalf("format = %q", calls[0].format)
	}
}

func TestSpeakSplitsOnlyPastTheModelLimitAndStitches(t *testing.T) {
	// Multilingual v2 documents request-ID stitching across split requests.
	mock := &mockAPI{models: []elevenlabs.Model{{ID: "eleven_multilingual_v2", CanDoTextToSpeech: true, MaxTextLength: 60}}}
	d := newLoadedWorker(t, mock, Options{ModelID: "eleven_multilingual_v2"})
	text := "The first paragraph is a single sentence of words. The second one keeps going for a while longer here."
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: text})
	if terminal, _ := d.nextTerminal(t); terminal.Type != protocol.ResponseDone {
		t.Fatalf("terminal = %+v", terminal)
	}
	calls := mock.ttsCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	if _, ok := calls[0].body["next_text"]; !ok {
		t.Fatalf("first piece should carry next_text: %v", calls[0].body)
	}
	ids, _ := calls[1].body["previous_request_ids"].([]any)
	if len(ids) != 1 || ids[0] != "req-1" {
		t.Fatalf("second piece should stitch to the first request: %v", calls[1].body)
	}
}

func TestExpressiveModelsStitchWithTextNotRequestIDs(t *testing.T) {
	mock := &mockAPI{models: []elevenlabs.Model{{ID: "eleven_v4", CanDoTextToSpeech: true, MaxTextLength: 60}}}
	d := newLoadedWorker(t, mock, Options{ModelID: "eleven_v4"})
	text := "The first paragraph is a single sentence of words. The second one keeps going for a while longer here."
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: text})
	if terminal, _ := d.nextTerminal(t); terminal.Type != protocol.ResponseDone {
		t.Fatalf("terminal = %+v", terminal)
	}
	calls := mock.ttsCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	if _, ok := calls[1].body["previous_request_ids"]; ok {
		t.Fatalf("request-ID stitching is not documented for this model: %v", calls[1].body)
	}
	if previous, _ := calls[1].body["previous_text"].(string); !strings.Contains(previous, "first paragraph") {
		t.Fatalf("second piece should carry the previous text: %v", calls[1].body)
	}
}

func TestRejectedPCMFallsBackToMP3Once(t *testing.T) {
	mock := &mockAPI{ttsFailures: []mockFailure{{status: http.StatusBadRequest,
		body: `{"detail":{"type":"validation_error","code":"invalid_output_format","param":"output_format","message":"Output format pcm_24000 is not available"}}`}}}
	d := newLoadedWorker(t, mock, Options{})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "Hello there."})
	terminal, chunks := d.nextTerminal(t)
	if terminal.Type != protocol.ResponseDone || len(chunks) == 0 || chunks[0].AudioFormat != "mp3" {
		t.Fatalf("terminal = %+v, chunks = %+v", terminal, chunks)
	}
	if calls := mock.ttsCalls(); len(calls) != 2 || calls[0].format != pcmOutputFormat || calls[1].format != "mp3_44100_128" {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestSpeakRetriesTransientRefusalsBeforeAudio(t *testing.T) {
	mock := &mockAPI{ttsFailures: []mockFailure{
		{status: http.StatusTooManyRequests, body: `{"detail":{"type":"rate_limit_error","code":"system_busy","message":"The system is busy"}}`},
		{status: http.StatusServiceUnavailable, body: `{"detail":{"type":"service_unavailable","code":"service_unavailable"}}`},
	}}
	d := newLoadedWorker(t, mock, Options{})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "Hello there."})
	if terminal, _ := d.nextTerminal(t); terminal.Type != protocol.ResponseDone {
		t.Fatalf("transient refusals should be retried, got %+v", terminal)
	}
	if calls := mock.ttsCalls(); len(calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(calls))
	}
}

func TestSpeakExplainsPermanentFailures(t *testing.T) {
	cases := map[string]struct {
		failure mockFailure
		code    string
		message string
	}{
		"quota": {mockFailure{status: http.StatusUnauthorized, body: `{"detail":{"status":"quota_exceeded","message":"This request exceeds your quota of 10000."}}`},
			protocol.ErrorCodeMissingDependency, "credits are used up"},
		"payment": {mockFailure{status: http.StatusPaymentRequired, body: `{"detail":{"type":"payment_required","code":"insufficient_credits"}}`},
			protocol.ErrorCodeMissingDependency, "credits are used up"},
		"voice": {mockFailure{status: http.StatusNotFound, body: `{"detail":{"type":"not_found","code":"voice_not_found","message":"A voice with the voice_id was not found."}}`},
			protocol.ErrorCodeInvalidRequest, "couldn't find the selected voice"},
		"free voice": {mockFailure{status: http.StatusForbidden, body: `{"detail":{"type":"authorization_error","code":"voice_access_denied","message":"This voice is not available for free users."}}`},
			protocol.ErrorCodeMissingDependency, "free ElevenLabs accounts"},
		"leaked key": {mockFailure{status: http.StatusBadRequest, body: `{"detail":{"type":"validation_error","code":"invalid_parameters","message":"bad request for key ` + testKey + `"}}`},
			protocol.ErrorCodeInvalidRequest, "[redacted]"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mock := &mockAPI{ttsFailures: []mockFailure{tc.failure, tc.failure}}
			d := newLoadedWorker(t, mock, Options{})
			d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "Hello there."})
			terminal, _ := d.nextTerminal(t)
			if terminal.Type != protocol.ResponseError || terminal.Error == nil || terminal.Error.Code != tc.code || terminal.Error.Retryable {
				t.Fatalf("terminal = %+v", terminal)
			}
			if !strings.Contains(terminal.Error.Message, tc.message) {
				t.Fatalf("message %q should contain %q", terminal.Error.Message, tc.message)
			}
			if calls := mock.ttsCalls(); len(calls) != 1 {
				t.Fatalf("a permanent failure must not be retried; calls = %d", len(calls))
			}
		})
	}
}

func TestSpeakShapesVoiceSettingsForTheModel(t *testing.T) {
	stability, similarity, style, speed := 0.3, 0.8, 0.6, 0.9
	boost := true
	mock := &mockAPI{}
	d := newLoadedWorker(t, mock, Options{ModelID: "eleven_v4", LanguageCode: "en", VoiceSettings: &elevenlabs.VoiceSettings{
		Stability: &stability, SimilarityBoost: &similarity, Style: &style, Speed: &speed, UseSpeakerBoost: &boost,
	}})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "[whispers] Come closer."})
	if terminal, _ := d.nextTerminal(t); terminal.Type != protocol.ResponseDone {
		t.Fatalf("terminal = %+v", terminal)
	}
	body := mock.ttsCalls()[0].body
	settings, _ := body["voice_settings"].(map[string]any)
	if len(settings) != 2 || settings["stability"] != 0.3 || settings["similarity_boost"] != 0.8 {
		t.Fatalf("Eleven v4 takes stability and similarity only: %v", settings)
	}
	if body["language_code"] != "en" || body["text"] != "[whispers] Come closer." {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestCancelInterruptsAnActiveStream(t *testing.T) {
	mock := &mockAPI{block: make(chan struct{})}
	d := newLoadedWorker(t, mock, Options{})

	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "One very long sentence that streams forever."})
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

func TestStalledStreamFailsInsteadOfHanging(t *testing.T) {
	mock := &mockAPI{stall: true}
	d := newLoadedWorker(t, mock, Options{StreamIdleTimeout: 150 * time.Millisecond})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "Hello there."})
	terminal, chunks := d.nextTerminal(t)
	if len(chunks) == 0 || terminal.Type != protocol.ResponseError || terminal.Error == nil ||
		!terminal.Error.Retryable || !strings.Contains(terminal.Error.Message, "stopped sending audio") {
		t.Fatalf("stalled stream = %+v after %d chunks", terminal, len(chunks))
	}
}

func TestSpeakBeforeLoadFails(t *testing.T) {
	server := newServer(t, &mockAPI{})
	d := startWorker(t, Options{APIKey: testKey, BaseURL: server.URL})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "hello"})
	response, _ := d.nextTerminal(t)
	if response.Type != protocol.ResponseError || response.Error == nil ||
		response.Error.Code != protocol.ErrorCodeModelNotLoaded {
		t.Fatalf("speak before load must be model_not_loaded, got %+v", response)
	}
}

func TestRequestVoiceOverridesTheConfiguredVoice(t *testing.T) {
	mock := &mockAPI{}
	d := newLoadedWorker(t, mock, Options{})
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s1", Text: "Hi.", Voice: "PersonaVoice123"})
	if terminal, _ := d.nextTerminal(t); terminal.Type != protocol.ResponseDone {
		t.Fatalf("terminal = %+v", terminal)
	}
	if voice := mock.ttsCalls()[0].voice; voice != "PersonaVoice123" {
		t.Fatalf("voice = %q", voice)
	}
	d.send(t, protocol.Request{Type: protocol.RequestSpeak, ID: "s2", Text: "Hi.", Voice: "../escape"})
	if terminal, _ := d.nextTerminal(t); terminal.Error == nil || terminal.Error.Code != protocol.ErrorCodeInvalidRequest {
		t.Fatalf("unsafe voice override must be rejected: %+v", terminal)
	}
}

func TestOptionsRejectUnplayableFormats(t *testing.T) {
	for _, format := range []string{"pcm_44100", "ulaw_8000", "wav"} {
		d := startWorker(t, Options{APIKey: testKey, OutputFormat: format})
		d.send(t, protocol.Request{Type: protocol.RequestLoad, ID: "l"})
		if response := d.next(t, 5*time.Second); response.Error == nil || response.Error.Code != protocol.ErrorCodeInvalidRequest {
			t.Fatalf("format %q should be rejected: %+v", format, response)
		}
	}
}

func TestHealthReportsQueueDepth(t *testing.T) {
	d := newLoadedWorker(t, &mockAPI{}, Options{})

	d.send(t, protocol.Request{Type: protocol.RequestHealth, ID: "q"})
	health := d.next(t, 5*time.Second)
	if health.Type != protocol.ResponseHealth || health.ModelState != protocol.ModelStateReady {
		t.Fatalf("health = %+v, want ready", health)
	}
	if health.QueueDepth != 0 {
		t.Fatalf("queue depth = %d, want 0", health.QueueDepth)
	}
}
