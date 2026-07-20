package media

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExplicitScanPairsVideosAndTracksMissingLifecycle(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Session.mp4"), "video-one")
	writeTestFile(t, filepath.Join(root, "Session.funscript"), `{"actions":[]}`)
	writeTestFile(t, filepath.Join(root, "ignored.mkv"), "not-browser-safe")
	writeTestFile(t, filepath.Join(root, ".hidden", "private.mp4"), "hidden")
	deep := root
	for index := 0; index <= MaxScanDepth; index++ {
		deep = filepath.Join(deep, "nested")
	}
	writeTestFile(t, filepath.Join(deep, "too-deep.mp4"), "deep")

	first := runTestScan(t, catalog, root)
	if first.Summary.Added != 1 || first.Summary.Skipped < 2 {
		t.Fatalf("first summary = %+v", first.Summary)
	}
	videos := listTestVideos(t, catalog)
	if len(videos) != 1 || videos[0].DisplayName != "Session" || !videos[0].HasFunscript {
		t.Fatalf("videos = %+v", videos)
	}
	if err := catalog.SetDuration(t.Context(), videos[0].ID, 12_345); err != nil {
		t.Fatalf("SetDuration: %v", err)
	}

	unchanged := runTestScan(t, catalog, root)
	if unchanged.Summary.Added != 0 || unchanged.Summary.Updated != 0 {
		t.Fatalf("unchanged summary = %+v", unchanged.Summary)
	}
	videos = listTestVideos(t, catalog)
	if videos[0].DurationMillis == nil || *videos[0].DurationMillis != 12_345 {
		t.Fatalf("duration after unchanged scan = %v", videos[0].DurationMillis)
	}

	time.Sleep(2 * time.Millisecond)
	writeTestFile(t, filepath.Join(root, "Session.mp4"), "video-one-modified")
	changed := runTestScan(t, catalog, root)
	if changed.Summary.Updated != 1 {
		t.Fatalf("changed summary = %+v", changed.Summary)
	}
	videos = listTestVideos(t, catalog)
	if videos[0].DurationMillis != nil {
		t.Fatalf("changed file retained stale duration %d", *videos[0].DurationMillis)
	}

	if err := os.Remove(filepath.Join(root, "Session.mp4")); err != nil {
		t.Fatalf("remove video: %v", err)
	}
	missing := runTestScan(t, catalog, root)
	if missing.Summary.Missing != 1 || len(listTestVideos(t, catalog)) != 1 || !listTestVideos(t, catalog)[0].Missing {
		t.Fatalf("missing lifecycle summary=%+v videos=%+v", missing.Summary, listTestVideos(t, catalog))
	}
	removed := runTestScan(t, catalog, root)
	if removed.Summary.Removed != 1 || len(listTestVideos(t, catalog)) != 0 {
		t.Fatalf("removal lifecycle summary=%+v videos=%+v", removed.Summary, listTestVideos(t, catalog))
	}
}

func TestPartialRootScanNeverMarksUnseenRowsMissing(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "kept.mp4"), "video")
	runTestScan(t, catalog, root)
	before := listTestVideos(t, catalog)

	delta, err := catalog.applyRootScan(t.Context(), rootScan{root: root, complete: false})
	if err != nil {
		t.Fatalf("apply partial root: %v", err)
	}
	after := listTestVideos(t, catalog)
	if delta.Missing != 0 || len(after) != 1 || after[0].Missing || after[0].ID != before[0].ID {
		t.Fatalf("partial scan changed catalog: delta=%+v before=%+v after=%+v", delta, before, after)
	}
}

func TestDiscoveryStopsAtFileLimitWithoutExceedingReportedBound(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "overflow.mp4")
	writeTestFile(t, path, "video")
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ReadDir: entries=%d err=%v", len(entries), err)
	}
	discovery := rootDiscovery{
		ctx: context.Background(),
		result: rootScan{
			root:     root,
			visited:  MaxFilesPerLocation,
			complete: true,
		},
		funScripts: map[string]string{},
		progress:   func(_, _ int) {},
	}

	err = discovery.visit(path, entries[0], nil)
	if !errors.Is(err, errFileLimit) {
		t.Fatalf("visit error = %v, want %v", err, errFileLimit)
	}
	if discovery.result.visited != MaxFilesPerLocation || discovery.result.complete || discovery.result.skipped != 1 {
		t.Fatalf("bounded discovery result = %+v", discovery.result)
	}
}

