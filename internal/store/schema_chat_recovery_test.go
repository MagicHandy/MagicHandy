package store

import (
	"database/sql"
	"testing"
)

func v20ChatRecoveryFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := db.SQL().QueryRow(`SELECT active_session_id FROM chat_workspace`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP INDEX messages_session_revision`, `DROP INDEX chat_cursors_updated`,
		`ALTER TABLE messages DROP COLUMN revision`,
		`ALTER TABLE chat_sessions DROP COLUMN revision`, `ALTER TABLE chat_sessions DROP COLUMN reset_revision`,
		`ALTER TABLE chat_sessions DROP COLUMN pruned_revision`, `ALTER TABLE chat_session_cursors DROP COLUMN last_revision`,
		`PRAGMA user_version = 20`,
	} {
		if _, err := db.SQL().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.SQL().Exec(`INSERT INTO messages(seq, session_id, role, content, created_at, committed)
		VALUES(10, ?, 'user', 'preserved', 'now', 1), (11, ?, 'assistant', 'pending', 'now', 0), (12, ?, 'user', 'newer', 'now', 1)`, id, id, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`INSERT INTO chat_session_cursors(client_id, session_id, last_seq, updated_at) VALUES('legacy-reader', ?, 10, 'now')`, id); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, id
}

func TestMigrationPreservesV20ChatAndSeedsCommittedRevisions(t *testing.T) {
	dir, id := v20ChatRecoveryFixture(t)
	upgraded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	for _, expected := range []struct{ seq, revision int64 }{{10, 10}, {11, 0}, {12, 12}} {
		var revision int64
		if err := upgraded.SQL().QueryRow(`SELECT revision FROM messages WHERE seq = ?`, expected.seq).Scan(&revision); err != nil || revision != expected.revision {
			t.Fatalf("message %d revision = %d, %v", expected.seq, revision, err)
		}
	}
	var head, cursor, cursorRevision int64
	if err := upgraded.SQL().QueryRow(`SELECT revision FROM chat_sessions WHERE id = ?`, id).Scan(&head); err != nil || head != 12 {
		t.Fatalf("head = %d, %v", head, err)
	}
	if err := upgraded.SQL().QueryRow(`SELECT last_seq, last_revision FROM chat_session_cursors WHERE client_id = 'legacy-reader'`).Scan(&cursor, &cursorRevision); err != nil || cursor != 10 || cursorRevision != 0 {
		t.Fatalf("cursor changed: %d/%d, %v", cursor, cursorRevision, err)
	}
	if err := upgraded.WithTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE messages SET revision = 20 WHERE seq = 10`); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE chat_sessions SET revision = 20 WHERE id = ?`, id); err != nil {
			return err
		}
		return migrateChatRecovery(t.Context(), tx)
	}); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.SQL().QueryRow(`SELECT revision FROM messages WHERE seq = 10`).Scan(&head); err != nil || head != 20 {
		t.Fatal("reapplied migration reset an existing committed revision")
	}
}

func TestChatRecoveryMigrationBoundsLegacyMarkers(t *testing.T) {
	dir, id := v20ChatRecoveryFixture(t)
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.SQL().Exec(`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value < ?)
		INSERT INTO chat_session_cursors(client_id, session_id, last_seq, updated_at)
		SELECT 'old-' || value, ?, 10, '2000-01-01T00:00:00Z' FROM n`, ChatCursorLimit, id); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return migrateChatRecovery(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
	var count, preserved int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM chat_session_cursors`).Scan(&count); err != nil || count != ChatCursorLimit {
		t.Fatalf("migration cursor count = %d, %v", count, err)
	}
	if err := db.SQL().QueryRow(`SELECT last_seq FROM chat_session_cursors WHERE client_id = 'legacy-reader'`).Scan(&preserved); err != nil || preserved != 10 {
		t.Fatalf("migration lost the recent legacy marker: %d, %v", preserved, err)
	}
}
