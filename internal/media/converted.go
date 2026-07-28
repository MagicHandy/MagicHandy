package media

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"
)

// convertedTargetPath resolves where a converted file is written: beside its
// source, inside the same registered location. The jail is revalidated here
// rather than trusted from the row, because this path becomes an argument to an
// external process that will create a file at it.
func (c *Catalog) convertedTargetPath(video Video) (string, error) {
	root, cleanRelative, _, err := validateRootedPath(video.LocationPath, video.RelativePath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrVideoUnavailable, err)
	}
	directory := filepath.Dir(filepath.Join(root, cleanRelative))
	target := filepath.Join(directory, ConvertedName(video.RelativePath))
	if !pathWithin(root, target) {
		return "", ErrVideoUnavailable
	}
	return target, nil
}

func directoryOf(path string) string { return filepath.Dir(path) }

// convertedRelativePath is the new file's path relative to the same root.
func convertedRelativePath(relativePath string) string {
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relativePath)))
	name := ConvertedName(relativePath)
	if directory == "." || directory == "" {
		return name
	}
	return directory + "/" + name
}

// adoptConvertedFile catalogs the output and retires the source in one
// transaction, so the library never shows the conversion half-applied.
//
// Two things are carried across deliberately:
//
//   - The paired script, because the pairing is a property of the content and
//     the content did not change.
//   - The per-video sync offset, because conversion moves no timestamp and the
//     script is the same file, so its bias is identical. Making someone
//     re-calibrate a file they just converted is a small, entirely avoidable
//     annoyance.
//
// The source row is hidden rather than deleted. Nothing is removed from disk,
// and deleting the converted file makes the original reappear on the next scan.
func (c *Catalog) adoptConvertedFile(ctx context.Context, source Video, info StreamInfo) error {
	relative := convertedRelativePath(source.RelativePath)
	id := stableVideoID(source.LocationPath, relative)
	target, err := c.convertedTargetPath(source)
	if err != nil {
		return err
	}
	stat, err := osStat(target)
	if err != nil {
		return fmt.Errorf("converted file could not be read back: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	displayName := baseStem(relative)

	return c.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_videos(
				id, location_path, relative_path, display_name, size_bytes, modified_at,
				duration_ms, funscript_relative_path, missing, scanned_at, script_offset_ms,
				compatibility, video_codec, audio_codec, superseded
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, 0)
			ON CONFLICT(id) DO UPDATE SET
				size_bytes = excluded.size_bytes,
				modified_at = excluded.modified_at,
				funscript_relative_path = excluded.funscript_relative_path,
				missing = 0,
				superseded = 0,
				scanned_at = excluded.scanned_at
		`,
			id, source.LocationPath, relative, displayName, stat.Size(),
			stat.ModTime().UTC().Format(time.RFC3339Nano), nullableInt64(source.DurationMillis),
			nullableString(source.FunscriptRelativePath), now, source.ScriptOffsetMillis,
			// The output is not asserted playable: it has not been played yet,
			// and unknown is the honest state until the browser says otherwise.
			string(CompatibilityUnknown), info.VideoCodec, info.AudioCodec,
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE media_videos SET superseded = 1 WHERE id = ?`, source.ID)
		return err
	})
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
