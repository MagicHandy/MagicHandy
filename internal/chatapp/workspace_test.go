package chatapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testWorkspace(t *testing.T, autopilot func() bool) (*Workspace, *chat.MessageLog, string) {
	t.Helper()
	log, err := chat.OpenMessageLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	session, err := log.ReconcileStartup("previous", true)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorkspace(t.Context(), log, autopilot)
	t.Cleanup(w.CancelTurns)
	return w, log, session.ID
}

func TestSessionUseCasesRespectTurnAndAutopilotAdmission(t *testing.T) {
	autopilot := false
	w, _, original := testWorkspace(t, func() bool { return autopilot })
	_, finish, err := w.BeginTurn(t.Context(), original)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	assertBlockedMutations(t, w, original, ErrReplyActive)
	// Saving is allowed during a turn; switching/deleting active history is not.
	if _, err := w.Save(original); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Delete(original); !errors.Is(err, chat.ErrActiveSessionDelete) {
		t.Fatalf("delete active = %v", err)
	}
	finish()
	autopilot = true
	assertBlockedMutations(t, w, original, ErrAutopilotActive)
	autopilot = false
	sessions, err := w.Create(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions after create = %d", len(sessions))
	}
	active, err := w.ResolveActive(t.Context(), "")
	if err != nil || active == original {
		t.Fatalf("new active session = %q, %v", active, err)
	}
	if _, finish, err := w.BeginTurn(t.Context(), original); err == nil {
		finish()
		t.Fatal("stale session admitted")
	}
	if _, err := w.Activate(original, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.ResolveActive(t.Context(), ""); got != original {
		t.Fatalf("active = %q", got)
	}
}

func assertBlockedMutations(t *testing.T, w *Workspace, session string, want error) {
	t.Helper()
	if _, err := w.Create(true); !errors.Is(err, want) {
		t.Fatalf("create error = %v, want %v", err, want)
	}
	if _, err := w.Activate(session, true); !errors.Is(err, want) {
		t.Fatalf("activate error = %v, want %v", err, want)
	}
	finish, err := w.BeginPersonaChange()
	if finish != nil {
		finish()
	}
	if !errors.Is(err, want) {
		t.Fatalf("persona change error = %v, want %v", err, want)
	}
}

func TestCanceledTurnCleanupCannotReleaseItsReplacement(t *testing.T) {
	w, _, session := testWorkspace(t, nil)
	old, finishOld, err := w.BeginTurn(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	defer finishOld()
	w.CancelTurns()
	if old.Err() != context.Canceled {
		t.Fatal("Stop did not cancel the active turn")
	}
	current, finishCurrent, err := w.BeginTurn(t.Context(), session)
	if err != nil {
		t.Fatal(err)
	}
	defer finishCurrent()
	finishOld()
	finishOld()
	if !w.TurnActive() || current.Err() != nil {
		t.Fatal("late cleanup released the replacement")
	}
	assertBlockedMutations(t, w, session, ErrReplyActive)
	finishCurrent()
	if w.TurnActive() {
		t.Fatal("finished turn remains active")
	}
}

type blockedSessions struct {
	sessionPort
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

// Alias the embedded port so its Sessions method remains promoted.
type sessionPort interface{ Sessions }

func (s *blockedSessions) ActiveSessionIDContext(ctx context.Context) (string, error) {
	s.once.Do(func() { close(s.entered) })
	<-s.release
	return s.sessionPort.ActiveSessionIDContext(ctx)
}

func TestTurnAdmissionInvalidatedDuringStorageRead(t *testing.T) {
	for _, cause := range []string{"stop", "request", "shutdown"} {
		t.Run(cause, func(t *testing.T) {
			w, log, session := testWorkspace(t, nil)
			blocked := &blockedSessions{sessionPort: log, entered: make(chan struct{}), release: make(chan struct{})}
			w.sessions = blocked
			release := sync.OnceFunc(func() { close(blocked.release) })
			defer release()
			parent, cancelRequest := context.WithCancel(t.Context())
			defer cancelRequest()
			lifetime, cancelLifetime := context.WithCancel(t.Context())
			defer cancelLifetime()
			w.lifetime = lifetime
			done := make(chan error, 1)
			go func() {
				_, finish, err := w.BeginTurn(parent, session)
				if finish != nil {
					finish()
				}
				done <- err
			}()
			<-blocked.entered
			want := context.Canceled
			switch cause {
			case "stop":
				stopped := make(chan struct{})
				go func() { w.CancelTurns(); close(stopped) }()
				select {
				case <-stopped:
				case <-time.After(time.Second):
					t.Fatal("Stop waited for storage")
				}
				want = ErrTurnInvalidated
			case "request":
				cancelRequest()
			case "shutdown":
				cancelLifetime()
			}
			release()
			if err := <-done; !errors.Is(err, want) {
				t.Fatalf("admission error = %v, want %v", err, want)
			}
			if w.TurnActive() {
				t.Fatal("invalidated turn was registered")
			}
		})
	}
}

func TestStopHistoryPreservesSessionAndObservation(t *testing.T) {
	w, log, session := testWorkspace(t, nil)
	diagnostics := chat.MessageDiagnostics{Source: "deterministic_stop"}
	stale, err := w.RecordStoppedReply("stale-session", "stop", "Stopped.", "client", false, diagnostics)
	if err == nil || stale.SessionID != "" {
		t.Fatalf("stale stop history = %+v, %v", stale, err)
	}
	if seq, _ := log.LatestSeqSession(session); seq != 0 {
		t.Fatal("stale Stop wrote to the current chat")
	}
	record, err := w.RecordStoppedReply(session, "stop", "Stopped.", "client", false, diagnostics)
	if err != nil || record.UserSeq <= 0 || record.ReplySeq <= record.UserSeq {
		t.Fatalf("stop record = %+v, %v", record, err)
	}
	state, err := w.Observe(t.Context(), false)
	if err != nil || state.ActiveSessionID != session || state.LatestSeq != record.ReplySeq {
		t.Fatalf("observation = %+v, %v", state, err)
	}
}

type shutdownSessions struct {
	sessionPort
	entered chan struct{}
	release chan struct{}
}

func (s *shutdownSessions) ActiveSessionIDContext(ctx context.Context) (string, error) {
	close(s.entered)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-s.release:
		return s.sessionPort.ActiveSessionIDContext(ctx)
	}
}

func TestShutdownCancelsStorageReadBeforeTurnRegistration(t *testing.T) {
	for _, kind := range []string{"turn", "preflight", "observation"} {
		t.Run(kind, func(t *testing.T) { testShutdownRead(t, kind) })
	}
}

func testShutdownRead(t *testing.T, kind string) {
	t.Helper()
	w, log, session := testWorkspace(t, nil)
	blocked := &shutdownSessions{sessionPort: log, entered: make(chan struct{}), release: make(chan struct{})}
	w.sessions = blocked
	lifetime, stop := context.WithCancel(t.Context())
	defer stop()
	w.lifetime = lifetime
	done := make(chan error, 1)
	go func() {
		var err error
		switch kind {
		case "preflight":
			_, err = w.ResolveActive(t.Context(), session)
		case "observation":
			_, err = w.Observe(t.Context(), false)
		case "turn":
			var finish context.CancelFunc
			_, finish, err = w.BeginTurn(t.Context(), session)
			if finish != nil {
				finish()
			}
		}
		done <- err
	}()
	<-blocked.entered
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown admission = %v", err)
		}
	case <-time.After(time.Second):
		close(blocked.release)
		<-done
		t.Fatal("application shutdown did not cancel pending admission storage")
	}
}
