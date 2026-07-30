// Package persona owns named personalization presets. A persona is a portrait,
// a display name, and values across the personalization axes the prompt already
// composes — reply register, reaction style, behavior profile, starting zone.
//
// It is deliberately not a second personalization system and not a prompt
// fragment: every field here maps to a value ComposeSystem already consumes, so
// a persona can change how the assistant sounds and can never change the motion
// contract, the capability gates, the speed limits, or the user's own anatomy
// vocabulary. See docs/persona-page.md §3 for the full guardrail table.
package persona

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

const (
	// MaxNameChars keeps a name legible in a tile. Longer names truncate with an
	// ellipsis rather than wrapping a portrait card to three lines.
	MaxNameChars = 60
	// MaxDescriptionChars reuses the bound the persona description has always
	// had, because it enters the prompt through the same validated path.
	MaxDescriptionChars = config.MaxLLMPersonaDescriptionChars
	// MaxGreetingChars bounds the greeting an imported character card opens a
	// chat with. It is message content seeded into the log, not prompt data,
	// so it is bounded like a message rather than like a prompt field.
	MaxGreetingChars = 2000
	// MaxPromptSetIDChars bounds the stored behavior-profile reference. Membership
	// is not checked here: a user prompt set can be deleted after a persona
	// selects it, and resolution already falls back to the bundled default rather
	// than failing a turn.
	MaxPromptSetIDChars = 80
	// maxPersonas matches the durable memory ceiling.
	maxPersonas = 200
	// idPrefix plus idHexDigits define the whole ID alphabet. Portrait files are
	// named from the ID, so the alphabet is what keeps a hostile identifier from
	// escaping the portrait directory.
	idPrefix    = "persona-"
	idHexDigits = 12
)

// Errors distinguish absence, rejection, and capacity so the API can map them
// to 404, 400, and 409 without inspecting strings.
var (
	// ErrNotFound reports an unknown persona identifier.
	ErrNotFound = errors.New("persona not found")
	// ErrInvalid reports a persona that cannot be stored as described.
	ErrInvalid = errors.New("invalid persona")
	// ErrLimit reports that the persona library is full.
	ErrLimit = errors.New("persona limit reached")
)

// Persona is one saved preset. HasPortrait and PortraitUpdatedAt are derived
// from the same column: the timestamp doubles as an existence flag and as the
// cache-buster a tile URL needs when a portrait is replaced in place.
type Persona struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	ChatVoice         string `json:"chat_voice"`
	ReactionStyle     string `json:"reaction_style"`
	PromptSetID       string `json:"prompt_set_id"`
	DefaultFocusArea  string `json:"default_focus_area"`
	LoreMode          string `json:"lore_mode"`
	Greeting          string `json:"greeting"`
	LoreCount         int    `json:"lore_count"`
	HasPortrait       bool   `json:"has_portrait"`
	PortraitUpdatedAt string `json:"portrait_updated_at,omitempty"`
	LastUsedAt        string `json:"last_used_at,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// Draft is the writable surface. Pointer fields mean "omitted preserves the
// saved value", matching the settings update contract, so a PATCH that carries
// only a name cannot silently reset a register.
type Draft struct {
	Name             *string `json:"name,omitempty"`
	Description      *string `json:"description,omitempty"`
	ChatVoice        *string `json:"chat_voice,omitempty"`
	ReactionStyle    *string `json:"reaction_style,omitempty"`
	PromptSetID      *string `json:"prompt_set_id,omitempty"`
	DefaultFocusArea *string `json:"default_focus_area,omitempty"`
	LoreMode         *string `json:"lore_mode,omitempty"`
	Greeting         *string `json:"greeting,omitempty"`
}

// Store owns persona rows and their portrait files.
type Store struct {
	mu     sync.RWMutex
	db     *dbstore.DB
	ownsDB bool
}

// Open borrows or creates the datastore for the persona domain.
func Open(dataDir string) (*Store, error) {
	database, err := dbstore.Open(dataDir)
	if err != nil {
		return nil, err
	}
	personas := &Store{db: database, ownsDB: true}
	if err := personas.reconcilePortraitFiles(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return personas, nil
}

// OpenWithDatabase borrows the process-owned datastore.
func OpenWithDatabase(database *dbstore.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("persona datastore is required")
	}
	personas := &Store{db: database}
	if err := personas.reconcilePortraitFiles(context.Background()); err != nil {
		return nil, err
	}
	return personas, nil
}

// Close releases the handle only when this store opened it.
func (s *Store) Close() error {
	if !s.ownsDB {
		return nil
	}
	return s.db.Close()
}

// List returns every persona, most recently used first, then by name. A persona
// that has never been used sorts after those that have, which puts the ones the
// user actually reaches for at the front of the grid.
func (s *Store) List(ctx context.Context) ([]Persona, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.SQL().QueryContext(ctx, selectColumns+`
		FROM personas
		ORDER BY last_used_at = '' , last_used_at DESC, name, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	personas := make([]Persona, 0, 8)
	for rows.Next() {
		item, scanErr := scanPersona(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		personas = append(personas, item)
	}
	return personas, rows.Err()
}