func TestOpenVideoRejectsCatalogTraversalAndSymlinkEscape(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	writeTestFile(t, outside, "outside")

	insertDiscoveredVideo(t, catalog, discoveredVideo{
		ID: "traversal", LocationPath: root, RelativePath: "../outside.mp4",
		DisplayName: "outside", SizeBytes: 7, ModifiedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if _, _, err := catalog.OpenVideo(t.Context(), "traversal"); !errors.Is(err, ErrVideoUnavailable) {
		t.Fatalf("traversal error = %v, want %v", err, ErrVideoUnavailable)
	}

	link := filepath.Join(root, "linked.mp4")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	insertDiscoveredVideo(t, catalog, discoveredVideo{
		ID: "symlink", LocationPath: root, RelativePath: "linked.mp4",
		DisplayName: "linked", SizeBytes: 7, ModifiedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if _, _, err := catalog.OpenVideo(t.Context(), "symlink"); !errors.Is(err, ErrVideoUnavailable) {
		t.Fatalf("symlink escape error = %v, want %v", err, ErrVideoUnavailable)
	}

	linkedDirectory := filepath.Join(root, "linked-directory")
	if err := os.Symlink(filepath.Dir(outside), linkedDirectory); err != nil {
		t.Skipf("directory symlink unavailable on this host: %v", err)
	}
	insertDiscoveredVideo(t, catalog, discoveredVideo{
		ID: "directory-symlink", LocationPath: root, RelativePath: "linked-directory/outside.mp4",
		DisplayName: "outside", SizeBytes: 7, ModifiedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if _, _, err := catalog.OpenVideo(t.Context(), "directory-symlink"); !errors.Is(err, ErrVideoUnavailable) {
		t.Fatalf("directory symlink escape error = %v, want %v", err, ErrVideoUnavailable)
	}
}

func TestRetainLocationsDeletesOnlyRemovedRoots(t *testing.T) {
	catalog := openTestCatalog(t)
	first := t.TempDir()
	second := t.TempDir()
	writeTestFile(t, filepath.Join(first, "first.mp4"), "first")
	writeTestFile(t, filepath.Join(second, "second.mp4"), "second")
	runTestScan(t, catalog, first, second)

	removed, err := catalog.RetainLocations(t.Context(), []string{second})
	if err != nil {
		t.Fatalf("RetainLocations: %v", err)
	}
	videos := listTestVideos(t, catalog)
	if removed != 1 || len(videos) != 1 || videos[0].LocationPath != second {
		t.Fatalf("removed=%d videos=%+v", removed, videos)
	}
}

func TestCatalogCloseWaitsForCatalogOperation(t *testing.T) {
	catalog := openTestCatalog(t)
	catalog.operationMu.Lock()

	closed := make(chan error, 1)
	go func() { closed <- catalog.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before maintenance drained: %v", err)
	default:
	}

	catalog.operationMu.Unlock()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after maintenance drained")
	}
}

func TestEmptyScanStatePublishesAnEmptyIssueList(t *testing.T) {
	state := cloneScanState(emptyScanState())
	if state.Summary.Issues == nil || len(state.Summary.Issues) != 0 {
		t.Fatalf("empty scan issues = %#v, want non-nil empty slice", state.Summary.Issues)
	}
}

func openTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := Open(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = catalog.Close() })
	return catalog
}

func runTestScan(t *testing.T, catalog *Catalog, roots ...string) ScanState {
	t.Helper()
	if _, err := catalog.StartScan(roots); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := catalog.ScanState()
		if !state.Running {
			if state.Error != "" {
				t.Fatalf("scan failed: %+v", state)
			}
			return state
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scan did not finish")
	return ScanState{}
}

func listTestVideos(t *testing.T, catalog *Catalog) []Video {
	t.Helper()
	videos, err := catalog.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return videos
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func insertDiscoveredVideo(t *testing.T, catalog *Catalog, video discoveredVideo) {
	t.Helper()
	if _, err := catalog.applyRootScan(context.Background(), rootScan{
		root: video.LocationPath, videos: []discoveredVideo{video}, complete: false,
	}); err != nil {
		t.Fatalf("insert catalog row: %v", err)
	}
}
