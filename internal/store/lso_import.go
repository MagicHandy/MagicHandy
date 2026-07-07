package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LSOImportResult counts rows copied from a legacy LSO app.sqlite database.
type LSOImportResult struct {
	Personas           int
	FunscriptFiles     int
	MotionBlocks       int
	SavedQueues        int
	PersonaMediaFiles  int
	ActivePersonaID    string
	SkippedExisting    bool
}

// ImportFromLSO copies library tables from an LSO SQLite file into an open
// MagicHandy datastore. Existing rows with the same primary key are skipped.
func ImportFromLSO(target *DB, lsoPath string) (LSOImportResult, error) {
	return ImportFromLSOWithOptions(target, lsoPath, LSOImportOptions{})
}

// LSOImportOptions configures optional LSO import side effects.
type LSOImportOptions struct {
	LSODataDir string
	TargetDir  string
}

// ImportFromLSOWithOptions copies LSO rows and optional persona media files.
func ImportFromLSOWithOptions(target *DB, lsoPath string, options LSOImportOptions) (LSOImportResult, error) {
	var result LSOImportResult
	if target == nil {
		return result, fmt.Errorf("target database is required")
	}
	abs, err := filepath.Abs(lsoPath)
	if err != nil {
		return result, fmt.Errorf("resolve LSO database path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return result, fmt.Errorf("LSO database: %w", err)
	}

	source, err := sql.Open("sqlite", abs)
	if err != nil {
		return result, fmt.Errorf("open LSO database: %w", err)
	}
	defer source.Close()

	err = target.withWrite(func(tx *sql.Tx) error {
		var copyErr error
		result.Personas, copyErr = copyTable(tx, source, "personas", `
			INSERT OR IGNORE INTO personas (
				id, name, description, system_prompt, tone_json, mood_json,
				boundaries_json, motion_bias_json, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"id", "name", "description", "system_prompt", "tone_json", "mood_json",
			"boundaries_json", "motion_bias_json", "created_at", "updated_at",
		)
		return copyErr
	})
	if err != nil {
		return result, err
	}

	activePersonaID, err := importLSOActivePersonaID(source)
	if err != nil {
		return result, err
	}
	if activePersonaID != "" {
		state, loadErr := target.LoadAppState()
		if loadErr != nil {
			return result, loadErr
		}
		changed := false
		if strings.TrimSpace(state.ActivePersonaID) == "" {
			state.ActivePersonaID = activePersonaID
			result.ActivePersonaID = activePersonaID
			changed = true
		}
		if mode := importLSOOperationMode(source); mode != "" && state.OperationMode == "hybrid" {
			state.OperationMode = mode
			changed = true
		}
		if changed {
			if _, saveErr := target.SaveAppState(state); saveErr != nil {
				return result, saveErr
			}
		}
	}

	err = target.withWrite(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
			return err
		}
		var copyErr error
		if result.FunscriptFiles, copyErr = copyTable(tx, source, "funscript_files", `
			INSERT OR IGNORE INTO funscript_files (
				id, filename, path, duration_ms, action_count, imported_at, hash
			) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"id", "filename", "path", "duration_ms", "action_count", "imported_at", "hash",
		); copyErr != nil {
			return copyErr
		}
		if result.MotionBlocks, copyErr = copyTableLenient(tx, source, "motion_blocks", `
			INSERT OR IGNORE INTO motion_blocks (
				id, source_file_id, start_ms, end_ms, duration_ms,
				min_pos, max_pos, avg_pos, amplitude, zone, stroke_length, speed, rhythm,
				intensity, tags_json, actions_json, user_rating, times_used, success_score,
				blocked, favorite, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"id", "source_file_id", "start_ms", "end_ms", "duration_ms",
			"min_pos", "max_pos", "avg_pos", "amplitude", "zone", "stroke_length", "speed", "rhythm",
			"intensity", "tags_json", "actions_json", "user_rating", "times_used", "success_score",
			"blocked", "favorite", "created_at",
		); copyErr != nil {
			return copyErr
		}
		if result.SavedQueues, copyErr = copyTable(tx, source, "saved_queues", `
			INSERT OR IGNORE INTO saved_queues (
				id, name, items_json, actions_json, duration_ms, funscript_file_id,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"id", "name", "items_json", "actions_json", "duration_ms", "funscript_file_id",
			"created_at", "updated_at",
		); copyErr != nil {
			return copyErr
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if options.LSODataDir != "" && options.TargetDir != "" {
		copied, copyErr := copyPersonaMedia(options.LSODataDir, options.TargetDir)
		if copyErr != nil {
			return result, copyErr
		}
		result.PersonaMediaFiles = copied
	}
	return result, nil
}

func copyTable(tx *sql.Tx, source *sql.DB, table, insertSQL string, columns ...string) (int, error) {
	if !lsoTableExists(source, table) {
		return 0, nil
	}
	query := "SELECT " + joinColumns(columns) + " FROM " + table
	rows, err := source.Query(query) // #nosec G202 -- table name is fixed by caller.
	if err != nil {
		return 0, fmt.Errorf("read %s from LSO database: %w", table, err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		dest := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return count, fmt.Errorf("scan %s row: %w", table, err)
		}
		res, err := tx.Exec(insertSQL, dest...)
		if err != nil {
			return count, fmt.Errorf("insert %s row: %w", table, err)
		}
		if affected, _ := res.RowsAffected(); affected > 0 {
			count++
		}
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("iterate %s: %w", table, err)
	}
	return count, nil
}

func copyTableLenient(tx *sql.Tx, source *sql.DB, table, insertSQL string, columns ...string) (int, error) {
	if !lsoTableExists(source, table) {
		return 0, nil
	}
	query := "SELECT " + joinColumns(columns) + " FROM " + table
	rows, err := source.Query(query) // #nosec G202 -- table name is fixed by caller.
	if err != nil {
		return 0, fmt.Errorf("read %s from LSO database: %w", table, err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		dest := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return count, fmt.Errorf("scan %s row: %w", table, err)
		}
		res, err := tx.Exec(insertSQL, dest...)
		if err != nil {
			continue
		}
		if affected, _ := res.RowsAffected(); affected > 0 {
			count++
		}
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("iterate %s: %w", table, err)
	}
	return count, nil
}

func lsoTableExists(db *sql.DB, table string) bool {
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
	).Scan(&name)
	return err == nil && name == table
}

func joinColumns(columns []string) string {
	if len(columns) == 0 {
		return ""
	}
	out := columns[0]
	for _, column := range columns[1:] {
		out += ", " + column
	}
	return out
}
