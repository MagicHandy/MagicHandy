package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const syncPrefsKey = "sync_prefs"

// SyncPrefs stores motion sync timing preferences for the LSO UI.
type SyncPrefs struct {
	OffsetMS        int  `json:"offset_ms"`
	MeasuredRTTMS   *int `json:"measured_rtt_ms,omitempty"`
	DeviceLatencyMS *int `json:"device_latency_ms,omitempty"`
	ClientLatencyMS *int `json:"client_latency_ms,omitempty"`
}

// DefaultSyncPrefs returns the LSO-style default sync offset.
func DefaultSyncPrefs() SyncPrefs {
	return SyncPrefs{OffsetMS: -160}
}

// LoadSyncPrefs reads persisted sync preferences.
func (db *DB) LoadSyncPrefs(ctx context.Context) (SyncPrefs, error) {
	prefs := DefaultSyncPrefs()
	var raw string
	err := db.SQL().QueryRowContext(ctx, `
		SELECT value FROM app_kv WHERE key = ?
	`, syncPrefsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
		return DefaultSyncPrefs(), nil
	}
	return prefs, nil
}

// SaveSyncPrefs persists sync preferences.
func (db *DB) SaveSyncPrefs(ctx context.Context, prefs SyncPrefs) error {
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app_kv(key, value, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET
				value = excluded.value,
				updated_at = excluded.updated_at
		`, syncPrefsKey, string(data), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}
