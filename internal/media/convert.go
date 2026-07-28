package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// browserPlayableVideoCodecs are codecs that are both legal in MP4 and actually
// decoded by current browsers. Legality alone is not the test: MPEG-4 Part 2
// stores happily in an MP4 and no browser will play it, so copying such a
// stream would produce a valid file that still does not work.
var browserPlayableVideoCodecs = map[string]struct{}{
	"h264": {}, "av1": {},
}

// browserPlayableAudioCodecs applies the same rule to audio. AC-3 is the
// notable exclusion: legal in MP4, shipped on plenty of rips, decoded by
// neither Firefox nor Chrome.
var browserPlayableAudioCodecs = map[string]struct{}{
	"aac": {}, "mp3": {},
}

// ConversionAction is what repairing a file actually requires.
type ConversionAction string

const (
	// ActionRemux copies both streams into MP4. Seconds, and lossless.
	ActionRemux ConversionAction = "remux"
	// ActionEncodeAudio keeps the video stream and re-encodes only audio.
	ActionEncodeAudio ConversionAction = "encode_audio"
	// ActionEncodeVideo re-encodes video, and audio only if it needs it.
	ActionEncodeVideo ConversionAction = "encode_video"
)

// ConversionPlan is the decision made from a probe, before any work starts.
type ConversionPlan struct {
	Action        ConversionAction `json:"action"`
	VideoCodec    string           `json:"video_codec"`
	AudioCodec    string           `json:"audio_codec"`
	TargetCodec   string           `json:"target_codec,omitempty"`
	DurationMilli int64            `json:"duration_ms"`
	// Lossless is true when no stream is re-encoded, which is worth saying in
	// the UI: it is the difference between ten seconds and an hour.
	Lossless bool `json:"lossless"`
}

// PlanConversion decides what a file needs. Remux is tried first rather than
// treated as an optimization: skipping it would turn a ten-second job into an
// hour-long one and degrade quality for no reason.
func PlanConversion(info StreamInfo, settings config.MediaSettings) ConversionPlan {
	plan := ConversionPlan{
		VideoCodec:    info.VideoCodec,
		AudioCodec:    info.AudioCodec,
		DurationMilli: info.DurationMillis,
	}
	videoKeepable := canCopyVideo(info.VideoCodec, settings)
	// An absent audio stream needs no encoder. Treating "" as unplayable would
	// re-encode silent video for nothing.
	audioKeepable := info.AudioCodec == "" || isPlayableAudio(info.AudioCodec)

	switch {
	case videoKeepable && audioKeepable:
		plan.Action = ActionRemux
		plan.Lossless = true
	case videoKeepable:
		plan.Action = ActionEncodeAudio
	default:
		plan.Action = ActionEncodeVideo
		plan.TargetCodec = settings.ReencodeCodecFor()
	}
	return plan
}

// canCopyVideo reports whether the video stream can be carried across
// untouched. HEVC is the interesting case: it is copyable by default because
// most setups decode it, and not copyable once the user has told us theirs does
// not — at which point ReencodeCodecFor has already forced the target to H.264,
// so the re-encode cannot land back on a codec that will not play.
func canCopyVideo(codec string, settings config.MediaSettings) bool {
	if codec == "hevc" || codec == "h265" {
		return !settings.ConvertH265ForCompatibility
	}
	_, ok := browserPlayableVideoCodecs[codec]
	return ok
}

func isPlayableAudio(codec string) bool {
	_, ok := browserPlayableAudioCodecs[codec]
	return ok
}

