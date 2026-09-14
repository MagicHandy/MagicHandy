package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
)

// Preserve existing logins, timestamps and any newer management metadata when a
// schema version is rewound. Backfill in bounded batches without exposing keys.
func migrateSessionManagement(ctx context.Context, tx *sql.Tx) error {
	for _, column := range []string{"public_id", "device_name", "client_browser", "client_platform"} {
		exists, err := columnExists(ctx, tx, "user_sessions", column)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE user_sessions ADD COLUMN %s TEXT NOT NULL DEFAULT ''", column)); err != nil {
				return err
			}
		}
	}
	for {
		rows, err := tx.QueryContext(ctx, `SELECT token_hash FROM user_sessions WHERE public_id = '' LIMIT 256`)
		if err != nil {
			return err
		}
		var keys []string
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				_ = rows.Close()
				return err
			}
			keys = append(keys, key)
		}
		readErr, closeErr := rows.Err(), rows.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(keys) == 0 {
			break
		}
		for _, key := range keys {
			var random [16]byte
			if _, err := rand.Read(random[:]); err != nil {
				return err
			}
			id := base64.RawURLEncoding.EncodeToString(random[:])
			if _, err := tx.ExecContext(ctx, `UPDATE user_sessions SET public_id = ? WHERE token_hash = ?`, id, key); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS user_sessions_public_id ON user_sessions(public_id)`)
	return err
}
