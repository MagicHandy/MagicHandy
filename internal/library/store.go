package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mapledaemon/MagicHandy/internal/funscript"
)

const schemaVersion = 1

// ErrNotFound reports a missing row.
var ErrNotFound = errors.New("library record not found")

// FunscriptFile is one imported source file.
type FunscriptFile struct {
	ID           string
	Filename     string
	Path         string
	DurationMS   int
	ActionCount  int
	ImportedAt   string
	Hash         string
	SourceFormat string
	Metadata     map[string]any
	Actions      []funscript.StoredAction
}

// MotionBlock is one segmented or full-script motion block.
type MotionBlock struct {
	ID              string
	SourceFileID    string
	StartMS         int
	EndMS           int
	DurationMS      int
	MinPos          int
	MaxPos          int
	AvgPos          float64
	Amplitude       int
	Zone            string
	StrokeLength    string
	Speed           string
	Rhythm          string
	Intensity       float64
	Tags            []string
	Actions         []funscript.StoredAction
	ContentHash     string
	SemanticSummary string
	DislikeCount    int
	UserRating      *int
	TimesUsed       int
	SuccessScore    float64
	Blocked         bool
	Favorite        bool
	CreatedAt       string
}

// Store owns SQLite CRUD for funscript_files and motion_blocks.
type Store struct {
	mu sync.RWMutex
	db *sql.DB
}

// Open opens or creates the library database at dbPath.
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the database handle.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) migrate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("create schema_meta: %w", err)
	}

	var version int
	row := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'version'`)
	switch err := row.Scan(&version); err {
	case nil:
		if version > schemaVersion {
			return fmt.Errorf("database schema version %d is newer than binary %d", version, schemaVersion)
		}
	case sql.ErrNoRows:
		version = 0
	default:
		return fmt.Errorf("read schema version: %w", err)
	}

	if version < 1 {
		if _, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS funscript_files (
				id TEXT PRIMARY KEY,
				filename TEXT NOT NULL,
				path TEXT NOT NULL,
				duration_ms INTEGER,
				action_count INTEGER,
				imported_at TEXT NOT NULL,
				hash TEXT UNIQUE,
				source_format TEXT,
				metadata_json TEXT,
				actions_json TEXT
			);

			CREATE TABLE IF NOT EXISTS motion_blocks (
				id TEXT PRIMARY KEY,
				source_file_id TEXT NOT NULL REFERENCES funscript_files(id) ON DELETE CASCADE,
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
				content_hash TEXT,
				semantic_summary TEXT,
				dislike_count INTEGER NOT NULL DEFAULT 0,
				user_rating INTEGER,
				times_used INTEGER NOT NULL DEFAULT 0,
				success_score REAL NOT NULL DEFAULT 0,
				blocked INTEGER NOT NULL DEFAULT 0,
				favorite INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			);
		`); err != nil {
			return fmt.Errorf("create library schema: %w", err)
		}
		if _, err := s.db.Exec(
			`INSERT INTO schema_meta(key, value) VALUES('version', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			fmt.Sprintf("%d", schemaVersion),
		); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	}
	if err := s.ensureLibraryColumnsLocked(); err != nil {
		return err
	}
	return nil
}

// CreateFunscriptFile inserts a new funscript file row.
func (s *Store) CreateFunscriptFile(ctx context.Context, file FunscriptFile) (FunscriptFile, error) {
	if strings.TrimSpace(file.ID) == "" {
		file.ID = newID()
	}
	if strings.TrimSpace(file.ImportedAt) == "" {
		file.ImportedAt = time.Now().UTC().Format(time.RFC3339)
	}
	metadataJSON, err := marshalJSON(file.Metadata)
	if err != nil {
		return FunscriptFile{}, err
	}
	actionsJSON, err := marshalJSON(file.Actions)
	if err != nil {
		return FunscriptFile{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO funscript_files(
			id, filename, path, duration_ms, action_count, imported_at,
			hash, source_format, metadata_json, actions_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		file.ID,
		file.Filename,
		file.Path,
		nullInt(file.DurationMS),
		nullInt(file.ActionCount),
		file.ImportedAt,
		nullString(file.Hash),
		nullString(file.SourceFormat),
		metadataJSON,
		actionsJSON,
	)
	if err != nil {
		return FunscriptFile{}, fmt.Errorf("insert funscript file: %w", err)
	}
	return file, nil
}

// GetFunscriptFile returns one funscript file by id.
func (s *Store) GetFunscriptFile(ctx context.Context, id string) (FunscriptFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, filename, path, duration_ms, action_count, imported_at,
		       hash, source_format, metadata_json, actions_json
		FROM funscript_files WHERE id = ?
	`, strings.TrimSpace(id))
	return scanFunscriptFile(row)
}

