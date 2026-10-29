package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenRouterReasoningDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name, metadata       string
		supported, mandatory bool
		efforts              []string
	}{
		{"absent", ``, false, false, nil},
		{"explicit", `,"reasoning":{"supported_efforts":["high","low","none","unknown","low"],"mandatory":false}`, true, false, []string{"none", "low", "high"}},
		{"mandatory", `,"reasoning":{"supported_efforts":["none","low"],"mandatory":true}`, true, true, []string{"low"}},
		{"null all", `,"reasoning":{"supported_efforts":null}`, true, false, routerEfforts},
		{"omitted effort", `,"reasoning":{"supports_max_tokens":true}`, true, false, nil},
		{"empty", `,"reasoning":{"supported_efforts":[]}`, true, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var item compatibleCatalogModel
			if err := json.Unmarshal([]byte(`{"id":"test","reasoning_efforts":["invented"]`+tc.metadata+`}`), &item); err != nil {
				t.Fatal(err)
			}
			m := item.withRouterReasoning()
			if m.ReasoningSupported != tc.supported || m.ReasoningMandatory != tc.mandatory || !reflect.DeepEqual(m.ReasoningEfforts, tc.efforts) {
				t.Fatalf("catalog: %+v", m)
			}
		})
	}
}

func TestOpenRouterReasoningHeadroomCoversEveryAnswerBudget(t *testing.T) {
	// These are the production readiness, candidate, Edge, chat and review
	// answer sizes. The provider boundary covers each, including repaired calls.
	for _, answer := range []int{64, 128, 192, 256, 512, 1536, 4096} {
		for _, effort := range []string{"", "none", "low", "high"} {
			var sent int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/models" {
					_, _ = fmt.Fprint(w, `{"data":[{"id":"fixture","supported_parameters":["max_tokens","reasoning"],"reasoning":{"supported_efforts":["none","low","high"]}}]}`)
					return
				}
				var body struct {
					MaxTokens int `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				sent = body.MaxTokens
				compatibleTestStream(w, "Ready")
			}))
			p, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "fixture", ContextSize: 16384}, Provider: "openrouter", ReasoningEffort: effort})
			if err != nil {
				t.Fatal(err)
			}
			var budget PromptBudget
			_, err = p.StreamChat(t.Context(), ChatRequest{MaxTokens: answer, OnBudget: func(b PromptBudget) { budget = b }}, nil)
			server.Close()
			want := max(answer, 2048)
			if effort == "none" {
				want = answer
			}
			if err != nil || sent != want || budget.LimitBytes != PromptInputByteLimit(16384, want, 0, 0) {
				t.Fatalf("answer=%d effort=%q sent=%d budget=%+v err=%v", answer, effort, sent, budget, err)
			}
		}
	}
}

func TestOpenRouterReasoningHeadroomReservesInputBeforeGeneration(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	p, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "fixture", ContextSize: 8192}, Provider: "openrouter"})
	if err != nil {
		t.Fatal(err)
	}
	// Fits with only 256 answer tokens, but not with the reasoning reserve.
	_, err = p.StreamChat(t.Context(), ChatRequest{MaxTokens: 256, Messages: []Message{{Role: "system", Content: strings.Repeat("x", 5000)}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "input budget") || calls != 0 {
		t.Fatalf("reasoning reserve admitted an oversized request: calls=%d err=%v", calls, err)
	}
}

func TestOpenRouterReasoningWireAndRejection(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name, effort string
		enabled      *bool
		metadata     string
		want         string
		reject       bool
	}{
		{name: "default absent", want: "null"},
		{name: "off", enabled: &no, metadata: `,"reasoning":{}`, want: `{"enabled":false}`},
		{name: "on without effort support", enabled: &yes, metadata: `,"reasoning":{}`, want: `{"enabled":true}`},
		{name: "explicit low", effort: "low", metadata: `,"reasoning":{"supported_efforts":["none","low"]}`, want: `{"effort":"low"}`},
		{name: "explicit none", effort: "none", metadata: `,"reasoning":{"supported_efforts":["none","low"]}`, want: `{"effort":"none"}`},
		{name: "missing capability", effort: "low", reject: true},
		{name: "unsupported effort", effort: "high", metadata: `,"reasoning":{"supported_efforts":["low"]}`, reject: true},
		{name: "mandatory off", enabled: &no, metadata: `,"reasoning":{"mandatory":true}`, reject: true},
		{name: "conflicting settings", effort: "low", enabled: &no, metadata: `,"reasoning":{"supported_efforts":["low"]}`, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests, catalogs atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/models" {
					catalogs.Add(1)
					_, _ = fmt.Fprint(w, `{"data":[{"id":"fixture","supported_parameters":["reasoning","max_tokens"]`+tc.metadata+`}]}`)
					return
				}
				requests.Add(1)
				var b map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
					t.Error(err)
				}
				got := "null"
				if b["reasoning"] != nil {
					got = string(b["reasoning"])
				}
				if got != tc.want {
					t.Errorf("reasoning %s want %s", got, tc.want)
				}
				if b["reasoning_effort"] != nil || b["chat_template_kwargs"] != nil {
					t.Error("wrong protocol")
				}
				compatibleTestStream(w, "Ready")
			}))
			defer server.Close()
			p, err := NewCompatibleProvider(CompatibleOptions{HTTPProviderOptions: HTTPProviderOptions{BaseURL: server.URL, Model: "fixture"}, Provider: "openrouter", ReasoningEffort: tc.effort, ReasoningEnabled: tc.enabled})
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				raw, err := p.StreamChat(t.Context(), ChatRequest{MaxTokens: 64}, nil)
				if (err != nil) != tc.reject {
					t.Fatalf("raw=%q err=%v", raw, err)
				}
				if tc.reject && !IsCloudOutcome(err) {
					t.Fatalf("reasoning capability failure lost its cloud outcome type: %v", err)
				}
			}
			if catalogs.Load() != 1 {
				t.Fatal("capability cache missed")
			}
			if tc.reject && requests.Load() != 0 {
				t.Fatal("unsupported setting sent to generation")
			}
		})
	}
}
