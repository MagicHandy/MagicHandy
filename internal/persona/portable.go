package persona

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/chat"
)

const (
	// ArchiveMediaType is the content type used for portable persona archives.
	ArchiveMediaType = "application/vnd.magichandy.persona+zip"
	// ArchiveExtension keeps persona archives distinct from generic ZIP files.
	ArchiveExtension = ".mhpersona"
	// MaxArchiveBytes bounds the compressed upload before the ZIP reader sees it.
	MaxArchiveBytes = 4 << 20

	archiveSchema           = "magichandy.persona"
	archiveVersion          = 1
	archiveManifestName     = "persona.json"
	archivePortraitName     = "portrait.jpg"
	maxArchiveManifestBytes = 64 << 10
	maxPortablePromptName   = 80
	maxPortablePromptBytes  = 16 << 10
)

// PortableArchive is the versioned, self-contained persona document carried by
// a .mhpersona ZIP. Runtime identifiers and timestamps are deliberately absent.
type PortableArchive struct {
	Schema          string                   `json:"schema"`
	Version         int                      `json:"version"`
	Persona         PortablePersona          `json:"persona"`
	Lore            []PortableLoreEntry      `json:"lore"`
	BehaviorProfile *PortableBehaviorProfile `json:"behavior_profile,omitempty"`
	Assets          *PortableAssets          `json:"assets,omitempty"`
	Portrait        []byte                   `json:"-"`
}

// PortablePersona contains only user-authored and code-owned persona axes.
type PortablePersona struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	ChatVoice        string `json:"chat_voice"`
	ReactionStyle    string `json:"reaction_style"`
	PromptSetID      string `json:"prompt_set_id"`
	DefaultFocusArea string `json:"default_focus_area"`
	LoreMode         string `json:"lore_mode"`
}

// PortableLoreEntry leaves identity and timestamps to the importing install.
type PortableLoreEntry struct {
	Text     string   `json:"text"`
	Keywords []string `json:"keywords"`
	Enabled  bool     `json:"enabled"`
}

// PortableBehaviorProfile embeds custom profile text but carries only the ID
// of a built-in profile, whose trusted local definition wins on import.
type PortableBehaviorProfile struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	System  string `json:"system,omitempty"`
	Builtin bool   `json:"builtin"`
}

// PortableAssets names the bounded binary entries present in the archive.
type PortableAssets struct {
	Portrait string `json:"portrait,omitempty"`
}

// ExportArchive gathers one persona and writes a deterministic portable
// archive. The optional behavior profile is supplied by the HTTP composition
// layer because prompt-set storage is not owned by this package.
func (s *Store) ExportArchive(
	ctx context.Context,
	id string,
	profile *chat.PromptSet,
) ([]byte, Persona, error) {
	s.mu.RLock()
	item, err := s.getLocked(ctx, id)
	if err != nil {
		s.mu.RUnlock()
		return nil, Persona{}, err
	}
	entries, err := listLoreRows(ctx, s.db.SQL(), id)
	if err != nil {
		s.mu.RUnlock()
		return nil, Persona{}, err
	}
	var portrait []byte
	if item.HasPortrait {
		portrait, err = s.readPortrait(id)
		if err != nil {
			s.mu.RUnlock()
			return nil, Persona{}, fmt.Errorf("read persona portrait for export: %w", err)
		}
	}
	s.mu.RUnlock()

	if profile != nil && profile.ID != item.PromptSetID {
		return nil, Persona{}, fmt.Errorf(
			"behavior profile %q does not match persona profile %q",
			profile.ID,
			item.PromptSetID,
		)
	}
	var portableProfile *PortableBehaviorProfile
	if profile != nil {
		portableProfile = &PortableBehaviorProfile{
			ID:      profile.ID,
			Builtin: profile.Builtin,
		}
		if !profile.Builtin {
			portableProfile.Name = profile.Name
			portableProfile.System = profile.System
		}
	}
	portable := PortableArchive{
		Schema:  archiveSchema,
		Version: archiveVersion,
		Persona: PortablePersona{
			Name:             item.Name,
			Description:      item.Description,
			ChatVoice:        item.ChatVoice,
			ReactionStyle:    item.ReactionStyle,
			PromptSetID:      item.PromptSetID,
			DefaultFocusArea: item.DefaultFocusArea,
			LoreMode:         item.LoreMode,
		},
		Lore:            make([]PortableLoreEntry, 0, len(entries)),
		BehaviorProfile: portableProfile,
		Portrait:        portrait,
	}
	for _, entry := range entries {
		portable.Lore = append(portable.Lore, PortableLoreEntry{
			Text:     entry.Text,
			Keywords: append([]string(nil), entry.Keywords...),
			Enabled:  entry.Enabled,
		})
	}
	if len(portrait) > 0 {
		portable.Assets = &PortableAssets{Portrait: archivePortraitName}
	}
	encoded, err := encodeArchive(portable)
	if err != nil {
		return nil, Persona{}, err
	}
	return encoded, item, nil
}

