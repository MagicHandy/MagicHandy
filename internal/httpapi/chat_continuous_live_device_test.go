//go:build integration

package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	continuousSessionDuration = 5 * time.Minute
	continuousBetweenSteps    = time.Duration(0)
	continuousMinActiveRatio  = 0.80
	continuousIdealMaxIdle    = time.Duration(0)
	continuousMaxIdleTolerance = 250 * time.Millisecond
	continuousTurboMaxDeltaMS = int64(30)
)

type continuousPhysicsStep struct {
	label   string
	command *chat.MotionCommand
}

type continuousStepResult struct {
	Label         string
	Regiao        string
	TipoBatida    string
	AtrasoMS      int
	PositionMin   float64
	PositionMax   float64
	MinHSPDeltaMS int64
}

// TestChatContinuousSyncOnRealDevice5Min validates uninterrupted procedural motion on a
// real Handy for 5 minutes across every tipo_batida (lento → turbo/vibrate 1ms) and every
// regiao, chaining target dispatches with bridge filler between segments.
//
// Run:
//
//	$env:MAGICHANDY_DATA_DIR = "c:\dev\git\MyProjects\Handy\MagicHandy\.local-data"
//	go test -tags=integration ./internal/httpapi -run TestChatContinuousSyncOnRealDevice5Min -v -timeout 12m
func TestChatContinuousSyncOnRealDevice5Min(t *testing.T) {
	server := newLiveSQLiteTestServer(t)
	recorder := installE2ERecordingTransport(t, server)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		settings.Motion.HardwareSafetyLock = false
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
	steps := buildContinuousPhysicsMatrix()
	if len(steps) == 0 {
		t.Fatal("physics matrix is empty")
	}

	stepGap := continuousBetweenSteps * time.Duration(len(steps))
	stepHold := (continuousSessionDuration - stepGap) / time.Duration(len(steps))
	if stepHold < 3*time.Second {
		stepHold = 3 * time.Second
	}

	sessionStart := time.Now()
	sessionEnd := sessionStart.Add(continuousSessionDuration)
	errorsBefore := recorder.transportErrorCount()
	addsBefore := recorder.hspAddCount()
	playsAtStart := recorder.hspPlayCount()

	var (
		allSamples  []proceduralSyncSample
		stepResults []continuousStepResult
	)

	t.Logf("physics matrix: %d steps (tipos×zonas), hold=%s gap=%s session=%s",
		len(steps), stepHold.Round(time.Millisecond), continuousBetweenSteps, continuousSessionDuration)

	for index, step := range steps {
		if time.Now().After(sessionEnd) {
			t.Logf("session budget reached at step %d/%d", index+1, len(steps))
			break
		}

		gen := bumpChatChaosGeneration(server)
		addsStepBefore := recorder.hspAddCount()

		if err := server.playChatChaoticMotion(ctx, cloneMotionCommand(step.command), settings, gen); err != nil {
			t.Fatalf("step %s playChatChaoticMotion: %v", step.label, err)
		}
		waitForChaosStepDispatch(t, server, recorder, addsBefore, index == 0)
		addsBefore = recorder.hspAddCount()
		logTransportIssuesSince(t, recorder, errorsBefore)

		hold := stepHold
		if remaining := time.Until(sessionEnd); remaining < hold {
			hold = remaining
		}
		if hold <= 0 {
			break
		}

		samples := sampleProceduralSync(t, server, hold)
		allSamples = append(allSamples, samples...)

		result := summarizeContinuousStep(step, samples, recorder, addsStepBefore)
		stepResults = append(stepResults, result)
		t.Logf("step %d/%d %s: regiao=%s tipo=%s atraso=%d pos=%.1f..%.1f min_hsp_delta=%dms player=%v",
			index+1, len(steps), step.label, result.Regiao, result.TipoBatida, result.AtrasoMS,
			result.PositionMin, result.PositionMax, result.MinHSPDeltaMS, isChatChaosPlayerRunning(server))

		if isTurboTipo(step.command.TipoBatida) && result.MinHSPDeltaMS >= continuousTurboMaxDeltaMS {
			t.Errorf("step %s: turbo/vibrate min_hsp_delta=%dms want <%dms",
				step.label, result.MinHSPDeltaMS, continuousTurboMaxDeltaMS)
		}
		if isTurboTipo(step.command.TipoBatida) {
			reversals := countPositionReversals(samples)
			if reversals < 2 && result.MinHSPDeltaMS >= continuousTurboMaxDeltaMS {
				t.Errorf("step %s: reversals=%d and min_hsp_delta=%dms — turbo/vibrate zigzag not detected",
					step.label, reversals, result.MinHSPDeltaMS)
			}
		}

		if time.Now().Before(sessionEnd) && index < len(steps)-1 {
			time.Sleep(continuousBetweenSteps)
		}
	}

	if remaining := time.Until(sessionEnd); remaining > 0 {
		t.Logf("padding %.0fs to complete 5-minute window", remaining.Seconds())
		allSamples = append(allSamples, sampleProceduralSync(t, server, remaining)...)
	}

	stopProceduralSyncMotion(t, server)

	coreSamples := trimTrailingIdle(allSamples)
	if len(coreSamples) == 0 {
		t.Fatal("no active sync samples collected during session")
	}

	report := analyzeProceduralSyncSamples(t, server, coreSamples, recorder)
	t.Logf("=== 5-minute continuous physics matrix report ===")
	t.Logf("session_duration=%s steps=%d samples=%d active_ratio=%.3f max_idle=%s position_range=%.1f",
		time.Since(sessionStart).Round(time.Second), len(stepResults), report.SampleCount,
		report.ActiveRatio, report.MaxIdleGap, report.PositionRange)
	t.Logf("hsp_plays=%d (start=%d) hsp_adds=%d starvation=%d max_hsp_gap_ms=%d",
		recorder.hspPlayCount(), playsAtStart, recorder.hspAddCount(),
		report.StarvationEvents, report.MaxHSPPointGapMS)

	logCoverageGaps(t, stepResults)

	if significantTransportErrors(recorder.snapshot().Errors, errorsBefore) > 0 {
		t.Fatalf("transport errors during session: %v", recorder.snapshot().Errors[errorsBefore:])
	}
	if report.ActiveRatio < continuousMinActiveRatio {
		t.Fatalf("active_ratio=%.3f want>=%.3f (motion interrupted)", report.ActiveRatio, continuousMinActiveRatio)
	}
	if report.MaxIdleGap > continuousIdealMaxIdle {
		t.Logf("WARNING: max_idle_gap=%s exceeds ideal %s — review bridge filler between dispatches",
			report.MaxIdleGap, continuousIdealMaxIdle)
	}
	if report.MaxIdleGap > continuousMaxIdleTolerance {
		t.Fatalf("max_idle_gap=%s exceeds tolerance %s (Handy must not pause)", report.MaxIdleGap, continuousMaxIdleTolerance)
	}
	if report.MaxIdleGap > 45*time.Second {
		t.Fatalf("max_idle_gap=%s exceeds 45s tolerance", report.MaxIdleGap)
	}
	if report.PositionRange < proceduralSyncMinPositionDelta {
		t.Fatalf("position_range=%.2f want>=%.2f", report.PositionRange, proceduralSyncMinPositionDelta)
	}

	t.Log("5-minute continuous physics matrix device session PASSED")
}

