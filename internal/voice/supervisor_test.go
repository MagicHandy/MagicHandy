package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/voice/stubworker"
)

// stubBinary builds cmd/voice-stub-worker once per test run so lifecycle
// tests exercise a real child process (crash, kill, stderr capture).
var (
	stubBinaryOnce sync.Once
	stubBinaryPath string
	stubBinaryErr  error
)

func stubBinary(t *testing.T) string {
	t.Helper()
	stubBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "voice-stub-worker")
		if err != nil {
			stubBinaryErr = err
			return
		}
		name := "voice-stub-worker"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		stubBinaryPath = filepath.Join(dir, name)

		// #nosec G204 -- test-only: builds the in-repo stub into a temp dir.
		build := exec.Command("go", "build", "-o", stubBinaryPath, "./cmd/voice-stub-worker")
		build.Dir = repoRoot()
		if output, err := build.CombinedOutput(); err != nil {
			stubBinaryErr = err
			t.Logf("stub build output: %s", output)
		}
	})
	if stubBinaryErr != nil {
		t.Fatalf("build stub worker: %v", stubBinaryErr)
	}
	return stubBinaryPath
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func newTestSupervisor(t *testing.T, role Role, args ...string) *Supervisor {
	t.Helper()
	supervisor := NewSupervisor(role)
	supervisor.SetConfig(WorkerConfig{
		Enabled: true,
		Command: stubBinary(t),
		Args:    append([]string{"-role", string(role)}, args...),
	})
	t.Cleanup(supervisor.Shutdown)
	return supervisor
}

func waitForState(t *testing.T, supervisor *Supervisor, want WorkerState) WorkerStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := supervisor.Status()
		if status.State == want {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker state = %q, want %q (last error: %s)", status.State, want, status.LastError)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForRequestState(t *testing.T, pending *PendingRequest, want string) RequestSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot := pending.Snapshot()
		if snapshot.State == want {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("request state = %q, want %q (error: %+v)", snapshot.State, want, snapshot.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartHandshakeAndStop(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS)

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	status := waitForState(t, supervisor, StateRunning)
	if status.Provider == "" || status.ProtocolVersion != ProtocolVersion {
		t.Fatalf("running status must carry provider identity, got %+v", status)
	}
	if status.StartedAt == "" {
		t.Fatal("running status must carry started_at")
	}

	health, err := supervisor.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if health.ModelState != ModelStateUnloaded {
		t.Fatalf("fresh stub model state = %q, want unloaded", health.ModelState)
	}

	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitForState(t, supervisor, StateStopped)
}

func TestStartWithoutConfigurationFails(t *testing.T) {
	supervisor := NewSupervisor(RoleTTS)

	if err := supervisor.Start(context.Background()); err == nil {
		t.Fatal("start must fail while voice is disabled")
	}
	if supervisor.Status().State != StateDisabled {
		t.Fatalf("state = %q, want disabled", supervisor.Status().State)
	}

	supervisor.SetConfig(WorkerConfig{Enabled: true})
	if err := supervisor.Start(context.Background()); err == nil {
		t.Fatal("start must fail without a worker command")
	}
	if supervisor.Status().State != StateNotConfigured {
		t.Fatalf("state = %q, want not_configured", supervisor.Status().State)
	}
}

func TestStartWithMissingBinaryFails(t *testing.T) {
	supervisor := NewSupervisor(RoleASR)
	supervisor.SetConfig(WorkerConfig{
		Enabled: true,
		Command: filepath.Join(t.TempDir(), "does-not-exist.exe"),
	})

	err := supervisor.Start(context.Background())
	if err == nil {
		t.Fatal("start must fail for a missing worker binary")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("error should name the missing command, got: %v", err)
	}
	if state := supervisor.Status().State; state != StateStopped {
		t.Fatalf("state = %q, want stopped (missing binary is not a crash)", state)
	}
}

func TestProtocolMismatchIsRejectedAtStart(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS, "-advertise-protocol", "99")

	err := supervisor.Start(context.Background())
	if err == nil {
		t.Fatal("start must fail when the worker speaks a different protocol version")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("error should explain the protocol mismatch, got: %v", err)
	}
	if status := supervisor.Status(); status.State == StateRunning {
		t.Fatalf("worker must not stay running after a mismatch, got %+v", status)
	}
}

func TestStartupCrashIsVisibleWithStderr(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS, "-fail-start")

	if err := supervisor.Start(context.Background()); err == nil {
		t.Fatal("start must fail when the worker exits immediately")
	}
	status := waitForState(t, supervisor, StateCrashed)
	streamEnded := strings.Contains(status.LastError, "stream ended")
	processExited := strings.Contains(status.LastError, "process exited")
	if !streamEnded && !processExited {
		t.Fatalf("crash reason missing from status: %+v", status)
	}
	if !strings.Contains(status.StderrTail, "missing dependency") {
		t.Fatalf("stderr tail must surface the worker's own message, got %q", status.StderrTail)
	}
}

