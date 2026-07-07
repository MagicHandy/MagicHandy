package chat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/store"
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
)

type promptSetsFile struct {
	Version int         `json:"version"`
	Sets    []PromptSet `json:"sets"`
}

// PromptLibrary owns user-created prompt sets in SQLite. Built-in sets are
// code-defined templates and never enter the database.
type PromptLibrary struct {
	mu        sync.RWMutex
	db        *store.DB
	sets      map[string]PromptSet
	recovered bool
}

// OpenPromptLibrary loads user prompt sets from dataDir, recovering to an
// empty library if the store is unreadable or corrupt (defaults stay usable).
func OpenPromptLibrary(dataDir string) (*PromptLibrary, error) {
	db, err := store.Open(dataDir)
	if err != nil {
		return nil, err
	}

	library := &PromptLibrary{
		db:   db,
		sets: map[string]PromptSet{},
	}
	library.loadFromDatabase()
	return library, nil
}

// Recovered reports whether an unreadable store was replaced by an empty library.
func (l *PromptLibrary) Recovered() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.recovered
}

// Resolve returns a built-in or user prompt set by identifier.
func (l *PromptLibrary) Resolve(id string) (PromptSet, bool) {
	if set, ok := BuiltinPromptSetByID(id); ok {
		return set, true
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	set, ok := l.sets[strings.TrimSpace(id)]
	return set, ok
}

// List returns built-in sets first, then user sets sorted by name.
func (l *PromptLibrary) List() []PromptSet {
	l.mu.RLock()
	defer l.mu.RUnlock()

	sets := BuiltinPromptSets()
	user := make([]PromptSet, 0, len(l.sets))
	for _, set := range l.sets {
		user = append(user, set)
	}
	sort.Slice(user, func(i, j int) bool {
		if user[i].Name == user[j].Name {
			return user[i].ID < user[j].ID
		}
		return user[i].Name < user[j].Name
	})
	return append(sets, user...)
}

// Create validates and persists a new user prompt set.
func (l *PromptLibrary) Create(name string, system string) (PromptSet, error) {
	name, system, err := validatePromptSetFields(name, system)
	if err != nil {
		return PromptSet{}, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.sets) >= maxUserPromptSets {
		return PromptSet{}, fmt.Errorf("prompt set limit reached (%d)", maxUserPromptSets)
	}
	set := PromptSet{
		ID:     userPromptSetPrefix + randomHex(6),
		Name:   name,
		System: system,
	}
	l.sets[set.ID] = set
	if err := l.persistLocked(); err != nil {
		delete(l.sets, set.ID)
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

	previous, ok := l.sets[strings.TrimSpace(id)]
	if !ok {
		return PromptSet{}, ErrPromptSetNotFound
	}
	next := previous
	next.Name = name
	next.System = system
	l.sets[previous.ID] = next
	if err := l.persistLocked(); err != nil {
		l.sets[previous.ID] = previous
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

	previous, ok := l.sets[strings.TrimSpace(id)]
	if !ok {
		return ErrPromptSetNotFound
	}
	delete(l.sets, previous.ID)
	if err := l.persistLocked(); err != nil {
		l.sets[previous.ID] = previous
		return err
	}
	return nil
}

func (l *PromptLibrary) loadFromDatabase() {
	rows, found, err := l.db.LoadPromptSets()
	if err != nil {
		l.recovered = true
		return
	}
	if !found {
		l.loadLegacyJSONFile()
		return
	}
	l.ingestLoadedSets(rowsToPromptSets(rows))
}

func (l *PromptLibrary) loadLegacyJSONFile() {
	path := filepath.Join(l.db.DataDir(), promptSetsFileName)
	data, err := os.ReadFile(path) // #nosec G304 -- resolved app data file.
	if err != nil {
		if !os.IsNotExist(err) {
			l.recovered = true
		}
		return
	}
	var file promptSetsFile
	if err := json.Unmarshal(data, &file); err != nil {
		l.recovered = true
		return
	}
	l.ingestLoadedSets(file.Sets)
}

func rowsToPromptSets(rows []store.PromptSetRow) []PromptSet {
	sets := make([]PromptSet, len(rows))
	for index, row := range rows {
		sets[index] = PromptSet{ID: row.ID, Name: row.Name, System: row.System}
	}
	return sets
}

func (l *PromptLibrary) ingestLoadedSets(sets []PromptSet) {
	seen := make(map[string]struct{}, len(sets))
	for _, set := range sets {
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
		if len(l.sets) >= maxUserPromptSets {
			l.recovered = true
			break
		}
		set.ID = id
		set.Name = name
		set.System = system
		set.Builtin = false
		l.sets[set.ID] = set
	}
}

func validatePromptSetFields(name string, system string) (string, string, error) {
	name = strings.TrimSpace(name)
	system = strings.TrimSpace(system)
	if name == "" {
		return "", "", errors.New("prompt set name is required")
	}
	if len(name) > maxPromptNameChars {
		return "", "", fmt.Errorf("prompt set name must be at most %d characters", maxPromptNameChars)
	}
	if system == "" {
		return "", "", errors.New("prompt set system text is required")
	}
	if len(system) > maxPromptSystemSize {
		return "", "", fmt.Errorf("prompt set system text must be at most %d bytes", maxPromptSystemSize)
	}
	return name, system, nil
}

func (l *PromptLibrary) persistLocked() error {
	rows := make([]store.PromptSetRow, 0, len(l.sets))
	for _, set := range l.sets {
		rows = append(rows, store.PromptSetRow{ID: set.ID, Name: set.Name, System: set.System})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return l.db.SavePromptSets(rows)
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
