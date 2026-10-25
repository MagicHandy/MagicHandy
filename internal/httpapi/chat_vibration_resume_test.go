package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const vibrationResumeResponse = `{"action":"none","edits":{},"vibration":{"mode":"wave","min_percent":20,"max_percent":60,"period_seconds":4},"reply":"Resuming a gentle wave."}`

func stopVibrationForResume(t *testing.T, s *Server) {
	t.Helper()
	if _, err := s.emergencyStop(t.Context(), "manual_resume_test"); err != nil {
		t.Fatal(err)
	}
	if v := s.intifaceSnapshot().Vibrator; v.IntensityPercent != 0 || v.ResumeIntensityPercent != 40 {
		t.Fatalf("Stop did not silence and remember the human level: %+v", v)
	}
}

func TestManualChatResumesVibrationAfterStop(t *testing.T) {
	for _, level := range []string{"external", config.ModelCapabilityAdvanced, config.ModelCapabilityIntermediate, config.ModelCapabilitySimple} {
		t.Run(level, func(t *testing.T) {
			s := independentVibrationServer(t)
			response := vibrationResumeResponse
			if level != "external" {
				saveSettings(t, s.store, func(settings config.Settings) config.Settings {
					settings.LLM.ConversationConnectionID = "local"
					settings.LLM.LocalCapabilityLevel = level
					settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 65536, 1024
					return settings
				})
				if level == config.ModelCapabilityIntermediate {
					response = lifecycleStartFixture(config.LLMMotionModeLayered)
				}
				if level == config.ModelCapabilitySimple {
					response = `{"action":"start","edits":{"controls":{"speed_percent":30}},"reply":"Playing the new rhythm."}`
				}
			}
			provider := &scriptedLLMProvider{responses: []string{`{"action":"none","edits":{},"reply":"The output is stopped."}`, response, response}}
			s.llm.provider = provider
			stopVibrationForResume(t, s)
			body := postChatStream(t, s, `{"message":"What is the current playback state?"}`)
			if strings.Contains(body, "event: error") || s.intifaceSnapshot().Vibrator.IntensityPercent != 0 {
				t.Fatalf("ordinary question changed intensity: %s", body)
			}
			for range 2 {
				body = postChatStream(t, s, `{"message":"Resume with a gently varying rhythm."}`)
				engine := s.currentMotionEngine()
				if strings.Contains(body, "event: error") || engine == nil || !engine.Snapshot().Running || s.intifaceSnapshot().Vibrator.IntensityPercent != 40 {
					t.Fatalf("fresh request failed to resume: %s", body)
				}
				stopVibrationForResume(t, s)
			}
			if provider.callCount() != 3 {
				t.Fatalf("unexpected repair/fallback calls: %d", provider.callCount())
			}
			prompt := ""
			for _, message := range provider.requests[1].Messages {
				prompt += message.Content
			}
			if level != config.ModelCapabilitySimple && !strings.Contains(prompt, `"manual_request_can_resume_previous_intensity":true`) {
				t.Fatal("manual permission did not reach model")
			}
		})
	}
}

// An admitted human request does not survive a later Stop or a changed output.
func TestManualVibrationResumeRejectsChangedAuthority(t *testing.T) {
	for _, change := range []string{"stop", "zero", "disconnect", "off", "cancel"} {
		t.Run(change, func(t *testing.T) {
			s := independentVibrationServer(t)
			stopVibrationForResume(t, s)
			settings, _ := s.store.Snapshot()
			sequence := s.stopSequence.Load()
			ctx, cancel := context.WithCancel(s.withManualVibrationResume(t.Context(), settings, sequence))
			defer cancel()
			switch change {
			case "stop":
				_, _ = s.emergencyStop(t.Context(), "new_stop")
			case "zero":
				_, _ = vibratorRequest(t, s, "/api/transport/intiface/vibrator/intensity", `{"intensity_percent":0}`, true)
			case "disconnect":
				_, _ = vibratorRequest(t, s, "/api/transport/intiface/disconnect", `{"device":"vibrator"}`, true)
			case "off":
				saveSettings(t, s.store, func(settings config.Settings) config.Settings {
					settings.LLM.MotionGenerationMode = config.LLMMotionModeOff
					return settings
				})
			case "cancel":
				cancel()
			}
			spec := motion.VibrationSpec{Mode: "steady", MinPercent: 50, MaxPercent: 50}
			result, err := s.dispatchChatMotionAt(ctx, &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &spec}, &sequence)
			if err == nil || result.Applied || s.intifaceSnapshot().Vibrator.IntensityPercent != 0 {
				t.Fatalf("changed authority resumed: %+v %v", result, err)
			}
		})
	}
}

func TestBackgroundVibrationCannotUseManualResume(t *testing.T) {
	s := independentVibrationServer(t)
	stopVibrationForResume(t, s)
	settings, _ := s.store.Snapshot()
	if s.chatOutputContext(settings.LLM.ConversationSettings()).Vibrator.ResumeOnMotionRequest {
		t.Fatal("background model received manual restart permission")
	}
	spec := motion.VibrationSpec{Mode: "steady", MinPercent: 50, MaxPercent: 50}
	if _, err := s.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionNone, Vibration: &spec}); err == nil {
		t.Fatal("background command restored intensity")
	}
}

func TestManualVibrationResumeRepair(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh repair", true: "zero before repair"}[revoke], func(t *testing.T) {
			s := independentVibrationServer(t)
			configureResponseRepair(t, s, false)
			p := &scriptedLLMProvider{responses: []string{`{"action":"none","edits":{},"vibration":{"mode":"wave","min_percent":20,"max_percent":160,"period_seconds":4},"reply":"Invalid."}`, vibrationResumeResponse}}
			s.llm.provider = p
			stopVibrationForResume(t, s)
			session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Resume a gentle vibration wave."}`))
			if revoke {
				_, _ = vibratorRequest(t, s, "/api/transport/intiface/vibrator/intensity", `{"intensity_percent":0}`, true)
			}
			r := postRepair(s, session, id)
			if revoke {
				if r.Code != http.StatusConflict || p.callCount() != 1 || s.intifaceSnapshot().Vibrator.IntensityPercent != 0 {
					t.Fatalf("revoked repair: %d %s", r.Code, r.Body.String())
				}
			} else if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"applied":true`) || s.intifaceSnapshot().Vibrator.IntensityPercent != 40 {
				t.Fatalf("fresh repair could not resume: %d %s", r.Code, r.Body.String())
			}
		})
	}
}

type stopDuringResumeProvider struct {
	*scriptedLLMProvider
	stop func()
}

func (p *stopDuringResumeProvider) StreamChat(ctx context.Context, req llm.ChatRequest, delta func(string) error) (string, error) {
	p.stop()
	return p.scriptedLLMProvider.StreamChat(ctx, req, delta)
}

func TestNewStopDuringManualChatGenerationStaysStopped(t *testing.T) {
	s := independentVibrationServer(t)
	stopVibrationForResume(t, s)
	s.llm.provider = &stopDuringResumeProvider{&scriptedLLMProvider{responses: []string{vibrationResumeResponse}}, func() { _, _ = s.emergencyStop(t.Context(), "during_generation") }}
	body := postChatStream(t, s, `{"message":"Resume a gentle vibration wave."}`)
	if strings.Contains(body, `"applied":true`) || s.intifaceSnapshot().Vibrator.IntensityPercent != 0 {
		t.Fatalf("late reply revived output: %s", body)
	}
}
