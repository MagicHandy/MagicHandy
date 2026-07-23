//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

const (
	e2eLocalLLMMinSession       = 5 * time.Minute
	e2eLocalLLMMinActiveRatio   = 0.80
	e2eLocalLLMMaxIdleGap       = 250 * time.Millisecond
	e2eLocalLLMStepTimeout      = 90 * time.Second
	e2eLocalLLMMinStepHold      = 2 * time.Second
	e2eLocalLLMMaxStepHold      = 8 * time.Second
)

var e2eLocalLLMPoses = []string{"handjob", "oral", "cavalgando", "deepthroat"}

// TestE2EChatLocalLLMMotionMatrix5Min drives every tipo×regiao through the real local
// LLM via POST /api/chat/send (manual mode), rotates posicoes, keeps the Handy moving
// for at least 5 minutes, then validates that "stop" halts playback.
//
// Run:
//
//	$env:MAGICHANDY_DATA_DIR = "c:\dev\git\MyProjects\Handy\MagicHandy\.local-data"
//	go test -tags=integration ./internal/httpapi -run TestE2EChatLocalLLMMotionMatrix5Min -v -count=1 -timeout 25m
func TestE2EChatLocalLLMMotionMatrix5Min(t *testing.T) {
	server := newLiveSQLiteTestServer(t)
	recorder := installE2ERecordingTransport(t, server)

	settings, _ := server.store.Snapshot()
	if strings.TrimSpace(settings.Device.HandyConnectionKey) == "" {
		t.Skip("no handy_connection_key in SQLite store")
	}
	requireLiveDeviceFromStore(t, server)
	requireLocalLLMReady(t, server)

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

	t.Cleanup(func() {
		server.stopChatAutoLoop(context.Background())
		putE2EOperationMode(t, server, "manual")
		stopProceduralSyncMotion(t, server)
	})

	putE2EOperationMode(t, server, "manual")
	server.stopChatAutoLoop(context.Background())

	ctx := context.Background()
	steps := buildContinuousPhysicsMatrix()
	if len(steps) == 0 {
		t.Fatal("physics matrix is empty")
	}
	sessionStart := time.Now()
	minSessionEnd := sessionStart.Add(e2eLocalLLMMinSession)
	errorsBefore := recorder.transportErrorCount()
	addsBefore := recorder.hspAddCount()

	var (
		allSamples  []proceduralSyncSample
		stepResults []continuousStepResult
		llmApplied  int
		llmFallback int
		posesSeen   = map[string]struct{}{}
	)

	t.Logf("local LLM matrix: %d steps (9 tipos × 7 zonas), min_session=%s provider=%s model=%s",
		len(steps), e2eLocalLLMMinSession, settings.LLM.Provider, settings.LLM.Model)

	for stepIndex, step := range steps {
		pose := e2eLocalLLMPoses[stepIndex%len(e2eLocalLLMPoses)]
		posesSeen[pose] = struct{}{}

		if !isChatChaosPlayerRunning(server) && stepIndex == 0 {
			t.Logf("cold start %s — waiting for LLM/fallback", step.label)
		}

		addsStepBefore := recorder.hspAddCount()
		prompt := llmMotionUserPrompt(step, pose, stepIndex == 0)

		t.Logf("step %d/%d %s pose=%s → LLM", stepIndex+1, len(steps), step.label, pose)
		applied, reply, err := postE2EChatSendMotion(t, server, prompt, e2eLocalLLMStepTimeout)
		if reply != "" {
			t.Logf("  assistant: %s", truncateForLog(reply, 120))
		}

		if !applied {
			t.Logf("  LLM motion not applied (%v) — direct dispatch fallback", err)
			gen := bumpChatChaosGeneration(server)
			if err := server.playChatChaoticMotion(ctx, cloneMotionCommand(step.command), settings, gen); err != nil {
				t.Fatalf("step %s fallback dispatch: %v", step.label, err)
			}
			llmFallback++
		} else {
			llmApplied++
		}

		waitForChaosDispatchSettled(t, server, recorder, addsStepBefore, stepIndex == 0)
		if !isChatChaosPlayerRunning(server) {
			t.Logf("  player still idle after LLM settle — direct dispatch fallback")
			gen := bumpChatChaosGeneration(server)
			if err := server.playChatChaoticMotion(ctx, cloneMotionCommand(step.command), settings, gen); err != nil {
				t.Fatalf("step %s settle fallback: %v", step.label, err)
			}
			waitForChaosDispatchSettled(t, server, recorder, recorder.hspAddCount(), false)
			llmFallback++
			if llmApplied > 0 {
				llmApplied--
			}
		}
		logTransportIssuesSince(t, recorder, errorsBefore)

		hold := e2eLocalLLMMinStepHold
		if stepIndex < len(steps)-1 {
			hold = e2eLocalLLMMaxStepHold
		}
		samples := sampleProceduralSync(t, server, hold)
		allSamples = append(allSamples, samples...)

		result := summarizeContinuousStep(step, samples, recorder, addsStepBefore)
		stepResults = append(stepResults, result)
		if !isChatChaosPlayerRunning(server) {
			t.Logf("  player stopped after hold — resuming %s", step.label)
			gen := bumpChatChaosGeneration(server)
			if err := server.playChatChaoticMotion(ctx, cloneMotionCommand(step.command), settings, gen); err != nil {
				t.Fatalf("step %s resume dispatch: %v", step.label, err)
			}
			waitForChaosDispatchSettled(t, server, recorder, recorder.hspAddCount(), false)
		}
		t.Logf("  regiao=%s tipo=%s pos=%.1f..%.1f player=running",
			result.Regiao, result.TipoBatida, result.PositionMin, result.PositionMax)
	}

	if len(posesSeen) < len(e2eLocalLLMPoses) {
		t.Fatalf("poses_seen=%v want all %v", keysOf(posesSeen), e2eLocalLLMPoses)
	}

	if remaining := time.Until(minSessionEnd); remaining > 0 {
		t.Logf("padding %.0fs to complete minimum 5-minute window", remaining.Seconds())
		allSamples = append(allSamples, sampleProceduralSync(t, server, remaining)...)
		if !isChatChaosPlayerRunning(server) {
			last := steps[len(steps)-1]
			gen := bumpChatChaosGeneration(server)
			_ = server.playChatChaoticMotion(ctx, cloneMotionCommand(last.command), settings, gen)
		}
	}

	coreSamples := trimTrailingIdle(allSamples)
	if len(coreSamples) == 0 {
		t.Fatal("no motion samples collected")
	}
	report := analyzeProceduralSyncSamples(t, server, coreSamples, recorder)

	t.Logf("=== local LLM 5min report ===")
	t.Logf("steps_run=%d llm_applied=%d llm_fallback=%d samples=%d active_ratio=%.3f max_idle=%s position_range=%.1f hsp_adds=%d",
		len(stepResults), llmApplied, llmFallback, report.SampleCount,
		report.ActiveRatio, report.MaxIdleGap, report.PositionRange, recorder.hspAddCount()-addsBefore)

	logCoverageGaps(t, stepResults)

	if significantTransportErrors(recorder.snapshot().Errors, errorsBefore) > 0 {
		t.Fatalf("transport errors: %v", recorder.snapshot().Errors[errorsBefore:])
	}
	if report.ActiveRatio < e2eLocalLLMMinActiveRatio {
		t.Fatalf("active_ratio=%.3f want>=%.3f — Handy paused during session", report.ActiveRatio, e2eLocalLLMMinActiveRatio)
	}
	if report.MaxIdleGap > e2eLocalLLMMaxIdleGap {
		t.Fatalf("max_idle_gap=%s exceeds %s — Handy must not pause", report.MaxIdleGap, e2eLocalLLMMaxIdleGap)
	}
	if llmApplied == 0 {
		t.Fatal("local LLM never applied motion — check LLM provider and prompt")
	}

	t.Log("stop command")
	addsBeforeStop := recorder.hspAddCount()
	postE2EChatSend(t, server, "stop")
	waitForChaosMotionStopped(t, server, recorder, addsBeforeStop, 15*time.Second)

	t.Logf("E2E local LLM PASSED — %d steps, llm=%d fallback=%d, stop OK", len(stepResults), llmApplied, llmFallback)
}

