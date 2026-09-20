package store

import "testing"

func TestPermanentControlMigrationPreservesTimedGrantAndForeignKeys(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.SQL().Exec(`INSERT INTO user_accounts(id,username,username_key,role,password_hash,created_at,updated_at)
		VALUES('owner','owner','owner','admin','synthetic','now','now'),('operator','operator','operator','operator','synthetic','now','now');
		DROP TABLE user_control_grants;
		CREATE TABLE user_control_grants (
			user_id TEXT PRIMARY KEY REFERENCES user_accounts(id) ON DELETE CASCADE,
			grant_id TEXT NOT NULL, issued_by TEXT NOT NULL REFERENCES user_accounts(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL, expires_at TEXT NOT NULL);
		INSERT INTO user_control_grants VALUES('operator','original','owner','2026-09-19T00:00:00Z','2026-09-19T01:00:00Z');
		PRAGMA user_version=25`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var grantID, expiry string
	if err = db.SQL().QueryRow(`SELECT grant_id, expires_at FROM user_control_grants WHERE user_id='operator'`).Scan(&grantID, &expiry); err != nil || grantID != "original" || expiry != "2026-09-19T01:00:00Z" {
		t.Fatalf("migration altered existing grant: %q %q %v", grantID, expiry, err)
	}
	if _, err = db.SQL().Exec(`UPDATE user_control_grants SET expires_at=NULL WHERE user_id='operator'; DELETE FROM user_accounts WHERE id='owner'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.SQL().QueryRow(`SELECT count(*) FROM user_control_grants`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("issuer deletion did not cascade: %d %v", count, err)
	}
}
