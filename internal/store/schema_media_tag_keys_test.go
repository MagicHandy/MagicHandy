package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestUnicodeTagMigrationPreservesAndMergesPreviewCuration(t *testing.T) {
	dir := t.TempDir()
	seedVersion26MediaRow(t, dir)
	raw, err := sql.Open("sqlite", dir+"/"+DatabaseFileName)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := raw.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateVideoMetadata(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO media_video_tags(video_id, tag, created_at) VALUES
  ('video', 'Été', '2026-09-27'), ('video', 'été', '2026-09-28'), ('video', 'Calm', '2026-09-27');
  UPDATE media_videos SET title = 'Keep me', notes = 'Keep these notes', rating = 4;
  PRAGMA user_version = 27`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = upgraded.Close() }()
	var count int
	if err := upgraded.SQL().QueryRow(`SELECT COUNT(*) FROM media_video_tags`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("case variants survived migration: %d tags", count)
	}
	var spelling, created, title, notes string
	var rating, offset int
	if err := upgraded.SQL().QueryRow(`SELECT tag, created_at FROM media_video_tags WHERE tag_key = 'été'`).Scan(&spelling, &created); err != nil {
		t.Fatal(err)
	}
	if spelling != "Été" || created != "2026-09-27" {
		t.Fatalf("first spelling or date lost: %q, %q", spelling, created)
	}
	if err := upgraded.SQL().QueryRow(`SELECT title, notes, rating, script_offset_ms FROM media_videos WHERE id = 'video'`).Scan(&title, &notes, &rating, &offset); err != nil {
		t.Fatal(err)
	}
	if title != "Keep me" || notes != "Keep these notes" || rating != 4 || offset != -70 {
		t.Fatal("migration changed catalog metadata")
	}
	if err := upgraded.WithTx(t.Context(), func(tx *sql.Tx) error { return migrateMediaTagKeys(context.Background(), tx) }); err != nil {
		t.Fatal(err)
	}
}
