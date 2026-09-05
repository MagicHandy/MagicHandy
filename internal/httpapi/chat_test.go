package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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

type blockingLLMProvider struct {
	started chan struct{}
}

type stubbornLLMProvider struct {
	started chan struct{}
	release chan struct{}
}

type blockingStopTransport struct {
	*transport.Fake
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingLLMProvider) StreamChat(ctx context.Context, _ llm.ChatRequest, _ func(string) error) (string, error) {
	close(p.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func (p *blockingLLMProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Provider: "blocking", Available: true}
}

func (p *stubbornLLMProvider) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(string) error) (string, error) {
	close(p.started)
	<-p.release
	return `{"reply":"Late start.","motion":{"action":"start","speed_percent":30}}`, nil
}

func (p *stubbornLLMProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Provider: "stubborn", Available: true}
}

func (t *blockingStopTransport) Stop(ctx context.Context, command transport.StopCommand) (transport.CommandResult, error) {
	t.once.Do(func() { close(t.started) })
	select {
	case <-t.release:
		return t.Fake.Stop(ctx, command)
	case <-ctx.Done():
		return transport.CommandResult{}, ctx.Err()
	}
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
		`{"reply":"Starting.","motion":{"action":"start","pattern_id":"flow-pace-wave","speed_percent":30}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"message":"start a pace wave at 30 percent"}`)
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

func TestChatStreamStartsAndRetargetsDynamicMotionThroughOneEngine(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Starting creatively.","motion":{"action":"start","speed_percent":30,"anchors":["tip","middle","base"],"span_min_percent":30,"span_profile":"wander","variation_percent":20,"segment_seconds":18}}`,
		`{"reply":"Changing the geometry.","motion":{"action":"update","center_percent":35,"span_percent":50,"span_min_percent":24,"span_profile":"contrast","variation_percent":30,"segment_seconds":22}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport: fake, MotionTransport: fake, LLMProvider: provider,
	})
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = config.LLMMotionModeDynamic
		return settings
	})

	started := postChatStream(t, server, `{"message":"start moving gently"}`)
	if !strings.Contains(started, `event: motion`) {
		t.Fatalf("dynamic start did not dispatch:\n%s", started)
	}
	snapshot := server.currentMotionEngine().Snapshot()
	assertStartedDynamicTarget(t, snapshot)

	updated := postChatStream(t, server, `{"message":"keep changing it up"}`)
	if !strings.Contains(updated, `"action":"update"`) {
		t.Fatalf("dynamic update did not dispatch:\n%s", updated)
	}
	snapshot = server.currentMotionEngine().Snapshot()
	assertUpdatedDynamicTarget(t, snapshot)
	plays := 0
	for _, command := range fake.Commands() {
		if command.Kind == transport.CommandKindPointsPlay {
			plays++
		}
	}
	if plays != 1 {
		t.Fatalf("dynamic retarget restarted transport playback %d times, want one shared stream", plays)
	}
}

func assertStartedDynamicTarget(t *testing.T, snapshot motion.ActiveMotionState) {
	t.Helper()
	dynamic := snapshot.Target.Dynamic
	if !snapshot.Running || dynamic == nil || snapshot.Target.PatternID != "" ||
		len(dynamic.Anchors) != 3 || dynamic.SpanMinPercent != 30 ||
		dynamic.SpanProfile != motion.DynamicSpanProfileWander || dynamic.PhraseSeed == 0 {
		t.Fatalf("dynamic start target = %+v", snapshot.Target)
	}
}

func assertUpdatedDynamicTarget(t *testing.T, snapshot motion.ActiveMotionState) {
	t.Helper()
	dynamic := snapshot.Target.Dynamic
	if dynamic == nil || dynamic.CenterPercent != 35 || dynamic.SpanPercent != 50 ||
		dynamic.SpanMinPercent != 24 || dynamic.SpanProfile != motion.DynamicSpanProfileContrast ||
		dynamic.PhraseSeed == 0 || dynamic.VariationPercent != 30 ||
		dynamic.SegmentSeconds != 22 || len(dynamic.Anchors) != 0 {
		t.Fatalf("dynamic update target = %+v", dynamic)
	}
}

func TestChatStreamPreservesDirectPartnerNoMotionChoice(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Come closer."}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"message":"Suck me"}`)
	if !strings.Contains(body, `"reply":"Come closer."`) {
		t.Fatalf("chat stream missing assistant message:\n%s", body)
	}
	if strings.Contains(body, `"semantic_fallback":true`) || strings.Contains(body, `event: motion`) {
		t.Fatalf("chat stream replaced the model's no-motion choice:\n%s", body)
	}
	if commands := fake.Commands(); len(commands) != 0 {
		t.Fatalf("model no-motion choice dispatched commands: %+v", commands)
	}
}

