package accounts

import "context"

// SaveOwnProfileImage keeps HTTP writes bound to a live session through the
// final metadata transaction. A revoked upload rolls back its staged image.
func (s *Store) SaveOwnProfileImage(ctx context.Context, key string, data []byte) (Account, error) {
	session, err := s.CheckSession(ctx, key)
	if err != nil {
		return Account{}, err
	}
	if !session.Account.FullAccess() || session.Interface != InterfaceFull {
		return Account{}, ErrInterfaceAccess
	}
	return s.saveProfileImage(ctx, session.Account.ID, key, data)
}

// DeleteOwnProfileImage applies the same admission contract as replacement.
func (s *Store) DeleteOwnProfileImage(ctx context.Context, key string) (Account, error) {
	session, err := s.CheckSession(ctx, key)
	if err != nil {
		return Account{}, err
	}
	if !session.Account.FullAccess() || session.Interface != InterfaceFull {
		return Account{}, ErrInterfaceAccess
	}
	return s.deleteProfileImage(ctx, session.Account.ID, key)
}
