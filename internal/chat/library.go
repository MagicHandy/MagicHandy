package chat

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
	"unicode/utf8"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	promptSetsFileName  = "prompt_sets.json"
	promptSetsVersion   = 1
	maxUserPromptSets   = 100
	maxPromptNameChars  = 80
	maxPromptSystemSize = 16384
	userPromptSetPrefix = "user-"
)

// Library errors distinguish protection from absence for API status codes.
var (
	ErrPromptSetNotFound  = errors.New("prompt set not found")
	ErrPromptSetProtected = errors.New("built-in prompt sets are read-only; duplicate one to edit it")
	ErrPromptSetInvalid   = errors.New("invalid prompt set")
	ErrPromptSetLimit     = errors.New("prompt set limit reached")
)

type promptSetsFile struct {
	Version int         `json:"version"`
	Sets    []PromptSet `json:"sets"`
}

// PromptLibrary owns user-created prompt sets in the datastore. Built-in sets
// are code-defined templates and never enter the DB.
type PromptLibrary struct {
	mu         sync.RWMutex
	path       string
	legacyPath string
	db         *dbstore.DB
	recovered  bool
}

// OpenPromptLibrary loads user prompt sets from dataDir, recovering to an
// empty library if the file is unreadable or corrupt (defaults stay usable).
func OpenPromptLibrary(dataDir string) (*PromptLibrary, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	database, err := dbstore.Open(absDir)
	if err != nil {
		return nil, err
	}
	library := &PromptLibrary{
		path:       database.Path(),
		legacyPath: filepath.Join(database.DataDir(), promptSetsFileName),
		db:         database,
	}

	if err := library.importLegacyPromptSets(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return library, nil
}

// Recovered reports whether an unreadable file was replaced by an empty library.
func (l *PromptLibrary) Recovered() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.recovered
}

// Close releases the prompt library database handle.
func (l *PromptLibrary) Close() error {
	return l.db.Close()
}

// Resolve returns a built-in or user prompt set by identifier. A missing set is
// reported as found=false; storage failures are returned separately.
func (l *PromptLibrary) Resolve(id string) (set PromptSet, found bool, err error) {
	if set, ok := BuiltinPromptSetByID(id); ok {
		return set, true, nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()

	set, err = l.resolveUserLocked(context.Background(), strings.TrimSpace(id))
	if errors.Is(err, sql.ErrNoRows) {
		return PromptSet{}, false, nil
	}
	if err != nil {
		return PromptSet{}, false, err
	}
	return set, true, nil
}

// List returns built-in sets first, then user sets sorted by name.
func (l *PromptLibrary) List() ([]PromptSet, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	sets := BuiltinPromptSets()
	userSets, err := l.userSetsLocked(context.Background())
	if err != nil {
		return nil, err
	}
	return append(sets, userSets...), nil
}

// Create validates and persists a new user prompt set.
func (l *PromptLibrary) Create(name string, system string) (PromptSet, error) {
	name, system, err := validatePromptSetFields(name, system)
	if err != nil {
		return PromptSet{}, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ctx := context.Background()
	set := PromptSet{
		ID:     userPromptSetPrefix + randomHex(6),
		Name:   name,
		System: system,
	}
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM prompt_sets").Scan(&count); err != nil {
			return err
		}
		if count >= maxUserPromptSets {
			return fmt.Errorf("%w (%d)", ErrPromptSetLimit, maxUserPromptSets)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO prompt_sets(id, name, system, created_at)
			VALUES(?, ?, ?, ?)
		`, set.ID, set.Name, set.System, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		return PromptSet{}, err
	}
	return set, nil
}

// Update edits a user prompt set. Built-in sets are protected.
func (l *PromptLibrary) Update(id string, name string, system string) (PromptSet, error) {
	if _, builtin := BuiltinPromptSetByID(id); builtin {
		return PromptSet{}, ErrPromptSetProtected
	}
	name, system, err := validatePromptSetFields(name, system)
	if err != nil {
		return PromptSet{}, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ctx := context.Background()
	trimmed := strings.TrimSpace(id)
	previous, err := l.resolveUserLocked(ctx, trimmed)
	if errors.Is(err, sql.ErrNoRows) {
		return PromptSet{}, ErrPromptSetNotFound
	} else if err != nil {
		return PromptSet{}, err
	}
	next := previous
	next.Name = name
	next.System = system
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE prompt_sets
			SET name = ?, system = ?
			WHERE id = ?
		`, next.Name, next.System, trimmed)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrPromptSetNotFound
		}
		return nil
	}); err != nil {
		return PromptSet{}, err
	}
	return next, nil
}

// Delete removes a user prompt set. Built-in sets are protected.
func (l *PromptLibrary) Delete(id string) error {
	if _, builtin := BuiltinPromptSetByID(id); builtin {
		return ErrPromptSetProtected
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ctx := context.Background()
	return l.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			DELETE FROM prompt_sets
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
			return ErrPromptSetNotFound
		}
		return nil
	})
}

