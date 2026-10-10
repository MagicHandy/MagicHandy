package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func compatibleTestStream(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": content}, "finish_reason": nil}}})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", chunk)
}

func TestOpenRouterPreservesIncludedMessagesAndRoutingPolicy(t *testing.T) {
	messages := []Message{{Role: "system", Content: "Persona and enabled memory."},
		{Role: "user", Content: "  Consenting adults: preserve NSFW wording, including orgasm.\nKeep this spacing.  "},
		{Role: "assistant", Content: "Original adult conversation response.\nNo wrapper or truncation."},
		{Role: "user", Content: "Continue our conversation."}}
	var completions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer router-key" {
			t.Error("wrong provider credential")
		}
		switch r.URL.Path {
		case "/models":
			_, _ = fmt.Fprint(w, `{"data":[{"id":"provider/model","name":"Model","supported_parameters":["structured_outputs","max_tokens","temperature"]}]}`)
		case "/chat/completions":
			completions.Add(1)
			var body struct {
				Messages []Message `json:"messages"`
				Provider struct {
					AllowFallbacks    bool     `json:"allow_fallbacks"`
					RequireParameters bool     `json:"require_parameters"`
					Only              []string `json:"only"`
					DataCollection    string   `json:"data_collection"`
					ZDR               bool     `json:"zdr"`
				} `json:"provider"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(body.Messages, messages) {
				t.Errorf("included messages changed: %#v", body.Messages)
			}
			if body.Provider.AllowFallbacks || !body.Provider.RequireParameters || !body.Provider.ZDR || body.Provider.DataCollection != "deny" || !reflect.DeepEqual(body.Provider.Only, []string{"allowed-host"}) {
				t.Errorf("routing policy changed: %+v", body.Provider)
			}
			compatibleTestStream(w, `{"reply":"Complete","edits":{"speed":null,"layers":[]}}`)
		default:
			t.Errorf("local or unexpected endpoint was used: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	provider, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "provider/model"}, Provider: "openrouter", Token: func(context.Context) (string, error) { return "router-key", nil }, AllowedProviders: []string{"allowed-host"}, DataCollection: "deny", ZeroDataRetention: true})
	if err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"type":"object","properties":{"reply":{"type":"string"},"edits":{"type":"object","properties":{"speed":{"type":"integer"},"layers":{"type":"array","items":{"type":"string"}}},"required":[]}},"required":["reply","edits"]}`)
	raw, err := provider.StreamChat(t.Context(), ChatRequest{Messages: messages, JSONSchema: schema, MaxTokens: 64}, nil)
	if err != nil || raw != `{"edits":{"layers":[]},"reply":"Complete"}` || completions.Load() != 1 {
		t.Fatalf("completion = %q, %v, calls=%d", raw, err, completions.Load())
	}
}