// GetFunscriptFileByHash returns a file with the given content hash.
func (s *Store) GetFunscriptFileByHash(ctx context.Context, hash string) (FunscriptFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, filename, path, duration_ms, action_count, imported_at,
		       hash, source_format, metadata_json, actions_json
		FROM funscript_files WHERE hash = ?
	`, strings.TrimSpace(hash))
	return scanFunscriptFile(row)
}

// ListFunscriptFiles returns imported files ordered by newest first.
func (s *Store) ListFunscriptFiles(ctx context.Context, limit int) ([]FunscriptFile, error) {
	if limit <= 0 {
		limit = 100
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, filename, path, duration_ms, action_count, imported_at,
		       hash, source_format, metadata_json, actions_json
		FROM funscript_files
		ORDER BY imported_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list funscript files: %w", err)
	}
	defer rows.Close()

	out := make([]FunscriptFile, 0, limit)
	for rows.Next() {
		file, err := scanFunscriptFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, file)
	}
	return out, rows.Err()
}

// UpdateFunscriptFile updates mutable file fields.
func (s *Store) UpdateFunscriptFile(ctx context.Context, file FunscriptFile) error {
	metadataJSON, err := marshalJSON(file.Metadata)
	if err != nil {
		return err
	}
	actionsJSON, err := marshalJSON(file.Actions)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `
		UPDATE funscript_files
		SET filename = ?, path = ?, duration_ms = ?, action_count = ?,
		    imported_at = ?, hash = ?, source_format = ?, metadata_json = ?, actions_json = ?
		WHERE id = ?
	`,
		file.Filename,
		file.Path,
		nullInt(file.DurationMS),
		nullInt(file.ActionCount),
		file.ImportedAt,
		nullString(file.Hash),
		nullString(file.SourceFormat),
		metadataJSON,
		actionsJSON,
		file.ID,
	)
	if err != nil {
		return fmt.Errorf("update funscript file: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteFunscriptFile deletes a file and its blocks.
func (s *Store) DeleteFunscriptFile(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `DELETE FROM funscript_files WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete funscript file: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateMotionBlock inserts one motion block row.
func (s *Store) CreateMotionBlock(ctx context.Context, block MotionBlock) (MotionBlock, error) {
	if strings.TrimSpace(block.ID) == "" {
		return MotionBlock{}, errors.New("motion block id is required")
	}
	if strings.TrimSpace(block.SourceFileID) == "" {
		return MotionBlock{}, errors.New("source_file_id is required")
	}
	if strings.TrimSpace(block.CreatedAt) == "" {
		block.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if block.ContentHash == "" {
		block.ContentHash = funscript.HashBlockActions(block.Actions)
	}
	if block.SemanticSummary == "" {
		block.SemanticSummary = funscript.SemanticSummaryFromRecord(blockToRecord(block))
	}

	tagsJSON, err := marshalJSON(block.Tags)
	if err != nil {
		return MotionBlock{}, err
	}
	actionsJSON, err := marshalJSON(block.Actions)
	if err != nil {
		return MotionBlock{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO motion_blocks(
			id, source_file_id, start_ms, end_ms, duration_ms,
			min_pos, max_pos, avg_pos, amplitude,
			zone, stroke_length, speed, rhythm, intensity,
			tags_json, actions_json, content_hash, semantic_summary,
			dislike_count, user_rating, times_used, success_score, blocked, favorite, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		block.ID,
		block.SourceFileID,
		block.StartMS,
		block.EndMS,
		block.DurationMS,
		nullInt(block.MinPos),
		nullInt(block.MaxPos),
		block.AvgPos,
		nullInt(block.Amplitude),
		nullString(block.Zone),
		nullString(block.StrokeLength),
		nullString(block.Speed),
		nullString(block.Rhythm),
		block.Intensity,
		tagsJSON,
		actionsJSON,
		nullString(block.ContentHash),
		nullString(block.SemanticSummary),
		block.DislikeCount,
		nullIntPtr(block.UserRating),
		block.TimesUsed,
		block.SuccessScore,
		boolInt(block.Blocked),
		boolInt(block.Favorite),
		block.CreatedAt,
	)
	if err != nil {
		return MotionBlock{}, fmt.Errorf("insert motion block: %w", err)
	}
	return block, nil
}

// GetMotionBlock returns one motion block by id.
func (s *Store) GetMotionBlock(ctx context.Context, id string) (MotionBlock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, source_file_id, start_ms, end_ms, duration_ms,
		       min_pos, max_pos, avg_pos, amplitude,
		       zone, stroke_length, speed, rhythm, intensity,
		       tags_json, actions_json, content_hash, semantic_summary,
		       dislike_count, user_rating, times_used, success_score, blocked, favorite, created_at
		FROM motion_blocks WHERE id = ?
	`, strings.TrimSpace(id))
	return scanMotionBlock(row)
}

// ListMotionBlocksByFileID returns blocks for one source file.
func (s *Store) ListMotionBlocksByFileID(ctx context.Context, fileID string) ([]MotionBlock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_file_id, start_ms, end_ms, duration_ms,
		       min_pos, max_pos, avg_pos, amplitude,
		       zone, stroke_length, speed, rhythm, intensity,
		       tags_json, actions_json, content_hash, semantic_summary,
		       dislike_count, user_rating, times_used, success_score, blocked, favorite, created_at
		FROM motion_blocks
		WHERE source_file_id = ?
		ORDER BY start_ms ASC
	`, strings.TrimSpace(fileID))
	if err != nil {
		return nil, fmt.Errorf("list motion blocks: %w", err)
	}
	defer rows.Close()

	out := make([]MotionBlock, 0, 16)
	for rows.Next() {
		block, err := scanMotionBlock(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, block)
	}
	return out, rows.Err()
}

// DeleteMotionBlock deletes one motion block.
func (s *Store) DeleteMotionBlock(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `DELETE FROM motion_blocks WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete motion block: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// BlockFilter selects motion blocks for library listing APIs.
type BlockFilter struct {
	Zone            string
	Speed           string
	Rhythm          string
	StrokeLength    string
	MinIntensity    *float64
	MaxIntensity    *float64
	MinDurationMS   *int
	MaxDurationMS   *int
	MinRating       *int
	FavoritesOnly   bool
	HideBlocked     bool
	Query           string
	Sort            string
	Offset          int
	Limit           int
}

// ListMotionBlocks returns motion blocks matching the filter.
func (s *Store) ListMotionBlocks(ctx context.Context, filter BlockFilter) ([]MotionBlock, error) {
	rows, err := s.queryMotionBlocks(ctx, filter, false)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MotionBlock, 0, filter.Limit)
	for rows.Next() {
		block, err := scanMotionBlock(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, block)
	}
	return out, rows.Err()
}

// CountMotionBlocks returns how many blocks match the filter.
func (s *Store) CountMotionBlocks(ctx context.Context, filter BlockFilter) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query, args := buildMotionBlockListSQL(filter, true)
	row := s.db.QueryRowContext(ctx, query, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count motion blocks: %w", err)
	}
	return count, nil
}

// CountMotionBlocksByFileID returns block count for one source file.
func (s *Store) CountMotionBlocksByFileID(ctx context.Context, fileID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM motion_blocks WHERE source_file_id = ?
	`, strings.TrimSpace(fileID))
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count motion blocks by file: %w", err)
	}
	return count, nil
}

// ListMotionBlockContentHashes returns all known content hashes.
func (s *Store) ListMotionBlockContentHashes(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT content_hash FROM motion_blocks WHERE content_hash IS NOT NULL AND content_hash != ''
	`)
	if err != nil {
		return nil, fmt.Errorf("list motion block hashes: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, 64)
	for rows.Next() {
		var hash sql.NullString
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		if hash.Valid {
			out = append(out, hash.String)
		}
	}
	return out, rows.Err()
}

// UpdateMotionBlockFields updates user-facing block metadata.
func (s *Store) UpdateMotionBlockFields(ctx context.Context, id string, favorite, blocked *bool, userRating *int, tags []string) error {
	block, err := s.GetMotionBlock(ctx, id)
	if err != nil {
		return err
	}
	if favorite != nil {
		block.Favorite = *favorite
	}
	if blocked != nil {
		block.Blocked = *blocked
	}
	if userRating != nil {
		block.UserRating = userRating
	}
	if tags != nil {
		block.Tags = tags
	}
	return s.updateMotionBlock(ctx, block)
}

func (s *Store) applyBlockFeedback(ctx context.Context, blockID, feedback string) (MotionBlock, error) {
	block, err := s.GetMotionBlock(ctx, blockID)
	if err != nil {
		return MotionBlock{}, err
	}
	switch strings.ToLower(strings.TrimSpace(feedback)) {
	case "like", "favorite", "fav":
		block.Favorite = true
		block.SuccessScore += 0.15
	case "dislike", "bad":
		block.DislikeCount++
		block.SuccessScore -= 0.2
		if block.DislikeCount >= 3 {
			block.Blocked = true
		}
	case "unblock":
		block.Blocked = false
	case "unfavorite":
		block.Favorite = false
	default:
		return MotionBlock{}, fmt.Errorf("unsupported feedback %q", feedback)
	}
	if block.SuccessScore < 0 {
		block.SuccessScore = 0
	}
	if block.SuccessScore > 1 {
		block.SuccessScore = 1
	}
	if err := s.updateMotionBlock(ctx, block); err != nil {
		return MotionBlock{}, err
	}
	return block, nil
}

func (s *Store) updateMotionBlock(ctx context.Context, block MotionBlock) error {
	tagsJSON, err := marshalJSON(block.Tags)
	if err != nil {
		return err
	}
	actionsJSON, err := marshalJSON(block.Actions)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `
		UPDATE motion_blocks
		SET favorite = ?, blocked = ?, user_rating = ?, tags_json = ?,
		    dislike_count = ?, success_score = ?, actions_json = ?
		WHERE id = ?
	`,
		boolInt(block.Favorite),
		boolInt(block.Blocked),
		nullIntPtr(block.UserRating),
		tagsJSON,
		block.DislikeCount,
		block.SuccessScore,
		actionsJSON,
		block.ID,
	)
	if err != nil {
		return fmt.Errorf("update motion block: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) nextScriptNumber(ctx context.Context) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(CAST(json_extract(metadata_json, '$.script_number') AS INTEGER)), 0)
		FROM funscript_files
		WHERE metadata_json IS NOT NULL AND metadata_json != ''
	`)
	var maxNumber int
	if err := row.Scan(&maxNumber); err != nil {
		return 1
	}
	return maxNumber + 1
}

