// Package memory owns the user-managed long-term memory store: short facts
// the user chooses to keep, inject into chat, disable, or remove. Nothing in
// here is model-written or hidden; rows live in magichandy.db.
package memory

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	memoriesFileName = "memories.json"
	memoriesVersion  = 1
	maxMemories      = 200
	maxMemoryChars   = 2000
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

// Store owns the durable memory rows and the global injection switch.
type Store struct {
	mu        sync.RWMutex
	db        *store.DB
	recovered bool
	state     memoriesFile
}

// Open loads memories from dataDir; a missing store starts enabled and empty,
// and a corrupt legacy file recovers to the same without failing startup.
func Open(dataDir string) (*Store, error) {
	db, err := store.Open(dataDir)
	if err != nil {
		return nil, err
	}

	s := &Store{
		db:    db,
		state: memoriesFile{Version: memoriesVersion, Enabled: true},
	}
	s.loadFromDatabase()
	return s, nil
}

// Recovered reports whether an unreadable store was replaced by defaults.
func (s *Store) Recovered() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recovered
}

// Snapshot returns a copy of the switch and every memory.
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	memories := make([]Memory, len(s.state.Memories))
	copy(memories, s.state.Memories)
	return Snapshot{Enabled: s.state.Enabled, Memories: memories}
}

// PromptTexts returns the enabled memory texts for prompt injection, or nil
// when the global switch is off — chat must work identically without them.
func (s *Store) PromptTexts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.state.Enabled {
		return nil
	}
	texts := make([]string, 0, len(s.state.Memories))
	for _, item := range s.state.Memories {
		if item.Enabled {
			texts = append(texts, item.Text)
		}
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

	if len(s.state.Memories) >= maxMemories {
		return Memory{}, fmt.Errorf("memory limit reached (%d)", maxMemories)
	}
	item := Memory{
		ID:        "mem-" + randomHex(6),
		Text:      text,
		Enabled:   true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.state.Memories = append(s.state.Memories, item)
	if err := s.persistLocked(); err != nil {
		s.state.Memories = s.state.Memories[:len(s.state.Memories)-1]
		return Memory{}, err
	}
	return item, nil
}

// SetItemEnabled toggles one memory without deleting it.
func (s *Store) SetItemEnabled(id string, enabled bool) (Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for index, item := range s.state.Memories {
		if item.ID == strings.TrimSpace(id) {
			previous := item
			item.Enabled = enabled
			s.state.Memories[index] = item
			if err := s.persistLocked(); err != nil {
				s.state.Memories[index] = previous
				return Memory{}, err
			}
			return item, nil
		}
	}
	return Memory{}, ErrMemoryNotFound
}

// Remove deletes one memory permanently.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for index, item := range s.state.Memories {
		if item.ID == strings.TrimSpace(id) {
			previous := s.state.Memories
			s.state.Memories = append(append([]Memory{}, previous[:index]...), previous[index+1:]...)
			if err := s.persistLocked(); err != nil {
				s.state.Memories = previous
				return err
			}
			return nil
		}
	}
	return ErrMemoryNotFound
}

// Clear deletes every memory but keeps the global switch as-is.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.state.Memories
	s.state.Memories = nil
	if err := s.persistLocked(); err != nil {
		s.state.Memories = previous
		return err
	}
	return nil
}

// SetEnabled flips the global injection switch.
func (s *Store) SetEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.state.Enabled
	s.state.Enabled = enabled
	if err := s.persistLocked(); err != nil {
		s.state.Enabled = previous
		return err
	}
	return nil
}

func (s *Store) loadFromDatabase() {
	enabled, rows, found, err := s.db.LoadMemories()
	if err != nil {
		s.recovered = true
		return
	}
	if !found {
		s.loadLegacyJSONFile()
		return
	}

	file := memoriesFile{
		Version:  memoriesVersion,
		Enabled:  enabled,
		Memories: make([]Memory, len(rows)),
	}
	for index, row := range rows {
		file.Memories[index] = Memory{
			ID:        row.ID,
			Text:      row.Text,
			Enabled:   row.Enabled,
			CreatedAt: row.CreatedAt,
		}
	}
	s.state = s.normalizeLoadedFile(file)
}

func (s *Store) loadLegacyJSONFile() {
	path := filepath.Join(s.db.DataDir(), memoriesFileName)
	data, err := os.ReadFile(path) // #nosec G304 -- resolved app data file.
	if err != nil {
		if !os.IsNotExist(err) {
			s.recovered = true
		}
		return
	}
	file := memoriesFile{Version: memoriesVersion, Enabled: true}
	if err := json.Unmarshal(data, &file); err != nil {
		s.recovered = true
		return
	}
	s.state = s.normalizeLoadedFile(file)
}

func (s *Store) persistLocked() error {
	rows := make([]store.MemoryRow, len(s.state.Memories))
	for index, item := range s.state.Memories {
		rows[index] = store.MemoryRow{
			ID:        item.ID,
			Text:      item.Text,
			Enabled:   item.Enabled,
			CreatedAt: item.CreatedAt,
		}
	}
	return s.db.SaveMemories(s.state.Enabled, rows)
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

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
