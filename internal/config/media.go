package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// MediaSettings contains user-selected roots for explicit video catalog scans.
// Paths are not secrets, but the scanner never accepts them from media routes.
type MediaSettings struct {
	LibraryPaths []string `json:"library_paths"`
	// ScriptOffsetMillis shifts a paired script against its video. Positive
	// delays the script, negative advances it. Some offset is not a defect the
	// app can remove: scripts are authored against a particular sense of "on
	// the beat", displays add their own presentation latency, and the device
	// takes real time to move. This is the calibration control for that
	// remainder, not a substitute for correct clock alignment.
	ScriptOffsetMillis int `json:"script_offset_ms"`
	// ScriptSmoothingPercent removes authored extrema below this prominence.
	// Zero is off, which is the default: the paired script plays exactly as
	// authored unless the user asks for something else.
	ScriptSmoothingPercent int `json:"script_smoothing_percent"`
	// PeakRoundingMillis rounds each direction change over this window, moving
	// a hand-authored triangle toward a sine. Zero is off.
	PeakRoundingMillis int `json:"peak_rounding_ms"`
	// FFmpegPath points at an external FFmpeg the user supplies. Empty means
	// absent, which is a supported state: every feature that needs it says so
	// rather than half-working. ffprobe is resolved beside it.
	FFmpegPath string `json:"ffmpeg_path"`
	// ConvertH265ForCompatibility declares that HEVC does not play in this
	// browser. The default assumes it does, because most current setups decode
	// it; Firefox is the common exception. Turning this on both makes H.265 a
	// conversion candidate and forces re-encodes to H.264 — see
	// ReencodeCodecFor, which owns that interlock so no caller can half-apply it.
	ConvertH265ForCompatibility bool `json:"convert_h265_for_compatibility"`
	// ReencodeCodec is the target when a remux cannot fix the file.
	ReencodeCodec string `json:"reencode_codec"`
	// ReencodeCRFH264 and ReencodeCRFH265 are separate because the two scales
	// are not the same number: carrying one value across a codec change would
	// silently move quality in whichever direction the user did not ask for.
	ReencodeCRFH264 int `json:"reencode_crf_h264"`
	ReencodeCRFH265 int `json:"reencode_crf_h265"`
	// ReencodePreset trades encoding time for file size at equal quality.
	ReencodePreset string `json:"reencode_preset"`
	// ReencodeAudioKbps applies only when the source audio is not already AAC.
	ReencodeAudioKbps int `json:"reencode_audio_kbps"`
	// GenerateThumbnailsOnScan and ConvertIncompatibleOnScan ride an explicit
	// scan the user started. Never at startup, never on a timer.
	GenerateThumbnailsOnScan  bool `json:"generate_thumbnails_on_scan"`
	ConvertIncompatibleOnScan bool `json:"convert_incompatible_on_scan"`
	// ShowSupersededOriginals reveals rows hidden because a converted copy of
	// the same basename exists beside them.
	ShowSupersededOriginals bool `json:"show_superseded_originals"`
}

// Re-encode targets. Code-owned enums rather than free text, because these
// become FFmpeg arguments.
const (
	// ReencodeCodecH264 always plays, which is why it is the default.
	ReencodeCodecH264 = "h264"
	// ReencodeCodecH265 halves the file at equal quality but depends on the
	// viewer's OS, browser, and hardware decode.
	ReencodeCodecH265 = "h265"
)

// Encoder speed presets, slowest-to-fastest ordering owned by x264/x265.
var reencodePresets = []string{
	"ultrafast", "superfast", "veryfast", "faster", "fast",
	"medium", "slow", "slower", "veryslow",
}

// ReencodePresets returns the accepted encoder presets.
func ReencodePresets() []string { return cloneStrings(reencodePresets) }

// Bounds for the re-encode dials. CRF is exposed over the range where the
// number still means something; outside it the encoder is either wasting space
// or visibly destroying detail.
const (
	// MinReencodeCRF is near-lossless and very large.
	MinReencodeCRF = 18
	// MaxReencodeCRF is where compression artifacts become obvious.
	MaxReencodeCRF = 30
	// DefaultReencodeCRFH264 is the x264 default quality point.
	DefaultReencodeCRFH264 = 23
	// DefaultReencodeCRFH265 matches roughly the same visual quality on the
	// x265 scale, which is offset from x264's by about five points.
	DefaultReencodeCRFH265 = 28
	// MinReencodeAudioKbps and MaxReencodeAudioKbps bound AAC output.
	MinReencodeAudioKbps = 96
	MaxReencodeAudioKbps = 320
	// DefaultReencodeAudioKbps is transparent for stereo AAC.
	DefaultReencodeAudioKbps = 192
	// DefaultReencodePreset balances encode time against size.
	DefaultReencodePreset = "medium"
)

