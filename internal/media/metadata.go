package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

// Curation is the user's own description of a catalog video: a title, a
// rating, notes and tags. It lives beside the scan-derived facts, survives
// rescans with the row, follows a converted copy, and is never written into a
// media file. Removing a catalog row removes its curation with it.

// ErrInvalidMetadata reports curation that fails the bounds below.
var ErrInvalidMetadata = errors.New("invalid video metadata")

const (
	// MaxVideoTags bounds one video's tag set.
	MaxVideoTags = 32
	// MaxBulkVideos bounds one bulk tag edit.
	MaxBulkVideos = 500
	maxBulkTags   = 16
)

// MetadataPatch changes only the fields present. An empty title or note and a
// zero rating clear the value; Tags, when present, replaces the whole set.
type MetadataPatch struct {
	Title  *string   `json:"title,omitempty"`
	Rating *int      `json:"rating,omitempty"`
	Notes  *string   `json:"notes,omitempty"`
	Tags   *[]string `json:"tags,omitempty"`
}

// TagCount is one tag in use and the number of videos carrying it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// NormalizeTitle trims a display title. Empty means "use the file name".
func NormalizeTitle(value string) (*string, error) {
	return normalizeText(value, dbstore.MediaTitleMaxRunes, false, "title")
}

// NormalizeNotes trims notes. Line breaks and tabs are kept.
func NormalizeNotes(value string) (*string, error) {
	return normalizeText(value, dbstore.MediaNotesMaxRunes, true, "notes")
}

func normalizeText(value string, maxRunes int, multiline bool, field string) (*string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if !utf8.ValidString(value) {
		return nil, fmt.Errorf("%w: %s must be valid text", ErrInvalidMetadata, field)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return nil, fmt.Errorf("%w: %s must be at most %d characters", ErrInvalidMetadata, field, maxRunes)
	}
	for _, character := range value {
		layout := multiline && (character == '\n' || character == '\t' || character == '\r')
		if unicode.IsControl(character) && !layout {
			return nil, fmt.Errorf("%w: %s contains a control character", ErrInvalidMetadata, field)
		}
	}
	return &value, nil
}

// NormalizeRating accepts 1 to 5 stars; 0 clears the rating.
func NormalizeRating(value int) (*int, error) {
	if value == 0 {
		return nil, nil
	}
	if value < 1 || value > 5 {
		return nil, fmt.Errorf("%w: rating must be from 1 to 5, or 0 to clear it", ErrInvalidMetadata)
	}
	return &value, nil
}

// NormalizeTag trims a tag and collapses inner whitespace. Commas are refused
// because the tag field separates tags with them.
func NormalizeTag(value string) (string, error) {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "", fmt.Errorf("%w: a tag cannot be empty", ErrInvalidMetadata)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > dbstore.MediaTagMaxRunes {
		return "", fmt.Errorf("%w: a tag must be at most %d characters", ErrInvalidMetadata, dbstore.MediaTagMaxRunes)
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == ',' {
			return "", fmt.Errorf("%w: a tag cannot contain commas or control characters", ErrInvalidMetadata)
		}
	}
	return value, nil
}

// NormalizeTags normalizes a tag set, removes case-insensitive duplicates
// (the first spelling wins) and orders it for stable display.
func NormalizeTags(values []string, limit int) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	tags := make([]string, 0, len(values))
	for _, value := range values {
		tag, err := NormalizeTag(value)
		if err != nil {
			return nil, err
		}
		key := dbstore.MediaTagKey(tag)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) > limit {
		return nil, fmt.Errorf("%w: use at most %d tags", ErrInvalidMetadata, limit)
	}
	sortTags(tags)
	return tags, nil
}

func sortTags(tags []string) {
	sort.SliceStable(tags, func(left, right int) bool {
		return strings.ToLower(tags[left]) < strings.ToLower(tags[right])
	})
}

// UpdateMetadata applies a patch to one video and returns the updated row.
func (c *Catalog) UpdateMetadata(ctx context.Context, id string, patch MetadataPatch, authorize ...MetadataAuthorization) (Video, error) {
	id = strings.TrimSpace(id)
	values, err := normalizeMetadataPatch(patch)
	if err != nil {
		return Video{}, err
	}
	err = c.metadataTx(ctx, authorize, func(tx *sql.Tx) error {
		if err := requireVideoRow(ctx, tx, id); err != nil {
			return err
		}
		return applyMetadataPatch(ctx, tx, id, patch, values)
	})
	if err != nil {
		return Video{}, err
	}
	return c.Video(ctx, id)
}

// normalizedMetadata holds a patch's validated values. Which fields change is
// still read from the patch, where a nil field means "leave unchanged".
type normalizedMetadata struct {
	title, notes *string
	rating       *int
	tags         []string
}

