// Package notices stores explanatory-notice preferences in the app datastore.
// It never suppresses operational alerts or grants any application capability.
package notices

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// BrowserLifetime bounds both the preference cookie and anonymous record lifetime.
const BrowserLifetime = 365 * 24 * time.Hour

//go:embed catalog.json
var catalogJSON []byte

// Definition is one stable explanatory notice shared with the UI.
type Definition struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	BrowserOnly bool   `json:"browser_only"`
}

// Catalog is shared with the frontend at build time.
var Catalog = func() []Definition {
	var definitions []Definition
	if err := json.Unmarshal(catalogJSON, &definitions); err != nil {
		panic(err)
	}
	return definitions
}()

// ErrUnknown prevents arbitrary or operational messages from being suppressed.
var ErrUnknown = errors.New("unknown informational notice")

// Owner is resolved by the HTTP boundary, never from a request body.
type Owner struct{ AccountID, BrowserHash string }

// Snapshot contains only effective preferences, without owner identifiers.
type Snapshot struct {
	Scope  string   `json:"scope"`
	Hidden []string `json:"hidden"`
}

func definition(id string) (Definition, bool) {
	for _, entry := range Catalog {
		if entry.ID == id {
			return entry, true
		}
	}
	return Definition{}, false
}

func key(owner Owner, entry Definition) string {
	if owner.AccountID != "" && !entry.BrowserOnly {
		return "account:" + owner.AccountID
	}
	return "browser:" + owner.BrowserHash
}

type queryRow interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func read(ctx context.Context, db queryRow, ownerKey string, now time.Time) ([]string, error) {
	var document string
	err := db.QueryRowContext(ctx, `SELECT hidden_json FROM notice_preferences
		WHERE owner_key = ? AND (account_id IS NOT NULL OR updated_at > ?)`, ownerKey, now.Add(-BrowserLifetime).Unix()).Scan(&document)
	if errors.Is(err, sql.ErrNoRows) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var hidden []string
	if err := json.Unmarshal([]byte(document), &hidden); err != nil {
		return nil, err
	}
	return hidden, nil
}

// Read combines the current account's choices with browser-only sign-in choices.
func Read(ctx context.Context, db *sql.DB, owner Owner, now time.Time) (Snapshot, error) {
	result := Snapshot{Scope: "browser", Hidden: []string{}}
	browser, err := read(ctx, db, "browser:"+owner.BrowserHash, now)
	if err != nil {
		return result, err
	}
	account := []string{}
	if owner.AccountID != "" {
		result.Scope = "account"
		account, err = read(ctx, db, "account:"+owner.AccountID, now)
		if err != nil {
			return result, err
		}
	}
	for _, entry := range Catalog {
		hidden := browser
		if owner.AccountID != "" && !entry.BrowserOnly {
			hidden = account
		}
		if slices.Contains(hidden, entry.ID) {
			result.Hidden = append(result.Hidden, entry.ID)
		}
	}
	return result, nil
}

// SetHiddenTx changes one stable notice ID, avoiding lost updates between tabs.
// The caller revalidates any account session inside this same write transaction.
func SetHiddenTx(ctx context.Context, tx *sql.Tx, owner Owner, id string, hide bool, now time.Time) error {
	entry, ok := definition(id)
	if !ok {
		return ErrUnknown
	}
	ownerKey := key(owner, entry)
	hidden, err := read(ctx, tx, ownerKey, now)
	if err != nil {
		return err
	}
	hidden = slices.DeleteFunc(hidden, func(value string) bool { _, known := definition(value); return value == id || !known })
	if hide {
		hidden = append(hidden, id)
	}
	if len(hidden) == 0 {
		_, err = tx.ExecContext(ctx, `DELETE FROM notice_preferences WHERE owner_key = ?`, ownerKey)
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM notice_preferences WHERE account_id IS NULL AND updated_at <= ?`, now.Add(-BrowserLifetime).Unix()); err != nil {
		return err
	}
	slices.Sort(hidden)
	document, err := json.Marshal(hidden)
	if err != nil {
		return err
	}
	var accountID any
	if owner.AccountID != "" && !entry.BrowserOnly {
		accountID = owner.AccountID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO notice_preferences(owner_key, account_id, hidden_json, updated_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(owner_key) DO UPDATE SET hidden_json = excluded.hidden_json, updated_at = excluded.updated_at`, ownerKey, accountID, string(document), now.Unix())
	return err
}

// ResetTx restores notices for this account and this browser only.
func ResetTx(ctx context.Context, tx *sql.Tx, owner Owner) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM notice_preferences WHERE owner_key IN (?, ?)`, "account:"+owner.AccountID, "browser:"+owner.BrowserHash)
	return err
}
