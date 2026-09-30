package httpapi

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/persona"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func postModelCheck(t *testing.T, server *Server, method string, controlled bool) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/api/llm/model-check", nil)
	if controlled {
		request = withController(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func waitForModelCheck(t *testing.T, server *Server, state string) modelCheckReport {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/llm/model-check", nil))
		var report modelCheckReport
		decodeResponse(t, recorder, &report)
		if report.State == state {
			return report
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("model check never reached %q: %+v", state, server.modelCheck.snapshot())
	return modelCheckReport{}
}

func TestModelCheckRunsTheScriptedTurnsWithoutMovingTheDevice(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Better now.","motion":{"action":"none"}}`,
		`{"reply":"Slowly.","motion":{"action":"start","pattern_id":"flow-pace-wave","speed_percent":20}}`,
		`{"reply":"All of it.","motion":{"action":"none"}}`,
		`{"reply":"Filthy things.","motion":{"action":"none"}}`,
		`{"reply":"So good.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})

	if recorder := postModelCheck(t, server, http.MethodPost, false); recorder.Code != http.StatusConflict {
		t.Fatalf("uncontrolled start = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if recorder := postModelCheck(t, server, http.MethodPost, true); recorder.Code != http.StatusAccepted {
		t.Fatalf("start = %d: %s", recorder.Code, recorder.Body.String())
	}
	report := waitForModelCheck(t, server, modelCheckComplete)
	if len(report.Turns) != 5 || report.Summary.FirstTry != 5 || report.Error != "" {
		t.Fatalf("report = %+v", report)
	}
	if report.Turns[1].Motion == "" {
		t.Fatalf("the start request's motion was not summarized: %+v", report.Turns[1])
	}
	verdict := map[string]string{}
	for _, item := range report.Items {
		verdict[item.ID] = item.Status
	}
	for _, id := range []string{modelCheckItemLoad, chat.ModelCheckReasoning, chat.ModelCheckContract, chat.ModelCheckTruncated} {
		if verdict[id] != chat.ModelCheckPass {
			t.Fatalf("%s = %q, want pass: %+v", id, verdict[id], report.Items)
		}
	}
	// An injected provider is not the managed runner, so no host-only items.
	if _, ok := verdict[modelCheckItemGPU]; ok {
		t.Fatalf("host items reported for an injected provider: %+v", report.Items)
	}
	if commands := fake.Commands(); len(commands) != 0 {
		t.Fatalf("the model check moved the device: %+v", commands)
	}
}

func TestModelCheckRejectsASecondRunAndCanBeCanceled(t *testing.T) {
	provider := &blockingLLMProvider{started: make(chan struct{})}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	if recorder := postModelCheck(t, server, http.MethodPost, true); recorder.Code != http.StatusAccepted {
		t.Fatalf("start = %d: %s", recorder.Code, recorder.Body.String())
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the check never reached the model")
	}
	if recorder := postModelCheck(t, server, http.MethodPost, true); recorder.Code != http.StatusConflict {
		t.Fatalf("second start = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if recorder := postModelCheck(t, server, http.MethodDelete, true); recorder.Code != http.StatusOK {
		t.Fatalf("cancel = %d: %s", recorder.Code, recorder.Body.String())
	}
	waitForModelCheck(t, server, modelCheckCanceled)
}

// httpAPIGemma4GGUF is a Gemma 4 GGUF with no chat template, the simplest
// shape MagicHandy offers its template fix for.
func httpAPIGemma4GGUF() []byte {
	payload := make([]byte, 0, 4096)
	appendUint32 := func(value uint32) { payload = binary.LittleEndian.AppendUint32(payload, value) }
	appendUint64 := func(value uint64) { payload = binary.LittleEndian.AppendUint64(payload, value) }
	appendString := func(value string) {
		appendUint64(uint64(len(value)))
		payload = append(payload, value...)
	}
	payload = append(payload, "GGUF"...)
	appendUint32(3)
	appendUint64(1)
	appendUint64(2)
	appendString("general.architecture")
	appendUint32(8)
	appendString("gemma4")
	appendString("gemma4.context_length")
	appendUint32(4)
	appendUint32(4096)
	return append(payload, make([]byte, 4096-len(payload))...)
}

func importHTTPAPIModel(t *testing.T, server *Server, data []byte) llm.ModelRecord {
	t.Helper()
	source := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := server.models.StartGGUFImport(source, "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	job = waitForAPIImport(t, server, job.ID)
	record, err := server.models.Model(context.Background(), job.ModelID)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestTemplateFixRouteTogglesTheOfferedFixAndFeedsTheRunner(t *testing.T) {
	server := newTestServer(t)
	record := importHTTPAPIModel(t, server, httpAPIGemma4GGUF())
	if record.TemplateFixOffer != llm.TemplateFixGemma4CloseThinking {
		t.Fatalf("record = %+v, want an offered fix", record)
	}
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.Provider, settings.LLM.LlamaCPPMode, settings.LLM.Model = config.LLMProviderLlamaCPP, config.LlamaCPPModeManaged, record.ID
		return settings
	})
	writeHTTPAPIManagedRuntime(t, server.store.DataDir())
	settings, _ := server.store.Snapshot()
	before, err := server.managedProviderPaths(context.Background(), settings.LLM)
	if err != nil || before.template != "" {
		t.Fatalf("paths before the fix = %+v, %v", before, err)
	}

	toggle := func(path string, body map[string]any) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(jsonAPIRequest(t, http.MethodPost, path, body)))
		return recorder
	}
	recorder := toggle("/api/llm/models/"+record.ID+"/template-fix", map[string]any{"enabled": true})
	if recorder.Code != http.StatusOK {
		t.Fatalf("enable = %d: %s", recorder.Code, recorder.Body.String())
	}
	var enabled llm.ModelRecord
	decodeResponse(t, recorder, &enabled)
	if enabled.TemplateFix != llm.TemplateFixGemma4CloseThinking || enabled.TemplateFixSource != llm.TemplateFixSourceUser {
		t.Fatalf("enabled = %+v", enabled)
	}
	after, err := server.managedProviderPaths(context.Background(), settings.LLM)
	if err != nil || !strings.HasSuffix(after.template, llm.TemplateFixGemma4CloseThinking+".jinja") || after.key == before.key {
		t.Fatalf("paths after the fix = %+v, %v", after, err)
	}

	for name, test := range map[string]struct {
		path string
		body map[string]any
		want int
	}{
		"missing flag":  {"/api/llm/models/" + record.ID + "/template-fix", map[string]any{}, http.StatusBadRequest},
		"unknown model": {"/api/llm/models/missing-model/template-fix", map[string]any{"enabled": true}, http.StatusNotFound},
	} {
		if recorder := toggle(test.path, test.body); recorder.Code != test.want {
			t.Fatalf("%s = %d, want %d: %s", name, recorder.Code, test.want, recorder.Body.String())
		}
	}
	llama := importHTTPAPIModel(t, server, httpAPITestGGUF())
	if recorder := toggle("/api/llm/models/"+llama.ID+"/template-fix", map[string]any{"enabled": true}); recorder.Code != http.StatusConflict {
		t.Fatalf("fix for a model without one = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if recorder := toggle("/api/llm/models/"+record.ID+"/template-fix", map[string]any{"enabled": false}); recorder.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPersonaReplyLengthOverridesSettings(t *testing.T) {
	settings := config.DefaultSettings().LLM
	settings.ChatVoice, settings.ReplyLength, settings.MaxOutputTokens = config.LLMChatVoiceExplicit, config.LLMReplyLengthShort, 256
	if got := chatCapabilities(settings, nil).ReplyLength; got != chat.ReplyLengthShort {
		t.Fatalf("settings reply length = %q", got)
	}
	detailed := &persona.Persona{ChatVoice: config.LLMChatVoiceExplicit, ReplyLength: config.LLMReplyLengthDetailed}
	if got := chatCapabilities(settings, detailed).ReplyLength; got != chat.ReplyLengthDetailed {
		t.Fatalf("persona override = %q", got)
	}
	if got := settings.ChatMaxOutputTokens(effectiveReplyLength(settings, detailed)); got != config.DetailedReplyMinOutputTokens {
		t.Fatalf("detailed persona budget = %d", got)
	}
	follows := &persona.Persona{ChatVoice: config.LLMChatVoiceExplicit}
	if got := chatCapabilities(settings, follows).ReplyLength; got != chat.ReplyLengthShort {
		t.Fatalf("a persona without an override = %q, want the settings length", got)
	}
}
