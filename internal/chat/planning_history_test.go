package chat

import (
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Planning replies are never dialogue, so an old human line stays the latest
// user turn. Marking its age keeps it from reading as a new reaction.
func TestPlanningHistoryMarksWhenEachHumanLineWasSaid(t *testing.T) {
	start := time.Unix(1000, 0)
	history := []llm.Message{
		{Role: MessageRoleUser, Content: "that's too much"},
		{Role: MessageRoleAssistant, Content: "Slowing down."},
		{Role: MessageRoleUser, Content: "better"},
	}
	at := []time.Time{start, start.Add(2 * time.Second), start.Add(100 * time.Second)}
	marked := PlanningHistory(history, at, start.Add(140*time.Second))
	if marked[0].Content != "[said 2 minutes ago] that's too much" || marked[1].Content != "Slowing down." || marked[2].Content != "[said 40 seconds ago] better" {
		t.Fatalf("marked history %+v", marked)
	}
	if history[0].Content != "that's too much" {
		t.Fatal("the caller's history was modified")
	}
	// An unknown time leaves the whole history as it was, never a guessed age.
	at[2] = time.Time{}
	if unmarked := PlanningHistory(history, at, start.Add(140*time.Second)); unmarked[0].Content != "that's too much" {
		t.Fatalf("history with an unknown time was marked: %+v", unmarked)
	}
}
