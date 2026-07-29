package media

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// TestSupportedContainerIsNotAPlayabilityClaim pins the correction this whole
// feature rests on. An .mp4 is a container every browser opens, and an .mp4
// holding HEVC still does not play in Firefox. Classifying by extension may
// therefore report "unknown", never "playable" — otherwise the app would assert
// a file works on the strength of its filename and never offer to repair it.
func TestSupportedContainerIsNotAPlayabilityClaim(t *testing.T) {
	for _, path := range []string{"clip.mp4", "clip.m4v", "clip.webm", "clip.mov"} {
		if got := ContainerCompatibility(path); got != CompatibilityUnknown {
			t.Fatalf("ContainerCompatibility(%q) = %q, want %q", path, got, CompatibilityUnknown)
		}
		if got := ContainerCompatibility(path); got.NeedsConversion() {
			t.Fatalf("%q was treated as a repair candidate on extension alone", path)
		}
	}
	for _, path := range []string{"clip.mkv", "clip.avi", "clip.wmv", "clip.ts"} {
		if got := ContainerCompatibility(path); got != CompatibilityUnsupportedContainer {
			t.Fatalf("ContainerCompatibility(%q) = %q, want %q", path, got, CompatibilityUnsupportedContainer)
		}
	}
}

// TestIncompatibleContainersAreCataloged guards the prerequisite: a file the
// browser cannot open has to appear in the library, or there is nothing for a
// conversion button to attach to.
func TestIncompatibleContainersAreCataloged(t *testing.T) {
	for _, extension := range []string{".mp4", ".mkv", ".avi", ".wmv"} {
		if !CatalogedExtension(extension) {
			t.Fatalf("CatalogedExtension(%q) = false, want true", extension)
		}
	}
	for _, extension := range []string{".txt", ".jpg", ".funscript", ".exe"} {
		if CatalogedExtension(extension) {
			t.Fatalf("CatalogedExtension(%q) = true, want false", extension)
		}
	}
}

func TestOnlyBrokenFilesAreConversionCandidates(t *testing.T) {
	cases := map[Compatibility]bool{
		CompatibilityUnknown:              false,
		CompatibilityPlayable:             false,
		CompatibilityUnsupportedContainer: true,
		CompatibilityUnsupportedCodec:     true,
	}
	for state, want := range cases {
		if got := state.NeedsConversion(); got != want {
			t.Fatalf("%q.NeedsConversion() = %v, want %v", state, got, want)
		}
	}
}

func TestUnrecognizedCompatibilityIsNotTrusted(t *testing.T) {
	if Compatibility("definitely_fine").Valid() {
		t.Fatal("an unknown compatibility value was accepted")
	}
}

func TestConvertedNamingRoundTrips(t *testing.T) {
	if got := ConvertedName("shows/Holiday.mkv"); got != "Holiday_MHConverted.mp4" {
		t.Fatalf("ConvertedName = %q", got)
	}
	if !HasConvertedSuffix("Holiday_MHConverted.mp4") {
		t.Fatal("converted output was not recognized as app output")
	}
	if HasConvertedSuffix("Holiday.mkv") {
		t.Fatal("a source file was mistaken for app output")
	}
	// The suffix is stripped once, not repeatedly, so a user's own oddly named
	// file is not unwound into a name they never had.
	if got := stripConvertedSuffix("Clip_MHConverted_MHConverted.mp4"); got != "Clip_MHConverted" {
		t.Fatalf("stripConvertedSuffix = %q, want %q", got, "Clip_MHConverted")
	}
}

