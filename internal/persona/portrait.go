package persona

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
)

var (
	// ErrPortraitInvalid reports a payload that is not a usable JPEG.
	ErrPortraitInvalid = errors.New("portrait is not a supported JPEG")
	// ErrPortraitNotFound reports a persona with no stored portrait.
	ErrPortraitNotFound = errors.New("portrait has not been set")
)

const (
	// MaxPortraitBytes bounds an accepted portrait. The browser is asked for a
	// downscaled JPEG, so anything near this is a client that ignored the
	// contract rather than an unusually detailed picture.
	MaxPortraitBytes = 2 << 20
	// MaxPortraitEdge is the largest dimension accepted. Resizing happens in the
	// browser, on the same canvas path video covers already use: server-side
	// scaling would need a new image-scaling dependency or FFmpeg, and FFmpeg is
	// deliberately optional. A portrait must not be what makes it mandatory.
	//
	// The server still checks the dimensions rather than trusting the client,
	// because this endpoint writes a file to disk.
	MaxPortraitEdge = 1024
)

// jpegMagic is the SOI marker every JPEG starts with.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF}

// PortraitDir is where portraits live: under the app data directory, beside
// generated video thumbnails, and purgeable in one step.
func (s *Store) PortraitDir() string {
	return filepath.Join(s.db.DataDir(), "personas")
}

// portraitPath resolves one portrait. The identifier is checked against this
// package's own ID alphabet before it becomes a path element, so a malformed or
// hostile ID cannot escape the portrait directory.
func (s *Store) portraitPath(id string) (string, error) {
	if !ValidID(id) {
		return "", ErrNotFound
	}
	return filepath.Join(s.PortraitDir(), id+".jpg"), nil
}

// SavePortrait stores a portrait and stamps the row. The payload is decoded
// rather than trusted: a client claiming "this is an image" is not enough when
// the result is a file on disk that is later served back with an image type.
func (s *Store) SavePortrait(ctx context.Context, id string, data []byte) (Persona, error) {
	if err := validatePortrait(data); err != nil {
		return Persona{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.getLocked(ctx, id); err != nil {
		return Persona{}, err
	}
	if err := s.writePortrait(ctx, id, data); err != nil {
		return Persona{}, err
	}
	return s.getLocked(ctx, id)
}

// validatePortrait enforces the three things the server can check cheaply and
// must not delegate: that it is a JPEG, that it decodes, and that it is not
// larger than the tile could ever need.
func validatePortrait(data []byte) error {
	if len(data) == 0 || len(data) > MaxPortraitBytes || !bytes.HasPrefix(data, jpegMagic) {
		return ErrPortraitInvalid
	}
	header, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPortraitInvalid, err)
	}
	if header.Width <= 0 || header.Height <= 0 {
		return ErrPortraitInvalid
	}
	if header.Width > MaxPortraitEdge || header.Height > MaxPortraitEdge {
		return fmt.Errorf("%w: %dx%d exceeds the %d pixel limit",
			ErrPortraitInvalid, header.Width, header.Height, MaxPortraitEdge)
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("%w: %v", ErrPortraitInvalid, err)
	}
	return nil
}

// writePortrait writes the file and stamps the row. Callers hold the lock.
func (s *Store) writePortrait(ctx context.Context, id string, data []byte) error {
	path, err := s.portraitPath(id)
	if err != nil {
		return err
	}
	previous, readErr := os.ReadFile(path) // #nosec G304 -- path comes from a validated persona ID.
	hadPrevious := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read portrait before replacement: %w", readErr)
	}
	if err := os.MkdirAll(s.PortraitDir(), 0o700); err != nil {
		return fmt.Errorf("create portrait directory: %w", err)
	}
	// Written to a temporary name and renamed, so a reader never sees a
	// half-written file and a failed write leaves the previous portrait intact.
	temporary := path + ".partial"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write portrait: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("finalize portrait: %w", err)
	}
	if err := s.stampPortrait(ctx, id, timestamp()); err != nil {
		var restoreErr error
		if hadPrevious {
			restoreErr = os.WriteFile(path, previous, 0o600) // #nosec G703 -- validated persona ID.
		} else {
			restoreErr = os.Remove(path)
			if errors.Is(restoreErr, os.ErrNotExist) {
				restoreErr = nil
			}
		}
		if restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore portrait after failed stamp: %w", restoreErr))
		}
		return err
	}
	return nil
}