func requireLocalLLMReady(t *testing.T, server *Server) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/diagnostics/ping-ollama", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Skipf("LLM ping status %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode LLM ping: %v", err)
	}
	connected, _ := payload["ollama_connected"].(bool)
	if !connected {
		errText, _ := payload["ollama_error"].(string)
		t.Skipf("local LLM not available: %s", errText)
	}
	t.Logf("local LLM ready: %v", payload["ollama_model"])
}

func llmMotionUserPrompt(step continuousPhysicsStep, pose string, first bool) string {
	action := "target"
	if first {
		action = "start"
	}
	return fmt.Sprintf(
		"%s motion procedural: action %s, posicao %s, regiao %s, tipo_batida %s, velocidade %d, intensidade %d. Mantenha o movimento continuo sem parar.",
		strings.ToUpper(action[:1])+action[1:],
		action,
		pose,
		step.command.Regiao,
		step.command.TipoBatida,
		step.command.Velocidade,
		step.command.Intensidade,
	)
}

func keysOf(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

func postE2EChatSendMotion(t *testing.T, server *Server, text string, timeout time.Duration) (applied bool, reply string, err error) {
	t.Helper()

	type result struct {
		payload map[string]any
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		recorder := httptest.NewRecorder()
		body := `{"text":` + jsonString(text) + `}`
		request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/send", strings.NewReader(body)))
		request.Header.Set("Content-Type", "application/json")
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			ch <- result{err: fmt.Errorf("status %d: %s", recorder.Code, recorder.Body.String())}
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{payload: payload}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return false, "", res.err
		}
		payload := res.payload
		if ok, _ := payload["ok"].(bool); !ok {
			errText, _ := payload["error"].(string)
			return false, "", fmt.Errorf("chat send failed: %s", errText)
		}
		reply, _ = payload["reply"].(string)
		if dispatch, ok := payload["motion_dispatch"].(map[string]any); ok {
			applied, _ = dispatch["applied"].(bool)
		}
		if !applied {
			if motion, ok := payload["motion"].(map[string]any); ok {
				if action, _ := motion["action"].(string); action == chat.MotionActionStart || action == chat.MotionActionTarget {
					applied = true
				}
			}
		}
		return applied, reply, nil
	case <-time.After(timeout):
		return false, "", fmt.Errorf("LLM timeout after %s", timeout)
	}
}

