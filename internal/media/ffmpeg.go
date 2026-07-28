package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	// ErrToolsUnavailable reports that no usable FFmpeg is configured. It is a
	// normal state, not a failure: every feature that needs FFmpeg says so and
	// the rest of the app is unaffected.
	ErrToolsUnavailable = errors.New("ffmpeg is not configured")
	// ErrToolInvalid reports a configured path that is not a working FFmpeg.
	ErrToolInvalid = errors.New("configured ffmpeg could not be verified")
)

const (
	// versionProbeTimeout bounds identification. A binary that cannot say what
	// it is within this window is not one worth invoking with real arguments.
	versionProbeTimeout = 10 * time.Second
	// inspectTimeout bounds a container probe. ffprobe reads headers, not the
	// whole file, so this is generous rather than tight.
	inspectTimeout = 30 * time.Second
	// maxToolOutputBytes caps captured stdout/stderr so a chatty or wedged
	// process cannot grow the heap without bound.
	maxToolOutputBytes = 1 << 20
)

// Tools is a verified pair of external binaries. It is produced only by
// ResolveTools, so holding one is proof the paths were identified rather than
// merely configured.
type Tools struct {
	FFmpegPath  string `json:"ffmpeg_path"`
	FFprobePath string `json:"ffprobe_path"`
	Version     string `json:"version"`
}

// ToolStatus is the API view of the dependency, including the absent case.
type ToolStatus struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	FFmpegPath string `json:"ffmpeg_path,omitempty"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ResolveTools verifies a configured FFmpeg and locates ffprobe beside it.
//
// Identification is not a formality. The path is a user-supplied string that
// becomes argv[0] of a child process, so it is run once with a harmless
// argument and required to identify itself before it is ever handed a real
// input path. A binary that does not print an FFmpeg banner is rejected here
// rather than discovered mid-conversion.
func ResolveTools(ctx context.Context, configuredPath string) (Tools, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" {
		return Tools{}, ErrToolsUnavailable
	}
	if !filepath.IsAbs(configuredPath) {
		return Tools{}, fmt.Errorf("%w: path must be absolute", ErrToolInvalid)
	}
	info, err := os.Stat(configuredPath)
	if err != nil {
		return Tools{}, fmt.Errorf("%w: %w", ErrToolInvalid, err)
	}
	if !info.Mode().IsRegular() {
		return Tools{}, fmt.Errorf("%w: not a regular file", ErrToolInvalid)
	}

	version, err := identifyTool(ctx, configuredPath)
	if err != nil {
		return Tools{}, err
	}
	probePath, err := locateProbe(ctx, configuredPath)
	if err != nil {
		return Tools{}, err
	}
	return Tools{FFmpegPath: configuredPath, FFprobePath: probePath, Version: version}, nil
}

func identifyTool(ctx context.Context, path string) (string, error) {
	output, err := runTool(ctx, versionProbeTimeout, path, "-version")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrToolInvalid, err)
	}
	banner := strings.TrimSpace(firstLine(string(output)))
	// Both binaries print "<name> version <x>" as their first line. Requiring
	// it rejects a path that happens to be executable but is not FFmpeg.
	if !strings.Contains(strings.ToLower(banner), "version") {
		return "", fmt.Errorf("%w: %q did not report a version", ErrToolInvalid, filepath.Base(path))
	}
	return banner, nil
}

// locateProbe finds ffprobe beside ffmpeg. Conversion needs it to decide
// between a remux and a re-encode, and guessing wrong costs an hour of CPU, so
// an FFmpeg without a matching ffprobe is reported rather than half-accepted.
func locateProbe(ctx context.Context, ffmpegPath string) (string, error) {
	directory := filepath.Dir(ffmpegPath)
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidate := filepath.Join(directory, name)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("%w: ffprobe was not found beside ffmpeg", ErrToolInvalid)
	}
	if _, err := identifyTool(ctx, candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

// StreamInfo is the subset of a container probe that decides what conversion
// has to do. Everything else ffprobe reports is deliberately discarded.
type StreamInfo struct {
	VideoCodec     string
	AudioCodec     string
	DurationMillis int64
	HasVideo       bool
}

type probeReport struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Inspect reads container metadata. It never decodes frames, so the cost is
// bounded by header size rather than by the length of the file.
func (t Tools) Inspect(ctx context.Context, path string) (StreamInfo, error) {
	output, err := runTool(ctx, inspectTimeout, t.FFprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_entries", "stream=codec_type,codec_name:format=duration",
		"-i", path,
	)
	if err != nil {
		return StreamInfo{}, fmt.Errorf("inspect media file: %w", err)
	}
	var report probeReport
	if err := json.Unmarshal(output, &report); err != nil {
		return StreamInfo{}, fmt.Errorf("parse media probe: %w", err)
	}
	info := StreamInfo{DurationMillis: parseDurationSeconds(report.Format.Duration)}
	for _, stream := range report.Streams {
		codec := strings.ToLower(strings.TrimSpace(stream.CodecName))
		switch strings.ToLower(stream.CodecType) {
		case "video":
			// The first video stream wins. Cover art is stored as a video
			// stream too, but it appears after the real one in every container
			// this app indexes.
			if !info.HasVideo {
				info.VideoCodec = codec
				info.HasVideo = true
			}
		case "audio":
			if info.AudioCodec == "" {
				info.AudioCodec = codec
			}
		}
	}
	if !info.HasVideo {
		return info, errors.New("file contains no video stream")
	}
	return info, nil
}

// runTool executes a tool with an explicit argv and no shell. No user text ever
// reaches these arguments as anything but a path the catalog already resolved.
func runTool(ctx context.Context, timeout time.Duration, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr boundedBuffer
	stdout.limit = maxToolOutputBytes
	stderr.limit = maxToolOutputBytes
	command := exec.CommandContext(ctx, path, args...) // #nosec G204 -- absolute verified path, code-owned args.
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s timed out", filepath.Base(path))
		}
		return nil, fmt.Errorf("%s failed: %s", filepath.Base(path), toolErrorDetail(stderr.String(), err))
	}
	return stdout.Bytes(), nil
}

// toolErrorDetail prefers the tool's own last line over the exit status, which
// on its own only ever says "exit status 1".
func toolErrorDetail(stderr string, err error) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if line := strings.TrimSpace(lines[index]); line != "" {
			return line
		}
	}
	return err.Error()
}

// boundedBuffer keeps at most limit bytes so a wedged child process cannot
// drive memory use through its output stream.
type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if remaining := b.limit - b.buffer.Len(); remaining > 0 {
		if len(data) > remaining {
			b.buffer.Write(data[:remaining])
		} else {
			b.buffer.Write(data)
		}
	}
	// The caller is told everything was written. A truncated log is not worth
	// failing a conversion that is otherwise succeeding.
	return len(data), nil
}

func (b *boundedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedBuffer) String() string { return b.buffer.String() }

func firstLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return value[:index]
	}
	return value
}

func parseDurationSeconds(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var seconds float64
	if _, err := fmt.Sscanf(value, "%f", &seconds); err != nil || seconds <= 0 {
		return 0
	}
	return int64(seconds * 1000)
}
