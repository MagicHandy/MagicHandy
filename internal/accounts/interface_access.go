package accounts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
)

const (
	// InterfaceFull is the main application. An empty value is accepted only for
	// old in-process callers; persisted records always contain an explicit value.
	InterfaceFull = "full"
	// InterfaceRemote allows the dedicated remote and account self-service only.
	InterfaceRemote = "remote"
)

// ErrInterfaceAccess rejects an incompatible account or login audience.
var ErrInterfaceAccess = errors.New("this account can sign in only to the remote interface")

func normalizedInterface(value string) string {
	if value == "" {
		return InterfaceFull
	}
	return value
}

// FullAccess is independent of role: an operator may be remote-only, but an
// administrator may not. Unknown values fail closed.
func (a Account) FullAccess() bool { return normalizedInterface(a.InterfaceAccess) == InterfaceFull }

// CanUseRemote checks the same time-bounded grant as local control without
// allowing a remote login to acquire controller ownership or call core APIs.
func (s Session) CanUseRemote(now time.Time) bool {
	return !s.Account.Disabled && (s.Account.Role == RoleAdmin ||
		(s.Account.Role == RoleOperator && s.ControlGrant != nil &&
			(s.ControlGrant.ExpiresAt == nil || now.Before(*s.ControlGrant.ExpiresAt))))
}

// RemoteDesktopAccount scopes a remote-only login to its granting
// administrator's desktop. A grant is never global access to other users.
func (s Session) RemoteDesktopAccount() string {
	if s.Account.FullAccess() {
		return s.Account.ID
	}
	if s.ControlGrant != nil {
		return s.ControlGrant.IssuedBy
	}
	return ""
}

// CreateWithAccessForSession creates an explicitly scoped account under the
// same serialized administrator check used for ordinary account creation.
func (s *Store) CreateWithAccessForSession(ctx context.Context, actorKey, username, password, role, access string) (Account, error) {
	if actorKey == "" {
		return Account{}, ErrInvalidSession
	}
	return s.create(ctx, username, password, role, false, actorKey, access)
}

// SetInterfaceAccessForSession invalidates all logins and grants when access
// changes, so an already admitted operation cannot outlive a downgrade.
func (s *Store) SetInterfaceAccessForSession(ctx context.Context, actorKey, accountID, access string) ([]string, error) {
	if access != InterfaceFull && access != InterfaceRemote {
		return nil, ErrInterfaceAccess
	}
	var keys []string
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.liveAdministrator(ctx, tx, actorKey); err != nil {
			return err
		}
		var role, current string
		if err := tx.QueryRowContext(ctx, `SELECT role, interface_access FROM user_accounts WHERE id = ?`, accountID).Scan(&role, &current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if access == InterfaceRemote && role != RoleOperator {
			return ErrInterfaceAccess
		}
		if current == access {
			return nil
		}
		var err error
		keys, err = recoverySessionKeys(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE user_accounts SET interface_access = ?, updated_at = ? WHERE id = ?`, access, s.now().Format(time.RFC3339Nano), accountID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE user_id = ?`, accountID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM user_control_grants WHERE user_id = ?`, accountID); err != nil {
			return err
		}
		return audit.AppendTx(ctx, tx, audit.Event{OccurredAt: s.now().UnixMilli(), Kind: audit.AccountAccessChanged, Outcome: "success", TargetAccountID: accountID})
	})
	return keys, err
}
