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

// PromptLibrary owns user-created prompt sets on disk. Built-in sets are
// code-defined templates and never enter the file.
type PromptLibrary struct {
	mu        sync.RWMutex
	path      string
	sets      map[string]PromptSet
	recovered bool
}

// OpenPromptLibrary loads user prompt sets from dataDir, recovering to an
// empty library if the file is unreadable or corrupt (defaults stay usable).
func OpenPromptLibrary(dataDir string) (*PromptLibrary, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	library := &PromptLibrary{
		path: filepath.Join(absDir, promptSetsFileName),
		sets: map[string]PromptSet{},
	}

	data, err := os.ReadFile(library.path) // #nosec G304 -- resolved app data file.
	if err != nil {
		if !os.IsNotExist(err) {
			library.recovered = true
		}
		return library, nil
	}
	var file promptSetsFile
	if err := json.Unmarshal(data, &file); err != nil {
		library.recovered = true
		return library, nil
	}
	seen := make(map[string]struct{}, len(file.Sets))
	for _, set := range file.Sets {
		id := strings.TrimSpace(set.ID)
		if id == "" {
			library.recovered = true
			continue
		}
		if _, exists := seen[id]; exists {
			library.recovered = true
			continue
		}
		seen[id] = struct{}{}
		if _, builtin := BuiltinPromptSetByID(id); builtin {
			library.recovered = true
			continue
		}
		name, system, err := validatePromptSetFields(set.Name, set.System)
		if err != nil {
			library.recovered = true
			continue
		}
		if len(library.sets) >= maxUserPromptSets {
			library.recovered = true
			break
		}
		set.ID = id
		set.Name = name
		set.System = system
		set.Builtin = false
		library.sets[set.ID] = set
	}
	return library, nil
}

// Recovered reports whether an unreadable file was replaced by an empty library.
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
	sets := make([]PromptSet, 0, len(l.sets))
	for _, set := range l.sets {
		sets = append(sets, set)
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].ID < sets[j].ID })

	return writeJSONFileAtomic(l.path, promptSetsFile{
		Version: promptSetsVersion,
		Sets:    sets,
	})
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

// writeJSONFileAtomic writes JSON with a temp-file rename so a crash cannot
// leave a truncated store behind (same durability rule as settings.json).
func writeJSONFileAtomic(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store file: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp store file: %w", err)
	}
	tempName := temp.Name()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return fmt.Errorf("write temp store file: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close temp store file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("replace store file: %w", err)
	}
	return nil
}