func (s *Store) readPortrait(id string) ([]byte, error) {
	path, err := s.portraitPath(id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path) // #nosec G304 -- path built from a validated persona ID.
}

func (s *Store) stampPortrait(ctx context.Context, id, stamp string) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`UPDATE personas SET portrait_updated_at = ? WHERE id = ?`, stamp, id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// OpenPortrait opens a stored portrait for serving.
func (s *Store) OpenPortrait(ctx context.Context, id string) (*os.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, err := s.getLocked(ctx, id)
	if err != nil {
		return nil, err
	}
	if !item.HasPortrait {
		return nil, ErrPortraitNotFound
	}
	path, err := s.portraitPath(id)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path) // #nosec G304 -- path built from a validated persona ID.
	if err != nil {
		// The row says a portrait exists and the file does not. Reporting absence
		// rather than a server error lets the tile fall back to its monogram,
		// which is a correct rendering of "there is no picture".
		return nil, ErrPortraitNotFound
	}
	return file, nil
}

// DeletePortrait reverts a persona to its generated monogram.
func (s *Store) DeletePortrait(ctx context.Context, id string) (Persona, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.getLocked(ctx, id)
	if err != nil {
		return Persona{}, err
	}
	previous, hadFile, err := s.removePortraitFiles(id)
	if err != nil {
		return Persona{}, err
	}
	if !item.HasPortrait {
		return item, nil
	}
	if err := s.stampPortrait(ctx, id, ""); err != nil {
		return Persona{}, s.restorePortraitAfterFailure(id, previous, hadFile, err)
	}
	return s.getLocked(ctx, id)
}

// removePortraitFiles removes the served file before database state is cleared.
// It returns enough information to restore the file if the database mutation
// fails, keeping a failed deletion retryable instead of silently leaking bytes.
func (s *Store) removePortraitFiles(id string) ([]byte, bool, error) {
	path, err := s.portraitPath(id)
	if err != nil {
		return nil, false, err
	}
	previous, readErr := os.ReadFile(path) // #nosec G304 -- path comes from a validated persona ID.
	hadFile := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, false, fmt.Errorf("read portrait before deletion: %w", readErr)
	}
	for _, candidate := range []string{path, path + ".partial"} {
		if removeErr := os.Remove(candidate); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			if hadFile {
				_ = os.WriteFile(path, previous, 0o600) // #nosec G703 -- path uses a validated persona ID.
			}
			return nil, false, fmt.Errorf("delete portrait file: %w", removeErr)
		}
	}
	return previous, hadFile, nil
}

func (s *Store) restorePortraitAfterFailure(id string, data []byte, hadFile bool, cause error) error {
	if !hadFile {
		return cause
	}
	path, pathErr := s.portraitPath(id)
	if pathErr != nil {
		return errors.Join(cause, pathErr)
	}
	if restoreErr := os.WriteFile(path, data, 0o600); restoreErr != nil { // #nosec G703 -- validated persona ID.
		return errors.Join(cause, fmt.Errorf("restore portrait after failed deletion: %w", restoreErr))
	}
	return cause
}

// reconcilePortraitFiles makes the database flag and filesystem agree after an
// interrupted write or delete. It removes temporary/orphaned files and clears a
// stale portrait flag when its file is gone.
func (s *Store) reconcilePortraitFiles(ctx context.Context) error {
	rows, err := s.db.SQL().QueryContext(ctx,
		`SELECT id FROM personas WHERE portrait_updated_at != ''`)
	if err != nil {
		return fmt.Errorf("list portrait records: %w", err)
	}
	expected := make(map[string]bool)
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("scan portrait record: %w", scanErr)
		}
		expected[id] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close portrait records: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read portrait records: %w", err)
	}

	entries, err := os.ReadDir(s.PortraitDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read portrait directory: %w", err)
	}
	present := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(s.PortraitDir(), name)
		switch {
		case strings.HasSuffix(name, ".jpg.partial"):
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove interrupted portrait write: %w", err)
			}
		case strings.HasSuffix(name, ".jpg"):
			id := strings.TrimSuffix(name, ".jpg")
			if !ValidID(id) || !expected[id] {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove orphaned portrait: %w", err)
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
			if _, err := tx.ExecContext(ctx,
				`UPDATE personas SET portrait_updated_at = '' WHERE id = ?`, id); err != nil {
				return fmt.Errorf("clear missing portrait record: %w", err)
			}
		}
		return nil
	})
}
