package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrCorruptDatastore classifies physical SQLite corruption that can be
// quarantined without interpreting or deleting the original bytes.
var ErrCorruptDatastore = errors.New("sqlite datastore is corrupt")

// RecoveryStatus describes a non-destructive physical-corruption recovery.
type RecoveryStatus struct {
	Recovered bool
	BackupDir string
	Message   string
}

func openDatastore(dataDir, path string) (*DB, error) {
	existed := isRegularFile(path)
	db, err := initializeDatastore(dataDir, path)
	if err == nil {
		return db, nil
	}
	if !existed || !isPhysicalCorruption(err) {
		return nil, err
	}

	backupDir, backupErr := quarantineSQLiteFiles(dataDir, path)
	if backupErr != nil {
		return nil, errors.Join(err, fmt.Errorf("quarantine corrupt SQLite datastore: %w", backupErr))
	}
	db, retryErr := initializeDatastore(dataDir, path)
	if retryErr != nil {
		return nil, errors.Join(
			err,
			fmt.Errorf("create replacement SQLite datastore after preserving the original at %q: %w", backupDir, retryErr),
		)
	}
	db.recovery = RecoveryStatus{
		Recovered: true,
		BackupDir: backupDir,
		Message:   "the corrupt SQLite datastore was preserved and a new datastore was created",
	}
	return db, nil
}

func initializeDatastore(dataDir, path string) (*DB, error) {
	handle, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open SQLite datastore: %w", err)
	}
	handle.SetMaxOpenConns(maxOpenConnections)
	handle.SetMaxIdleConns(maxIdleConnections)
	db := &DB{sql: handle, dataDir: dataDir, path: path}
	closeOnError := func(err error) (*DB, error) {
		return nil, errors.Join(err, handle.Close())
	}

	ctx := context.Background()
	if err := db.quickCheck(ctx); err != nil {
		return closeOnError(err)
	}
	if err := db.configure(ctx); err != nil {
		return closeOnError(err)
	}
	if err := db.migrate(ctx); err != nil {
		return closeOnError(err)
	}
	if err := db.validateSchema(ctx); err != nil {
		return closeOnError(err)
	}
	if err := secureSQLiteFiles(path); err != nil {
		return closeOnError(err)
	}
	return db, nil
}

func (db *DB) quickCheck(ctx context.Context) error {
	var result string
	if err := db.sql.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&result); err != nil {
		if isPhysicalCorruption(err) {
			return fmt.Errorf("%w: quick check: %v", ErrCorruptDatastore, err)
		}
		return fmt.Errorf("check SQLite datastore: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") {
		result = strings.TrimSpace(result)
		if len(result) > 256 {
			result = result[:256]
		}
		return fmt.Errorf("%w: quick check reported %q", ErrCorruptDatastore, result)
	}
	return nil
}

func isPhysicalCorruption(err error) bool {
	if errors.Is(err, ErrCorruptDatastore) {
		return true
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	switch sqliteErr.Code() & 0xff {
	case sqlite3.SQLITE_CORRUPT, sqlite3.SQLITE_FORMAT, sqlite3.SQLITE_NOTADB:
		return true
	default:
		return false
	}
}

func quarantineSQLiteFiles(dataDir, databasePath string) (string, error) {
	recoveryRoot := filepath.Join(dataDir, "recovery")
	if err := os.MkdirAll(recoveryRoot, 0o700); err != nil {
		return "", err
	}
	if err := secureDataDirectory(recoveryRoot); err != nil {
		return "", err
	}
	backupDir, err := os.MkdirTemp(recoveryRoot, "sqlite-corrupt-")
	if err != nil {
		return "", err
	}

	sources := []string{databasePath, databasePath + "-wal", databasePath + "-shm"}
	moved := make([]string, 0, len(sources))
	for _, source := range sources {
		if _, err := os.Lstat(source); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", rollbackQuarantine(backupDir, moved, err)
		}
		destination := filepath.Join(backupDir, filepath.Base(source))
		if err := os.Rename(source, destination); err != nil {
			return "", rollbackQuarantine(backupDir, moved, err)
		}
		moved = append(moved, source)
	}
	if len(moved) == 0 {
		_ = os.Remove(backupDir)
		return "", errors.New("database files disappeared before quarantine")
	}
	return backupDir, nil
}

func rollbackQuarantine(backupDir string, moved []string, cause error) error {
	errs := []error{cause}
	for index := len(moved) - 1; index >= 0; index-- {
		source := moved[index]
		backup := filepath.Join(backupDir, filepath.Base(source))
		if err := os.Rename(backup, source); err != nil {
			errs = append(errs, fmt.Errorf("restore %q: %w", source, err))
		}
	}
	if err := os.Remove(backupDir); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func secureDataDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil { // #nosec G302 -- directories require owner traversal permission.
		return fmt.Errorf("secure data directory %q: %w", path, err)
	}
	return nil
}

func secureSQLiteFiles(databasePath string) error {
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect SQLite file %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("SQLite path %q is not a regular file", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("secure SQLite file %q: %w", path, err)
		}
	}
	return nil
}

func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