func waitForChaosMotionStopped(t *testing.T, server *Server, recorder *recordingMotionTransport, addsBefore int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isChatChaosPlayerRunning(server) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if isChatChaosPlayerRunning(server) {
		t.Fatal("player still running after stop")
	}

	holdStart := time.Now()
	for time.Since(holdStart) < 3*time.Second {
		if recorder.hspAddCount() > addsBefore {
			t.Fatalf("HSP adds after stop: %d -> %d", addsBefore, recorder.hspAddCount())
		}
		time.Sleep(300 * time.Millisecond)
	}

	samples := sampleProceduralSync(t, server, 2*time.Second)
	activeAfter := 0
	for _, s := range samples {
		if s.PlaybackActive {
			activeAfter++
		}
	}
	if activeAfter > 1 {
		t.Fatalf("playback active after stop (%d/%d samples)", activeAfter, len(samples))
	}
	t.Log("stop confirmed")
}

func waitForChaosDispatchSettled(
	t *testing.T,
	server *Server,
	recorder *recordingMotionTransport,
	addsBefore int,
	firstStart bool,
) {
	t.Helper()
	waitForChaosStepDispatch(t, server, recorder, addsBefore, firstStart)

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		server.chatChaos.mu.Lock()
		inFlight := server.chatChaos.dispatchInFlight
		server.chatChaos.mu.Unlock()
		if !inFlight && isChatChaosPlayerRunning(server) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !isChatChaosPlayerRunning(server) {
		t.Fatalf("dispatch did not settle (adds=%d plays=%d in_flight=%v)",
			recorder.hspAddCount(), recorder.hspPlayCount(), server.chatChaos.dispatchInFlight)
	}
}
