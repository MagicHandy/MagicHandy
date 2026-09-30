package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestInterfaceMigrationPreservesExistingAccountsAndSessions(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.SQL().Exec(`INSERT INTO user_accounts(id,username,username_key,role,password_hash,created_at,updated_at) VALUES('owner','Owner','owner','admin','preserved-hash','before','before');
	INSERT INTO user_sessions(token_hash,user_id,created_at,last_seen_at,expires_at,public_id) VALUES('preserved-token-digest','owner','before','before','later','preserved-public-id')`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", filepath.Join(dir, DatabaseFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`ALTER TABLE user_sessions DROP COLUMN interface; ALTER TABLE user_accounts DROP COLUMN interface_access; PRAGMA user_version=28`); err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		db, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		var access, audience, hash, key, publicID string
		err = db.SQL().QueryRow(`SELECT a.interface_access,s.interface,a.password_hash,s.token_hash,s.public_id FROM user_accounts a JOIN user_sessions s ON s.user_id=a.id`).Scan(&access, &audience, &hash, &key, &publicID)
		if err != nil || access != "full" || audience != "full" || hash != "preserved-hash" || key != "preserved-token-digest" || publicID != "preserved-public-id" {
			t.Fatalf("migration changed account state: %q %q %v", access, audience, err)
		}
		if _, err = db.SQL().Exec(`PRAGMA user_version=28`); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
