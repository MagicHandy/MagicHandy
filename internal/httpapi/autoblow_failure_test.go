package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestAutoblowLinkedFailurePoliciesAndIdleReconnect(t *testing.T) {
	for _, policy := range []string{config.LinkedFailureStopAll, config.LinkedFailureContinueHealthy} {
		t.Run(policy, func(t *testing.T) {
			s, sim := newAutoblowTestServer(t)
			addVacuGlide(t, s, true)
			saveVacuGlideToken(t, s)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.Device.LinkedFailurePolicy = policy
				return settings
			})
			connectVacuGlide(t, s)
			engine, _, err := s.motionEngineForStart()
			if err != nil {
				t.Fatal(err)
			}
			settings, _ := s.store.Snapshot()
			if _, err = engine.Start(context.Background(), motion.MotionTarget{PatternID: motion.PatternStroke, SpeedPercent: 40}, settings.Motion); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "motor running", func() bool { return sim.VacuGlideSpeed() > 0 })
			sim.SetOnline("vacuglide", false)
			waitUntil(t, "lost cloud participant", func() bool { return !s.autoblowSnapshot().VacuGlide.Connected })
			if policy == config.LinkedFailureStopAll {
				waitUntil(t, "linked Stop", func() bool { return !engine.Snapshot().Running })
				if engine.Snapshot().LastError == "" {
					t.Fatal("lost participant was not reported")
				}
			} else {
				time.Sleep(350 * time.Millisecond)
				if !engine.Snapshot().Running {
					t.Fatal("healthy participant did not continue")
				}
			}
			sim.SetOnline("vacuglide", true)
			connectVacuGlide(t, s)
			time.Sleep(300 * time.Millisecond)
			if got := s.autoblowSnapshot().VacuGlide; got.PlaybackState != "idle" || sim.VacuGlideSpeed() != 0 {
				t.Fatalf("reconnect joined old run: %+v", got)
			}
			if _, err = engine.Stop(context.Background(), "review_stop"); err != nil {
				t.Fatal(err)
			}
			fresh, _, err := s.motionEngineForStart()
			if err != nil || fresh == engine {
				t.Fatalf("restart did not create a fresh session: %v", err)
			}
		})
	}
}

func TestAutoblowDiagnosticExportRedactsBothTokens(t *testing.T) {
	s, _ := newAutoblowTestServer(t)
	secrets := []string{"ultra-private+token/value", "vacu-private+token/value"}
	for i, device := range []string{config.DeviceAutoblow, config.DeviceVacuGlide} {
		playOnly(t, s, device)
		code, body := autoblowRequest(t, s, http.MethodPut, "/api/settings/device/autoblow-token", `{"device":"`+device+`","token":"`+secrets[i]+`"}`, true)
		if code != http.StatusOK {
			t.Fatalf("save token: %d %s", code, body)
		}
		s.traces.Add(diagnostics.MotionTraceRow{Source: "autoblow", Annotation: "command failed " + secrets[i] + " " + url.QueryEscape(secrets[i])})
	}
	files := diagnosticFiles(t, s, "?chat=full&model_output=true&personas=true&prompts=true&memories=true")
	for _, secret := range secrets {
		assertDiagnosticExcludes(t, files, secret, url.QueryEscape(secret))
	}
	if !strings.Contains(files["motion/live-trace.json"], "command failed [redacted]") {
		t.Fatal("redaction removed useful context")
	}
}
