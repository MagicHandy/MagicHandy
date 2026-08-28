package store

import (
	"database/sql"
	"testing"
)

func TestMigrationUpgradesV17ToAccountSchema(t *testing.T) {
	directory := t.TempDir()
	database, err := Open(directory)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := database.Path()
	if _, err := database.SQL().Exec(`
		INSERT INTO personas(
			id, name, description, chat_voice, reaction_style, prompt_set_id,
			default_focus_area, lore_mode, portrait_updated_at, last_used_at,
			created_at, updated_at
		) VALUES('keep-persona', 'Keep', '', 'warm', 'neutral', '', 'full', 'off', '', '', 'now', 'now')
	`); err != nil {
		_ = database.Close()
		t.Fatalf("seed v17 data: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	for _, statement := range []string{
		`DROP TABLE user_sessions`,
		`DROP TABLE user_accounts`,
		`PRAGMA user_version = 17`,
	} {
		if _, err := raw.Exec(statement); err != nil {
			_ = raw.Close()
			t.Fatalf("rewind %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	upgraded, err := Open(directory)
	if err != nil {
		t.Fatalf("reopen v17 database: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	assertTableExists(t, upgraded.SQL(), "user_accounts")
	assertTableExists(t, upgraded.SQL(), "user_sessions")
	var name string
	if err := upgraded.SQL().QueryRow(`SELECT name FROM personas WHERE id = 'keep-persona'`).Scan(&name); err != nil {
		t.Fatalf("read preserved persona: %v", err)
	}
	if name != "Keep" {
		t.Fatalf("preserved persona name = %q", name)
	}
}
