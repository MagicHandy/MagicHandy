package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type scriptedLLMProvider struct {
	mu        sync.Mutex
	responses []string
	requests  []llm.ChatRequest
}

func (p *scriptedLLMProvider) StreamChat(_ context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		p.mu.Unlock()
		return "", errors.New("scripted response missing")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	p.mu.Unlock()

	midpoint := len(response) / 2
	if onDelta != nil {
		if midpoint > 0 {
			if err := onDelta(response[:midpoint]); err != nil {
				return response, err
			}
		}
		if err := onDelta(response[midpoint:]); err != nil {
			return response, err
		}
	}
	return response, nil
}

func (p *scriptedLLMProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Provider: "scripted", Available: true}
}

func (p *scriptedLLMProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func TestChatStreamRepairsMalformedJSONAndReportsIndicator(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		"not json",
		`{"reply":"Fixed.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	settings, _ := server.store.Snapshot()
	settings.LLM.ReasoningMode = config.LLMReasoningAuto
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("save automatic reasoning settings: %v", err)
	}

	body := postChatStream(t, server, `{"message":"hello"}`)
	if !strings.Contains(body, "event: malformed") {
		t.Fatalf("chat stream missing malformed event:\n%s", body)
	}
	if !strings.Contains(body, `"repaired":true`) {
		t.Fatalf("chat stream missing repaired indicator:\n%s", body)
	}
	if !strings.Contains(body, `"reply":"Fixed."`) {
		t.Fatalf("chat stream missing repaired reply:\n%s", body)
	}
	if provider.callCount() != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.callCount())
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	initial, repair := provider.requests[0], provider.requests[1]
	if initial.MaxTokens != 256 || initial.ReasoningMode != "auto" || initial.ReasoningBudgetTokens != 0 {
		t.Fatalf("initial generation settings = %+v", initial)
	}
	if repair.MaxTokens != 256 || repair.ReasoningMode != "off" || repair.ReasoningBudgetTokens != 0 {
		t.Fatalf("repair generation settings = %+v", repair)
	}
}

func TestManagedLlamaReasoningBudgetRequiresCurrentPinnedRuntime(t *testing.T) {
	settings := config.DefaultSettings().LLM
	settings.ReasoningMode = config.LLMReasoningAuto
	if got := managedLlamaReasoningBudget(settings, true); got != settings.MaxOutputTokens/2 {
		t.Fatalf("current managed budget = %d", got)
	}
	if got := managedLlamaReasoningBudget(settings, false); got != 0 {
		t.Fatalf("outdated managed budget = %d", got)
	}
	settings.LlamaCPPMode = config.LlamaCPPModeExternal
	if got := managedLlamaReasoningBudget(settings, true); got != 0 {
		t.Fatalf("external llama.cpp budget = %d", got)
	}
	settings.Provider = config.LLMProviderOllama
	if got := managedLlamaReasoningBudget(settings, true); got != 0 {
		t.Fatalf("Ollama budget = %d", got)
	}
}

func TestChatStreamStartsMotionThroughMotionEngine(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Starting.","motion":{"action":"start","pattern_id":"pulse","speed_percent":30}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"message":"start a pulse at 30 percent"}`)
	if !strings.Contains(body, `"reply":"Starting."`) {
		t.Fatalf("chat stream missing assistant message:\n%s", body)
	}
	if !strings.Contains(body, `event: motion`) {
		t.Fatalf("chat stream missing motion event:\n%s", body)
	}

	commands := fake.Commands()
	if len(commands) < 3 {
		t.Fatalf("motion engine commands = %d, want at least 3: %+v", len(commands), commands)
	}
	if commands[0].Kind != transport.CommandKindStrokeWindow ||
		commands[1].Kind != transport.CommandKindPointsAdd ||
		commands[2].Kind != transport.CommandKindPointsPlay {
		t.Fatalf("commands did not flow through motion engine: %+v", commands[:3])
	}
}

func TestChatStopBypassesLLMAndStopsMotion(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"This should not be used.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	_ = callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"stroke","speed_percent":30}`)
	body := postChatStream(t, server, `{"message":"stop"}`)
	if !strings.Contains(body, `"reply":"Stopping motion."`) {
		t.Fatalf("chat stop response missing deterministic reply:\n%s", body)
	}
	if provider.callCount() != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.callCount())
	}

	commands := fake.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("last command = %+v, want stop", commands)
	}
}

func TestChatSpeedOnlyTargetPreservesActiveProgram(t *testing.T) {
	program := motion.ProgramDefinition{
		ID: "active-program", Name: "Active program", DurationMillis: 1000,
		Points: []motion.CurvePoint{{TimeMillis: 0}, {TimeMillis: 1000, PositionPercent: 100}},
	}
	speed := 25
	target := (&Server{}).chatMotionTarget(&chat.MotionCommand{
		Action: chat.MotionActionTarget, SpeedPercent: &speed,
	}, motion.ActiveMotionState{
		Running: true,
		Target: motion.MotionTarget{
			ProgramID: program.ID, Program: &program, SpeedPercent: 30,
		},
	})
	if target.ProgramID != program.ID || target.Program == nil || target.PatternID != "" {
		t.Fatalf("speed-only target replaced active program: %+v", target)
	}
	if target.SpeedPercent != speed {
		t.Fatalf("speed-only target speed = %d, want %d", target.SpeedPercent, speed)
	}
}

func TestChatNoneLeavesActiveMotionUnchanged(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(server.Close)
	_ = callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"pulse","speed_percent":30}`)
	engine := server.currentMotionEngine()
	before := engine.Snapshot()
	dispatch, err := server.dispatchChatMotion(context.Background(), &chat.MotionCommand{Action: chat.MotionActionNone})
	if err != nil {
		t.Fatalf("dispatch none: %v", err)
	}
	if dispatch.Applied {
		t.Fatalf("none dispatch was marked applied: %+v", dispatch)
	}
	after := engine.Snapshot()
	if !after.Running || after.Generation != before.Generation || after.PlanID != before.PlanID ||
		after.Target.PatternID != before.Target.PatternID || after.Target.SpeedPercent != before.Target.SpeedPercent {
		t.Fatalf("none action changed active motion: before=%+v after=%+v", before, after)
	}
}

func postChatStream(t *testing.T, server *Server, body string) string {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(body))
	request = withController(request)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}
