package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/persona"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func diagnosticFiles(t *testing.T, server *Server, query string) map[string]string {
	t.Helper()
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/diagnostics/report"+query, nil))
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/zip" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("download failed: %d %s", w.Code, w.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]string)
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
			if line != "" && !json.Valid([]byte(line)) {
				t.Fatalf("invalid JSON in %s", file.Name)
			}
		}
		files[file.Name] = string(body)
	}
	return files
}

func seedDiagnosticReport(t *testing.T, server *Server) (string, string) {
	t.Helper()
	const deviceSecret, cloudSecret = "device-export-private", "cloud\"export+private/value"
	if err := server.cloudPlanning.auth.SetAPIKey(t.Context(), cloudSecret); err != nil {
		t.Fatal(err)
	}
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Device.HandyConnectionKey = deviceSecret
		settings.LLM.Model = "local-" + deviceSecret
		settings.LLM.PersonaDescription = "DEFAULT_PERSONA_CONTENT"
		settings.LLM.CustomAnatomy = "CUSTOM_ANATOMY_CONTENT"
		settings.Voice.TTSReferenceText = "VOICE_REFERENCE_CONTENT"
		settings.Voice.TTSTonePrompt = "VOICE_PROMPT_CONTENT"
		return settings
	})
	name, description, lore := "PERSONA_NAME", "PERSONA_DESCRIPTION", "PERSONA_LORE"
	p, err := server.personas.Create(t.Context(), persona.Draft{Name: &name, Description: &description})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.personas.CreateLore(t.Context(), p.ID, persona.LoreDraft{Text: &lore}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.personalization.prompts.Create("CUSTOM_PROMPT_NAME", "CUSTOM_PROMPT_TEXT "+cloudSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := server.personalization.memory.Add("SAVED_MEMORY_TEXT " + deviceSecret); err != nil {
		t.Fatal(err)
	}
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.chatLog.AppendTo(sessionID, "user", "SUCCESSFUL_CHAT_TEXT", "private-browser-id", nil); err != nil {
		t.Fatal(err)
	}
	_, err = server.chatLog.SaveFailure(t.Context(), sessionID, chat.RequestFailure{Error: "missing action " + deviceSecret,
		Diagnostics: chat.MessageDiagnostics{PersonaName: name, PersonaID: "FAILURE_PERSONA_ID"},
		Details:     &chat.FailureContent{Raw: "FAILED_MODEL_TEXT " + cloudSecret, RepairRaw: "FAILED_REPAIR_TEXT", Prompt: "CAPTURED_SYSTEM_PROMPT", Schema: "CAPTURED_SCHEMA"}})
	if err != nil {
		t.Fatal(err)
	}
	server.traces.Add(diagnostics.MotionTraceRow{Source: "export-test", Reason: "trace-example", Annotation: "TRACE_ANNOTATION " + cloudSecret})
	_, document, ok, err := boundedMotionTraceArchive(server.currentTraceExport(), 1, "fixture")
	if err != nil || !ok {
		t.Fatal("trace fixture", err)
	}
	server.writeLastMotionTrace(t.Context(), document)
	return deviceSecret, cloudSecret
}

func TestDiagnosticReportSelectionsAndSecretRedaction(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{}
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	deviceSecret, cloudSecret := seedDiagnosticReport(t, server)
	defaults := diagnosticFiles(t, server, "")
	for _, file := range []string{"status.json", "settings.json", "motion/live-trace.json", "motion/last-stopped-trace.json", "chat/failures.json", "manifest.json"} {
		if _, ok := defaults[file]; !ok {
			t.Errorf("default report omitted %s", file)
		}
	}
	assertDiagnosticExcludes(t, defaults, deviceSecret, cloudSecret, "SUCCESSFUL_CHAT_TEXT", "FAILED_MODEL_TEXT", "CAPTURED_SYSTEM_PROMPT", "PERSONA_NAME", "FAILURE_PERSONA_ID", "PERSONA_DESCRIPTION", "PERSONA_LORE", "DEFAULT_PERSONA_CONTENT", "CUSTOM_ANATOMY_CONTENT", "VOICE_REFERENCE_CONTENT", "VOICE_PROMPT_CONTENT", "CUSTOM_PROMPT_TEXT", "SAVED_MEMORY_TEXT", "private-browser-id")
	if !strings.Contains(defaults["chat/failures.json"], "missing action [redacted]") {
		t.Fatal("failure evidence was not retained safely")
	}

	full := diagnosticFiles(t, server, "?chat=full&model_output=true&personas=true&prompts=true&memories=true")
	for file, marker := range map[string]string{"personas.json": "PERSONA_LORE", "prompts.json": "CUSTOM_PROMPT_TEXT", "memories.json": "SAVED_MEMORY_TEXT", "chat/failures.json": "CAPTURED_SYSTEM_PROMPT", "chat/history-1.jsonl": "SUCCESSFUL_CHAT_TEXT"} {
		if !strings.Contains(full[file], marker) {
			t.Errorf("selected content missing from %s", file)
		}
	}
	if !strings.Contains(full["chat/failures.json"], "FAILED_MODEL_TEXT") || !strings.Contains(full["personas.json"], "DEFAULT_PERSONA_CONTENT") || !strings.Contains(full["prompts.json"], "VOICE_REFERENCE_CONTENT") {
		t.Fatal("independent captured content missing")
	}
	assertDiagnosticExcludes(t, full, deviceSecret, cloudSecret, url.QueryEscape(cloudSecret), "private-browser-id")
	if !strings.Contains(full["chat/failures.json"], "PERSONA_NAME") || !strings.Contains(full["chat/failures.json"], "FAILURE_PERSONA_ID") {
		t.Fatal("selected persona metadata missing from failures")
	}
	for _, query := range []string{"?model_output=true", "?prompts=true"} {
		files := diagnosticFiles(t, server, query)
		if strings.Contains(files["chat/failures.json"], "FAILED_MODEL_TEXT") != (query == "?model_output=true") || strings.Contains(files["chat/failures.json"], "CAPTURED_SYSTEM_PROMPT") != (query == "?prompts=true") {
			t.Fatalf("content toggles coupled: %s", query)
		}
	}
	minimal := diagnosticFiles(t, server, "?settings=false&traces=false&chat=none&model_output=true")
	if len(minimal) != 2 || minimal["status.json"] == "" || minimal["manifest.json"] == "" || strings.Contains(fmt.Sprint(minimal), "TRACE_ANNOTATION") {
		t.Fatal("disabled sections were exported")
	}
	if len(fake.Commands()) != 0 || provider.callCount() != 0 {
		t.Fatal("diagnostic export commanded motion or called a model")
	}
}

func assertDiagnosticExcludes(t *testing.T, files map[string]string, markers ...string) {
	t.Helper()
	joined := fmt.Sprint(files)
	for _, marker := range markers {
		encoded, err := json.Marshal(marker)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(joined, marker) || strings.Contains(joined, string(encoded[1:len(encoded)-1])) {
			t.Errorf("report leaked excluded content: %s", marker)
		}
	}
}

func TestDiagnosticFullHistoryIncludesEveryRetainedSessionBeyondDisplayPage(t *testing.T) {
	server := newTestServer(t)
	session, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	for i := range 233 {
		if _, err := server.chatLog.AppendTo(session, "user", fmt.Sprintf("message-%03d café 中文", i), "browser", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := server.chatLog.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	second, err := server.chatLog.CreateSession(false)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.TrimSpace(strings.Repeat("Complete retained text. ", 200))
	if _, err := server.chatLog.AppendTo(second.ID, "user", content, "browser", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := server.chatLog.AppendPendingAssistantTo(second.ID, "UNCOMMITTED_OUTPUT", nil); err != nil {
		t.Fatal(err)
	}
	files := diagnosticFiles(t, server, "?chat=full")
	if len(strings.Split(strings.TrimSpace(files["chat/history-1.jsonl"]), "\n")) != 233 || !strings.Contains(files["chat/history-1.jsonl"], "message-000 café 中文") || !strings.Contains(files["chat/history-1.jsonl"], "message-232 café 中文") {
		t.Fatal("full export inherited display pagination or lost UTF-8")
	}
	if !strings.Contains(files["chat/history-2.jsonl"], content) || strings.Contains(fmt.Sprint(files), "UNCOMMITTED_OUTPUT") {
		t.Fatal("unsaved committed history was truncated, or pending text escaped")
	}
}

func TestDiagnosticReportRejectsInvalidOptionsAndIncompleteArchive(t *testing.T) {
	server := newTestServer(t)
	for _, query := range []string{"?chat=all", "?traces=yes", "?secret=true", "?chat=full&chat=none"} {
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/diagnostics/report"+query, nil))
		if w.Code != http.StatusBadRequest || w.Header().Get("Content-Disposition") != "" {
			t.Fatalf("invalid option exported a report: %s", query)
		}
	}
	sessionID, _ := server.chatLog.ActiveSessionID()
	id, err := server.chatLog.SaveFailure(t.Context(), sessionID, chat.RequestFailure{Error: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.Datastore().SQL().ExecContext(t.Context(), `UPDATE chat_failures SET details = '{"raw_response":{"private":"broken private content"}}' WHERE seq = ?`, id); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/diagnostics/report?model_output=true", nil))
	if w.Code != http.StatusInternalServerError || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), "broken private content") {
		t.Fatal("failed archive was downloaded or revealed private data")
	}
}

func TestDiagnosticArchiveBoundsAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		var buffer bytes.Buffer
		archive := &diagnosticArchive{zip: zip.NewWriter(&buffer), ctx: ctx, remaining: 10, redact: func(value string) string { return value }}
		if canceled {
			cancel()
		}
		err := archive.addJSON("fixture.json", map[string]string{"value": "too much content"})
		cancel()
		_ = archive.zip.Close()
		if canceled && !errors.Is(err, context.Canceled) || !canceled && !errors.Is(err, errDiagnosticReportTooLarge) {
			t.Fatal("archive did not enforce its budget or cancellation", err)
		}
	}
}
