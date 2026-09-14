package store

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"testing"
)

func TestSessionManagementMigrationPreservesExistingLogins(t *testing.T) {
	dir := t.TempDir()
	seedPreSessionManagementDatabase(t, dir)
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rows, err := db.SQL().Query(`SELECT public_id, created_at, last_seen_at, expires_at FROM user_sessions`)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for rows.Next() {
		var id, created, seen, expires string
		if err := rows.Scan(&id, &created, &seen, &expires); err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil || len(decoded) != 16 || len(id) != 22 || ids[id] {
			t.Fatal("invalid or duplicate management identity")
		}
		if created != "created" || seen != "seen" || expires != "expiry" {
			t.Fatal("migration changed login lifetime")
		}
		ids[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 300 {
		t.Fatal("migration dropped logins")
	}
	assertSessionManagementReapplication(t, db)
}

func assertSessionManagementReapplication(t *testing.T, db *DB) {
	t.Helper()
	var priorID string
	if err := db.SQL().QueryRow(`SELECT public_id FROM user_sessions WHERE token_hash='synthetic-0'`).Scan(&priorID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`UPDATE user_sessions SET device_name='preserved device' WHERE token_hash='synthetic-0'`); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return migrateSessionManagement(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
	var afterID, name string
	if err := db.SQL().QueryRow(`SELECT public_id,device_name FROM user_sessions WHERE token_hash='synthetic-0'`).Scan(&afterID, &name); err != nil {
		t.Fatal(err)
	}
	if afterID != priorID || name != "preserved device" {
		t.Fatal("reapplication replaced session management metadata")
	}
}

func seedPreSessionManagementDatabase(t *testing.T, dir string) {
	t.Helper()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, statement := range []string{
		`DROP INDEX user_sessions_public_id`,
		`ALTER TABLE user_sessions DROP COLUMN public_id`,
		`ALTER TABLE user_sessions DROP COLUMN device_name`,
		`ALTER TABLE user_sessions DROP COLUMN client_browser`,
		`ALTER TABLE user_sessions DROP COLUMN client_platform`,
		`PRAGMA user_version = 21`,
		`INSERT INTO user_accounts(id, username, username_key, role, password_hash, disabled, last_login_at, created_at, updated_at)
		 VALUES('owner', 'owner', 'owner', 'admin', 'synthetic migration hash', 0, '', 'created', 'updated')`,
	} {
		if _, err := db.SQL().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 300; index++ { // crosses the bounded backfill batch
		if _, err := db.SQL().Exec(`INSERT INTO user_sessions(token_hash,user_id,created_at,last_seen_at,expires_at) VALUES(?,'owner','created','seen','expiry')`, fmt.Sprint("synthetic-", index)); err != nil {
			t.Fatal(err)
		}
	}
}
