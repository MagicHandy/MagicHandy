package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/estim"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func newCoyoteTestServer(t *testing.T, output string, acknowledged bool) (*Server, *estim.SimulatedConnector) {
	t.Helper()
	connector := estim.NewSimulatedConnector()
	handy := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{CoyoteConnector: connector, Transport: handy, MotionTransport: handy})
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device = settings.Device.WithPlaybackOutput(output)
		settings.Estim.SafetyAcknowledged = acknowledged
		return settings
	})
	return server, connector
}

func coyoteRequest(t *testing.T, server *Server, method, path, body string, controller bool) (int, coyoteSnapshot) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if controller {
		request = withController(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	var snapshot coyoteSnapshot
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return recorder.Code, snapshot
}

func connectCoyote(t *testing.T, server *Server) coyoteSnapshot {
	t.Helper()
	code, snapshot := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/connect", `{}`, true)
	if code != http.StatusOK || !snapshot.Status.Connected {
		t.Fatalf("connect = %d %+v", code, snapshot)
	}
	waitCoyoteStatus(t, server, func(status transport.CoyoteStatus) bool { return status.StrengthKnown && !status.StrengthPending })
	return snapshot
}

func waitCoyoteStatus(t *testing.T, server *Server, done func(transport.CoyoteStatus) bool) transport.CoyoteStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		status := server.coyote.owner.Status()
		if done(status) {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("Coyote status never settled: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCoyoteConnectRequiresTheOutputAndTheSafetyNotes(t *testing.T) {
	server, _ := newCoyoteTestServer(t, config.DeviceOutputHandy, true)
	if code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/connect", `{}`, true); code != http.StatusConflict {
		t.Fatalf("connect with the Handy output = %d, want 409", code)
	}
	server, _ = newCoyoteTestServer(t, config.DeviceOutputCoyote, false)
	if code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/connect", `{}`, true); code != http.StatusConflict {
		t.Fatalf("connect before the safety notes = %d, want 409", code)
	}
}

func TestCoyoteHTTPConnectStrengthAndGlobalStop(t *testing.T) {
	server, connector := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	snapshot := connectCoyote(t, server)
	if !snapshot.Available || !snapshot.Status.Simulated || snapshot.Output != config.DeviceOutputCoyote {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if settings, _ := server.store.Snapshot(); settings.Estim.RememberedAddress != "" {
		t.Fatal("the simulator must never be remembered as a device")
	}
	link := connector.LastLink()
	if state := link.Snapshot(); state.CeilingA != config.EstimDefaultMaxStrength || state.Limits != 1 {
		t.Fatalf("connect must write the saved ceilings: %+v", state)
	}
	// Wait for the accepted zero's B1 window before raising the commanded baseline.
	waitCoyoteStatus(t, server, func(status transport.CoyoteStatus) bool { return status.StrengthKnown && !status.StrengthPending })

	code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/strength", `{"channel":"a","delta":2}`, true)
	if code != http.StatusOK {
		t.Fatalf("raise = %d", code)
	}
	waitCoyoteStatus(t, server, func(status transport.CoyoteStatus) bool { return status.StrengthA == 2 })
	if code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/strength", `{"channel":"c","delta":2}`, true); code != http.StatusBadRequest {
		t.Fatalf("unknown channel = %d", code)
	}
	if code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/strength", `{"channel":"a","delta":11}`, true); code != http.StatusBadRequest {
		t.Fatalf("oversized step = %d", code)
	}

	stop := httptest.NewRecorder()
	server.Handler().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if stop.Code != http.StatusOK && stop.Code != http.StatusBadGateway {
		t.Fatalf("Stop = %d %s", stop.Code, stop.Body.String())
	}
	if state := link.Snapshot(); state.StrengthA != 0 || state.StrengthB != 0 {
		t.Fatalf("global Stop must zero the Coyote: %+v", state)
	}
}

