package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestPersonaReplyLengthMigrationKeepsPersonasFollowingSettings(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SQL().Exec(`INSERT INTO personas(id,name,description,chat_voice,reaction_style,prompt_set_id,default_focus_area,lore_mode,portrait_updated_at,last_used_at,created_at,updated_at)
		VALUES('persona-1','Rowan','','warm','neutral','','full','off','','','before','before')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", filepath.Join(dir, DatabaseFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`ALTER TABLE personas DROP COLUMN reply_length; PRAGMA user_version=29`); err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	// Twice: re-running the hook over a current table is a recovery path.
	for range 2 {
		db, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		var length, name string
		if err = db.SQL().QueryRow(`SELECT reply_length, name FROM personas WHERE id = 'persona-1'`).Scan(&length, &name); err != nil || length != "" || name != "Rowan" {
			t.Fatalf("migrated persona = %q %q, %v", length, name, err)
		}
		if _, err = db.SQL().Exec(`UPDATE personas SET reply_length = 'novel' WHERE id = 'persona-1'`); err == nil {
			t.Fatal("the column accepted an unknown reply length")
		}
		if _, err = db.SQL().Exec(`PRAGMA user_version=29`); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
