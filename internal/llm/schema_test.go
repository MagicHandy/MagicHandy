package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderPassesOptionalResponseSchema(t *testing.T) {
	for _, kind := range []string{"ollama", "llama_cpp"} {
		t.Run(kind, func(t *testing.T) {
			var captured map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Error(err)
				}
				if kind == "ollama" {
					_, _ = w.Write([]byte("{\"message\":{\"content\":\"{}\"},\"done\":true}\n"))
				} else {
					_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{}\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
				}
			}))
			defer server.Close()
			options := HTTPProviderOptions{BaseURL: server.URL, Model: "test", Client: server.Client()}
			var provider Provider
			var err error
			if kind == "ollama" {
				provider, err = NewOllamaProvider(options)
			} else {
				provider, err = NewLlamaCPPProvider(options)
			}
			if err != nil {
				t.Fatal(err)
			}
			schema := json.RawMessage(`{"type":"object","properties":{"reply":{"type":"string"}}}`)
			if _, err := provider.StreamChat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "test"}}, JSONSchema: schema}, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			actual := captured["format"]
			if kind == "llama_cpp" {
				var format struct {
					Schema json.RawMessage `json:"schema"`
				}
				if err := json.Unmarshal(captured["response_format"], &format); err != nil {
					t.Fatal(err)
				}
				actual = format.Schema
			}
			if string(actual) != string(schema) {
				t.Fatalf("schema changed: %s", actual)
			}
		})
	}
}
