package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func controlledModelWorker(t *testing.T) (*Supervisor, <-chan Request, func(Response)) {
	t.Helper()
	input, writer := io.Pipe()
	reader, output := io.Pipe()
	c := newConn(writer, reader)
	s := NewSupervisor(RoleTTS)
	s.conn, s.state = c, StateRunning
	requests := make(chan Request, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		decoder := json.NewDecoder(input)
		for {
			var request Request
			if decoder.Decode(&request) != nil {
				return
			}
			requests <- request
		}
	}()
	t.Cleanup(func() { c.closeWithError(io.EOF); _ = input.Close(); _ = reader.Close(); _ = output.Close(); <-done })
	return s, requests, func(response Response) {
		t.Helper()
		if err := json.NewEncoder(output).Encode(response); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMatchingModelLoadsShareOneOperation(t *testing.T) {
	s, requests, reply := controlledModelWorker(t)
	first := make(chan error, 1)
	go func() { _, err := s.SetModelLoaded(t.Context(), true); first <- err }()
	load := <-requests
	joined := make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	go func() { _, err := s.SetModelLoaded(ctx, true); joined <- err }()
	cancel()
	if err := <-joined; err != context.Canceled {
		t.Fatalf("canceled join = %v", err)
	}
	if _, err := s.SetModelLoaded(t.Context(), false); err == nil {
		t.Fatal("opposing model action was admitted")
	}
	if got := s.Status(); got.ModelState != ModelStateLoading || got.LastError != "" {
		t.Fatalf("joining caller altered load: %+v", got)
	}
	reply(Response{Type: ResponseHealth, RequestID: load.ID, ModelState: ModelStateReady})
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	select {
	case extra := <-requests:
		t.Fatalf("duplicate protocol action: %+v", extra)
	default:
	}
	if got := s.Status(); got.ModelState != ModelStateReady {
		t.Fatalf("load did not finish: %+v", got)
	}
}

func TestModelLoadOutlivesItsInitiatingRequest(t *testing.T) {
	s, requests, reply := controlledModelWorker(t)
	ctx, cancel := context.WithCancel(t.Context())
	first := make(chan error, 1)
	go func() { _, err := s.SetModelLoaded(ctx, true); first <- err }()
	load := <-requests
	cancel() // The tab closes, or a settings save replaces the autoload caller.
	if err := <-first; err != context.Canceled {
		t.Fatalf("caller cancel = %v", err)
	}
	if got := s.Status(); got.ModelState != ModelStateLoading || got.LastError != "" {
		t.Fatalf("caller killed shared load: %+v", got)
	}
	joined := make(chan error, 1)
	go func() { _, err := s.SetModelLoaded(t.Context(), true); joined <- err }()
	reply(Response{Type: ResponseHealth, RequestID: load.ID, ModelState: ModelStateReady})
	if err := <-joined; err != nil {
		t.Fatal(err)
	}
	if got := s.Status(); got.ModelState != ModelStateReady || got.LastError != "" {
		t.Fatalf("load did not recover: %+v", got)
	}
	select {
	case extra := <-requests:
		t.Fatalf("replacement sent duplicate load: %+v", extra)
	default:
	}
}

func TestIdleStopPreservesCrashEvidence(t *testing.T) {
	s := NewSupervisor(RoleTTS)
	s.state = StateCrashed
	s.lastError = "test worker exhausted its memory budget"
	s.stderr = newTailBuffer(1024)
	_, _ = s.stderr.Write([]byte("allocation failed"))
	if err := s.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := s.Status(); got.State != StateCrashed || got.StderrTail != "allocation failed" {
		t.Fatalf("cleanup hid crash evidence: %+v", got)
	}
}

func TestWorkerHealthRecoversWithoutOpeningSettings(t *testing.T) {
	s := newTestSupervisor(t, RoleTTS, "-start-loaded")
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.lastHealth = Response{}
	s.lastError = "context deadline exceeded"
	s.mu.Unlock()
	deadline := time.After(15 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if got := s.Status(); got.ModelState == ModelStateReady && got.LastError == "" {
			return
		}
		select {
		case <-deadline:
			t.Fatal("background health never recovered")
		case <-tick.C:
		}
	}
}

func TestEarlierHealthCannotOverwriteCompletedModelLoad(t *testing.T) {
	s, requests, reply := controlledModelWorker(t)
	healthDone := make(chan Response, 1)
	go func() { response, _ := s.Health(t.Context()); healthDone <- response }()
	health := <-requests
	loadDone := make(chan error, 1)
	go func() { _, err := s.SetModelLoaded(t.Context(), true); loadDone <- err }()
	load := <-requests
	reply(Response{Type: ResponseHealth, RequestID: load.ID, ModelState: ModelStateReady})
	if err := <-loadDone; err != nil {
		t.Fatal(err)
	}
	reply(Response{Type: ResponseHealth, RequestID: health.ID, ModelState: ModelStateUnloaded})
	if result := <-healthDone; result.ModelState != ModelStateReady {
		t.Fatalf("stale probe won: %+v", result)
	}
	if got := s.Status(); got.ModelState != ModelStateReady {
		t.Fatalf("stale probe changed status: %+v", got)
	}
}

func TestModelFailureIsVisibleAndRetryClearsIt(t *testing.T) {
	s, requests, reply := controlledModelWorker(t)
	done := make(chan error, 1)
	go func() { _, err := s.SetModelLoaded(t.Context(), true); done <- err }()
	load := <-requests
	reply(Response{Type: ResponseError, RequestID: load.ID, Error: &WorkerError{Code: ErrorCodeMissingDependency, Message: "model files are missing"}})
	if err := <-done; err == nil {
		t.Fatal("failure hidden")
	}
	if got := s.Status(); got.LastError != "missing_dependency: model files are missing" {
		t.Fatalf("missing real error: %+v", got)
	}
	go func() { _, err := s.SetModelLoaded(t.Context(), true); done <- err }()
	load = <-requests
	if got := s.Status(); got.LastError != "" || got.ModelState != ModelStateLoading {
		t.Fatalf("retry kept stale error: %+v", got)
	}
	reply(Response{Type: ResponseHealth, RequestID: load.ID, ModelState: ModelStateReady})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestFailedAndCanceledRequestsRetainElapsedTime(t *testing.T) {
	for name, finish := range map[string]func(*PendingRequest){
		"failure":       func(p *PendingRequest) { p.fail(&WorkerError{Code: ErrorCodeInternal}) },
		"invalid audio": func(p *PendingRequest) { p.failAndCancel(&WorkerError{Code: ErrorCodeInternal}) },
		"timeout":       func(p *PendingRequest) { p.timeOut(&WorkerError{Code: ErrorCodeTimeout}) },
		"cancel":        func(p *PendingRequest) { p.markCanceled() },
		"stop":          func(p *PendingRequest) { p.invalidate() },
	} {
		t.Run(name, func(t *testing.T) {
			p := &PendingRequest{state: RequestStateActive, createdAt: time.Now().Add(-time.Second)}
			finish(p)
			if elapsed := p.Snapshot().CompletionMillis; elapsed < 1000 {
				t.Fatalf("terminal elapsed time = %d", elapsed)
			}
		})
	}
}

// A slow real adapter load deliberately occupies its protocol reader. Status
// polls must describe that operation, not enqueue health probes that time out.
func TestModelLoadStatusRemainsPendingDuringSlowStartup(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(mock.Close)
	t.Cleanup(func() { close(release) })
	s := NewSupervisor(RoleTTS)
	s.SetConfig(WorkerConfig{Enabled: true, Command: buildWorkerBinary(t, "./cmd/voice-openai-tts-worker", "voice-openai-tts-worker"), Args: []string{"-base-url", mock.URL}})
	t.Cleanup(s.Shutdown)
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.SetModelLoaded(t.Context(), true); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("load never reached provider")
	}
	if got := s.Status(); got.ModelState != ModelStateLoading || got.LastError != "" {
		t.Errorf("in-progress load looks failed or idle: %+v", got)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	health, err := s.Health(ctx)
	if err != nil || health.ModelState != ModelStateLoading {
		t.Errorf("status probe during load = %+v, %v", health, err)
	}
	if got := s.Status(); got.LastError != "" {
		t.Errorf("loading generated a false error: %s", got.LastError)
	}
	// Stop must release every waiter; a late load result cannot revive readiness.
	if err := s.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("load survived Stop")
	}
	if got := s.Status(); got.State != StateStopped || got.ModelState != "" || got.LastError != "" {
		t.Fatalf("late load result overwrote stopped status: %+v", got)
	}
}
