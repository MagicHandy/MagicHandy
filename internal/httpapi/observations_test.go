package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type heldObservationDiagnostics struct {
	*transport.Fake
	blocked atomic.Bool
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (d *heldObservationDiagnostics) Diagnostics() transport.TransportDiagnostics {
	if d.blocked.Load() {
		d.once.Do(func() { close(d.entered) })
		<-d.resume
	}
	return d.Fake.Diagnostics()
}

func serveObservationRequest(s *Server, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, withController(httptest.NewRequest(http.MethodGet, path, nil)))
	return w
}

func TestSlowFullStateDoesNotBlockMotionObservationAndRetainsCaptureOrder(t *testing.T) {
	diagnostics := &heldObservationDiagnostics{Fake: transport.NewFake(), entered: make(chan struct{}), resume: make(chan struct{})}
	s := newTestServerWithRuntime(t, Runtime{Transport: diagnostics, MotionTransport: transport.NewFake()})
	diagnostics.blocked.Store(true)
	fullDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { fullDone <- serveObservationRequest(s, "/api/state") }()
	released := false
	defer func() {
		if !released {
			close(diagnostics.resume)
		}
	}()
	select {
	case <-diagnostics.entered:
	case <-time.After(time.Second):
		t.Fatal("full snapshot did not reach the blocked diagnostics read")
	}
	motionDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { motionDone <- serveObservationRequest(s, "/api/motion/state") }()
	var motionResult *httptest.ResponseRecorder
	select {
	case motionResult = <-motionDone:
	case <-time.After(time.Second):
		t.Fatal("slow full-state diagnostics blocked the independent motion observation")
	}
	close(diagnostics.resume)
	released = true
	var fullResult *httptest.ResponseRecorder
	select {
	case fullResult = <-fullDone:
	case <-time.After(time.Second):
		t.Fatal("full snapshot did not finish after diagnostics were released")
	}
	var complete struct {
		Observation observationStamp `json:"observation"`
		Motion      struct {
			Observation observationStamp `json:"observation"`
		} `json:"motion"`
	}
	var independent struct {
		Observation observationStamp `json:"observation"`
	}
	if err := json.Unmarshal(fullResult.Body.Bytes(), &complete); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(motionResult.Body.Bytes(), &independent); err != nil {
		t.Fatal(err)
	}
	if complete.Observation.Epoch == "" || complete.Observation.Epoch != independent.Observation.Epoch ||
		complete.Motion.Observation.Epoch != complete.Observation.Epoch {
		t.Fatal("observation channels do not share the server boot identity")
	}
	if complete.Observation.Revision >= independent.Observation.Revision || independent.Observation.Revision >= complete.Motion.Observation.Revision {
		t.Fatalf("capture revisions follow response completion instead of observation order: app=%d independent=%d embedded=%d",
			complete.Observation.Revision, independent.Observation.Revision, complete.Motion.Observation.Revision)
	}
	if complete.Observation.ObservedAt.IsZero() || independent.Observation.ObservedAt.IsZero() {
		t.Fatal("observation timestamp omitted")
	}
	for _, response := range []*httptest.ResponseRecorder{motionResult, fullResult} {
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("snapshot response can be cached or failed: %d %+v", response.Code, response.Header())
		}
	}
}

func TestControllerObservationsOrderRenewalWithinAnOwnershipGeneration(t *testing.T) {
	c := newControllerRuntime()
	actor := controllerIdentity{clientID: "tab", sessionKey: "session"}
	first := c.Heartbeat(actor, 0)
	observed := c.Observe(actor)
	renewed := c.Heartbeat(actor, first.Generation)
	if first.Generation != observed.Generation || observed.Generation != renewed.Generation {
		t.Fatal("ordinary observation unexpectedly changed ownership")
	}
	if first.Revision == 0 || first.Revision >= observed.Revision || observed.Revision >= renewed.Revision {
		t.Fatal("controller snapshots lack monotonic read revisions")
	}
}
