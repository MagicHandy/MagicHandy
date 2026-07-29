package persona

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// LoreModeOff keeps saved lore out of model prompts.
	LoreModeOff = "off"
	// LoreModeRelevant selects enabled entries whose keywords match recent chat.
	LoreModeRelevant = "relevant"
	// LoreModeFull selects every enabled entry within the hard storage budget.
	LoreModeFull = "full"

	// MaxLoreEntries bounds the number of saved entries per persona.
	MaxLoreEntries = 8
	// MaxLoreTextChars bounds one entry before it reaches prompt composition.
	MaxLoreTextChars = 500
	// MaxLoreTotalChars bounds all saved lore for one persona.
	MaxLoreTotalChars = 2000
	// MaxLoreKeywords bounds the relevant-mode match vocabulary per entry.
	MaxLoreKeywords = 12
	// MaxLoreKeywordChars bounds one relevant-mode keyword or phrase.
	MaxLoreKeywordChars = 40

	loreIDPrefix    = "lore-"
	loreIDHexDigits = 12
)

// LoreEntry is one bounded fact about a persona. Keywords are matching data,
// not prompt instructions; only Text is quoted into the composed prompt.
type LoreEntry struct {
	ID        string   `json:"id"`
	PersonaID string   `json:"persona_id"`
	Text      string   `json:"text"`
	Keywords  []string `json:"keywords"`
	Enabled   bool     `json:"enabled"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// LoreDraft is the partial mutation surface for a lore entry.
type LoreDraft struct {
	Text     *string   `json:"text,omitempty"`
	Keywords *[]string `json:"keywords,omitempty"`
	Enabled  *bool     `json:"enabled,omitempty"`
}

// LoreSelection records exactly what entered one prompt. The inspector uses the
// same value as chat, so its counts cannot drift from model input.
type LoreSelection struct {
	Texts      []string `json:"-"`
	EntryIDs   []string `json:"entry_ids"`
	Characters int      `json:"characters"`
}

// ValidLoreMode reports whether a persona's prompt policy is code-owned.
func ValidLoreMode(mode string) bool {
	switch mode {
	case LoreModeOff, LoreModeRelevant, LoreModeFull:
		return true
	default:
		return false
	}
}

// LoreModes returns the server's accepted vocabulary for the editor.
func LoreModes() []string {
	return []string{LoreModeOff, LoreModeRelevant, LoreModeFull}
}

// ListLore returns one persona's entries in stable authoring order.
func (s *Store) ListLore(ctx context.Context, personaID string) ([]LoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := s.getLocked(ctx, personaID); err != nil {
		return nil, err
	}
	return listLoreRows(ctx, s.db.SQL(), personaID)
}

// CreateLore adds one entry while enforcing both the row and aggregate budgets.
func (s *Store) CreateLore(ctx context.Context, personaID string, draft LoreDraft) (LoreEntry, error) {
	entry := LoreEntry{
		ID:        loreIDPrefix + randomHex(loreIDHexDigits/2),
		PersonaID: personaID,
		Enabled:   true,
		CreatedAt: timestamp(),
		UpdatedAt: timestamp(),
	}
	entry = applyLoreDraft(entry, draft)
	if err := validateLoreEntry(entry); err != nil {
		return LoreEntry{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := getPersonaFrom(ctx, tx, personaID); err != nil {
			return err
		}
		if err := validateLoreCapacity(ctx, tx, personaID, "", entry.Text); err != nil {
			return err
		}
		keywords, _ := json.Marshal(entry.Keywords)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO persona_lore(
				id, persona_id, text, keywords_json, enabled, created_at, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?)
		`, entry.ID, entry.PersonaID, entry.Text, string(keywords), entry.Enabled,
			entry.CreatedAt, entry.UpdatedAt)
		return err
	})
	return entry, err
}

// UpdateLore applies a partial entry update in one transaction.
func (s *Store) UpdateLore(ctx context.Context, personaID, loreID string, draft LoreDraft) (LoreEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var updated LoreEntry
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := loreEntryFrom(ctx, tx, personaID, loreID)
		if err != nil {
			return err
		}
		updated = applyLoreDraft(current, draft)
		updated.UpdatedAt = timestamp()
		if err := validateLoreEntry(updated); err != nil {
			return err
		}
		if err := validateLoreCapacity(ctx, tx, personaID, loreID, updated.Text); err != nil {
			return err
		}
		keywords, _ := json.Marshal(updated.Keywords)
		_, err = tx.ExecContext(ctx, `
			UPDATE persona_lore
			SET text = ?, keywords_json = ?, enabled = ?, updated_at = ?
			WHERE id = ? AND persona_id = ?
		`, updated.Text, string(keywords), updated.Enabled, updated.UpdatedAt,
			loreID, personaID)
		return err
	})
	return updated, err
}

