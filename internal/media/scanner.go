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

// ScanTrigger identifies why a catalog scan started.
type ScanTrigger string

const (
	// ScanTriggerManual is a controller-requested scan.
	ScanTriggerManual ScanTrigger = "manual"
	// ScanTriggerStartup is an opted-in background scan during core startup.
	ScanTriggerStartup ScanTrigger = "startup"
)

// ScanOptions are snapshotted when a scan starts.
type ScanOptions struct {
	// RemoveMissing deletes rows absent from a completely enumerated root.
	// Partial and unavailable roots are preserved regardless of this value.
	RemoveMissing bool
	Trigger       ScanTrigger
}

// DefaultScanOptions returns the product defaults for direct callers.
func DefaultScanOptions() ScanOptions {
	return ScanOptions{RemoveMissing: true, Trigger: ScanTriggerManual}
}

// ScanSummary is the durable-catalog delta from one scan.
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
	Trigger         ScanTrigger `json:"trigger,omitempty"`
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
	Compatibility         Compatibility
	Superseded            bool
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
	return c.StartScanWithOptions(locations, DefaultScanOptions())
}

// StartScanWithOptions starts a scan with an explicit missing-file policy.
func (c *Catalog) StartScanWithOptions(locations []string, options ScanOptions) (ScanState, error) {
	return c.StartScanThenWithOptions(locations, options, nil)
}

// StartScanThen starts a scan and runs after() once it has finished, which is
// how the opt-in generate-and-convert options ride a scan.
// The callback receives the final state so it can decline: a cancelled or
// failed scan should not silently launch an hour of encoding.
func (c *Catalog) StartScanThen(locations []string, after func(ScanState)) (ScanState, error) {
	return c.StartScanThenWithOptions(locations, DefaultScanOptions(), after)
}