func TestConvertedFilePairsWithTheSourceScript(t *testing.T) {
	discovery := rootDiscovery{funScripts: map[string]string{
		pairKey("Holiday.funscript"): "Holiday.funscript",
	}}
	script := discovery.resolveScript("Holiday_MHConverted.mp4")
	if script == nil || *script != "Holiday.funscript" {
		t.Fatalf("converted file lost its script pairing: %v", script)
	}
	// An exact-name script still wins when one exists.
	discovery.funScripts[pairKey("Holiday_MHConverted.funscript")] = "Holiday_MHConverted.funscript"
	script = discovery.resolveScript("Holiday_MHConverted.mp4")
	if script == nil || *script != "Holiday_MHConverted.funscript" {
		t.Fatalf("exact pairing did not win: %v", script)
	}
}

func TestConvertedFileSupersedesOnlyItsOwnSource(t *testing.T) {
	videos := []videoCandidate{
		{relative: "Holiday.mkv"},
		{relative: "Holiday_MHConverted.mp4"},
		{relative: "Holidays.mkv"},
		{relative: "other/Holiday.mkv"},
	}
	keys := supersededKeys(videos)
	if !containsKey(keys, pairKey("Holiday.mkv")) {
		t.Fatal("the converted file did not supersede its source")
	}
	if containsKey(keys, pairKey("Holiday_MHConverted.mp4")) {
		t.Fatal("the converted file superseded itself")
	}
	if containsKey(keys, pairKey("Holidays.mkv")) {
		t.Fatal("a prefix match was superseded")
	}
	// Superseding is scoped to a directory: a same-named file elsewhere is a
	// different file and must stay visible.
	if containsKey(keys, pairKey("other/Holiday.mkv")) {
		t.Fatal("a file in another directory was superseded")
	}
}

func TestRemuxIsTriedBeforeReencoding(t *testing.T) {
	settings := defaultMediaSettings(t)
	cases := []struct {
		name       string
		info       StreamInfo
		wantAction ConversionAction
		lossless   bool
	}{
		{"h264 and aac copy across", StreamInfo{VideoCodec: "h264", AudioCodec: "aac"}, ActionRemux, true},
		{"silent video still remuxes", StreamInfo{VideoCodec: "h264"}, ActionRemux, true},
		{"hevc is assumed playable", StreamInfo{VideoCodec: "hevc", AudioCodec: "aac"}, ActionRemux, true},
		{"ac3 audio alone is re-encoded", StreamInfo{VideoCodec: "h264", AudioCodec: "ac3"}, ActionEncodeAudio, false},
		{"mpeg4 video needs a real encode", StreamInfo{VideoCodec: "mpeg4", AudioCodec: "aac"}, ActionEncodeVideo, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			plan := PlanConversion(testCase.info, settings)
			if plan.Action != testCase.wantAction {
				t.Fatalf("action = %q, want %q", plan.Action, testCase.wantAction)
			}
			if plan.Lossless != testCase.lossless {
				t.Fatalf("lossless = %v, want %v", plan.Lossless, testCase.lossless)
			}
		})
	}
}

// TestH265CompatibilityToggleCannotProduceH265 pins the interlock. Someone who
// has just declared that HEVC does not play here must never get an HEVC file
// back: that is an hour of encoding spent arriving at another unplayable file.
func TestH265CompatibilityToggleCannotProduceH265(t *testing.T) {
	settings := defaultMediaSettings(t)
	settings.ReencodeCodec = config.ReencodeCodecH265
	settings.ConvertH265ForCompatibility = true

	plan := PlanConversion(StreamInfo{VideoCodec: "hevc", AudioCodec: "aac"}, settings)
	if plan.Action != ActionEncodeVideo {
		t.Fatalf("action = %q, want a re-encode once HEVC is declared unplayable", plan.Action)
	}
	if plan.TargetCodec != config.ReencodeCodecH264 {
		t.Fatalf("target codec = %q, want %q", plan.TargetCodec, config.ReencodeCodecH264)
	}
	args := strings.Join(plan.ffmpegArgs("in.mkv", "out.mp4", settings), " ")
	if strings.Contains(args, "libx265") {
		t.Fatalf("re-encode targeted HEVC after it was declared unplayable: %s", args)
	}
	if !strings.Contains(args, "libx264") {
		t.Fatalf("re-encode did not target H.264: %s", args)
	}
}

