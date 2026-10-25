package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaContextAllocationIsExplicitAndLegacyLeavesDaemonDefault(t *testing.T) {
	for _, size := range []int{0, 32768, 65536} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var body struct{ Options map[string]any }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				_, _ = fmt.Fprintln(w, `{"message":{"content":"{\"reply\":\"Ready.\"}"},"done":true}`)
			}))
			defer server.Close()
			provider, err := NewOllamaProvider(HTTPProviderOptions{BaseURL: server.URL, Model: "fixture", ContextSize: size})
			if err != nil {
				t.Fatal(err)
			}
			reply, err := provider.StreamChat(t.Context(), ChatRequest{Messages: []Message{{Role: "user", Content: "Ready?"}}, MaxTokens: 256, ReasoningMode: "off"}, nil)
			if err != nil || reply == "" {
				t.Fatalf("generation: %q %v", reply, err)
			}
			if size == 0 {
				if _, exists := body.Options["num_ctx"]; exists {
					t.Fatal("legacy request now reallocates context")
				}
			} else if body.Options["num_ctx"] != float64(size) {
				t.Fatalf("allocation=%v", body.Options)
			}
		})
	}
}
