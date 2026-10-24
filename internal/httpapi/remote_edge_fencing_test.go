package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

// Stop and handoff synchronously fence the old controller before stopping the
// engine. A phone press admitted just before that fence must not apply after
// it, even if the same target is still running while physical Stop catches up.
func TestRemoteEdgeRequestsEndWithTheirDesktopAuthority(t *testing.T) {
	for _, fence := range []string{"Stop", "handoff"} {
		for _, returning := range []bool{false, true} {
			name := fence + "/Edge"
			if returning {
				name = fence + "/Return"
			}
			t.Run(name, func(t *testing.T) {
				s, store, admin, desktop, _ := newRemoteFixture(t)
				startRemoteFixtureMotion(t, s, desktop)
				_, phone := remoteOnlyLogin(t, s, store, admin, desktop)
				engine := s.currentMotionEngine()
				s.modes.Shutdown()
				entered, release := make(chan struct{}), make(chan struct{})
				unblock := sync.OnceFunc(func() { close(release) })
				t.Cleanup(unblock)
				var armed atomic.Bool
				var once sync.Once
				manager, err := modes.NewManager(modes.Options{
					Ensure: func(context.Context) (modes.Engine, error) { return engine, nil },
					Current: func() modes.Engine {
						if armed.Load() {
							once.Do(func() { close(entered); <-release })
						}
						return engine
					},
					Settings: func() config.MotionSettings { settings, _ := s.store.Snapshot(); return settings.Motion },
				})
				if err != nil {
					t.Fatal(err)
				}
				s.modes = manager
				route := "/api/remote/edge"
				if returning {
					if response := remotePortalRequest(s, phone, http.MethodPost, route, `{}`); response.Code != http.StatusOK {
						t.Fatalf("initial hold: %d %s", response.Code, response.Body.String())
					}
					route += "/resume"
				}
				before := engine.Snapshot()
				armed.Store(true)
				responses := make(chan *httptest.ResponseRecorder, 1)
				go func() { responses <- remotePortalRequest(s, phone, http.MethodPost, route, `{}`) }()
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("phone request did not reach mode application")
				}
				if fence == "Stop" {
					s.controller.AdvanceStopGeneration()
				} else {
					next := s.controller.Owner()
					next.clientID = "next-desktop-tab"
					if _, started, err := s.controller.BeginTakeover(next); err != nil || !started {
						t.Fatalf("handoff fence: %v %v", started, err)
					}
					t.Cleanup(func() { s.controller.CancelTakeover(next) })
				}
				unblock()
				select {
				case response := <-responses:
					if response.Code != http.StatusConflict {
						t.Errorf("retired phone request: %d %s", response.Code, response.Body.String())
					}
				case <-time.After(5 * time.Second):
					t.Fatal("retired phone request did not finish")
				}
				if after := engine.Snapshot(); after.PlanID != before.PlanID || after.Target.SpeedPercent != before.Target.SpeedPercent {
					t.Fatal("retired phone request changed the engine target")
				}
			})
		}
	}
}

func TestLocalStopCancelsBoundPhoneRequestsWithoutRetiringTheController(t *testing.T) {
	c := newControllerRuntime()
	owner := controllerIdentity{clientID: "local-desktop"}
	before := c.Observe(owner)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	detach := c.bindRequest(owner, cancel)
	if detach == nil {
		t.Fatal("local phone request could not bind to its desktop")
	}
	defer detach()
	c.AdvanceStopGeneration()
	if ctx.Err() == nil {
		t.Fatal("Stop left a local phone request active")
	}
	if after := c.Peek(owner); !after.Active || after.Generation != before.Generation {
		t.Fatal("Stop changed the unprotected controller's existing lease policy")
	}
}
