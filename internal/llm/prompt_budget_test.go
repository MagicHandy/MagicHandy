package llm

import (
	"reflect"
	"strings"
	"testing"
)

func TestBudgetDropsOldHistoryAndPreservesRepairExchange(t *testing.T) {
	request := ChatRequest{MaxTokens: 256, PreserveTailMessages: 3, Messages: []Message{
		{Role: "system", Content: "immutable control rules"},
		{Role: "user", Content: strings.Repeat("old history", 900)},
		{Role: "assistant", Content: "old reply"},
		{Role: "user", Content: "current request"},
		{Role: "assistant", Content: "malformed response"},
		{Role: "user", Content: "repair instructions"},
	}}
	before := append([]Message(nil), request.Messages...)
	bounded, budget, err := BudgetChatRequest(request, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if budget.HistoryMessagesDropped != 1 || budget.InputBytes > budget.LimitBytes {
		t.Fatalf("budget: %+v", budget)
	}
	if bounded.Messages[0] != request.Messages[0] || !reflect.DeepEqual(bounded.Messages[len(bounded.Messages)-3:], request.Messages[3:]) {
		t.Fatalf("essential exchange changed: %+v", bounded.Messages)
	}
	if !reflect.DeepEqual(before, request.Messages) {
		t.Fatal("request mutated")
	}
}

func TestBudgetRejectsOversizedSystemInsteadOfTruncatingRules(t *testing.T) {
	request := ChatRequest{Messages: []Message{{Role: "system", Content: strings.Repeat("safety ", 2000)}, {Role: "user", Content: "hello"}}}
	if _, _, err := BudgetChatRequest(request, 4096); err == nil {
		t.Fatal("oversized essential prompt accepted")
	}
}
