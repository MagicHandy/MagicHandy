package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/buttplugsim"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// The simulator advertises a stroker at index 0 and a two-motor vibrator at
// index 1.
const simulatedVibratorIndex = `{"device_index":1}`

func newVibratorTestServer(t *testing.T, playback ...string) *Server {
	t.Helper()
	sim, err := buttplugsim.Start()
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServerWithRuntime(t, Runtime{IntifaceSimulatorAddress: sim.Address()})
	t.Cleanup(func() {
		server.Close()
		_ = sim.Close()
	})
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device, _ = settings.Device.ChangeConnection(config.DeviceVibrator, "add")
		settings.Device = settings.Device.WithPlaybackDevices(playback)
		return settings
	})
	return server
}

func vibratorRequest(t *testing.T, server *Server, path, body string, controller bool, header ...string) (int, intifaceSnapshot) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for index := 0; index+1 < len(header); index += 2 {
		request.Header.Set(header[index], header[index+1])
	}
	if controller {
		request = withController(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	var snapshot intifaceSnapshot
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return recorder.Code, snapshot
}

func connectVibrator(t *testing.T, server *Server) {
	t.Helper()
	if code, snapshot := vibratorRequest(t, server, "/api/transport/intiface/connect", `{"device":"vibrator"}`, true); code != http.StatusOK || !snapshot.Simulated {
		t.Fatalf("connect = %d %+v", code, snapshot)
	}
	code, snapshot := vibratorRequest(t, server, "/api/transport/intiface/vibrator/select", simulatedVibratorIndex, true)
	if code != http.StatusOK || !snapshot.Vibrator.Selected || len(snapshot.Vibrator.Motors) != 2 || snapshot.Vibrator.Suspended {
		t.Fatalf("select = %d %+v", code, snapshot.Vibrator)
	}
}

func TestVibratorConnectNeedsASavedVibrator(t *testing.T) {
	server := newVibratorTestServer(t, config.DeviceOutputHandy)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device, _ = settings.Device.ChangeConnection(config.DeviceVibrator, "remove")
		return settings
	})
	if code, _ := vibratorRequest(t, server, "/api/transport/intiface/connect", `{"device":"vibrator"}`, true); code != http.StatusConflict {
		t.Fatalf("connect without a saved vibrator = %d", code)
	}
}

func TestVibratorOnlyPlaybackUsesTheVibratorAlone(t *testing.T) {
	server := newVibratorTestServer(t, config.DeviceVibrator)
	connectVibrator(t, server)
	selected, err := server.newSelectedMotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	if _, linked := selected.(*transport.FanOut); linked {
		t.Fatal("a single vibrator must not be wrapped in a linked session")
	}
	if timing := selected.(transport.MotionTimingCapabilitiesProvider).MotionTimingCapabilities(); timing.MinimumPointInterval != 0 || timing.MinimumBufferedLead == 0 {
		t.Fatalf("vibrator timing = %+v; it must not coarsen sampling", timing)
	}
	settings, _ := server.store.Snapshot()
	if !strings.HasPrefix(motionOwnerKey(settings.Device), config.DeviceVibrator) {
		t.Fatalf("owner key = %q", motionOwnerKey(settings.Device))
	}
}

func TestHandyAndVibratorShareOneIntifaceSession(t *testing.T) {
	server := newVibratorTestServer(t, config.DeviceOutputHandy, config.DeviceVibrator)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerIntiface
		return settings
	})
	connectVibrator(t, server)
	if code, snapshot := vibratorRequest(t, server, "/api/transport/intiface/connect", `{}`, true); code != http.StatusOK || len(snapshot.Leases) != 2 {
		t.Fatalf("the Handy must join the same session: %d %+v", code, snapshot)
	}
	if code, _ := vibratorRequest(t, server, "/api/transport/intiface/select", `{"device_index":0,"actuator_index":0}`, true); code != http.StatusOK {
		t.Fatalf("stroker select = %d", code)
	}
	selected, err := server.newSelectedMotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	if _, linked := selected.(*transport.FanOut); !linked {
		t.Fatalf("Handy + vibrator transport = %T, want a linked session", selected)
	}
	// The Handy leaving keeps the session for the vibrator.
	if code, snapshot := vibratorRequest(t, server, "/api/transport/intiface/disconnect", `{}`, true); code != http.StatusOK || !snapshot.Status.Connected || len(snapshot.Leases) != 1 || !snapshot.Vibrator.Selected {
		t.Fatalf("after the Handy left: %d %+v", code, snapshot)
	}
	if code, snapshot := vibratorRequest(t, server, "/api/transport/intiface/disconnect", `{"device":"vibrator"}`, true); code != http.StatusOK || snapshot.Status.Connected {
		t.Fatalf("the last lease must close the session: %d %+v", code, snapshot)
	}
}

