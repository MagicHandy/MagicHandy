package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLLMLabStrokeModesStartTheirOwnPreviewScore(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{`{"edits":{"groove":{"bottom_at_percent":0,"top_at_percent":100,"feel":"steady"}},"reply":"Full strokes."}`}}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	post := func(path string, body map[string]any) llmLabState {
		t.Helper()
		encoded, _ := json.Marshal(body)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s %d: %s", path, recorder.Code, recorder.Body.String())
		}
		var state llmLabState
		if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	state := post("/api/labs/llm/reset", map[string]any{"method": chat.LabMethodGroove})
	if state.Current.Strokes == nil || state.Current.Gesture != nil {
		t.Fatalf("a stroke mode did not start a stroke score: %+v", state.Current)
	}
	state = post("/api/labs/llm/chat", map[string]any{"message": "Use the whole length", "method": chat.LabMethodGroove,
		"revision": state.Revision, "schema_guided": true})
	if len(state.Turns) != 1 || !state.Turns[0].Valid || state.Current.Strokes.Bottom.AtPercent != 0 || state.Current.Strokes.Top.AtPercent != 100 {
		t.Fatalf("the groove reply was not applied: %+v", state.Turns)
	}
	if len(fake.Commands()) != 0 || server.currentMotionEngine() != nil {
		t.Fatal("a preview reply moved the device")
	}
	state = post("/api/labs/llm/reset", map[string]any{"method": "controls"})
	if state.Current.Strokes != nil {
		t.Fatal("leaving the stroke modes kept a stroke score")
	}
	if !strings.Contains(labContinuationMessage(chat.LabMethodPlainWords, "fallback"), "Plain words can develop") {
		t.Fatal("stroke modes lost their Autopilot continuation")
	}
}

func TestLLMLabCompareRunsOneModeWithoutRecording(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{`{"edits":{"depth":"base","pull_back":"upper"},"reply":"Long strokes to the base."}`}}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	before := server.labState()
	compare := func(method string) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(map[string]any{"message": "Go deeper", "method": method, "schema_guided": true})
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/labs/llm/compare", bytes.NewReader(encoded))))
		return recorder
	}
	recorder := compare(chat.LabMethodPlainWords)
	if recorder.Code != http.StatusOK {
		t.Fatalf("compare %d: %s", recorder.Code, recorder.Body.String())
	}
	var result labCompareResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Trial.Valid || len(result.Trace) != 121 || result.Perceptual.PositionMinPercent > 1 || result.Perceptual.PositionMaxPercent < 60 {
		t.Fatalf("unexpected comparison: valid=%t trace=%d reach=%.1f-%.1f", result.Trial.Valid, len(result.Trace),
			result.Perceptual.PositionMinPercent, result.Perceptual.PositionMaxPercent)
	}
	after := server.labState()
	if after.Revision != before.Revision || len(after.Turns) != 0 || len(fake.Commands()) != 0 || server.currentMotionEngine() != nil {
		t.Fatal("a comparison changed the Lab conversation or moved the device")
	}
	if compare("no_such_mode").Code != http.StatusConflict {
		t.Fatal("an unknown mode was compared")
	}
	server.lab.mu.Lock()
	server.lab.busy = true
	server.lab.mu.Unlock()
	if compare(chat.LabMethodGroove).Code != http.StatusConflict {
		t.Fatal("a comparison ran beside another Lab reply")
	}
}
