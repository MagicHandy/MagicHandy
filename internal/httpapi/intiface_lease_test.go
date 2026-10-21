package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func connectedIntifaceServer(t *testing.T) (*Server, *httpAPIButtplugServer) {
	t.Helper()
	fake := newHTTPAPIButtplugServer(t)
	server := newTestServerWithRuntime(t, Runtime{})
	t.Cleanup(func() {
		server.Close()
		fake.Close()
	})
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerIntiface
		settings.Device.IntifaceServerAddress = fake.URL()
		return settings
	})
	if snapshot := callIntifaceAPI(t, server, http.MethodPost, "/api/transport/intiface/connect", `{}`); !snapshot.Status.Connected {
		t.Fatalf("connect = %+v", snapshot)
	}
	return server, fake
}

func TestGlobalStopSendsStopAllDevicesOnALiveIntifaceSession(t *testing.T) {
	server, fake := connectedIntifaceServer(t)
	if snapshot := server.intifaceSnapshot(); len(snapshot.Leases) != 1 || snapshot.Leases[0] != config.DeviceOutputHandy || !snapshot.PingWatchdog {
		t.Fatalf("lease snapshot = %+v", snapshot)
	}
	// No device is selected and no engine exists: Stop still reaches the
	// server, which stops every device it has.
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("Stop = %d: %s", recorder.Code, recorder.Body.String())
	}
	fake.waitForKind(t, "StopAllDevices")
}

func TestIntifaceAddressChangeClosesTheSharedSession(t *testing.T) {
	server, _ := connectedIntifaceServer(t)
	previous, _ := server.store.Snapshot()
	next := previous
	next.Device.IntifaceServerAddress = "ws://127.0.0.1:1"
	if err := server.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatal(err)
	}
	if snapshot := server.intifaceSnapshot(); snapshot.Status.Connected || len(snapshot.Leases) != 0 {
		t.Fatalf("a new address kept the old session: %+v", snapshot)
	}
}

func TestHandyLeavingIntifaceReleasesItsLease(t *testing.T) {
	server, _ := connectedIntifaceServer(t)
	previous, _ := server.store.Snapshot()
	next := previous
	next.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
	if err := server.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatal(err)
	}
	// The Handy held the only lease, so the shared session closes.
	if snapshot := server.intifaceSnapshot(); snapshot.Status.Connected || len(snapshot.Leases) != 0 {
		t.Fatalf("the last lease ended but the session stayed: %+v", snapshot)
	}
}

func TestIntifaceDisconnectRejectsDevicesThatDoNotUseIt(t *testing.T) {
	server, _ := connectedIntifaceServer(t)
	request := withController(httptest.NewRequest(http.MethodPost, "/api/transport/intiface/disconnect", strings.NewReader(`{"device":"coyote"}`)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("Coyote disconnect through Intiface = %d", recorder.Code)
	}
	if !server.intifaceSnapshot().Status.Connected {
		t.Fatal("a rejected disconnect closed the session")
	}
}
