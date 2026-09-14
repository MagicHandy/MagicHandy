package accounts

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

// ControlGrant authorizes one account to operate this shared installation's
// existing motion/chat/media path. It grants no host-administration privileges
// and never follows the selected linked-account attribution profile.
type ControlGrant struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	IssuedBy  string    `json:"issued_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CanControl reports authority at the supplied instant, including grant expiry.
func (s Session) CanControl(now time.Time) bool {
	return !s.Account.Disabled && (s.Account.Role == RoleAdmin || (s.ControlGrant != nil && now.Before(s.ControlGrant.ExpiresAt)))
}

// ControlGrant returns an unexpired permission, or nil for an observer.
func (s *Store) ControlGrant(ctx context.Context, accountID string) (*ControlGrant, error) {
	var grant ControlGrant
	var created, expires string
	err := s.db.SQL().QueryRowContext(ctx, `SELECT g.grant_id, g.user_id, g.issued_by, g.created_at, g.expires_at
		FROM user_control_grants g JOIN user_accounts owner ON owner.id = g.issued_by
		WHERE g.user_id = ? AND owner.disabled = 0 AND owner.role = 'admin'`, accountID).
		Scan(&grant.ID, &grant.AccountID, &grant.IssuedBy, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	grant.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, err
	}
	grant.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return nil, err
	}
	if !s.now().Before(grant.ExpiresAt) {
		return nil, nil
	}
	return &grant, nil
}

// GrantControl replaces an account's previous permission with a fresh identity.
// The issuing administrator is checked inside the same write transaction.
func (s *Store) GrantControl(ctx context.Context, administratorID, accountID string, lifetime time.Duration) (*ControlGrant, error) {
	if lifetime < time.Minute || lifetime > 12*time.Hour {
		return nil, errors.New("control permission must expire between one minute and twelve hours from now")
	}
	random := make([]byte, 16)
	if err := s.randomBytes(random); err != nil {
		return nil, err
	}
	now := s.now()
	grant := &ControlGrant{ID: hex.EncodeToString(random), AccountID: accountID, IssuedBy: administratorID,
		CreatedAt: now, ExpiresAt: now.Add(lifetime)}
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_accounts owner, user_accounts target
			WHERE owner.id = ? AND owner.disabled = 0 AND owner.role = 'admin'
			AND target.id = ? AND target.disabled = 0 AND target.role = 'operator'`, administratorID, accountID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("an enabled administrator can grant control only to an enabled operator")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO user_control_grants(user_id, grant_id, issued_by, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET grant_id = excluded.grant_id,
			issued_by = excluded.issued_by, created_at = excluded.created_at, expires_at = excluded.expires_at`,
			accountID, grant.ID, administratorID, now.Format(time.RFC3339Nano), grant.ExpiresAt.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: now.UnixMilli(), Kind: audit.GrantIssued, Outcome: "success", Actor: audit.ActingAccount(ctx, administratorID), TargetAccountID: accountID, GrantID: grant.ID, ExpiresAt: grant.ExpiresAt.UnixMilli()})
	})
	return grant, err
}

// RevokeControl removes permission without ending the account's read access.
func (s *Store) RevokeControl(ctx context.Context, administratorID, accountID string) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_accounts WHERE id = ? AND disabled = 0 AND role = 'admin'`, administratorID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("administrator access required")
		}
		var grantID string
		if err := tx.QueryRowContext(ctx, "SELECT grant_id FROM user_control_grants WHERE user_id = ?", accountID).Scan(&grantID); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM user_control_grants WHERE user_id = ?", accountID); err != nil {
			return err
		}
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: s.now().UnixMilli(), Kind: audit.GrantRevoked, Outcome: "success", Actor: audit.ActingAccount(ctx, administratorID), TargetAccountID: accountID, GrantID: grantID})
	})
}
