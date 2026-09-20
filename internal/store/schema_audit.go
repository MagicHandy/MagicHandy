package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// AuditRowLimit bounds durable history independently of the number of accounts
// or control requests. Domain writes share this cap through an insert trigger.
const AuditRowLimit = 10000

const auditBoundTrigger = `CREATE TRIGGER IF NOT EXISTS access_audit_bound AFTER INSERT ON access_audit BEGIN
	DELETE FROM access_audit WHERE seq <= NEW.seq - 10000;
END`

func migrateAccessAudit(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS access_audit (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL,
			occurred_at INTEGER NOT NULL,
			document TEXT NOT NULL CHECK(length(document) <= 2048)
		)`,
		`CREATE INDEX IF NOT EXISTS access_audit_time ON access_audit(occurred_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS access_audit_event_id ON access_audit(event_id)`,
		auditBoundTrigger,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) validateAuditRetention(ctx context.Context) error {
	var definition string
	if err := db.sql.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='trigger' AND name='access_audit_bound' AND tbl_name='access_audit'`).Scan(&definition); err != nil {
		return fmt.Errorf("%w: access audit retention trigger is unavailable", ErrInvalidSchema)
	}
	normalize := func(value string) string {
		return strings.ReplaceAll(strings.ToUpper(strings.Join(strings.Fields(value), " ")), "IF NOT EXISTS ", "")
	}
	if normalize(definition) != normalize(auditBoundTrigger) {
		return fmt.Errorf("%w: access audit retention trigger does not match its bound", ErrInvalidSchema)
	}
	return nil
}
