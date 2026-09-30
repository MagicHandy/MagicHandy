package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// MediaTagKey uses the same Unicode normalization for deduplication and SQL
// lookup. SQLite's built-in NOCASE collation only folds ASCII letters.
func MediaTagKey(tag string) string { return strings.ToLower(tag) }

// v27 -> v28 retains the first spelling and timestamp when case variants in
// a preview database collapse to one tag. Media files and metadata stay intact.
func migrateMediaTagKeys(ctx context.Context, tx *sql.Tx) error {
	exists, err := columnExists(ctx, tx, "media_video_tags", "tag_key")
	if err != nil || exists {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE media_video_tags_folded (
  video_id TEXT NOT NULL REFERENCES media_videos(id) ON DELETE CASCADE,
  tag TEXT NOT NULL CHECK (length(tag) BETWEEN 1 AND 40),
  tag_key TEXT NOT NULL CHECK (length(tag_key) BETWEEN 1 AND 40),
  created_at TEXT NOT NULL,
  PRIMARY KEY (video_id, tag_key)
 ); CREATE INDEX media_video_tags_folded_key ON media_video_tags_folded(tag_key, video_id)`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT video_id, tag, created_at FROM media_video_tags ORDER BY created_at, video_id, tag COLLATE BINARY`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, tag, created string
		if err := rows.Scan(&id, &tag, &created); err != nil {
			return err
		}
		key := MediaTagKey(tag)
		var canonical string
		err := tx.QueryRowContext(ctx, `SELECT tag FROM media_video_tags_folded WHERE tag_key = ? LIMIT 1`, key).Scan(&canonical)
		if err == nil {
			tag = canonical
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_video_tags_folded(video_id, tag, tag_key, created_at) VALUES(?, ?, ?, ?) ON CONFLICT(video_id, tag_key) DO NOTHING`, id, tag, key, created); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DROP TABLE media_video_tags;
  ALTER TABLE media_video_tags_folded RENAME TO media_video_tags;
  DROP INDEX media_video_tags_folded_key;
  CREATE INDEX media_video_tags_tag ON media_video_tags(tag_key, video_id)`)
	return err
}