// ReencodeCodecFor resolves the effective re-encode target. It exists so the
// compatibility toggle cannot be half-applied: a user who has just declared
// that H.265 does not play here must never get an H.265 file back, which is an
// hour of encoding spent arriving at another file that will not play.
func (s MediaSettings) ReencodeCodecFor() string {
	if s.ConvertH265ForCompatibility {
		return ReencodeCodecH264
	}
	if s.ReencodeCodec == ReencodeCodecH265 {
		return ReencodeCodecH265
	}
	return ReencodeCodecH264
}

// ReencodeCRF returns the quality point for the effective codec.
func (s MediaSettings) ReencodeCRF() int {
	if s.ReencodeCodecFor() == ReencodeCodecH265 {
		return s.ReencodeCRFH265
	}
	return s.ReencodeCRFH264
}

// MaxScriptOffsetMillis bounds the paired-script calibration offset. Two
// seconds covers authoring bias, display presentation latency, and device
// actuation lag together; beyond that the pairing itself is wrong.
const MaxScriptOffsetMillis = 2000

// MaxScriptSmoothingPercent bounds jitter removal. Above this the filter stops
// removing noise and starts removing strokes.
const MaxScriptSmoothingPercent = 5

// MaxPeakRoundingMillis bounds how far a rounded corner reaches. Beyond this
// the fillet stops being a softened peak and becomes a different stroke shape.
const MaxPeakRoundingMillis = 200

// MediaUpdate patches only the submitted media settings. Filter controls save
// independently from the general Settings form, so omitted fields must retain
// the latest durable values rather than being reset by a stale form snapshot.
type MediaUpdate struct {
	LibraryPaths                *[]string `json:"library_paths,omitempty"`
	ScriptOffsetMillis          *int      `json:"script_offset_ms,omitempty"`
	ScriptSmoothingPercent      *int      `json:"script_smoothing_percent,omitempty"`
	PeakRoundingMillis          *int      `json:"peak_rounding_ms,omitempty"`
	FFmpegPath                  *string   `json:"ffmpeg_path,omitempty"`
	ConvertH265ForCompatibility *bool     `json:"convert_h265_for_compatibility,omitempty"`
	ReencodeCodec               *string   `json:"reencode_codec,omitempty"`
	ReencodeCRFH264             *int      `json:"reencode_crf_h264,omitempty"`
	ReencodeCRFH265             *int      `json:"reencode_crf_h265,omitempty"`
	ReencodePreset              *string   `json:"reencode_preset,omitempty"`
	ReencodeAudioKbps           *int      `json:"reencode_audio_kbps,omitempty"`
	GenerateThumbnailsOnScan    *bool     `json:"generate_thumbnails_on_scan,omitempty"`
	ConvertIncompatibleOnScan   *bool     `json:"convert_incompatible_on_scan,omitempty"`
	ShowSupersededOriginals     *bool     `json:"show_superseded_originals,omitempty"`
}

// mergeMediaUpdate applies only the fields a caller actually submitted. The
// media panel and the floating playback panel patch different subsets of the
// same struct, so an omitted field has to keep its durable value rather than be
// reset by whichever form posted last.
func mergeMediaUpdate(media MediaSettings, update MediaUpdate) MediaSettings {
	if update.LibraryPaths != nil {
		media.LibraryPaths = append([]string{}, (*update.LibraryPaths)...)
	}
	assignInt(&media.ScriptOffsetMillis, update.ScriptOffsetMillis)
	assignInt(&media.ScriptSmoothingPercent, update.ScriptSmoothingPercent)
	assignInt(&media.PeakRoundingMillis, update.PeakRoundingMillis)
	assignString(&media.FFmpegPath, update.FFmpegPath)
	assignBool(&media.ConvertH265ForCompatibility, update.ConvertH265ForCompatibility)
	assignString(&media.ReencodeCodec, update.ReencodeCodec)
	assignInt(&media.ReencodeCRFH264, update.ReencodeCRFH264)
	assignInt(&media.ReencodeCRFH265, update.ReencodeCRFH265)
	assignString(&media.ReencodePreset, update.ReencodePreset)
	assignInt(&media.ReencodeAudioKbps, update.ReencodeAudioKbps)
	assignBool(&media.GenerateThumbnailsOnScan, update.GenerateThumbnailsOnScan)
	assignBool(&media.ConvertIncompatibleOnScan, update.ConvertIncompatibleOnScan)
	assignBool(&media.ShowSupersededOriginals, update.ShowSupersededOriginals)
	return media
}

