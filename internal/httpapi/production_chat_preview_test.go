package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestProductionChatPreviewUsesLiveContractWithoutPublishingOrMotion(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{`{"action":"start","edits":[{"speed_percent":25,"variation_percent":40}],"reply":"Starting a varied phrase."}`}}
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		s.LLM.ReasoningMode = config.LLMReasoningAuto
		return s
	})
	w := httptest.NewRecorder()
	before := server.currentMotionEngine()
	server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/diagnostics/chat-test", strings.NewReader(`{"message":"Begin moving with more variation."}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Valid  bool                `json:"valid"`
		Report chat.RequestFailure `json:"report"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Report.ReasoningMode != config.LLMReasoningAuto || result.Report.Details == nil || !strings.Contains(result.Report.Details.Prompt, "LIVE CHAT DECISION") || !strings.Contains(result.Report.Details.Schema, "action") {
		t.Fatalf("preview mismatch: %+v", result)
	}
	if len(fake.Commands()) > 0 || server.currentMotionEngine() != before || provider.callCount() != 1 {
		t.Fatal("preview dispatched motion or ran another generation")
	}
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 0 {
		t.Fatal("preview changed conversation history")
	}
}

func TestProductionChatPreviewIsCanceledByStop(t *testing.T) {
	provider := &blockingLLMProvider{started: make(chan struct{})}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/diagnostics/chat-test", strings.NewReader(`{"message":"Hello"}`)))
		close(done)
	}()
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("preview did not start")
	}
	stop := httptest.NewRecorder()
	server.Handler().ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not cancel preview generation")
	}
	if w.Code != http.StatusConflict || stop.Code != http.StatusOK {
		t.Fatalf("preview=%d Stop=%d", w.Code, stop.Code)
	}
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 0 {
		t.Fatal("canceled preview entered the chat")
	}
}
