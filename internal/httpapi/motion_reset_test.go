package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type positionResetTransport struct {
	*transport.Fake
	mu          sync.Mutex
	position    float64
	readEntered chan struct{}
}

func (p *positionResetTransport) ReadMotionStartupState(ctx context.Context) (transport.MotionStartupState, transport.MotionStartupStateResults, error) {
	p.mu.Lock()
	position, entered := p.position, p.readEntered
	p.mu.Unlock()
	if entered != nil {
		close(entered)
		<-ctx.Done()
		return transport.MotionStartupState{}, transport.MotionStartupStateResults{}, ctx.Err()
	}
	return transport.MotionStartupState{PositionAbsolute: position, PositionWithinStrokePercent: position,
		StrokeMaxPercent: 100, StrokeMaxAbsolute: 100}, transport.MotionStartupStateResults{}, nil
}

func positionResetServer(t *testing.T) (*Server, *positionResetTransport, string) {
	t.Helper()
	owner := &positionResetTransport{Fake: transport.NewFake(), position: 120}
	s := newTestServerWithRuntime(t, Runtime{Transport: owner, MotionTransport: owner})
	e, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	state, err := e.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 20}, settings.Motion)
	if !errors.Is(err, motion.ErrUnsafeStartupState) || state.StartupRecovery == nil {
		t.Fatalf("fixture did not fail positioning: %v", err)
	}
	owner.mu.Lock()
	owner.position = 50
	owner.mu.Unlock()
	return s, owner, state.StartupRecovery.ID
}

func resetPositionRequest(s *Server, id, client string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/motion/reset-position", strings.NewReader(fmt.Sprintf(`{"failure_id":%q}`, id)))
	r.Header.Set("Content-Type", "application/json")
	r = withControllerID(r, client)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestPositionFailureRemainsVisibleAndCanBeReset(t *testing.T) {
	s, owner, id := positionResetServer(t)
	state := callMotion(t, s, http.MethodGet, "/api/motion/state", "")
	if state.Engine.StartupRecovery == nil || state.Engine.StartupRecovery.ID != id || state.Engine.Running {
		t.Fatalf("idle snapshot lost the recovery action: %+v", state.Engine.StartupRecovery)
	}
	before := len(owner.Commands())
	w := resetPositionRequest(s, id, "test-controller")
	if w.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	var result motionEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Engine.Running || result.Engine.Paused || result.Engine.StartupRecovery != nil {
		t.Fatalf("reset resumed or kept the fault: %+v", result.Engine.StartupRecovery)
	}
	commands := owner.Commands()[before:]
	if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop {
		t.Fatalf("reset sent motion: %+v", commands)
	}
	if s.stopSequence.Load() == 0 {
		t.Fatal("reset did not invalidate pending motion work")
	}
	before = len(owner.Commands())
	if w := resetPositionRequest(s, id, "test-controller"); w.Code != http.StatusConflict || len(owner.Commands()) != before {
		t.Fatalf("stale request touched the device: %d", w.Code)
	}
}

func TestPositionResetRejectsReadOnlyAndWrongFailure(t *testing.T) {
	s, owner, id := positionResetServer(t)
	// Controller admission claims this tab even when its failure ID is stale.
	resetPositionRequest(s, "old-failure", "test-controller")
	before := len(owner.Commands())
	for _, request := range []struct{ id, client string }{{id, "other-tab"}, {"old-failure", "test-controller"}} {
		w := resetPositionRequest(s, request.id, request.client)
		if w.Code != http.StatusConflict || len(owner.Commands()) != before {
			t.Fatalf("invalid reset accepted: status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestEmergencyStopInterruptsPositionRecheckHTTP(t *testing.T) {
	s, owner, id := positionResetServer(t)
	entered := make(chan struct{})
	owner.mu.Lock()
	owner.readEntered = entered
	owner.mu.Unlock()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- resetPositionRequest(s, id, "test-controller") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reset did not reach read")
	}
	stopDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader("{}")))
		stopDone <- w
	}()
	select {
	case w := <-stopDone:
		if w.Code != http.StatusOK {
			t.Fatalf("Stop: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop waited on the reset")
	}
	select {
	case w := <-done:
		if w.Code == http.StatusOK {
			t.Fatal("canceled recheck was acknowledged")
		}
	case <-time.After(time.Second):
		t.Fatal("recheck did not exit")
	}
	if state := s.currentMotionEngine().Snapshot(); state.Running || state.StartupRecovery == nil || state.StartupRecovery.Resetting {
		t.Fatal("Stop lost the pending failure")
	}
}

func TestAuthenticatedPositionResetAndStop(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		t.Run(fmt.Sprintf("interrupt=%v", interrupt), func(t *testing.T) {
			s, _, _, cookie := newControllerSessionFixture(t)
			owner := &positionResetTransport{Fake: transport.NewFake(), position: 120}
			s.motion.mu.Lock()
			s.motion.transport = owner
			s.motion.mu.Unlock()
			claimAuthenticatedController(t, s, cookie)
			e, _, err := s.motionEngineForStart()
			if err != nil {
				t.Fatal(err)
			}
			settings, _ := s.store.Snapshot()
			state, err := e.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 20}, settings.Motion)
			if !errors.Is(err, motion.ErrUnsafeStartupState) || state.StartupRecovery == nil {
				t.Fatalf("fixture: %v", err)
			}
			entered := make(chan struct{})
			owner.mu.Lock()
			owner.position = 50
			if interrupt {
				owner.readEntered = entered
			}
			owner.mu.Unlock()
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- authenticatedControlRequest(s, cookie, http.MethodPost, "/api/motion/reset-position", "test-controller", fmt.Sprintf(`{"failure_id":%q}`, state.StartupRecovery.ID))
			}()
			if interrupt {
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("authenticated reset did not start")
				}
				stopDone := make(chan *httptest.ResponseRecorder, 1)
				go func() { stopDone <- serveCommand(s, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil)) }()
				select {
				case w := <-stopDone:
					if w.Code != http.StatusOK {
						t.Fatalf("Stop: %d %s", w.Code, w.Body.String())
					}
				case <-time.After(2 * time.Second):
					t.Fatal("authenticated reset blocked Stop")
				}
			}
			select {
			case w := <-done:
				if (w.Code == http.StatusOK) == interrupt {
					t.Fatalf("reset: %d %s", w.Code, w.Body.String())
				}
			case <-time.After(time.Second):
				t.Fatal("authenticated reset did not finish")
			}
			if e.Snapshot().Running {
				t.Fatal("reset resumed motion")
			}
		})
	}
}
