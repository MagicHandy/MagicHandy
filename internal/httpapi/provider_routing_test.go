package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type modelRoutingTransport func(*http.Request) (*http.Response, error)

func (f modelRoutingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenRouterChatUsesSelectedProviderAndPreservesAdultVocabulary(t *testing.T) {
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{Transport: fake, MotionTransport: fake})
	t.Cleanup(s.Close)
	chatgpt := config.ModelConnection{ID: "chatgpt-1", Name: "ChatGPT", Provider: config.LLMProviderChatGPT, Model: "account-model"}.Normalize()
	router := config.ModelConnection{ID: "openrouter-1", Name: "ChatGPT", Provider: config.LLMProviderOpenRouter, Model: "vendor/model", OutputMode: "prompt"}.Normalize()
	if err := s.cloudPlanning.auth.SetConnectionKey(t.Context(), router, "fixture-only-key"); err != nil {
		t.Fatal(err)
	}
	const message = "Technical input mentions orgasm, penis and the word fuck. Acknowledge only; keep motion off."
	var calls atomic.Int32
	s.cloudPlanning.client = &http.Client{Transport: modelRoutingTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Host == "openrouter.ai" && r.URL.Path == "/api/v1/models" {
			w := httptest.NewRecorder()
			_, _ = fmt.Fprint(w, `{"data":[{"id":"vendor/model","supported_parameters":[]}]}`)
			return w.Result(), nil
		}
		calls.Add(1)
		if r.URL.Host != "openrouter.ai" || r.URL.Path != "/api/v1/chat/completions" {
			t.Errorf("chat used the wrong provider endpoint: %s", r.URL.Redacted())
		}
		var body struct {
			Model    string        `json:"model"`
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range body.Messages {
			if item.Role == "user" && item.Content == message {
				found = true
			}
		}
		if !found || body.Model != router.Model {
			t.Error("selected model or verbatim user message was lost")
		}
		w := httptest.NewRecorder()
		writeHostedFixtureReply(w, `{"reply":"Acknowledged. Motion stays off."}`)
		return w.Result(), nil
	})}
	saveSettings(t, s.store, func(settings config.Settings) config.Settings {
		settings.LLM.Connections = []config.ModelConnection{chatgpt, router}
		settings.LLM.ConversationConnectionID = router.ID
		settings.LLM.MotionPlanner = config.MotionPlannerSettings{Provider: "connection", ConnectionID: chatgpt.ID, ContextPolicy: "technical"}
		settings.LLM.MotionGenerationMode = config.LLMMotionModeOff
		return settings
	})
	body, _ := json.Marshal(map[string]string{"message": message})
	stream := postChatStream(t, s, string(body))
	if calls.Load() != 1 || !strings.Contains(stream, `"provider":"openrouter"`) || !strings.Contains(stream, `"reply":"Acknowledged. Motion stays off."`) || strings.Contains(stream, "event: motion") || len(fake.Commands()) != 0 {
		t.Fatalf("wrong routing, fallback or motion: calls=%d stream=%s", calls.Load(), stream)
	}
}

func TestInteractiveProviderRefusalIsReportedWithoutFallbackOrMotion(t *testing.T) {
	var calls atomic.Int32
	s, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"refusal\":\"private provider refusal detail\"}}]}\n\ndata: [DONE]\n\n")
	})
	t.Cleanup(s.Close)
	stream := postChatStream(t, s, `{"message":"A technical adult-language test, with motion off."}`)
	if calls.Load() != 1 || !strings.Contains(stream, "The selected cloud provider declined this request.") || !strings.Contains(stream, "event: error") || strings.Contains(stream, "private provider refusal detail") || strings.Contains(stream, "event: motion") || strings.Contains(stream, `"repaired":true`) || strings.Contains(stream, `"semantic_fallback":true`) || len(fake.Commands()) != 0 {
		t.Fatalf("refusal was obscured, retried or moved a device: calls=%d stream=%s", calls.Load(), stream)
	}
}
