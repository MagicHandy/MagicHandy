package media

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func stringPointer(value string) *string { return &value }
func intPointer(value int) *int          { return &value }

func curatedLibrary(t *testing.T, names ...string) (*Catalog, map[string]string) {
	t.Helper()
	catalog := openTestCatalog(t)
	root := t.TempDir()
	for _, name := range names {
		writeTestFile(t, filepath.Join(root, name+".mp4"), "video "+name)
	}
	runTestScan(t, catalog, root)
	ids := make(map[string]string)
	for _, video := range listTestVideos(t, catalog) {
		ids[video.DisplayName] = video.ID
	}
	if len(ids) != len(names) {
		t.Fatalf("scanned %d videos, want %d", len(ids), len(names))
	}
	return catalog, ids
}

func TestMetadataNormalizationBounds(t *testing.T) {
	if title, err := NormalizeTitle("  Evening take  "); err != nil || title == nil || *title != "Evening take" {
		t.Fatalf("NormalizeTitle = %v, %v", title, err)
	}
	if title, err := NormalizeTitle("   "); err != nil || title != nil {
		t.Fatalf("a blank title must clear, got %v, %v", title, err)
	}
	for _, invalid := range []string{strings.Repeat("a", 201), "bell\a"} {
		if _, err := NormalizeTitle(invalid); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("NormalizeTitle(%q) err = %v, want invalid metadata", invalid, err)
		}
	}
	if notes, err := NormalizeNotes("line one\nline two"); err != nil || notes == nil || !strings.Contains(*notes, "\n") {
		t.Fatalf("notes lost their line break: %v, %v", notes, err)
	}
	for _, rating := range []int{-1, 6} {
		if _, err := NormalizeRating(rating); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("rating %d was accepted", rating)
		}
	}
	if rating, err := NormalizeRating(0); err != nil || rating != nil {
		t.Fatalf("rating 0 must clear, got %v, %v", rating, err)
	}
	tags, err := NormalizeTags([]string{"  slow   build ", "Calm", "calm", "Alpha"}, MaxVideoTags)
	if err != nil {
		t.Fatalf("NormalizeTags: %v", err)
	}
	if want := []string{"Alpha", "Calm", "slow build"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %q, want %q", tags, want)
	}
	for _, invalid := range [][]string{{"a,b"}, {""}, {strings.Repeat("x", 41)}} {
		if _, err := NormalizeTags(invalid, MaxVideoTags); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("NormalizeTags(%q) err = %v", invalid, err)
		}
	}
	many := make([]string, MaxVideoTags+1)
	for index := range many {
		many[index] = fmt.Sprintf("tag %02d", index)
	}
	if _, err := NormalizeTags(many, MaxVideoTags); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("%d tags were accepted", len(many))
	}
}

func TestUpdateMetadataPatchesAndClearsCuration(t *testing.T) {
	catalog, ids := curatedLibrary(t, "clip")
	id := ids["clip"]
	tags := []string{"calm", "Long"}
	video, err := catalog.UpdateMetadata(t.Context(), id, MetadataPatch{
		Title: stringPointer("Evening take"), Rating: intPointer(4), Notes: stringPointer("Good first half."), Tags: &tags,
	})
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	if video.Title == nil || *video.Title != "Evening take" || video.Rating == nil || *video.Rating != 4 ||
		video.Notes == nil || *video.Notes != "Good first half." || !reflect.DeepEqual(video.Tags, []string{"calm", "Long"}) {
		t.Fatalf("updated video = %+v", video)
	}
	if video.DisplayName != "clip" {
		t.Fatalf("a title changed the display name to %q", video.DisplayName)
	}

	// Only the fields present change; empty values clear.
	cleared, err := catalog.UpdateMetadata(t.Context(), id, MetadataPatch{Title: stringPointer(""), Rating: intPointer(0)})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.Title != nil || cleared.Rating != nil || cleared.Notes == nil || len(cleared.Tags) != 2 {
		t.Fatalf("cleared video = %+v", cleared)
	}

	if _, err := catalog.UpdateMetadata(t.Context(), "unknown", MetadataPatch{Rating: intPointer(3)}); !errors.Is(err, ErrVideoNotFound) {
		t.Fatalf("unknown video err = %v", err)
	}
	if _, err := catalog.UpdateMetadata(t.Context(), id, MetadataPatch{Rating: intPointer(9)}); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("invalid rating err = %v", err)
	}
}

func TestCurationSurvivesRescanAndListsTags(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Session.mp4"), "video-one")
	runTestScan(t, catalog, root)
	id := listTestVideos(t, catalog)[0].ID
	tags := []string{"zeta", "Alpha"}
	if _, err := catalog.UpdateMetadata(t.Context(), id, MetadataPatch{Title: stringPointer("Kept"), Tags: &tags}); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}

	runTestScan(t, catalog, root)
	videos := listTestVideos(t, catalog)
	if len(videos) != 1 || videos[0].Title == nil || *videos[0].Title != "Kept" {
		t.Fatalf("curation after rescan = %+v", videos)
	}
	if !reflect.DeepEqual(videos[0].Tags, []string{"Alpha", "zeta"}) {
		t.Fatalf("listed tags = %q, want sorted case-insensitively", videos[0].Tags)
	}
}

