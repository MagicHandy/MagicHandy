package media

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var (
	// ErrThumbnailInvalid reports a cover payload that is not a usable JPEG.
	ErrThumbnailInvalid = errors.New("thumbnail image is not a supported JPEG")
	// ErrThumbnailNotFound reports a video with no generated cover.
	ErrThumbnailNotFound = errors.New("thumbnail has not been generated")
)

const (
	// MaxThumbnailBytes bounds an accepted cover. The browser is asked for a
	// small JPEG, so anything approaching this is a client that ignored the
	// contract rather than an unusually detailed frame.
	MaxThumbnailBytes = 2 << 20
	// thumbnailMaxEdge is the widest a generated cover gets. The grid tile is
	// far smaller; this leaves room for a high-density display without storing
	// a second copy of the video.
	thumbnailMaxEdge = 640
	// thumbnailJPEGQuality is FFmpeg's fixed MJPEG quantizer. Lower is better;
	// three keeps small covers crisp without the storage cost of near-lossless
	// output and avoids FFmpeg's content-dependent default bitrate control.
	thumbnailJPEGQuality = 3
	// thumbnailCaptureFraction is how far into a video a batch capture seeks.
	// Not frame zero: the first frame of a video is very often black, and a
	// library of black tiles is indistinguishable from a broken feature.
	thumbnailCaptureFraction = 0.15
	// thumbnailFallbackSeconds is used when duration is unknown.
	thumbnailFallbackSeconds = 10
	// thumbnailTimeout bounds one capture. Seeking and decoding a single frame
	// is fast even on a large file.
	thumbnailTimeout = 60 * time.Second
)

// jpegMagic is the SOI marker every JPEG starts with.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF}

// ThumbnailDir is where generated covers live: under the app data directory,
// never beside the user's media. Writing into someone's video folders is a
// surprise, and it makes the whole feature impossible to cleanly undo.
func (c *Catalog) ThumbnailDir() string {
	return filepath.Join(c.db.DataDir(), "thumbnails")
}

// thumbnailPath resolves one cover. The identifier is checked against the
// catalog's own ID alphabet before it becomes a path element, so a malformed or
// hostile ID cannot escape the thumbnail directory.
func (c *Catalog) thumbnailPath(id string) (string, error) {
	if !validVideoID(id) {
		return "", ErrVideoNotFound
	}
	return filepath.Join(c.ThumbnailDir(), id+".jpg"), nil
}

// validVideoID accepts only what stableVideoID produces: lowercase hex.
func validVideoID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, char := range id {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// SaveThumbnail stores a cover the browser captured and marks the row. The
// payload is validated as a JPEG rather than trusted: this endpoint writes a
// file to disk, so "the client said it was an image" is not enough.
func (c *Catalog) SaveThumbnail(ctx context.Context, id string, image []byte) error {
	if len(image) == 0 || len(image) > MaxThumbnailBytes || !bytes.HasPrefix(image, jpegMagic) {
		return ErrThumbnailInvalid
	}
	path, err := c.thumbnailPath(id)
	if err != nil {
		return err
	}
	if _, err := c.Video(ctx, id); err != nil {
		return err
	}
	if err := os.MkdirAll(c.ThumbnailDir(), 0o700); err != nil {
		return fmt.Errorf("create thumbnail directory: %w", err)
	}
	// Written to a temporary name and renamed, so a reader never sees a
	// half-written file and a failed write leaves the previous cover intact.
	temporary := path + ".partial"
	if err := os.WriteFile(temporary, image, 0o600); err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("finalize thumbnail: %w", err)
	}
	return c.markThumbnailGenerated(ctx, id)
}

func (c *Catalog) markThumbnailGenerated(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return c.db.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx,
			`UPDATE media_videos SET thumbnail_generated_at = ? WHERE id = ?`, now, id)
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

// OpenThumbnail opens a stored cover for serving.
func (c *Catalog) OpenThumbnail(ctx context.Context, id string) (*os.File, error) {
	video, err := c.Video(ctx, id)
	if err != nil {
		return nil, err
	}
	if video.ThumbnailGeneratedAt == nil {
		return nil, ErrThumbnailNotFound
	}
	path, err := c.thumbnailPath(id)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path) // #nosec G304 -- path built from a validated catalog ID.
	if err != nil {
		return nil, ErrThumbnailNotFound
	}
	return file, nil
}

