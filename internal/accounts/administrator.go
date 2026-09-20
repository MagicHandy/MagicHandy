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
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM user_accounts WHERE id = ?`, owner).Scan(&role); err != nil {
		return "", err
	}
	if role != RoleAdmin {
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