func normalizeMetadataPatch(patch MetadataPatch) (normalizedMetadata, error) {
	var values normalizedMetadata
	var err error
	if patch.Title != nil {
		if values.title, err = NormalizeTitle(*patch.Title); err != nil {
			return values, err
		}
	}
	if patch.Rating != nil {
		if values.rating, err = NormalizeRating(*patch.Rating); err != nil {
			return values, err
		}
	}
	if patch.Notes != nil {
		if values.notes, err = NormalizeNotes(*patch.Notes); err != nil {
			return values, err
		}
	}
	if patch.Tags != nil {
		if values.tags, err = NormalizeTags(*patch.Tags, MaxVideoTags); err != nil {
			return values, err
		}
	}
	return values, nil
}

func applyMetadataPatch(ctx context.Context, tx *sql.Tx, id string, patch MetadataPatch, values normalizedMetadata) error {
	for _, column := range []struct {
		changed bool
		query   string
		value   any
	}{
		{patch.Title != nil, `UPDATE media_videos SET title = ? WHERE id = ?`, nullableString(values.title)},
		{patch.Rating != nil, `UPDATE media_videos SET rating = ? WHERE id = ?`, nullableInt(values.rating)},
		{patch.Notes != nil, `UPDATE media_videos SET notes = ? WHERE id = ?`, nullableString(values.notes)},
	} {
		if !column.changed {
			continue
		}
		if _, err := tx.ExecContext(ctx, column.query, column.value, id); err != nil {
			return err
		}
	}
	if patch.Tags == nil {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_video_tags WHERE video_id = ?`, id); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tag := range values.tags {
		if err := insertTag(ctx, tx, id, tag, now); err != nil {
			return err
		}
	}
	return nil
}

// UpdateTags adds and removes tags across several videos in one transaction.
// Every video must exist; one that does not fails the whole edit.
func (c *Catalog) UpdateTags(ctx context.Context, ids []string, add, remove []string, authorize ...MetadataAuthorization) ([]Video, error) {
	cleanIDs := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		cleanIDs = append(cleanIDs, id)
	}
	if len(cleanIDs) == 0 || len(cleanIDs) > MaxBulkVideos {
		return nil, fmt.Errorf("%w: choose from 1 to %d videos", ErrInvalidMetadata, MaxBulkVideos)
	}
	addTags, err := NormalizeTags(add, maxBulkTags)
	if err != nil {
		return nil, err
	}
	removeTags, err := NormalizeTags(remove, maxBulkTags)
	if err != nil {
		return nil, err
	}
	if len(addTags)+len(removeTags) == 0 {
		return nil, fmt.Errorf("%w: choose a tag to add or remove", ErrInvalidMetadata)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err = c.metadataTx(ctx, authorize, func(tx *sql.Tx) error {
		for _, id := range cleanIDs {
			if err := requireVideoRow(ctx, tx, id); err != nil {
				return err
			}
			for _, tag := range removeTags {
				if _, err := tx.ExecContext(ctx, `DELETE FROM media_video_tags WHERE video_id = ? AND tag_key = ?`, id, dbstore.MediaTagKey(tag)); err != nil {
					return err
				}
			}
			for _, tag := range addTags {
				if err := insertTag(ctx, tx, id, tag, now); err != nil {
					return err
				}
			}
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_video_tags WHERE video_id = ?`, id).Scan(&count); err != nil {
				return err
			}
			if count > MaxVideoTags {
				return fmt.Errorf("%w: a video can have at most %d tags", ErrInvalidMetadata, MaxVideoTags)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c.metadataVideos(ctx, cleanIDs)
}

// Tags lists every tag in use with the number of videos carrying it.
func (c *Catalog) Tags(ctx context.Context) ([]TagCount, error) {
	rows, err := c.db.SQL().QueryContext(ctx, `
		SELECT MIN(tag), COUNT(*) FROM media_video_tags
		GROUP BY tag_key
		ORDER BY tag_key
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tags := make([]TagCount, 0)
	for rows.Next() {
		var entry TagCount
		if err := rows.Scan(&entry.Tag, &entry.Count); err != nil {
			return nil, err
		}
		tags = append(tags, entry)
	}
	return tags, rows.Err()
}

// RenameTag renames a tag everywhere. Renaming onto an existing tag merges
// the two, and the new spelling is applied to every video carrying it.
func (c *Catalog) RenameTag(ctx context.Context, from, to string, authorize ...MetadataAuthorization) (int, error) {
	from, err := NormalizeTag(from)
	if err != nil {
		return 0, err
	}
	to, err = NormalizeTag(to)
	if err != nil {
		return 0, err
	}
	var affected int
	fromKey, toKey := dbstore.MediaTagKey(from), dbstore.MediaTagKey(to)
	err = c.metadataTx(ctx, authorize, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_video_tags WHERE tag_key = ?`, fromKey).Scan(&affected); err != nil {
			return err
		}
		if fromKey != toKey {
			// A video already carrying the target keeps one copy of it.
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM media_video_tags
				WHERE tag_key = ? AND video_id IN (SELECT video_id FROM media_video_tags WHERE tag_key = ?)
			`, fromKey, toKey); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE media_video_tags SET tag = ?, tag_key = ? WHERE tag_key = ?`, to, toKey, fromKey); err != nil {
				return err
			}
		}
		// The typed spelling becomes the library's spelling of the tag.
		_, err := tx.ExecContext(ctx, `UPDATE media_video_tags SET tag = ? WHERE tag_key = ?`, to, toKey)
		return err
	})
	return affected, err
}

