package accounts

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrProfileImageInvalid reports a payload that is not a bounded JPEG.
	ErrProfileImageInvalid = errors.New("profile image is not a supported JPEG")
	// ErrProfileImageNotFound reports an account with no stored image.
	ErrProfileImageNotFound = errors.New("profile image has not been set")
)

const (
	// MaxProfileImageBytes bounds an accepted browser-normalized JPEG.
	MaxProfileImageBytes = 2 << 20
	// MaxProfileImageEdge is the largest decoded dimension accepted by the API.
	MaxProfileImageEdge = 1024
)

var jpegMagic = []byte{0xFF, 0xD8, 0xFF}

// ProfileImageDir is app-owned and removed with the rest of MagicHandy data.
func (s *Store) ProfileImageDir() string {
	return filepath.Join(s.db.DataDir(), "account-profiles")
}

// SaveProfileImage validates and atomically replaces one account image.
func (s *Store) SaveProfileImage(ctx context.Context, id string, data []byte) (Account, error) {
	if err := validateProfileImage(data); err != nil {
		return Account{}, err
	}
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	if _, err := s.accountByID(ctx, id); err != nil {
		return Account{}, err
	}
	if err := s.writeProfileImage(ctx, id, data); err != nil {
		return Account{}, err
	}
	return s.accountByID(ctx, id)
}

func validateProfileImage(data []byte) error {
	if len(data) == 0 || len(data) > MaxProfileImageBytes || !bytes.HasPrefix(data, jpegMagic) {
		return ErrProfileImageInvalid
	}
	header, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProfileImageInvalid, err)
	}
	if header.Width <= 0 || header.Height <= 0 {
		return ErrProfileImageInvalid
	}
	if header.Width > MaxProfileImageEdge || header.Height > MaxProfileImageEdge {
		return fmt.Errorf("%w: %dx%d exceeds the %d pixel limit",
			ErrProfileImageInvalid, header.Width, header.Height, MaxProfileImageEdge)
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("%w: %v", ErrProfileImageInvalid, err)
	}
	return nil
}

func (s *Store) writeProfileImage(ctx context.Context, id string, data []byte) error {
	path, err := s.profileImagePath(id)
	if err != nil {
		return err
	}
	previous, readErr := os.ReadFile(path) // #nosec G304 -- path uses a validated account ID.
	hadPrevious := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read profile image before replacement: %w", readErr)
	}
	if err := os.MkdirAll(s.ProfileImageDir(), 0o700); err != nil {
		return fmt.Errorf("create profile image directory: %w", err)
	}
	temporary := path + ".partial"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write profile image: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("finalize profile image: %w", err)
	}
	if err := s.stampProfileImage(ctx, id, s.now().Format(time.RFC3339Nano)); err != nil {
		var restoreErr error
		if hadPrevious {
			restoreErr = os.WriteFile(path, previous, 0o600) // #nosec G703 -- validated account ID.
		} else if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			restoreErr = removeErr
		}
		if restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore profile image after failed stamp: %w", restoreErr))
		}
		return err
	}
	return nil
}

func (s *Store) stampProfileImage(ctx context.Context, id, stamp string) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE user_accounts SET profile_updated_at = ?, updated_at = ? WHERE id = ?
		`, stamp, s.now().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrNotFound
		}
		return nil
	})
}

// OpenProfileImage opens a stable image file while holding the read lock only
// for lookup/open. The returned descriptor remains valid after the lock exits.
func (s *Store) OpenProfileImage(ctx context.Context, id string) (*os.File, error) {
	s.profileMu.RLock()
	defer s.profileMu.RUnlock()
	account, err := s.accountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !account.HasProfileImage {
		return nil, ErrProfileImageNotFound
	}
	path, err := s.profileImagePath(id)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path) // #nosec G304 -- path uses a validated account ID.
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrProfileImageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open profile image: %w", err)
	}
	return file, nil
}

// DeleteProfileImage returns the account to its generated monogram.
func (s *Store) DeleteProfileImage(ctx context.Context, id string) (Account, error) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	account, err := s.accountByID(ctx, id)
	if err != nil {
		return Account{}, err
	}
	previous, hadFile, err := s.removeProfileImageFiles(id)
	if err != nil {
		return Account{}, err
	}
	if !account.HasProfileImage {
		return account, nil
	}
	if err := s.stampProfileImage(ctx, id, ""); err != nil {
		if hadFile {
			path, pathErr := s.profileImagePath(id)
			if pathErr != nil {
				return Account{}, errors.Join(err, pathErr)
			}
			if restoreErr := os.WriteFile(path, previous, 0o600); restoreErr != nil { // #nosec G703 -- validated account ID.
				return Account{}, errors.Join(err, fmt.Errorf("restore profile image: %w", restoreErr))
			}
		}
		return Account{}, err
	}
	return s.accountByID(ctx, id)
}

func (s *Store) profileImagePath(id string) (string, error) {
	if !validAccountID(id) {
		return "", ErrNotFound
	}
	return filepath.Join(s.ProfileImageDir(), id+".jpg"), nil
}

func validAccountID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, character := range id {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func (s *Store) removeProfileImageFiles(id string) ([]byte, bool, error) {
	path, err := s.profileImagePath(id)
	if err != nil {
		return nil, false, err
	}
	previous, readErr := os.ReadFile(path) // #nosec G304 -- path uses a validated account ID.
	hadFile := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, false, fmt.Errorf("read profile image before deletion: %w", readErr)
	}
	for _, candidate := range []string{path, path + ".partial"} {
		if removeErr := os.Remove(candidate); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			if hadFile {
				_ = os.WriteFile(path, previous, 0o600) // #nosec G703 -- path uses a validated account ID.
			}
			return nil, false, fmt.Errorf("delete profile image: %w", removeErr)
		}
	}
	return previous, hadFile, nil
}

func (s *Store) reconcileProfileImages(ctx context.Context) error {
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id FROM user_accounts WHERE profile_updated_at != ''
	`)
	if err != nil {
		return fmt.Errorf("list account profile images: %w", err)
	}
	expected := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan account profile image: %w", err)
		}
		expected[id] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close account profile image rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read account profile image rows: %w", err)
	}

	entries, err := os.ReadDir(s.ProfileImageDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read account profile image directory: %w", err)
	}
	present := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(s.ProfileImageDir(), name)
		switch {
		case strings.HasSuffix(name, ".jpg.partial"):
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove interrupted account profile image: %w", err)
			}
		case strings.HasSuffix(name, ".jpg"):
			id := strings.TrimSuffix(name, ".jpg")
			if !validAccountID(id) || !expected[id] {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove orphaned account profile image: %w", err)
				}
				continue
			}
			present[id] = true
		}
	}

	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		for id := range expected {
			if present[id] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE user_accounts SET profile_updated_at = '' WHERE id = ?
			`, id); err != nil {
				return fmt.Errorf("clear missing account profile image: %w", err)
			}
		}
		return nil
	})
}
