package llm

import (
	"strings"
	"testing"
)

func TestProviderProgressSeparatesThinkingFromVisibleText(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		read func(string, func(string) error, func(ProviderProgress)) (string, error)
	}{
		{
			name: "llama.cpp",
			body: "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"private reasoning\"}}]}\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\"Ready\"}}]}\n" +
				"data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":41,\"completion_tokens\":7},\"timings\":{\"prompt_ms\":123.5,\"predicted_ms\":70.2,\"predicted_n\":7}}\n" +
				"data: [DONE]\n",
			read: func(body string, delta func(string) error, progress func(ProviderProgress)) (string, error) {
				return readOpenAIEventStream(strings.NewReader(body), delta, progress)
			},
		},
		{
			name: "ollama",
			body: "{\"message\":{\"thinking\":\"private reasoning\"}}\n" +
				"{\"message\":{\"content\":\"Ready\"}}\n" +
				"{\"done\":true,\"load_duration\":10000000,\"prompt_eval_duration\":123500000,\"prompt_eval_count\":41,\"eval_count\":7,\"eval_duration\":70200000}\n",
			read: func(body string, delta func(string) error, progress func(ProviderProgress)) (string, error) {
				return readOllamaStream(strings.NewReader(body), delta, progress)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var visible strings.Builder
			var updates []ProviderProgress
			result, err := test.read(test.body, func(text string) error {
				visible.WriteString(text)
				return nil
			}, func(update ProviderProgress) {
				if len(updates) == 0 && visible.Len() != 0 {
					t.Fatal("reasoning activity was delayed until visible text")
				}
				updates = append(updates, update)
			})
			if err != nil || result != "Ready" || visible.String() != "Ready" {
				t.Fatalf("visible result=%q deltas=%q err=%v", result, visible.String(), err)
			}
			if len(updates) != 3 || !updates[0].Activity {
				t.Fatalf("progress=%+v", updates)
			}
			last := updates[len(updates)-1]
			if last.PromptEvalMillis != 123 || last.PromptTokens != 41 || last.GeneratedTokens != 7 {
				t.Fatalf("missing provider phase metrics: %+v", last)
			}
			if updates[0].ReasoningChars != len("private reasoning") || updates[1].ReasoningChars != 0 {
				t.Fatalf("reasoning counts = %d, %d", updates[0].ReasoningChars, updates[1].ReasoningChars)
			}
			if last.DecodeMillis != 70 || last.DecodeTokens != 7 {
				t.Fatalf("decode timing = %d ms, %d tokens", last.DecodeMillis, last.DecodeTokens)
			}
			if test.name == "ollama" && last.LoadMillis != 10 {
				t.Fatalf("load millis=%d", last.LoadMillis)
			}
		})
	}
}