// Get returns one persona.
func (s *Store) Get(ctx context.Context, id string) (Persona, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getLocked(ctx, id)
}

func (s *Store) getLocked(ctx context.Context, id string) (Persona, error) {
	if !ValidID(id) {
		return Persona{}, ErrNotFound
	}
	row := s.db.SQL().QueryRowContext(ctx, selectColumns+` FROM personas WHERE id = ?`, id)
	item, err := scanPersona(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Persona{}, ErrNotFound
	}
	return item, err
}

// Create stores a new persona. Unset fields resolve to the defaults rather than
// being rejected, so the "New persona" tile can create an editable row from a
// name alone.
func (s *Store) Create(ctx context.Context, draft Draft) (Persona, error) {
	now := timestamp()
	item := Persona{
		ID:               idPrefix + randomHex(idHexDigits/2),
		ChatVoice:        config.LLMChatVoiceWarm,
		ReactionStyle:    config.LLMReactionStyleNeutral,
		DefaultFocusArea: chat.AreaZoneFull,
		LoreMode:         LoreModeOff,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	item = applyDraft(item, draft)
	if err := validate(item); err != nil {
		return Persona{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM personas").Scan(&count); err != nil {
			return err
		}
		if count >= maxPersonas {
			return fmt.Errorf("%w (%d)", ErrLimit, maxPersonas)
		}
		return insert(ctx, tx, item)
	}); err != nil {
		return Persona{}, err
	}
	return item, nil
}

// Update applies a partial change. The read and the write share one transaction
// so two tabs editing the same persona cannot interleave into a row that has
// neither one's values.
func (s *Store) Update(ctx context.Context, id string, draft Draft) (Persona, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var updated Persona
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if !ValidID(id) {
			return ErrNotFound
		}
		row := tx.QueryRowContext(ctx, selectColumns+` FROM personas WHERE id = ?`, id)
		current, scanErr := scanPersona(row)
		if errors.Is(scanErr, sql.ErrNoRows) {
			return ErrNotFound
		} else if scanErr != nil {
			return scanErr
		}
		updated = applyDraft(current, draft)
		updated.UpdatedAt = timestamp()
		if validateErr := validate(updated); validateErr != nil {
			return validateErr
		}
		_, execErr := tx.ExecContext(ctx, `
			UPDATE personas SET
				name = ?, description = ?, chat_voice = ?, reaction_style = ?,
				prompt_set_id = ?, default_focus_area = ?, lore_mode = ?,
				greeting = ?, updated_at = ?
			WHERE id = ?
		`, updated.Name, updated.Description, updated.ChatVoice, updated.ReactionStyle,
			updated.PromptSetID, updated.DefaultFocusArea, updated.LoreMode,
			updated.Greeting, updated.UpdatedAt, id)
		return execErr
	})
	if err != nil {
		return Persona{}, err
	}
	return updated, nil
}

