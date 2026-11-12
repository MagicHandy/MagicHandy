package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func tempoRequest(t *testing.T, server *Server, body string) (int, string) {
	t.Helper()
	code, payload := autoblowRequest(t, server, http.MethodPost, "/api/motion/tempo", body, true)
	return code, string(payload)
}

func stopMotion(t *testing.T, server *Server) {
	t.Helper()
	stop := httptest.NewRecorder()
	server.Handler().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if stop.Code != http.StatusOK {
		t.Fatalf("Stop = %d %s", stop.Code, stop.Body.String())
	}
}

func TestTempoRoutineStartsATempoOnlyRunAndChangesInPlace(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	playOnly(t, server, config.DeviceVacuGlide)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)

	code, body := autoblowRequest(t, server, http.MethodGet, "/api/motion/tempo/routines", "", false)
	var listed struct {
		Routines []motion.TempoRoutine `json:"routines"`
	}
	if code != http.StatusOK || json.Unmarshal(body, &listed) != nil || len(listed.Routines) != len(motion.TempoRoutines()) {
		t.Fatalf("routines = %d %s", code, body)
	}
	if code, body := tempoRequest(t, server, `{"tempo":{"mode":"routine","routine":"bursts"}}`); code != http.StatusOK {
		t.Fatalf("start a routine = %d %s", code, body)
	}
	engine := server.currentMotionEngine()
	state := engine.Snapshot()
	if !state.Running || !state.Target.TempoOnly || state.Target.Tempo == nil || state.Target.Tempo.Routine != "bursts" || state.Target.Tempo.Seed == 0 {
		t.Fatalf("run = %+v", state.Target)
	}
	waitUntil(t, "the motor to play the routine", func() bool { return sim.VacuGlideSpeed() >= 5 })
	generation := state.Generation
	if code, body := tempoRequest(t, server, `{"tempo":{"mode":"steady","level_percent":100}}`); code != http.StatusOK {
		t.Fatalf("change the tempo = %d %s", code, body)
	}
	if current := server.currentMotionEngine(); current != engine || current.Snapshot().Generation <= generation {
		t.Fatal("a tempo change replaced the run instead of changing it in place")
	}
	waitUntil(t, "the motor to reach the top of the range", func() bool { return sim.VacuGlideSpeed() == 100 })
	stopMotion(t, server)
	if speed := sim.VacuGlideSpeed(); speed != 0 {
		t.Fatalf("motor speed after Stop = %.0f", speed)
	}
}

func TestTempoRangeBoundsWhatTheMotorPlays(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	playOnly(t, server, config.DeviceVacuGlide)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.VacuGlide.TempoMinPercent, settings.VacuGlide.TempoMaxPercent = 20, 60
		return settings
	})
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	if code, body := tempoRequest(t, server, `{"tempo":{"mode":"steady","level_percent":100}}`); code != http.StatusOK {
		t.Fatalf("start = %d %s", code, body)
	}
	waitUntil(t, "the motor to reach the range's top", func() bool { return sim.VacuGlideSpeed() == 60 })
	stopMotion(t, server)
}

func TestTempoNeedsTheVacuGlide(t *testing.T) {
	server, _ := newAutoblowTestServer(t)
	if code, body := tempoRequest(t, server, `{"tempo":{"mode":"steady","level_percent":40}}`); code != http.StatusConflict {
		t.Fatalf("tempo without a VacuGlide = %d %s", code, body)
	}
	playOnly(t, server, config.DeviceVacuGlide)
	if code, body := tempoRequest(t, server, `{"tempo":{"mode":"steady","level_percent":40}}`); code != http.StatusConflict {
		t.Fatalf("tempo while disconnected = %d %s", code, body)
	}
	if code, body := tempoRequest(t, server, `{"tempo":{"mode":"steady","level_percent":0}}`); code != http.StatusBadRequest {
		t.Fatalf("an invalid tempo = %d %s", code, body)
	}
	if code, _ := autoblowRequest(t, server, http.MethodPost, "/api/motion/tempo", `{"tempo":{"mode":"steady","level_percent":40}}`, false); code == http.StatusOK {
		t.Fatal("a client without control changed the tempo")
	}
}