// DeleteLore removes one entry.
func (s *Store) DeleteLore(ctx context.Context, personaID, loreID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ValidID(personaID) || !validLoreID(loreID) {
		return ErrNotFound
	}
	return s.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`DELETE FROM persona_lore WHERE id = ? AND persona_id = ?`,
			loreID, personaID)
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

// SelectLore applies one persona's off/relevant/full policy to recent visible
// chat text. It never runs for Autopilot; persona lore may shape a reply, not an
// autonomous motion decision.
func (s *Store) SelectLore(ctx context.Context, personaID string, recentText []string) (LoreSelection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, err := s.getLocked(ctx, personaID)
	if err != nil {
		return LoreSelection{}, err
	}
	// Fail closed if a manually edited or future database contains a mode this
	// binary does not understand. Unknown policy must never expand user-authored
	// prompt data as if "full" had been selected.
	if !ValidLoreMode(item.LoreMode) || item.LoreMode == LoreModeOff {
		return LoreSelection{}, nil
	}
	entries, err := listLoreRows(ctx, s.db.SQL(), personaID)
	if err != nil {
		return LoreSelection{}, err
	}
	if item.LoreMode == LoreModeRelevant {
		entries = relevantLoreEntries(entries, strings.Join(recentText, "\n"))
	}
	var selection LoreSelection
	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		selection.EntryIDs = append(selection.EntryIDs, entry.ID)
		selection.Texts = append(selection.Texts, entry.Text)
		selection.Characters += utf8.RuneCountInString(entry.Text)
	}
	return selection, nil
}