func (s *Store) queryMotionBlocks(ctx context.Context, filter BlockFilter, countOnly bool) (*sql.Rows, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query, args := buildMotionBlockListSQL(filter, countOnly)
	return s.db.QueryContext(ctx, query, args...)
}

func buildMotionBlockListSQL(filter BlockFilter, countOnly bool) (string, []any) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 24
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	selectClause := `SELECT id, source_file_id, start_ms, end_ms, duration_ms,
		min_pos, max_pos, avg_pos, amplitude,
		zone, stroke_length, speed, rhythm, intensity,
		tags_json, actions_json, content_hash, semantic_summary,
		dislike_count, user_rating, times_used, success_score, blocked, favorite, created_at
		FROM motion_blocks`
	if countOnly {
		selectClause = `SELECT COUNT(*) FROM motion_blocks`
	}

	parts := []string{selectClause}
	args := make([]any, 0, 8)
	where := make([]string, 0, 8)

	if filter.HideBlocked {
		where = append(where, "blocked = 0")
	}
	if filter.FavoritesOnly {
		where = append(where, "favorite = 1")
	}
	if zone := strings.TrimSpace(filter.Zone); zone != "" {
		where = append(where, "zone = ?")
		args = append(args, zone)
	}
	if speed := strings.TrimSpace(filter.Speed); speed != "" {
		where = append(where, "speed = ?")
		args = append(args, speed)
	}
	if rhythm := strings.TrimSpace(filter.Rhythm); rhythm != "" {
		where = append(where, "rhythm = ?")
		args = append(args, rhythm)
	}
	if stroke := strings.TrimSpace(filter.StrokeLength); stroke != "" {
		where = append(where, "stroke_length = ?")
		args = append(args, stroke)
	}
	if filter.MinIntensity != nil {
		where = append(where, "intensity >= ?")
		args = append(args, *filter.MinIntensity)
	}
	if filter.MaxIntensity != nil {
		where = append(where, "intensity <= ?")
		args = append(args, *filter.MaxIntensity)
	}
	if filter.MinDurationMS != nil {
		where = append(where, "duration_ms >= ?")
		args = append(args, *filter.MinDurationMS)
	}
	if filter.MaxDurationMS != nil {
		where = append(where, "duration_ms <= ?")
		args = append(args, *filter.MaxDurationMS)
	}
	if filter.MinRating != nil {
		where = append(where, "user_rating >= ?")
		args = append(args, *filter.MinRating)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, "(id LIKE ? OR semantic_summary LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if len(where) > 0 {
		parts = append(parts, "WHERE "+strings.Join(where, " AND "))
	}
	if !countOnly {
		sortKey := strings.ToLower(strings.TrimSpace(filter.Sort))
		switch sortKey {
		case "duration":
			parts = append(parts, "ORDER BY duration_ms ASC")
		case "duration_desc":
			parts = append(parts, "ORDER BY duration_ms DESC")
		default:
			parts = append(parts, "ORDER BY success_score DESC, created_at DESC")
		}
		parts = append(parts, "LIMIT ? OFFSET ?")
		args = append(args, limit, offset)
	}
	return strings.Join(parts, " "), args
}

