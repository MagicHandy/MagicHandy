package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
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
}

func TestChatStreamStartsChaoticMotionThroughManualQueue(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Starting.","motion":{"action":"start","velocidade":60,"intensidade":70,"regiao":"meio","tipo_batida":"simples"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"message":"start motion"}`)
	if !strings.Contains(body, `"reply":"Starting."`) {
		t.Fatalf("chat stream missing assistant message:\n%s", body)
	}
	if !strings.Contains(body, `event: motion`) {
		t.Fatalf("chat stream missing motion event:\n%s", body)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		commands := fake.Commands()
		if hasManualQueueHSPDispatch(commands) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("manual queue HSP dispatch missing: %+v", fake.Commands())
}

func TestChatStreamDirectorModeEventOrder(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"action":"riding","location":"tip","intensity":6}`,
		`Keep riding with me.`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.DirectorMode = true
		settings.Motion.MotionGenerationMode = config.MotionGenerationModeProcedural
		return settings
	})

	body := postChatStream(t, server, `{"message":"faster on the tip"}`)
	intentIdx := strings.Index(body, "event: intent")
	motionIdx := strings.Index(body, "event: motion")
	deltaIdx := strings.Index(body, "event: delta")
	messageIdx := strings.Index(body, "event: message")
	if intentIdx < 0 || motionIdx < 0 || deltaIdx < 0 || messageIdx < 0 {
		t.Fatalf("missing SSE events:\n%s", body)
	}
	if !(intentIdx < deltaIdx && deltaIdx < messageIdx) {
		t.Fatalf("expected intent before delta before message:\n%s", body)
	}
	if !(intentIdx < motionIdx) {
		t.Fatalf("expected intent before motion:\n%s", body)
	}
	if !strings.Contains(body, `"action":"riding"`) {
		t.Fatalf("intent payload missing:\n%s", body)
	}
}

func hasManualQueueHSPDispatch(commands []transport.Command) bool {
	var sawAdd, sawPlay bool
	for _, command := range commands {
		switch command.Kind {
		case transport.CommandKindHSPAdd:
			sawAdd = true
		case transport.CommandKindHSPPlay:
			sawPlay = true
		}
	}
	return sawAdd && sawPlay
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