func assignInt(target *int, value *int) {
	if value != nil {
		*target = *value
	}
}

func assignBool(target *bool, value *bool) {
	if value != nil {
		*target = *value
	}
}

func assignString(target *string, value *string) {
	if value != nil {
		*target = *value
	}
}

func normalizeMediaSettings(settings MediaSettings) MediaSettings {
	paths := make([]string, 0, len(settings.LibraryPaths))
	for _, value := range settings.LibraryPaths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = filepath.Clean(value)
		duplicate := false
		for _, existing := range paths {
			if existing == value || (runtime.GOOS == "windows" && strings.EqualFold(existing, value)) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			paths = append(paths, value)
		}
	}
	settings.LibraryPaths = paths
	settings.ScriptOffsetMillis = clampInt(
		settings.ScriptOffsetMillis,
		-MaxScriptOffsetMillis,
		MaxScriptOffsetMillis,
	)
	settings.ScriptSmoothingPercent = clampInt(settings.ScriptSmoothingPercent, 0, MaxScriptSmoothingPercent)
	settings.PeakRoundingMillis = clampInt(settings.PeakRoundingMillis, 0, MaxPeakRoundingMillis)
	return normalizeConversionSettings(settings)
}

// normalizeConversionSettings fills the encode dials. Zero means "absent" for
// every one of them, so a settings document written before these fields existed
// loads with working defaults instead of a CRF of 18 and a nameless preset.
func normalizeConversionSettings(settings MediaSettings) MediaSettings {
	settings.FFmpegPath = strings.TrimSpace(settings.FFmpegPath)
	if settings.ReencodeCodec != ReencodeCodecH265 {
		settings.ReencodeCodec = ReencodeCodecH264
	}
	if settings.ReencodeCRFH264 == 0 {
		settings.ReencodeCRFH264 = DefaultReencodeCRFH264
	}
	if settings.ReencodeCRFH265 == 0 {
		settings.ReencodeCRFH265 = DefaultReencodeCRFH265
	}
	settings.ReencodeCRFH264 = clampInt(settings.ReencodeCRFH264, MinReencodeCRF, MaxReencodeCRF)
	settings.ReencodeCRFH265 = clampInt(settings.ReencodeCRFH265, MinReencodeCRF, MaxReencodeCRF)
	settings.ReencodePreset = strings.ToLower(strings.TrimSpace(settings.ReencodePreset))
	if !slices.Contains(reencodePresets, settings.ReencodePreset) {
		settings.ReencodePreset = DefaultReencodePreset
	}
	if settings.ReencodeAudioKbps == 0 {
		settings.ReencodeAudioKbps = DefaultReencodeAudioKbps
	}
	settings.ReencodeAudioKbps = clampInt(settings.ReencodeAudioKbps, MinReencodeAudioKbps, MaxReencodeAudioKbps)
	return settings
}

func validateMediaSettings(settings MediaSettings) error {
	// The script offset is clamped in normalizeMediaSettings rather than
	// rejected here: it is a calibration dial, and a hand-edited settings file
	// should still load with a usable value instead of failing to open.
	const maxLibraryPaths = 32
	if len(settings.LibraryPaths) > maxLibraryPaths {
		return fmt.Errorf("media library supports at most %d locations", maxLibraryPaths)
	}
	for _, value := range settings.LibraryPaths {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("media library location must be an absolute path: %q", value)
		}
		if len(value) > 4096 {
			return errors.New("media library location is too long")
		}
	}
	// An absolute path is required so the resolved binary never depends on the
	// process working directory or on PATH order, either of which could put a
	// different executable behind the same setting between runs.
	if settings.FFmpegPath != "" {
		if !filepath.IsAbs(settings.FFmpegPath) {
			return fmt.Errorf("ffmpeg path must be an absolute path: %q", settings.FFmpegPath)
		}
		if len(settings.FFmpegPath) > 4096 {
			return errors.New("ffmpeg path is too long")
		}
	}
	return nil
}
