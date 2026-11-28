package chat

import (
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExistingChatMigratesWithNoEdgeHistory(t *testing.T) {
	dir := t.TempDir()
	old, err := OpenMessageLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := old.EdgeChatAnchor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Append(MessageRoleUser, "saved before update", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = old.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	if err = old.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, e := tx.Exec("DROP TABLE chat_edge_holds"); e != nil {
			return e
		}
		_, e := tx.Exec("PRAGMA user_version=31")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenMessageLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	messages, err := upgraded.RecentSession(session, 10)
	if err != nil || len(messages) != 1 || messages[0].Content != "saved before update" {
		t.Fatal("lost existing chat", err)
	}
	if obs, err := upgraded.LatestEdge(t.Context(), session); err != nil || obs != nil {
		t.Fatal("invented prior Edge", obs, err)
	}
}

func TestEdgeEvidenceIsBoundToPressConversationAndCommittedUserLines(t *testing.T) {
	log := openTestLog(t)
	ctx := t.Context()
	add := func(role, content string) int64 {
		t.Helper()
		n, err := log.Append(role, content, "test")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	add(MessageRoleUser, "before the button")
	session, seq, err := log.EdgeChatAnchor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	if err = log.RecordEdge(ctx, session, at, seq, "moving", 20); err != nil {
		t.Fatal(err)
	}
	if err = log.RecordEdgePlan(ctx, session, at, EdgePlan{HoldSeconds: 33, ResumeSpeed: 27, ResumeStyle: "gentle"}); err != nil {
		t.Fatal(err)
	}
	add(MessageRoleAssistant, "assistant invented a duration")
	add(MessageRoleUser, "that was too close")
	add(MessageRoleUser, strings.Repeat("x", 385))
	for _, v := range []string{"second", "middle1", "Next Edge should give me longer", "middle3", "late1", "late2", "latest correction"} {
		add(MessageRoleUser, v)
	}
	_, through, _ := log.EdgeChatAnchor(ctx)
	next := at.Add(time.Second)
	if err = log.RecordEdge(ctx, session, next, through, "moving", 21); err != nil {
		t.Fatal(err)
	}
	add(MessageRoleUser, "after next press")
	feedback, err := log.EdgeFeedbackBefore(ctx, session, next, through)
	if err != nil || feedback == nil || feedback.Observation.PlannedSeconds != 33 || !slices.Equal(feedback.Remarks, []string{"that was too close", "second", "Next Edge should give me longer", "late1", "late2", "latest correction"}) {
		t.Fatalf("feedback %+v %v", feedback, err)
	}
	if missing, err := log.EdgeFeedbackBefore(ctx, session, next.Add(time.Millisecond), through); err != nil || missing != nil {
		t.Fatal("missing press borrowed earlier evidence")
	}
	if _, err = log.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	if _, err = log.CreateSession(false); err != nil {
		t.Fatal(err)
	}
	other, otherSeq, _ := log.EdgeChatAnchor(ctx)
	if observed, err := log.LatestEdge(ctx, other); err != nil || observed != nil {
		t.Fatal("observation crossed chat")
	}
	if notes, err := log.EdgeFeedbackBefore(ctx, other, next, otherSeq); err != nil || notes != nil {
		t.Fatal("feedback crossed chat")
	}
	checkEdgeRetention(t, log, session, next, through)
}

func checkEdgeRetention(t *testing.T, log *MessageLog, session string, next time.Time, through int64) {
	t.Helper()
	ctx := t.Context()
	var err error
	if err = log.RecordEdge(ctx, session, next.Add(time.Second), through, "still", 0); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err = log.db.SQL().QueryRow("SELECT COUNT(*) FROM chat_edge_holds WHERE session_id=?", session).Scan(&retained); err != nil || retained != 2 {
		t.Fatal("unbounded Edge retention", retained, err)
	}
	if err = log.DeleteSession(session); err != nil {
		t.Fatal(err)
	}
	if obs, err := log.LatestEdge(ctx, session); err != nil || obs != nil {
		t.Fatal("deleted chat retained button evidence")
	}
}
