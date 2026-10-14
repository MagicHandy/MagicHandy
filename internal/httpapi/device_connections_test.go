package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func editConnection(t *testing.T, s *Server, device, action string) {
	t.Helper()
	r := withController(httptest.NewRequest(http.MethodPut, "/api/settings/device/connections", strings.NewReader(fmt.Sprintf(`{"device":%q,"action":%q}`, device, action))))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s: %d %s", action, device, w.Code, w.Body.String())
	}
}

func TestExcludedHandyDisconnectLeavesCoyotePlaybackRunning(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	connectCoyote(t, s)
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(context.Background(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 40}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	if err := s.stopForHandyDisconnect(context.Background(), "excluded_handy_disconnect"); err != nil {
		t.Fatal(err)
	}
	if s.currentMotionEngine() != engine || !engine.Snapshot().Running {
		t.Fatal("excluded device disconnect stopped playback")
	}
	if _, err := engine.Stop(context.Background(), "test_cleanup"); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionInventoryRequiresControl(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputHandy, true)
	r := httptest.NewRequest(http.MethodPut, "/api/settings/device/connections", strings.NewReader(`{"device":"coyote","action":"add"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("unowned mutation: %d %s", w.Code, w.Body.String())
	}
	settings, _ := s.store.Snapshot()
	if settings.Device.HasDevice("coyote") {
		t.Fatal("unowned mutation was saved")
	}
}

func TestChangingLinkedFailurePolicyStopsTheCurrentRun(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputHandyCoyote, true)
	connectCoyote(t, s)
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	previous, _ := s.store.Snapshot()
	if _, err := engine.Start(context.Background(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 40}, previous.Motion); err != nil {
		t.Fatal(err)
	}
	next := previous
	next.Device.LinkedFailurePolicy = config.LinkedFailureContinueHealthy
	if err := s.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatal(err)
	}
	if engine.Snapshot().Running || s.currentMotionEngine() != nil {
		t.Fatal("policy hot-swapped into a live run")
	}
}

func TestConnectionInventoryAndPlaybackAreIndependent(t *testing.T) {
	s, connector := newCoyoteTestServer(t, config.DeviceOutputHandy, true)
	editConnection(t, s, "coyote", "add")
	settings, _ := s.store.Snapshot()
	if settings.Device.Output != config.DeviceOutputHandy || connector.LastLink() != nil || s.currentMotionEngine() != nil {
		t.Fatal("Add connected or started playback")
	}
	connectCoyote(t, s)
	editConnection(t, s, "coyote", "include")
	settings, _ = s.store.Snapshot()
	if settings.Device.Output != config.DeviceOutputHandyCoyote {
		t.Fatal("include not saved")
	}
	editConnection(t, s, "coyote", "exclude")
	if !s.coyote.owner.Status().Connected || connector.LastLink().Snapshot().Closed {
		t.Fatal("exclude disconnected Coyote")
	}
	if _, err := s.coyote.owner.Disconnect(); err != nil {
		t.Fatal(err)
	}
	settings, _ = s.store.Snapshot()
	if !settings.Device.HasDevice("coyote") {
		t.Fatal("disconnect removed saved device")
	}
	connectCoyote(t, s)
	editConnection(t, s, "coyote", "remove")
	if s.coyote.owner.Status().Connected || !connector.LastLink().Snapshot().Closed {
		t.Fatal("remove kept link alive")
	}
	editConnection(t, s, "handy", "remove")
	settings, _ = s.store.Snapshot()
	if settings.Device.Output != config.DeviceOutputNone || len(settings.Device.ConfiguredDevices) != 0 {
		t.Fatalf("last removal: %+v", settings.Device)
	}
	if _, _, err := s.motionEngineForStart(); err == nil {
		t.Fatal("chat-only configuration started motion")
	}
	if _, err := s.stopSelectedTransport(context.Background(), "empty_stop"); err != nil {
		t.Fatalf("Stop without devices: %v", err)
	}
}

func TestLinkedEngineRespondsToCoyoteLossAccordingToPolicy(t *testing.T) {
	for _, policy := range []string{config.LinkedFailureStopAll, config.LinkedFailureContinueHealthy} {
		t.Run(policy, func(t *testing.T) {
			s, connector := newCoyoteTestServer(t, config.DeviceOutputHandyCoyote, true)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.Device.LinkedFailurePolicy = policy
				return settings
			})
			connectCoyote(t, s)
			engine, _, err := s.motionEngineForStart()
			if err != nil {
				t.Fatal(err)
			}
			settings, _ := s.store.Snapshot()
			if _, err := engine.Start(context.Background(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 40}, settings.Motion); err != nil {
				t.Fatal(err)
			}
			connector.LastLink().Lose(fmt.Errorf("simulated radio loss"))
			waitCoyoteStatus(t, s, func(status transport.CoyoteStatus) bool { return !status.Connected })
			deadline := time.Now().Add(3 * time.Second)
			if policy == config.LinkedFailureStopAll {
				for engine.Snapshot().Running && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if state := engine.Snapshot(); state.Running || state.LastError == "" {
					t.Fatalf("linked fault did not stop engine: %+v", state)
				}
			} else {
				time.Sleep(350 * time.Millisecond)
				if !engine.Snapshot().Running {
					t.Fatal("healthy Handy did not continue")
				}
			}
			// A fresh connection stays silent, regardless of policy.
			connectCoyote(t, s)
			if s.coyote.owner.Status().PlaybackState != "idle" {
				t.Fatal("reconnect joined old run")
			}
			if _, err := engine.Stop(context.Background(), "test_stop"); err != nil {
				t.Fatal(err)
			}
			fresh, _, err := s.motionEngineForStart()
			if err != nil || fresh == engine {
				t.Fatalf("explicit restart must get a new session: %v", err)
			}
		})
	}
}

func TestEmptyInventoryCannotConnectAnUnconfiguredHandy(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputHandy, true)
	editConnection(t, s, "handy", "remove")
	for _, route := range []string{"cloud", "intiface", "bluetooth"} {
		r := withController(httptest.NewRequest(http.MethodPost, "/api/transport/"+route+"/connect", strings.NewReader(`{}`)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusConflict {
			t.Fatalf("%s: %d %s", route, w.Code, w.Body.String())
		}
	}
}