func TestVibratorResumeNeedsTheHostAndTheCurrentStopSequence(t *testing.T) {
	server := newVibratorTestServer(t, config.DeviceVibrator)
	connectVibrator(t, server)
	if _, err := server.emergencyStop(t.Context(), "resume_test"); err != nil {
		t.Fatal(err)
	}
	current := strconv.FormatUint(server.stopSequence.Load(), 10)
	if code, _ := vibratorRequest(t, server, "/api/transport/intiface/vibrator/resume", `{}`, false, stopSequenceHeader, current); code == http.StatusOK {
		t.Fatal("a non-controller resumed the vibrator")
	}
	if code, _ := vibratorRequest(t, server, "/api/transport/intiface/vibrator/resume", `{}`, true); code != http.StatusBadRequest {
		t.Fatalf("resume without a Stop sequence = %d", code)
	}
	stale := strconv.FormatUint(server.stopSequence.Load()+1, 10)
	if code, _ := vibratorRequest(t, server, "/api/transport/intiface/vibrator/resume", `{}`, true, stopSequenceHeader, stale); code != http.StatusConflict {
		t.Fatalf("a resume from another Stop = %d", code)
	}
	if !server.intifaceSnapshot().Vibrator.Suspended {
		t.Fatal("a refused resume lifted the suspension")
	}
	code, snapshot := vibratorRequest(t, server, "/api/transport/intiface/vibrator/resume", `{}`, true, stopSequenceHeader, current)
	if code != http.StatusOK || snapshot.Vibrator.Suspended || snapshot.Vibrator.CeilingPercent != config.VibrationDefaultCeilingPercent {
		t.Fatalf("resume = %d %+v", code, snapshot.Vibrator)
	}
}

func TestVibratorTestNeedsTheStopSequence(t *testing.T) {
	server := newVibratorTestServer(t, config.DeviceVibrator)
	connectVibrator(t, server)
	if code, _ := vibratorRequest(t, server, "/api/transport/intiface/vibrator/test", `{}`, true); code != http.StatusBadRequest {
		t.Fatalf("test without a Stop sequence = %d", code)
	}
	current := strconv.FormatUint(server.stopSequence.Load(), 10)
	if code, snapshot := vibratorRequest(t, server, "/api/transport/intiface/vibrator/test", `{}`, true, stopSequenceHeader, current); code != http.StatusOK || !snapshot.Vibrator.Testing {
		t.Fatalf("test = %d %+v", code, snapshot.Vibrator)
	}
}

func TestGlobalStopSuspendsTheVibrator(t *testing.T) {
	server := newVibratorTestServer(t, config.DeviceVibrator)
	connectVibrator(t, server)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("Stop = %d %s", recorder.Code, recorder.Body.String())
	}
	if status := server.intifaceSnapshot().Vibrator; !status.Suspended || status.SilentReason != "stopped" || status.CeilingPercent != config.VibrationDefaultCeilingPercent {
		t.Fatalf("after Stop: %+v", status)
	}
}

func TestVibrationSettingsApplyLive(t *testing.T) {
	server := newVibratorTestServer(t, config.DeviceVibrator)
	connectVibrator(t, server)
	previous, _ := server.store.Snapshot()
	next := previous
	next.Vibration.CeilingPercent = 25
	if err := server.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatal(err)
	}
	if status := server.intifaceSnapshot().Vibrator; status.CeilingPercent != 25 {
		t.Fatalf("a lowered Max must apply live: %+v", status)
	}
	manual := next
	manual.Vibration.LevelControl = config.VibrationLevelManual
	if err := server.applySettingsRuntimeTransition(context.Background(), next, manual); err != nil {
		t.Fatal(err)
	}
	if status := server.intifaceSnapshot().Vibrator; status.ModelControl {
		t.Fatalf("model control stayed on: %+v", status)
	}
	// Removing the vibrator from the inventory ends its lease.
	removed := next
	removed.Device, _ = removed.Device.ChangeConnection(config.DeviceVibrator, "remove")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.applySettingsRuntimeTransition(ctx, next, removed); err != nil {
		t.Fatal(err)
	}
	if server.intifaceSnapshot().Status.Connected {
		t.Fatal("the session outlived its last device")
	}
}
