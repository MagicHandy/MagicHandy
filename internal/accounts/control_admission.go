package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrControlPermissionRequired reports an expired or revoked control grant.
var ErrControlPermissionRequired = errors.New("this account does not have permission to control motion")

// RequireControlSessionTx checks login and control permission in the same
// transaction as a domain write, including time spent waiting for the writer.
func (s *Store) RequireControlSessionTx(ctx context.Context, tx *sql.Tx, key string) error {
	owner, err := s.liveSessionOwner(ctx, tx, key)
	if err != nil {
		return err
	}
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM user_accounts WHERE id = ?`, owner).Scan(&role); err != nil {
		return err
	}
	if role == RoleAdmin {
		return nil
	}
	var expiry sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT g.expires_at FROM user_control_grants g
 JOIN user_accounts issuer ON issuer.id = g.issued_by
 WHERE g.user_id = ? AND issuer.disabled = 0 AND issuer.role = 'admin'`, owner).Scan(&expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrControlPermissionRequired
	}
	if err != nil {
		return err
	}
	if !expiry.Valid {
		return nil
	}
	deadline, err := time.Parse(time.RFC3339Nano, expiry.String)
	if err != nil {
		return err
	}
	if !s.now().Before(deadline) {
		return ErrControlPermissionRequired
	}
	return nil
}
