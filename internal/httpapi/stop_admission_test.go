package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestConcurrentHTTPStopsShareDispatchAndBoundWaiters(t *testing.T) {
	device := newBlockingControllerStopTransport()
	s := newTestServerWithRuntime(t, Runtime{Transport: device, MotionTransport: device})
	engine := startStopTestEngine(t, s)
	var release sync.Once
	defer release.Do(func() { close(device.release) })
	const followers = 64
	results := make(chan *httptest.ResponseRecorder, followers+1)
	call := func() {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
		results <- w
	}
	go call()
	<-device.started
	for range followers {
		go call()
	}
	// Overflow still invalidates work, but replies honestly without joining an
	// unbounded queue behind the deliberately blocked transport.
	for range followers - stopWaiterLimit {
		select {
		case result := <-results:
			if result.Code != http.StatusServiceUnavailable || !strings.Contains(result.Body.String(), `"stop_pending":true`) || !strings.Contains(result.Body.String(), `"stopped":false`) {
				t.Fatalf("pending Stop was misreported: %d %s", result.Code, result.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("overflow Stop waited for the transport")
		}
	}
	if s.stopSequence.Load() != followers+1 {
		t.Fatal("coalescing skipped a Stop invalidation")
	}
	if waiting := s.stopAdmission.snapshot()["waiting"]; waiting != stopWaiterLimit {
		t.Fatalf("unexpected waiter count %v", waiting)
	}
	release.Do(func() { close(device.release) })
	for range stopWaiterLimit + 1 {
		select {
		case result := <-results:
			if result.Code != http.StatusOK {
				t.Fatalf("shared Stop failed: %d %s", result.Code, result.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("shared Stop did not finish")
		}
	}
	if engine.Snapshot().Running || countTransportStops(device.Commands()) != 1 {
		t.Fatal("concurrent HTTP requests flooded the transport")
	}
	if s.stopAdmission.snapshot()["waiting"] != 0 {
		t.Fatal("Stop waiter leaked")
	}
	// A later intentional retry always makes another physical Stop attempt.
	call()
	if result := <-results; result.Code != http.StatusOK || countTransportStops(device.Commands()) != 2 {
		t.Fatal("a completed Stop was incorrectly cached")
	}
}

func startStopTestEngine(t *testing.T, s *Server) *motion.Engine {
	t.Helper()
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternHardAndRegular, SpeedPercent: 20}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestStopDelayedBeforeInvalidationCannotReuseAnEarlierStop(t *testing.T) {
	device := newBlockingControllerStopTransport()
	s := newTestServerWithRuntime(t, Runtime{Transport: device, MotionTransport: device})
	startStopTestEngine(t, s)
	var releaseDevice, releaseFollower sync.Once
	followerGate := make(chan struct{})
	defer releaseDevice.Do(func() { close(device.release) })
	defer releaseFollower.Do(func() { close(followerGate) })
	leaderDone, followerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(leaderDone)
		s.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	}()
	<-device.started
	blocked := make(chan struct{})
	ctx := context.WithValue(t.Context(), controllerCancellationKey{}, func() bool {
		close(blocked)
		<-followerGate
		return true
	})
	go func() {
		defer close(followerDone)
		s.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil).WithContext(ctx))
	}()
	<-blocked
	releaseDevice.Do(func() { close(device.release) })
	<-leaderDone
	// A new run begins after the first physical Stop, while the other request
	// has not yet invalidated work. That request must make a fresh Stop attempt.
	engine := startStopTestEngine(t, s)
	releaseFollower.Do(func() { close(followerGate) })
	select {
	case <-followerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("delayed Stop did not finish")
	}
	if engine.Snapshot().Running || countTransportStops(device.Commands()) != 2 {
		t.Fatal("a Stop delayed before invalidation reused confirmation from an earlier run")
	}
}

func countTransportStops(commands []transport.Command) int {
	count := 0
	for _, command := range commands {
		if command.Kind == transport.CommandKindStop {
			count++
		}
	}
	return count
}

func TestCanceledStopWaiterCannotCancelSharedDispatch(t *testing.T) {
	var admission stopAdmissionRuntime
	leader := admission.enter()
	defer leader.release()
	follower := admission.enter()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := follower.await(ctx)
	follower.release()
	if err != errStopConfirmationPending || !result.pending {
		t.Fatal("canceled waiter claimed confirmation")
	}
	if admission.snapshot()["active"] != true || admission.snapshot()["waiting"] != 0 {
		t.Fatal("canceled waiter ended the shared operation or leaked capacity")
	}
	leader.complete(emergencyStopResult{engineAvailable: true}, nil)
	if admission.snapshot()["active"] != false {
		t.Fatal("completed Stop retained admission")
	}
}