// StartScanThenWithOptions starts a scan with explicit policy and follow-up.
func (c *Catalog) StartScanThenWithOptions(
	locations []string,
	options ScanOptions,
	after func(ScanState),
) (ScanState, error) {
	roots, err := normalizeRoots(locations)
	if err != nil {
		return ScanState{}, err
	}
	if options.Trigger != ScanTriggerStartup {
		options.Trigger = ScanTriggerManual
	}

	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if c.closed.Load() {
		return ScanState{}, ErrClosed
	}
	if c.maintenance || c.scanState.Running {
		return c.scanState, ErrScanBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.scanCancel = cancel
	c.scanState = ScanState{
		Running:     true,
		Trigger:     options.Trigger,
		Cancellable: true,
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Summary:     ScanSummary{Locations: len(roots), Issues: []ScanIssue{}},
	}
	state := cloneScanState(c.scanState)
	c.scanWG.Add(1)
	go c.runScan(ctx, roots, options, after)
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

func (c *Catalog) runScan(ctx context.Context, roots []string, options ScanOptions, after func(ScanState)) {
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
		delta, err := c.applyRootScan(ctx, result, options)
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

	// A cancelled or failed scan does not trigger follow-up work. Its catalog
	// is incomplete, and launching an hour of encoding from incomplete input
	// would be surprising for both manual and startup scans.
	if after != nil && !state.Cancelled && state.Error == "" {
		after(state)
	}

	level := slogLevelForScan(state)
	c.logger.Log(context.Background(), level, "media library scan finished",
		"trigger", state.Trigger,
		"remove_missing", options.RemoveMissing,
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
	if !CatalogedExtension(extension) {
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
	superseded := supersededKeys(d.videos)
	d.result.videos = make([]discoveredVideo, 0, len(d.videos))
	for _, video := range d.videos {
		d.result.videos = append(d.result.videos, discoveredVideo{
			ID:                    stableVideoID(d.result.root, video.relative),
			LocationPath:          d.result.root,
			RelativePath:          video.relative,
			DisplayName:           video.name,
			SizeBytes:             video.size,
			ModifiedAt:            video.modified,
			FunscriptRelativePath: d.resolveScript(video.relative),
			Compatibility:         ContainerCompatibility(video.relative),
			Superseded:            containsKey(superseded, pairKey(video.relative)),
		})
	}
	return d.result
}

// resolveScript pairs by exact basename first, then retries without the
// reserved conversion suffix so a converted file keeps the script its source
// was paired with.
func (d *rootDiscovery) resolveScript(relative string) *string {
	if script, ok := d.funScripts[pairKey(relative)]; ok {
		return &script
	}
	if !HasConvertedSuffix(relative) {
		return nil
	}
	if script, ok := d.funScripts[convertedPairKey(relative)]; ok {
		return &script
	}
	return nil
}

// supersededKeys derives which rows a converted file hides. Derived at scan
// time rather than stored as a flag, because the filesystem is the source of
// truth and people move and delete files outside the app: delete the converted
// copy and the original reappears on the next scan with no stale flag to clean
// up. The row survives either way, so its per-video sync offset does too.
func supersededKeys(videos []videoCandidate) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, video := range videos {
		if !HasConvertedSuffix(video.relative) {
			continue
		}
		keys[convertedPairKey(video.relative)] = struct{}{}
	}
	return keys
}

func containsKey(keys map[string]struct{}, key string) bool {
	_, ok := keys[key]
	return ok
}

func (c *Catalog) applyRootScan(ctx context.Context, result rootScan, options ScanOptions) (ScanSummary, error) {
	delta := ScanSummary{Issues: []ScanIssue{}}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var dropped []string
	err := c.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Reset per attempt: WithTx may retry, and a partial list from an
		// aborted attempt would delete covers for rows that still exist.
		dropped = nil
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
			if options.RemoveMissing {
				if _, err := tx.ExecContext(ctx, `DELETE FROM media_videos WHERE id = ?`, id); err != nil {
					return err
				}
				dropped = append(dropped, id)
				delta.Removed++
				continue
			}
			if current.Missing {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE media_videos SET missing = 1, scanned_at = ? WHERE id = ?`, now, id); err != nil {
				return err
			}
			delta.Missing++
		}
		return nil
	})
	if err == nil {
		// After the commit, never before: a rolled-back transaction would
		// otherwise leave the row intact and its cover gone.
		for _, id := range dropped {
			c.deleteThumbnail(id)
		}
	}
	return delta, err
}

func videosForRoot(ctx context.Context, tx *sql.Tx, root string) (map[string]Video, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT `+videoColumns+`
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

// upsertVideo writes one discovered row.
//
// The learned facts — compatibility, probed codecs, the generated cover — are
// kept only while the file is byte-for-byte the one they were learned from.
// A changed size or timestamp means a different file behind the same name, and
// carrying "this plays" across that would be a stale claim in exactly the
// direction that hides a broken video. The same CASE already guards duration.
func upsertVideo(ctx context.Context, tx *sql.Tx, video discoveredVideo, scannedAt string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO media_videos(
			id, location_path, relative_path, display_name, size_bytes, modified_at,
			duration_ms, funscript_relative_path, missing, scanned_at, compatibility, superseded
		) VALUES(?, ?, ?, ?, ?, ?, NULL, ?, 0, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			location_path = excluded.location_path,
			relative_path = excluded.relative_path,
			display_name = excluded.display_name,
			duration_ms = CASE WHEN media_videos.size_bytes = excluded.size_bytes
				AND media_videos.modified_at = excluded.modified_at
				THEN media_videos.duration_ms ELSE NULL END,
			thumbnail_generated_at = CASE WHEN media_videos.size_bytes = excluded.size_bytes
				AND media_videos.modified_at = excluded.modified_at
				THEN media_videos.thumbnail_generated_at ELSE NULL END,
			video_codec = CASE WHEN media_videos.size_bytes = excluded.size_bytes
				AND media_videos.modified_at = excluded.modified_at
				THEN media_videos.video_codec ELSE NULL END,
			audio_codec = CASE WHEN media_videos.size_bytes = excluded.size_bytes
				AND media_videos.modified_at = excluded.modified_at
				THEN media_videos.audio_codec ELSE NULL END,
			compatibility = CASE WHEN media_videos.size_bytes = excluded.size_bytes
				AND media_videos.modified_at = excluded.modified_at
				THEN media_videos.compatibility ELSE excluded.compatibility END,
			size_bytes = excluded.size_bytes,
			modified_at = excluded.modified_at,
			funscript_relative_path = excluded.funscript_relative_path,
			superseded = excluded.superseded,
			missing = 0,
			scanned_at = excluded.scanned_at
	`, video.ID, video.LocationPath, video.RelativePath, video.DisplayName, video.SizeBytes,
		video.ModifiedAt, nullableString(video.FunscriptRelativePath), scannedAt,
		string(video.Compatibility), boolToInt(video.Superseded))
	return err
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
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

func pairKey(relative string) string {
	base := strings.TrimSuffix(filepath.Base(relative), filepath.Ext(relative))
	return directoryKey(relative, base)
}

// convertedPairKey is the key a converted file falls back to. Scripts pair by
// exact basename, so Holiday_MHConverted.mp4 would look for
// Holiday_MHConverted.funscript, find nothing, and silently cost the user the
// pairing that is the whole point of the library. Stripping the reserved suffix
// and trying again is the one fallback that fixes it, and it changes nothing on
// disk: no file is copied, renamed, or duplicated.
func convertedPairKey(relative string) string {
	return directoryKey(relative, stripConvertedSuffix(relative))
}

func directoryKey(relative, base string) string {
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)))
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