// DecodeArchive parses and validates untrusted archive bytes without writing
// anything. Import orchestration can therefore resolve the behavior profile
// before committing a persona row.
func DecodeArchive(data []byte) (PortableArchive, error) {
	if len(data) == 0 || len(data) > MaxArchiveBytes {
		return PortableArchive{}, fmt.Errorf(
			"%w: persona archive must be between 1 byte and %d bytes",
			ErrInvalid,
			MaxArchiveBytes,
		)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return PortableArchive{}, fmt.Errorf("%w: persona archive is not a readable ZIP", ErrInvalid)
	}
	if len(reader.File) == 0 || len(reader.File) > 2 {
		return PortableArchive{}, fmt.Errorf("%w: persona archive has an unexpected file count", ErrInvalid)
	}

	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		limit := int64(0)
		switch file.Name {
		case archiveManifestName:
			if file.UncompressedSize64 > maxArchiveManifestBytes {
				return PortableArchive{}, fmt.Errorf(
					"%w: archive entry %q is too large",
					ErrInvalid,
					file.Name,
				)
			}
			limit = maxArchiveManifestBytes
		case archivePortraitName:
			if file.UncompressedSize64 > MaxPortraitBytes {
				return PortableArchive{}, fmt.Errorf(
					"%w: archive entry %q is too large",
					ErrInvalid,
					file.Name,
				)
			}
			limit = MaxPortraitBytes
		default:
			return PortableArchive{}, fmt.Errorf(
				"%w: unexpected persona archive entry %q",
				ErrInvalid,
				file.Name,
			)
		}
		if _, duplicate := files[file.Name]; duplicate {
			return PortableArchive{}, fmt.Errorf("%w: duplicate archive entry %q", ErrInvalid, file.Name)
		}
		if !file.FileInfo().Mode().IsRegular() {
			return PortableArchive{}, fmt.Errorf("%w: archive entry %q is not a regular file", ErrInvalid, file.Name)
		}
		content, readErr := readArchiveEntry(file, limit)
		if readErr != nil {
			return PortableArchive{}, readErr
		}
		files[file.Name] = content
	}

	manifest, ok := files[archiveManifestName]
	if !ok {
		return PortableArchive{}, fmt.Errorf("%w: persona.json is missing", ErrInvalid)
	}
	var portable PortableArchive
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&portable); err != nil {
		return PortableArchive{}, fmt.Errorf("%w: decode persona.json: %v", ErrInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return PortableArchive{}, fmt.Errorf("%w: persona.json has trailing content", ErrInvalid)
	}
	portable.Portrait = files[archivePortraitName]
	if err := normalizePortableArchive(&portable); err != nil {
		return PortableArchive{}, err
	}
	return portable, nil
}