func (s *Store) ensureLibraryColumns() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureLibraryColumnsLocked()
}

func (s *Store) ensureLibraryColumnsLocked() error {
	type column struct {
		table string
		name  string
		ddl   string
	}
	columns := []column{
		{"funscript_files", "source_format", "TEXT"},
		{"funscript_files", "metadata_json", "TEXT"},
		{"funscript_files", "actions_json", "TEXT"},
		{"motion_blocks", "content_hash", "TEXT"},
		{"motion_blocks", "semantic_summary", "TEXT"},
		{"motion_blocks", "dislike_count", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, col := range columns {
		if !s.tableExistsLocked(col.table) {
			continue
		}
		rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", col.table))
		if err != nil {
			return fmt.Errorf("inspect %s columns: %w", col.table, err)
		}
		found := false
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == col.name {
				found = true
				break
			}
		}
		rows.Close()
		if found {
			continue
		}
		if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", col.table, col.name, col.ddl)); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				return fmt.Errorf("add column %s.%s: %w", col.table, col.name, err)
			}
		}
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS ix_motion_blocks_content_hash ON motion_blocks(content_hash)`); err != nil {
		if !s.tableExistsLocked("motion_blocks") {
			return nil
		}
		return fmt.Errorf("ensure content_hash index: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS ix_motion_blocks_source_file_id ON motion_blocks(source_file_id)`); err != nil {
		if !s.tableExistsLocked("motion_blocks") {
			return nil
		}
		return fmt.Errorf("ensure source_file_id index: %w", err)
	}
	return nil
}

