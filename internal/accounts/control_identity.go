package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	// ControlRelationshipSelf is the authenticated account itself.
	ControlRelationshipSelf = "self"
	// ControlRelationshipLinked is an explicitly active directional link.
	ControlRelationshipLinked = "linked"
)

// ErrControlIdentityNotAllowed rejects a target that is neither self nor an
// enabled account connected by an active backend-owned link.
var ErrControlIdentityNotAllowed = errors.New("control identity is not available to this account")

// ControlIdentities returns the backend-authorized choices for one signed-in
// account. The authenticated account is always first and labeled Self.
func (s *Store) ControlIdentities(ctx context.Context, ownerID, selectedID string) ([]ControlIdentity, error) {
	self, err := s.accountByID(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if selectedID == "" {
		selectedID = ownerID
	}
	identities := []ControlIdentity{{
		Account:      self,
		Relationship: ControlRelationshipSelf,
		Label:        "Self",
		Selected:     selectedID == ownerID,
	}}

	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT a.id, a.username, a.role, a.disabled, a.last_login_at,
		       a.created_at, a.updated_at, a.profile_updated_at, l.label
		FROM user_account_links l
		JOIN user_accounts a ON a.id = l.linked_user_id
		WHERE l.owner_user_id = ? AND l.status = 'active' AND a.disabled = 0
		ORDER BY a.username_key, a.id
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list linked control identities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	selectedFound := identities[0].Selected
	for rows.Next() {
		var account Account
		var label string
		var disabled int
		if err := rows.Scan(
			&account.ID, &account.Username, &account.Role, &disabled,
			&account.LastLoginAt, &account.CreatedAt, &account.UpdatedAt,
			&account.ProfileUpdatedAt, &label,
		); err != nil {
			return nil, fmt.Errorf("scan linked control identity: %w", err)
		}
		account.Disabled = disabled != 0
		account.HasProfileImage = account.ProfileUpdatedAt != ""
		label = strings.TrimSpace(label)
		if label == "" {
			label = account.Username
		}
		selected := account.ID == selectedID
		selectedFound = selectedFound || selected
		identities = append(identities, ControlIdentity{
			Account:      account,
			Relationship: ControlRelationshipLinked,
			Label:        label,
			Selected:     selected,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read linked control identities: %w", err)
	}
	if !selectedFound {
		identities[0].Selected = true
	}
	return identities, nil
}

// SetControlIdentity changes only this opaque login session's attribution
// context. It cannot change authentication role or controller ownership.
func (s *Store) SetControlIdentity(ctx context.Context, token, targetAccountID string) error {
	if len(token) < 32 || len(token) > 128 {
		return ErrInvalidSession
	}
	targetAccountID = strings.TrimSpace(targetAccountID)
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var ownerID string
		if err := tx.QueryRowContext(ctx, `
			SELECT user_id FROM user_sessions WHERE token_hash = ?
		`, hashSessionToken(token)).Scan(&ownerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidSession
			}
			return err
		}
		if targetAccountID == "" || targetAccountID == ownerID {
			_, err := tx.ExecContext(ctx, `
				UPDATE user_sessions SET control_account_id = NULL WHERE token_hash = ?
			`, hashSessionToken(token))
			return err
		}

		var allowed int
		err := tx.QueryRowContext(ctx, `
			SELECT 1
			FROM user_account_links l
			JOIN user_accounts a ON a.id = l.linked_user_id
			WHERE l.owner_user_id = ? AND l.linked_user_id = ?
			  AND l.status = 'active' AND a.disabled = 0
		`, ownerID, targetAccountID).Scan(&allowed)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrControlIdentityNotAllowed
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE user_sessions SET control_account_id = ? WHERE token_hash = ?
		`, targetAccountID, hashSessionToken(token))
		return err
	})
}

// CanViewProfile reports whether viewer may fetch target's profile image.
func (s *Store) CanViewProfile(ctx context.Context, viewer Account, targetAccountID string) (bool, error) {
	if viewer.ID == targetAccountID || viewer.Role == RoleAdmin {
		return true, nil
	}
	var allowed int
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT 1
		FROM user_account_links l
		JOIN user_accounts a ON a.id = l.linked_user_id
		WHERE l.owner_user_id = ? AND l.linked_user_id = ?
		  AND l.status = 'active' AND a.disabled = 0
	`, viewer.ID, targetAccountID).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check profile visibility: %w", err)
	}
	return true, nil
}

func (s *Store) accountByID(ctx context.Context, accountID string) (Account, error) {
	row := s.db.SQL().QueryRowContext(ctx, `
		SELECT id, username, role, disabled, last_login_at, created_at, updated_at, profile_updated_at
		FROM user_accounts WHERE id = ?
	`, accountID)
	account, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("read user account: %w", err)
	}
	return account, nil
}
