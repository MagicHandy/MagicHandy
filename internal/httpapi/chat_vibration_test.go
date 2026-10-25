package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func independentVibrationServer(t *testing.T) *Server {
	t.Helper()
	s := newVibratorTestServer(t, config.DeviceOutputHandy, config.DeviceVibrator)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Device.HSPDispatchOwner = config.DispatchOwnerIntiface
		settings.LLM.Connections = []config.ModelConnection{{ID: "vibration-review", Name: "Review", Provider: config.LLMProviderChatGPT, Model: "gpt-6.1-sol"}}
		settings.LLM.ConversationConnectionID = "vibration-review"
		settings.LLM.MotionGenerationMode = config.LLMMotionModeLayered
		return settings
	})
	connectVibrator(t, s)
	for _, r := range []struct{ path, body string }{
		{"/api/transport/intiface/connect", `{}`},
		{"/api/transport/intiface/select", `{"device_index":0,"actuator_index":0}`},
		{"/api/transport/intiface/vibrator/intensity", `{"intensity_percent":40}`},
	} {
		if code, _ := vibratorRequest(t, s, r.path, r.body, true); code != 200 {
			t.Fatalf("%s = %d", r.path, code)
		}
	}
	return s
}

func TestChatIndependentVibrationMixedLifecycle(t *testing.T) {
	s := independentVibrationServer(t)
	steady := motion.VibrationSpec{Mode: "steady", MinPercent: 50, MaxPercent: 50}
	result, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &steady})
	if err != nil || !result.Applied || !result.Engine.Target.VibrationOnly {
		t.Fatalf("vibration-only start: %+v %v", result, err)
	}
	engine := s.currentMotionEngine()
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	time.Sleep(2300 * time.Millisecond)
	snapshot := s.intifaceSnapshot()
	if !engine.Snapshot().Running || snapshot.Status.PlaybackState == "playing" || snapshot.Vibrator.LevelPercent != 20 {
		t.Fatalf("independent start affected Handy or failed: %+v", snapshot)
	}
	// An explicit subsequent linear start prepares a normal run and preserves vibration.
	flow := chat.DefaultLayeredScore(40)
	result, err = s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionStart, Layered: &flow})
	if err != nil || result.Engine.Target.VibrationOnly || result.Engine.Target.Vibration == nil {
		t.Fatalf("start linear: %+v %v", result, err)
	}
	before := engine.Snapshot()
	off := motion.VibrationSpec{Mode: "off"}
	result, err = s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &off})
	if err != nil || result.Engine.PlanID != before.PlanID {
		t.Fatalf("off retargeted Handy: %+v %v", result, err)
	}
	time.Sleep(150 * time.Millisecond)
	if v := s.intifaceSnapshot().Vibrator; v.LevelPercent != 0 || v.IntensityPercent != 40 {
		t.Fatalf("off changed arming or failed: %+v", v)
	}
	if !engine.Snapshot().Running {
		t.Fatal("vibration off stopped linear run")
	}
	_, err = s.emergencyStop(t.Context(), "test_emergency")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &steady}); err == nil {
		t.Fatal("model rearmed after Emergency Stop")
	}
}

func TestLocalVibrationControlRemainsUnavailable(t *testing.T) {
	s := independentVibrationServer(t)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.ConversationConnectionID = "local"
		return settings
	})
	spec := motion.VibrationSpec{Mode: "steady", MinPercent: 30, MaxPercent: 30}
	if _, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &spec}); err == nil {
		t.Fatal("local-only route gained independent output control")
	}
}

func TestVibrationAuthorityUsesResolvedModelRole(t *testing.T) {
	s := independentVibrationServer(t)
	spec := motion.VibrationSpec{Mode: "steady", MinPercent: 30, MaxPercent: 30}
	response := chat.AutopilotResponse{Motion: &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &spec}}
	for _, conversation := range []string{"local", "vibration-review"} {
		for _, planner := range []string{"local", "vibration-review"} {
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.ConversationConnectionID = conversation
				settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "connection", ConnectionID: planner}
				return settings
			})
			settings, _ := s.store.Snapshot()
			chatModel := settings.LLM.ConversationSettings()
			planModel, err := settings.LLM.PlanningSettings()
			if err != nil {
				t.Fatal(err)
			}
			if s.chatOutputContext(chatModel).Vibrator.Independent != (conversation != "local") || s.chatOutputContext(planModel).Vibrator.Independent != (planner != "local") {
				t.Fatal("capability used the wrong role")
			}
			if err := s.validateChatVibration(&spec); (err == nil) != (conversation != "local") {
				t.Fatalf("conversation=%s validation=%v", conversation, err)
			}
			decision, err := s.mapAutopilotResponse(response, modes.DecisionInput{})
			if (err == nil) != (planner != "local") {
				t.Fatalf("planner=%s validation=%v", planner, err)
			}
			if err == nil && (!decision.Hold || decision.Vibration == nil || *decision.Vibration != spec) {
				t.Fatal("planner lost independent hold")
			}
			_, speechErr := s.mapAutopilotResponseForModel(response, modes.DecisionInput{}, chatModel)
			if (speechErr == nil) != (conversation != "local") {
				t.Fatal("speech output validated against the planner role")
			}
		}
	}
}

func TestVibrationFollowAdmissionAndUnavailableOutput(t *testing.T) {
	if !motionCommandRoute("/api/motion/vibration/follow") {
		t.Fatal("Follow does not supersede deferred motion")
	}
	s := independentVibrationServer(t)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.Device = settings.Device.WithPlaybackDevices([]string{config.DeviceOutputHandy})
		return settings
	})
	if status, _ := vibratorRequest(t, s, "/api/motion/vibration/follow", `{}`, true); status != 409 {
		t.Fatalf("Follow without vibrator = %d", status)
	}
}

func TestStoppedEnvelopeIsNotReportedAsCurrent(t *testing.T) {
	s := independentVibrationServer(t)
	spec := motion.VibrationSpec{Mode: "wave", MinPercent: 10, MaxPercent: 50, PeriodSeconds: 5}
	if _, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &spec}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.currentMotionEngine().Stop(t.Context(), "stopped"); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if s.chatOutputContext(settings.LLM.ConversationSettings()).Vibrator.Current != nil {
		t.Fatal("stopped plan presented as current vibration")
	}
}

func TestVibrationLostArmingDoesNotDiscardValidLinearUpdate(t *testing.T) {
	s := independentVibrationServer(t)
	flow := chat.DefaultLayeredScore(40)
	if _, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionStart, Layered: &flow}); err != nil {
		t.Fatal(err)
	}
	if code, _ := vibratorRequest(t, s, "/api/transport/intiface/vibrator/intensity", `{"intensity_percent":0}`, true); code != 200 {
		t.Fatal(code)
	}
	flow.SpeedPercent = 50
	spec := motion.VibrationSpec{Mode: "steady", MinPercent: 50, MaxPercent: 50}
	result, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionUpdate, Layered: &flow, Vibration: &spec})
	if err == nil || !result.Applied || !result.Engine.Running || result.Engine.Target.SpeedPercent != 50 || s.intifaceSnapshot().Vibrator.IntensityPercent != 0 {
		t.Fatalf("partial output was not applied/reported: %+v %v", result, err)
	}
}