func (s *Store) tableExistsLocked(name string) bool {
	row := s.db.QueryRow(`
		SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1
	`, name)
	var one int
	return row.Scan(&one) == nil
}

// DeleteMotionBlocksByFileID deletes all blocks for a source file.
func (s *Store) DeleteMotionBlocksByFileID(ctx context.Context, fileID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `DELETE FROM motion_blocks WHERE source_file_id = ?`, strings.TrimSpace(fileID))
	if err != nil {
		return 0, fmt.Errorf("delete motion blocks by file: %w", err)
	}
	return result.RowsAffected()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFunscriptFile(row rowScanner) (FunscriptFile, error) {
	var file FunscriptFile
	var duration, actionCount sql.NullInt64
	var hash, sourceFormat, metadataJSON, actionsJSON sql.NullString
	if err := row.Scan(
		&file.ID,
		&file.Filename,
		&file.Path,
		&duration,
		&actionCount,
		&file.ImportedAt,
		&hash,
		&sourceFormat,
		&metadataJSON,
		&actionsJSON,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FunscriptFile{}, ErrNotFound
		}
		return FunscriptFile{}, fmt.Errorf("scan funscript file: %w", err)
	}
	if duration.Valid {
		file.DurationMS = int(duration.Int64)
	}
	if actionCount.Valid {
		file.ActionCount = int(actionCount.Int64)
	}
	if hash.Valid {
		file.Hash = hash.String
	}
	if sourceFormat.Valid {
		file.SourceFormat = sourceFormat.String
	}
	if err := unmarshalJSON(metadataJSON.String, &file.Metadata); err != nil {
		return FunscriptFile{}, err
	}
	if err := unmarshalJSON(actionsJSON.String, &file.Actions); err != nil {
		return FunscriptFile{}, err
	}
	return file, nil
}

