package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestLsoGetSettingsReturnsHandyConnectionKey(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
		settings.Device.HandyConnectionKey = "rySSd"
		return settings
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	handy, _ := payload["handy"].(map[string]any)
	if handy == nil {
		t.Fatalf("payload = %#v, want handy section", payload)
	}
	if handy["connection_key"] != "rySSd" {
		t.Fatalf("connection_key = %#v, want rySSd", handy["connection_key"])
	}
	if handy["transport"] != "handy_cloud" {
		t.Fatalf("transport = %#v, want handy_cloud", handy["transport"])
	}
}

func TestLsoPutSettingsUpdatesHandyKey(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{
		"updates": {
			"handy": {
				"transport": "handy_cloud",
				"connection_key": "rySSd"
			}
		}
	}`)))
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if settings.Device.HandyConnectionKey != "rySSd" {
		t.Fatalf("connection key = %q, want rySSd", settings.Device.HandyConnectionKey)
	}
	if settings.Device.HSPDispatchOwner != config.DispatchOwnerCloudREST {
		t.Fatalf("dispatch owner = %q, want cloud_rest", settings.Device.HSPDispatchOwner)
	}
}

func TestLsoPutSettingsUpdatesUserProfile(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{
		"updates": {
			"user_profile": {
				"gender": "female",
				"sexual_orientation": "bisexual",
				"about_me": "Prefiro ser chamada de amor."
			}
		}
	}`)))
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if settings.UserProfile.Gender != config.UserGenderFemale {
		t.Fatalf("gender = %q, want female", settings.UserProfile.Gender)
	}
	if settings.UserProfile.SexualOrientation != config.UserOrientationBisexual {
		t.Fatalf("orientation = %q, want bisexual", settings.UserProfile.SexualOrientation)
	}
	if settings.UserProfile.AboutMe != "Prefiro ser chamada de amor." {
		t.Fatalf("about_me = %q", settings.UserProfile.AboutMe)
	}
}

func TestLsoPutSettingsUpdatesChatAutoFields(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{
		"updates": {
			"autodom": {
				"wait_for_user_message": false,
				"segment_duration_min_sec": 48,
				"segment_duration_max_sec": 58,
				"prefetch_lead_seconds": 12
			}
		}
	}`)))
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if settings.AutoDom.ShouldWaitForUserMessage() {
		t.Fatal("expected wait_for_user_message=false")
	}
	if settings.AutoDom.SegmentDurationMinSec != 48 {
		t.Fatalf("min = %d, want 48", settings.AutoDom.SegmentDurationMinSec)
	}
	if settings.AutoDom.PrefetchLeadSeconds != 12 {
		t.Fatalf("prefetch = %d, want 12", settings.AutoDom.PrefetchLeadSeconds)
	}
}

func TestLsoPutSettingsUpdatesDiagnosticsVerbose(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{
		"updates": {
			"diagnostics": {
				"log_handy_motion": true,
				"log_handy_motion_verbose": true
			}
		}
	}`)))
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if !settings.Diagnostics.ShouldLogHandyMotion() {
		t.Fatal("expected log_handy_motion=true")
	}
	if !settings.Diagnostics.VerboseHandyMotion() {
		t.Fatal("expected log_handy_motion_verbose=true")
	}

	// Simulate restart: reload from store write path.
	reloaded, err := config.OpenStore(server.store.DataDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer reloaded.Close()
	persisted, _ := reloaded.Snapshot()
	if !persisted.Diagnostics.VerboseHandyMotion() {
		t.Fatal("expected verbose handy motion to persist across reload")
	}
}

func TestLsoPutSettingsMergesMotionWithoutWipingMode(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeSynsual
		settings.Motion.SpeedMinPercent = 25
		settings.Motion.HardwareSafetyLock = true
		return settings
	})

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{
		"updates": {
			"motion": {
				"speed_min_percent": 40
			}
		}
	}`)))
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if settings.Motion.SpeedMinPercent != 40 {
		t.Fatalf("speed_min = %d, want 40", settings.Motion.SpeedMinPercent)
	}
	if settings.Motion.MotionGenerationMode != config.MotionGenerationModeSynsual {
		t.Fatalf("mode = %q, want synsual", settings.Motion.MotionGenerationMode)
	}
	if !settings.Motion.HardwareSafetyLock {
		t.Fatal("expected hardware_safety_lock to remain true")
	}
}

func TestLsoPutSettingsPersistsUISections(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{
		"updates": {
			"safety": {
				"limits_enabled": false,
				"stop_words": ["red", "stop"]
			},
			"llm": {
				"prompt_set": "clarissa_synsual_v1"
			}
		}
	}`)))
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if settings.LLM.PromptSet != config.PromptSetClarissaSynsualV1 {
		t.Fatalf("prompt_set = %q, want %q", settings.LLM.PromptSet, config.PromptSetClarissaSynsualV1)
	}
	raw, ok := settings.UISections["safety"]
	if !ok {
		t.Fatal("expected safety ui section to be stored")
	}
	if !strings.Contains(string(raw), "limits_enabled") {
		t.Fatalf("safety section = %s", string(raw))
	}

	reloaded, err := config.OpenStore(server.store.DataDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer reloaded.Close()
	persisted, _ := reloaded.Snapshot()
	if persisted.LLM.PromptSet != config.PromptSetClarissaSynsualV1 {
		t.Fatalf("reloaded prompt_set = %q", persisted.LLM.PromptSet)
	}
	if _, ok := persisted.UISections["safety"]; !ok {
		t.Fatal("expected safety ui section to persist across reload")
	}
}

func TestDeviceTransportPostKeepsExistingKeyWhenBlank(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
		settings.Device.HandyConnectionKey = "rySSd"
		return settings
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/device/transport", strings.NewReader(`{
		"transport": "handy_cloud",
		"connection_key": ""
	}`))
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	settings, _ := server.store.Snapshot()
	if settings.Device.HandyConnectionKey != "rySSd" {
		t.Fatalf("connection key = %q, want preserved rySSd", settings.Device.HandyConnectionKey)
	}
}