// ImportPortable commits a decoded archive under fresh persona and lore IDs.
// promptSetID is the local behavior-profile ID resolved by the composition
// layer; it may differ from the source installation's custom profile ID.
func (s *Store) ImportPortable(
	ctx context.Context,
	portable PortableArchive,
	promptSetID string,
) (Persona, error) {
	if err := normalizePortableArchive(&portable); err != nil {
		return Persona{}, err
	}
	now := timestamp()
	item := Persona{
		ID:               idPrefix + randomHex(idHexDigits/2),
		Name:             portable.Persona.Name,
		Description:      portable.Persona.Description,
		ChatVoice:        portable.Persona.ChatVoice,
		ReactionStyle:    portable.Persona.ReactionStyle,
		PromptSetID:      strings.TrimSpace(promptSetID),
		DefaultFocusArea: portable.Persona.DefaultFocusArea,
		LoreMode:         portable.Persona.LoreMode,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := validate(item); err != nil {
		return Persona{}, err
	}
	lore := make([]LoreEntry, 0, len(portable.Lore))
	for _, portableEntry := range portable.Lore {
		lore = append(lore, LoreEntry{
			ID:        loreIDPrefix + randomHex(loreIDHexDigits/2),
			PersonaID: item.ID,
			Text:      portableEntry.Text,
			Keywords:  append([]string(nil), portableEntry.Keywords...),
			Enabled:   portableEntry.Enabled,
			CreatedAt: now,
			UpdatedAt: now,
		})
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
		if err := insert(ctx, tx, item); err != nil {
			return err
		}
		for _, entry := range lore {
			keywords, _ := json.Marshal(entry.Keywords)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO persona_lore(
					id, persona_id, text, keywords_json, enabled, created_at, updated_at
				) VALUES(?, ?, ?, ?, ?, ?, ?)
			`, entry.ID, entry.PersonaID, entry.Text, string(keywords), entry.Enabled,
				entry.CreatedAt, entry.UpdatedAt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Persona{}, err
	}
	if len(portable.Portrait) > 0 {
		if err := s.writePortrait(ctx, item.ID, portable.Portrait); err != nil {
			return Persona{}, s.rollbackPortableImport(item.ID, err)
		}
	}
	return s.getLocked(ctx, item.ID)
}

func encodeArchive(portable PortableArchive) ([]byte, error) {
	if err := normalizePortableArchive(&portable); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(portable, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode portable persona: %w", err)
	}
	manifest = append(manifest, '\n')
	if len(manifest) > maxArchiveManifestBytes {
		return nil, fmt.Errorf("%w: persona manifest is too large", ErrInvalid)
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err := writeArchiveEntry(writer, archiveManifestName, manifest); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if len(portable.Portrait) > 0 {
		if err := writeArchiveEntry(writer, archivePortraitName, portable.Portrait); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finalize persona archive: %w", err)
	}
	if buffer.Len() > MaxArchiveBytes {
		return nil, fmt.Errorf("%w: persona archive is too large", ErrInvalid)
	}
	return buffer.Bytes(), nil
}

func writeArchiveEntry(writer *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o600)
	header.Modified = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create persona archive entry %q: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("write persona archive entry %q: %w", name, err)
	}
	return nil
}

func readArchiveEntry(file *zip.File, limit int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open archive entry %q: %v", ErrInvalid, file.Name, err)
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read archive entry %q: %v", ErrInvalid, file.Name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: archive entry %q is too large", ErrInvalid, file.Name)
	}
	return data, nil
}

func normalizePortableArchive(portable *PortableArchive) error {
	if portable.Schema != archiveSchema || portable.Version != archiveVersion {
		return fmt.Errorf(
			"%w: unsupported persona archive schema %q version %d",
			ErrInvalid,
			portable.Schema,
			portable.Version,
		)
	}
	if err := normalizePortablePersona(portable); err != nil {
		return err
	}
	if err := normalizePortableLore(portable); err != nil {
		return err
	}
	if err := validatePortableAssets(portable); err != nil {
		return err
	}
	return normalizePortableBehaviorProfile(portable)
}

func normalizePortablePersona(portable *PortableArchive) error {
	item := Persona{
		Name:             collapseSpaces(portable.Persona.Name),
		Description:      collapseSpaces(portable.Persona.Description),
		ChatVoice:        normalizeToken(portable.Persona.ChatVoice),
		ReactionStyle:    normalizeToken(portable.Persona.ReactionStyle),
		PromptSetID:      strings.TrimSpace(portable.Persona.PromptSetID),
		DefaultFocusArea: normalizeToken(portable.Persona.DefaultFocusArea),
		LoreMode:         normalizeToken(portable.Persona.LoreMode),
	}
	if err := validate(item); err != nil {
		return err
	}
	portable.Persona = PortablePersona{
		Name:             item.Name,
		Description:      item.Description,
		ChatVoice:        item.ChatVoice,
		ReactionStyle:    item.ReactionStyle,
		PromptSetID:      item.PromptSetID,
		DefaultFocusArea: item.DefaultFocusArea,
		LoreMode:         item.LoreMode,
	}
	return nil
}

func normalizePortableLore(portable *PortableArchive) error {
	if len(portable.Lore) > MaxLoreEntries {
		return fmt.Errorf("%w: a persona may have at most %d lore entries", ErrLimit, MaxLoreEntries)
	}
	total := 0
	for index := range portable.Lore {
		entry := LoreEntry{
			ID:        loreIDPrefix + strings.Repeat("0", loreIDHexDigits),
			PersonaID: idPrefix + strings.Repeat("0", idHexDigits),
			Text:      strings.TrimSpace(strings.ReplaceAll(portable.Lore[index].Text, "\r\n", "\n")),
			Keywords:  normalizeKeywords(portable.Lore[index].Keywords),
			Enabled:   portable.Lore[index].Enabled,
		}
		if err := validateLoreEntry(entry); err != nil {
			return err
		}
		total += utf8.RuneCountInString(entry.Text)
		if total > MaxLoreTotalChars {
			return fmt.Errorf(
				"%w: persona lore must total at most %d characters",
				ErrInvalid,
				MaxLoreTotalChars,
			)
		}
		portable.Lore[index] = PortableLoreEntry{
			Text:     entry.Text,
			Keywords: entry.Keywords,
			Enabled:  entry.Enabled,
		}
	}
	if portable.Lore == nil {
		portable.Lore = []PortableLoreEntry{}
	}
	return nil
}

func validatePortableAssets(portable *PortableArchive) error {
	hasPortrait := len(portable.Portrait) > 0
	declaresPortrait := portable.Assets != nil && portable.Assets.Portrait != ""
	if hasPortrait != declaresPortrait {
		return fmt.Errorf("%w: portrait asset declaration does not match the archive", ErrInvalid)
	}
	if portable.Assets != nil && portable.Assets.Portrait != archivePortraitName {
		return fmt.Errorf("%w: unsupported portrait asset %q", ErrInvalid, portable.Assets.Portrait)
	}
	if hasPortrait {
		if err := validatePortrait(portable.Portrait); err != nil {
			return fmt.Errorf("%w: portrait asset is invalid: %v", ErrInvalid, err)
		}
	}
	return nil
}

func normalizePortableBehaviorProfile(portable *PortableArchive) error {
	profile := portable.BehaviorProfile
	if profile == nil {
		return nil
	}
	if profile.ID == "" || profile.ID != portable.Persona.PromptSetID {
		return fmt.Errorf("%w: behavior profile does not match prompt_set_id", ErrInvalid)
	}
	if profile.Builtin {
		if profile.Name != "" || profile.System != "" {
			return fmt.Errorf("%w: built-in behavior profiles must contain only an id", ErrInvalid)
		}
		return nil
	}
	profile.Name = collapseSpaces(profile.Name)
	profile.System = strings.TrimSpace(profile.System)
	if profile.Name == "" || utf8.RuneCountInString(profile.Name) > maxPortablePromptName {
		return fmt.Errorf("%w: behavior profile name is invalid", ErrInvalid)
	}
	if profile.System == "" || len(profile.System) > maxPortablePromptBytes {
		return fmt.Errorf("%w: behavior profile text is invalid", ErrInvalid)
	}
	return nil
}

func (s *Store) rollbackPortableImport(id string, cause error) error {
	cleanupCtx := context.Background()
	dbErr := s.db.WithTx(cleanupCtx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(cleanupCtx, "DELETE FROM personas WHERE id = ?", id)
		return err
	})
	_, _, fileErr := s.removePortraitFiles(id)
	if dbErr != nil {
		dbErr = fmt.Errorf("remove imported persona after portrait failure: %w", dbErr)
	}
	if fileErr != nil && !errors.Is(fileErr, os.ErrNotExist) {
		fileErr = fmt.Errorf("remove imported portrait after failure: %w", fileErr)
	}
	return errors.Join(cause, dbErr, fileErr)
}
