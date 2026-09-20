package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRecoveryMigrationPreservesAccountsSessionsAndSavedCodes(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE user_recovery_codes`, `PRAGMA user_version=23`,
		`INSERT INTO user_accounts(id,username,username_key,role,password_hash,disabled,last_login_at,created_at,updated_at) VALUES('recovery-owner','recovery-owner','recovery-owner','admin','synthetic password hash',0,'login','created','updated')`,
		`INSERT INTO user_sessions(token_hash,user_id,created_at,last_seen_at,expires_at,public_id,device_name) VALUES('synthetic session hash','recovery-owner','created','seen','expiry','synthetic session ID','saved browser')`,
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
	if _, err := db.SQL().Exec(`INSERT INTO user_recovery_codes(code_hash,user_id,created_at) VALUES(?,'recovery-owner','issued')`, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	// Re-applying the append-only migration must not rotate saved credentials.
	if _, err := db.SQL().Exec(`PRAGMA user_version=23`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var password, name, seen, expires, issued string
	err = db.SQL().QueryRow(`SELECT a.password_hash,s.device_name,s.last_seen_at,s.expires_at,c.created_at FROM user_accounts a JOIN user_sessions s ON s.user_id=a.id JOIN user_recovery_codes c ON c.user_id=a.id`).Scan(&password, &name, &seen, &expires, &issued)
	if err != nil || password != "synthetic password hash" || name != "saved browser" || seen != "seen" || expires != "expiry" || issued != "issued" {
		t.Fatal("migration changed existing credentials or lifetime", err)
	}
}

func TestRecoveryStorageEnforcesBoundsImmutabilityAndCascade(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.SQL().Exec(`INSERT INTO user_accounts(id,username,username_key,role,password_hash,created_at,updated_at) VALUES('owner','owner','owner','admin','synthetic hash','created','updated')`); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []any{nil, "", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		if _, err := db.SQL().Exec(`INSERT INTO user_recovery_codes(code_hash,user_id,created_at) VALUES(?,'owner','created')`, invalid); err == nil {
			t.Fatal("invalid digest accepted")
		}
	}
	for i := range 8 {
		if _, err := db.SQL().Exec(`INSERT INTO user_recovery_codes(code_hash,user_id,created_at) VALUES(?,'owner','created')`, fmt.Sprintf("%064x", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.SQL().Exec(`INSERT INTO user_recovery_codes(code_hash,user_id,created_at) VALUES(?,'owner','created')`, strings.Repeat("a", 64)); err == nil {
		t.Fatal("ninth recovery code accepted")
	}
	if _, err := db.SQL().Exec(`UPDATE user_recovery_codes SET created_at='changed'`); err == nil {
		t.Fatal("stored recovery code mutation accepted")
	}
	if _, err := db.SQL().Exec(`DELETE FROM user_accounts WHERE id='owner'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT count(*) FROM user_recovery_codes`).Scan(&count); err != nil || count != 0 {
		t.Fatal("account deletion retained recovery credentials", err)
	}
}

func TestRecoveryStorageRejectsMissingOrChangedBounds(t *testing.T) {
	for _, replacement := range []string{"", `CREATE TRIGGER recovery_codes_limit BEFORE INSERT ON user_recovery_codes BEGIN SELECT 1; END`} {
		t.Run(fmt.Sprint(len(replacement)), func(t *testing.T) {
			db, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.SQL().Exec(`DROP TRIGGER recovery_codes_limit`); err != nil {
				t.Fatal(err)
			}
			if replacement != "" {
				if _, err := db.SQL().Exec(replacement); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.validateSchema(t.Context()); !errors.Is(err, ErrInvalidSchema) {
				t.Fatal("weakened recovery limit accepted", err)
			}
		})
	}
}
