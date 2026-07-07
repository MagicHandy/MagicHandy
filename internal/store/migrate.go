package store

import (
	"database/sql"
	"fmt"
)

func migrate(db *DB) error {
	version, err := schemaUserVersion(db.sql)
	if err != nil {
		return err
	}
	if version > SchemaVersion {
		return fmt.Errorf("database schema version %d is newer than this binary supports (%d)", version, SchemaVersion)
	}
	for v := version; v < SchemaVersion; v++ {
		if err := runMigration(db, v+1); err != nil {
			return fmt.Errorf("migration to version %d: %w", v+1, err)
		}
	}
	return nil
}

func schemaUserVersion(sqlDB *sql.DB) (int, error) {
	var version int
	if err := sqlDB.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return version, nil
}

func runMigration(db *DB, version int) error {
	return db.withWrite(func(tx *sql.Tx) error {
		switch version {
		case 1:
			if err := migrateV1(tx); err != nil {
				return err
			}
		case 2:
			if err := migrateV2(tx); err != nil {
				return err
			}
		case 3:
			if err := migrateV3(tx); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown migration version %d", version)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
			return fmt.Errorf("set user_version: %w", err)
		}
		return nil
	})
}

func migrateV1(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			document TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE memory_meta (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE memories (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE prompt_sets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			system TEXT NOT NULL
		)`,
		`CREATE TABLE funscript_files (
			id TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			path TEXT NOT NULL,
			duration_ms INTEGER,
			action_count INTEGER,
			imported_at TEXT,
			hash TEXT UNIQUE
		)`,
		`CREATE TABLE motion_blocks (
			id TEXT PRIMARY KEY,
			source_file_id TEXT NOT NULL,
			start_ms INTEGER NOT NULL,
			end_ms INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			min_pos INTEGER,
			max_pos INTEGER,
			avg_pos REAL,
			amplitude INTEGER,
			zone TEXT,
			stroke_length TEXT,
			speed TEXT,
			rhythm TEXT,
			intensity REAL,
			tags_json TEXT,
			actions_json TEXT NOT NULL,
			user_rating INTEGER,
			times_used INTEGER NOT NULL DEFAULT 0,
			success_score REAL NOT NULL DEFAULT 0,
			blocked INTEGER NOT NULL DEFAULT 0,
			favorite INTEGER NOT NULL DEFAULT 0,
			created_at TEXT,
			FOREIGN KEY(source_file_id) REFERENCES funscript_files(id)
		)`,
		`CREATE TABLE saved_queues (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			items_json TEXT NOT NULL,
			actions_json TEXT,
			duration_ms INTEGER,
			funscript_file_id TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(funscript_file_id) REFERENCES funscript_files(id)
		)`,
		`CREATE TABLE personas (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			system_prompt TEXT NOT NULL,
			tone_json TEXT,
			mood_json TEXT,
			boundaries_json TEXT,
			motion_bias_json TEXT,
			created_at TEXT,
			updated_at TEXT
		)`,
		`CREATE INDEX idx_memories_created_at ON memories(created_at)`,
		`CREATE INDEX idx_prompt_sets_name ON prompt_sets(name)`,
		`CREATE INDEX idx_motion_blocks_source ON motion_blocks(source_file_id)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration v1: %w", err)
		}
	}
	return nil
}

func migrateV2(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE ui_preferences (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			locale TEXT NOT NULL DEFAULT 'en',
			locale_prompt_dismissed INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO ui_preferences (id, locale, locale_prompt_dismissed, updated_at)
		 VALUES (1, 'en', 0, datetime('now'))`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration v2: %w", err)
		}
	}
	return nil
}

func migrateV3(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE app_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			active_persona_id TEXT NOT NULL DEFAULT '',
			operation_mode TEXT NOT NULL DEFAULT 'hybrid',
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO app_state (id, active_persona_id, operation_mode, updated_at)
		 VALUES (1, '', 'hybrid', datetime('now'))`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration v3: %w", err)
		}
	}
	return nil
}