func TestBulkTagsKeepOneSpellingAndRollBackAsAUnit(t *testing.T) {
	catalog, ids := curatedLibrary(t, "a", "b")
	first := []string{"Calm"}
	if _, err := catalog.UpdateMetadata(t.Context(), ids["a"], MetadataPatch{Tags: &first}); err != nil {
		t.Fatal(err)
	}
	updated, err := catalog.UpdateTags(t.Context(), []string{ids["a"], ids["b"], ids["b"]}, []string{"calm", "Build"}, nil)
	if err != nil {
		t.Fatalf("UpdateTags: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("updated %d videos, want 2", len(updated))
	}
	for _, video := range updated {
		if !reflect.DeepEqual(video.Tags, []string{"Build", "Calm"}) {
			t.Fatalf("%s tags = %q, want the library spelling of calm", video.DisplayName, video.Tags)
		}
	}

	if _, err := catalog.UpdateTags(t.Context(), []string{ids["a"], "unknown"}, []string{"Late"}, nil); !errors.Is(err, ErrVideoNotFound) {
		t.Fatalf("unknown video err = %v", err)
	}
	video, err := catalog.Video(t.Context(), ids["a"])
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range video.Tags {
		if tag == "Late" {
			t.Fatal("a failed bulk edit left a partial change")
		}
	}

	removed, err := catalog.UpdateTags(t.Context(), []string{ids["b"]}, nil, []string{"BUILD"})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !reflect.DeepEqual(removed[0].Tags, []string{"Calm"}) {
		t.Fatalf("tags after removal = %q", removed[0].Tags)
	}
	if _, err := catalog.UpdateTags(t.Context(), []string{ids["b"]}, nil, nil); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("an empty edit err = %v", err)
	}
}

func TestRenameMergeAndDeleteTags(t *testing.T) {
	catalog, ids := curatedLibrary(t, "a", "b", "c")
	for name, tags := range map[string][]string{"a": {"slow"}, "b": {"slow", "calm"}, "c": {"calm"}} {
		tags := tags
		if _, err := catalog.UpdateMetadata(t.Context(), ids[name], MetadataPatch{Tags: &tags}); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := catalog.Tags(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if want := []TagCount{{Tag: "calm", Count: 2}, {Tag: "slow", Count: 2}}; !reflect.DeepEqual(counts, want) {
		t.Fatalf("tags = %+v, want %+v", counts, want)
	}

	// Renaming onto an existing tag merges: b keeps one "Calm".
	renamed, err := catalog.RenameTag(t.Context(), "slow", "Calm")
	if err != nil || renamed != 2 {
		t.Fatalf("RenameTag = %d, %v", renamed, err)
	}
	counts, _ = catalog.Tags(t.Context())
	if want := []TagCount{{Tag: "Calm", Count: 3}}; !reflect.DeepEqual(counts, want) {
		t.Fatalf("tags after merge = %+v, want %+v", counts, want)
	}
	// A case-only rename changes the spelling everywhere.
	if _, err := catalog.RenameTag(t.Context(), "calm", "calm"); err != nil {
		t.Fatal(err)
	}
	counts, _ = catalog.Tags(t.Context())
	if len(counts) != 1 || counts[0].Tag != "calm" {
		t.Fatalf("spelling after case rename = %+v", counts)
	}

	deleted, err := catalog.DeleteTag(t.Context(), "CALM")
	if err != nil || deleted != 3 {
		t.Fatalf("DeleteTag = %d, %v", deleted, err)
	}
	for _, video := range listTestVideos(t, catalog) {
		if len(video.Tags) != 0 {
			t.Fatalf("%s kept tags %q", video.DisplayName, video.Tags)
		}
	}
}

func TestConvertedCopyCarriesCuration(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Clip.mkv"), "video")
	writeTestFile(t, filepath.Join(root, "Clip_MHConverted.mp4"), "converted video")
	runTestScan(t, catalog, root)
	var source Video
	for _, video := range listTestVideos(t, catalog) {
		if video.DisplayName == "Clip" {
			source = video
		}
	}
	tags := []string{"favorite"}
	if _, err := catalog.UpdateMetadata(t.Context(), source.ID, MetadataPatch{
		Title: stringPointer("Holiday"), Rating: intPointer(5), Notes: stringPointer("Keep"), Tags: &tags,
	}); err != nil {
		t.Fatal(err)
	}
	source, err := catalog.Video(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.adoptConvertedFile(t.Context(), source, StreamInfo{VideoCodec: "h264", HasVideo: true}); err != nil {
		t.Fatalf("adoptConvertedFile: %v", err)
	}
	for _, video := range listTestVideos(t, catalog) {
		if video.DisplayName != "Clip_MHConverted" {
			continue
		}
		if video.Title == nil || *video.Title != "Holiday" || video.Rating == nil || *video.Rating != 5 ||
			video.Notes == nil || *video.Notes != "Keep" || !reflect.DeepEqual(video.Tags, []string{"favorite"}) {
			t.Fatalf("converted copy curation = %+v", video)
		}
		return
	}
	t.Fatal("converted row was not cataloged")
}