// Duplicate copies a persona, including its portrait file. Copying the picture
// matters: a duplicate that lost its portrait would look like a different
// persona in the grid, which is the opposite of what duplicating is for.
func (s *Store) Duplicate(ctx context.Context, id string) (Persona, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, err := s.getLocked(ctx, id)
	if err != nil {
		return Persona{}, err
	}
	now := timestamp()
	copied := source
	copied.ID = idPrefix + randomHex(idHexDigits/2)
	copied.Name = copyName(source.Name)
	copied.LastUsedAt = ""
	copied.PortraitUpdatedAt = ""
	copied.HasPortrait = false
	copied.CreatedAt = now
	copied.UpdatedAt = now
	if err := validate(copied); err != nil {
		return Persona{}, err
	}

	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM personas").Scan(&count); err != nil {
			return err
		}
		if count >= maxPersonas {
			return fmt.Errorf("%w (%d)", ErrLimit, maxPersonas)
		}
		if err := insert(ctx, tx, copied); err != nil {
			return err
		}
		return duplicateLoreEntries(ctx, tx, source.ID, copied.ID)
	}); err != nil {
		return Persona{}, err
	}
	if source.HasPortrait {
		if portrait, readErr := s.readPortrait(source.ID); readErr == nil {
			// A failed portrait copy leaves a usable persona with a monogram, so
			// it is reported through the returned row rather than failing the
			// whole duplicate.
			if writeErr := s.writePortrait(ctx, copied.ID, portrait); writeErr == nil {
				copied.HasPortrait = true
				copied.PortraitUpdatedAt = timestamp()
			}
		}
	}
	return copied, nil
}

// Delete removes a persona and its portrait. Chat sessions keep their recorded
// persona_id: a past conversation should still read as the conversation it was,
// and callers resolve the dangling reference to the global axis values.
func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.getLocked(ctx, id); err != nil {
		return err
	}
	previous, hadFile, err := s.removePortraitFiles(id)
	if err != nil {
		return err
	}
	if err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM personas WHERE id = ?`, id)
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
	}); err != nil {
		return s.restorePortraitAfterFailure(id, previous, hadFile, err)
	}
	return nil
}

// MarkUsed records selection. This is what orders the grid, and it is
// deliberately not an update of updated_at: "last talked to" and "last edited"
// are different questions.
func (s *Store) MarkUsed(ctx context.Context, id string) (Persona, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !ValidID(id) {
		return Persona{}, ErrNotFound
	}
	var item Persona
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`UPDATE personas SET last_used_at = ? WHERE id = ?`, timestamp(), id)
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
		item, err = getPersonaFrom(ctx, tx, id)
		return err
	})
	return item, err
}

// ValidID reports whether an identifier is one this package minted. Portrait
// paths are built from it, so this is a security boundary and not a nicety.
func ValidID(id string) bool {
	if !strings.HasPrefix(id, idPrefix) {
		return false
	}
	digits := id[len(idPrefix):]
	if len(digits) != idHexDigits {
		return false
	}
	for _, char := range digits {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

const selectColumns = `
	SELECT id, name, description, chat_voice, reaction_style, prompt_set_id,
		default_focus_area, lore_mode, greeting,
		(SELECT COUNT(*) FROM persona_lore WHERE persona_id = personas.id),
		portrait_updated_at, last_used_at, created_at, updated_at