func buildContinuousPhysicsMatrix() []continuousPhysicsStep {
	tipos := []struct {
		name   string
		atraso int
		vel    int
	}{
		{name: "lento", vel: 22},
		{name: "fluido", vel: 38},
		{name: "leve", vel: 32},
		{name: "simples", vel: 42},
		{name: "moderado", vel: 52},
		{name: "alto", vel: 68},
		{name: "very_fast", vel: 82},
		{name: "vibrate", atraso: 1, vel: 88},
		{name: "turbo", atraso: 1, vel: 95},
	}
	zonas := []string{
		"base",
		"meio",
		"cabeca",
		"meio_cabeca",
		"meio_base",
		"cabeca_base",
		"full",
	}

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
				Intensidade: 40 + tipo.vel/3,
				Regiao:      zona,
				TipoBatida:  tipo.name,
			}
			if tipo.atraso > 0 {
				cmd.AtrasoMS = tipo.atraso
			}
			steps = append(steps, continuousPhysicsStep{
				label: tipo.name + "_" + zona,
				command: cmd,
			})
		}
	}
	return steps
}

func summarizeContinuousStep(
	step continuousPhysicsStep,
	samples []proceduralSyncSample,
	recorder *recordingMotionTransport,
	addsBefore int,
) continuousStepResult {
	result := continuousStepResult{
		Label:      step.label,
		Regiao:     step.command.Regiao,
		TipoBatida: step.command.TipoBatida,
		AtrasoMS:   step.command.AtrasoMS,
	}
	if len(samples) == 0 {
		return result
	}
	minPos, maxPos := samples[0].PositionPct, samples[0].PositionPct
	for _, sample := range samples {
		if sample.PositionPct < minPos {
			minPos = sample.PositionPct
		}
		if sample.PositionPct > maxPos {
			maxPos = sample.PositionPct
		}
	}
	result.PositionMin = minPos
	result.PositionMax = maxPos

	points := collectHSPPointsFromAdds(recorder, addsBefore)
	if len(points) >= 2 {
		result.MinHSPDeltaMS = minConsecutiveDelta(points)
	}
	return result
}

