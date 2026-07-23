package manualqueue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	playerChunkSize     = 40
	playerLeadMS        = 650
	playerDispatchTick  = 90 * time.Millisecond
	playerStreamPrefix  = "manual-queue"
	playerFinishTailMS  = 500
)

// hspAbsoluteBatch maps compacted session points onto the monotonic HSP stream timeline.
func hspAbsoluteBatch(baseMS int, batch []transport.TimedPoint) []transport.TimedPoint {
	if baseMS == 0 || len(batch) == 0 {
		return batch
	}
	out := make([]transport.TimedPoint, len(batch))
	offset := int64(baseMS)
	for index, point := range batch {
		out[index] = transport.TimedPoint{
			PositionPercent: point.PositionPercent,
			TimeMillis:      point.TimeMillis + offset,
		}
	}
	return out
}

// Session is the prepared script payload for playback.
type Session struct {
	Actions       []Action
	Points        []transport.TimedPoint
	DurationMS    int
	SegmentStarts []int
	Autoloop      bool
	Continuous    bool // streaming append mode; do not auto-finish when timeline ends
	StrokeMin     int
	StrokeMax     int
}

// Snapshot is a thread-safe player status view.
type Snapshot struct {
	Running        bool
	Paused         bool
	PlayheadMS     int
	DurationMS     int
	PositionPct    float64
	CurrentSegment int
	Autoloop       bool
}

// Player streams a prepared manual queue script through HSP.
type Player struct {
	mu sync.Mutex

	transport transport.Transport
	session   Session
	streamID  string

	running    bool
	paused     bool
	pauseExit  bool
	playheadMS int
	startedAt  time.Time
	baseMS     int

	cancel context.CancelFunc
	done   chan struct{}

	nextIndex int
	dispatchDebug func(event string, data map[string]any)

	onFinished func()
}

// NewPlayer creates a manual queue script player.
func NewPlayer(commandTransport transport.Transport) *Player {
	return &Player{transport: commandTransport}
}

// SetOnFinished registers a callback when playback ends normally.
func (p *Player) SetOnFinished(fn func()) {
	p.mu.Lock()
	p.onFinished = fn
	p.mu.Unlock()
}

// SetDispatchDebug registers a callback for player dispatch diagnostics.
func (p *Player) SetDispatchDebug(fn func(event string, data map[string]any)) {
	p.mu.Lock()
	p.dispatchDebug = fn
	p.mu.Unlock()
}

// Start begins playback from playheadMS (usually 0).
func (p *Player) Start(ctx context.Context, session Session, playheadMS int) error {
	if p.transport == nil {
		return fmt.Errorf("motion transport is unavailable")
	}
	if len(session.Points) == 0 {
		return fmt.Errorf("manual queue script is empty")
	}
	p.Stop(context.Background())
	return p.startLoop(ctx, session, playheadMS)
}

func (p *Player) startLoop(ctx context.Context, session Session, playheadMS int) error {
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})

	p.mu.Lock()
	p.session = session
	p.streamID = fmt.Sprintf("%s-%d", playerStreamPrefix, time.Now().UnixNano())
	p.running = true
	p.paused = false
	p.pauseExit = false
	p.playheadMS = playheadMS
	p.baseMS = playheadMS
	p.startedAt = time.Now()
	p.cancel = cancel
	p.done = done
	p.nextIndex = 0
	p.mu.Unlock()

	go p.runLoop(loopCtx)
	return nil
}

// Pause stops device motion while keeping the session alive.
func (p *Player) Pause(ctx context.Context) error {
	p.mu.Lock()
	if !p.running || p.paused {
		p.mu.Unlock()
		return nil
	}
	elapsed := p.elapsedMSLocked()
	p.playheadMS = elapsed
	p.baseMS = elapsed
	p.paused = true
	p.pauseExit = true
	cancel := p.cancel
	done := p.done
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	_, err := p.transport.Stop(ctx, transport.StopCommand{Reason: "manual_queue_pause"})
	return err
}