`

// rowScanner covers both *sql.Row and *sql.Rows so one scan helper serves the
// single-row and list paths.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanPersona(row rowScanner) (Persona, error) {
	var item Persona
	if err := row.Scan(&item.ID, &item.Name, &item.Description, &item.ChatVoice,
		&item.ReactionStyle, &item.PromptSetID, &item.DefaultFocusArea,
		&item.LoreMode, &item.Greeting, &item.LoreCount,
		&item.PortraitUpdatedAt, &item.LastUsedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Persona{}, err
	}
	item.HasPortrait = item.PortraitUpdatedAt != ""
	return item, nil
}

func insert(ctx context.Context, tx *sql.Tx, item Persona) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO personas(id, name, description, chat_voice, reaction_style,
			prompt_set_id, default_focus_area, lore_mode, greeting,
			portrait_updated_at, last_used_at, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.Name, item.Description, item.ChatVoice, item.ReactionStyle,
		item.PromptSetID, item.DefaultFocusArea, item.LoreMode, item.Greeting,
		item.PortraitUpdatedAt, item.LastUsedAt, item.CreatedAt, item.UpdatedAt)
	return err
}

func applyDraft(item Persona, draft Draft) Persona {
	if draft.Name != nil {
		item.Name = collapseSpaces(*draft.Name)
	}
	if draft.Description != nil {
		item.Description = collapseSpaces(*draft.Description)
	}
	if draft.ChatVoice != nil {
		item.ChatVoice = normalizeToken(*draft.ChatVoice)
	}
	if draft.ReactionStyle != nil {
		item.ReactionStyle = normalizeToken(*draft.ReactionStyle)
	}
	if draft.PromptSetID != nil {
		item.PromptSetID = strings.TrimSpace(*draft.PromptSetID)
	}
	if draft.DefaultFocusArea != nil {
		item.DefaultFocusArea = normalizeToken(*draft.DefaultFocusArea)
	}
	if draft.LoreMode != nil {
		item.LoreMode = normalizeToken(*draft.LoreMode)
	}
	if draft.Greeting != nil {
		// TrimSpace, not collapseSpaces: a greeting is multi-line message
		// content and its line breaks are part of the writing.
		item.Greeting = strings.TrimSpace(*draft.Greeting)
	}
	return item
}

func validate(item Persona) error {
	if item.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if utf8.RuneCountInString(item.Name) > MaxNameChars {
		return fmt.Errorf("%w: name must be at most %d characters", ErrInvalid, MaxNameChars)
	}
	if utf8.RuneCountInString(item.Description) > MaxDescriptionChars {
		return fmt.Errorf("%w: description must be at most %d characters", ErrInvalid, MaxDescriptionChars)
	}
	if utf8.RuneCountInString(item.PromptSetID) > MaxPromptSetIDChars {
		return fmt.Errorf("%w: prompt set reference must be at most %d characters", ErrInvalid, MaxPromptSetIDChars)
	}
	if !config.ValidLLMChatVoice(item.ChatVoice) {
		return fmt.Errorf("%w: unknown reply register %q", ErrInvalid, item.ChatVoice)
	}
	if !config.ValidLLMReactionStyle(item.ReactionStyle) {
		return fmt.Errorf("%w: unknown reaction style %q", ErrInvalid, item.ReactionStyle)
	}
	if !validFocusArea(item.DefaultFocusArea) {
		return fmt.Errorf("%w: unknown starting zone %q", ErrInvalid, item.DefaultFocusArea)
	}
	if !ValidLoreMode(item.LoreMode) {
		return fmt.Errorf("%w: unknown lore mode %q", ErrInvalid, item.LoreMode)
	}
	if utf8.RuneCountInString(item.Greeting) > MaxGreetingChars {
		return fmt.Errorf("%w: greeting must be at most %d characters", ErrInvalid, MaxGreetingChars)
	}
	return nil
}

func validFocusArea(area string) bool {
	for _, zone := range chat.AreaZones() {
		if zone == area {
			return true
		}
	}
	return false
}

// copyName suffixes a duplicate without letting repeated duplication grow a
// name past the bound.
func copyName(name string) string {
	const suffix = " copy"
	runes := []rune(name)
	if len(runes)+utf8.RuneCountInString(suffix) > MaxNameChars {
		runes = runes[:MaxNameChars-utf8.RuneCountInString(suffix)]
	}
	return strings.TrimSpace(string(runes)) + suffix
}

func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%0*x", bytes*2, time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
