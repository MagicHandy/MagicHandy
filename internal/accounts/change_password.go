package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

// ChangeOwnPassword binds password confirmation and the eventual mutation to
// the same still-live session and password. Return retired private keys only
// for immediate server-side cancellation; never serialize them to a browser.
func (s *Store) ChangeOwnPassword(ctx context.Context, actorKey, currentPassword, newPassword string) ([]string, error) {
	proof, err := s.recoveryOwnerProof(ctx, actorKey, currentPassword)
	if err != nil {
		return nil, err
	}
	encoded, err := s.hashPassword(ctx, newPassword)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var keys []string
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.revalidateRecoveryOwner(ctx, tx, actorKey, proof); err != nil {
			return err
		}
		keys, err = s.replacePasswordTx(ctx, tx, proof.account.ID, encoded, now)
		return err
	})
	return keys, err
}

// ResetPasswordForSession rechecks the administrator's own login inside the
// mutation, so a queued reset cannot survive that administrator's recovery.
func (s *Store) ResetPasswordForSession(ctx context.Context, actorKey, accountID, password string) ([]string, error) {
	encoded, err := s.hashPassword(ctx, password)
	if err != nil {
		return nil, err
	}
	var keys []string
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		owner, err := s.liveSessionOwner(ctx, tx, actorKey)
		if err != nil {
			return err
		}
		var role string
		if err := tx.QueryRowContext(ctx, `SELECT role FROM user_accounts WHERE id = ?`, owner).Scan(&role); err != nil {
			return err
		}
		if role != RoleAdmin {
			return errors.New("administrator access required")
		}
		keys, err = s.replacePasswordTx(ctx, tx, accountID, encoded, s.now())
		return err
	})
	return keys, err
}

func (s *Store) replacePasswordTx(ctx context.Context, tx *sql.Tx, accountID, encoded string, now time.Time) ([]string, error) {
	keys, err := recoverySessionKeys(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE user_accounts SET password_hash = ?, updated_at = ? WHERE id = ?`, encoded, now.Format(time.RFC3339Nano), accountID)
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed != 1 {
		return nil, ErrNotFound
	}
	for _, statement := range []string{`DELETE FROM user_sessions WHERE user_id = ?`, `DELETE FROM user_recovery_codes WHERE user_id = ?`} {
		if _, err := tx.ExecContext(ctx, statement, accountID); err != nil {
			return nil, err
		}
	}
	err = audit.AppendTx(ctx, tx, audit.Event{OccurredAt: now.UnixMilli(), Kind: audit.PasswordChanged, Outcome: "success", TargetAccountID: accountID})
	return keys, err
}
