package chat

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

func TestFailedRequestRollbackDoesNotExposeAnUncommittedID(t *testing.T) {
	log := openTestLog(t)
	session, err := log.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	for range FailureRetention {
		if _, err := log.SaveFailure(t.Context(), session, RequestFailure{Error: "retained"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.db.WithTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER fail_failure_pruning BEFORE DELETE ON chat_failures BEGIN SELECT RAISE(ABORT, 'test pruning failure'); END`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id, err := log.SaveFailure(t.Context(), session, RequestFailure{Error: "rolled back"})
	if err == nil || id != 0 {
		t.Fatalf("failed transaction exposed an ID: %d %v", id, err)
	}
	items, err := log.Failures(t.Context(), session)
	if err != nil || len(items) != FailureRetention || items[0].Error != "retained" {
		t.Fatalf("transaction was not rolled back: %+v %v", items, err)
	}
}

func TestFailedRequestsAreBoundedDurableAndOutsideConversation(t *testing.T) {
	directory := t.TempDir()
	log, err := OpenMessageLog(directory)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := log.ActiveSessionID()
	if err != nil {
		t.Fatal(err)
	}
	for index := range FailureRetention + 3 {
		_, err := log.SaveFailure(t.Context(), sessionID, RequestFailure{Error: fmt.Sprintf("failure %d", index), Details: &FailureContent{Raw: `{"edits":[]}`, Prompt: "behavior"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	log, err = OpenMessageLog(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	items, err := log.Failures(t.Context(), sessionID)
	if err != nil || len(items) != FailureRetention || items[0].Details != nil {
		t.Fatalf("retention: %d, %v", len(items), err)
	}
	if items[0].Error != "failure 26" || items[len(items)-1].Error != "failure 3" {
		t.Fatal("incorrect bounded order")
	}
	plain, err := log.Failure(t.Context(), sessionID, items[0].ID, false)
	if err != nil || plain.Details != nil {
		t.Fatal("content escaped into default report")
	}
	full, err := log.Failure(t.Context(), sessionID, items[0].ID, true)
	if err != nil || full.Details == nil || full.Details.Raw != `{"edits":[]}` {
		t.Fatalf("explicit content unavailable: %+v, %v", full, err)
	}
	checkFailureHistoryIsolation(t, log, sessionID)
}

func checkFailureHistoryIsolation(t *testing.T, log *MessageLog, sessionID string) {
	t.Helper()
	messages, err := log.RecentSession(sessionID, 200)
	if err != nil || len(messages) != 0 {
		t.Fatal("failed requests entered conversation history")
	}
	context, err := log.ReadPromptContext(t.Context(), sessionID)
	if err != nil || len(context.RecentAssistantReplies) != 0 {
		t.Fatal("failed requests entered continuity")
	}
	if err := log.Clear(); err != nil {
		t.Fatal(err)
	}
	items, err := log.Failures(t.Context(), sessionID)
	if err != nil || len(items) != 0 {
		t.Fatal("clearing chat retained diagnostic content")
	}
}

func TestFailedRequestStorageRejectsOversizedContent(t *testing.T) {
	log := openTestLog(t)
	sessionID, _ := log.ActiveSessionID()
	_, err := log.SaveFailure(t.Context(), sessionID, RequestFailure{Details: &FailureContent{Raw: strings.Repeat("x", 262145)}})
	if err == nil {
		t.Fatal("unbounded diagnostic content accepted")
	}
}
