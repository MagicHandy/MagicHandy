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
	replacement := `{"edits":[{"speed_percent":35}],"reply":"Still invalid: ` + secret + `"}`
	provider := &scriptedLLMProvider{responses: []string{raw, replacement}}
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(s config.Settings) config.Settings {
		s.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
		s.Device.HandyConnectionKey = secret
		return s
	})
	response := postChatStream(t, server, `{"message":"Make it slightly faster."}`)
	if !strings.Contains(response, "action none, start or update") || strings.Contains(response, "event: message") || len(fake.Commands()) != 0 {
		t.Fatal("either invalid attempt escaped validation or hid the action failure")
	}
	assertOnlyUserChatHistory(t, server)
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
	assertCreativeRepairRedaction(t, retained, raw, replacement, secret)
	if retained.Diagnostics.ProviderCalls != 2 || provider.callCount() != 2 {
		t.Fatal("failure diagnostics lost the bounded correction count or generated another response")
	}
}

func assertCreativeRepairRedaction(t *testing.T, retained chat.RequestFailure, raw, replacement, secret string) {
	t.Helper()
	if retained.Details == nil || retained.Details.Raw != strings.ReplaceAll(raw, secret, "[redacted]") ||
		retained.Details.RepairRaw != strings.ReplaceAll(replacement, secret, "[redacted]") ||
		!strings.Contains(retained.Details.InitialError, "action none, start or update") ||
		!strings.Contains(retained.Details.Prompt, "LIVE CHAT DECISION") || !strings.Contains(retained.Details.Schema, "action") {
		t.Fatalf("incomplete or unredacted correction evidence: %+v", retained)
	}
	if retained.Kind != "response_repair" || retained.RepairOutcome != "failed" || retained.Diagnostics.Repaired {
		t.Fatalf("failed correction recorded as success: %+v", retained)
	}
}
