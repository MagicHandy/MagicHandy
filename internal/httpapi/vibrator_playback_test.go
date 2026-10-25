package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestVibratorEngineStartup(t *testing.T) {
	for _, linked := range []bool{false, true} {
		for _, follow := range []string{"speed", "depth", "steady"} {
			name := follow
			if linked {
				name += "/handy-intiface"
			} else {
				name += "/solo"
			}
			t.Run(name, func(t *testing.T) {
				playing := []string{config.DeviceVibrator}
				if linked {
					playing = []string{config.DeviceOutputHandy, config.DeviceVibrator}
				}
				server := newVibratorTestServer(t, playing...)
				saveSettings(t, server.store, func(s config.Settings) config.Settings {
					s.Vibration.Follow = follow
					if linked {
						s.Device.HSPDispatchOwner = config.DispatchOwnerIntiface
					}
					return s
				})
				connectVibrator(t, server)
				if linked {
					if code, _ := vibratorRequest(t, server, "/api/transport/intiface/connect", `{}`, true); code != 200 {
						t.Fatalf("linear connect %d", code)
					}
					if code, _ := vibratorRequest(t, server, "/api/transport/intiface/select", `{"device_index":0,"actuator_index":0}`, true); code != 200 {
						t.Fatalf("linear select %d", code)
					}
				}
				if code, _ := vibratorRequest(t, server, "/api/transport/intiface/vibrator/intensity", `{"intensity_percent":40}`, true); code != 200 {
					t.Fatalf("arm %d", code)
				}
				engine, admission, err := server.motionEngineForStart()
				if err != nil {
					t.Fatal(err)
				}
				settings, _ := server.store.Snapshot()
				target, err := motion.FlowTarget(motion.DefaultFlowSpec(), settings.Motion)
				if err != nil {
					t.Fatal(err)
				}
				target.Source = "chat"
				started := time.Now()
				_, err = engine.StartAtGeneration(context.Background(), target, settings.Motion, admission)
				if err != nil {
					t.Fatalf("start rejected: %v", err)
				}
				timer := time.NewTimer(900 * time.Millisecond)
				defer timer.Stop()
				<-timer.C
				state := engine.Snapshot()
				t.Logf("after=%s running=%v paused=%v reason=%s vibrator=%+v linked=%+v", time.Since(started), state.Running, state.Paused, state.LastError, server.intifaceSnapshot().Vibrator, state.LinkedSession)
				if !state.Running {
					t.Errorf("fully buffered simulation stopped during startup: %s", state.LastError)
				}
				_, _ = engine.Stop(context.Background(), "review_finished")
			})
		}
	}
}
