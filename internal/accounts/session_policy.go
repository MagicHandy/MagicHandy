package accounts

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

const sessionIdleKey = "accounts.session_idle_minutes"
const sessionIdleSelect = `(SELECT value FROM app_kv WHERE key = 'accounts.session_idle_minutes')`

// ErrInvalidSessionPolicy rejects unsupported idle sign-out durations.
var ErrInvalidSessionPolicy = errors.New("idle sign-out must be off or 30, 60, 120, 240 or 480 minutes")

// SessionPolicy controls login inactivity, independently of controller liveness.
// Zero disables only idle expiry; absolute expiry and revocation still apply.
type SessionPolicy struct {
	IdleMinutes     int `json:"idle_minutes"`
	LifetimeMinutes int `json:"lifetime_minutes"`
}

func validIdleMinutes(minutes int) bool {
	switch minutes {
	case 0, 30, 60, 120, 240, 480:
		return true
	default:
		return false
	}
}

type policyReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readIdleMinutes(ctx context.Context, db policyReader) (int, error) {
	var minutes int
	if err := db.QueryRowContext(ctx, `SELECT value FROM app_kv WHERE key = ?`, sessionIdleKey).Scan(&minutes); err != nil {
		return 0, err
	}
	if !validIdleMinutes(minutes) {
		return 0, ErrInvalidSessionPolicy
	}
	return minutes, nil
}

// The first upgrade retires logins already expired under the old 30-minute
// policy before adopting the longer default. Neither upgrades nor later policy
// changes can revive a credential that has already expired.
func (s *Store) initializeSessionPolicy(ctx context.Context) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := readIdleMinutes(ctx, tx)
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := s.pruneExpiredSessions(ctx, tx, 30); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO app_kv(key,value,updated_at) VALUES(?,?,?)`,
			sessionIdleKey, strconv.Itoa(int(DefaultSessionIdleLimit/time.Minute)), s.now().Format(time.RFC3339Nano))
		return err
	})
}

// SessionPolicy reads persisted policy without renewing the caller's login.
func (s *Store) SessionPolicy(ctx context.Context) (SessionPolicy, error) {
	minutes, err := readIdleMinutes(ctx, s.db.SQL())
	return SessionPolicy{IdleMinutes: minutes, LifetimeMinutes: int(s.lifetime / time.Minute)}, err
}

// SetSessionPolicy rechecks administrator authority and changes all still-valid
// logins atomically. Returned private keys let the HTTP owner cancel any work
// whose login expired. The policy is read with session rows, not cached.
func (s *Store) SetSessionPolicy(ctx context.Context, actorKey string, minutes int) ([]string, error) {
	if !validIdleMinutes(minutes) {
		return nil, ErrInvalidSessionPolicy
	}
	var retired []string
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		administrator, err := s.liveAdministrator(ctx, tx, actorKey)
		if err != nil {
			return err
		}
		previous, err := readIdleMinutes(ctx, tx)
		if err != nil {
			return err
		}
		retired, err = s.pruneExpiredSessions(ctx, tx, previous)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE app_kv SET value=?,updated_at=? WHERE key=?`,
			strconv.Itoa(minutes), s.now().Format(time.RFC3339Nano), sessionIdleKey); err != nil {
			return err
		}
		newlyExpired, err := s.pruneExpiredSessions(ctx, tx, minutes)
		if err != nil {
			return err
		}
		retired = append(retired, newlyExpired...)
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: s.now().UnixMilli(), Kind: audit.SessionPolicyChanged,
			Outcome: "success", Actor: audit.ActingAccount(ctx, administrator)})
	})
	return retired, err
}

func (s *Store) pruneExpiredSessions(ctx context.Context, tx *sql.Tx, minutes int) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT token_hash,last_seen_at,expires_at FROM user_sessions`)
	if err != nil {
		return nil, err
	}
	var expired []string
	for rows.Next() {
		var key, lastSeen, expires string
		if err := rows.Scan(&key, &lastSeen, &expires); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if _, _, valid := s.sessionTimes(lastSeen, expires, minutes); !valid {
			expired = append(expired, key)
		}
	}
	readErr, closeErr := rows.Err(), rows.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	for _, key := range expired {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE token_hash=?`, key); err != nil {
			return nil, err
		}
	}
	return expired, nil
}
