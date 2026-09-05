package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestLLMLabConversationIsSeparateAndPreviewOnly(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{`{"reply":"Tip anchor in preview.","controls":{"anchor_percent":100}}`, `{"reply":"Invalid speed.","controls":{"speed_percent":101}}`}}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	state := server.labState()
	beforeLog := httptest.NewRecorder()
	server.Handler().ServeHTTP(beforeLog, httptest.NewRequest(http.MethodGet, "/api/chat/messages", nil))
	for index := 0; index < 2; index++ {
		encoded, _ := json.Marshal(map[string]any{"message": "Test the next proposal", "method": "controls", "revision": state.Revision, "schema_guided": true})
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/labs/llm/chat", bytes.NewReader(encoded))))
		if recorder.Code != http.StatusOK {
			t.Fatalf("chat %d: %s", recorder.Code, recorder.Body.String())
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		if state.Current.AnchorPercent != 100 || state.Current.SpeedPercent != 25 || state.Turns[index].ProviderCalls != 1 {
			t.Fatalf("unexpected lab state: %+v", state)
		}
	}
	if !state.Turns[0].Valid || state.Turns[1].Valid || state.Turns[1].Raw == "" {
		t.Fatal("failed output was repaired, dropped or applied")
	}
	if state.Turns[1].Changed == nil || len(state.Turns[1].Changed) != 0 {
		t.Fatal("rejected proposal must expose an empty change list for the UI")
	}
	afterLog := httptest.NewRecorder()
	server.Handler().ServeHTTP(afterLog, httptest.NewRequest(http.MethodGet, "/api/chat/messages", nil))
	if beforeLog.Body.String() != afterLog.Body.String() || len(fake.Commands()) != 0 || server.currentMotionEngine() != nil {
		t.Fatal("lab changed production history or motion")
	}
}

func TestLLMLabStopCancelsAndDiscardsLateReply(t *testing.T) {
	fake := transport.NewFake()
	provider := &blockingLLMProvider{started: make(chan struct{})}
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake, LLMProvider: provider})
	result := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/labs/llm/chat", bytes.NewBufferString(`{"method":"controls","message":"Change range","revision":0}`))))
		result <- recorder.Code
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("model request did not start")
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	select {
	case code := <-result:
		if code != http.StatusConflict {
			t.Fatalf("late reply: %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the lab")
	}
	state := server.labState()
	if len(state.Turns) != 0 || state.Revision != 0 || state.Busy {
		t.Fatal("canceled lab work changed its conversation")
	}
}

func TestFlowAuditionUsesSharedEngine(t *testing.T) {
	fake := transport.NewFake()
	server := newEnabledLabServer(t, Runtime{Transport: fake, MotionTransport: fake})
	settings, _ := server.store.Snapshot()
	spec := motion.DefaultFlowSpec()
	encoded, _ := json.Marshal(motionRequest{Lab: &motion.LabStart{Method: "flow", Flow: &spec, SettingsKey: motion.LabSettingsKey(settings.Motion)}})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodPost, "/api/motion/start", bytes.NewReader(encoded))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("flow start %d: %s", recorder.Code, recorder.Body.String())
	}
	engine := server.currentMotionEngine()
	if engine == nil {
		t.Fatal("flow did not use the shared engine")
	}
	snapshot := engine.Snapshot()
	if !snapshot.Running || snapshot.Target.Dynamic != nil || snapshot.Target.PatternName != "Continuous flow" || len(fake.Commands()) == 0 {
		t.Fatalf("flow was replaced by legacy content: %+v", snapshot.Target)
	}
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
	if engine.Snapshot().Running {
		t.Fatal("flow survived Stop")
	}
}
