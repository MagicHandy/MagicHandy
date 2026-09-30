package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Curation bounds shared by the migration checks and the media domain.
const (
	MediaTitleMaxRunes = 200
	MediaNotesMaxRunes = 2000
	MediaTagMaxRunes   = 40
)

// v26 -> v27: user curation of catalog rows. Titles, ratings, notes and tags
// are the user's own descriptions of a video; they live beside the
// scan-derived facts and are never written into media files. The columns are
// added by a guarded hook for the same reason as v14 and v15: SQLite has no
// conditional ADD COLUMN and re-running a migration is a normal recovery path.
// Tag rows cascade with their video, so removing a catalog row removes them.
var mediaMetadataColumns = []struct {
	name      string
	statement string
}{
	{"title", `ALTER TABLE media_videos ADD COLUMN title TEXT CHECK (title IS NULL OR length(title) BETWEEN 1 AND 200)`},
	{"rating", `ALTER TABLE media_videos ADD COLUMN rating INTEGER CHECK (rating IS NULL OR rating BETWEEN 1 AND 5)`},
	{"notes", `ALTER TABLE media_videos ADD COLUMN notes TEXT CHECK (notes IS NULL OR length(notes) BETWEEN 1 AND 2000)`},
}

var mediaMetadataTables = []string{
	`CREATE TABLE IF NOT EXISTS media_video_tags (
		video_id TEXT NOT NULL REFERENCES media_videos(id) ON DELETE CASCADE,
		tag TEXT NOT NULL COLLATE NOCASE CHECK (length(tag) BETWEEN 1 AND 40),
		created_at TEXT NOT NULL,
		PRIMARY KEY (video_id, tag)
	)`,
	`CREATE INDEX IF NOT EXISTS media_video_tags_tag ON media_video_tags(tag, video_id)`,
}

func migrateVideoMetadata(ctx context.Context, tx *sql.Tx) error {
	for _, column := range mediaMetadataColumns {
		exists, err := columnExists(ctx, tx, "media_videos", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, column.statement); err != nil {
			return fmt.Errorf("add media %s column: %w", column.name, err)
		}
	}
	for _, statement := range mediaMetadataTables {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create media tag schema: %w", err)
		}
	}
	return nil
}
