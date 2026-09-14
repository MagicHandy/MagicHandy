package accounts

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

// MaxSessionNameRunes bounds optional, informational device names.
const MaxSessionNameRunes = 80

var (
	// ErrManagedSessionNotFound conceals whether an identifier belongs to another account.
	ErrManagedSessionNotFound = errors.New("session is no longer available")
	// ErrInvalidSessionName rejects oversized or non-displayable names.
	ErrInvalidSessionName = errors.New("session name must be at most 80 characters without control characters")
)

// SessionClient contains coarse hints, never a fingerprint, address or raw UA.
// These labels are informational and confer no authority.
type SessionClient struct {
	Browser  string `json:"browser"`
	Platform string `json:"platform"`
}

// SessionInfo is the public management view. Key is private bookkeeping for
// server-side cancellation and must not enter a response or export.
type SessionInfo struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Client        SessionClient `json:"client"`
	CreatedAt     string        `json:"created_at"`
	LastActiveAt  string        `json:"last_active_at"`
	ExpiresAt     time.Time     `json:"expires_at"`
	IdleExpiresAt time.Time     `json:"idle_expires_at"`
	Current       bool          `json:"current"`
	Key           string        `json:"-"`
}

func normalizedSessionClient(client SessionClient) SessionClient {
	switch client.Browser {
	case "edge", "chrome", "firefox", "safari":
	default:
		client.Browser = "other"
	}
	switch client.Platform {
	case "windows", "macos", "linux", "android", "ios":
	default:
		client.Platform = "other"
	}
	return client
}

func validSessionManagementID(id string) bool {
	if len(id) != 22 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	return err == nil && len(decoded) == 16
}

// ListOwnSessions reads one bounded snapshot without extending any idle time.
func (s *Store) ListOwnSessions(ctx context.Context, actorKey string) ([]SessionInfo, error) {
	tx, err := s.db.SQL().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	owner, err := s.liveSessionOwner(ctx, tx, actorKey)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT token_hash, public_id, device_name, client_browser, client_platform,
		created_at, last_seen_at, expires_at FROM user_sessions WHERE user_id = ?
		ORDER BY (token_hash = ?) DESC, created_at DESC, token_hash DESC LIMIT ?`, owner, actorKey, MaxSessionsPerAccount)
	if err != nil {
		return nil, err
	}
	sessions := make([]SessionInfo, 0)
	for rows.Next() {
		var item SessionInfo
		var expires string
		if err := rows.Scan(&item.Key, &item.ID, &item.Name, &item.Client.Browser, &item.Client.Platform,
			&item.CreatedAt, &item.LastActiveAt, &expires); err != nil {
			_ = rows.Close()
			return nil, err
		}
		lastSeen, deadline, valid := s.sessionTimes(item.LastActiveAt, expires)
		if !valid {
			continue
		}
		item.ExpiresAt, item.IdleExpiresAt = deadline, lastSeen.Add(s.idleLimit)
		if item.IdleExpiresAt.After(deadline) {
			item.IdleExpiresAt = deadline
		}
		item.Current, item.Client = item.Key == actorKey, normalizedSessionClient(item.Client)
		sessions = append(sessions, item)
	}
	readErr, closeErr := rows.Err(), rows.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *Store) sessionTimes(lastSeenRaw, expiresRaw string) (time.Time, time.Time, bool) {
	lastSeen, seenErr := time.Parse(time.RFC3339Nano, lastSeenRaw)
	expires, expiresErr := time.Parse(time.RFC3339Nano, expiresRaw)
	now := s.now()
	return lastSeen, expires, seenErr == nil && expiresErr == nil && now.Before(expires) && now.Sub(lastSeen) <= s.idleLimit
}

// Called inside the same transaction as a management mutation: a request that
// waited behind logout, password reset, disabling or expiry cannot apply later.
func (s *Store) liveSessionOwner(ctx context.Context, tx *sql.Tx, key string) (string, error) {
	var owner, lastSeen, expires string
	err := tx.QueryRowContext(ctx, `SELECT s.user_id, s.last_seen_at, s.expires_at FROM user_sessions s
		JOIN user_accounts a ON a.id = s.user_id WHERE s.token_hash = ? AND a.disabled = 0`, key).Scan(&owner, &lastSeen, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidSession
	}
	if err != nil {
		return "", err
	}
	if _, _, valid := s.sessionTimes(lastSeen, expires); !valid {
		return "", ErrInvalidSession
	}
	return owner, nil
}

// RenameOwnSession updates a display name after checking ownership and current
// login validity in the writer transaction. Empty names restore the client hint.
func (s *Store) RenameOwnSession(ctx context.Context, actorKey, id, name string) error {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxSessionNameRunes || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return ErrInvalidSessionName
	}
	if !validSessionManagementID(id) {
		return ErrManagedSessionNotFound
	}
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		owner, err := s.liveSessionOwner(ctx, tx, actorKey)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE user_sessions SET device_name = ? WHERE user_id = ? AND public_id = ?`, name, owner, id)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err == nil && changed == 0 {
			return ErrManagedSessionNotFound
		}
		return err
	})
}

// RevokeOwnSession returns a private key for immediate runtime invalidation.
// Another account's management ID, including one presented by an administrator,
// cannot address this caller's self-service session collection.
func (s *Store) RevokeOwnSession(ctx context.Context, actorKey, id string) (string, error) {
	if !validSessionManagementID(id) {
		return "", ErrManagedSessionNotFound
	}
	var key string
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		owner, err := s.liveSessionOwner(ctx, tx, actorKey)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `SELECT token_hash FROM user_sessions WHERE user_id = ? AND public_id = ?`, owner, id).Scan(&key)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrManagedSessionNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE token_hash = ? AND user_id = ?`, key, owner)
		if err != nil {
			return err
		}
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: s.now().UnixMilli(), Kind: audit.SessionRevoked, Outcome: "success", Actor: audit.ActingAccount(ctx, owner), TargetSessionID: id})
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

// RevokeOtherSessions preserves the caller and ends the account's other logins.
// The private returned keys are solely for immediate server-side cancellation.
func (s *Store) RevokeOtherSessions(ctx context.Context, actorKey string) ([]string, error) {
	var keys []string
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		owner, err := s.liveSessionOwner(ctx, tx, actorKey)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT token_hash FROM user_sessions WHERE user_id = ? AND token_hash <> ? LIMIT ?`, owner, actorKey, MaxSessionsPerAccount+1)
		if err != nil {
			return err
		}
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
		if len(keys) >= MaxSessionsPerAccount {
			return errors.New("session collection exceeds its retention limit")
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = ? AND token_hash <> ?`, owner, actorKey)
		if err != nil {
			return err
		}
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: s.now().UnixMilli(), Kind: audit.SessionsRevoked, Outcome: "success", Actor: audit.ActingAccount(ctx, owner), TargetAccountID: owner, Count: uint64(len(keys))})
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}