func TestCompatibleEmitsOnlyDeclaredParameters(t *testing.T) {
	provider, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: "http://127.0.0.1:1/v1", Model: "model"}, Provider: "compatible", OutputMode: "prompt"})
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := provider.completionBody(ChatRequest{MaxTokens: 64, Temperature: .7, TopP: .9, ReasoningMode: "on", JSONSchema: json.RawMessage(`{"type":"object"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 3 || body["stream"] != true {
		t.Fatalf("unsupported parameters escaped: %v", body)
	}
	provider.options.Provider, provider.options.OutputMode = "openrouter", "strict"
	_, _, err = provider.completionBody(ChatRequest{JSONSchema: json.RawMessage(`{"type":"object"}`)}, []string{"max_tokens"})
	var outcome *CloudError
	if !errors.As(err, &outcome) || outcome.Kind != "capability" {
		t.Fatalf("unsupported schema must fail before network: %v", err)
	}
}

// A server on this computer left on automatic output (LM Studio, an MLX server,
// llama-server, vLLM, Ollama) gets the app's sampling and the standard
// non-strict schema form. Declared policies and remote endpoints are unchanged.
func TestCompatibleLocalServerDefaultsToSchemaAndSampling(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"reply":{"type":"string"}},"required":["reply"]}`)
	request := ChatRequest{MaxTokens: 64, Temperature: .3, TopP: .95, JSONSchema: schema}
	for _, tc := range []struct {
		name, baseURL, mode string
		declared            []string
		format              string
		sampling            bool
	}{
		{name: "loopback", baseURL: "http://127.0.0.1:1234/v1", format: `{"json_schema":{"name":"magichandy_response","schema":` + string(schema) + `},"type":"json_schema"}`, sampling: true},
		{name: "localhost", baseURL: "http://localhost:8080/v1", format: `{"json_schema":{"name":"magichandy_response","schema":` + string(schema) + `},"type":"json_schema"}`, sampling: true},
		{name: "remote", baseURL: "https://models.example.test/v1"},
		{name: "explicit prompt", baseURL: "http://127.0.0.1:1234/v1", mode: "prompt"},
		{name: "declared json", baseURL: "http://127.0.0.1:1234/v1", declared: []string{"response_format"}, format: `{"type":"json_object"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: tc.baseURL, Model: "model"}, Provider: "compatible", OutputMode: tc.mode, SupportedParameters: tc.declared})
			if err != nil {
				t.Fatal(err)
			}
			body, strict, err := provider.completionBody(request, tc.declared)
			if err != nil || strict {
				t.Fatalf("body: %v strict=%t", err, strict)
			}
			format, _ := json.Marshal(body["response_format"])
			if body["response_format"] == nil {
				format = nil
			}
			if string(format) != tc.format {
				t.Fatalf("response_format = %s, want %s", format, tc.format)
			}
			_, hasTemperature := body["temperature"]
			if hasTemperature != tc.sampling || (tc.sampling && (body["top_p"] != .95 || body["max_tokens"] != 64)) {
				t.Fatalf("sampling = %v", body)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	defer server.Close()
	provider, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "model"}, Provider: "compatible"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.StreamChat(t.Context(), ChatRequest{Messages: []Message{{Role: "user", Content: "test"}}, JSONSchema: schema}, nil)
	var outcome *CloudError
	if !errors.As(err, &outcome) || outcome.Kind != "capability" {
		t.Fatalf("a local server rejecting the default request must name the output mode: %v", err)
	}
}

func TestCompatibleNeverReleasesPartialOrRefusedOutput(t *testing.T) {
	partial := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"{\\\"reply\\\":\\\"partial\\\"}\"}}]}\n\n"
	cases := map[string]string{
		"mid_stream_error": partial + "data: {\"error\":{\"message\":\"private prompt and secret\"}}\n\ndata: [DONE]\n\n",
		"refusal":          partial + "data: {\"choices\":[{\"delta\":{\"refusal\":\"declined\"}}]}\n\n",
		"filter":           partial + "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"content_filter\"}]}\n\n",
		"length":           partial + "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n",
		"missing_done":     partial + "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
		"missing_finish":   partial + "data: [DONE]\n\n",
	}
	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, stream) }))
			defer server.Close()
			provider, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "model"}, Provider: "compatible"})
			if err != nil {
				t.Fatal(err)
			}
			deltas := 0
			raw, err := provider.StreamChat(t.Context(), ChatRequest{Messages: []Message{{Role: "user", Content: "test"}}}, func(string) error { deltas++; return nil })
			if err == nil || raw != "" || deltas != 0 || !IsCloudOutcome(err) || strings.Contains(err.Error(), "private prompt") {
				t.Fatalf("partial/refused output escaped: %q, %v, deltas=%d", raw, err, deltas)
			}
		})
	}
}

func TestHostedProvidersDoNotFollowCredentialRedirects(t *testing.T) {
	var leaked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sink" {
			leaked.Store(true)
			t.Errorf("redirect carried credential: %s", r.Header.Get("Authorization"))
			return
		}
		http.Redirect(w, r, "/sink", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	for _, name := range []string{"compatible", "responses"} {
		t.Run(name, func(t *testing.T) {
			token := func(context.Context) (string, error) { return "test-private-key", nil }
			var provider Provider
			var err error
			if name == "compatible" {
				provider, err = NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "model"}, Provider: name, Token: token})
			} else {
				provider, err = NewOpenAIResponsesProvider(OpenAIOptions{BaseURL: server.URL, Model: "model", Token: token})
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.StreamChat(t.Context(), ChatRequest{Messages: []Message{{Role: "user", Content: "test"}}}, nil)
			if err == nil || leaked.Load() {
				t.Fatalf("redirect was accepted: %v", err)
			}
		})
	}
}

func TestCompatibleCancellationStopsReading(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	provider, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "model"}, Provider: "compatible"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := provider.StreamChat(ctx, ChatRequest{}, nil); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled stream kept reading")
	}
}
