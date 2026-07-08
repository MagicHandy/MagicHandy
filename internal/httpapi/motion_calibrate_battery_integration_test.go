//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type deviceCalibrateCase struct {
	name    string
	profile string
	command *chat.MotionCommand
}

func TestMotionCalibrateBatteryDevice(t *testing.T) {
	server := newCloudTestServer(t, Runtime{})
	saveE2EDeviceSettings(t, server)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.HardwareSafetyLock = false
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		return settings
	})
	requireE2EDeviceReady(t, server)

	recorder := installE2ERecordingTransport(t, server)
	t.Cleanup(func() { stopE2EMotion(t, server) })

	cases := deviceCalibrationCases()
	passed := 0
	for index, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if index > 0 {
				time.Sleep(900 * time.Millisecond)
			}
			errorsBefore := recorder.transportErrorCount()
			settings, _ := server.store.Snapshot()
			command := cloneMotionCommand(tc.command)
			chat.NormalizeChaoticPhysics(command)

			physics := motion.ChaoticPhysics{
				Velocidade:  command.Velocidade,
				Intensidade: command.Intensidade,
				Regiao:      command.Regiao,
				TipoBatida:  command.TipoBatida,
				AtrasoMS:    command.AtrasoMS,
			}
			continueFrom := chatChaosContinueFrom(server)
			waypoints := motion.GenerateStrokeWaypointsFromPosition(
				physics,
				motion.EstimateChatMotionDurationMS(physics),
				settings.Motion.HardwareSafetyLock,
				nil,
				continueFrom,
			)
			lead := int64(300)
			if continueFrom >= 0 {
				lead = 80
			}
			expected := motion.SummarizeMotionTrace(physics, waypoints, lead, continueFrom)

			ctx := context.Background()
			addsBeforePlay := recorder.hspAddCount()
			generation := bumpChatChaosGeneration(server)
			if err := server.playChatChaoticMotion(ctx, command, settings, generation); err != nil {
				t.Fatalf("playChatChaoticMotion: %v", err)
			}
			errorsBefore = recorder.transportErrorCount()
			waitForHSPDispatchAfter(t, recorder, addsBeforePlay)
			assertRecorderTransportOKSince(t, recorder, errorsBefore)

			points := newestHSPAddPoints(recorder, addsBeforePlay)
			if len(points) < 4 {
				t.Fatalf("HSP points = %d, want >= 4", len(points))
			}
			const travelMargin = 5
			for _, point := range points {
				if point.PositionPercent < expected.ZoneMin-travelMargin ||
					point.PositionPercent > expected.ZoneMax+travelMargin {
					t.Fatalf("HSP point x=%d outside zone %d..%d (travel %d..%d continue_from=%d)",
						point.PositionPercent, expected.ZoneMin, expected.ZoneMax,
						expected.PosMin, expected.PosMax, continueFrom)
				}
			}
			minDelta := minConsecutiveDelta(points)
			if minDelta > 120 && command.TipoBatida != "very_fast" {
				t.Fatalf("min HSP delta = %dms, want fluid pacing for %s", minDelta, command.TipoBatida)
			}
			if command.TipoBatida == "very_fast" && minDelta > 15 {
				t.Fatalf("very_fast min delta = %dms, want turbo", minDelta)
			}

			tracePayload, _ := json.Marshal(map[string]any{
				"case":     tc.name,
				"profile":  tc.profile,
				"physics":  physics,
				"expected": expected,
				"hsp": map[string]any{
					"points":         len(points),
					"min_delta":      minDelta,
					"continue_from":  continueFrom,
				},
			})
			motion.MotionDebugLog("BAT", "motion_calibrate_battery_integration_test.go", "device case ok", map[string]any{
				"summary": string(tracePayload),
			})
			passed++
		})
	}
	t.Logf("device calibration battery passed %d/%d cases with hardware_safety_lock=false", passed, len(cases))
}

func deviceCalibrationCases() []deviceCalibrateCase {
	return []deviceCalibrateCase{
		profileCase("gentle_fluido_meio", "gentle", "meio", "fluido", 30, 35, 160),
		profileCase("gentle_lento_meio_cabeca", "gentle", "meio_cabeca", "lento", 25, 30, 160),
		profileCase("gentle_leve_meio", "gentle", "meio", "leve", 35, 40, 120),
		profileCase("balanced_fluido_meio_cabeca", "balanced", "meio_cabeca", "fluido", 50, 55, 160),
		profileCase("balanced_moderado_cabeca", "balanced", "cabeca", "moderado", 55, 60, 80),
		profileCase("balanced_leve_meio", "balanced", "meio", "leve", 45, 50, 120),
		profileCase("intense_alto_cabeca", "intense", "cabeca", "alto", 85, 80, 40),
		profileCase("intense_moderado_meio_cabeca", "intense", "meio_cabeca", "moderado", 75, 70, 80),
		profileCase("intense_fluido_cabeca", "intense", "cabeca", "fluido", 70, 75, 160),
		regionSweep("cabeca", "fluido", 50),
		regionSweep("meio", "moderado", 50),
		regionSweep("base", "lento", 40),
		regionSweep("meio_cabeca", "leve", 55),
		regionSweep("meio_base", "fluido", 50),
		regionSweep("full", "simples", 60),
		velocityCase("cabeca_vel25", "cabeca", "fluido", 25),
		velocityCase("cabeca_vel50", "cabeca", "fluido", 50),
		velocityCase("cabeca_vel75", "cabeca", "fluido", 75),
		velocityCase("cabeca_vel95", "cabeca", "fluido", 95),
		tipoCase("tipo_simples", "meio", "simples", 50),
		tipoCase("tipo_alto", "cabeca", "alto", 80),
		tipoCase("tipo_very_fast", "cabeca", "very_fast", 90),
	}
}

func profileCase(name, profile, regiao, tipo string, vel, inten, atraso int) deviceCalibrateCase {
	return deviceCalibrateCase{
		name:    name,
		profile: profile,
		command: &chat.MotionCommand{
			Action:      chat.MotionActionStart,
			Regiao:      regiao,
			TipoBatida:  tipo,
			Velocidade:  vel,
			Intensidade: inten,
			AtrasoMS:    atraso,
		},
	}
}

func regionSweep(regiao, tipo string, vel int) deviceCalibrateCase {
	return profileCase("sweep_"+regiao+"_"+tipo, "balanced", regiao, tipo, vel, 50, 160)
}

func velocityCase(name, regiao, tipo string, vel int) deviceCalibrateCase {
	return profileCase(name, "balanced", regiao, tipo, vel, 50, 160)
}

func tipoCase(name, regiao, tipo string, vel int) deviceCalibrateCase {
	return profileCase(name, "intense", regiao, tipo, vel, 70, 0)
}

func bumpChatChaosGeneration(server *Server) uint64 {
	server.chatChaos.mu.Lock()
	defer server.chatChaos.mu.Unlock()
	server.chatChaos.generation++
	return server.chatChaos.generation
}

func chatChaosContinueFrom(server *Server) int {
	continueFrom := -1
	server.chatChaos.mu.Lock()
	if existing := server.chatChaos.player; existing != nil {
		if snap := existing.Snapshot(); snap.Running {
			continueFrom = int(math.Round(snap.PositionPct))
		}
	}
	server.chatChaos.mu.Unlock()
	return continueFrom
}

func newestHSPAddPoints(recorder *recordingMotionTransport, skipAdds int) []transport.TimedPoint {
	snap := recorder.snapshot()
	if len(snap.HSPAdds) <= skipAdds {
		return nil
	}
	return snap.HSPAdds[len(snap.HSPAdds)-1].Points
}
