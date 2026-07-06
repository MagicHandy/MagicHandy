// Package memory owns the user-managed long-term memory store: short facts
// the user chooses to keep, inject into chat, disable, or remove. Nothing in
// here is model-written or hidden; persistence lives in the local SQLite
// datastore.
package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	memoriesFileName = "memories.json"
	memoriesVersion  = 1
	maxMemories      = 200
	maxMemoryChars   = 2000
	memoryEnabledKey = "memory.enabled"
)

// ErrMemoryNotFound reports an unknown memory identifier.
var ErrMemoryNotFound = errors.New("memory not found")

// Memory is one saved fact.
type Memory struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// Snapshot is the UI-facing view of the store.
type Snapshot struct {
	Enabled  bool     `json:"enabled"`
	Memories []Memory `json:"memories"`
}

type memoriesFile struct {
	Version  int      `json:"version"`
	Enabled  bool     `json:"enabled"`
	Memories []Memory `json:"memories"`
}

// Store owns durable memory rows and the global injection switch.
type Store struct {
	mu         sync.RWMutex
	path       string
	legacyPath string
	db         *dbstore.DB
	recovered  bool
}

// Open loads the memory store from dataDir; a missing store starts enabled and
// empty, and a corrupt file recovers to the same without failing startup.
func Open(dataDir string) (*Store, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	database, err := dbstore.Open(absDir)
	if err != nil {
		return nil, err
	}
	store := &Store{
		path:       database.Path(),
		legacyPath: filepath.Join(database.DataDir(), memoriesFileName),
		db:         database,
	}

	if err := store.importLegacyMemories(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

// Recovered reports whether an unreadable file was replaced by defaults.
func (s *Store) Recovered() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recovered
}

// Close releases the memory store database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// Snapshot returns a copy of the switch and every memory.
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	enabled := s.memoryEnabledLocked(ctx)
	memories := s.memoriesLocked(ctx, "")
	return Snapshot{Enabled: enabled, Memories: memories}
}

// PromptTexts returns the enabled memory texts for prompt injection, or nil
// when the global switch is off — chat must work identically without them.
func (s *Store) PromptTexts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx := context.Background()
	if !s.memoryEnabledLocked(ctx) {
		return nil
	}
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT text
		FROM memories
		WHERE enabled = 1
		ORDER BY created_at, id
	`)
	if err != nil {
		return nil
	}
	defer func() {
		_ = rows.Close()
	}()

	var texts []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil
		}
		texts = append(texts, text)
	}
	return texts
}

// Add validates, stores, and persists one memory (enabled by default).
func (s *Store) Add(text string) (Memory, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Memory{}, errors.New("memory text is required")
	}
	if len(text) > maxMemoryChars {
		return Memory{}, fmt.Errorf("memory text must be at most %d characters", maxMemoryChars)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	if s.memoryCountLocked(ctx) >= maxMemories {
		return Memory{}, fmt.Errorf("memory limit reached (%d)", maxMemories)
	}
	item := Memory{
		ID:        "mem-" + randomHex(6),
		Text:      text,
		Enabled:   true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO memories(id, text, enabled, created_at)
			VALUES(?, ?, ?, ?)
		`, item.ID, item.Text, boolToInt(item.Enabled), item.CreatedAt)
		return err
	}); err != nil {
		return Memory{}, err
	}
	return item, nil
}

// SetItemEnabled toggles one memory without deleting it.
func (s *Store) SetItemEnabled(id string, enabled bool) (Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	trimmed := strings.TrimSpace(id)
	var item Memory
	if err := s.db.SQL().QueryRowContext(ctx, `
		SELECT id, text, enabled, created_at
		FROM memories
		WHERE id = ?
	`, trimmed).Scan(&item.ID, &item.Text, scanBool(&item.Enabled), &item.CreatedAt); errors.Is(err, sql.ErrNoRows) {
		return Memory{}, ErrMemoryNotFound
	} else if err != nil {
		return Memory{}, err
	}
	item.Enabled = enabled
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE memories
			SET enabled = ?
			WHERE id = ?
		`, boolToInt(enabled), trimmed)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrMemoryNotFound
		}
		return nil
	}); err != nil {
		return Memory{}, err
	}
	return item, nil
}

// Remove deletes one memory permanently.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			DELETE FROM memories
			WHERE id = ?
		`, strings.TrimSpace(id))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrMemoryNotFound
		}
		return nil
	})
}

// Clear deletes every memory but keeps the global switch as-is.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM memories")
		return err
	})
}

// SetEnabled flips the global injection switch.
func (s *Store) SetEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.setMemoryEnabledLocked(context.Background(), enabled)
}

