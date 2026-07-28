package media

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Compatibility records what is known about whether a file plays in the browser
// that is actually running the UI.
//
// Playability is a codec question at least as much as a container one, and that
// distinction is the whole reason this type exists. An .mp4 is a container every
// browser opens; an .mp4 holding HEVC does not play in Firefox, which ships no
// HEVC decoder on most platforms. Classifying by extension alone would call that
// file supported and never offer to fix it.
//
// So the extension set below is a lower bound on what cannot play, never a claim
// about what can. Only CompatibilityPlayable is a positive statement, and only
// the browser can produce it.
type Compatibility string

const (
	// CompatibilityUnknown is a file in an openable container that has not been
	// played yet. Most rows sit here, and it is not a problem: the app makes no
	// claim it cannot support, and the first playback settles the question.
	CompatibilityUnknown Compatibility = "unknown"
	// CompatibilityPlayable is a file this browser has actually decoded. It is
	// the only compatibility value the app does not infer.
	CompatibilityPlayable Compatibility = "playable"
	// CompatibilityUnsupportedContainer is an extension the <video> element
	// will not open regardless of what is inside it.
	CompatibilityUnsupportedContainer Compatibility = "unsupported_container"
	// CompatibilityUnsupportedCodec is a file the browser opened and then
	// refused to decode. This is the case extension checks cannot reach.
	CompatibilityUnsupportedCodec Compatibility = "unsupported_codec"
)

// NeedsConversion reports whether repair would help. Conversion is repair only,
// so a file that plays is never a candidate no matter what else is true of it.
func (c Compatibility) NeedsConversion() bool {
	return c == CompatibilityUnsupportedContainer || c == CompatibilityUnsupportedCodec
}

// Valid reports whether a stored value is one this binary understands. A row
// written by a newer build is treated as unknown rather than trusted.
func (c Compatibility) Valid() bool {
	switch c {
	case CompatibilityUnknown, CompatibilityPlayable,
		CompatibilityUnsupportedContainer, CompatibilityUnsupportedCodec:
		return true
	default:
		return false
	}
}

// ConvertedSuffix marks a file this app produced. It is fixed rather than
// configurable: changing it would orphan every file already carrying it.
const ConvertedSuffix = "_MHConverted"

// ConvertedExtension is the only container conversion targets.
const ConvertedExtension = ".mp4"

// playableExtensions are containers the <video> element opens. Whether the
// streams inside them decode is a separate question that only playback answers.
var playableExtensions = map[string]struct{}{
	".mp4": {}, ".m4v": {}, ".webm": {}, ".mov": {},
}

// unplayableExtensions are containers no browser opens. Kept deliberately
// short: every entry adds catalog rows, so a container earns a place here only
// if people actually keep video in it and conversion can repair it.
var unplayableExtensions = map[string]struct{}{
	".mkv": {}, ".avi": {}, ".wmv": {}, ".flv": {}, ".ts": {}, ".m2ts": {},
	".mpg": {}, ".mpeg": {}, ".vob": {}, ".ogv": {}, ".rm": {}, ".rmvb": {},
	".asf": {}, ".divx": {}, ".3gp": {},
}

// CatalogedExtension reports whether the scanner indexes this extension at all.
// Unplayable containers are indexed too, because a file the app hides is a file
// the user cannot ask it to repair.
func CatalogedExtension(extension string) bool {
	extension = strings.ToLower(extension)
	_, playable := playableExtensions[extension]
	if playable {
		return true
	}
	_, unplayable := unplayableExtensions[extension]
	return unplayable
}

// containerTypes are the MIME types the stream endpoint serves. They are sent
// to the client so it can ask its own engine, via canPlayType, whether it
// supports the container — which is a far better answer than any table here.
var containerTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime",
	".mkv": "video/x-matroska", ".avi": "video/x-msvideo", ".wmv": "video/x-ms-wmv",
	".flv": "video/x-flv", ".ts": "video/mp2t", ".m2ts": "video/mp2t",
	".mpg": "video/mpeg", ".mpeg": "video/mpeg", ".vob": "video/mpeg", ".ogv": "video/ogg",
	".rm": "application/vnd.rn-realmedia", ".rmvb": "application/vnd.rn-realmedia",
	".asf": "video/x-ms-asf", ".divx": "video/x-msvideo", ".3gp": "video/3gpp",
}

// ContainerType returns the MIME type for a path, or empty when unknown.
func ContainerType(path string) string {
	return containerTypes[strings.ToLower(filepath.Ext(path))]
}

// ContainerCompatibility classifies a path by extension alone. It returns
// unknown rather than playable for openable containers, because opening a
// container is not decoding its contents.
func ContainerCompatibility(path string) Compatibility {
	if _, ok := unplayableExtensions[strings.ToLower(filepath.Ext(path))]; ok {
		return CompatibilityUnsupportedContainer
	}
	return CompatibilityUnknown
}

// ConvertedName returns the output name for a source path: the basename gains
// the reserved suffix and the extension becomes .mp4.
func ConvertedName(relativePath string) string {
	base := filepath.Base(filepath.FromSlash(relativePath))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return stem + ConvertedSuffix + ConvertedExtension
}

// HasConvertedSuffix reports whether a path is app output. Such a file is never
// converted again, so NAME_MHConverted_MHConverted.mp4 cannot exist.
func HasConvertedSuffix(relativePath string) bool {
	return stripConvertedSuffix(relativePath) != baseStem(relativePath)
}

// stripConvertedSuffix removes one trailing reserved suffix from a basename.
// Exactly one: the suffix is stripped once, not repeatedly, so a user's own
// file called "Clip_MHConverted_MHConverted" is left alone rather than
// unwound into a name they never had.
func stripConvertedSuffix(relativePath string) string {
	stem := baseStem(relativePath)
	if len(stem) <= len(ConvertedSuffix) {
		return stem
	}
	tail := stem[len(stem)-len(ConvertedSuffix):]
	if !equalPathFold(tail, ConvertedSuffix) {
		return stem
	}
	return stem[:len(stem)-len(ConvertedSuffix)]
}

func baseStem(relativePath string) string {
	base := filepath.Base(filepath.FromSlash(relativePath))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// equalPathFold compares the way the host filesystem does. Windows is
// case-insensitive, so Clip_mhconverted.mp4 supersedes Clip.mkv there and does
// not on Linux — matching how those two systems would treat the names anyway.
func equalPathFold(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
