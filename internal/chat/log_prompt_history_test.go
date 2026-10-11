package chat

import (
	"context"
	"strings"
	"testing"
)

func TestPromptHistoryByteAdmissionNeverReturnsPartialText(t *testing.T) {
	l, id := recoveryLog(t)
	appendRecoveryMessage(t, l, "older")
	appendRecoveryMessage(t, l, strings.Repeat("日本語", 100000))
	appendRecoveryMessage(t, l, "recent café\nunchanged")
	got, omitted, err := l.RecentPromptHistoryContext(t.Context(), id, 512, 192000)
	if err != nil || omitted != 2 || len(got) != 1 || got[0].Content != "recent café\nunchanged" {
		t.Fatal("oversized message entered prompt or broke chronology", len(got), err)
	}
	if _, err := l.AppendPendingAssistantTo(id, "uncommitted", nil); err != nil {
		t.Fatal(err)
	}
	got, omitted, err = l.RecentPromptHistoryContext(t.Context(), id, 256, 96000)
	if err != nil || omitted != 2 || len(got) != 1 || got[0].Content != "recent café\nunchanged" {
		t.Fatal("pending reply entered history", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := l.RecentPromptHistoryContext(ctx, id, 256, 96000); err == nil {
		t.Fatal("canceled prompt read succeeded")
	}
}

func TestRecentDisplayWindowDoesNotFetchAllRetainedContext(t *testing.T) {
	l, id := recoveryLog(t)
	for i := 0; i < 250; i++ {
		appendRecoveryMessage(t, l, "conversation")
	}
	zero := int64(0)
	page, err := l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &zero, RecentOnly: true, Limit: 50})
	if err != nil || len(page.Messages) != 50 || page.Messages[0].Seq != 51 || !page.HasMore {
		t.Fatal("display did not start at the newest 200", page, err)
	}
	appendRecoveryMessage(t, l, "after the anchored snapshot")
	total := len(page.Messages)
	for page.Snapshot != nil {
		page, err = l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &page.NextRevision, Snapshot: page.Snapshot, RecentOnly: true, Limit: 50})
		if err != nil || page.Reset {
			t.Fatal("display snapshot shifted", page, err)
		}
		total += len(page.Messages)
	}
	if total != 200 || page.NextRevision != 250 {
		t.Fatal("display loaded hidden context", total, page)
	}
	page, err = l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &page.NextRevision, RecentOnly: true})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Content != "after the anchored snapshot" {
		t.Fatal("new tail was lost", page, err)
	}
	history, omitted, err := l.RecentPromptHistoryContext(t.Context(), id, 256, 96000)
	if err != nil || len(history) != 251 || omitted != 0 {
		t.Fatal("display window removed model context", len(history), omitted, err)
	}
}

func TestRecentDisplayRetainsLateCommittedReplyInsideWindow(t *testing.T) {
	l, id := recoveryLog(t)
	for i := 0; i < 200; i++ {
		appendRecoveryMessage(t, l, "before pending reply")
	}
	pending, err := l.AppendPendingAssistantTo(id, "late visible reply", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		appendRecoveryMessage(t, l, "newer user turn")
	}
	zero := int64(0)
	page, err := l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &zero, RecentOnly: true})
	if err != nil || page.HasMore {
		t.Fatal(page, err)
	}
	if err := l.CommitPending(pending); err != nil {
		t.Fatal(err)
	}
	page, err = l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &page.NextRevision, RecentOnly: true})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Seq != pending {
		t.Fatal("late reply lost below display head", page, err)
	}
}