func (s *Store) importLegacyMemories(ctx context.Context) error {
	const domain = "memories"
	if _, ok, err := s.db.LegacyImportStatus(ctx, domain); err != nil {
		return fmt.Errorf("read memory import status: %w", err)
	} else if ok {
		return nil
	}

	status := dbstore.LegacyImportStatus{
		Domain:     domain,
		SourcePath: s.legacyPath,
		Status:     dbstore.LegacyStatusAbsent,
	}
	data, err := os.ReadFile(s.legacyPath) // #nosec G304 -- resolved legacy app memory file.
	if err != nil {
		if !os.IsNotExist(err) {
			s.recovered = true
			status.Status = dbstore.LegacyStatusRecovered
			status.Message = "legacy memory file could not be read; defaults are active"
		}
		return s.db.RecordLegacyImport(ctx, status)
	}

	file := memoriesFile{Version: memoriesVersion, Enabled: true}
	if err := json.Unmarshal(data, &file); err != nil {
		s.recovered = true
		status.Status = dbstore.LegacyStatusRecovered
		status.Message = "legacy memory file could not be parsed; defaults are active"
		return s.db.RecordLegacyImport(ctx, status)
	}
	file = s.normalizeLoadedFile(file)
	status.Status = dbstore.LegacyStatusImported
	status.Message = "legacy memories imported"
	if s.recovered {
		status.Message = "legacy memories imported with invalid records skipped"
	}

	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_kv(key, value, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(key) DO NOTHING
		`, memoryEnabledKey, boolString(file.Enabled), now); err != nil {
			return err
		}
		for _, item := range file.Memories {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO memories(id, text, enabled, created_at)
				VALUES(?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING
			`, item.ID, item.Text, boolToInt(item.Enabled), item.CreatedAt); err != nil {
				return err
			}
		}
		return dbstore.RecordLegacyImportTx(ctx, tx, status)
	}); err != nil {
		return fmt.Errorf("import legacy memories: %w", err)
	}

	archivePath, archiveErr := dbstore.ArchiveLegacyJSON(s.legacyPath)
	if archiveErr != nil {
		status.Message += "; legacy memory archive failed: " + archiveErr.Error()
	} else if archivePath != "" {
		status.ArchivedPath = archivePath
	}
	if status.ArchivedPath != "" || archiveErr != nil {
		return s.db.RecordLegacyImport(ctx, status)
	}
	return nil
}

func (s *Store) normalizeLoadedFile(file memoriesFile) memoriesFile {
	file.Version = memoriesVersion
	normalized := file.Memories[:0]
	seen := make(map[string]struct{}, len(file.Memories))
	for _, item := range file.Memories {
		item.ID = strings.TrimSpace(item.ID)
		item.Text = strings.TrimSpace(item.Text)
		if item.ID == "" || item.Text == "" || len(item.Text) > maxMemoryChars {
			s.recovered = true
			continue
		}
		if _, exists := seen[item.ID]; exists {
			s.recovered = true
			continue
		}
		seen[item.ID] = struct{}{}
		if len(normalized) >= maxMemories {
			s.recovered = true
			break
		}
		normalized = append(normalized, item)
	}
	file.Memories = normalized
	return file
}

func (s *Store) memoryEnabledLocked(ctx context.Context) bool {
	var value string
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT value
		FROM app_kv
		WHERE key = ?
	`, memoryEnabledKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	return err == nil && value == "true"
}

func (s *Store) setMemoryEnabledLocked(ctx context.Context, enabled bool) error {
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO app_kv(key, value, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET
				value = excluded.value,
				updated_at = excluded.updated_at
		`, memoryEnabledKey, boolString(enabled), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *Store) memoriesLocked(ctx context.Context, where string, args ...any) []Memory {
	query := `
		SELECT id, text, enabled, created_at
		FROM memories
	` + where + `
		ORDER BY created_at, id
	`
	rows, err := s.db.SQL().QueryContext(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer func() {
		_ = rows.Close()
	}()

	var memories []Memory
	for rows.Next() {
		var item Memory
		if err := rows.Scan(&item.ID, &item.Text, scanBool(&item.Enabled), &item.CreatedAt); err != nil {
			return nil
		}
		memories = append(memories, item)
	}
	return memories
}

func (s *Store) memoryCountLocked(ctx context.Context) int {
	var count int
	if err := s.db.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&count); err != nil {
		return maxMemories
	}
	return count
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func scanBool(target *bool) sql.Scanner {
	return boolScanner{target: target}
}

type boolScanner struct {
	target *bool
}

func (s boolScanner) Scan(value any) error {
	switch typed := value.(type) {
	case int64:
		*s.target = typed != 0
	case bool:
		*s.target = typed
	default:
		return fmt.Errorf("scan bool from %T", value)
	}
	return nil
}