// ffmpegArgs builds the argv for a plan. Every value here is a code-owned
// constant or a number the settings layer has already clamped; no user text is
// interpolated into an encoder, filter, or metadata argument.
func (p ConversionPlan) ffmpegArgs(sourcePath, targetPath string, settings config.MediaSettings) []string {
	args := []string{
		"-nostdin",
		"-loglevel", "error",
		"-i", sourcePath,
		// Take the first video stream and the first audio stream if one
		// exists. Subtitles and data streams are dropped: MKV's ASS/SSA has no
		// MP4 equivalent worth carrying, and silently mangling it into mov_text
		// would be worse than leaving it behind.
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-sn", "-dn",
		"-map_metadata", "-1",
	}
	switch p.Action {
	case ActionRemux:
		args = append(args, "-c", "copy")
	case ActionEncodeAudio:
		args = append(args, "-c:v", "copy")
		args = append(args, audioEncodeArgs(settings)...)
	case ActionEncodeVideo:
		args = append(args, videoEncodeArgs(settings)...)
		if isPlayableAudio(p.AudioCodec) {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, audioEncodeArgs(settings)...)
		}
	}
	// faststart is not optional. It relocates the index to the front of the
	// file so the browser can begin playing from a range request instead of
	// fetching the whole thing first. Without it a converted file technically
	// plays and practically feels broken over this app's own streaming path.
	args = append(args,
		"-movflags", "+faststart",
		// The muxer is named rather than inferred. Output is written to a
		// temporary "<name>.partial" path and renamed on success, and FFmpeg
		// picks its format from the file extension — so without this it sees
		// ".partial", cannot resolve a muxer, and fails with a bare
		// "Error opening output files: Invalid argument".
		"-f", "mp4",
		"-progress", "pipe:1",
		"-nostats",
		// Never overwrite. The target is a temporary name this code chose, so
		// an existing file there means something unexpected is happening.
		"-n", targetPath,
	)
	return args
}

func videoEncodeArgs(settings config.MediaSettings) []string {
	encoder := "libx264"
	if settings.ReencodeCodecFor() == config.ReencodeCodecH265 {
		encoder = "libx265"
	}
	args := []string{
		"-c:v", encoder,
		"-crf", strconv.Itoa(settings.ReencodeCRF()),
		"-preset", settings.ReencodePreset,
		// yuv420p is what browsers decode. Sources in 10-bit or 4:2:2 would
		// otherwise produce a technically valid file that plays nowhere.
		"-pix_fmt", "yuv420p",
	}
	if encoder == "libx265" {
		// Without hvc1 tagging, Safari and QuickTime refuse the result.
		args = append(args, "-tag:v", "hvc1")
	}
	return args
}

func audioEncodeArgs(settings config.MediaSettings) []string {
	return []string{"-c:a", "aac", "-b:a", strconv.Itoa(settings.ReencodeAudioKbps) + "k"}
}

// ConversionProgress reports how far a running conversion has reached.
type ConversionProgress struct {
	ProcessedMillis int64
	TotalMillis     int64
}

// runConversion executes a plan and reports progress. Output goes to a
// temporary name in the destination directory and is renamed only on success,
// so a cancelled or failed run never leaves a partial file that looks finished.
func (t Tools) runConversion(
	ctx context.Context,
	plan ConversionPlan,
	sourcePath, targetPath string,
	settings config.MediaSettings,
	progress func(ConversionProgress),
) error {
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("a converted file already exists at %s", filepath.Base(targetPath))
	}
	temporaryPath := targetPath + ".partial"
	_ = os.Remove(temporaryPath)

	args := plan.ffmpegArgs(sourcePath, temporaryPath, settings)
	command := exec.CommandContext(ctx, t.FFmpegPath, args...) // #nosec G204 -- verified path, code-owned args.
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr boundedBuffer
	stderr.limit = maxToolOutputBytes
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		return fmt.Errorf("start conversion: %w", err)
	}
	readConversionProgress(stdout, plan.DurationMilli, progress)
	if err := command.Wait(); err != nil {
		_ = os.Remove(temporaryPath)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New(toolErrorDetail(stderr.String(), err))
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("finalize converted file: %w", err)
	}
	return nil
}

// readConversionProgress consumes FFmpeg's -progress stream. The stream must be
// drained whether or not anyone is watching, because a full pipe buffer would
// block the encoder.
func readConversionProgress(stdout io.Reader, totalMillis int64, progress func(ConversionProgress)) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		key, value, found := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !found || key != "out_time_ms" || progress == nil {
			continue
		}
		// The key says milliseconds and the value is microseconds. This has
		// been true and misnamed in FFmpeg for years.
		microseconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || microseconds < 0 {
			continue
		}
		progress(ConversionProgress{ProcessedMillis: microseconds / 1000, TotalMillis: totalMillis})
	}
}
