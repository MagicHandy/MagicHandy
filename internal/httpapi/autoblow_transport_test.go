package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
	"github.com/mapledaemon/MagicHandy/internal/autoblowsim"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const vacuglideTestDeviceID = "vacuglide-test-token"

func newAutoblowTestServer(t *testing.T) (*Server, *autoblowsim.Server) {
	t.Helper()
	sim, err := autoblowsim.StartWith(autoblowsim.Options{Heartbeat: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sim.Close() })
	handy := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport: handy, MotionTransport: handy,
		AutoblowClient:    autoblow.Options{LatencyURL: sim.URL(), ClusterURL: sim.ClusterURL},
		AutoblowSimulated: true,
	})
	return server, sim
}

func autoblowRequest(t *testing.T, server *Server, method, path, body string, controller bool, headers ...string) (int, []byte) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for index := 0; index+1 < len(headers); index += 2 {
		request.Header.Set(headers[index], headers[index+1])
	}
	if controller {
		request = withController(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.Bytes()
}

func addVacuGlide(t *testing.T, server *Server, plays bool) {
	t.Helper()
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.ConfiguredDevices = append(settings.Device.ConfiguredDevices, config.DeviceVacuGlide)
		playback := settings.Device.PlaybackSet()
		if plays {
			playback = append(playback, config.DeviceVacuGlide)
		}
		settings.Device = settings.Device.WithPlaybackDevices(playback)
		return settings
	})
}

func saveVacuGlideToken(t *testing.T, server *Server) {
	t.Helper()
	code, body := autoblowRequest(t, server, http.MethodPut, "/api/settings/device/autoblow-token", `{"device":"vacuglide","token":"`+vacuglideTestDeviceID+`"}`, true)
	if code != http.StatusOK {
		t.Fatalf("save token = %d %s", code, body)
	}
	if strings.Contains(string(body), vacuglideTestDeviceID) {
		t.Fatal("the token save echoed the token")
	}
}

func connectVacuGlide(t *testing.T, server *Server) autoblowSnapshot {
	t.Helper()
	code, body := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/connect", `{"device":"vacuglide"}`, true)
	if code != http.StatusOK {
		t.Fatalf("connect = %d %s", code, body)
	}
	var snapshot autoblowSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.VacuGlide.Connected || !snapshot.VacuGlide.Simulated {
		t.Fatalf("snapshot = %+v", snapshot.VacuGlide)
	}
	return snapshot
}

func waitUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAutoblowTokenSaveIsCheckedAndNeverEchoed(t *testing.T) {
	server, _ := newAutoblowTestServer(t)
	saveVacuGlideToken(t, server)
	settings, _ := server.store.Snapshot()
	if settings.VacuGlide.DeviceToken != vacuglideTestDeviceID || !settings.Public().VacuGlide.TokenSet {
		t.Fatal("the token was not saved")
	}
	for body, want := range map[string]int{
		`{"device":"vacuglide","token":"has space"}`: http.StatusBadRequest,
		`{"device":"vacupump","token":"abc"}`:        http.StatusBadRequest,
		`{"device":"vacuglide","token":"  "}`:        http.StatusBadRequest,
	} {
		if code, _ := autoblowRequest(t, server, http.MethodPut, "/api/settings/device/autoblow-token", body, true); code != want {
			t.Errorf("%s = %d, want %d", body, code, want)
		}
	}
	if code, _ := autoblowRequest(t, server, http.MethodPut, "/api/settings/device/autoblow-token", `{"device":"vacuglide","token":"abc"}`, false); code == http.StatusOK {
		t.Fatal("a client that is not the controller saved a token")
	}
}

