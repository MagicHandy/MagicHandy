//go:build integration

package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	turboFastSessionDuration   = 3 * time.Minute
	turboFastMinActiveRatio    = 0.85
	turboFastMaxIdleTolerance  = 250 * time.Millisecond
	turboFastMinPositionSpan   = 18.0
	turboFastMaxHSPDeltaMS     = int64(55)
	turboFastMinReversals      = 2
)

// TestTurboFastVelocitiesOnRealDevice validates very_fast, vibrate, and turbo only —
// full-zone sawtooth at 1ms with zero idle gaps.
//
// Run:
//
//	$env:MAGICHANDY_DATA_DIR = "c:\dev\git\MyProjects\Handy\MagicHandy\.local-data"
//	go test -tags=integration ./internal/httpapi -run TestTurboFastVelocitiesOnRealDevice -v -timeout 8m
func TestTurboFastVelocitiesOnRealDevice(t *testing.T) {
	server := newLiveSQLiteTestServer(t)
	recorder := installE2ERecordingTransport(t, server)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		settings.Motion.HardwareSafetyLock = false
		settings.Motion.StrokeMinPercent = 0
		settings.Motion.StrokeMaxPercent = 100
		settings.Diagnostics.Verbosity = config.DiagnosticsVerbosityDebug
		if settings.Device.HSPDispatchOwner == "" {
			settings.Device.HSPDispatchOwner = config.DispatchOwnerCloudREST
		}
		return settings
	})

	settings, _ := server.store.Snapshot()
	if settings.Device.HandyConnectionKey == "" {
		t.Skip("no handy_connection_key in SQLite store — configure device in Settings first")
	}

	requireLiveDeviceFromStore(t, server)
	t.Cleanup(func() { stopProceduralSyncMotion(t, server) })

	ctx := context.Background()
	steps := buildTurboFastMatrix()
	sessionStart := time.Now()
	sessionEnd := sessionStart.Add(turboFastSessionDuration)
	stepHold := turboFastSessionDuration / time.Duration(len(steps))
	if stepHold < 4*time.Second {
		stepHold = 4 * time.Second
	}

	errorsBefore := recorder.transportErrorCount()
	addsBefore := recorder.hspAddCount()

	var allSamples []proceduralSyncSample

	t.Logf("turbo-fast matrix: %d steps, hold=%s session=%s", len(steps), stepHold.Round(time.Millisecond), turboFastSessionDuration)

	for index, step := range steps {
		if time.Now().After(sessionEnd) {
			t.Logf("session budget reached at step %d/%d", index+1, len(steps))
			break
		}

		gen := bumpChatChaosGeneration(server)
		addsStepBefore := recorder.hspAddCount()

		if err := server.playChatChaoticMotion(ctx, cloneMotionCommand(step.command), settings, gen); err != nil {
			t.Fatalf("step %s: %v", step.label, err)
		}
		waitForChaosStepDispatch(t, server, recorder, addsBefore, index == 0)
		addsBefore = recorder.hspAddCount()
		logTransportIssuesSince(t, recorder, errorsBefore)

		// Validate the dispatch batch immediately (not the whole 12s window).
		if batch := newestHSPAddPoints(recorder, addsStepBefore); len(batch) > 0 {
			hspMin, hspMax := batch[0].PositionPercent, batch[0].PositionPercent
			for _, point := range batch {
				if point.PositionPercent < hspMin {
					hspMin = point.PositionPercent
				}
				if point.PositionPercent > hspMax {
					hspMax = point.PositionPercent
				}
			}
			if minPct, maxPct, ok := motion.RegionBounds(step.command.Regiao); ok {
				tolerance := 12
				if step.command.Regiao == "cabeca" || step.command.Regiao == "base" {
					tolerance = 5
				}
				if hspMax > 0 && (hspMax < minPct-tolerance || hspMin > maxPct+tolerance) {
					t.Errorf("step %s: dispatch HSP %d..%d outside regiao %d..%d",
						step.label, hspMin, hspMax, minPct, maxPct)
				}
			}
		}

		hold := stepHold
		if remaining := time.Until(sessionEnd); remaining < hold {
			hold = remaining
		}
		samples := sampleProceduralSync(t, server, hold)
		allSamples = append(allSamples, samples...)

		result := summarizeContinuousStep(step, samples, recorder, addsStepBefore)
		span := result.PositionMax - result.PositionMin
		reversals := countPositionReversals(samples)
		hspMin, hspMax := hspPointRange(recorder, addsStepBefore)

		t.Logf("step %d/%d %s: tipo=%s regiao=%s pos=%.1f..%.1f span=%.1f hsp_session=%d..%d rev=%d min_delta=%dms player=%v",
			index+1, len(steps), step.label, result.TipoBatida, result.Regiao,
			result.PositionMin, result.PositionMax, span, hspMin, hspMax, reversals, result.MinHSPDeltaMS,
			isChatChaosPlayerRunning(server))

		if minPct, maxPct, ok := motion.RegionBounds(result.Regiao); ok {
			if result.PositionMax > 0 && (result.PositionMax < float64(minPct)-15 || result.PositionMin > float64(maxPct)+15) {
				t.Logf("WARNING: step %s visual pos %.1f..%.1f vs regiao %d..%d",
					step.label, result.PositionMin, result.PositionMax, minPct, maxPct)
			}
		}

		if result.MinHSPDeltaMS > turboFastMaxHSPDeltaMS {
			t.Errorf("step %s: min_hsp_delta=%dms want <=%dms", step.label, result.MinHSPDeltaMS, turboFastMaxHSPDeltaMS)
		}
		if span < turboFastMinPositionSpan {
			t.Errorf("step %s: position span=%.1f want >=%.1f (full-speed travel)", step.label, span, turboFastMinPositionSpan)
		}
		if reversals < turboFastMinReversals && result.MinHSPDeltaMS > turboFastMaxHSPDeltaMS {
			t.Errorf("step %s: reversals=%d and min_hsp_delta=%dms — turbo oscillation weak", step.label, reversals, result.MinHSPDeltaMS)
		}
	}

	if remaining := time.Until(sessionEnd); remaining > 0 {
		allSamples = append(allSamples, sampleProceduralSync(t, server, remaining)...)
	}
	stopProceduralSyncMotion(t, server)

	coreSamples := trimTrailingIdle(allSamples)
	report := analyzeProceduralSyncSamples(t, server, coreSamples, recorder)

	t.Logf("=== turbo-fast device report ===")
	t.Logf("duration=%s samples=%d active_ratio=%.3f max_idle=%s position_range=%.1f hsp_adds=%d",
		time.Since(sessionStart).Round(time.Second), report.SampleCount,
		report.ActiveRatio, report.MaxIdleGap, report.PositionRange, recorder.hspAddCount())

	if report.ActiveRatio < turboFastMinActiveRatio {
		t.Fatalf("active_ratio=%.3f want>=%.3f", report.ActiveRatio, turboFastMinActiveRatio)
	}
	if report.MaxIdleGap > turboFastMaxIdleTolerance {
		t.Fatalf("max_idle_gap=%s want <=%s", report.MaxIdleGap, turboFastMaxIdleTolerance)
	}
	if report.PositionRange < 25 {
		t.Fatalf("position_range=%.1f want broad travel during turbo session", report.PositionRange)
	}

	t.Log("turbo-fast device session PASSED")
}

