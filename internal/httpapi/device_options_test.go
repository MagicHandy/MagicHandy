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

func settingsRequest(t *testing.T, s *Server, method, path, body string, controller bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if controller {
		r = withController(r)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestDeviceOptionsPatchSavesOneSettingAndConfiguresTheCoyote(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputHandyCoyote, true)
	before, _ := s.store.Snapshot()
	w := settingsRequest(t, s, http.MethodPatch, "/api/settings/device", `{"estim":{"max_strength_a":45}}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	var payload struct {
		Settings config.PublicSettings `json:"settings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Settings.Estim.MaxStrengthA != 45 {
		t.Fatalf("response settings: %+v", payload.Settings.Estim)
	}
	after, _ := s.store.Snapshot()
	if after.Estim.MaxStrengthA != 45 || after.Estim.MaxStrengthB != before.Estim.MaxStrengthB || after.Vibration != before.Vibration {
		t.Fatalf("saved: %+v %+v", after.Estim, after.Vibration)
	}
	if status := s.coyote.owner.Status(); status.CeilingA != 45 {
		t.Fatalf("the Coyote kept its old ceiling: %+v", status)
	}
	if strings.Contains(w.Body.String(), "handy_connection_key") {
		t.Fatal("the response exposed the connection key field")
	}
}

func TestDeviceOptionsPatchRequiresControlAndValidSettings(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputHandy, true)
	if w := settingsRequest(t, s, http.MethodPatch, "/api/settings/device", `{"linked_failure_policy":"continue_healthy"}`, false); w.Code != http.StatusConflict {
		t.Fatalf("unowned patch: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{`{}`, `{"vibration":{"ceiling_percent":101}}`, `{"intiface_server_address":"ftp://example"}`, `{"estim":{"follow":"tickle"}}`} {
		if w := settingsRequest(t, s, http.MethodPatch, "/api/settings/device", body, true); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	settings, _ := s.store.Snapshot()
	if settings.Device.LinkedFailurePolicy != config.LinkedFailureStopAll || settings.Vibration.CeilingPercent != config.VibrationDefaultCeilingPercent {
		t.Fatalf("a rejected patch was saved: %+v %+v", settings.Device, settings.Vibration)
	}
}

func TestDeviceOptionsPatchOfTheFailurePolicyStopsTheRun(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputHandyCoyote, true)
	connectCoyote(t, s)
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(context.Background(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 40}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	if w := settingsRequest(t, s, http.MethodPatch, "/api/settings/device", `{"linked_failure_policy":"continue_healthy"}`, true); w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	if engine.Snapshot().Running || s.currentMotionEngine() != nil {
		t.Fatal("the new failure policy reached a live run")
	}
}

func TestServerPortSavesForTheNextStart(t *testing.T) {
	s, _ := newCoyoteTestServer(t, config.DeviceOutputHandy, true)
	if w := settingsRequest(t, s, http.MethodPut, "/api/settings/server", `{"port":8123}`, false); w.Code != http.StatusConflict {
		t.Fatalf("unowned port change: %d %s", w.Code, w.Body.String())
	}
	for _, body := range []string{`{"port":0}`, `{"port":65536}`, `{}`} {
		if w := settingsRequest(t, s, http.MethodPut, "/api/settings/server", body, true); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	if w := settingsRequest(t, s, http.MethodPut, "/api/settings/server", `{"port":8123}`, true); w.Code != http.StatusOK {
		t.Fatalf("port: %d %s", w.Code, w.Body.String())
	}
	if settings, _ := s.store.Snapshot(); settings.Server.Port != 8123 {
		t.Fatalf("port not saved: %+v", settings.Server)
	}
}
