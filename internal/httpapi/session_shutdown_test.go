package httpapi

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type shutdownDeadlineObserver struct {
	http.ResponseWriter
	observe func() bool
	applied chan time.Time
}

func (w shutdownDeadlineObserver) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w shutdownDeadlineObserver) SetWriteDeadline(deadline time.Time) error {
	err := http.NewResponseController(w.ResponseWriter).SetWriteDeadline(deadline)
	if w.observe() && !deadline.IsZero() {
		select {
		case w.applied <- deadline:
		default:
		}
	}
	return err
}

func TestQuiesceFinishesHTTPFramingAfterCancellationInterruptRuns(t *testing.T) {
	s := newTestServer(t)
	deadlineApplied := make(chan time.Time, 1)
	resume := make(chan struct{})
	released := false
	tracked := s.trackSessionActivity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSSEHeaders(w)
		if err := writeSSE(w, "ready", map[string]bool{"ready": true}); err != nil {
			return
		}
		<-r.Context().Done()
		// Force the cancellation callback to run before the handler returns.
		// Healthy shutdown must still write the HTTP/1 chunk terminator.
		<-resume
	}))
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracked.ServeHTTP(shutdownDeadlineObserver{ResponseWriter: w, observe: s.quiescing.Load, applied: deadlineApplied}, r)
	}))
	defer func() {
		if !released {
			close(resume)
		}
		host.Close()
	}()
	response, err := http.Get(host.URL + "/api/motion/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\n" {
			break
		}
	}
	s.Quiesce()
	select {
	case <-deadlineApplied:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not interrupt the active request")
	}
	close(resume)
	released = true
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatalf("healthy quiesced stream lost its HTTP framing: %v", err)
	}
}

func TestAccessRevocationStillInterruptsWritesImmediately(t *testing.T) {
	s := newTestServer(t)
	deadlineApplied := make(chan time.Time, 1)
	ready, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	tracked := s.trackSessionActivity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSSEHeaders(w)
		_ = writeSSE(w, "ready", map[string]bool{"ready": true})
		close(ready)
		<-r.Context().Done()
		<-resume
	}))
	go func() {
		defer close(done)
		tracked.ServeHTTP(shutdownDeadlineObserver{
			ResponseWriter: httptest.NewRecorder(),
			observe:        func() bool { return s.auth.unprotectedCtx.Err() != nil }, applied: deadlineApplied,
		}, httptest.NewRequest(http.MethodGet, "/api/motion/events", nil))
	}()
	defer func() { s.auth.endUnprotected(); close(resume); <-done }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}
	// Enabling account protection revokes the previous unprotected lifetime;
	// the same interruption path handles authenticated session/owner revocation.
	s.auth.endUnprotected()
	select {
	case deadline := <-deadlineApplied:
		if deadline.After(time.Now()) {
			t.Fatal("live-server revocation incorrectly received a shutdown grace period")
		}
	case <-time.After(time.Second):
		t.Fatal("revocation did not interrupt stream writes")
	}
}
