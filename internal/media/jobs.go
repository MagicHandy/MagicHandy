package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// ErrJobBusy reports a second media job while one is running. One at a time,
// like scans: two long media jobs at once is a way to make the app feel broken.
var ErrJobBusy = errors.New("a media job is already running")

// JobKind names the long media tasks. Both run through the same runner so
// progress, cancellation, and the one-at-a-time rule cannot drift apart.
type JobKind string

const (
	// JobThumbnails generates covers FFmpeg has to produce.
	JobThumbnails JobKind = "thumbnails"
	// JobConversion repairs files that cannot play.
	JobConversion JobKind = "conversion"
)

// JobIssue is one file that could not be processed. A job continues past a
// failure: one unreadable file should not abandon the other nine hundred.
type JobIssue struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// JobState is safe to poll while a job runs.
type JobState struct {
	Kind        JobKind `json:"kind,omitempty"`
	Running     bool    `json:"running"`
	Cancellable bool    `json:"cancellable"`
	Cancelled   bool    `json:"cancelled"`
	StartedAt   string  `json:"started_at,omitempty"`
	CompletedAt string  `json:"completed_at,omitempty"`
	CurrentName string  `json:"current_name,omitempty"`
	Total       int     `json:"total"`
	Processed   int     `json:"processed"`
	Succeeded   int     `json:"succeeded"`
	Failed      int     `json:"failed"`
	// ItemPercent is progress within the current file. Conversion reports it;
	// thumbnail capture is fast enough that it stays at zero.
	ItemPercent int        `json:"item_percent"`
	Issues      []JobIssue `json:"issues"`
	Error       string     `json:"error,omitempty"`
}

func emptyJobState() JobState {
	return JobState{Issues: []JobIssue{}}
}

func cloneJobState(state JobState) JobState {
	state.Issues = append([]JobIssue{}, state.Issues...)
	return state
}

// JobState returns a race-safe progress snapshot.
func (c *Catalog) JobState() JobState {
	c.jobMu.Lock()
	defer c.jobMu.Unlock()
	return cloneJobState(c.jobState)
}

// CancelJob requests cancellation. State stays running until the worker has
// stopped touching the filesystem and database.
func (c *Catalog) CancelJob() JobState {
	c.jobMu.Lock()
	defer c.jobMu.Unlock()
	if c.jobCancel != nil {
		c.jobCancel()
	}
	return cloneJobState(c.jobState)
}

// beginJob claims the single job slot.
func (c *Catalog) beginJob(kind JobKind, total int) (context.Context, error) {
	c.jobMu.Lock()
	defer c.jobMu.Unlock()
	if c.closed.Load() {
		return nil, ErrClosed
	}
	if c.jobState.Running {
		return nil, ErrJobBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.jobCancel = cancel
	c.jobDone = make(chan struct{})
	c.jobState = JobState{
		Kind:        kind,
		Running:     true,
		Cancellable: true,
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Total:       total,
		Issues:      []JobIssue{},
	}
	return ctx, nil
}

// WaitForJob blocks until the running job finishes, or returns immediately when
// none is running. It exists for the scan follow-up, which has to let the
// thumbnail pass finish before asking for the single job slot again.
func (c *Catalog) WaitForJob() {
	c.jobMu.Lock()
	done := c.jobDone
	running := c.jobState.Running
	c.jobMu.Unlock()
	if running && done != nil {
		<-done
	}
}

func (c *Catalog) finishJob(runErr error) {
	c.jobMu.Lock()
	defer c.jobMu.Unlock()
	c.jobState.Running = false
	c.jobState.Cancellable = false
	c.jobState.CurrentName = ""
	c.jobState.ItemPercent = 0
	c.jobState.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	c.jobState.Cancelled = errors.Is(runErr, context.Canceled)
	if runErr != nil && !c.jobState.Cancelled {
		c.jobState.Error = runErr.Error()
	}
	c.jobCancel = nil
	if c.jobDone != nil {
		close(c.jobDone)
		c.jobDone = nil
	}
}

func (c *Catalog) startItem(name string) {
	c.jobMu.Lock()
	c.jobState.CurrentName = name
	c.jobState.ItemPercent = 0
	c.jobMu.Unlock()
}

func (c *Catalog) reportItemPercent(percent int) {
	c.jobMu.Lock()
	c.jobState.ItemPercent = percent
	c.jobMu.Unlock()
}

func (c *Catalog) finishItem(name string, err error) {
	c.jobMu.Lock()
	defer c.jobMu.Unlock()
	c.jobState.Processed++
	c.jobState.ItemPercent = 0
	if err == nil {
		c.jobState.Succeeded++
		return
	}
	c.jobState.Failed++
	// Bounded: a job over a large library with a systemic problem would
	// otherwise accumulate one issue per file and ship them all to the client.
	const maxJobIssues = 20
	if len(c.jobState.Issues) < maxJobIssues {
		c.jobState.Issues = append(c.jobState.Issues, JobIssue{Name: name, Message: err.Error()})
	}
}

// StartThumbnailJob generates covers for videos the browser cannot reach:
// those never opened, and those it cannot decode at all.
func (c *Catalog) StartThumbnailJob(ctx context.Context, tools Tools, redo bool) (JobState, error) {
	videos, err := c.List(ctx)
	if err != nil {
		return JobState{}, err
	}
	pending := make([]Video, 0, len(videos))
	for _, video := range videos {
		if video.Missing || video.Superseded {
			continue
		}
		if video.ThumbnailGeneratedAt != nil && !redo {
			continue
		}
		pending = append(pending, video)
	}
	jobCtx, err := c.beginJob(JobThumbnails, len(pending))
	if err != nil {
		return c.JobState(), err
	}
	c.jobWG.Add(1)
	go c.runThumbnailJob(jobCtx, tools, pending)
	return c.JobState(), nil
}

func (c *Catalog) runThumbnailJob(ctx context.Context, tools Tools, pending []Video) {
	defer c.jobWG.Done()
	var runErr error
	for _, video := range pending {
		if err := ctx.Err(); err != nil {
			runErr = err
			break
		}
		c.startItem(video.DisplayName)
		c.finishItem(video.DisplayName, c.generateThumbnail(ctx, tools, video))
	}
	c.finishJob(runErr)
	state := c.JobState()
	c.logger.Info("media thumbnail job finished",
		"cancelled", state.Cancelled, "total", state.Total,
		"succeeded", state.Succeeded, "failed", state.Failed)
}

// StartConversionJob repairs files that cannot play. Passing no identifiers
// sweeps the library.
//
// Both entry points gate on the same NeedsConversion test rather than
// re-deriving it, because a sweep that re-encoded a playable library would be
// the single worst thing this feature could do.
func (c *Catalog) StartConversionJob(
	ctx context.Context, tools Tools, settings config.MediaSettings, ids []string,
) (JobState, error) {
	pending, err := c.conversionCandidates(ctx, ids)
	if err != nil {
		return JobState{}, err
	}
	jobCtx, err := c.beginJob(JobConversion, len(pending))
	if err != nil {
		return c.JobState(), err
	}
	c.jobWG.Add(1)
	go c.runConversionJob(jobCtx, tools, settings, pending)
	return c.JobState(), nil
}

func (c *Catalog) conversionCandidates(ctx context.Context, ids []string) ([]Video, error) {
	selected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		selected[id] = struct{}{}
	}
	videos, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	pending := make([]Video, 0, len(videos))
	for _, video := range videos {
		if len(selected) > 0 {
			if _, ok := selected[video.ID]; !ok {
				continue
			}
		}
		// A file that already plays is never converted, by any path. The
		// suffix check keeps the app from reprocessing its own output.
		if !video.NeedsConversion() || HasConvertedSuffix(video.RelativePath) {
			continue
		}
		pending = append(pending, video)
	}
	if len(pending) == 0 {
		return nil, errors.New("no files need conversion")
	}
	return pending, nil
}

