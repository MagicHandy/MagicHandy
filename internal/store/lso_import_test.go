package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestImportFromLSO(t *testing.T) {
	t.Parallel()

	lsoDir := TestDir(t)
	lsoPath := filepath.Join(lsoDir, "app.sqlite")
	seedLSODatabase(t, lsoPath)

	targetDir := TestDir(t)
	target, err := Open(targetDir)
	if err != nil {
		t.Fatalf("Open target: %v", err)
	}
	defer func() { _ = target.Close() }()

	result, err := ImportFromLSO(target, lsoPath)
	if err != nil {
		t.Fatalf("ImportFromLSO: %v", err)
	}
	if result.Personas != 1 {
		t.Fatalf("personas = %d, want 1", result.Personas)
	}
	if result.FunscriptFiles != 1 {
		t.Fatalf("funscript_files = %d, want 1", result.FunscriptFiles)
	}
	if result.MotionBlocks != 1 {
		t.Fatalf("motion_blocks = %d, want 1", result.MotionBlocks)
	}

	repeat, err := ImportFromLSO(target, lsoPath)
	if err != nil {
		t.Fatalf("ImportFromLSO repeat: %v", err)
	}
	if repeat.Personas != 0 || repeat.MotionBlocks != 0 {
		t.Fatalf("repeat import should skip existing rows: %+v", repeat)
	}
}

func seedLSODatabase(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open LSO seed db: %v", err)
	}
	defer func() { _ = db.Close() }()

	stmts := []string{
		`CREATE TABLE personas (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT,
			system_prompt TEXT NOT NULL, tone_json TEXT, mood_json TEXT,
			boundaries_json TEXT, motion_bias_json TEXT,
			created_at TEXT, updated_at TEXT
		)`,
		`CREATE TABLE funscript_files (
			id TEXT PRIMARY KEY, filename TEXT NOT NULL, path TEXT NOT NULL,
			duration_ms INTEGER, action_count INTEGER, imported_at TEXT, hash TEXT UNIQUE
		)`,
		`CREATE TABLE motion_blocks (
			id TEXT PRIMARY KEY, source_file_id TEXT NOT NULL,
			start_ms INTEGER NOT NULL, end_ms INTEGER NOT NULL, duration_ms INTEGER NOT NULL,
			min_pos INTEGER, max_pos INTEGER, avg_pos REAL, amplitude INTEGER,
			zone TEXT, stroke_length TEXT, speed TEXT, rhythm TEXT, intensity REAL,
			tags_json TEXT, actions_json TEXT NOT NULL,
			user_rating INTEGER, times_used INTEGER NOT NULL DEFAULT 0,
			success_score REAL NOT NULL DEFAULT 0, blocked INTEGER NOT NULL DEFAULT 0,
			favorite INTEGER NOT NULL DEFAULT 0, created_at TEXT
		)`,
		`INSERT INTO personas (id, name, system_prompt, created_at, updated_at)
		 VALUES ('p1', 'Test', 'prompt', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO funscript_files (id, filename, path, duration_ms, action_count, imported_at)
		 VALUES ('f1', 'demo.funscript', '/demo.funscript', 1000, 2, '2026-01-01T00:00:00Z')`,
		`INSERT INTO motion_blocks (
			id, source_file_id, start_ms, end_ms, duration_ms, actions_json, created_at
		) VALUES (
			'b1', 'f1', 0, 1000, 1000, '[{"at":0,"pos":50}]', '2026-01-01T00:00:00Z'
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed LSO db: %v", err)
		}
	}
}
