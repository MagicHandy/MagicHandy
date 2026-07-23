//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chatauto"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	e2eRoteiroOralJSON  = `{"autodom":{"humor":"tesao","posicao":"oral","intensidade":8,"velocidade":7}}`
	e2eRoteiroDeepJSON  = `{"autodom":{"humor":"intensa","posicao":"deepthroat","intensidade":9,"velocidade":8}}`
	e2eRoteiroRideJSON  = `{"autodom":{"humor":"intensa","posicao":"cavalgando","intensidade":7,"velocidade":6}}`
	e2eAutoReplyOralJSON = `{"reply":"Vem sentir minha boca devagar, amor."}`
	e2eAutoReplyDeepJSON = `{"reply":"Agora mais fundo, sem pressa."}`
	e2eAutoReplyRideJSON = `{"reply":"Sobe em mim e deixa eu marcar o ritmo."}`
	e2eAutoUserMessage   = "continua assim, quero sentir na cabeca"
)

// TestE2EChatAutoRoteiroOnRealDevice mirrors the UI flow:
// LLM roteiro → Chat Auto motion → user message via /api/chat/send → assistant reply in /api/chat/messages.
func TestE2EChatAutoRoteiroOnRealDevice(t *testing.T) {
	provider := &scriptedLLMProvider{responses: e2eChatAutoScriptedResponses()}
	server := newLiveSQLiteTestServerWithRuntime(t, Runtime{
		Traces:      diagnostics.NewTraceRing(128),
		LLMProvider: provider,
	})
	recorder := installE2ERecordingTransport(t, server)

	settings, _ := server.store.Snapshot()
	if strings.TrimSpace(settings.Device.HandyConnectionKey) == "" {
		t.Skip("no handy_connection_key in SQLite store — configure device in Settings first")
	}
	requireLiveDeviceFromStore(t, server)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		settings.Motion.HardwareSafetyLock = false
		settings.Motion.StrokeMinPercent = 0
		settings.Motion.StrokeMaxPercent = 100
		settings.AutoDom.WaitForUserMessage = boolPtr(false)
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
	putE2EOperationMode(t, server, "auto")

	addsBefore := recorder.hspAddCount()
	waitForChatAutoMotion(t, server, recorder, provider, addsBefore, 45*time.Second)

	status := fetchE2EStatus(t, server)
	chatAuto, _ := status["chat_auto"].(map[string]any)
	if chatAuto == nil {
		t.Fatalf("status missing chat_auto: %#v", status)
	}
	if active, _ := chatAuto["active"].(bool); !active {
		t.Fatalf("chat_auto.active = false, want true")
	}
	if pos, _ := chatAuto["posicao"].(string); pos != string(chatauto.PoseOral) {
		t.Fatalf("chat_auto.posicao = %q, want %q", pos, chatauto.PoseOral)
	}
	motionState, _ := chatAuto["motion"].(map[string]any)
	if motionState == nil {
		t.Fatal("chat_auto.motion missing")
	}
	if regiao, _ := motionState["regiao"].(string); regiao != "cabeca" {
		t.Fatalf("chat_auto.motion.regiao = %q, want cabeca (oral roteiro)", regiao)
	}

	assertE2EHSPRegion(t, recorder, addsBefore, "cabeca", 8)

	t.Log("UI flow: POST /api/chat/send (user message)")
	messagesBefore := len(fetchE2EChatMessages(t, server))
	postE2EChatSend(t, server, e2eAutoUserMessage)

	waitForAssistantMessages(t, server, messagesBefore+1, 60*time.Second)
	messages := fetchE2EChatMessages(t, server)
	lastAssistant := lastAssistantMessage(messages)
	if lastAssistant == "" {
		t.Fatal("expected assistant reply in /api/chat/messages after user send")
	}
	if !strings.Contains(strings.ToLower(lastAssistant), "boca") &&
		!strings.Contains(strings.ToLower(lastAssistant), "fundo") &&
		!strings.Contains(strings.ToLower(lastAssistant), "ritmo") {
		t.Logf("assistant reply = %q (scripted LLM)", lastAssistant)
	}

	t.Log("holding 12s — sampling motion visual for cabeca envelope")
	samples := sampleProceduralSync(t, server, 12*time.Second)
	minPos, maxPos := 100.0, 0.0
	activeSamples := 0
	for _, sample := range samples {
		if !sample.PlaybackActive && sample.PositionPct <= 0 {
			continue
		}
		activeSamples++
		if sample.PositionPct < minPos {
			minPos = sample.PositionPct
		}
		if sample.PositionPct > maxPos {
			maxPos = sample.PositionPct
		}
	}
	if activeSamples < 8 {
		t.Fatalf("active visual samples = %d, want continuous motion during hold", activeSamples)
	}
	if minPct, maxPct, ok := motion.RegionBounds("cabeca"); ok {
		if maxPos < float64(minPct)-10 {
			t.Errorf("visual max=%.1f want >=%d (cabeca oral roteiro)", maxPos, minPct)
		}
		if minPos > float64(maxPct)+10 {
			t.Errorf("visual min=%.1f want <=%d", minPos, maxPct)
		}
	}
	t.Logf("cabeca visual range %.1f..%.1f over %d active samples", minPos, maxPos, activeSamples)

	assertRecorderTransportOKSince(t, recorder, 0)
	t.Log("E2E chat auto roteiro PASSED — LLM roteiro, UI chat reply, cabeca motion validated")
}

