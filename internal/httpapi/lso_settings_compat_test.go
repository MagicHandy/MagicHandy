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