// TestConversionArgsCarryFaststartAndDropSubtitles guards two things that are
// easy to lose and expensive to rediscover: faststart, without which a
// converted file feels broken over range requests, and the explicit stream maps
// that keep MKV subtitle tracks from failing the mux.
func TestConversionArgsCarryFaststartAndDropSubtitles(t *testing.T) {
	settings := defaultMediaSettings(t)
	plan := PlanConversion(StreamInfo{VideoCodec: "h264", AudioCodec: "aac"}, settings)
	args := plan.ffmpegArgs("in.mkv", "out.mp4", settings)
	joined := strings.Join(args, " ")

	// -f mp4 is load-bearing, not decoration: output goes to a ".partial" name
	// that FFmpeg cannot resolve a muxer from, so an inferred format fails the
	// whole conversion with an opaque "Invalid argument".
	for _, required := range []string{"-movflags +faststart", "-f mp4", "-map 0:v:0", "-map 0:a:0?", "-sn -dn", "-c copy"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing %q in %s", required, joined)
		}
	}
	// Never overwrite: the source is not recoverable if conversion clobbers it.
	if !strings.Contains(joined, "-n out.mp4") {
		t.Fatalf("conversion did not refuse to overwrite: %s", joined)
	}
	if strings.Contains(joined, "-y") {
		t.Fatalf("conversion enabled overwrite: %s", joined)
	}
}

func TestConvertedPathStaysInsideItsLocation(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "shows", "Holiday.mkv"), "video")

	target, err := catalog.convertedTargetPath(Video{
		LocationPath: root,
		RelativePath: "shows/Holiday.mkv",
	})
	if err != nil {
		t.Fatalf("convertedTargetPath: %v", err)
	}
	want := filepath.Join(root, "shows", "Holiday_MHConverted.mp4")
	if target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if _, err := catalog.convertedTargetPath(Video{
		LocationPath: root,
		RelativePath: "../escape.mkv",
	}); err == nil {
		t.Fatal("a traversing relative path produced a conversion target")
	}
}

func TestThumbnailRejectsNonJPEGPayloads(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Session.mp4"), "video")
	runTestScan(t, catalog, root)
	video := listTestVideos(t, catalog)[0]

	for _, payload := range [][]byte{nil, []byte("not an image"), {0x89, 0x50, 0x4E, 0x47}} {
		if err := catalog.SaveThumbnail(t.Context(), video.ID, payload); err == nil {
			t.Fatalf("SaveThumbnail accepted %v", payload)
		}
	}
	jpeg := append([]byte{0xFF, 0xD8, 0xFF}, []byte("body")...)
	if err := catalog.SaveThumbnail(t.Context(), video.ID, jpeg); err != nil {
		t.Fatalf("SaveThumbnail rejected a JPEG: %v", err)
	}
	stored, err := catalog.Video(t.Context(), video.ID)
	if err != nil || stored.ThumbnailGeneratedAt == nil {
		t.Fatalf("thumbnail was not recorded: %+v err=%v", stored, err)
	}
}

func TestThumbnailArgsPinQualityAndScaling(t *testing.T) {
	args := strings.Join(thumbnailFFmpegArgs("input.mp4", "cover.partial", "12.5"), " ")
	for _, required := range []string{
		"-ss 12.5",
		"-vf scale='min(640,iw)':-2:flags=spline",
		"-f image2",
		"-c:v mjpeg",
		"-q:v 3",
		"-y cover.partial",
	} {
		if !strings.Contains(args, required) {
			t.Fatalf("thumbnail arguments missing %q in %s", required, args)
		}
	}
}

