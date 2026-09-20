package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNoticeMigrationPreservesOtherDataAndEnforcesBrowserBounds(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL().Exec(`INSERT INTO app_kv(key,value,updated_at) VALUES('notice-migration-fixture','kept','now'); DROP TABLE notice_preferences; PRAGMA user_version=24`); err != nil {
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
	var value string
	if err = db.SQL().QueryRow(`SELECT value FROM app_kv WHERE key='notice-migration-fixture'`).Scan(&value); err != nil || value != "kept" {
		t.Fatal("migration replaced existing data", err)
	}
	tx, err := db.SQL().Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for i := range 2048 {
		if _, err = tx.Exec(`INSERT INTO notice_preferences(owner_key,hidden_json,updated_at) VALUES(?,'["sign-in-safety"]',1)`, fmt.Sprintf("browser:%064x", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(`INSERT INTO notice_preferences(owner_key,hidden_json,updated_at) VALUES(?,'[]',1)`, "browser:"+strings.Repeat("f", 64)); err == nil {
		t.Fatal("anonymous profile limit not enforced")
	}
	if _, err = tx.Exec(`INSERT INTO notice_preferences(owner_key,hidden_json,updated_at) VALUES(?,'[]',2) ON CONFLICT(owner_key) DO UPDATE SET hidden_json=excluded.hidden_json`, fmt.Sprintf("browser:%064x", 0)); err != nil {
		t.Fatal("existing browser cannot update at capacity", err)
	}
	if _, err = tx.Exec(`UPDATE notice_preferences SET hidden_json='not JSON' WHERE owner_key=?`, fmt.Sprintf("browser:%064x", 0)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err = tx.Exec(`UPDATE notice_preferences SET hidden_json=? WHERE owner_key=?`, `[`+strings.Repeat(`"x",`, 16)+`"x"]`, fmt.Sprintf("browser:%064x", 0)); err == nil {
		t.Fatal("oversized array accepted")
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestNoticeStorageRejectsMissingLimitAndCascadesAccountDeletion(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.SQL().Exec(`INSERT INTO user_accounts(id,username,username_key,role,password_hash,created_at,updated_at) VALUES('notice-user','notice-user','notice-user','operator','synthetic hash','now','now');
		INSERT INTO notice_preferences(owner_key,account_id,hidden_json,updated_at) VALUES('account:notice-user','notice-user','["model-generation"]',1);
		DELETE FROM user_accounts WHERE id='notice-user'`)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.SQL().QueryRow(`SELECT count(*) FROM notice_preferences`).Scan(&count); err != nil || count != 0 {
		t.Fatal("account preference did not cascade", err)
	}
	if _, err = db.SQL().Exec(`DROP TRIGGER notice_browser_limit`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	broken, err := Open(dir)
	if broken != nil {
		_ = broken.Close()
	}
	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("missing bound accepted: %v", err)
	}
}
