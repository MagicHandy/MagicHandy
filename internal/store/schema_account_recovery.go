package store

import (
	"context"
	"fmt"
	"strings"
)

const recoveryCodeInsertLimit = `CREATE TRIGGER IF NOT EXISTS recovery_codes_limit
	BEFORE INSERT ON user_recovery_codes
	WHEN (SELECT count(*) FROM user_recovery_codes WHERE user_id = NEW.user_id) >= 8
	BEGIN SELECT RAISE(ABORT, 'account recovery code limit reached'); END`

const recoveryCodeImmutable = `CREATE TRIGGER IF NOT EXISTS recovery_codes_immutable
	BEFORE UPDATE ON user_recovery_codes
	BEGIN SELECT RAISE(ABORT, 'account recovery codes are immutable'); END`

var accountRecoverySchema = []string{
	`CREATE TABLE IF NOT EXISTS user_recovery_codes (
		code_hash TEXT NOT NULL PRIMARY KEY CHECK(length(code_hash) = 64 AND code_hash NOT GLOB '*[^0-9a-f]*'),
		user_id TEXT NOT NULL REFERENCES user_accounts(id) ON DELETE CASCADE,
		created_at TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS recovery_codes_user ON user_recovery_codes(user_id)`,
	recoveryCodeInsertLimit,
	recoveryCodeImmutable,
}

func (db *DB) validateRecoveryCodeBounds(ctx context.Context) error {
	for name, expected := range map[string]string{"recovery_codes_limit": recoveryCodeInsertLimit, "recovery_codes_immutable": recoveryCodeImmutable} {
		var actual string
		if err := db.sql.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type = 'trigger' AND name = ?`, name).Scan(&actual); err != nil {
			return fmt.Errorf("%w: missing account recovery bound", ErrInvalidSchema)
		}
		normalize := func(statement string) string {
			return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(statement, "IF NOT EXISTS ", "")), " "))
		}
		if normalize(actual) != normalize(expected) {
			return fmt.Errorf("%w: account recovery bound changed", ErrInvalidSchema)
		}
	}
	return nil
}
