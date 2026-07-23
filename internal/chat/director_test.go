package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion/semantic"
)

type fixtureLLMProvider struct {
	responses []string
}

func (p *fixtureLLMProvider) StreamChat(_ context.Context, _ llm.ChatRequest, onDelta func(string) error) (string, error) {
	if len(p.responses) == 0 {
		return "", nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	if onDelta != nil && response != "" {
		mid := len(response) / 2
		if mid > 0 {
			_ = onDelta(response[:mid])
		}
		_ = onDelta(response[mid:])
	}
	return response, nil
}

func (p *fixtureLLMProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Available: true}
}

func TestAskDirectorValidatesIntent(t *testing.T) {
	provider := &fixtureLLMProvider{responses: []string{
		`{"action":"riding","location":"tip","intensity":7}`,
	}}
	intent, latency, err := AskDirector(context.Background(), provider, "test-model", "faster please", nil)
	if err != nil {
		t.Fatal(err)
	}
	if latency < 0 {
		t.Fatalf("latency = %v, want >= 0", latency)
	}
	if intent.Action != semantic.ActionRiding || intent.Location != semantic.LocationTip || intent.Intensity != 7 {
		t.Fatalf("intent = %+v", intent)
	}
}

func TestAskDirectorRepairsMalformedJSON(t *testing.T) {
	provider := &fixtureLLMProvider{responses: []string{
		`not json`,
		`{"action":"handjob","location":"shaft","intensity":5}`,
	}}
	intent, _, err := AskDirector(context.Background(), provider, "test-model", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Action != semantic.ActionHandjob {
		t.Fatalf("intent = %+v", intent)
	}
}

func TestAskActorInjectsIntentPrefix(t *testing.T) {
	var captured []llm.Message
	provider := &capturingLLMProvider{
		response: "Hey there.",
		onRequest: func(request llm.ChatRequest) {
			captured = request.Messages
		},
	}
	intent := semantic.LLMIntent{Action: semantic.ActionOral, Location: semantic.LocationTip, Intensity: 8}
	var tokens []string
	reply, err := AskActor(context.Background(), provider, "test-model", "more", nil, intent, func(token string) error {
		tokens = append(tokens, token)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "Hey there." {
		t.Fatalf("reply = %q", reply)
	}
	if len(captured) == 0 || !strings.Contains(captured[0].Content, "oral") {
		t.Fatalf("system prefix missing intent: %+v", captured)
	}
	if len(tokens) == 0 {
		t.Fatal("expected streamed tokens")
	}
}

type capturingLLMProvider struct {
	response  string
	onRequest func(llm.ChatRequest)
}

func (p *capturingLLMProvider) StreamChat(_ context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	if p.onRequest != nil {
		p.onRequest(request)
	}
	if onDelta != nil {
		_ = onDelta(p.response)
	}
	return p.response, nil
}

func (p *capturingLLMProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Available: true}
}

func TestMotionCommandFromLLMIntentStrokeRange(t *testing.T) {
	intent := semantic.LLMIntent{Action: semantic.ActionHandjob, Location: semantic.LocationTip, Intensity: 6}
	cmd, err := MotionCommandFromLLMIntent(intent, semantic.DefaultMotionPreferences(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.StrokeRange) != 2 || cmd.StrokeRange[0] != 0.7 || cmd.StrokeRange[1] != 1.0 {
		t.Fatalf("stroke range = %v", cmd.StrokeRange)
	}
	if cmd.PhysicalAction != string(semantic.ActionHandjob) {
		t.Fatalf("physical action = %q, want handjob", cmd.PhysicalAction)
	}
}