func TestChatStreamAcceptsDirectPartnerStartChosenByModel(t *testing.T) {
	fake := transport.NewFake()
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Right there.","motion":{"action":"start","speed_percent":25}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"message":"kiss it"}`)
	if !strings.Contains(body, `event: motion`) || !strings.Contains(body, `"action":"start"`) {
		t.Fatalf("chat stream stripped the model-selected start:\n%s", body)
	}
	if strings.Contains(body, `"semantic_fallback":true`) {
		t.Fatalf("chat stream reported deterministic fallback for a model-selected start:\n%s", body)
	}

	commands := fake.Commands()
	if len(commands) < 3 {
		t.Fatalf("motion engine commands = %d, want at least 3: %+v", len(commands), commands)
	}
	if commands[0].Kind != transport.CommandKindStrokeWindow ||
		commands[1].Kind != transport.CommandKindPointsAdd ||
		commands[2].Kind != transport.CommandKindPointsPlay {
		t.Fatalf("model-selected start bypassed the motion engine: %+v", commands[:3])
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

	_ = callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":30}`)
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

func TestChatStopRemainsAvailableWhenLLMMotionIsOff(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
	})
	t.Cleanup(server.Close)

	_ = callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"flow-full-sweeps","speed_percent":30}`)
	_, _, err := server.store.Update(func(settings config.Settings) (config.Settings, error) {
		settings.LLM.MotionGenerationMode = config.LLMMotionModeOff
		return settings, nil
	})
	if err != nil {
		t.Fatalf("disable LLM motion: %v", err)
	}

	dispatch, err := server.dispatchChatMotion(t.Context(), &chat.MotionCommand{Action: chat.MotionActionStop})
	if err != nil {
		t.Fatalf("dispatch Stop while LLM motion is off: %v", err)
	}
	if !dispatch.Applied {
		t.Fatalf("Stop dispatch = %+v, want applied", dispatch)
	}
	commands := fake.Commands()
	if len(commands) == 0 || commands[len(commands)-1].Kind != transport.CommandKindStop {
		t.Fatalf("last transport command = %+v, want Stop", commands)
	}
}

func TestChatStopMatcherCoversBuiltInPromptLanguages(t *testing.T) {
	for _, message := range []string{
		"Please stop the motion.",
		"Por favor, detén el movimiento.",
		"Por favor, pare o movimento.",
		"请停止运动。",
		"動きを止めてください。",
	} {
		if !isChatStopMessage(message) {
			t.Errorf("clear Stop request %q did not use the deterministic path", message)
		}
	}
	for promptSet, want := range map[string]string{
		chat.DefaultPromptSetID:           "Stopping motion.",
		chat.PromptSetIDSpanish:           "Deteniendo el movimiento.",
		chat.PromptSetIDPortugueseBrazil:  "Parando o movimento.",
		chat.PromptSetIDSimplifiedChinese: "正在停止运动。",
		chat.PromptSetIDJapanese:          "モーションを停止します。",
	} {
		if got := chatStopReply(promptSet); got != want {
			t.Errorf("Stop reply for %q = %q, want %q", promptSet, got, want)
		}
	}
	for _, message := range []string{
		"Do not stop the story.",
		"Explica la parada de emergencia.",
		"Conte uma história sobre movimento.",
		"解释停止运动的含义。",
		"停止について説明して。",
	} {
		if isChatStopMessage(message) {
			t.Errorf("conversation %q was mistaken for a deterministic Stop", message)
		}
	}
}

func TestChatStopPublishesSequenceWhileTransportIsBlocked(t *testing.T) {
	blocking := &blockingStopTransport{
		Fake:    transport.NewFake(),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	server := newTestServerWithRuntime(t, Runtime{Transport: blocking, MotionTransport: blocking})
	t.Cleanup(server.Close)

	stopRequest := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"stop"}`)))
	stopRequest.Header.Set("Content-Type", "application/json")
	stopRecorder := httptest.NewRecorder()
	stopDone := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(stopRecorder, stopRequest)
		close(stopDone)
	}()
	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		close(blocking.release)
		t.Fatal("chat Stop did not reach the transport")
	}

	stateRecorder := httptest.NewRecorder()
	stateDone := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(stateRecorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
		close(stateDone)
	}()
	select {
	case <-stateDone:
	case <-time.After(2 * time.Second):
		close(blocking.release)
		t.Fatal("state publication blocked behind transport Stop")
	}
	if stateRecorder.Code != http.StatusOK || !strings.Contains(stateRecorder.Body.String(), `"stop_sequence":1`) {
		close(blocking.release)
		t.Fatalf("state did not publish the Chat Stop sequence: %d %s", stateRecorder.Code, stateRecorder.Body.String())
	}
	admissionDone := make(chan error, 1)
	go func() {
		_, _, err := server.motionEngineForStart()
		admissionDone <- err
	}()
	select {
	case err := <-admissionDone:
		close(blocking.release)
		t.Fatalf("new engine admission completed before physical Stop: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(blocking.release)
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("chat Stop did not finish after transport release")
	}
	select {
	case err := <-admissionDone:
		if err != nil {
			t.Fatalf("engine admission after Stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("engine admission remained blocked after Stop")
	}
	if !strings.Contains(stopRecorder.Body.String(), `"stop_sequence":1`) {
		t.Fatalf("chat Stop stream omitted its sequence:\n%s", stopRecorder.Body.String())
	}
}

func TestChatStopIgnoresStaleSessionForSafety(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"session_id":"stale-session","message":"stop"}`)
	if !strings.Contains(body, `"state":"deterministic_stop"`) || !strings.Contains(body, `"user_seq":0`) {
		t.Fatalf("stale-session Stop response = %s", body)
	}
	commands := fake.Commands()
	if len(commands) != 1 || commands[0].Kind != transport.CommandKindStop {
		t.Fatalf("stale-session Stop commands = %+v", commands)
	}
}

func TestChatStopInvalidatesOverlappingGenerationBeforeItCanRestartMotion(t *testing.T) {
	fake := transport.NewFake()
	provider := &stubbornLLMProvider{started: make(chan struct{}), release: make(chan struct{})}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"start moving"}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(stopSequenceHeader, "0")
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(recorder, request)
		close(done)
	}()
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("chat provider did not start")
	}

	stopBody := postChatStream(t, server, `{"message":"stop"}`)
	if !strings.Contains(stopBody, `"reply":"Stopping motion."`) {
		t.Fatalf("chat stop response missing deterministic reply:\n%s", stopBody)
	}
	close(provider.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("invalidated chat did not finish")
	}
	for _, command := range fake.Commands() {
		if command.Kind != transport.CommandKindStop {
			t.Fatalf("overlapping generation issued post-Stop motion: %+v", fake.Commands())
		}
	}
}

func TestChatStartRechecksStopAfterWaitingForEngineAdmission(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(server.Close)
	stopSequence := server.stopSequence.Load()
	server.motion.lifecycleMu.Lock()

	type dispatchResult struct {
		dispatch chatMotionDispatch
		err      error
	}
	result := make(chan dispatchResult, 1)
	go func() {
		dispatch, err := server.dispatchChatMotionAt(t.Context(), &chat.MotionCommand{Action: chat.MotionActionStart}, &stopSequence)
		result <- dispatchResult{dispatch: dispatch, err: err}
	}()
	time.Sleep(20 * time.Millisecond)
	finishInvalidation := server.invalidateWorkForStop("test_stop")
	server.motion.lifecycleMu.Unlock()
	defer finishInvalidation()

	select {
	case got := <-result:
		if got.err == nil || got.dispatch.Applied {
			t.Fatalf("stale chat start after admission wait = %+v, %v", got.dispatch, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stale chat start remained blocked")
	}
	if commands := fake.Commands(); len(commands) != 0 {
		t.Fatalf("stale chat start reached transport: %+v", commands)
	}
}

func TestEmergencyStopCancelsInflightChatBeforeMotionDispatch(t *testing.T) {
	fake := transport.NewFake()
	provider := &blockingLLMProvider{started: make(chan struct{})}
	server := newTestServerWithRuntime(t, Runtime{
		Transport:       fake,
		MotionTransport: fake,
		LLMProvider:     provider,
	})
	t.Cleanup(server.Close)

	request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"start moving"}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(stopSequenceHeader, "0")
	chatRecorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(chatRecorder, request)
		close(done)
	}()

	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("chat provider did not start")
	}
	stopRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopRecorder, httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`)))
	if stopRecorder.Code != http.StatusOK {
		t.Fatalf("stop status = %d: %s", stopRecorder.Code, stopRecorder.Body.String())
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight chat did not cancel")
	}
	for _, command := range fake.Commands() {
		if command.Kind != transport.CommandKindStop {
			t.Fatalf("post-Stop chat issued motion command: %+v", command)
		}
	}
}