// Resume continues from the saved playhead.
func (p *Player) Resume(ctx context.Context) error {
	p.mu.Lock()
	if !p.running || !p.paused {
		p.mu.Unlock()
		return fmt.Errorf("manual queue player is not paused")
	}
	session := p.session
	playhead := p.playheadMS
	p.paused = false
	p.pauseExit = false
	p.mu.Unlock()

	return p.startLoop(ctx, session, playhead)
}

// Stop cancels playback and clears state.
func (p *Player) Stop(ctx context.Context) {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	cancel := p.cancel
	done := p.done
	p.running = false
	p.paused = false
	p.cancel = nil
	p.done = nil
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	_, _ = p.transport.Stop(ctx, transport.StopCommand{Reason: "manual_queue_stop"})
}

// SkipSegment advances to the next queue segment.
func (p *Player) SkipSegment(ctx context.Context) (int, error) {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return 0, fmt.Errorf("manual queue player is not active")
	}
	session := p.session
	playhead := p.elapsedMSLocked()
	segment := SegmentIndexAt(session.SegmentStarts, playhead)
	skipTo := session.DurationMS
	if segment+1 < len(session.SegmentStarts) {
		skipTo = session.SegmentStarts[segment+1]
	}
	autoloop := session.Autoloop
	p.mu.Unlock()

	p.Stop(ctx)
	if skipTo >= session.DurationMS {
		return skipTo, nil
	}
	session.Autoloop = autoloop
	if err := p.Start(ctx, session, skipTo); err != nil {
		return skipTo, err
	}
	return skipTo, nil
}

// Snapshot returns the current player status.
func (p *Player) Snapshot() Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	snap := Snapshot{
		Running:    p.running,
		Paused:     p.paused,
		PlayheadMS: p.playheadMS,
		DurationMS: p.session.DurationMS,
		Autoloop:   p.session.Autoloop,
	}
	if p.running && !p.paused {
		snap.PlayheadMS = p.localPlayheadMSLocked()
	}
	if len(p.session.Actions) > 0 {
		snap.PositionPct = PositionAt(p.session.Actions, snap.PlayheadMS)
		snap.CurrentSegment = SegmentIndexAt(p.session.SegmentStarts, snap.PlayheadMS)
	}
	return snap
}