// TestThumbnailIdentifiersCannotEscapeTheirDirectory keeps a catalog ID from
// becoming a path traversal, since the ID reaches the filesystem as a filename.
func TestThumbnailIdentifiersCannotEscapeTheirDirectory(t *testing.T) {
	catalog := openTestCatalog(t)
	for _, id := range []string{"../escape", "..\\escape", "", strings.Repeat("z", 64)} {
		if _, err := catalog.thumbnailPath(id); err == nil {
			t.Fatalf("thumbnailPath accepted %q", id)
		}
	}
	valid := strings.Repeat("ab", 32)
	path, err := catalog.thumbnailPath(valid)
	if err != nil {
		t.Fatalf("thumbnailPath rejected a real identifier: %v", err)
	}
	if filepath.Dir(path) != catalog.ThumbnailDir() {
		t.Fatalf("thumbnail escaped its directory: %q", path)
	}
}

func TestConversionCandidatesRefusePlayableFiles(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Plays.mp4"), "video")
	writeTestFile(t, filepath.Join(root, "Broken.mkv"), "video")
	runTestScan(t, catalog, root)

	for _, video := range listTestVideos(t, catalog) {
		if strings.HasPrefix(video.DisplayName, "Plays") {
			if err := catalog.SetCompatibility(t.Context(), video.ID, CompatibilityPlayable); err != nil {
				t.Fatalf("SetCompatibility: %v", err)
			}
		}
	}
	pending, err := catalog.conversionCandidates(t.Context(), nil)
	if err != nil {
		t.Fatalf("conversionCandidates: %v", err)
	}
	if len(pending) != 1 || pending[0].DisplayName != "Broken" {
		t.Fatalf("candidates = %+v, want only the unplayable container", pending)
	}
	// Naming a playable file explicitly must not convert it either: the gate is
	// the same in both entry points rather than re-derived per caller.
	var playableID string
	for _, video := range listTestVideos(t, catalog) {
		if video.DisplayName == "Plays" {
			playableID = video.ID
		}
	}
	if playableID == "" {
		t.Fatal("the playable fixture was not cataloged")
	}
	if _, err := catalog.conversionCandidates(t.Context(), []string{playableID}); err == nil {
		t.Fatal("a playable file was accepted as a conversion target")
	}
}

// TestObservedFailureSurvivesAndReverses covers the round trip the user asked
// for: a browser refusal is remembered so the offer survives a reload, and a
// later success in a browser that does have the decoder clears it. The verdict
// is browser-specific, so it has to be reversible.
func TestObservedFailureSurvivesAndReverses(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Hevc.mp4"), "video")
	runTestScan(t, catalog, root)
	video := listTestVideos(t, catalog)[0]

	if video.Compatibility != CompatibilityUnknown || video.NeedsConversion() {
		t.Fatalf("a fresh .mp4 row started as %q", video.Compatibility)
	}
	if err := catalog.SetCompatibility(t.Context(), video.ID, CompatibilityUnsupportedCodec); err != nil {
		t.Fatalf("SetCompatibility: %v", err)
	}
	reloaded, err := catalog.Video(t.Context(), video.ID)
	if err != nil || !reloaded.NeedsConversion() {
		t.Fatalf("observed failure did not survive: %+v err=%v", reloaded, err)
	}
	if err := catalog.SetCompatibility(t.Context(), video.ID, CompatibilityPlayable); err != nil {
		t.Fatalf("SetCompatibility: %v", err)
	}
	reloaded, err = catalog.Video(t.Context(), video.ID)
	if err != nil || reloaded.NeedsConversion() {
		t.Fatalf("a successful play did not clear the verdict: %+v err=%v", reloaded, err)
	}
}