func TestAutoblowConnectNeedsTheDeviceAndItsToken(t *testing.T) {
	server, _ := newAutoblowTestServer(t)
	if code, _ := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/connect", `{"device":"vacuglide"}`, true); code != http.StatusConflict {
		t.Fatalf("connect before adding = %d, want 409", code)
	}
	addVacuGlide(t, server, false)
	if code, _ := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/connect", `{"device":"vacuglide"}`, true); code != http.StatusConflict {
		t.Fatalf("connect without a token = %d, want 409", code)
	}
	if code, _ := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/connect", `{"device":"toaster"}`, true); code != http.StatusBadRequest {
		t.Fatalf("connect an unknown device = %d, want 400", code)
	}
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	code, body := autoblowRequest(t, server, http.MethodGet, "/api/state", ``, false)
	if code != http.StatusOK || !strings.Contains(string(body), `"autoblow_transport"`) {
		t.Fatalf("state = %d, want the Autoblow snapshot", code)
	}
}

func TestVacuGlideStrokeStepsAndPermissions(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	addVacuGlide(t, server, false)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	// Both valves change an unmeasured physical state and need a controller.
	if code, _ := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/stroke", `{"direction":"shorter"}`, false); code == http.StatusOK {
		t.Fatal("a non-controller adjusted a valve")
	}
	if code, _ := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/stroke", `{"direction":"longer"}`, true); code != http.StatusBadRequest {
		t.Fatalf("longer without the Stop sequence = %d, want 400", code)
	}
	sequence := strconv.FormatUint(server.stopSequence.Load(), 10)
	if code, body := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/stroke", `{"direction":"shorter"}`, true, stopSequenceHeader, sequence); code != http.StatusOK {
		t.Fatalf("shorter = %d %s", code, body)
	}
	if code, body := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/stroke", `{"direction":"longer"}`, true, stopSequenceHeader, sequence); code != http.StatusOK {
		t.Fatalf("longer = %d %s", code, body)
	}
	if _, _, net := sim.VacuGlideValves(); net != 0 {
		t.Fatalf("net valve steps = %d, want shorter then longer to cancel", net)
	}
	if code, _ := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/stroke", `{"direction":"sideways"}`, true); code != http.StatusBadRequest {
		t.Fatalf("an unknown direction = %d, want 400", code)
	}
}

func TestEmergencyStopReachesAConnectedVacuGlide(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	addVacuGlide(t, server, false)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	sequence := strconv.FormatUint(server.stopSequence.Load(), 10)
	if code, body := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/test", `{"device":"vacuglide"}`, true, stopSequenceHeader, sequence); code != http.StatusOK {
		t.Fatalf("test = %d %s", code, body)
	}
	if sim.VacuGlideSpeed() == 0 {
		t.Fatal("the test did not run the motor")
	}
	stop := httptest.NewRecorder()
	server.Handler().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if stop.Code != http.StatusOK {
		t.Fatalf("Stop = %d %s", stop.Code, stop.Body.String())
	}
	if speed := sim.VacuGlideSpeed(); speed != 0 {
		t.Fatalf("motor speed after Stop = %.0f", speed)
	}
}

func TestDeviceButtonsStopAndNudgeTheSpeedLimit(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	addVacuGlide(t, server, false)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	waitUntil(t, "the button stream", func() bool { return server.autoblow.vacuglide.Status().ButtonEvents })
	before, _ := server.store.Snapshot()
	sim.Press("vacuglide", autoblow.ButtonVacuGlideSpeedMinus)
	waitUntil(t, "the speed limit to drop", func() bool {
		settings, _ := server.store.Snapshot()
		return settings.Motion.SpeedMaxPercent == max(before.Motion.SpeedMinPercent, before.Motion.SpeedMaxPercent-autoblowSpeedStep)
	})
	sequence := server.stopSequence.Load()
	sim.Press("vacuglide", autoblow.ButtonVacuGlideMode)
	waitUntil(t, "the mode button to Stop", func() bool { return server.stopSequence.Load() > sequence })
}