func TestChatRejectsStaleStopSequence(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{`{"reply":"Starting.","motion":{"action":"start"}}`}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	callMotion(t, server, http.MethodPost, "/api/motion/stop", `{}`)

	request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"start"}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(stopSequenceHeader, "0")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || provider.callCount() != 0 {
		t.Fatalf("stale chat = %d, provider calls = %d: %s", recorder.Code, provider.callCount(), recorder.Body.String())
	}
}

func TestChatSpeedOnlyTargetPreservesActiveProgram(t *testing.T) {
	program := motion.ProgramDefinition{
		ID: "active-program", Name: "Active program", DurationMillis: 1000,
		Points: []motion.CurvePoint{{TimeMillis: 0}, {TimeMillis: 1000, PositionPercent: 100}},
	}
	speed := 25
	target, err := (&Server{}).chatMotionTarget(&chat.MotionCommand{
		Action: chat.MotionActionTarget, SpeedPercent: &speed,
	}, motion.ActiveMotionState{
		Running: true,
		Target: motion.MotionTarget{
			ProgramID: program.ID, Program: &program, SpeedPercent: 30,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.ProgramID != program.ID || target.Program == nil || target.PatternID != "" {
		t.Fatalf("speed-only target replaced active program: %+v", target)
	}
	if target.SpeedPercent != speed {
		t.Fatalf("speed-only target speed = %d, want %d", target.SpeedPercent, speed)
	}
}

func TestDynamicDefinitionFromCommandPreservesAndClearsSpanEnvelope(t *testing.T) {
	current := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{
		CenterPercent: 50, SpanPercent: 78, SpanMinPercent: 32,
		SpanProfile: motion.DynamicSpanProfileWander, VariationPercent: 20, SegmentSeconds: 18,
	})
	if current.PhraseSeed == 0 {
		t.Fatal("fixture has no phrase seed")
	}

	speed := 34
	preserved := dynamicDefinitionFromCommand(&chat.MotionCommand{
		Action: chat.MotionActionUpdate, SpeedPercent: &speed,
	}, &current)
	if preserved.SpanMinPercent != current.SpanMinPercent || preserved.SpanProfile != current.SpanProfile ||
		preserved.PhraseSeed != current.PhraseSeed {
		t.Fatalf("speed-only update reset envelope: current=%+v updated=%+v", current, preserved)
	}

	steady := dynamicDefinitionFromCommand(&chat.MotionCommand{
		Action: chat.MotionActionUpdate, SpanProfile: chat.DynamicSpanProfileSteady,
	}, &current)
	if steady.SpanProfile != motion.DynamicSpanProfileSteady || steady.SpanMinPercent != steady.SpanPercent || steady.PhraseSeed == 0 {
		t.Fatalf("steady update did not clear envelope: %+v", steady)
	}

	spanMin := 26
	restarted := dynamicDefinitionFromCommand(&chat.MotionCommand{
		Action: chat.MotionActionUpdate, SpanMinPercent: &spanMin,
	}, &steady)
	if restarted.SpanProfile != motion.DynamicSpanProfileWander || restarted.SpanMinPercent != spanMin || restarted.PhraseSeed == 0 {
		t.Fatalf("span floor did not restart a wandering envelope: %+v", restarted)
	}
}

func TestChatNoneLeavesActiveMotionUnchanged(t *testing.T) {
	fake := transport.NewFake()
	server := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(server.Close)
	_ = callMotion(t, server, http.MethodPost, "/api/motion/start", `{"pattern":"flow-pace-wave","speed_percent":30}`)
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
	request.Header.Set(stopSequenceHeader, strconv.FormatUint(server.stopSequence.Load(), 10))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}