func scanMotionBlock(row rowScanner) (MotionBlock, error) {
	var block MotionBlock
	var minPos, maxPos, amplitude sql.NullInt64
	var zone, strokeLength, speed, rhythm, contentHash, semanticSummary sql.NullString
	var userRating sql.NullInt64
	var blocked, favorite int
	var tagsJSON, actionsJSON sql.NullString

	if err := row.Scan(
		&block.ID,
		&block.SourceFileID,
		&block.StartMS,
		&block.EndMS,
		&block.DurationMS,
		&minPos,
		&maxPos,
		&block.AvgPos,
		&amplitude,
		&zone,
		&strokeLength,
		&speed,
		&rhythm,
		&block.Intensity,
		&tagsJSON,
		&actionsJSON,
		&contentHash,
		&semanticSummary,
		&block.DislikeCount,
		&userRating,
		&block.TimesUsed,
		&block.SuccessScore,
		&blocked,
		&favorite,
		&block.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MotionBlock{}, ErrNotFound
		}
		return MotionBlock{}, fmt.Errorf("scan motion block: %w", err)
	}
	if minPos.Valid {
		block.MinPos = int(minPos.Int64)
	}
	if maxPos.Valid {
		block.MaxPos = int(maxPos.Int64)
	}
	if amplitude.Valid {
		block.Amplitude = int(amplitude.Int64)
	}
	if zone.Valid {
		block.Zone = zone.String
	}
	if strokeLength.Valid {
		block.StrokeLength = strokeLength.String
	}
	if speed.Valid {
		block.Speed = speed.String
	}
	if rhythm.Valid {
		block.Rhythm = rhythm.String
	}
	if contentHash.Valid {
		block.ContentHash = contentHash.String
	}
	if semanticSummary.Valid {
		block.SemanticSummary = semanticSummary.String
	}
	if userRating.Valid {
		value := int(userRating.Int64)
		block.UserRating = &value
	}
	block.Blocked = blocked != 0
	block.Favorite = favorite != 0
	if err := unmarshalJSON(tagsJSON.String, &block.Tags); err != nil {
		return MotionBlock{}, err
	}
	if err := unmarshalJSON(actionsJSON.String, &block.Actions); err != nil {
		return MotionBlock{}, err
	}
	return block, nil
}

func blockToRecord(block MotionBlock) funscript.BlockRecord {
	return funscript.BlockRecord{
		ID:              block.ID,
		StartMS:         block.StartMS,
		EndMS:           block.EndMS,
		DurationMS:      block.DurationMS,
		MinPos:          block.MinPos,
		MaxPos:          block.MaxPos,
		AvgPos:          block.AvgPos,
		Amplitude:       block.Amplitude,
		ActionCount:     len(block.Actions),
		Zone:            block.Zone,
		StrokeLength:    block.StrokeLength,
		Speed:           block.Speed,
		Rhythm:          block.Rhythm,
		Intensity:       block.Intensity,
		Tags:            block.Tags,
		Actions:         block.Actions,
		SemanticSummary: block.SemanticSummary,
	}
}

func marshalJSON(value any) (sql.NullString, error) {
	if value == nil {
		return sql.NullString{}, nil
	}
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			return sql.NullString{}, nil
		}
	case []funscript.StoredAction:
		if len(v) == 0 {
			return sql.NullString{}, nil
		}
	case []string:
		if len(v) == 0 {
			return sql.NullString{}, nil
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshal json: %w", err)
	}
	return sql.NullString{String: string(data), Valid: true}, nil
}

func unmarshalJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func nullInt(value int) sql.NullInt64 {
	if value == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(value), Valid: true}
}

func nullIntPtr(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fs-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