// ClearThumbnails removes every generated cover and forgets that any existed,
// making the whole storage cost recoverable in one step.
func (c *Catalog) ClearThumbnails(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(c.ThumbnailDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("read thumbnail directory: %w", err)
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(c.ThumbnailDir(), entry.Name())); err == nil {
			removed++
		}
	}
	err = c.db.WithTx(ctx, func(tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx, `UPDATE media_videos SET thumbnail_generated_at = NULL`)
		return execErr
	})
	return removed, err
}

// deleteThumbnail removes one cover, ignoring an already-absent file.
func (c *Catalog) deleteThumbnail(id string) {
	if path, err := c.thumbnailPath(id); err == nil {
		_ = os.Remove(path)
	}
}

// generateThumbnail captures one frame with FFmpeg, for videos the browser
// cannot reach: those never opened, and those it cannot decode at all.
func (c *Catalog) generateThumbnail(ctx context.Context, tools Tools, video Video) error {
	sourcePath, err := c.resolveVideoPath(video)
	if err != nil {
		return err
	}
	path, err := c.thumbnailPath(video.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.ThumbnailDir(), 0o700); err != nil {
		return fmt.Errorf("create thumbnail directory: %w", err)
	}
	offset := c.captureOffsetSeconds(ctx, tools, video, sourcePath)
	if err := captureFrame(ctx, tools, sourcePath, path, offset); err != nil {
		// A seek can still land past the last keyframe of a short or oddly
		// indexed file, and FFmpeg then writes nothing at all. Frame zero is a
		// worse thumbnail than one taken further in, and a far better one than
		// none.
		if offset == "0" {
			return err
		}
		if retryErr := captureFrame(ctx, tools, sourcePath, path, "0"); retryErr != nil {
			return retryErr
		}
	}
	return c.markThumbnailGenerated(ctx, video.ID)
}

func captureFrame(ctx context.Context, tools Tools, sourcePath, targetPath, offsetSeconds string) error {
	temporary := targetPath + ".partial"
	_ = os.Remove(temporary)
	if _, err := runTool(ctx, thumbnailTimeout, tools.FFmpegPath,
		thumbnailFFmpegArgs(sourcePath, temporary, offsetSeconds)...,
	); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, targetPath); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("finalize thumbnail: %w", err)
	}
	return nil
}

func thumbnailFFmpegArgs(sourcePath, targetPath, offsetSeconds string) []string {
	return []string{
		"-nostdin",
		"-loglevel", "error",
		// Seeking before -i is the fast form: FFmpeg jumps to the keyframe
		// instead of decoding everything up to that point.
		"-ss", offsetSeconds,
		"-i", sourcePath,
		"-frames:v", "1",
		// Downscale only when the source is larger, and keep the aspect ratio
		// on an even height so the JPEG encoder accepts the result. Spline is a
		// high-quality compromise that rings less than Lanczos around hard edges.
		"-vf", fmt.Sprintf("scale='min(%d,iw)':-2:flags=spline", thumbnailMaxEdge),
		// Muxer and encoder are both named. Output goes to a ".partial" path,
		// and FFmpeg would otherwise try to infer them from that extension.
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", strconv.Itoa(thumbnailJPEGQuality),
		"-y", targetPath,
	}
}

// captureOffsetSeconds picks a frame a fraction of the way in.
//
// The duration has to be right, not guessed: a fixed fallback offset seeks past
// the end of anything shorter, and FFmpeg then reports only that no packets
// arrived. Most rows have no duration yet — it is written by the browser after
// playback, and these are exactly the videos nobody has played — so ffprobe is
// asked, and the answer is kept so nothing has to ask twice.
func (c *Catalog) captureOffsetSeconds(ctx context.Context, tools Tools, video Video, sourcePath string) string {
	durationMillis := int64(0)
	if video.DurationMillis != nil {
		durationMillis = *video.DurationMillis
	}
	if durationMillis <= 0 {
		if info, err := tools.Inspect(ctx, sourcePath); err == nil && info.DurationMillis > 0 {
			durationMillis = info.DurationMillis
			if setErr := c.SetDuration(ctx, video.ID, durationMillis); setErr != nil {
				c.logger.Warn("probed duration could not be stored", "video_id", video.ID, "error", setErr)
			}
		}
	}
	if durationMillis <= 0 {
		return strconv.Itoa(thumbnailFallbackSeconds)
	}
	seconds := float64(durationMillis) / 1000 * thumbnailCaptureFraction
	// Below a second in, seeking buys nothing over frame zero.
	if seconds < 1 {
		return "0"
	}
	return strconv.FormatFloat(seconds, 'f', 2, 64)
}
