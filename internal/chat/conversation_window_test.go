package chat

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestHostedHistoryUsesWholeMessagesAndLocalBudgetIsUnchanged(t *testing.T) {
	history := []llm.Message{}
	for i := range 240 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		history = append(history, llm.Message{Role: role, Content: fmt.Sprintf("%d: multilingual café 日本語\n", i) + strings.Repeat("story ", 48)})
	}
	before := append([]llm.Message(nil), history...)
	long := Capabilities{PreserveConversationText: true, HistoryMessages: 256, HistoryBytes: 96000}
	messages := capabilityMessages("authoritative state", history, "latest request", long)
	if !reflect.DeepEqual(messages[1:len(messages)-1], history) {
		t.Fatal("long context altered or dropped retained dialogue")
	}
	standard := capabilityMessages("authoritative state", history, "latest request", Capabilities{PreserveConversationText: true})
	if len(standard) > PromptHistoryLimit+2 || len(standard) >= len(messages) {
		t.Fatal("standard budget was widened")
	}
	local := long
	local.PreserveConversationText = false
	if got, want := capabilityMessages("system", history, "latest", local), capabilityMessages("system", history, "latest", Capabilities{}); !reflect.DeepEqual(got, want) {
		t.Fatal("hosted budget changed local prompts")
	}
	long.HistoryBytes = 700
	bounded := capabilityMessages("system", history, "latest", long)
	if !reflect.DeepEqual(bounded[1:len(bounded)-1], history[len(history)-(len(bounded)-2):]) {
		t.Fatal("history was rewritten instead of removing whole oldest messages")
	}
	if !reflect.DeepEqual(before, history) {
		t.Fatal("caller history mutated")
	}
}

func BenchmarkLocalPromptWithLongRetainedHistory(b *testing.B) {
	for _, count := range []int{200, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			history := make([]llm.Message, count)
			for i := range history {
				history[i] = llm.Message{Role: "user", Content: strings.Repeat("a conversation turn ", 30)}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = capabilityMessages("system", history, "latest", Capabilities{})
			}
		})
	}
}
