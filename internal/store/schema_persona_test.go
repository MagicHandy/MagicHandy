package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestOpenRejectsDamagedPersonaSchemaAtCurrentVersion(t *testing.T) {
	tests := []struct {
		name       string
		statements []string
	}{
		{
			name:       "session persona column",
			statements: []string{`ALTER TABLE chat_sessions DROP COLUMN persona_id`},
		},
		{
			name: "persona table",
			statements: []string{
				`DROP TABLE persona_lore`,
				`DROP TABLE personas`,
			},
		},
		{
			name:       "persona ordering index",
			statements: []string{`DROP INDEX personas_used`},
		},
		{
			name: "lore cascade",
			statements: []string{
				`DROP TABLE persona_lore`,
				`CREATE TABLE persona_lore (
					id TEXT PRIMARY KEY,
					persona_id TEXT NOT NULL,
					text TEXT NOT NULL,
					keywords_json TEXT NOT NULL DEFAULT '[]',
					enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`,
				`CREATE INDEX persona_lore_persona_created
					ON persona_lore(persona_id, created_at, id)`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := Open(dir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			path := db.Path()
			if err := db.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("raw open: %v", err)
			}
			for _, statement := range test.statements {
				if _, err := raw.Exec(statement); err != nil {
					_ = raw.Close()
					t.Fatalf("damage persona schema with %q: %v", statement, err)
				}
			}
			if err := raw.Close(); err != nil {
				t.Fatalf("raw close: %v", err)
			}

			if _, err := Open(dir); !errors.Is(err, ErrInvalidSchema) {
				t.Fatalf("Open damaged persona schema error = %v, want ErrInvalidSchema", err)
			}
		})
	}
}
