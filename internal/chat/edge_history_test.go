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
	if err = log.db.SQL().QueryRow("SELECT COUNT(*) FROM chat_edge_holds WHERE session_id=?", session).Scan(&retained); err != nil || retained != 3 {
		t.Fatal("unbounded Edge retention", retained, err)
	}
	if err = log.DeleteSession(session); err != nil {
		t.Fatal(err)
	}
	if obs, err := log.LatestEdge(ctx, session); err != nil || obs != nil {
		t.Fatal("deleted chat retained button evidence")
	}
}

func TestStillPressesPreserveFeedbackForNextMovingEdge(t *testing.T) {
	log := openTestLog(t)
	ctx := t.Context()
	id, seq, err := log.EdgeChatAnchor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	for i := 0; i < 3; i++ {
		if err := log.RecordEdge(ctx, id, at.Add(time.Duration(i)*time.Second), seq, "moving", 20); err != nil {
			t.Fatal(err)
		}
	}
	const wish = "Next Edge, give me 90 seconds."
	seq, err = log.Append(MessageRoleUser, wish, "test")
	if err != nil {
		t.Fatal(err)
	}
	for i := 3; i < 6; i++ {
		if err = log.RecordEdge(ctx, id, at.Add(time.Duration(i)*time.Second), seq, "still", 0); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := log.LatestEdge(ctx, id)
	if err != nil || latest == nil || latest.Kind != "still" {
		t.Fatalf("read-back lost last press: %+v %v", latest, err)
	}
	next := at.Add(6 * time.Second)
	if err = log.RecordEdge(ctx, id, next, seq, "moving", 20); err != nil {
		t.Fatal(err)
	}
	feedback, err := log.EdgeFeedbackBefore(ctx, id, next, seq)
	if err != nil || feedback == nil || feedback.Observation.Kind != "moving" || !slices.Equal(feedback.Remarks, []string{wish}) {
		t.Fatalf("intervening still holds consumed feedback: %+v %v", feedback, err)
	}
	var count int
	if err = log.db.SQL().QueryRow("SELECT COUNT(*) FROM chat_edge_holds WHERE session_id=?", id).Scan(&count); err != nil || count != 3 {
		t.Fatalf("retained %d rows, err=%v", count, err)
	}
}

func TestEdgeFeedbackIncludesLocalizedButtonNamesInTheMiddle(t *testing.T) {
	for _, note := range []string{"Al límite, dame más tiempo.", "No limite, espere mais.", "次の寸止めは長めに。", "下次边缘控制请稍久一点。"} {
		t.Run(note, func(t *testing.T) {
			log := openTestLog(t)
			ctx := t.Context()
			id, seq, err := log.EdgeChatAnchor(ctx)
			if err != nil {
				t.Fatal(err)
			}
			at := time.Now()
			if err = log.RecordEdge(ctx, id, at, seq, "moving", 20); err != nil {
				t.Fatal(err)
			}
			for _, line := range []string{"one", "two", note, "three", "four", "five", "six"} {
				if seq, err = log.Append(MessageRoleUser, line, "test"); err != nil {
					t.Fatal(err)
				}
			}
			next := at.Add(time.Second)
			if err = log.RecordEdge(ctx, id, next, seq, "motor_tempo", 15); err != nil {
				t.Fatal(err)
			}
			feedback, err := log.EdgeFeedbackBefore(ctx, id, next, seq)
			if err != nil || feedback == nil || !slices.Contains(feedback.Remarks, note) {
				t.Fatal("lost localized reference", feedback, err)
			}
		})
	}
}

func TestV32StillObservationsMigrateToLatestReadback(t *testing.T) {
	dir := t.TempDir()
	log, err := OpenMessageLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := log.EdgeChatAnchor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = log.SaveSession(id); err != nil {
		t.Fatal(err)
	}
	err = log.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("DROP TRIGGER trim_chat_edge_holds"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO chat_edge_holds(session_id,pressed_ns,after_seq,kind,holding_pace) VALUES(?,1,0,'still',0),(?,2,0,'still',0)`, id, id); err != nil {
			return err
		}
		_, err := tx.Exec("PRAGMA user_version=32")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = log.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenMessageLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	latest, err := reopened.LatestEdge(t.Context(), id)
	if err != nil || latest == nil || latest.PressedAt.UnixNano() != 2 {
		t.Fatal("lost latest v32 button action", latest, err)
	}
	var n int
	if err = reopened.db.SQL().QueryRow("SELECT count(*) FROM chat_edge_holds WHERE session_id=?", id).Scan(&n); err != nil || n != 1 {
		t.Fatal("old still observation survived", n, err)
	}
}
