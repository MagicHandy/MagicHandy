package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHistoryResponseHasByteBudget(t *testing.T) {
	l, id := recoveryLog(t)
	for index := 0; index < 20; index++ {
		appendRecoveryMessage(t, l, strings.Repeat("<&", 64<<10))
	}
	page := recoveryPage(t, l, id, 0, 0)
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 256<<10 {
		t.Fatalf("history page is %d bytes; exceeds 256 KiB budget", len(encoded))
	}
	var seen []int64
	var totalBytes, pages int
	for {
		for _, message := range page.Messages {
			if !message.ContentTruncated || len(message.Content) != MessagePreviewBytes || message.ContentBytes != 128<<10 {
				t.Fatalf("long content preview is not explicit: %+v", message)
			}
			seen = append(seen, message.Seq)
		}
		encoded, err = json.Marshal(page)
		if err != nil || len(encoded) > MessagePageMaxBytes {
			t.Fatalf("page bytes: %d, %v", len(encoded), err)
		}
		totalBytes += len(encoded)
		pages++
		if !page.HasMore {
			break
		}
		if pages > 20 {
			t.Fatal("snapshot continuation did not make progress")
		}
		page, err = l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &page.NextRevision, Snapshot: page.Snapshot})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 20 || page.NextRevision != 20 {
		t.Fatalf("lost delivered rows: %v", seen)
	}
	for index, seq := range seen {
		if seq != int64(index+1) {
			t.Fatalf("duplicate or missing row: %v", seen)
		}
	}
	t.Logf("20 x 128 KiB escaped messages: %d pages, %d total JSON bytes", pages, totalBytes)
	assertLargeCanonicalMessagesUnchanged(t, l, id)
}

func assertLargeCanonicalMessagesUnchanged(t *testing.T, l *MessageLog, id string) {
	t.Helper()
	stored, err := l.RecentSession(id, 20)
	if err != nil || len(stored) != 20 || len(stored[0].Content) != 128<<10 {
		t.Fatal("paging changed stored content")
	}
}

func TestPreviewPreservesUTF8AndBoundsDiagnostics(t *testing.T) {
	l, id := recoveryLog(t)
	content := strings.Repeat("界", MessagePreviewBytes)
	seq, err := l.AppendTo(id, MessageRoleAssistant, content, "", &MessageDiagnostics{Model: strings.Repeat("x", 100<<10)})
	if err != nil {
		t.Fatal(err)
	}
	page := recoveryPage(t, l, id, 0, 0)
	message := page.Messages[0]
	if !utf8.ValidString(message.Content) || !strings.HasPrefix(content, message.Content) || !message.ContentTruncated || !message.DiagnosticsOmitted || message.Diagnostics != nil {
		t.Fatal("unbounded diagnostics or invalid UTF-8 preview")
	}
	var restored strings.Builder
	for offset := int64(0); offset < int64(len(content)); {
		chunk, err := l.ReadMessageContentChunkContext(t.Context(), id, seq, message.Revision, offset)
		if err != nil || len(chunk.Data) > MessageContentChunkBytes || len(chunk.Data) == 0 || chunk.TotalBytes != int64(len(content)) {
			t.Fatalf("download chunk: %+v, %v", chunk, err)
		}
		restored.Write(chunk.Data)
		offset += int64(len(chunk.Data))
	}
	if restored.String() != content {
		t.Fatal("chunked content lost bytes at UTF-8 boundaries")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := l.ReadMessageContentChunkContext(ctx, id, seq, message.Revision, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled content read: %v", err)
	}
}

func TestSnapshotDefersNewCommitsIncludingEarlierDisplayRows(t *testing.T) {
	l, id := recoveryLog(t)
	appendRecoveryMessage(t, l, "first")
	pending, err := l.AppendPendingAssistantTo(id, "late", nil)
	if err != nil {
		t.Fatal(err)
	}
	appendRecoveryMessage(t, l, "second committed")
	appendRecoveryMessage(t, l, "third committed")
	first := recoveryPage(t, l, id, 0, 1)
	if err := l.CommitPending(pending); err != nil {
		t.Fatal(err)
	}
	page, err := l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &first.NextRevision, Snapshot: first.Snapshot})
	if err != nil || page.Reset || page.NextRevision != 3 || !page.HasMore || page.Snapshot != nil || len(page.Messages) != 2 {
		t.Fatalf("snapshot skipped to new head: %+v, %v", page, err)
	}
	late := recoveryPage(t, l, id, page.NextRevision, 0)
	if late.Reset || late.HasMore || len(late.Messages) != 1 || late.Messages[0].Seq != pending || late.NextRevision != 4 {
		t.Fatalf("late commit lost after snapshot: %+v", late)
	}
}

func TestSnapshotRestartsAfterRemovalBetweenPages(t *testing.T) {
	for _, removal := range []string{"delete", "prune"} {
		t.Run(removal, func(t *testing.T) {
			l, id := recoveryLog(t)
			for index := 0; index < MessageLogCap; index++ {
				appendRecoveryMessage(t, l, fmt.Sprintf("row %d", index))
			}
			page := recoveryPage(t, l, id, 0, 1)
			if removal == "delete" {
				if err := l.Delete(2); err != nil {
					t.Fatal(err)
				}
			} else {
				appendRecoveryMessage(t, l, "prunes first row")
			}
			next, err := l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &page.NextRevision, Snapshot: page.Snapshot, Limit: 1})
			if err != nil || !next.Reset || next.Snapshot == nil || next.Snapshot.Revision != 201 {
				t.Fatalf("removed snapshot stayed valid: %+v, %v", next, err)
			}
		})
	}
}

func TestSnapshotDetectsPruningBelowAnEarlierHighPrunedRevision(t *testing.T) {
	l, id := recoveryLog(t)
	pending, err := l.AppendPendingAssistantTo(id, "committed after the retained window passed it", nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MessageLogCap; index++ {
		appendRecoveryMessage(t, l, fmt.Sprint(index))
	}
	if err := l.CommitPending(pending); err != nil {
		t.Fatal(err)
	}
	page := recoveryPage(t, l, id, 0, 1)
	if page.Snapshot == nil || page.Snapshot.PrunedRevision != MessageLogCap+1 {
		t.Fatal("missing high pruning marker fixture")
	}
	appendRecoveryMessage(t, l, "prunes a much older revision")
	next, err := l.ReadMessagePageContext(t.Context(), MessagePageRequest{SessionID: id, AfterRevision: &page.NextRevision, Snapshot: page.Snapshot, Limit: 1})
	if err != nil || !next.Reset || next.FirstSeq != page.FirstSeq+1 {
		t.Fatalf("pruning below the marker kept an obsolete snapshot: %+v, %v", next, err)
	}
}

func BenchmarkChatHistoryBoundedPage(b *testing.B) {
	l, err := OpenMessageLog(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = l.Close() })
	id, err := l.ActiveSessionID()
	if err != nil {
		b.Fatal(err)
	}
	for index := 0; index < 20; index++ {
		if _, err := l.AppendTo(id, MessageRoleAssistant, strings.Repeat("<&", 64<<10), "", nil); err != nil {
			b.Fatal(err)
		}
	}
	zero := int64(0)
	request := MessagePageRequest{SessionID: id, AfterRevision: &zero}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		page, err := l.ReadMessagePageContext(b.Context(), request)
		if err != nil {
			b.Fatal(err)
		}
		encoded, err := json.Marshal(page)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(len(encoded)), "bytes/response")
	}
}
