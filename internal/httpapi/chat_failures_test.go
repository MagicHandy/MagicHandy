package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestCreativeV2FailureRetainsRedactedOutputAndRealContract(t *testing.T) {
	const secret = "local-fixture-private-key"
	raw := `{"edits":[{"speed_percent":30}],"reply":"` + secret + `"}`
	provider := &scriptedLLMProvider{responses: []string{raw}}
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		s.Device.HandyConnectionKey = secret
		return s
	})
	response := postChatStream(t, server, `{"message":"Make it slightly faster."}`)
	if !strings.Contains(response, "action none, start or update") || len(fake.Commands()) != 0 {
		t.Fatal("invalid action escaped validation")
	}
	sessionID, _ := server.chatLog.ActiveSessionID()
	items, err := server.chatLog.Failures(t.Context(), sessionID)
	if err != nil || len(items) != 1 || items[0].Details != nil {
		t.Fatalf("retained metadata: %+v, %v", items, err)
	}
	path := fmt.Sprintf("/api/diagnostics/chat-failures/%d?session_id=%s", items[0].ID, sessionID)
	read := func(path string) string {
		t.Helper()
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read: %d %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if strings.Contains(read(path), "raw_response") {
		t.Fatal("default report includes private content")
	}
	full := read(path + "&include_content=true")
	if strings.Contains(full, secret) {
		t.Fatal("credential retained or exported")
	}
	var retained chat.RequestFailure
	if err := json.Unmarshal([]byte(full), &retained); err != nil {
		t.Fatal(err)
	}
	if retained.Details == nil || !strings.Contains(retained.Details.Raw, "[redacted]") || !strings.Contains(retained.Details.Prompt, "LIVE CHAT DECISION") || !strings.Contains(retained.Details.Schema, "action") {
		t.Fatalf("incomplete evidence: %+v", retained)
	}
	if retained.Diagnostics.ProviderCalls != 1 || provider.callCount() != 1 {
		t.Fatal("failure diagnostics generated another response")
	}
}