func TestCoyoteLoweringNeedsNoControllerButRaisingDoes(t *testing.T) {
	server, _ := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	connectCoyote(t, server)
	waitCoyoteStatus(t, server, func(status transport.CoyoteStatus) bool { return status.StrengthKnown && !status.StrengthPending })
	// A different browser becomes read-only while the controller holds control.
	reader := httptest.NewRequest(http.MethodPost, "/api/transport/coyote/strength", strings.NewReader(`{"channel":"b","delta":1}`))
	reader = withControllerID(reader, "another-browser")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, reader)
	if recorder.Code == http.StatusOK {
		t.Fatalf("a read-only browser raised strength: %d", recorder.Code)
	}
	lower := httptest.NewRequest(http.MethodPost, "/api/transport/coyote/strength", strings.NewReader(`{"channel":"b","delta":-1}`))
	lower = withControllerID(lower, "another-browser")
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, lower)
	if recorder.Code != http.StatusOK {
		t.Fatalf("lowering from a read-only browser = %d %s", recorder.Code, recorder.Body.String())
	}
	if code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/zero", ``, false); code != http.StatusOK {
		t.Fatalf("zero without controller = %d", code)
	}
}

func TestCoyoteOutputSelectsTheEngineTransport(t *testing.T) {
	server, _ := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	if _, err := server.newSelectedMotionTransport(); err == nil || !strings.Contains(err.Error(), "connect the Coyote first") {
		t.Fatalf("motion before connecting = %v", err)
	}
	connectCoyote(t, server)
	selected, err := server.newSelectedMotionTransport()
	if err != nil || selected == transport.Transport(server.coyote.owner) {
		t.Fatalf("Coyote output transport = %T, %v", selected, err)
	}
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device = settings.Device.WithPlaybackOutput(config.DeviceOutputHandyCoyote)
		return settings
	})
	selected, err = server.newSelectedMotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := selected.(*transport.FanOut); !ok {
		t.Fatalf("Handy + Coyote transport = %T, want the fan-out", selected)
	}
	if motionOwnerKey(config.DeviceSettings{Output: config.DeviceOutputCoyote, HSPDispatchOwner: "x"}) == motionOwnerKey(config.DeviceSettings{Output: config.DeviceOutputHandyCoyote, HSPDispatchOwner: "x"}) {
		t.Fatal("changing the output must replace the engine")
	}
}

func TestCoyoteExclusionKeepsTheConnectionSilent(t *testing.T) {
	server, connector := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	connectCoyote(t, server)
	previous, _ := server.store.Snapshot()
	next := previous
	next.Device = next.Device.WithPlaybackDevices([]string{config.DeviceOutputHandy})
	if err := server.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatal(err)
	}
	if !server.coyote.owner.Status().Connected || connector.LastLink().Snapshot().Closed {
		t.Fatal("excluding the Coyote must keep its connection")
	}
}

func TestCoyoteCeilingChangeReachesTheConnectedDevice(t *testing.T) {
	server, connector := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	connectCoyote(t, server)
	previous, _ := server.store.Snapshot()
	next := previous
	next.Estim.MaxStrengthB = 12
	if err := server.applySettingsRuntimeTransition(context.Background(), previous, next); err != nil {
		t.Fatal(err)
	}
	if state := connector.LastLink().Snapshot(); state.Limits != 2 || state.CeilingB != 12 {
		t.Fatalf("live ceiling change = %+v", state)
	}
}

