package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestMigrationAddsVideoMetadataWithoutTouchingCatalogRows(t *testing.T) {
	dir := t.TempDir()
	database, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := database.Path()
	if _, err := database.SQL().Exec(`
		INSERT INTO media_videos(
			id, location_path, relative_path, display_name, size_bytes,
			modified_at, duration_ms, funscript_relative_path, missing, scanned_at, script_offset_ms
		) VALUES('video', 'C:/media', 'clip.mp4', 'clip', 1, 'now', 5000, NULL, 0, 'now', -70)
	`); err != nil {
		t.Fatalf("seed media row: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{
		"DROP TABLE media_video_tags",
		"ALTER TABLE media_videos DROP COLUMN title",
		"ALTER TABLE media_videos DROP COLUMN rating",
		"ALTER TABLE media_videos DROP COLUMN notes",
		"PRAGMA user_version = 26",
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("rewind %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	upgraded, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen v26 database: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	db := upgraded.SQL()

	var title, notes sql.NullString
	var rating sql.NullInt64
	var offset int
	if err := db.QueryRow(`SELECT title, rating, notes, script_offset_ms FROM media_videos WHERE id = 'video'`).Scan(&title, &rating, &notes, &offset); err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if title.Valid || rating.Valid || notes.Valid || offset != -70 {
		t.Fatalf("migrated row = title %v rating %v notes %v offset %d; want empty curation and the saved offset", title, rating, notes, offset)
	}

	if _, err := db.Exec(`UPDATE media_videos SET rating = 6 WHERE id = 'video'`); err == nil {
		t.Fatal("rating 6 was accepted")
	}
	if _, err := db.Exec(`UPDATE media_videos SET title = '' WHERE id = 'video'`); err == nil {
		t.Fatal("an empty title was stored instead of NULL")
	}
	if _, err := db.Exec(`INSERT INTO media_video_tags(video_id, tag, created_at) VALUES('video', 'Calm', 'now')`); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO media_video_tags(video_id, tag, created_at) VALUES('video', 'calm', 'now')`); err == nil {
		t.Fatal("a tag differing only by case was stored twice")
	}
	if _, err := db.Exec(`INSERT INTO media_video_tags(video_id, tag, created_at) VALUES('missing-video', 'calm', 'now')`); err == nil {
		t.Fatal("a tag for an unknown video was stored")
	}
	if _, err := db.Exec(`DELETE FROM media_videos WHERE id = 'video'`); err != nil {
		t.Fatalf("delete video: %v", err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_video_tags`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("%d tags outlived their video", remaining)
	}
}

func TestVideoMetadataMigrationCanRunTwice(t *testing.T) {
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.WithTx(context.Background(), func(tx *sql.Tx) error {
		return migrateVideoMetadata(context.Background(), tx)
	}); err != nil {
		t.Fatalf("re-run metadata migration: %v", err)
	}
}
