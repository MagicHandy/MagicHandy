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
	var role, access, audience string
	if err := tx.QueryRowContext(ctx, `SELECT a.role, a.interface_access, s.interface FROM user_accounts a JOIN user_sessions s ON s.user_id = a.id WHERE a.id = ? AND s.token_hash = ?`, owner, key).Scan(&role, &access, &audience); err != nil {
		return err
	}
	if access != InterfaceFull || audience != InterfaceFull {
		return ErrControlPermissionRequired
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