func TestImmediateDeviceOutputAndSafetyEndpoints(t *testing.T) {
	server, connector := newCoyoteTestServer(t, config.DeviceOutputCoyote, false)
	put := func(path, body string) int {
		request := withController(httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		return recorder.Code
	}
	if code := put("/api/settings/estim/safety", `{"acknowledged":true}`); code != http.StatusOK {
		t.Fatalf("accept safety notes = %d", code)
	}
	if settings, _ := server.store.Snapshot(); !settings.Estim.SafetyAcknowledged {
		t.Fatal("the acknowledgment was not saved")
	}
	connectCoyote(t, server)
	if code := put("/api/settings/device/output", `{"output":"toaster"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown output = %d", code)
	}
	if code := put("/api/settings/device/output", `{"output":"handy"}`); code != http.StatusOK {
		t.Fatalf("switch to the Handy = %d", code)
	}
	if settings, _ := server.store.Snapshot(); settings.Device.Output != config.DeviceOutputHandy {
		t.Fatalf("output after the switch = %q", settings.Device.Output)
	}
	if connector.LastLink().Snapshot().Closed || !server.coyote.owner.Status().Connected {
		t.Fatal("switching away from the Coyote must zero it and keep its connection")
	}
}

func TestStateReportsTheCoyote(t *testing.T) {
	server, _ := newCoyoteTestServer(t, config.DeviceOutputCoyote, true)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	var state struct {
		Coyote   coyoteSnapshot `json:"coyote_transport"`
		Settings struct {
			Device struct {
				Output string `json:"output"`
			} `json:"device"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Coyote.Output != config.DeviceOutputCoyote || state.Settings.Device.Output != config.DeviceOutputCoyote || !state.Coyote.Available {
		t.Fatalf("state = %+v", state)
	}
}

// Following speed spans the saved Handy speed band on the vibrator's scale, so
// a band change reconfigures the Coyote (ADR 0063).
func TestCoyoteConfigFollowsTheSavedSpeedBand(t *testing.T) {
	settings := config.DefaultSettings()
	low, high := motion.FollowSpeedBand(settings.EffectiveMotion())
	profile := coyoteConfig(settings).Profile
	if profile.SpeedLow != low || profile.SpeedHigh != high || low <= 0 || high <= low {
		t.Fatalf("profile band %.3f..%.3f, want %.3f..%.3f", profile.SpeedLow, profile.SpeedHigh, low, high)
	}
	wider := settings
	wider.Motion.SpeedMinPercent, wider.Motion.SpeedMaxPercent = 1, 100
	if coyoteConfig(wider) == coyoteConfig(settings) {
		t.Fatal("a speed band change must reconfigure the Coyote")
	}
	if coyoteConfig(wider).Limits != coyoteConfig(settings).Limits {
		t.Fatal("a speed band change must not touch the device ceilings")
	}
}

// A Coyote reaches the model as facts, never a control: the context describes
// it without a vibrator, and reports silence until the user raises a channel
// (ADR 0063). Strength numbers stay out of the model's context.
func TestCoyoteFactsReachTheChatModel(t *testing.T) {
	server, _ := newCoyoteTestServer(t, config.DeviceOutputHandyCoyote, true)
	settings, _ := server.store.Snapshot()
	outputs := server.chatOutputContext(settings.LLM.ConversationSettings())
	if outputs == nil || !outputs.Coyote || outputs.Vibrator != nil || outputs.Estim == nil || outputs.Strokers != 1 ||
		outputs.Estim.Connected || outputs.Estim.Silent || outputs.Estim.Follows != settings.Estim.Follow {
		t.Fatalf("disconnected Coyote facts = %+v", outputs)
	}
	connectCoyote(t, server)
	// A commanded zero is not a measurement, so silence is not claimed yet.
	if estim := server.chatOutputContext(settings.LLM.ConversationSettings()).Estim; !estim.Connected || estim.Silent {
		t.Fatalf("a new connection: %+v", estim)
	}
	if code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/strength", `{"channel":"b","delta":1}`, true); code != http.StatusOK {
		t.Fatalf("raise = %d", code)
	}
	waitCoyoteStatus(t, server, func(status transport.CoyoteStatus) bool { return status.StrengthB == 1 && !status.StrengthPending })
	outputs = server.chatOutputContext(settings.LLM.ConversationSettings())
	if outputs.Estim.Silent {
		t.Fatalf("a raised channel is felt: %+v", outputs.Estim)
	}
	if code, _ := coyoteRequest(t, server, http.MethodPost, "/api/transport/coyote/strength", `{"channel":"b","delta":-1}`, true); code != http.StatusOK {
		t.Fatalf("lower = %d", code)
	}
	waitCoyoteStatus(t, server, func(status transport.CoyoteStatus) bool { return status.StrengthB == 0 && !status.StrengthPending })
	if estim := server.chatOutputContext(settings.LLM.ConversationSettings()).Estim; !estim.Silent {
		t.Fatalf("the device reported zero on both channels: %+v", estim)
	}
	if encoded, _ := json.Marshal(outputs); strings.Contains(string(encoded), "strength_b") || strings.Contains(string(encoded), "ceiling") {
		t.Fatalf("strength numbers leaked into the model context: %s", encoded)
	}
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device = settings.Device.WithPlaybackOutput(config.DeviceOutputHandy)
		return settings
	})
	if outputs := server.chatOutputContext(settings.LLM.ConversationSettings()); outputs != nil {
		t.Fatalf("Handy-only playback has no output context: %+v", outputs)
	}
}