func TestMidRequestCrashIsVisibleAndRestartRecovers(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS, "-start-loaded")

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)
	requireReadyModel(t, supervisor)

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: stubworker.CrashText})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	status := waitForState(t, supervisor, StateCrashed)
	if !strings.Contains(status.StderrTail, "crashing on request") {
		t.Fatalf("stderr tail should capture the crash banner, got %q", status.StderrTail)
	}
	if status.ModelState != "" || status.WorkerQueue != 0 {
		t.Fatalf("crashed worker retained stale readiness: %+v", status)
	}
	snapshot := waitForRequestState(t, pending, RequestStateFailed)
	if snapshot.Error == nil {
		t.Fatal("the in-flight request must fail when the worker dies")
	}

	if err := supervisor.Restart(context.Background()); err != nil {
		t.Fatalf("restart after crash: %v", err)
	}
	waitForState(t, supervisor, StateRunning)
}

func TestSubmitSpeakCompletesWithChunks(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS, "-start-loaded")

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)
	requireReadyModel(t, supervisor)

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "hello"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	snapshot := waitForRequestState(t, pending, RequestStateDone)
	if snapshot.AudioChunks == 0 {
		t.Fatalf("completed speak recorded no audio chunks: %+v", snapshot)
	}
}

func TestRequestIDsAreUniqueAcrossWorkerRoles(t *testing.T) {
	tts := NewSupervisor(RoleTTS)
	asr := NewSupervisor(RoleASR)
	if ttsID, asrID := tts.newRequestID(), asr.newRequestID(); ttsID == asrID || !strings.HasPrefix(ttsID, "tts-") || !strings.HasPrefix(asrID, "asr-") {
		t.Fatalf("request IDs = %q and %q, want distinct role-prefixed IDs", ttsID, asrID)
	}
}

func TestWorkerConfigAndStatusSnapshotsOwnMutableData(t *testing.T) {
	supervisor := NewSupervisor(RoleTTS)
	args := []string{"-role", "tts"}
	environment := map[string]string{"TOKEN": "original"}
	supervisor.SetConfig(WorkerConfig{Enabled: true, Command: "worker", Args: args, Env: environment})
	args[0] = "mutated"
	environment["TOKEN"] = "mutated"

	supervisor.mu.Lock()
	if supervisor.config.Args[0] != "-role" || supervisor.config.Env["TOKEN"] != "original" {
		t.Fatalf("stored config aliases caller data: %+v", supervisor.config)
	}
	supervisor.hello = Response{Provider: "test", Capabilities: []string{"cancel"}}
	supervisor.mu.Unlock()

	status := supervisor.Status()
	status.Capabilities[0] = "mutated"
	if next := supervisor.Status(); next.Capabilities[0] != "cancel" {
		t.Fatalf("status capabilities alias supervisor state: %+v", next.Capabilities)
	}
}