func (l *PromptLibrary) importLegacyPromptSets(ctx context.Context) error {
	const domain = "prompt_sets"
	if _, ok, err := l.db.LegacyImportStatus(ctx, domain); err != nil {
		return fmt.Errorf("read prompt set import status: %w", err)
	} else if ok {
		return nil
	}

	status := dbstore.LegacyImportStatus{
		Domain:     domain,
		SourcePath: l.legacyPath,
		Status:     dbstore.LegacyStatusAbsent,
	}
	data, err := os.ReadFile(l.legacyPath) // #nosec G304 -- resolved legacy app prompt set file.
	if err != nil {
		if !os.IsNotExist(err) {
			l.recovered = true
			status.Status = dbstore.LegacyStatusRecovered
			status.Message = "legacy prompt set file could not be read; empty library is active"
		}
		return l.db.RecordLegacyImport(ctx, status)
	}
	var file promptSetsFile
	if err := json.Unmarshal(data, &file); err != nil {
		l.recovered = true
		status.Status = dbstore.LegacyStatusRecovered
		status.Message = "legacy prompt set file could not be parsed; empty library is active"
		return l.db.RecordLegacyImport(ctx, status)
	}

	sets := l.normalizeLoadedSets(file.Sets)
	status.Status = dbstore.LegacyStatusImported
	status.Message = "legacy prompt sets imported"
	if l.recovered {
		status.Message = "legacy prompt sets imported with invalid records skipped"
	}
	if err := l.db.WithTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, set := range sets {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO prompt_sets(id, name, system, created_at)
				VALUES(?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING
			`, set.ID, set.Name, set.System, now); err != nil {
				return err
			}
		}
		return dbstore.RecordLegacyImportTx(ctx, tx, status)
	}); err != nil {
		return fmt.Errorf("import legacy prompt sets: %w", err)
	}

	archivePath, archiveErr := dbstore.ArchiveLegacyJSON(l.legacyPath)
	if archiveErr != nil {
		status.Message += "; legacy prompt set archive failed: " + archiveErr.Error()
	} else if archivePath != "" {
		status.ArchivedPath = archivePath
	}
	if status.ArchivedPath != "" || archiveErr != nil {
		return l.db.RecordLegacyImport(ctx, status)
	}
	return nil
}

func (l *PromptLibrary) normalizeLoadedSets(raw []PromptSet) []PromptSet {
	seen := make(map[string]struct{}, len(raw))
	sets := make([]PromptSet, 0, len(raw))
	for _, set := range raw {
		id := strings.TrimSpace(set.ID)
		if id == "" {
			l.recovered = true
			continue
		}
		if _, exists := seen[id]; exists {
			l.recovered = true
			continue
		}
		seen[id] = struct{}{}
		if _, builtin := BuiltinPromptSetByID(id); builtin {
			l.recovered = true
			continue
		}
		name, system, err := validatePromptSetFields(set.Name, set.System)
		if err != nil {
			l.recovered = true
			continue
		}
		if len(sets) >= maxUserPromptSets {
			l.recovered = true
			break
		}
		set.ID = id
		set.Name = name
		set.System = system
		set.Builtin = false
		sets = append(sets, set)
	}
	return sets
}

func (l *PromptLibrary) resolveUserLocked(ctx context.Context, id string) (PromptSet, error) {
	var set PromptSet
	err := l.db.SQL().QueryRowContext(ctx, `
		SELECT id, name, system
		FROM prompt_sets
		WHERE id = ?
	`, strings.TrimSpace(id)).Scan(&set.ID, &set.Name, &set.System)
	if err != nil {
		return PromptSet{}, err
	}
	return set, nil
}

func (l *PromptLibrary) userSetsLocked(ctx context.Context) ([]PromptSet, error) {
	rows, err := l.db.SQL().QueryContext(ctx, `
		SELECT id, name, system
		FROM prompt_sets
		ORDER BY name, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var sets []PromptSet
	for rows.Next() {
		var set PromptSet
		if err := rows.Scan(&set.ID, &set.Name, &set.System); err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sets, nil
}

func validatePromptSetFields(name string, system string) (string, string, error) {
	name = strings.TrimSpace(name)
	system = strings.TrimSpace(system)
	if name == "" {
		return "", "", fmt.Errorf("%w: name is required", ErrPromptSetInvalid)
	}
	if utf8.RuneCountInString(name) > maxPromptNameChars {
		return "", "", fmt.Errorf("%w: name must be at most %d characters", ErrPromptSetInvalid, maxPromptNameChars)
	}
	if system == "" {
		return "", "", fmt.Errorf("%w: system text is required", ErrPromptSetInvalid)
	}
	if len(system) > maxPromptSystemSize {
		return "", "", fmt.Errorf("%w: system text must be at most %d bytes", ErrPromptSetInvalid, maxPromptSystemSize)
	}
	return name, system, nil
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