func buildTurboFastMatrix() []continuousPhysicsStep {
	tipos := []struct {
		name   string
		atraso int
		vel    int
	}{
		{name: "very_fast", vel: 90},
		{name: "vibrate", atraso: 1, vel: 92},
		{name: "turbo", atraso: 1, vel: 98},
	}
	zonas := []string{"base", "meio", "cabeca", "meio_cabeca", "full"}

	steps := make([]continuousPhysicsStep, 0, len(tipos)*len(zonas))
	first := true
	for _, tipo := range tipos {
		for _, zona := range zonas {
			action := chat.MotionActionTarget
			if first {
				action = chat.MotionActionStart
				first = false
			}
			cmd := &chat.MotionCommand{
				Action:      action,
				Velocidade:  tipo.vel,
				Intensidade: 70,
				Regiao:      zona,
				TipoBatida:  tipo.name,
			}
			if tipo.atraso > 0 {
				cmd.AtrasoMS = tipo.atraso
			}
			steps = append(steps, continuousPhysicsStep{
				label:   tipo.name + "_" + zona,
				command: cmd,
			})
		}
	}
	return steps
}

func hspPointRange(recorder *recordingMotionTransport, addsBefore int) (int, int) {
	points := collectHSPPointsFromAdds(recorder, addsBefore)
	if len(points) == 0 {
		return 0, 0
	}
	minPos, maxPos := points[0].PositionPercent, points[0].PositionPercent
	for _, point := range points {
		if point.PositionPercent < minPos {
			minPos = point.PositionPercent
		}
		if point.PositionPercent > maxPos {
			maxPos = point.PositionPercent
		}
	}
	return minPos, maxPos
}
