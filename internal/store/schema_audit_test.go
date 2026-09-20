package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestAuditMigrationPreservesAccountsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE access_audit`, `PRAGMA user_version=22`,
		`INSERT INTO user_accounts(id,username,username_key,role,password_hash,disabled,last_login_at,created_at,updated_at) VALUES('audit-owner','audit-owner','audit-owner','admin','synthetic hash',0,'','created','updated')`,
	} {
		if _, err := db.SQL().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.SQL().Exec(`INSERT INTO access_audit(event_id,occurred_at,document) VALUES('migration-fixture',1,'{}')`); err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(t.Context(), func(tx *sql.Tx) error { return migrateAccessAudit(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
	var password string
	if err := db.SQL().QueryRow(`SELECT password_hash FROM user_accounts WHERE id='audit-owner'`).Scan(&password); err != nil || password != "synthetic hash" {
		t.Fatal("migration changed existing account", err)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT count(*) FROM access_audit WHERE event_id='migration-fixture'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("reapplication removed history", err)
	}
	if err := db.validateSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`DROP TRIGGER access_audit_bound`); err != nil {
		t.Fatal(err)
	}
	if err := db.validateSchema(t.Context()); !errors.Is(err, ErrInvalidSchema) {
		t.Fatal("missing retention bound was accepted", err)
	}
}
