package accounts

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
)

// Password proofs never cross the account boundary or become bearer tokens.
// Any operation using one rechecks the password and enabled state in its own
// transaction so a queued password reset cannot be undone by an older request.
type passwordProof struct {
	account Account
	encoded string
}

func (s *Store) checkPassword(ctx context.Context, username, password string) (passwordProof, error) {
	_, usernameKey, usernameErr := normalizeUsername(username)
	var proof passwordProof
	found := usernameErr == nil
	if found {
		var disabled int
		err := s.db.SQL().QueryRowContext(ctx, `SELECT id, username, role, disabled,
			last_login_at, created_at, updated_at, profile_updated_at, password_hash
			FROM user_accounts WHERE username_key = ?`, usernameKey).Scan(
			&proof.account.ID, &proof.account.Username, &proof.account.Role, &disabled,
			&proof.account.LastLoginAt, &proof.account.CreatedAt, &proof.account.UpdatedAt,
			&proof.account.ProfileUpdatedAt, &proof.encoded)
		proof.account.Disabled = disabled != 0
		proof.account.HasProfileImage = proof.account.ProfileUpdatedAt != ""
		if errors.Is(err, sql.ErrNoRows) {
			found = false
		} else if err != nil {
			return passwordProof{}, fmt.Errorf("read user account: %w", err)
		}
	}
	if !found {
		proof.encoded = dummyPasswordHash()
	}
	matched, err := s.verifyPassword(ctx, password, proof.encoded)
	if err != nil {
		if ctx.Err() != nil {
			return passwordProof{}, ctx.Err()
		}
		return passwordProof{}, ErrInvalidCredentials
	}
	if !found || proof.account.Disabled || !matched {
		return passwordProof{}, ErrInvalidCredentials
	}
	return proof, nil
}

func (p passwordProof) revalidate(ctx context.Context, tx *sql.Tx) error {
	var encoded string
	err := tx.QueryRowContext(ctx, `SELECT password_hash FROM user_accounts WHERE id = ? AND disabled = 0`, p.account.ID).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(p.encoded), []byte(encoded)) != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

// LoginWithClient verifies the password outside the writer, then revalidates
// it and creates the session in the same transaction. HTTP password login must
// use this method, not separate Authenticate and NewSession calls.
func (s *Store) LoginWithClient(ctx context.Context, username, password string, client SessionClient) (string, Session, error) {
	proof, err := s.checkPassword(ctx, username, password)
	if err != nil {
		return "", Session{}, err
	}
	return s.newSessionWithClient(ctx, proof.account.ID, client, &proof)
}
