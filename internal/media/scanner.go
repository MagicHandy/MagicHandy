package media

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	// MaxScanDepth bounds recursion below each registered root.
	MaxScanDepth = 6
	// MaxFilesPerLocation bounds all encountered regular files, not just videos.
	MaxFilesPerLocation = 10_000
)

var errFileLimit = errors.New("media scan file limit reached")

// ScanSummary is the durable-catalog delta from one explicit scan.
type ScanSummary struct {
	Locations int         `json:"locations"`
	Added     int         `json:"added"`
	Updated   int         `json:"updated"`
	Missing   int         `json:"missing"`
	Removed   int         `json:"removed"`
	Skipped   int         `json:"skipped"`
	Issues    []ScanIssue `json:"issues"`
}

// ScanIssue reports a root that could not be completely enumerated.
type ScanIssue struct {
	Location string `json:"location"`
	Message  string `json:"message"`
}

// ScanState is safe to poll while a scan runs.
type ScanState struct {
	Running         bool        `json:"running"`
	Cancellable     bool        `json:"cancellable"`
	Cancelled       bool        `json:"cancelled"`
	StartedAt       string      `json:"started_at,omitempty"`
	CompletedAt     string      `json:"completed_at,omitempty"`
	CurrentLocation string      `json:"current_location,omitempty"`
	FilesVisited    int         `json:"files_visited"`
	VideosFound     int         `json:"videos_found"`
	Summary         ScanSummary `json:"summary"`
	Error           string      `json:"error,omitempty"`
}

type discoveredVideo struct {
	ID                    string
	LocationPath          string
	RelativePath          string
	DisplayName           string
	SizeBytes             int64
	ModifiedAt            string
	FunscriptRelativePath *string
}

type rootScan struct {
	root     string
	videos   []discoveredVideo
	visited  int
	skipped  int
	complete bool
}

type videoCandidate struct {
	relative string
	name     string
	size     int64
	modified string
}

type rootDiscovery struct {
	ctx          context.Context
	result       rootScan
	videos       []videoCandidate
	funScripts   map[string]string
	lastReported int
	progress     func(visited, found int)
}

func emptyScanState() ScanState {
	return ScanState{Summary: ScanSummary{Issues: []ScanIssue{}}}
}

// StartScan snapshots configured roots and starts one cancellable scan.
func (c *Catalog) StartScan(locations []string) (ScanState, error) {
	roots, err := normalizeRoots(locations)
	if err != nil {
		return ScanState{}, err
	}

	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if c.closed {
		return ScanState{}, ErrClosed
	}
	if c.maintenance || c.scanState.Running {
		return c.scanState, ErrScanBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.scanCancel = cancel
	c.scanState = ScanState{
		Running:     true,
		Cancellable: true,
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Summary:     ScanSummary{Locations: len(roots), Issues: []ScanIssue{}},
	}
	state := cloneScanState(c.scanState)
	c.scanWG.Add(1)
	go c.runScan(ctx, roots)
	return state, nil
}

// ScanState returns a race-safe progress snapshot.
func (c *Catalog) ScanState() ScanState {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	return cloneScanState(c.scanState)
}

// CancelScan requests cancellation. State remains running until the worker
// has stopped touching the filesystem and database.
func (c *Catalog) CancelScan() ScanState {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if c.scanCancel != nil {
		c.scanCancel()
	}
	return cloneScanState(c.scanState)
}

func (c *Catalog) runScan(ctx context.Context, roots []string) {
	defer c.scanWG.Done()
	summary := ScanSummary{Locations: len(roots), Issues: []ScanIssue{}}
	var runErr error
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			runErr = err
			break
		}
		c.updateScanProgress(root, 0, 0)
		result, err := discoverRoot(ctx, root, func(visited, found int) {
			c.updateScanProgress(root, visited, found)
		})
		summary.Skipped += result.skipped
		if err != nil {
			if errors.Is(err, context.Canceled) {
				runErr = err
				break
			}
			summary.Issues = append(summary.Issues, ScanIssue{Location: root, Message: err.Error()})
			continue
		}
		if !result.complete {
			summary.Issues = append(summary.Issues, ScanIssue{
				Location: root,
				Message:  "location was only partially scanned; existing catalog entries were preserved",
			})
		}
		delta, err := c.applyRootScan(ctx, result)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				runErr = err
				break
			}
			runErr = err
			break
		}
		mergeScanSummary(&summary, delta)
	}

	c.scanMu.Lock()
	c.scanState.Running = false
	c.scanState.Cancellable = false
	c.scanState.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	c.scanState.CurrentLocation = ""
	c.scanState.Summary = summary
	c.scanState.Cancelled = errors.Is(runErr, context.Canceled)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		c.scanState.Error = runErr.Error()
	}
	c.scanCancel = nil
	state := cloneScanState(c.scanState)
	c.scanMu.Unlock()

	level := slogLevelForScan(state)
	c.logger.Log(context.Background(), level, "media library scan finished",
		"cancelled", state.Cancelled,
		"locations", state.Summary.Locations,
		"added", state.Summary.Added,
		"updated", state.Summary.Updated,
		"missing", state.Summary.Missing,
		"removed", state.Summary.Removed,
		"skipped", state.Summary.Skipped,
		"issues", len(state.Summary.Issues),
		"error", state.Error,
	)
}