func isTurboTipo(tipo string) bool {
	return motion.IsTurboTipo(tipo)
}

func countPositionReversals(samples []proceduralSyncSample) int {
	reversals := 0
	for i := 2; i < len(samples); i++ {
		prev := samples[i-1].PositionPct - samples[i-2].PositionPct
		cur := samples[i].PositionPct - samples[i-1].PositionPct
		if prev != 0 && cur != 0 && (prev > 0) != (cur > 0) {
			reversals++
		}
	}
	return reversals
}

func logCoverageGaps(t *testing.T, results []continuousStepResult) {
	t.Helper()

	seenTipo := map[string]struct{}{}
	seenZona := map[string]struct{}{}
	for _, result := range results {
		seenTipo[result.TipoBatida] = struct{}{}
		seenZona[result.Regiao] = struct{}{}
	}

	wantTipos := []string{"lento", "fluido", "leve", "simples", "moderado", "alto", "very_fast", "vibrate", "turbo"}
	wantZonas := []string{"base", "meio", "cabeca", "meio_cabeca", "meio_base", "cabeca_base", "full"}

	for _, tipo := range wantTipos {
		if _, ok := seenTipo[tipo]; !ok {
			t.Errorf("coverage gap: tipo_batida %q not exercised", tipo)
		}
	}
	for _, zona := range wantZonas {
		if _, ok := seenZona[zona]; !ok {
			t.Errorf("coverage gap: regiao %q not exercised", zona)
		}
	}

	for _, result := range results {
		minPct, maxPct, ok := motion.RegionBounds(result.Regiao)
		if !ok {
			continue
		}
		// Allow ±12% slack for blend crossfades at zone boundaries.
		low := float64(minPct) - 12
		high := float64(maxPct) + 12
		if result.PositionMax < low || result.PositionMin > high {
			t.Logf("WARNING: step %s regiao=%s expected ~%d..%d got pos %.1f..%.1f",
				result.Label, result.Regiao, minPct, maxPct, result.PositionMin, result.PositionMax)
		}
	}
}

func waitForChaosStepDispatch(
	t *testing.T,
	server *Server,
	recorder *recordingMotionTransport,
	addsBefore int,
	firstStart bool,
) {
	t.Helper()

	if firstStart {
		waitForHSPDispatchAfter(t, recorder, addsBefore)
		return
	}

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if isChatChaosPlayerRunning(server) {
			return
		}
		if recorder.hspAddCount() > addsBefore {
			time.Sleep(100 * time.Millisecond)
			if isChatChaosPlayerRunning(server) {
				return
			}
		}
		time.Sleep(e2ePollInterval)
	}
	t.Fatalf("target dispatch did not keep player running (adds=%d plays=%d errors=%v)",
		recorder.hspAddCount(), recorder.hspPlayCount(), recorder.snapshot().Errors)
}

func logTransportIssuesSince(t *testing.T, recorder *recordingMotionTransport, since int) {
	t.Helper()
	for _, err := range recorder.snapshot().Errors[since:] {
		if strings.Contains(strings.ToLower(err), "context canceled") {
			continue
		}
		t.Logf("transport note: %s", err)
	}
}

func significantTransportErrors(errors []string, since int) int {
	count := 0
	for _, err := range errors[since:] {
		lower := strings.ToLower(err)
		if strings.Contains(lower, "context canceled") {
			continue
		}
		if strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout") {
			continue
		}
		count++
	}
	return count
}
