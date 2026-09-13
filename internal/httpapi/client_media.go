package httpapi

import (
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/media"
)

func (s *Server) clientMediaState(r *http.Request) map[string]any {
	state := s.mediaState(r.Context())
	if !s.capabilities(r).ConfigureHost {
		if scan, ok := state["scan"].(media.ScanState); ok {
			state["scan"] = s.clientScanState(r, scan)
		}
		if job, ok := state["job"].(media.JobState); ok {
			state["job"] = s.clientJobState(r, job)
		}
		if sync, ok := state["sync"].(mediaSyncStatus); ok {
			state["sync"] = s.clientSyncStatus(r, sync)
		}
	}
	return state
}

func (s *Server) clientScanState(r *http.Request, state media.ScanState) media.ScanState {
	if s.capabilities(r).ConfigureHost {
		return state
	}
	return media.ScanState{
		Running: state.Running, Trigger: state.Trigger, Cancelled: state.Cancelled,
		StartedAt: state.StartedAt, CompletedAt: state.CompletedAt, FilesVisited: state.FilesVisited, VideosFound: state.VideosFound,
		Summary: media.ScanSummary{Locations: state.Summary.Locations, Added: state.Summary.Added, Updated: state.Summary.Updated, Missing: state.Summary.Missing, Removed: state.Summary.Removed, Skipped: state.Summary.Skipped, Issues: []media.ScanIssue{}},
		Error:   clientFailure(state.Error),
	}
}

func (s *Server) clientJobState(r *http.Request, state media.JobState) media.JobState {
	if s.capabilities(r).ConfigureHost {
		return state
	}
	return media.JobState{
		Kind: state.Kind, Running: state.Running, Cancelled: state.Cancelled,
		StartedAt: state.StartedAt, CompletedAt: state.CompletedAt,
		Total: state.Total, Processed: state.Processed, Succeeded: state.Succeeded, Failed: state.Failed,
		ItemPercent: state.ItemPercent, Issues: []media.JobIssue{}, Error: clientFailure(state.Error),
	}
}

func (s *Server) clientVideos(r *http.Request, videos []media.Video) []media.Video {
	if s.capabilities(r).ConfigureHost {
		return videos
	}
	views := make([]media.Video, 0, len(videos))
	for _, video := range videos {
		views = append(views, media.Video{
			ID: video.ID, DisplayName: video.DisplayName, SizeBytes: video.SizeBytes, ModifiedAt: video.ModifiedAt,
			DurationMillis: video.DurationMillis, HasFunscript: video.HasFunscript, Missing: video.Missing,
			ScannedAt: video.ScannedAt, ScriptOffsetMillis: video.ScriptOffsetMillis, ThumbnailGeneratedAt: video.ThumbnailGeneratedAt,
			Compatibility: video.Compatibility, VideoCodec: video.VideoCodec, AudioCodec: video.AudioCodec,
			Superseded: video.Superseded, ContainerType: video.ContainerType,
		})
	}
	return views
}