// AppendExtension queues additional points after the current session timeline.
// Safe while the player is running; extension times are shifted by the pre-append duration.
func (p *Player) AppendExtension(extension Session) error {
	if len(extension.Points) == 0 {
		return fmt.Errorf("extension has no points")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running || p.paused {
		return fmt.Errorf("manual queue player is not running")
	}

	// Do not compact here — runLoop already compacts, and compacting during append
	// jumps baseMS past the shortened DurationMS and kills playback.
	offset := int64(p.session.DurationMS)
	if n := len(p.session.Points); n > 0 {
		if tail := p.session.Points[n-1].TimeMillis + 1; tail > offset {
			offset = tail
		}
	}
	localPlayhead := p.localPlayheadMSLocked()
	minOffset := int64(localPlayhead + playerLeadMS)
	if offset < minOffset {
		offset = minOffset
	}
	newPoints := make([]transport.TimedPoint, len(extension.Points))
	for index, point := range extension.Points {
		newPoints[index] = transport.TimedPoint{
			TimeMillis:      point.TimeMillis + offset,
			PositionPercent: point.PositionPercent,
		}
	}
	newActions := make([]Action, len(extension.Actions))
	for index, action := range extension.Actions {
		newActions[index] = Action{
			At:  action.At + int(offset),
			Pos: action.Pos,
		}
	}

	p.session.Points = append(p.session.Points, newPoints...)
	p.session.Actions = append(p.session.Actions, newActions...)
	if len(extension.SegmentStarts) > 0 {
		for _, start := range extension.SegmentStarts {
			p.session.SegmentStarts = append(p.session.SegmentStarts, int(offset)+start)
		}
	} else {
		p.session.SegmentStarts = append(p.session.SegmentStarts, int(offset))
	}
	p.session.DurationMS += extension.DurationMS
	return nil
}

// SpliceExtensionAtPlayhead drops queued points after playhead+leadMS and splices new motion there.
// Used when chat chaos retargets zone/tempo without waiting for bridge filler to drain.
func (p *Player) SpliceExtensionAtPlayhead(extension Session, leadMS int) error {
	if len(extension.Points) == 0 {
		return fmt.Errorf("extension has no points")
	}
	if leadMS < 0 {
		leadMS = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running || p.paused {
		return fmt.Errorf("manual queue player is not running")
	}

	localPlayhead := p.localPlayheadMSLocked()
	spliceAt := localPlayhead + leadMS

	cutIdx := len(p.session.Points)
	for index, point := range p.session.Points {
		if int(point.TimeMillis) > spliceAt {
			cutIdx = index
			break
		}
	}
	p.session.Points = append([]transport.TimedPoint(nil), p.session.Points[:cutIdx]...)

	trimmedActions := make([]Action, 0, len(p.session.Actions))
	for _, action := range p.session.Actions {
		if action.At <= spliceAt {
			trimmedActions = append(trimmedActions, action)
		}
	}
	p.session.Actions = trimmedActions

	if len(p.session.Points) > 0 {
		p.session.DurationMS = int(p.session.Points[len(p.session.Points)-1].TimeMillis)
	} else {
		p.session.DurationMS = spliceAt
	}
	if p.nextIndex > cutIdx {
		p.nextIndex = cutIdx
	}

	offset := int64(spliceAt)
	if len(p.session.Points) > 0 {
		if tail := p.session.Points[len(p.session.Points)-1].TimeMillis + 1; tail > offset {
			offset = tail
		}
	}

	newPoints := make([]transport.TimedPoint, len(extension.Points))
	for index, point := range extension.Points {
		newPoints[index] = transport.TimedPoint{
			TimeMillis:      point.TimeMillis + offset,
			PositionPercent: point.PositionPercent,
		}
	}
	newActions := make([]Action, len(extension.Actions))
	for index, action := range extension.Actions {
		newActions[index] = Action{
			At:  action.At + int(offset),
			Pos: action.Pos,
		}
	}

	p.session.Points = append(p.session.Points, newPoints...)
	p.session.Actions = append(p.session.Actions, newActions...)
	if len(p.session.Points) > 0 {
		p.session.DurationMS = int(p.session.Points[len(p.session.Points)-1].TimeMillis)
	}
	return nil
}

// compactTimelineLocked drops dispatched points and shifts the remaining timeline to t=0.
// Caller must hold p.mu.
func (p *Player) compactTimelineLocked() {
	const keepMargin = 2
	if p.nextIndex <= keepMargin || len(p.session.Points) <= keepMargin {
		return
	}
	keepFrom := p.nextIndex - keepMargin
	if keepFrom <= 0 || keepFrom >= len(p.session.Points) {
		return
	}
	shift := p.session.Points[keepFrom].TimeMillis
	if shift <= 0 {
		return
	}

	p.session.Points = append([]transport.TimedPoint(nil), p.session.Points[keepFrom:]...)
	for index := range p.session.Points {
		p.session.Points[index].TimeMillis -= shift
	}

	trimmedActions := make([]Action, 0, len(p.session.Actions))
	cutoff := int(shift)
	for _, action := range p.session.Actions {
		if action.At >= cutoff {
			trimmedActions = append(trimmedActions, Action{
				At:  action.At - cutoff,
				Pos: action.Pos,
			})
		}
	}
	p.session.Actions = trimmedActions

	if p.session.DurationMS > int(shift) {
		p.session.DurationMS -= int(shift)
	} else {
		p.session.DurationMS = 0
	}
	if n := len(p.session.Points); n > 0 {
		if last := int(p.session.Points[n-1].TimeMillis); last > p.session.DurationMS {
			p.session.DurationMS = last
		}
	}
	p.baseMS += int(shift)
	p.nextIndex = keepMargin
}

// Running reports whether playback is active.
func (p *Player) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running && !p.paused
}

func (p *Player) localPlayheadMSLocked() int {
	return p.elapsedMSLocked() - p.baseMS
}

// TimelineEndMS returns the current session duration in milliseconds.
func (p *Player) TimelineEndMS() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	timelineEnd := p.session.DurationMS
	if timelineEnd <= 0 {
		timelineEnd = DurationMS(p.session.Actions)
	}
	if n := len(p.session.Points); n > 0 {
		if last := int(p.session.Points[n-1].TimeMillis); last > timelineEnd {
			timelineEnd = last
		}
	}
	return timelineEnd
}