func TestSubmittedInputAudioIsReleasedAfterDispatch(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleASR, "-start-loaded")
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)
	requireReadyModel(t, supervisor)

	pending, err := supervisor.Submit(Request{Type: RequestTranscribe, AudioB64: "c3BlZWNo", AudioFormat: "wav"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForRequestState(t, pending, RequestStateDone)
	pending.mu.Lock()
	retained := pending.request.AudioB64
	pending.mu.Unlock()
	if retained != "" {
		t.Fatal("completed transcription retained its input audio")
	}
}

func TestWorkerResponseValidationRejectsCorruptAudio(t *testing.T) {
	supervisor := NewSupervisor(RoleTTS)
	pending := &PendingRequest{ID: "tts-1", Role: RoleTTS, Type: RequestSpeak, state: RequestStateActive}

	terminal, cancelWorker := supervisor.applyResponse(pending, Response{
		Type: ResponseAudioChunk, RequestID: pending.ID, Seq: 1,
		AudioB64: base64.StdEncoding.EncodeToString([]byte("audio")), AudioFormat: "mp3",
	})
	if !terminal || cancelWorker {
		t.Fatalf("invalid sequence result = terminal %t cancel %t", terminal, cancelWorker)
	}
	snapshot := pending.Snapshot()
	if snapshot.State != RequestStateFailed || snapshot.Error == nil || !strings.Contains(snapshot.Error.Message, "sequence") {
		t.Fatalf("invalid sequence snapshot = %+v", snapshot)
	}
}

func TestWorkerResponseValidationRejectsEmptyCompletion(t *testing.T) {
	supervisor := NewSupervisor(RoleTTS)
	pending := &PendingRequest{ID: "tts-1", Role: RoleTTS, Type: RequestSpeak, state: RequestStateActive}
	terminal, _ := supervisor.applyResponse(pending, Response{Type: ResponseDone, RequestID: pending.ID})
	if !terminal {
		t.Fatal("empty completion was not terminal")
	}
	if snapshot := pending.Snapshot(); snapshot.State != RequestStateFailed || snapshot.Error == nil {
		t.Fatalf("empty completion snapshot = %+v", snapshot)
	}
}

func TestRequestSnapshotOwnsTranscriptAndError(t *testing.T) {
	pending := &PendingRequest{
		ID: "asr-1", Role: RoleASR, Type: RequestTranscribe, state: RequestStateFailed,
		transcript: []TranscriptCandidate{{Text: "original", Confidence: 1}},
		failure:    &WorkerError{Code: ErrorCodeInternal, Message: "original"},
	}
	snapshot := pending.Snapshot()
	snapshot.Transcript[0].Text = "mutated"
	snapshot.Error.Message = "mutated"
	next := pending.Snapshot()
	if next.Transcript[0].Text != "original" || next.Error.Message != "original" {
		t.Fatalf("request snapshot aliases internal data: %+v", next)
	}
}

func TestInvalidateAllRejectsCompletedASRResults(t *testing.T) {
	manager := NewManager()
	pending := &PendingRequest{ID: "asr-1", Role: RoleASR, Type: RequestTranscribe, state: RequestStateDone}
	manager.Track(pending)
	if active := manager.InvalidateAll(RoleASR); len(active) != 0 {
		t.Fatalf("completed request returned as active: %+v", active)
	}
	if state := pending.Snapshot().State; state != RequestStateCanceled {
		t.Fatalf("invalidated transcript state = %q, want canceled", state)
	}
}

func TestCancelInvalidatedSendsBoundWorkerCancelExactlyOnce(t *testing.T) {
	manager := NewManager()
	var workerInput bytes.Buffer
	workerConn := &conn{
		writer:  &workerInput,
		pending: make(map[string]*responseSink),
		done:    make(chan struct{}),
	}
	pending := &PendingRequest{
		ID: "tts-active", Role: RoleTTS, Type: RequestSpeak,
		state: RequestStateActive, wire: workerConn,
	}
	manager.Track(pending)

	active := manager.InvalidateAll(RoleTTS)
	if len(active) != 1 || active[0] != pending {
		t.Fatalf("invalidated active work = %+v", active)
	}
	if workerInput.Len() != 0 {
		t.Fatalf("local invalidation wrote to worker before safety teardown: %q", workerInput.String())
	}

	manager.CancelInvalidated(RoleTTS, active)
	manager.CancelInvalidated(RoleTTS, active)
	var cancel Request
	if err := json.Unmarshal(bytes.TrimSpace(workerInput.Bytes()), &cancel); err != nil {
		t.Fatalf("decode worker cancel: %v", err)
	}
	if cancel.Type != RequestCancel || cancel.TargetID != pending.ID {
		t.Fatalf("worker cancel = %+v", cancel)
	}
}

func TestCancelDoesNotRewriteTerminalRequest(t *testing.T) {
	supervisor := NewSupervisor(RoleTTS)
	pending := &PendingRequest{
		ID: "tts-done", Role: RoleTTS, Type: RequestSpeak,
		state: RequestStateDone,
	}
	supervisor.Cancel(pending)
	if state := pending.Snapshot().State; state != RequestStateDone {
		t.Fatalf("terminal request state = %q after cancel, want done", state)
	}
}

func TestTranscriptionStagingIsSessionScopedAndRemovedOnShutdown(t *testing.T) {
	manager := NewManager()
	dataDir := t.TempDir()
	if err := manager.PrepareTranscriptionStaging(dataDir); err != nil {
		t.Fatalf("prepare staging: %v", err)
	}
	dir := manager.stagingDir
	if filepath.Base(filepath.Dir(dir)) != "inputs" || !strings.HasPrefix(filepath.Base(dir), "session-") {
		t.Fatalf("staging directory = %q, want voice/inputs/session-*", dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.wav"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.Shutdown()
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging session survives shutdown: %v", err)
	}
	if err := manager.PrepareTranscriptionStaging(dataDir); err == nil {
		t.Fatal("shut-down manager recreated transcription staging")
	}
}

func TestManagerRejectsInvalidRoleAndTranscriptionFormat(t *testing.T) {
	manager := NewManager()
	t.Cleanup(manager.Shutdown)
	if _, err := manager.Submit(Role("invalid"), Request{Type: RequestSpeak, Text: "hello"}); err == nil {
		t.Fatal("unknown role was accepted")
	}
	if _, err := manager.SubmitTranscription([]byte("audio"), `wav/../../escape`, t.TempDir()); err == nil {
		t.Fatal("unsafe transcription format was accepted")
	}
	if _, err := manager.SubmitTranscription(nil, "wav", t.TempDir()); err == nil {
		t.Fatal("empty transcription was accepted")
	}
}

func TestCancelStopsActiveRequest(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS, "-start-loaded")

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)
	requireReadyModel(t, supervisor)

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "hello", DelayMillis: 10000})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForRequestState(t, pending, RequestStateActive)

	start := time.Now()
	supervisor.Cancel(pending)
	waitForRequestState(t, pending, RequestStateCanceled)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %s; must not wait out the full request delay", elapsed)
	}
}