func (c *Catalog) runConversionJob(
	ctx context.Context, tools Tools, settings config.MediaSettings, pending []Video,
) {
	defer c.jobWG.Done()
	var runErr error
	for _, video := range pending {
		if err := ctx.Err(); err != nil {
			runErr = err
			break
		}
		c.startItem(video.DisplayName)
		err := c.convertVideo(ctx, tools, settings, video)
		if errors.Is(err, context.Canceled) {
			runErr = err
			break
		}
		c.finishItem(video.DisplayName, err)
	}
	c.finishJob(runErr)
	state := c.JobState()
	c.logger.Info("media conversion job finished",
		"cancelled", state.Cancelled, "total", state.Total,
		"succeeded", state.Succeeded, "failed", state.Failed)
}

// conversionSpaceFactor estimates output size against the source. A remux is
// about the same size and a re-encode is normally smaller, so requiring the
// source size again is a deliberately generous floor that still catches the
// case that matters: a disk with no real room left.
const conversionSpaceFactor = 1.1

// convertVideo probes, plans, converts, and catalogs the result.
func (c *Catalog) convertVideo(
	ctx context.Context, tools Tools, settings config.MediaSettings, video Video,
) error {
	sourcePath, err := c.resolveVideoPath(video)
	if err != nil {
		return err
	}
	info, err := tools.Inspect(ctx, sourcePath)
	if err != nil {
		return err
	}
	// Record the codecs before doing any work. If the conversion then fails,
	// the library can still explain what the file actually contains.
	if probeErr := c.setProbeCodecs(ctx, video.ID, info); probeErr != nil {
		c.logger.Warn("probe result could not be stored", "video_id", video.ID, "error", probeErr)
	}
	plan := PlanConversion(info, settings)
	targetPath, err := c.convertedTargetPath(video)
	if err != nil {
		return err
	}
	if err := checkFreeSpace(targetPath, video.SizeBytes); err != nil {
		return err
	}
	total := info.DurationMillis
	err = tools.runConversion(ctx, plan, sourcePath, targetPath, settings, func(progress ConversionProgress) {
		if total > 0 {
			c.reportItemPercent(int(min(progress.ProcessedMillis*100/total, 100)))
		}
	})
	if err != nil {
		return err
	}
	// Probe the output rather than reusing the source's stream info. A
	// re-encode changes the codecs by definition, and carrying the source's
	// across would leave the new row claiming the very codec it was converted
	// away from — which the UI would then name as the reason it will not play.
	converted := info
	if probed, probeErr := tools.Inspect(ctx, targetPath); probeErr == nil {
		converted = probed
	} else {
		// Unknown beats wrong: an unprobeable output has no codecs to report.
		converted.VideoCodec = ""
		converted.AudioCodec = ""
		c.logger.Warn("converted file could not be probed",
			"video_id", video.ID, "error", probeErr)
	}
	return c.adoptConvertedFile(ctx, video, converted)
}

func checkFreeSpace(targetPath string, sourceBytes int64) error {
	free, err := availableBytes(directoryOf(targetPath))
	if err != nil {
		// An unreadable volume is not a reason to refuse the work; FFmpeg will
		// report a real out-of-space failure if one happens.
		return nil //nolint:nilerr // best-effort precheck.
	}
	required := uint64(float64(sourceBytes) * conversionSpaceFactor)
	if free < required {
		return fmt.Errorf("not enough free space: %d MB available, about %d MB needed",
			free/(1<<20), required/(1<<20))
	}
	return nil
}
