package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/handyble"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func nativeRequest(t *testing.T, s *Server, action string, control bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/transport/native-bluetooth/"+action, strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	if control {
		r = withController(r)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func nativeTestServer(t *testing.T) (*Server, *handyble.SimulatedConnector) {
	t.Helper()
	c := handyble.NewSimulatedConnector()
	s := newTestServerWithRuntime(t, Runtime{NativeBluetoothConnector: c, CoyoteConnector: estim.NewSimulatedConnector()})
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerNativeBluetooth
		return settings
	})
	return s, c
}

func TestNativeSelectionDoesNotConnectAndEndpointsRequireControl(t *testing.T) {
	s, c := nativeTestServer(t)
	if c.LastLink() != nil || s.nativeBluetooth.owner.Status().Connected {
		t.Fatal("selection opened a radio link")
	}
	if w := nativeRequest(t, s, "connect", false); w.Code != http.StatusConflict {
		t.Fatalf("unowned connect %d %s", w.Code, w.Body.String())
	}
	if w := nativeRequest(t, s, "connect", true); w.Code != http.StatusOK {
		t.Fatalf("connect %d %s", w.Code, w.Body.String())
	}
	if s.currentMotionEngine() != nil {
		t.Fatal("connect started motion")
	}
	settings, _ := s.store.Snapshot()
	if settings.Device.NativeBluetoothAddress != "" {
		t.Fatal("simulated address persisted")
	}
	if w := nativeRequest(t, s, "forget", true); w.Code != http.StatusConflict {
		t.Fatalf("forgot connected device: %d", w.Code)
	}
	if w := nativeRequest(t, s, "disconnect", true); w.Code != http.StatusOK {
		t.Fatalf("disconnect %d %s", w.Code, w.Body.String())
	}
}

func TestNativePrivateAddressAndProtocolChange(t *testing.T) {
	s, _ := nativeTestServer(t)
	if w := nativeRequest(t, s, "connect", true); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Device.NativeBluetoothAddress = "AA:BB:CC:DD:EE:FF"
		return settings
	})
	settings, _ := s.store.Snapshot()
	data, _ := json.Marshal(settings.Public())
	status, _ := json.Marshal(s.nativeBluetoothSnapshot())
	if bytes.Contains(data, []byte("AA:BB")) || bytes.Contains(status, []byte("AA:BB")) {
		t.Fatal("hardware address leaked")
	}
	_, _, saveErr, runtimeErr := s.updateSettingsAndRuntime(t.Context(), func(settings config.Settings) (config.Settings, error) {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
		return settings, nil
	})
	if saveErr != nil || runtimeErr != nil {
		t.Fatalf("change protocol: %v %v", saveErr, runtimeErr)
	}
	if s.nativeBluetooth.owner.Status().Connected {
		t.Fatal("protocol change retained native connection")
	}
}

func TestGlobalStopReachesPreparedNativeHandyOutsideCoyoteEngine(t *testing.T) {
	s, c := nativeTestServer(t)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Device = settings.Device.WithPlaybackOutput(config.DeviceOutputHandyCoyote)
		settings.Device.Output = config.DeviceOutputCoyote
		settings.Estim.SafetyAcknowledged = true
		return settings
	})
	if w := nativeRequest(t, s, "connect", true); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	connectCoyote(t, s)
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 30}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	before := len(c.LastLink().Writes())
	if _, err := s.emergencyStop(t.Context(), "prepared_native_test"); err != nil {
		t.Fatal(err)
	}
	writes := c.LastLink().Writes()
	if len(writes) <= before || !bytes.Contains(writes[len(writes)-1], []byte{0x82, 0x36, 0x00}) {
		t.Fatal("global Stop did not reach prepared native Handy")
	}
	if !s.nativeBluetooth.owner.Status().Connected {
		t.Fatal("Stop unexpectedly removed the prepared connection")
	}
}

func TestNativeHandyLinkedFailurePolicies(t *testing.T) {
	for _, policy := range []string{config.LinkedFailureStopAll, config.LinkedFailureContinueHealthy} {
		t.Run(policy, func(t *testing.T) {
			s, c := nativeTestServer(t)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.Device = settings.Device.WithPlaybackOutput(config.DeviceOutputHandyCoyote)
				settings.Device.LinkedFailurePolicy = policy
				settings.Estim.SafetyAcknowledged = true
				return settings
			})
			if w := nativeRequest(t, s, "connect", true); w.Code != http.StatusOK {
				t.Fatal(w.Body.String())
			}
			connectCoyote(t, s)
			engine, _, err := s.motionEngineForStart()
			if err != nil {
				t.Fatal(err)
			}
			settings, _ := s.store.Snapshot()
			if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 30}, settings.Motion); err != nil {
				t.Fatal(err)
			}
			_ = c.LastLink().Close() // lose only the simulated Handy radio
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				state := engine.Snapshot()
				if policy == config.LinkedFailureStopAll && !state.Running {
					break
				}
				if policy == config.LinkedFailureContinueHealthy && state.Running && state.LinkedSession != nil && len(state.LinkedSession.RetiredDevices) > 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			state := engine.Snapshot()
			if policy == config.LinkedFailureStopAll && state.Running {
				t.Fatal("linked motion survived stop-all failure")
			}
			if policy == config.LinkedFailureContinueHealthy && (!state.Running || state.LinkedSession == nil || len(state.LinkedSession.RetiredDevices) == 0) {
				t.Fatal("healthy Coyote was stopped")
			}
			_, _ = engine.Stop(context.Background(), "test_cleanup")
		})
	}
}
