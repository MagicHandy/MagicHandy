package chat

import (
	"errors"
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

func cappedTestSession(t *testing.T) (*MessageLog, string) {
	t.Helper()
	log := openTestLog(t)
	sessionID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	for index := 0; index < MessageLogCap; index++ {
		if _, err := log.AppendTo(sessionID, MessageRoleUser, fmt.Sprintf("visible %d", index), "client", nil); err != nil {
			t.Fatalf("seed visible message %d: %v", index, err)
		}
	}
	return log, sessionID
}

func TestPendingAssistantReplyDoesNotEvictOrAffectSessionContext(t *testing.T) {
	log, sessionID := cappedTestSession(t)
	pendingDiagnostics := &MessageDiagnostics{Mood: MoodTeasing, MoodChanged: true}
	pending, err := log.AppendPendingAssistantTo(sessionID, "pending reply", pendingDiagnostics)
	if err != nil {
		t.Fatalf("append pending reply: %v", err)
	}

	visible, err := log.AfterSession(sessionID, 0, 0)
	if err != nil {
		t.Fatalf("read while pending: %v", err)
	}
	if len(visible) != MessageLogCap || visible[0].Content != "visible 0" || visible[len(visible)-1].Content != fmt.Sprintf("visible %d", MessageLogCap-1) {
		t.Fatalf("pending reply changed visible capped history: len=%d first=%q last=%q", len(visible), visible[0].Content, visible[len(visible)-1].Content)
	}
	context, err := log.PromptContext(sessionID)
	if err != nil {
		t.Fatalf("prompt context while pending: %v", err)
	}
	if context.CurrentMood != "" || len(context.RecentAssistantReplies) != 0 {
		t.Fatalf("pending reply entered prompt context: %+v", context)
	}

	if err := log.Delete(pending); err != nil {
		t.Fatalf("roll back pending reply: %v", err)
	}
	visible, err = log.AfterSession(sessionID, 0, 0)
	if err != nil || len(visible) != MessageLogCap || visible[0].Content != "visible 0" {
		t.Fatalf("pending rollback did not preserve history: len=%d first=%q err=%v", len(visible), visible[0].Content, err)
	}
	context, err = log.PromptContext(sessionID)
	if err != nil || context.CurrentMood != "" {
		t.Fatalf("pending rollback changed mood context: %+v, err=%v", context, err)
	}
}

func TestPendingAssistantCommitAppliesCapAndMoodAtomically(t *testing.T) {
	log, sessionID := cappedTestSession(t)
	pending, err := log.AppendPendingAssistantTo(sessionID, "committed reply", &MessageDiagnostics{Mood: MoodTeasing, MoodChanged: true})
	if err != nil {
		t.Fatalf("append pending reply: %v", err)
	}
	if err := log.CommitPending(pending); err != nil {
		t.Fatalf("commit pending reply: %v", err)
	}
	visible, err := log.AfterSession(sessionID, 0, 0)
	if err != nil {
		t.Fatalf("read committed reply: %v", err)
	}
	if len(visible) != MessageLogCap || visible[0].Content != "visible 1" || visible[len(visible)-1].Content != "committed reply" {
		t.Fatalf("committed reply did not apply cap atomically: len=%d first=%q last=%q", len(visible), visible[0].Content, visible[len(visible)-1].Content)
	}
	context, err := log.PromptContext(sessionID)
	if err != nil || context.CurrentMood != MoodTeasing {
		t.Fatalf("committed mood context = %+v, err=%v", context, err)
	}
}

func TestMessageLogRecentReturnsBoundedChronologicalTail(t *testing.T) {
	log := openTestLog(t)
	for i := 0; i < 6; i++ {
		if _, err := log.Append(MessageRoleUser, fmt.Sprintf("message %d", i), "c"); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	recent, err := log.Recent(3)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 3 || recent[0].Content != "message 3" || recent[2].Content != "message 5" {
		t.Fatalf("recent tail = %+v, want messages 3..5 in chronological order", recent)
	}
}

func TestMessageLogPromptContextIsSessionScopedBoundedAndChronological(t *testing.T) {
	log := openTestLog(t)
	sessionID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	appendAssistant := func(content string, mood Mood) {
		t.Helper()
		var diagnostics *MessageDiagnostics
		if mood != "" {
			diagnostics = &MessageDiagnostics{Mood: mood}
		}
		if _, err := log.AppendTo(sessionID, MessageRoleAssistant, content, "", diagnostics); err != nil {
			t.Fatalf("append assistant: %v", err)
		}
	}
	appendAssistant("excluded oldest", MoodCurious)
	if _, err := log.AppendTo(sessionID, MessageRoleUser, "user text must not count", "client", nil); err != nil {
		t.Fatalf("append user: %v", err)
	}
	appendAssistant("second line", "")
	longLine := strings.Repeat("界", maxRecentAssistantRunes+10)
	appendAssistant(longLine, MoodTeasing)
	appendAssistant(" latest\nline\tcollapsed ", "")

	context, err := log.PromptContext(sessionID)
	if err != nil {
		t.Fatalf("PromptContext: %v", err)
	}
	if context.CurrentMood != MoodTeasing {
		t.Fatalf("current mood = %q, want %q", context.CurrentMood, MoodTeasing)
	}
	if len(context.RecentAssistantReplies) != maxRecentAssistantReplies {
		t.Fatalf("recent replies = %v", context.RecentAssistantReplies)
	}
	if context.RecentAssistantReplies[0] != "second line" || context.RecentAssistantReplies[2] != "latest line collapsed" {
		t.Fatalf("recent reply order/content = %v", context.RecentAssistantReplies)
	}
	if len([]rune(context.RecentAssistantReplies[1])) != maxRecentAssistantRunes {
		t.Fatalf("bounded reply length = %d", len([]rune(context.RecentAssistantReplies[1])))
	}

	if _, err := log.SaveSession(sessionID); err != nil {
		t.Fatalf("save session: %v", err)
	}
	second, err := log.CreateSession(false)
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	secondContext, err := log.PromptContext(second.ID)
	if err != nil {
		t.Fatalf("second PromptContext: %v", err)
	}
	if secondContext.CurrentMood != "" || len(secondContext.RecentAssistantReplies) != 0 {
		t.Fatalf("second session leaked context: %+v", secondContext)
	}
}

func TestMessageLogExplicitMoodChangeWinsOverStaleCarriedMood(t *testing.T) {
	log := openTestLog(t)
	sessionID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	appendMood := func(reply string, mood Mood, changed bool) {
		t.Helper()
		if _, err := log.AppendTo(sessionID, MessageRoleAssistant, reply, "", &MessageDiagnostics{Mood: mood, MoodChanged: changed}); err != nil {
			t.Fatalf("append mood: %v", err)
		}
	}
	appendMood("Initial mood.", MoodCurious, true)
	appendMood("New interactive mood.", MoodTeasing, true)
	appendMood("Late stale autonomous line.", MoodCurious, false)

	context, err := log.PromptContext(sessionID)
	if err != nil {
		t.Fatalf("PromptContext: %v", err)
	}
	if context.CurrentMood != MoodTeasing {
		t.Fatalf("current mood = %q, want latest explicit change %q", context.CurrentMood, MoodTeasing)
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

func TestChatSessionsKeepMessagesAndCursorsIsolated(t *testing.T) {
	log := openTestLog(t)
	pair := seedChatSessionPair(t, log)

	firstMessages, err := log.AfterSession(pair.firstID, 0, 0)
	if err != nil || len(firstMessages) != 1 || firstMessages[0].Seq != pair.firstSeq {
		t.Fatalf("first messages = %+v, %v", firstMessages, err)
	}
	if firstMessages[0].Diagnostics == nil || firstMessages[0].Diagnostics.Model != "gemma" {
		t.Fatalf("message diagnostics were not retained: %+v", firstMessages[0])
	}
	secondMessages, err := log.AfterSession(pair.second.ID, 0, 0)
	if err != nil || len(secondMessages) != 1 || secondMessages[0].Seq != pair.secondSeq {
		t.Fatalf("second messages = %+v, %v", secondMessages, err)
	}

	if _, err := log.AdvanceCursorSession("client", pair.firstID, pair.firstSeq); err != nil {
		t.Fatalf("advance first cursor: %v", err)
	}
	firstCursor, _ := log.CursorSession("client", pair.firstID)
	secondCursor, _ := log.CursorSession("client", pair.second.ID)
	if firstCursor != pair.firstSeq || secondCursor != 0 {
		t.Fatalf("per-session cursors = first:%d second:%d", firstCursor, secondCursor)
	}
}

type chatSessionPair struct {
	firstID   string
	firstSeq  int64
	second    Session
	secondSeq int64
}

func seedChatSessionPair(t *testing.T, log *MessageLog) chatSessionPair {
	t.Helper()
	firstID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	firstSeq, err := log.AppendTo(firstID, MessageRoleUser, "A useful first conversation", "client", &MessageDiagnostics{Source: "interactive", Model: "gemma"})
	if err != nil {
		t.Fatalf("append first session: %v", err)
	}
	first, err := log.SaveSession(firstID)
	if err != nil {
		t.Fatalf("save first session: %v", err)
	}
	if !first.Saved || first.Title != "A useful first conversation" {
		t.Fatalf("saved session = %+v", first)
	}

	second, err := log.CreateSession(false)
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	if !second.Active || second.Saved || second.ID == firstID {
		t.Fatalf("new session = %+v", second)
	}
	secondSeq, err := log.AppendTo(second.ID, MessageRoleAssistant, "Second session reply", "", nil)
	if err != nil {
		t.Fatalf("append second session: %v", err)
	}
	return chatSessionPair{firstID: firstID, firstSeq: firstSeq, second: second, secondSeq: secondSeq}
}

func TestChatStartupPolicyDropsUnsavedUnlessExplicitlyKept(t *testing.T) {
	log := openTestLog(t)
	oldID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := log.Append("user", "private transient conversation", "client"); err != nil {
		t.Fatalf("append transient chat: %v", err)
	}

	replacement, err := log.ReconcileStartup("previous", false)
	if err != nil {
		t.Fatalf("reconcile private startup: %v", err)
	}
	if replacement.ID == oldID || replacement.MessageCount != 0 || replacement.Saved {
		t.Fatalf("replacement session = %+v", replacement)
	}
	if _, err := log.Session(oldID); !errors.Is(err, ErrChatSessionNotFound) {
		t.Fatalf("unsaved prior session error = %v, want not found", err)
	}

	if _, err := log.Append("user", "retain this draft", "client"); err != nil {
		t.Fatalf("append retained draft: %v", err)
	}
	kept, err := log.ReconcileStartup("previous", true)
	if err != nil {
		t.Fatalf("reconcile retained startup: %v", err)
	}
	if kept.ID != replacement.ID || kept.MessageCount != 1 {
		t.Fatalf("retained session = %+v", kept)
	}
}

func TestChatShutdownPolicyDropsUnsavedUnlessExplicitlyKept(t *testing.T) {
	log := openTestLog(t)
	discardedID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := log.Append("user", "discard on clean shutdown", "client"); err != nil {
		t.Fatalf("append discarded draft: %v", err)
	}
	if err := log.ReconcileShutdown(false); err != nil {
		t.Fatalf("reconcile private shutdown: %v", err)
	}
	if _, err := log.Session(discardedID); !errors.Is(err, ErrChatSessionNotFound) {
		t.Fatalf("discarded shutdown session error = %v, want not found", err)
	}

	keptID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("replacement session: %v", err)
	}
	if _, err := log.Append("user", "keep on clean shutdown", "client"); err != nil {
		t.Fatalf("append retained draft: %v", err)
	}
	if err := log.ReconcileShutdown(true); err != nil {
		t.Fatalf("reconcile retained shutdown: %v", err)
	}
	if kept, err := log.Session(keptID); err != nil || kept.MessageCount != 1 {
		t.Fatalf("retained shutdown session = %+v, %v", kept, err)
	}
}

func TestActivateSessionCanDiscardTheUnsavedWorkingTab(t *testing.T) {
	log := openTestLog(t)
	savedID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := log.Append("user", "saved conversation", "client"); err != nil {
		t.Fatalf("append saved chat: %v", err)
	}
	if _, err := log.SaveSession(savedID); err != nil {
		t.Fatalf("save session: %v", err)
	}

	draft, err := log.CreateSession(false)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if _, err := log.Append("user", "discard this draft", "client"); err != nil {
		t.Fatalf("append draft: %v", err)
	}
	if _, err := log.ActivateSession(savedID, false); !errors.Is(err, ErrUnsavedSessionConflict) {
		t.Fatalf("activate without resolving draft error = %v, want unsaved conflict", err)
	}
	if active, err := log.ActiveSessionID(); err != nil || active != draft.ID {
		t.Fatalf("active session after rejected switch = %q, %v; want %q", active, err, draft.ID)
	}
	if _, err := log.ActivateSession(savedID, true); err != nil {
		t.Fatalf("activate saved session: %v", err)
	}
	if _, err := log.Session(draft.ID); !errors.Is(err, ErrChatSessionNotFound) {
		t.Fatalf("discarded draft error = %v, want not found", err)
	}
}

func TestCreateSessionRequiresResolvingANonemptyUnsavedChat(t *testing.T) {
	log := openTestLog(t)
	oldID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := log.Append("user", "keep or discard me explicitly", "client"); err != nil {
		t.Fatalf("append draft: %v", err)
	}

	if _, err := log.CreateSession(false); !errors.Is(err, ErrUnsavedSessionConflict) {
		t.Fatalf("create without resolving draft error = %v, want unsaved conflict", err)
	}
	sessions, err := log.Sessions()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != oldID || !sessions[0].Active {
		t.Fatalf("sessions after rejected create = %+v", sessions)
	}

	replacement, err := log.CreateSession(true)
	if err != nil {
		t.Fatalf("create with discard: %v", err)
	}
	if replacement.ID == oldID || !replacement.Active {
		t.Fatalf("replacement session = %+v", replacement)
	}
}

func TestStartupKeepsAtMostOneUnsavedWorkingTab(t *testing.T) {
	log := openTestLog(t)
	staleID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := log.Append("user", "stale draft", "client"); err != nil {
		t.Fatalf("append stale draft: %v", err)
	}
	currentID := "current-draft"
	now := nowUTC()
	if _, err := log.db.SQL().Exec(`
		INSERT INTO chat_sessions(id, title, saved, created_at, updated_at)
		VALUES(?, 'New chat', 0, ?, ?)
	`, currentID, now, now); err != nil {
		t.Fatalf("insert legacy current draft: %v", err)
	}
	if _, err := log.db.SQL().Exec(`UPDATE chat_workspace SET active_session_id = ?, updated_at = ? WHERE id = 'current'`, currentID, now); err != nil {
		t.Fatalf("select legacy current draft: %v", err)
	}
	if _, err := log.AppendTo(currentID, "user", "current draft", "client", nil); err != nil {
		t.Fatalf("append current draft: %v", err)
	}

	kept, err := log.ReconcileStartup("previous", true)
	if err != nil {
		t.Fatalf("reconcile startup: %v", err)
	}
	if kept.ID != currentID {
		t.Fatalf("active session = %q, want %q", kept.ID, currentID)
	}
	if _, err := log.Session(staleID); !errors.Is(err, ErrChatSessionNotFound) {
		t.Fatalf("inactive unsaved session error = %v, want not found", err)
	}
}
