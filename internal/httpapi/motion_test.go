package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type motionEnvelope struct {
	Available bool                     `json:"available"`
	Error     string                   `json:"error"`
	Engine    motion.ActiveMotionState `json:"engine"`
}

func callMotion(t *testing.T, server *Server, method, path, body string) motionEnvelope {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d, body = %s", method, path, recorder.Code, recorder.Body.String())
	}
	var envelope motionEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
	return envelope
}

func TestMotionStartStateStop(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":60}`)
	if !started.Available || !started.Engine.Running {
		t.Fatalf("expected running motion after start, got %+v", started)
	}
	if started.Engine.Target.SpeedPercent != 60 {
		t.Fatalf("target speed = %d, want 60", started.Engine.Target.SpeedPercent)
	}

	state := callMotion(t, server, http.MethodGet, "/api/motion/state", "")
	if !state.Engine.Running {
		t.Fatalf("state should report running: %+v", state)
	}

	stopped := callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)
	if stopped.Engine.Running {
		t.Fatalf("motion should be stopped, got %+v", stopped)
	}
}

func TestMotionStartClampsSpeedToSettings(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.SpeedMinPercent = 10
		settings.Motion.SpeedMaxPercent = 40
		return settings
	})

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":95}`)
	if started.Engine.Target.SpeedPercent != 40 {
		t.Fatalf("speed should clamp to max 40, got %d", started.Engine.Target.SpeedPercent)
	}
}

func TestMotionRefreshAppliesToActiveLoopImmediately(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := callMotion(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":70}`)
	if !started.Engine.Running {
		t.Fatalf("expected running motion")
	}

	tighter := config.MotionSettings{SpeedMinPercent: 10, SpeedMaxPercent: 25, StrokeMinPercent: 0, StrokeMaxPercent: 100}
	server.refreshActiveMotion(context.Background(), tighter)

	snapshot := server.motion.engine.Snapshot()
	if !snapshot.Running {
		t.Fatalf("refresh must not stop active motion")
	}
	if snapshot.Settings.SpeedMaxPercent != 25 {
		t.Fatalf("settings not applied: max = %d, want 25", snapshot.Settings.SpeedMaxPercent)
	}
	if snapshot.Target.SpeedPercent > 25 {
		t.Fatalf("target speed %d should be reclamped to <= 25", snapshot.Target.SpeedPercent)
	}
}

func TestMotionStateExposedInAggregateState(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	var body struct {
		Motion struct {
			Available bool `json:"available"`
		} `json:"motion"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if !body.Motion.Available {
		t.Fatalf("state should report motion available with a command transport")
	}
}