func e2eChatAutoScriptedResponses() []string {
	base := []string{
		e2eRoteiroOralJSON,
		e2eAutoReplyOralJSON,
		e2eRoteiroDeepJSON,
		e2eAutoReplyDeepJSON,
		e2eRoteiroRideJSON,
		e2eAutoReplyRideJSON,
	}
	out := make([]string, 0, len(base)*6)
	for i := 0; i < 6; i++ {
		out = append(out, base...)
	}
	return out
}

func boolPtr(v bool) *bool {
	return &v
}

func putE2EOperationMode(t *testing.T, server *Server, mode string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	body := `{"updates":{"app":{"operation_mode":"` + mode + `"}}}`
	request := withController(httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(lsoUIHeader, "lso")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT operation_mode=%s status=%d: %s", mode, recorder.Code, recorder.Body.String())
	}
}

func postE2EChatSend(t *testing.T, server *Server, text string) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	body := `{"text":` + jsonString(text) + `}`
	request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/send", strings.NewReader(body)))
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/chat/send status=%d: %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chat send: %v", err)
	}
	return payload
}

func fetchE2EStatus(t *testing.T, server *Server) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/status status=%d", recorder.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return payload
}

func fetchE2EChatMessages(t *testing.T, server *Server) []chatMessageRecord {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/chat/messages", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/chat/messages status=%d", recorder.Code)
	}
	var payload struct {
		Messages []chatMessageRecord `json:"messages"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	return payload.Messages
}

func lastAssistantMessage(messages []chatMessageRecord) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func waitForChatAutoMotion(t *testing.T, server *Server, recorder *recordingMotionTransport, provider *scriptedLLMProvider, addsBefore int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := fetchE2EStatus(t, server)
		chatAuto, _ := status["chat_auto"].(map[string]any)
		if chatAuto != nil {
			active, _ := chatAuto["active"].(bool)
			motionState, _ := chatAuto["motion"].(map[string]any)
			regiao, _ := motionState["regiao"].(string)
			if errText, _ := chatAuto["error"].(string); errText != "" {
				t.Fatalf("chat_auto error: %s", errText)
			}
			snap := recorder.snapshot()
			if active && regiao == "cabeca" && len(snap.HSPAdds) > addsBefore && len(snap.HSPPlays) > 0 {
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	status := fetchE2EStatus(t, server)
	t.Fatalf("timed out waiting for chat_auto cabeca motion (status chat_auto=%v adds=%d plays=%d llm_calls=%d)",
		status["chat_auto"], recorder.hspAddCount(), recorder.hspPlayCount(), provider.callCount())
}

func waitForAssistantMessages(t *testing.T, server *Server, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(fetchE2EChatMessages(t, server)) >= want {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d chat messages", want)
}

func assertE2EHSPRegion(t *testing.T, recorder *recordingMotionTransport, addsBefore int, regiao string, tolerance int) {
	t.Helper()
	batch := newestHSPAddPoints(recorder, addsBefore)
	if len(batch) == 0 {
		t.Fatal("expected HSP dispatch batch after roteiro motion")
	}
	minPos, maxPos := batch[0].PositionPercent, batch[0].PositionPercent
	for _, point := range batch {
		if point.PositionPercent < minPos {
			minPos = point.PositionPercent
		}
		if point.PositionPercent > maxPos {
			maxPos = point.PositionPercent
		}
	}
	minPct, maxPct, ok := motion.RegionBounds(regiao)
	if !ok {
		return
	}
	if maxPos < minPct-tolerance || minPos > maxPct+tolerance {
		t.Fatalf("roteiro HSP dispatch %d..%d outside regiao %s %d..%d",
			minPos, maxPos, regiao, minPct, maxPct)
	}
	t.Logf("roteiro HSP dispatch %d..%d matches regiao %s", minPos, maxPos, regiao)
}