// TestChangedFileForgetsWhatWasLearnedAboutIt keeps a stale verdict from
// hiding a broken video: a different file behind the same name has not been
// played, probed, or had a cover captured.
func TestChangedFileForgetsWhatWasLearnedAboutIt(t *testing.T) {
	catalog := openTestCatalog(t)
	root := t.TempDir()
	path := filepath.Join(root, "Session.mp4")
	writeTestFile(t, path, "video-one")
	runTestScan(t, catalog, root)
	video := listTestVideos(t, catalog)[0]

	if err := catalog.SetCompatibility(t.Context(), video.ID, CompatibilityPlayable); err != nil {
		t.Fatalf("SetCompatibility: %v", err)
	}
	jpeg := append([]byte{0xFF, 0xD8, 0xFF}, []byte("cover")...)
	if err := catalog.SaveThumbnail(t.Context(), video.ID, jpeg); err != nil {
		t.Fatalf("SaveThumbnail: %v", err)
	}

	writeTestFile(t, path, "an entirely different video")
	runTestScan(t, catalog, root)

	reloaded, err := catalog.Video(t.Context(), video.ID)
	if err != nil {
		t.Fatalf("Video: %v", err)
	}
	if reloaded.Compatibility != CompatibilityUnknown {
		t.Fatalf("replaced file kept a stale playability claim: %q", reloaded.Compatibility)
	}
	if reloaded.ThumbnailGeneratedAt != nil {
		t.Fatal("replaced file kept a cover of the previous video")
	}
}

func TestResolveToolsReportsAbsenceDistinctlyFromInvalid(t *testing.T) {
	if _, err := ResolveTools(t.Context(), "  "); err != ErrToolsUnavailable {
		t.Fatalf("empty path error = %v, want %v", err, ErrToolsUnavailable)
	}
	relative := "ffmpeg"
	if runtime.GOOS == "windows" {
		relative = "ffmpeg.exe"
	}
	if _, err := ResolveTools(t.Context(), relative); err == nil {
		t.Fatal("a relative path was accepted")
	}
	missing := filepath.Join(t.TempDir(), "ffmpeg")
	if _, err := ResolveTools(t.Context(), missing); err == nil {
		t.Fatal("a missing binary was accepted")
	}
}

// defaultMediaSettings returns normalized defaults, which is what the server
// always hands the conversion planner.
func defaultMediaSettings(t *testing.T) config.MediaSettings {
	t.Helper()
	settings, err := config.NormalizeSettings(config.DefaultSettings())
	if err != nil {
		t.Fatalf("NormalizeSettings: %v", err)
	}
	return settings.Media
}

// TestConvertedRowDoesNotInheritSourceCodecs pins a real defect: the converted
// row used to be written with the *source* file's stream info. After a
// re-encode that is the codec the file was converted away from, and the UI
// names the stored codec when it explains why something will not play — so the
// row would blame H.264 output for being the MPEG-4 it replaced.
func TestConvertedRowDoesNotInheritSourceCodecs(t *testing.T) {
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
	if source.ID == "" {
		t.Fatal("source fixture was not cataloged")
	}

	// What the encoder actually produced, not what went into it.
	if err := catalog.adoptConvertedFile(t.Context(), source, StreamInfo{
		VideoCodec: "h264", AudioCodec: "aac", HasVideo: true,
	}); err != nil {
		t.Fatalf("adoptConvertedFile: %v", err)
	}
	for _, video := range listTestVideos(t, catalog) {
		if video.DisplayName != "Clip_MHConverted" {
			continue
		}
		if video.VideoCodec == nil || *video.VideoCodec != "h264" {
			t.Fatalf("converted row video codec = %v, want h264", video.VideoCodec)
		}
		return
	}
	t.Fatal("converted row was not cataloged")
}

// TestUnprobeableConversionReportsNoCodec keeps "not probed" distinguishable
// from "probed and found nothing".
func TestUnprobeableConversionReportsNoCodec(t *testing.T) {
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
	if err := catalog.adoptConvertedFile(t.Context(), source, StreamInfo{}); err != nil {
		t.Fatalf("adoptConvertedFile: %v", err)
	}
	for _, video := range listTestVideos(t, catalog) {
		if video.DisplayName == "Clip_MHConverted" && video.VideoCodec != nil {
			t.Fatalf("unprobed conversion reported a codec: %q", *video.VideoCodec)
		}
	}
}