// DeleteTag removes a tag from every video and reports how many carried it.
func (c *Catalog) DeleteTag(ctx context.Context, tag string, authorize ...MetadataAuthorization) (int, error) {
	tag, err := NormalizeTag(tag)
	if err != nil {
		return 0, err
	}
	var affected int64
	err = c.metadataTx(ctx, authorize, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM media_video_tags WHERE tag_key = ?`, dbstore.MediaTagKey(tag))
		if err != nil {
			return err
		}
		affected, err = result.RowsAffected()
		return err
	})
	return int(affected), err
}

// insertTag keeps one spelling per tag across the library: a tag that already
// exists in another spelling is stored the way it is already written.
func insertTag(ctx context.Context, tx *sql.Tx, videoID, tag, now string) error {
	var existing string
	key := dbstore.MediaTagKey(tag)
	err := tx.QueryRowContext(ctx, `SELECT tag FROM media_video_tags WHERE tag_key = ? LIMIT 1`, key).Scan(&existing)
	switch {
	case err == nil:
		tag = existing
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO media_video_tags(video_id, tag, tag_key, created_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(video_id, tag_key) DO NOTHING
	`, videoID, tag, key, now)
	return err
}

func requireVideoRow(ctx context.Context, tx *sql.Tx, id string) error {
	var present int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM media_videos WHERE id = ?`, id).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrVideoNotFound
	}
	return err
}

// attachTags fills every row's tag list from one query over the tag table.
func (c *Catalog) attachTags(ctx context.Context, videos []Video) error {
	rows, err := c.db.SQL().QueryContext(ctx, `SELECT video_id, tag FROM media_video_tags ORDER BY video_id`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	byVideo := make(map[string][]string)
	for rows.Next() {
		var videoID, tag string
		if err := rows.Scan(&videoID, &tag); err != nil {
			return err
		}
		byVideo[videoID] = append(byVideo[videoID], tag)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for index := range videos {
		tags := byVideo[videos[index].ID]
		if tags == nil {
			tags = []string{}
		}
		sortTags(tags)
		videos[index].Tags = tags
	}
	return nil
}

// attachTagsFor loads the tags of a few rows by identifier.
func (c *Catalog) attachTagsFor(ctx context.Context, videos []Video) error {
	if len(videos) == 0 {
		return nil
	}
	ids := make([]string, len(videos))
	for index, video := range videos {
		ids[index] = video.ID
	}
	placeholders, args := metadataIDs(ids)
	rows, err := c.db.SQL().QueryContext(ctx, `SELECT video_id, tag FROM media_video_tags WHERE video_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[string][]string, len(ids))
	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return err
		}
		byID[id] = append(byID[id], tag)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for index := range videos {
		tags := byID[videos[index].ID]
		if tags == nil {
			tags = []string{}
		}
		sortTags(tags)
		videos[index].Tags = tags
	}
	return nil
}

// carryCuration copies a source row's curation onto its converted copy inside
// the adoption transaction. Values already on the copy are kept.
func carryCuration(ctx context.Context, tx *sql.Tx, sourceID, targetID string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE media_videos SET
			title = COALESCE(title, (SELECT title FROM media_videos WHERE id = ?)),
			rating = COALESCE(rating, (SELECT rating FROM media_videos WHERE id = ?)),
			notes = COALESCE(notes, (SELECT notes FROM media_videos WHERE id = ?))
		WHERE id = ?
	`, sourceID, sourceID, sourceID, targetID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO media_video_tags(video_id, tag, tag_key, created_at)
		SELECT ?, tag, tag_key, created_at FROM media_video_tags WHERE video_id = ?
		ON CONFLICT(video_id, tag_key) DO NOTHING
	`, targetID, sourceID)
	return err
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
