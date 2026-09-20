package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

// RecoveredAccount supplies private session identities only to the HTTP edge
// for immediate lifetime retirement. Recovery never creates a login session.
type RecoveredAccount struct {
	AccountID   string   `json:"-"`
	SessionKeys []string `json:"-"`
}

// RecoverPassword redeems a saved code, changes the password and revokes every
// login and every remaining recovery code in one transaction. Concurrent uses,
// replacement, disabling and password changes are rechecked before committing.
func (s *Store) RecoverPassword(ctx context.Context, username, code, password string) (RecoveredAccount, error) {
	_, usernameKey, nameErr := normalizeUsername(username)
	if nameErr != nil {
		usernameKey = "" // Still perform the same lookup for an invalid name.
	}
	digest := recoveryCodeDigest(code)
	accountID, err := s.recoveryAccount(ctx, s.db.SQL(), usernameKey, digest)
	if err != nil {
		return RecoveredAccount{}, err
	}
	if normalizeRecoveryCode(code) == "" {
		return RecoveredAccount{}, ErrInvalidRecoveryCode
	}
	encoded, err := s.hashPassword(ctx, password)
	if err != nil {
		return RecoveredAccount{}, err
	}
	result := RecoveredAccount{AccountID: accountID}
	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := s.recoveryAccount(ctx, tx, usernameKey, digest)
		if err != nil {
			return err
		}
		if current != accountID {
			return ErrInvalidRecoveryCode
		}
		result.SessionKeys, err = recoverySessionKeys(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE user_accounts SET password_hash = ?, updated_at = ? WHERE id = ?`, encoded, s.now().Format(time.RFC3339Nano), accountID); err != nil {
			return err
		}
		for _, statement := range []string{
			`DELETE FROM user_sessions WHERE user_id = ?`,
			`DELETE FROM user_recovery_codes WHERE user_id = ?`,
		} {
			if _, err := tx.ExecContext(ctx, statement, accountID); err != nil {
				return err
			}
		}
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: s.now().UnixMilli(), Kind: audit.PasswordRecovered, Outcome: "success",
			Actor: audit.Actor{Type: "account", AccountID: accountID}, TargetAccountID: accountID, Count: uint64(len(result.SessionKeys))})
	})
	if err != nil {
		return RecoveredAccount{}, err
	}
	return result, nil
}

type recoveryQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) recoveryAccount(ctx context.Context, query recoveryQuerier, usernameKey, digest string) (string, error) {
	var accountID string
	err := query.QueryRowContext(ctx, `SELECT a.id FROM user_accounts a JOIN user_recovery_codes c ON c.user_id = a.id
		WHERE a.username_key = ? AND c.code_hash = ? AND a.disabled = 0`, usernameKey, digest).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidRecoveryCode
	}
	return accountID, err
}

func recoverySessionKeys(ctx context.Context, tx *sql.Tx, accountID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT token_hash FROM user_sessions WHERE user_id = ? LIMIT ?`, accountID, MaxSessionsPerAccount+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if len(keys) > MaxSessionsPerAccount {
		return nil, errors.New("account session limit is invalid")
	}
	return keys, rows.Err()
}