func TestCancelQueuedRequestDoesNotSendUnknownWorkerCancel(t *testing.T) {
	var workerInput bytes.Buffer
	workerConn := &conn{
		writer:  &workerInput,
		pending: make(map[string]*responseSink),
		done:    make(chan struct{}),
	}
	supervisor := NewSupervisor(RoleTTS)
	supervisor.mu.Lock()
	supervisor.state = StateRunning
	supervisor.conn = workerConn
	supervisor.queue = make(chan *PendingRequest, queueCapacity)
	supervisor.lastHealth = Response{ModelState: ModelStateReady}
	supervisor.mu.Unlock()

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	supervisor.Cancel(pending)
	if workerInput.Len() != 0 {
		t.Fatalf("queued request emitted a cancel before its work frame: %q", workerInput.String())
	}
	if state := pending.Snapshot().State; state != RequestStateCanceled {
		t.Fatalf("queued cancellation state = %q, want canceled", state)
	}
}

func TestConfiguredJobTimeoutOverridesDefault(t *testing.T) {
	supervisor := NewSupervisor(RoleTTS)
	supervisor.SetConfig(WorkerConfig{
		Enabled:    true,
		Command:    stubBinary(t),
		Args:       []string{"-role", string(RoleTTS), "-start-loaded"},
		JobTimeout: 25 * time.Millisecond,
	})
	t.Cleanup(supervisor.Shutdown)
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	requireReadyModel(t, supervisor)
	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "slow", DelayMillis: 1000})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	snapshot := waitForRequestState(t, pending, RequestStateFailed)
	if snapshot.Error == nil || snapshot.Error.Code != ErrorCodeTimeout || !strings.Contains(snapshot.Error.Message, "25ms") {
		t.Fatalf("timeout error = %+v", snapshot.Error)
	}
}

