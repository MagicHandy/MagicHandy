package media

import (
	"context"
	"strings"
)

// A bulk edit returns its acknowledged rows with two bounded reads, rather
// than a row query and a tag query for each of up to 500 videos.
func (c *Catalog) metadataVideos(ctx context.Context, ids []string) ([]Video, error) {
	placeholders, args := metadataIDs(ids)
	rows, err := c.db.SQL().QueryContext(ctx, `SELECT `+videoColumns+` FROM media_videos WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[string]Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	videos := make([]Video, 0, len(ids))
	for _, id := range ids {
		video, ok := byID[id]
		if !ok {
			return nil, ErrVideoNotFound
		}
		videos = append(videos, video)
	}
	if err := c.attachTagsFor(ctx, videos); err != nil {
		return nil, err
	}
	return videos, nil
}

func metadataIDs(ids []string) (string, []any) {
	args := make([]any, len(ids))
	for index, id := range ids {
		args[index] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}
