// Package media owns MagicHandy's bounded local video catalog. It discovers
// files only after an explicit scan and never drives motion or transports.
package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	dbstore "github.com/mapledaemon/MagicHandy/internal/store"
)

var (
	// ErrVideoNotFound reports an unknown catalog identifier.
	ErrVideoNotFound = errors.New("media video not found")
	// ErrVideoUnavailable reports a catalog row whose file cannot be opened.
	ErrVideoUnavailable = errors.New("media video is unavailable")
	// ErrScanBusy reports a second scan or catalog-maintenance overlap.
	ErrScanBusy = errors.New("media scan is already running")
	// ErrNoLocations reports an explicit scan with no configured roots.
	ErrNoLocations = errors.New("no media library locations are configured")
	// ErrClosed reports use after catalog shutdown.
	ErrClosed = errors.New("media catalog is closed")
)

// Video is one scan-derived catalog row. Relative paths stay server-side;
// clients receive only the registered location and display metadata.
type Video struct {
	ID                    string  `json:"id"`
	LocationPath          string  `json:"location_path"`
	RelativePath          string  `json:"-"`
	DisplayName           string  `json:"display_name"`
	SizeBytes             int64   `json:"size_bytes"`
	ModifiedAt            string  `json:"modified_at"`
	DurationMillis        *int64  `json:"duration_ms"`
	FunscriptRelativePath *string `json:"-"`
	HasFunscript          bool    `json:"has_funscript"`
	Missing               bool    `json:"missing"`
	ScannedAt             string  `json:"scanned_at"`
}

// Summary is the constant-size media snapshot used by the regular app poll.
type Summary struct {
	AvailableCount int `json:"available_count"`
	VideoCount     int `json:"video_count"`
	PairedCount    int `json:"paired_count"`
}

// Catalog borrows the process datastore and owns the explicit scanner
// lifecycle. It has no startup scan and no timer.
type Catalog struct {
	db     *dbstore.DB
	logger *slog.Logger
	ownsDB bool

	operationMu sync.Mutex
	scanMu      sync.Mutex
	scanCancel  context.CancelFunc
	scanWG      sync.WaitGroup
	scanState   ScanState
	maintenance bool
	closed      bool
}

// Open opens a standalone catalog for tools and tests.
func Open(dataDir string, logger *slog.Logger) (*Catalog, error) {
	database, err := dbstore.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open media catalog: %w", err)
	}
	catalog := newCatalog(database, logger, true)
	return catalog, nil
}

// OpenWithDatabase borrows the process-owned datastore.
func OpenWithDatabase(database *dbstore.DB, logger *slog.Logger) (*Catalog, error) {
	if database == nil {
		return nil, errors.New("media datastore is required")
	}
	return newCatalog(database, logger, false), nil
}

func newCatalog(database *dbstore.DB, logger *slog.Logger, ownsDB bool) *Catalog {
	if logger == nil {
		logger = slog.Default()
	}
	return &Catalog{db: database, logger: logger, ownsDB: ownsDB, scanState: emptyScanState()}
}

// Close cancels and drains scanning before releasing an owned datastore.
func (c *Catalog) Close() error {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()

	c.scanMu.Lock()
	if c.closed {
		c.scanMu.Unlock()
		return nil
	}
	c.closed = true
	c.maintenance = true
	if c.scanCancel != nil {
		c.scanCancel()
	}
	c.scanMu.Unlock()
	c.scanWG.Wait()
	if c.ownsDB {
		return c.db.Close()
	}
	return nil
}

// List returns the bounded catalog, available entries first.
func (c *Catalog) List(ctx context.Context) ([]Video, error) {
	rows, err := c.db.SQL().QueryContext(ctx, `
		SELECT id, location_path, relative_path, display_name, size_bytes,
		       modified_at, duration_ms, funscript_relative_path, missing, scanned_at
		FROM media_videos
		ORDER BY missing, display_name COLLATE NOCASE, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	videos := make([]Video, 0)
	for rows.Next() {
		video, scanErr := scanVideo(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		videos = append(videos, video)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return videos, nil
}

// Summary returns indexed catalog counts without loading media rows.
func (c *Catalog) Summary(ctx context.Context) (Summary, error) {
	var summary Summary
	err := c.db.SQL().QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN missing = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN funscript_relative_path IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM media_videos
	`).Scan(&summary.VideoCount, &summary.AvailableCount, &summary.PairedCount)
	return summary, err
}