func TestSubmitWhileStoppedFails(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS)

	if _, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "hello"}); err == nil {
		t.Fatal("submit must fail while the worker is stopped")
	}
}

func TestSubmitWhileModelIsNotReadyFails(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS)
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)

	if _, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "hello"}); err == nil || !strings.Contains(err.Error(), "model is not ready") {
		t.Fatalf("submit error = %v, want model-not-ready rejection", err)
	}
}

func TestModelLoadUnloadThroughSupervisor(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleASR)

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)

	loaded, err := supervisor.SetModelLoaded(context.Background(), true)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ModelState != ModelStateReady {
		t.Fatalf("model state = %q, want ready", loaded.ModelState)
	}
	if status := supervisor.Status(); status.ModelState != ModelStateReady {
		t.Fatalf("status must cache the model state, got %+v", status)
	}

	unloaded, err := supervisor.SetModelLoaded(context.Background(), false)
	if err != nil {
		t.Fatalf("unload: %v", err)
	}
	if unloaded.ModelState != ModelStateUnloaded {
		t.Fatalf("model state = %q, want unloaded", unloaded.ModelState)
	}
}

func TestConfigChangeStopsRunningWorker(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS)

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)

	supervisor.SetConfig(WorkerConfig{Enabled: false})
	waitForState(t, supervisor, StateDisabled)
}

func TestCompletedSpeakRetainsBoundedAudio(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS, "-start-loaded")

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)
	requireReadyModel(t, supervisor)

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "retain me"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	snapshot := waitForRequestState(t, pending, RequestStateDone)
	if snapshot.AudioBytes == 0 {
		t.Fatalf("completed speak retained no audio: %+v", snapshot)
	}
	audio, format := pending.Audio()
	if len(audio) != snapshot.AudioBytes+44 || format != "wav" {
		t.Fatalf("Audio() = %d bytes %q, want %d-byte PCM plus WAV header", len(audio), format, snapshot.AudioBytes)
	}

	// The manager keeps enough audio for the full accepted TTS workload.
	manager := NewManager()
	tracked := make([]*PendingRequest, 0, audioRetainCount+3)
	for i := 0; i < audioRetainCount+3; i++ {
		request := &PendingRequest{
			ID: strconv.Itoa(i), Role: RoleTTS, Type: RequestSpeak,
			audio: []byte{1, 2, 3},
		}
		manager.Track(request)
		tracked = append(tracked, request)
	}
	for i, request := range tracked {
		audio, _ := request.Audio()
		wantAudio := i >= len(tracked)-audioRetainCount
		if wantAudio && len(audio) == 0 {
			t.Fatalf("request %d should retain audio", i)
		}
		if !wantAudio && len(audio) != 0 {
			t.Fatalf("request %d should have dropped audio", i)
		}
	}

	// ASR history is metadata-only and must not consume TTS retention slots.
	newestSpeech := tracked[len(tracked)-1]
	for i := 0; i < audioRetainCount+3; i++ {
		manager.Track(&PendingRequest{
			ID: fmt.Sprintf("asr-%d", i), Role: RoleASR, Type: RequestTranscribe,
			state: RequestStateDone,
		})
	}
	if audio, _ := newestSpeech.Audio(); len(audio) == 0 {
		t.Fatal("ASR history evicted retained TTS audio")
	}
}

