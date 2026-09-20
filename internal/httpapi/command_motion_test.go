package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestDelayedChatKeepsReplyWithoutOverridingNewerControls(t *testing.T) {
	provider := &stubbornLLMProvider{started: make(chan struct{}), release: make(chan struct{})}
	s, _, _, cookie := newControllerSessionFixture(t, provider)
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.MotionGenerationMode = config.LLMMotionModePattern
		return settings
	})
	claimAuthenticatedController(t, s, cookie)
	r := commandTestRequest(s, cookie, "/api/chat/stream", `{"message":"start moving"}`, "delayed-model", 1)
	r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- serveCommand(s, r) }()
	released := false
	defer func() {
		if !released {
			close(provider.release)
		}
	}()
	select {
	case <-provider.started:
	case result := <-done:
		t.Fatalf("chat did not reach generation: %d %s", result.Code, result.Body.String())
	case <-time.After(2 * time.Second):
		t.Fatal("chat did not reach generation")
	}
	w := serveCommand(s, commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":42}`, "newer-controls", 2))
	if w.Code != http.StatusOK {
		t.Fatalf("inference blocked live controls: %d %s", w.Code, w.Body.String())
	}
	close(provider.release)
	released = true
	select {
	case result := <-done:
		body := result.Body.String()
		if !strings.Contains(body, `"reply":"Late start."`) || !strings.Contains(body, `"applied":false`) || !strings.Contains(body, "superseded") {
			t.Fatalf("obsolete motion was not rejected while retaining the reply: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("chat did not finish after provider response")
	}
	if engine := s.currentMotionEngine(); engine != nil && motionStateActive(engine.Snapshot()) {
		t.Fatal("old chat response restarted motion after newer controls")
	}
}

func TestBackgroundCommandDoesNotSupersedeDeferredMotion(t *testing.T) {
	s := &Server{commands: newCommandRuntime()}
	scope := commandScope{actor: controllerIdentity{sessionKey: "session", clientID: "tab"}, generation: 1}
	receipt := &commandReceipt{key: commandReceiptKey{id: "chat"}, Sequence: 1, route: "/api/chat/stream"}
	if err := s.commands.begin(scope, receipt); err != nil {
		t.Fatal(err)
	}
	if err := s.commands.begin(scope, &commandReceipt{key: commandReceiptKey{id: "voice"}, Sequence: 2, route: "/api/voice/requests/played"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), commandInvocationKey{}, &commandInvocation{scope: scope, receipt: receipt})
	release, err := s.beginDeferredMotion(ctx)
	if err != nil {
		t.Fatalf("voice bookkeeping discarded pending motion: %v", err)
	}
	defer release()
	// Application keeps the same lane as immediate commands: no newer motion
	// can be admitted between the freshness check and the engine call.
	waitCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancel()
	if acquired, err := s.commands.acquire(waitCtx); err == nil {
		acquired()
		t.Fatal("another control command entered during deferred application")
	}
}
