package accounts

import (
	"context"
	"database/sql"
	"errors"
)

// ErrAdministratorRequired rejects a live login without administrative authority.
var ErrAdministratorRequired = errors.New("administrator access required")

// liveAdministrator checks current authority under the shared writer, rather
// than trusting the account snapshot taken when the HTTP request was admitted.
func (s *Store) liveAdministrator(ctx context.Context, tx *sql.Tx, actorKey string) (string, error) {
	owner, err := s.liveSessionOwner(ctx, tx, actorKey)
	if err != nil {
		return "", err
	}
	var role, access, audience string
	if err := tx.QueryRowContext(ctx, `SELECT a.role, a.interface_access, s.interface
		FROM user_accounts a JOIN user_sessions s ON s.user_id = a.id
		WHERE a.id = ? AND s.token_hash = ?`, owner, actorKey).Scan(&role, &access, &audience); err != nil {
		return "", err
	}
	if role != RoleAdmin || access != InterfaceFull || audience != InterfaceFull {
		return "", ErrAdministratorRequired
	}
	return owner, nil
}

// WithConfirmedAdministrator binds a host configuration write to a still-live
// administrator login and the exact password confirmed before acquiring the
// writer. The callback must use this transaction, never start another one or
// perform external side effects. Password material stays in the accounts domain.
func (s *Store) WithConfirmedAdministrator(ctx context.Context, actorKey, password string, write func(*sql.Tx) error) error {
	proof, err := s.recoveryOwnerProof(ctx, actorKey, password)
	if err != nil {
		return err
	}
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.liveAdministrator(ctx, tx, actorKey); err != nil {
			return err
		}
		if err := s.revalidateRecoveryOwner(ctx, tx, actorKey, proof); err != nil {
			return err
		}
		return write(tx)
	})
}
