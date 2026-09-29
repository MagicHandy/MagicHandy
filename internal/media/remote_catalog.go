package media

import (
	"context"
	"strings"
)

// RemoteVideo is deliberately display-only. Paths, notes, file sizes, scan
// state and tool details have no place in a playback remote's catalog.
type RemoteVideo struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	DurationMillis *int64 `json:"duration_ms"`
	HasFunscript   bool   `json:"has_funscript"`
}

// RemoteVideos keeps both materialized rows and each response bounded. The
// display-name match uses Go's Unicode fold, as does the main library. Tag keys
// are already normalized in storage. A page is one query, without per-row reads.
func (c *Catalog) RemoteVideos(ctx context.Context, query string, offset int) ([]RemoteVideo, bool, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	rows, err := c.db.SQL().QueryContext(ctx, `SELECT v.id, COALESCE(NULLIF(v.title,''), v.display_name),
		v.duration_ms, v.funscript_relative_path IS NOT NULL,
		EXISTS(SELECT 1 FROM media_video_tags t WHERE t.video_id = v.id AND instr(t.tag_key, ?) > 0)
		FROM media_videos v WHERE v.missing = 0 AND v.superseded = 0
		ORDER BY COALESCE(NULLIF(v.title,''), v.display_name) COLLATE NOCASE, v.id`, query)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]RemoteVideo, 0, 60)
	for rows.Next() {
		var item RemoteVideo
		var tagged bool
		if err := rows.Scan(&item.ID, &item.Title, &item.DurationMillis, &item.HasFunscript, &tagged); err != nil {
			return nil, false, err
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Title), query) && !tagged {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		if len(items) == 60 {
			return items, true, nil
		}
		items = append(items, item)
	}
	return items, false, rows.Err()
}
