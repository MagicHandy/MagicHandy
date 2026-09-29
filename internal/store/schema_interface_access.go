package store

import (
	"context"
	"database/sql"
)

// Interface access is independent of a control grant. Existing accounts and
// sessions retain full-app access; the migration never delegates a desktop.
func migrateInterfaceAccess(ctx context.Context, tx *sql.Tx) error {
	for _, column := range []struct{ table, name, statement string }{
		{"user_accounts", "interface_access", `ALTER TABLE user_accounts ADD COLUMN interface_access TEXT NOT NULL DEFAULT 'full' CHECK (interface_access IN ('full', 'remote') AND (interface_access = 'full' OR role = 'operator'))`},
		{"user_sessions", "interface", `ALTER TABLE user_sessions ADD COLUMN interface TEXT NOT NULL DEFAULT 'full' CHECK (interface IN ('full', 'remote'))`},
	} {
		exists, err := columnExists(ctx, tx, column.table, column.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, column.statement); err != nil {
				return err
			}
		}
	}
	return nil
}