// Video returns one catalog row by opaque ID.
func (c *Catalog) Video(ctx context.Context, id string) (Video, error) {
	video, err := scanVideo(c.db.SQL().QueryRowContext(ctx, `
		SELECT id, location_path, relative_path, display_name, size_bytes,
		       modified_at, duration_ms, funscript_relative_path, missing, scanned_at
		FROM media_videos WHERE id = ?
	`, strings.TrimSpace(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return Video{}, ErrVideoNotFound
	}
	return video, err
}

// SetDuration stores browser-decoded metadata without probing media in Go.
func (c *Catalog) SetDuration(ctx context.Context, id string, durationMillis int64) error {
	if durationMillis <= 0 {
		return errors.New("video duration must be positive")
	}
	return c.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE media_videos SET duration_ms = ? WHERE id = ?`, durationMillis, strings.TrimSpace(id))
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrVideoNotFound
		}
		return nil
	})
}

// RetainLocations removes rows for roots deleted from settings. It first
// drains a scan so an obsolete root cannot be reinserted after the delete.
func (c *Catalog) RetainLocations(ctx context.Context, locations []string) (int64, error) {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()

	if err := c.beginMaintenance(); err != nil {
		return 0, err
	}
	defer c.endMaintenance()

	roots, err := normalizeRoots(locations)
	if err != nil && !errors.Is(err, ErrNoLocations) {
		return 0, err
	}
	keep := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		keep[pathKey(root)] = struct{}{}
	}

	var removed int64
	err = c.db.WithTx(ctx, func(tx *sql.Tx) error {
		rows, queryErr := tx.QueryContext(ctx, `SELECT DISTINCT location_path FROM media_videos`)
		if queryErr != nil {
			return queryErr
		}
		var stale []string
		for rows.Next() {
			var root string
			if scanErr := rows.Scan(&root); scanErr != nil {
				_ = rows.Close()
				return scanErr
			}
			if _, ok := keep[pathKey(root)]; !ok {
				stale = append(stale, root)
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			_ = rows.Close()
			return rowsErr
		}
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
		for _, root := range stale {
			result, deleteErr := tx.ExecContext(ctx, `DELETE FROM media_videos WHERE location_path = ?`, root)
			if deleteErr != nil {
				return deleteErr
			}
			count, countErr := result.RowsAffected()
			if countErr != nil {
				return countErr
			}
			removed += count
		}
		return nil
	})
	return removed, err
}

func (c *Catalog) beginMaintenance() error {
	c.scanMu.Lock()
	if c.closed {
		c.scanMu.Unlock()
		return ErrClosed
	}
	if c.maintenance {
		c.scanMu.Unlock()
		return ErrScanBusy
	}
	c.maintenance = true
	if c.scanCancel != nil {
		c.scanCancel()
	}
	c.scanMu.Unlock()
	c.scanWG.Wait()
	return nil
}

func (c *Catalog) endMaintenance() {
	c.scanMu.Lock()
	c.maintenance = false
	c.scanMu.Unlock()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVideo(row rowScanner) (Video, error) {
	var video Video
	var duration sql.NullInt64
	var funscript sql.NullString
	var missing int
	err := row.Scan(
		&video.ID, &video.LocationPath, &video.RelativePath, &video.DisplayName,
		&video.SizeBytes, &video.ModifiedAt, &duration, &funscript, &missing, &video.ScannedAt,
	)
	if err != nil {
		return Video{}, err
	}
	if duration.Valid {
		value := duration.Int64
		video.DurationMillis = &value
	}
	if funscript.Valid {
		value := funscript.String
		video.FunscriptRelativePath = &value
		video.HasFunscript = true
	}
	video.Missing = missing != 0
	return video, nil
}

// OpenVideo revalidates the catalog jail and opens a regular file for
// http.ServeContent. No HTTP endpoint accepts a filesystem path.
func (c *Catalog) OpenVideo(ctx context.Context, id string) (*os.File, Video, error) {
	video, err := c.Video(ctx, id)
	if err != nil {
		return nil, Video{}, err
	}
	if video.Missing {
		return nil, video, fmt.Errorf("%w: catalog entry is marked missing", ErrVideoUnavailable)
	}
	file, err := openInsideRoot(video.LocationPath, video.RelativePath)
	if err != nil {
		return nil, video, fmt.Errorf("%w: open catalog file: %w", ErrVideoUnavailable, err)
	}
	return file, video, nil
}

func openInsideRoot(root, relative string) (*os.File, error) {
	root, cleanRelative, rootInfo, err := validateRootedPath(root, relative)
	if err != nil {
		return nil, err
	}

	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rootHandle.Close() }()
	openedRootInfo, err := rootHandle.Stat(".")
	if err != nil || !os.SameFile(rootInfo, openedRootInfo) {
		return nil, ErrVideoUnavailable
	}

	expectedFileInfo, err := validateRootedComponents(rootHandle, cleanRelative)
	if err != nil {
		return nil, err
	}
	file, err := rootHandle.Open(cleanRelative)
	if err != nil {
		return nil, err
	}
	openedFileInfo, err := file.Stat()
	if err != nil || !openedFileInfo.Mode().IsRegular() || !os.SameFile(expectedFileInfo, openedFileInfo) {
		_ = file.Close()
		return nil, ErrVideoUnavailable
	}
	return file, nil
}

func validateRootedPath(root, relative string) (string, string, fs.FileInfo, error) {
	if filepath.IsAbs(relative) || relative == "" {
		return "", "", nil, ErrVideoUnavailable
	}
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", nil, err
	}
	cleanRelative := filepath.Clean(filepath.FromSlash(relative))
	candidate := filepath.Join(root, cleanRelative)
	if !pathWithin(root, candidate) {
		return "", "", nil, ErrVideoUnavailable
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return "", "", nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return "", "", nil, ErrVideoUnavailable
	}
	return root, cleanRelative, rootInfo, nil
}

func validateRootedComponents(rootHandle *os.Root, cleanRelative string) (fs.FileInfo, error) {
	current := ""
	var expectedFileInfo fs.FileInfo
	parts := strings.Split(cleanRelative, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, lstatErr := rootHandle.Lstat(current)
		if lstatErr != nil {
			return nil, lstatErr
		}
		if info.Mode()&fs.ModeSymlink != 0 || index < len(parts)-1 && !info.IsDir() {
			return nil, ErrVideoUnavailable
		}
		if index == len(parts)-1 {
			expectedFileInfo = info
		}
	}
	if expectedFileInfo == nil {
		return nil, ErrVideoUnavailable
	}
	return expectedFileInfo, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
