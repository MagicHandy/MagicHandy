package store

import (
	"context"
	"database/sql"
	"fmt"
)

// v29 -> v30: a persona may override the Settings reply length. Empty means
// "follow Settings", so every existing persona keeps today's behavior. The
// column is added by a guarded hook because SQLite has no conditional ADD
// COLUMN and re-running a migration is a normal recovery path.
func migratePersonaReplyLength(ctx context.Context, tx *sql.Tx) error {
	exists, err := columnExists(ctx, tx, "personas", "reply_length")
	if err != nil || exists {
		return err
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE personas ADD COLUMN reply_length TEXT NOT NULL DEFAULT ''
		CHECK (reply_length IN ('', 'short', 'balanced', 'detailed'))`); err != nil {
		return fmt.Errorf("add persona reply_length column: %w", err)
	}
	return nil
}