// Actions returns the active script actions for UI curves.
func (p *Player) Actions() []Action {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Action(nil), p.session.Actions...)
}

func (p *Player) runLoop(ctx context.Context) {
	p.mu.Lock()
	done := p.done
	p.mu.Unlock()
	defer func() {
		if done != nil {
			close(done)
		}
	}()

	p.mu.Lock()
	session := p.session
	streamID := p.streamID
	autoloop := p.session.Autoloop
	p.mu.Unlock()

	if _, err := p.transport.SetStrokeWindow(ctx, transport.StrokeWindowCommand{
		MinPercent: session.StrokeMin,
		MaxPercent: session.StrokeMax,
	}); err != nil {
		p.finish(false)
		return
	}
	_, _ = p.transport.Stop(ctx, transport.StopCommand{Reason: "manual_queue_prepare"})

	played := false
	startAt := time.Now()
	ticker := time.NewTicker(playerDispatchTick)
	defer ticker.Stop()
	tickCount := 0

	for {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			pauseOnly := p.pauseExit
			p.pauseExit = false
			p.mu.Unlock()
			if pauseOnly {
				return
			}
			p.finish(false)
			return
		case <-ticker.C:
			tickCount++
			p.mu.Lock()
			session = p.session
			elapsed := p.elapsedMSLocked()
			hspBaseMS := p.baseMS
			localElapsed := elapsed - hspBaseMS
			p.playheadMS = localElapsed
			nextIndex := p.nextIndex
			debugFn := p.dispatchDebug
			p.mu.Unlock()

			bufferAheadMS := int64(0)
			if nextIndex < len(session.Points) {
				bufferAheadMS = session.Points[nextIndex].TimeMillis - int64(localElapsed)
				if bufferAheadMS < 0 {
					bufferAheadMS = 0
				}
			}
			if debugFn != nil && (tickCount%5 == 0 || (!played && tickCount > 3) || bufferAheadMS < int64(playerLeadMS)) {
				debugFn("player_dispatch", map[string]any{
					"elapsed_ms":       elapsed,
					"local_elapsed_ms": localElapsed,
					"hsp_base_ms":      hspBaseMS,
					"next_index":       nextIndex,
					"total_points":     len(session.Points),
					"buffer_ahead_ms":  bufferAheadMS,
					"played":           played,
					"duration_ms":      session.DurationMS,
					"stream_id":        streamID,
					"starvation_risk":  played && bufferAheadMS < int64(playerLeadMS),
				})
			}

			for nextIndex < len(session.Points) {
				pointMS := int(session.Points[nextIndex].TimeMillis)
				if pointMS > localElapsed+playerLeadMS {
					break
				}
				end := nextIndex + playerChunkSize
				if end > len(session.Points) {
					end = len(session.Points)
				}
				batch := session.Points[nextIndex:end]
				hspBatch := hspAbsoluteBatch(hspBaseMS, batch)
				// #region agent log
				if debugFn != nil {
					localMin, localMax := batch[0].TimeMillis, batch[len(batch)-1].TimeMillis
					hspMin, hspMax := hspBatch[0].TimeMillis, hspBatch[len(hspBatch)-1].TimeMillis
					debugFn("hsp_dispatch_absolute", map[string]any{
						"hypothesisId":     "H8",
						"hsp_base_ms":      hspBaseMS,
						"elapsed_ms":       elapsed,
						"local_elapsed_ms": localElapsed,
						"local_time_min":   localMin,
						"local_time_max":   localMax,
						"hsp_time_min":     hspMin,
						"hsp_time_max":     hspMax,
					})
				}
				// #endregion
				if _, err := p.transport.AddHSP(ctx, transport.HSPAddCommand{
					StreamID: streamID,
					Points:   hspBatch,
				}); err != nil {
					p.finish(false)
					return
				}
				nextIndex = end

				if !played {
					startTime := int64(hspBaseMS)
					if len(hspBatch) > 0 && hspBatch[0].TimeMillis > startTime {
						// Align play start with the first buffered point so
						// pause_on_starving does not stall before chat chaos lead.
						startTime = hspBatch[0].TimeMillis
					}
					if _, err := p.transport.PlayHSP(ctx, transport.HSPPlayCommand{
						StreamID:        streamID,
						StartTimeMillis: startTime,
					}); err != nil {
						p.finish(false)
						return
					}
					played = true
				}
			}

			p.mu.Lock()
			p.nextIndex = nextIndex
			if len(p.session.Points) > 180 {
				p.compactTimelineLocked()
				nextIndex = p.nextIndex
			}
			durationMS := p.session.DurationMS
			if durationMS <= 0 {
				durationMS = DurationMS(p.session.Actions)
			}
			timelineEndMS := durationMS
			if n := len(p.session.Points); n > 0 {
				if last := int(p.session.Points[n-1].TimeMillis); last > timelineEndMS {
					timelineEndMS = last
				}
			}
			// Use p.session.Points — a stale local copy misses AppendExtension reallocations.
			localElapsed = p.localPlayheadMSLocked()
			continuous := p.session.Continuous
			shouldFinish := !continuous && played && nextIndex >= len(p.session.Points) && localElapsed > timelineEndMS+playerFinishTailMS
			p.mu.Unlock()

			if shouldFinish {
				// #region agent log
				if p.dispatchDebug != nil {
					p.dispatchDebug("player_finish_triggered", map[string]any{
						"elapsed_ms":      elapsed,
						"local_elapsed_ms": localElapsed,
						"next_index":      nextIndex,
						"total_points":    len(p.session.Points),
						"timeline_end_ms": timelineEndMS,
						"continuous":      continuous,
						"autoloop":        autoloop,
					})
				}
				// #endregion
				if autoloop {
					_, _ = p.transport.Stop(ctx, transport.StopCommand{Reason: "manual_queue_loop"})
					p.mu.Lock()
					p.nextIndex = 0
					p.mu.Unlock()
					played = false
					startAt = time.Now()
					streamID = fmt.Sprintf("%s-%d", playerStreamPrefix, time.Now().UnixNano())
					p.mu.Lock()
					p.baseMS = 0
					p.playheadMS = 0
					p.startedAt = startAt
					p.streamID = streamID
					p.mu.Unlock()
					continue
				}
				_, _ = p.transport.Stop(ctx, transport.StopCommand{Reason: "manual_queue_complete"})
				p.finish(true)
				return
			}
		}
	}
}

func (p *Player) elapsedMSLocked() int {
	if p.startedAt.IsZero() {
		return p.baseMS
	}
	return int(time.Since(p.startedAt).Milliseconds()) + p.baseMS
}

func (p *Player) finish(notify bool) {
	var callback func()
	p.mu.Lock()
	p.running = false
	p.paused = false
	callback = p.onFinished
	p.mu.Unlock()
	if notify && callback != nil {
		callback()
	}
}