func slogLevelForScan(state ScanState) slog.Level {
	if state.Error != "" || len(state.Summary.Issues) > 0 {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func (c *Catalog) updateScanProgress(root string, visited, found int) {
	c.scanMu.Lock()
	c.scanState.CurrentLocation = root
	c.scanState.FilesVisited += visited
	c.scanState.VideosFound += found
	c.scanMu.Unlock()
}

func discoverRoot(ctx context.Context, root string, progress func(visited, found int)) (rootScan, error) {
	result := rootScan{root: root, complete: true}
	info, err := os.Lstat(root)
	if err != nil {
		return result, fmt.Errorf("location is unavailable: %w", err)
	}
	if !info.IsDir() {
		return result, errors.New("location is not a directory")
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return result, errors.New("symlink locations are not scanned")
	}

	discovery := rootDiscovery{
		ctx:        ctx,
		result:     result,
		videos:     make([]videoCandidate, 0),
		funScripts: make(map[string]string),
		progress:   progress,
	}
	err = filepath.WalkDir(root, discovery.visit)
	discovery.flushProgress()
	if err != nil && !errors.Is(err, errFileLimit) {
		return discovery.result, err
	}
	return discovery.catalogResult(), nil
}

func (d *rootDiscovery) visit(path string, entry fs.DirEntry, walkErr error) error {
	if err := d.ctx.Err(); err != nil {
		return err
	}
	if walkErr != nil {
		d.result.skipped++
		d.result.complete = false
		if entry != nil && entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	relative, err := filepath.Rel(d.result.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		d.result.skipped++
		d.result.complete = false
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if relative == "." {
		return nil
	}
	if entry.IsDir() {
		if strings.HasPrefix(entry.Name(), ".") || pathDepth(relative) > MaxScanDepth {
			d.result.skipped++
			return filepath.SkipDir
		}
		return nil
	}
	if entry.Type()&fs.ModeSymlink != 0 {
		d.result.skipped++
		return nil
	}

	if d.result.visited >= MaxFilesPerLocation {
		d.result.complete = false
		d.result.skipped++
		return errFileLimit
	}
	d.result.visited++
	extension := strings.ToLower(filepath.Ext(entry.Name()))
	relative = filepath.ToSlash(relative)
	if extension == ".funscript" {
		if entry.Type().IsRegular() {
			d.funScripts[pairKey(relative)] = relative
		} else {
			d.result.skipped++
			d.result.complete = false
		}
		d.reportProgress(0)
		return nil
	}
	if !supportedVideoExtension(extension) {
		d.reportProgress(0)
		return nil
	}
	fileInfo, err := entry.Info()
	if err != nil || !fileInfo.Mode().IsRegular() {
		d.result.skipped++
		d.result.complete = false
		return nil
	}
	d.videos = append(d.videos, videoCandidate{
		relative: relative,
		name:     strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
		size:     fileInfo.Size(),
		modified: fileInfo.ModTime().UTC().Format(time.RFC3339Nano),
	})
	d.reportProgress(1)
	return nil
}

func (d *rootDiscovery) reportProgress(found int) {
	visited := d.result.visited - d.lastReported
	if found == 0 && visited < 100 {
		return
	}
	d.progress(visited, found)
	d.lastReported = d.result.visited
}

func (d *rootDiscovery) flushProgress() {
	if d.result.visited > d.lastReported {
		d.progress(d.result.visited-d.lastReported, 0)
		d.lastReported = d.result.visited
	}
}

func (d *rootDiscovery) catalogResult() rootScan {
	sort.Slice(d.videos, func(i, j int) bool { return d.videos[i].relative < d.videos[j].relative })
	d.result.videos = make([]discoveredVideo, 0, len(d.videos))
	for _, video := range d.videos {
		var pair *string
		if relative, ok := d.funScripts[pairKey(video.relative)]; ok {
			value := relative
			pair = &value
		}
		d.result.videos = append(d.result.videos, discoveredVideo{
			ID:                    stableVideoID(d.result.root, video.relative),
			LocationPath:          d.result.root,
			RelativePath:          video.relative,
			DisplayName:           video.name,
			SizeBytes:             video.size,
			ModifiedAt:            video.modified,
			FunscriptRelativePath: pair,
		})
	}
	return d.result
}

func (c *Catalog) applyRootScan(ctx context.Context, result rootScan) (ScanSummary, error) {
	delta := ScanSummary{Issues: []ScanIssue{}}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := c.db.WithTx(ctx, func(tx *sql.Tx) error {
		existing, err := videosForRoot(ctx, tx, result.root)
		if err != nil {
			return err
		}
		found := make(map[string]struct{}, len(result.videos))
		for _, video := range result.videos {
			found[video.ID] = struct{}{}
			current, exists := existing[video.ID]
			if !exists {
				delta.Added++
			} else if videoChanged(current, video) {
				delta.Updated++
			}
			if err := upsertVideo(ctx, tx, video, now); err != nil {
				return err
			}
		}
		if !result.complete {
			return nil
		}
		for id, current := range existing {
			if _, ok := found[id]; ok {
				continue
			}
			if current.Missing {
				if _, err := tx.ExecContext(ctx, `DELETE FROM media_videos WHERE id = ?`, id); err != nil {
					return err
				}
				delta.Removed++
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE media_videos SET missing = 1, scanned_at = ? WHERE id = ?`, now, id); err != nil {
				return err
			}
			delta.Missing++
		}
		return nil
	})
	return delta, err
}

func videosForRoot(ctx context.Context, tx *sql.Tx, root string) (map[string]Video, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, location_path, relative_path, display_name, size_bytes,
		       modified_at, duration_ms, funscript_relative_path, missing, scanned_at
		FROM media_videos WHERE location_path = ?
	`, root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]Video)
	for rows.Next() {
		video, scanErr := scanVideo(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result[video.ID] = video
	}
	return result, rows.Err()
}

func upsertVideo(ctx context.Context, tx *sql.Tx, video discoveredVideo, scannedAt string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO media_videos(
			id, location_path, relative_path, display_name, size_bytes, modified_at,
			duration_ms, funscript_relative_path, missing, scanned_at
		) VALUES(?, ?, ?, ?, ?, ?, NULL, ?, 0, ?)
		ON CONFLICT(id) DO UPDATE SET
			location_path = excluded.location_path,
			relative_path = excluded.relative_path,
			display_name = excluded.display_name,
			duration_ms = CASE
				WHEN media_videos.size_bytes = excluded.size_bytes AND media_videos.modified_at = excluded.modified_at
				THEN media_videos.duration_ms ELSE NULL END,
			size_bytes = excluded.size_bytes,
			modified_at = excluded.modified_at,
			funscript_relative_path = excluded.funscript_relative_path,
			missing = 0,
			scanned_at = excluded.scanned_at
	`, video.ID, video.LocationPath, video.RelativePath, video.DisplayName, video.SizeBytes,
		video.ModifiedAt, nullableString(video.FunscriptRelativePath), scannedAt)
	return err
}

func videoChanged(current Video, next discoveredVideo) bool {
	return current.Missing || current.DisplayName != next.DisplayName || current.SizeBytes != next.SizeBytes ||
		current.ModifiedAt != next.ModifiedAt || !sameOptionalString(current.FunscriptRelativePath, next.FunscriptRelativePath)
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func normalizeRoots(locations []string) ([]string, error) {
	roots := make([]string, 0, len(locations))
	seen := make(map[string]struct{}, len(locations))
	for _, value := range locations {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, fmt.Errorf("resolve media location: %w", err)
		}
		absolute = filepath.Clean(absolute)
		key := pathKey(absolute)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		roots = append(roots, absolute)
	}
	if len(roots) == 0 {
		return nil, ErrNoLocations
	}
	return roots, nil
}

func pathKey(value string) string {
	value = filepath.Clean(value)
	if runtime.GOOS == "windows" {
		return strings.ToLower(value)
	}
	return value
}

func pathDepth(relative string) int {
	clean := filepath.Clean(relative)
	if clean == "." || clean == "" {
		return 0
	}
	return strings.Count(clean, string(filepath.Separator)) + 1
}

func supportedVideoExtension(extension string) bool {
	switch extension {
	case ".mp4", ".m4v", ".webm", ".mov":
		return true
	default:
		return false
	}
}

func pairKey(relative string) string {
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)))
	base := strings.TrimSuffix(filepath.Base(relative), filepath.Ext(relative))
	key := directory + "\x00" + base
	if runtime.GOOS == "windows" {
		return strings.ToLower(key)
	}
	return key
}

func stableVideoID(root, relative string) string {
	relative = filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if runtime.GOOS == "windows" {
		relative = strings.ToLower(relative)
	}
	digest := sha256.Sum256([]byte(pathKey(root) + "\x00" + relative))
	return hex.EncodeToString(digest[:])
}

func mergeScanSummary(target *ScanSummary, delta ScanSummary) {
	target.Added += delta.Added
	target.Updated += delta.Updated
	target.Missing += delta.Missing
	target.Removed += delta.Removed
	target.Skipped += delta.Skipped
	target.Issues = append(target.Issues, delta.Issues...)
}

func cloneScanState(state ScanState) ScanState {
	state.Summary.Issues = append([]ScanIssue{}, state.Summary.Issues...)
	return state
}
