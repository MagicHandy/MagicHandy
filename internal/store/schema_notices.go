package store

import (
	"context"
	"fmt"
	"strings"
)

// Anonymous preferences have a hard profile cap, fixed-size documents and no
// user-supplied content. Account records are removed with their account.
const noticeBrowserLimit = `CREATE TRIGGER IF NOT EXISTS notice_browser_limit
	BEFORE INSERT ON notice_preferences
	WHEN NEW.account_id IS NULL
	AND NOT EXISTS (SELECT 1 FROM notice_preferences WHERE owner_key = NEW.owner_key)
	AND (SELECT count(*) FROM notice_preferences WHERE account_id IS NULL) >= 2048
	BEGIN SELECT RAISE(ABORT, 'browser notice preference capacity reached'); END`

var noticePreferencesSchema = []string{
	`CREATE TABLE IF NOT EXISTS notice_preferences (
		owner_key TEXT NOT NULL PRIMARY KEY,
		account_id TEXT REFERENCES user_accounts(id) ON DELETE CASCADE,
		hidden_json TEXT NOT NULL CHECK(length(hidden_json) <= 2048 AND json_valid(hidden_json) AND json_type(hidden_json) = 'array' AND json_array_length(hidden_json) <= 16),
		updated_at INTEGER NOT NULL,
		CHECK((account_id IS NOT NULL AND owner_key = 'account:' || account_id)
			OR (account_id IS NULL AND length(owner_key) = 72 AND substr(owner_key, 1, 8) = 'browser:' AND substr(owner_key, 9) NOT GLOB '*[^0-9a-f]*'))
	)`,
	noticeBrowserLimit,
}

func (db *DB) validateNoticePreferenceBounds(ctx context.Context) error {
	var actual string
	if err := db.sql.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type = 'trigger' AND name = 'notice_browser_limit'`).Scan(&actual); err != nil {
		return fmt.Errorf("%w: missing browser notice preference bound", ErrInvalidSchema)
	}
	normalize := func(value string) string {
		return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(value, "IF NOT EXISTS ", "")), " "))
	}
	if normalize(actual) != normalize(noticeBrowserLimit) {
		return fmt.Errorf("%w: browser notice preference bound changed", ErrInvalidSchema)
	}
	return nil
}
