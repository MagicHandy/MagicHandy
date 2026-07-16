package chat

import (
	"fmt"
	"strings"
	"testing"
)

func openTestLog(t *testing.T) *MessageLog {
	t.Helper()
	log, err := OpenMessageLog(t.TempDir())
	if err != nil {
		t.Fatalf("OpenMessageLog: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	return log
}

func TestMessageLogAppendAndReadAfter(t *testing.T) {
	log := openTestLog(t)

	first, err := log.Append(MessageRoleUser, "hello", "client-a")
	if err != nil {
		t.Fatalf("append user: %v", err)
	}
	second, err := log.Append(MessageRoleAssistant, "hi there", "")
	if err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	if second <= first {
		t.Fatalf("sequence must increase: %d then %d", first, second)
	}

	all, err := log.After(0, 0)
	if err != nil {
		t.Fatalf("After(0): %v", err)
	}
	if len(all) != 2 || all[0].Content != "hello" || all[1].Content != "hi there" {
		t.Fatalf("unexpected log contents: %+v", all)
	}

	tail, err := log.After(first, 0)
	if err != nil {
		t.Fatalf("After(first): %v", err)
	}
	if len(tail) != 1 || tail[0].Seq != second {
		t.Fatalf("After must return only newer messages, got %+v", tail)
	}

	latest, err := log.LatestSeq()
	if err != nil || latest != second {
		t.Fatalf("LatestSeq = %d, %v; want %d", latest, err, second)
	}
	if err := log.Delete(second); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	tail, err = log.After(first, 0)
	if err != nil || len(tail) != 0 {
		t.Fatalf("deleted message remains visible: %+v, %v", tail, err)
	}
}

func TestMessageLogRejectsEmptyAndBadRoles(t *testing.T) {
	log := openTestLog(t)

	if _, err := log.Append(MessageRoleAssistant, "   ", ""); err == nil {
		t.Fatal("empty content must be rejected — errors never become blank chat rows")
	}
	if _, err := log.Append("system", "x", ""); err == nil {
		t.Fatal("unknown roles must be rejected")
	}
}

func TestMessageLogPrunesToCap(t *testing.T) {
	log := openTestLog(t)

	total := MessageLogCap + 25
	for i := 0; i < total; i++ {
		if _, err := log.Append(MessageRoleUser, fmt.Sprintf("m%d", i), "c"); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	all, err := log.After(0, 0)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	if len(all) != MessageLogCap {
		t.Fatalf("log length = %d, want cap %d", len(all), MessageLogCap)
	}
	if !strings.HasSuffix(all[len(all)-1].Content, fmt.Sprint(total-1)) {
		t.Fatalf("newest message must survive pruning, got %q", all[len(all)-1].Content)
	}
}

func TestCursorsAreIsolatedPerClient(t *testing.T) {
	log := openTestLog(t)

	seqA, _ := log.Append(MessageRoleUser, "one", "a")
	seqB, err := log.Append(MessageRoleAssistant, "two", "")
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	if _, err := log.AdvanceCursor("client-a", seqB); err != nil {
		t.Fatalf("advance a: %v", err)
	}

	// Client A consumed everything; client B still sees the whole log.
	cursorA, _ := log.Cursor("client-a")
	cursorB, _ := log.Cursor("client-b")
	if cursorA != seqB || cursorB != 0 {
		t.Fatalf("cursors = a:%d b:%d, want a:%d b:0", cursorA, cursorB, seqB)
	}
	unseenB, err := log.After(cursorB, 0)
	if err != nil {
		t.Fatalf("After for b: %v", err)
	}
	if len(unseenB) != 2 {
		t.Fatalf("client b must still see both messages, got %d", len(unseenB))
	}
	unseenA, _ := log.After(cursorA, 0)
	if len(unseenA) != 0 {
		t.Fatalf("client a acked everything, got %d unseen", len(unseenA))
	}
	_ = seqA
}

func TestCursorNeverMovesBackward(t *testing.T) {
	log := openTestLog(t)
	for i := 0; i < 7; i++ {
		if _, err := log.Append(MessageRoleUser, fmt.Sprintf("message %d", i), "c"); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	if _, err := log.AdvanceCursor("c", 7); err != nil {
		t.Fatalf("advance to 7: %v", err)
	}
	stored, err := log.AdvanceCursor("c", 3)
	if err != nil {
		t.Fatalf("advance to 3: %v", err)
	}
	if stored != 7 {
		t.Fatalf("cursor moved backward to %d; must stay at 7", stored)
	}
}

func TestCursorCannotAdvancePastCurrentLogHead(t *testing.T) {
	log := openTestLog(t)
	first, err := log.Append(MessageRoleUser, "first", "c")
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	stored, err := log.AdvanceCursor("c", first+1000)
	if err != nil {
		t.Fatalf("advance beyond head: %v", err)
	}
	if stored != first {
		t.Fatalf("cursor = %d, want current head %d", stored, first)
	}
	second, err := log.Append(MessageRoleAssistant, "second", "")
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	unseen, err := log.After(stored, 0)
	if err != nil {
		t.Fatalf("read after clamped cursor: %v", err)
	}
	if len(unseen) != 1 || unseen[0].Seq != second {
		t.Fatalf("future message was skipped after cursor clamp: %+v", unseen)
	}
}
