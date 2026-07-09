package voice

import (
	"context"
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
	if !strings.Contains(status.LastError, "exited unexpectedly") {
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

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: stubworker.CrashText})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	status := waitForState(t, supervisor, StateCrashed)
	if !strings.Contains(status.StderrTail, "crashing on request") {
		t.Fatalf("stderr tail should capture the crash banner, got %q", status.StderrTail)
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

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "hello"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	snapshot := waitForRequestState(t, pending, RequestStateDone)
	if snapshot.AudioChunks == 0 {
		t.Fatalf("completed speak recorded no audio chunks: %+v", snapshot)
	}
}

func TestCancelStopsActiveRequest(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS, "-start-loaded")

	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, supervisor, StateRunning)

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

func TestSubmitWhileStoppedFails(t *testing.T) {
	supervisor := newTestSupervisor(t, RoleTTS)

	if _, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "hello"}); err == nil {
		t.Fatal("submit must fail while the worker is stopped")
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

	pending, err := supervisor.Submit(Request{Type: RequestSpeak, Text: "retain me"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	snapshot := waitForRequestState(t, pending, RequestStateDone)
	if snapshot.AudioBytes == 0 {
		t.Fatalf("completed speak retained no audio: %+v", snapshot)
	}
	audio, format := pending.Audio()
	if len(audio) != snapshot.AudioBytes || format != "wav" {
		t.Fatalf("Audio() = %d bytes %q, want %d bytes wav", len(audio), format, snapshot.AudioBytes)
	}

	// The manager keeps audio only for the newest few requests.
	manager := NewManager()
	tracked := make([]*PendingRequest, 0, audioRetainCount+3)
	for i := 0; i < audioRetainCount+3; i++ {
		request := &PendingRequest{ID: strconv.Itoa(i), audio: []byte{1, 2, 3}}
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
}