func TestChangingTheTokenDisconnectsTheDevice(t *testing.T) {
	server, _ := newAutoblowTestServer(t)
	addVacuGlide(t, server, false)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	if code, _ := autoblowRequest(t, server, http.MethodPut, "/api/settings/device/autoblow-token", `{"device":"vacuglide","token":"another-token"}`, true); code != http.StatusOK {
		t.Fatalf("replace token = %d", code)
	}
	if server.autoblow.vacuglide.Connected() {
		t.Fatal("a new token must disconnect the old connection")
	}
}

func TestAutoblowSnapshotHidesHostDetailsFromOtherClients(t *testing.T) {
	server, _ := newAutoblowTestServer(t)
	addVacuGlide(t, server, false)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	request := httptest.NewRequest(http.MethodGet, "/api/transport/autoblow/status", nil)
	if snapshot := server.clientAutoblowSnapshot(request); snapshot.VacuGlide.Cluster == "" {
		t.Fatal("the host should see the cluster")
	}
	if snapshot := server.autoblowSnapshot(); snapshot.Ultra.Connected || !snapshot.VacuGlide.TokenSet {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// playOnly makes device the only playback participant.
func playOnly(t *testing.T, server *Server, device string) {
	t.Helper()
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.ConfiguredDevices = append(settings.Device.ConfiguredDevices, device)
		settings.Device = settings.Device.WithPlaybackDevices([]string{device})
		return settings
	})
}

func startMotionAt(t *testing.T, server *Server, speed int) {
	t.Helper()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", strings.NewReader(`{"speed_percent":`+strconv.Itoa(speed)+`}`)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("start = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestTheEnginePlaysThroughTheVacuGlideAlone(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	playOnly(t, server, config.DeviceVacuGlide)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	startMotionAt(t, server, 60)
	waitUntil(t, "the motor to follow the engine", func() bool { return sim.VacuGlideSpeed() >= 5 })
	stop := httptest.NewRecorder()
	server.Handler().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if stop.Code != http.StatusOK {
		t.Fatalf("Stop = %d %s", stop.Code, stop.Body.String())
	}
	if speed := sim.VacuGlideSpeed(); speed != 0 {
		t.Fatalf("motor speed after Stop = %.0f", speed)
	}
}

func TestTheEnginePlaysThroughTheUltraAlone(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	playOnly(t, server, config.DeviceAutoblow)
	if code, body := autoblowRequest(t, server, http.MethodPut, "/api/settings/device/autoblow-token", `{"device":"autoblow","token":"ultra-test-token"}`, true); code != http.StatusOK {
		t.Fatalf("save token = %d %s", code, body)
	}
	if code, body := autoblowRequest(t, server, http.MethodPost, "/api/transport/autoblow/connect", `{"device":"autoblow"}`, true); code != http.StatusOK {
		t.Fatalf("connect = %d %s", code, body)
	}
	startMotionAt(t, server, 50)
	waitUntil(t, "go-to commands", func() bool {
		count := 0
		for _, request := range sim.Requests() {
			if request.Path == "goto" {
				count++
			}
		}
		return count >= 3
	})
	stop := httptest.NewRecorder()
	server.Handler().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if stop.Code != http.StatusOK {
		t.Fatalf("Stop = %d %s", stop.Code, stop.Body.String())
	}
}

func TestAutoblowButtonsMapToActions(t *testing.T) {
	for _, tc := range []struct{ device, button, want string }{
		{transport.AutoblowUltraDevice, autoblow.ButtonUltraPause, "stop"},
		{transport.AutoblowUltraDevice, autoblow.ButtonUltraMode, ""},
		{transport.AutoblowUltraDevice, autoblow.ButtonUltraSpeedUp, "faster"},
		{transport.VacuGlideDevice, autoblow.ButtonVacuGlideMode, "stop"},
		{transport.VacuGlideDevice, autoblow.ButtonVacuGlideSpeedMinus, "slower"},
		{transport.VacuGlideDevice, "unknown-button-pressed", ""},
	} {
		if got := autoblowButtonAction(tc.device, tc.button); got != tc.want {
			t.Errorf("%s %s = %q, want %q", tc.device, tc.button, got, tc.want)
		}
	}
}