func TestRequestLogNeverEvictsActiveWork(t *testing.T) {
	manager := NewManager()
	active := &PendingRequest{ID: "asr-active", Role: RoleASR, state: RequestStateActive}
	manager.Track(active)
	for i := 0; i < requestLogLimit+4; i++ {
		manager.Track(&PendingRequest{ID: fmt.Sprintf("done-%d", i), state: RequestStateDone})
	}
	if got, ok := manager.Request(active.ID); !ok || got != active {
		t.Fatal("active voice request was evicted from the request index")
	}
}

// TestWorkerEnvCarriesCredentialsPrivately proves a provider credential set
// via WorkerConfig.Env reaches the child process (the ElevenLabs worker
// validates it against a mock API) without ever appearing in the command
// line or any status snapshot.
func TestWorkerEnvCarriesCredentialsPrivately(t *testing.T) {
	const secret = "el-env-secret-42" // #nosec G101 -- synthetic test credential, not a real key
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/user" && r.Header.Get("xi-api-key") == secret {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(mock.Close)

	binary := buildWorkerBinary(t, "./cmd/voice-elevenlabs-worker", "voice-elevenlabs-worker")
	supervisor := NewSupervisor(RoleTTS)
	supervisor.SetConfig(WorkerConfig{
		Enabled: true,
		Command: binary,
		Args:    []string{"-base-url", mock.URL},
		Env:     map[string]string{"ELEVENLABS_API_KEY": secret},
	})
	t.Cleanup(supervisor.Shutdown)

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)

	health, err := supervisor.SetModelLoaded(context.Background(), true)
	if err != nil {
		t.Fatalf("load with env credential failed: %v", err)
	}
	if health.ModelState != ModelStateReady {
		t.Fatalf("model state = %q, want ready (key must reach the child via env)", health.ModelState)
	}

	status := supervisor.Status()
	if strings.Contains(status.Command, secret) || strings.Contains(strings.Join(status.Capabilities, " "), secret) ||
		strings.Contains(status.LastError, secret) || strings.Contains(status.StderrTail, secret) {
		t.Fatalf("credential leaked into worker status: %+v", status)
	}
}

func requireReadyModel(t *testing.T, supervisor *Supervisor) {
	t.Helper()
	health, err := supervisor.Health(t.Context())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if health.ModelState != ModelStateReady {
		t.Fatalf("model state = %q, want ready", health.ModelState)
	}
}

// buildWorkerBinary builds one cmd/ worker into a temp dir (cached per path).
var (
	workerBuildMu    sync.Mutex
	workerBuildCache = map[string]string{}
)

func buildWorkerBinary(t *testing.T, packagePath string, name string) string {
	t.Helper()
	workerBuildMu.Lock()
	defer workerBuildMu.Unlock()
	if cached, ok := workerBuildCache[packagePath]; ok {
		return cached
	}
	dir, err := os.MkdirTemp("", "voice-worker-build")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	output := filepath.Join(dir, name)
	// #nosec G204 -- test-only: builds an in-repo worker into a temp dir.
	build := exec.Command("go", "build", "-o", output, packagePath)
	build.Dir = repoRoot()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, out)
	}
	workerBuildCache[packagePath] = output
	return output
}