func TestTempoLeadsOnlyWithoutAPositionDevice(t *testing.T) {
	device := config.DefaultSettings().Device
	if tempoLeads(device.WithPlaybackDevices([]string{config.DeviceOutputHandy, config.DeviceVacuGlide})) {
		t.Fatal("tempo led beside The Handy")
	}
	if tempoLeads(device.WithPlaybackDevices([]string{config.DeviceAutoblow, config.DeviceVacuGlide})) {
		t.Fatal("tempo led beside the Ultra")
	}
	if !tempoLeads(device.WithPlaybackDevices([]string{config.DeviceVacuGlide, config.DeviceVibrator})) {
		t.Fatal("tempo did not lead with the VacuGlide and a vibrator")
	}
}

func TestChatTempoStartsRunsAndStepsAirOnlyWithPermission(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	playOnly(t, server, config.DeviceVacuGlide)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	steps := 1
	command := &chat.MotionCommand{Action: chat.MotionActionNone, Tempo: &motion.TempoSpec{Mode: motion.TempoModeSteady, LevelPercent: 50}, AirSteps: &steps}
	result, err := server.dispatchChatMotion(context.Background(), command)
	if err == nil || !strings.Contains(err.Error(), "allow it") {
		t.Fatalf("air without permission = %+v, %v", result, err)
	}
	if !server.currentMotionEngine().Snapshot().Target.TempoOnly {
		t.Fatal("the chat tempo did not start a tempo-only run")
	}
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.VacuGlide.MotionAir = true
		return settings
	})
	server.autoblow.vacuglide.Configure(vacuglideConfig(func() config.Settings { s, _ := server.store.Snapshot(); return s }()))
	if _, err := server.dispatchChatMotion(context.Background(), &chat.MotionCommand{Action: chat.MotionActionNone, AirSteps: &steps}); err != nil {
		t.Fatal(err)
	}
	if _, _, net := sim.VacuGlideValves(); net != 1 {
		t.Fatalf("net valve steps = %d", net)
	}
	output := server.chatOutputContext(config.DefaultSettings().LLM)
	if output == nil || output.Tempo == nil || !output.Tempo.Leads || !output.Tempo.AirAllowed || output.Tempo.AirSteps != 1 || output.Tempo.Current == nil {
		t.Fatalf("chat tempo context = %+v", output)
	}
	stopMotion(t, server)
}

func TestCancelledTempoAirCannotReachTheValves(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	playOnly(t, server, config.DeviceVacuGlide)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.VacuGlide.MotionAir = true
		return settings
	})
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	if code, body := tempoRequest(t, server, `{"tempo":{"mode":"steady","level_percent":40}}`); code != http.StatusOK {
		t.Fatalf("start = %d %s", code, body)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	step := 1
	_, err := server.dispatchChatMotion(ctx, &chat.MotionCommand{Action: chat.MotionActionNone, AirSteps: &step})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("canceled air change returned %v", err)
	}
	if _, _, net := sim.VacuGlideValves(); net != 0 {
		t.Errorf("canceled air change applied %d valve steps", net)
	}
	stopMotion(t, server)
}

func TestDelayedTempoControlCannotRestartAfterStop(t *testing.T) {
	server, sim := newAutoblowTestServer(t)
	playOnly(t, server, config.DeviceVacuGlide)
	saveVacuGlideToken(t, server)
	connectVacuGlide(t, server)
	sequence := server.stopSequence.Load()
	stopMotion(t, server)
	code, _ := autoblowRequest(t, server, http.MethodPost, "/api/motion/tempo", `{"tempo":{"mode":"steady","level_percent":70}}`, true,
		stopSequenceHeader, strconv.FormatUint(sequence, 10))
	if code != http.StatusConflict {
		t.Fatalf("stale control returned %d", code)
	}
	if engine := server.currentMotionEngine(); engine != nil && engine.Snapshot().Running {
		t.Fatal("stale control started playback")
	}
	if speed := sim.VacuGlideSpeed(); speed != 0 {
		t.Fatalf("stale control sent speed %g", speed)
	}
}