func listLoreRows(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, personaID string) ([]LoreEntry, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT id, persona_id, text, keywords_json, enabled, created_at, updated_at
		FROM persona_lore
		WHERE persona_id = ?
		ORDER BY created_at, id
	`, personaID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	entries := make([]LoreEntry, 0, MaxLoreEntries)
	for rows.Next() {
		entry, err := scanLoreEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func loreEntryFrom(ctx context.Context, tx *sql.Tx, personaID, loreID string) (LoreEntry, error) {
	if !ValidID(personaID) || !validLoreID(loreID) {
		return LoreEntry{}, ErrNotFound
	}
	entry, err := scanLoreEntry(tx.QueryRowContext(ctx, `
		SELECT id, persona_id, text, keywords_json, enabled, created_at, updated_at
		FROM persona_lore WHERE id = ? AND persona_id = ?
	`, loreID, personaID))
	if errors.Is(err, sql.ErrNoRows) {
		return LoreEntry{}, ErrNotFound
	}
	return entry, err
}

func scanLoreEntry(row rowScanner) (LoreEntry, error) {
	var entry LoreEntry
	var keywords string
	if err := row.Scan(&entry.ID, &entry.PersonaID, &entry.Text, &keywords,
		&entry.Enabled, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
		return LoreEntry{}, err
	}
	if err := json.Unmarshal([]byte(keywords), &entry.Keywords); err != nil {
		return LoreEntry{}, fmt.Errorf("decode persona lore keywords: %w", err)
	}
	if entry.Keywords == nil {
		entry.Keywords = []string{}
	}
	return entry, nil
}

func applyLoreDraft(entry LoreEntry, draft LoreDraft) LoreEntry {
	if draft.Text != nil {
		entry.Text = strings.TrimSpace(strings.ReplaceAll(*draft.Text, "\r\n", "\n"))
	}
	if draft.Keywords != nil {
		entry.Keywords = normalizeKeywords(*draft.Keywords)
	}
	if draft.Enabled != nil {
		entry.Enabled = *draft.Enabled
	}
	return entry
}

func validateLoreEntry(entry LoreEntry) error {
	if !ValidID(entry.PersonaID) || !validLoreID(entry.ID) {
		return ErrNotFound
	}
	if entry.Text == "" {
		return fmt.Errorf("%w: lore text is required", ErrInvalid)
	}
	if utf8.RuneCountInString(entry.Text) > MaxLoreTextChars {
		return fmt.Errorf("%w: lore text must be at most %d characters",
			ErrInvalid, MaxLoreTextChars)
	}
	if len(entry.Keywords) > MaxLoreKeywords {
		return fmt.Errorf("%w: lore may have at most %d keywords",
			ErrInvalid, MaxLoreKeywords)
	}
	for _, keyword := range entry.Keywords {
		if keyword == "" || utf8.RuneCountInString(keyword) > MaxLoreKeywordChars {
			return fmt.Errorf("%w: each lore keyword must contain 1 to %d characters",
				ErrInvalid, MaxLoreKeywordChars)
		}
	}
	return nil
}

func validateLoreCapacity(ctx context.Context, tx *sql.Tx, personaID, replacingID, replacementText string) error {
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM persona_lore WHERE persona_id = ? AND id <> ?`,
		personaID, replacingID,
	).Scan(&count); err != nil {
		return err
	}
	if count >= MaxLoreEntries {
		return fmt.Errorf("%w: a persona may have at most %d lore entries",
			ErrLimit, MaxLoreEntries)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT text FROM persona_lore WHERE persona_id = ? AND id <> ?`,
		personaID, replacingID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	total := utf8.RuneCountInString(replacementText)
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return err
		}
		total += utf8.RuneCountInString(text)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if total > MaxLoreTotalChars {
		return fmt.Errorf("%w: persona lore must total at most %d characters",
			ErrInvalid, MaxLoreTotalChars)
	}
	return nil
}

func relevantLoreEntries(entries []LoreEntry, text string) []LoreEntry {
	type match struct {
		entry LoreEntry
		score int
		order int
	}
	matches := make([]match, 0, len(entries))
	for index, entry := range entries {
		if !entry.Enabled {
			continue
		}
		score := 0
		for _, keyword := range entry.Keywords {
			if keywordMatch(text, keyword) {
				score += 100 + utf8.RuneCountInString(keyword)
			}
		}
		if score > 0 {
			matches = append(matches, match{entry: entry, score: score, order: index})
		}
	}
	sort.SliceStable(matches, func(left, right int) bool {
		if matches[left].score == matches[right].score {
			return matches[left].order < matches[right].order
		}
		return matches[left].score > matches[right].score
	})
	result := make([]LoreEntry, 0, len(matches))
	for _, item := range matches {
		result = append(result, item.entry)
	}
	return result
}

func keywordMatch(text, keyword string) bool {
	haystack := []rune(strings.ToLower(text))
	needle := []rune(strings.ToLower(strings.TrimSpace(keyword)))
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		if string(haystack[start:start+len(needle)]) != string(needle) {
			continue
		}
		leftOK := start == 0 || !isKeywordRune(haystack[start-1])
		end := start + len(needle)
		rightOK := end == len(haystack) || !isKeywordRune(haystack[end])
		if leftOK && rightOK {
			return true
		}
	}
	return false
}

func isKeywordRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsNumber(value) || value == '_'
}

func normalizeKeywords(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.Join(strings.Fields(value), " "))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validLoreID(id string) bool {
	if !strings.HasPrefix(id, loreIDPrefix) {
		return false
	}
	digits := id[len(loreIDPrefix):]
	if len(digits) != loreIDHexDigits {
		return false
	}
	for _, char := range digits {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func getPersonaFrom(ctx context.Context, tx *sql.Tx, id string) (Persona, error) {
	if !ValidID(id) {
		return Persona{}, ErrNotFound
	}
	item, err := scanPersona(tx.QueryRowContext(ctx,
		selectColumns+` FROM personas WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Persona{}, ErrNotFound
	}
	return item, err
}

func duplicateLoreEntries(ctx context.Context, tx *sql.Tx, sourceID, targetID string) error {
	entries, err := listLoreRows(ctx, tx, sourceID)
	if err != nil {
		return err
	}
	now := timestamp()
	for _, entry := range entries {
		keywords, _ := json.Marshal(entry.Keywords)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO persona_lore(
				id, persona_id, text, keywords_json, enabled, created_at, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?)
		`, loreIDPrefix+randomHex(loreIDHexDigits/2), targetID, entry.Text,
			string(keywords), entry.Enabled, now, now); err != nil {
			return err
		}
	}
	return nil
}
