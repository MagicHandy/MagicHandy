package transport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/companion"
)

// vibratorSend is one composed level command.
type vibratorSend struct {
	deviceIndex uint32
	generation  uint64
	epoch       uint64
	serial      uint64
	run         uint64
	steps       []int
	fields      map[string]any
}

// MotionTransport admits a new engine bound to the current selection. A
// reselected or reconnected vibrator ends the old binding, so an old engine
// cannot append, play or stop the new device.
func (v *IntifaceVibrator) MotionTransport() (Transport, error) {
	v.session.mu.Lock()
	connected := v.session.connected
	v.session.mu.Unlock()
	v.mu.Lock()
	defer v.mu.Unlock()
	if !connected {
		return nil, errors.New("Intiface connection is stale")
	}
	if !v.selection.selected {
		return nil, errVibratorNotSelected
	}
	return &vibratorRun{owner: v, epoch: v.epoch}, nil
}

// AppendPoints stores the stream's neutral points. The owner only samples
// them; it never reshapes the frame.
func (v *IntifaceVibrator) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	return v.appendPoints(ctx, command, 0)
}
func (v *IntifaceVibrator) appendPoints(ctx context.Context, command AppendPointsCommand, epoch uint64) (CommandResult, error) {
	recorded := Command{Kind: CommandKindPointsAdd, PointsAdd: &command}
	if err := ctx.Err(); err != nil {
		return v.record(recorded, err)
	}
	points := make([]companion.Point, 0, len(command.Points))
	for _, point := range command.Points {
		points = append(points, companion.Point{TimeMillis: point.TimeMillis, Position: point.PositionPercent})
	}
	envelope, err := vibrationFrameTimeline(command.Vibration)
	if err != nil {
		return v.record(recorded, err)
	}
	if command.Vibration != nil {
		v.session.writeMu.Lock()
		defer v.session.writeMu.Unlock()
	}
	v.mu.Lock()
	if epoch != 0 && v.epoch != epoch {
		v.mu.Unlock()
		return CommandResult{}, errVibratorStale
	}
	newStream := command.StreamID != v.stream.id
	if !newStream && command.Vibration != nil && command.Vibration.Revision < v.stream.vibrationRevision {
		v.mu.Unlock()
		return v.record(recorded, errPacerSuperseded)
	}
	next := v.stream.timeline
	if newStream {
		if len(command.Points) == 0 {
			v.mu.Unlock()
			return v.record(recorded, errVibratorStale)
		}
		next = companion.Timeline{}
	}
	if len(points) > 0 {
		err = next.Append(points)
	}
	if err == nil {
		if newStream {
			v.stream.vibration = companion.Timeline{}
			v.stream.vibrationRevision = 0
			v.stream.independent = false
			v.stream.id = command.StreamID
		}
		v.stream.timeline = next
		v.applyVibrationFrameLocked(command.Vibration, envelope)
	}
	v.mu.Unlock()
	return v.record(recorded, err)
}

// Play starts the stream on its own clock: stream millisecond 0 lands after
// a short pre-roll.
func (v *IntifaceVibrator) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	return v.play(ctx, command, time.Time{})
}

// PlayAligned starts the stream on a linked session's clock, so no level is
// rendered against a provisional anchor first.
func (v *IntifaceVibrator) PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	return v.play(ctx, command, start)
}

func (v *IntifaceVibrator) play(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	return v.playBound(ctx, command, start, 0)
}
func (v *IntifaceVibrator) playBound(ctx context.Context, command PlayCommand, start time.Time, epoch uint64) (CommandResult, error) {
	v.lifecycleMu.Lock()
	defer v.lifecycleMu.Unlock()
	recorded := Command{Kind: CommandKindPointsPlay, PointsPlay: &command}
	if err := ctx.Err(); err != nil {
		return v.record(recorded, err)
	}
	v.session.mu.Lock()
	connected := v.session.connected
	v.session.mu.Unlock()
	v.mu.Lock()
	var err error
	switch {
	case epoch != 0 && v.epoch != epoch:
		err = errVibratorStale
	case !connected:
		err = errors.New("Intiface connection is stale")
	case !v.selection.selected:
		err = errVibratorNotSelected
	case command.StreamID != v.stream.id:
		err = fmt.Errorf("the vibrator stream %q has no points", command.StreamID)
	}
	if err == nil {
		if start.IsZero() {
			v.stream.origin.Start(v.clock(), command.StartTimeMillis, vibratorPreroll)
		} else {
			v.stream.origin.StartAt(start)
		}
		v.invalidateOutputLocked()
		v.runGeneration++
		v.stream.playing = true
		v.stream.coverage.Reset()
		v.stream.renderer.Restart(float64(command.StartTimeMillis))
		v.stream.state = companion.StateCovered
		v.testUntil = time.Time{}
		v.testDeadline.Store(0)
		v.stats.lastError = ""
		v.playingFlag.Store(true)
		v.stalled.Store(false)
		v.lastPace.Store(v.clock().UnixNano())
		v.addEventLocked("play", 0, nil)
	}
	v.mu.Unlock()
	return v.record(recorded, err)
}

// Stop ends the stream and silences the device: a zero level for its motors,
// then StopDeviceCmd. Pause arrives here too. A Stop that cannot be
// confirmed fails the session, because the server then stops its devices.
func (v *IntifaceVibrator) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	return v.stopBound(ctx, command, 0)
}
func (v *IntifaceVibrator) stopBound(ctx context.Context, command StopCommand, epoch uint64) (CommandResult, error) {
	v.lifecycleMu.Lock()
	defer v.lifecycleMu.Unlock()
	v.mu.Lock()
	if epoch != 0 && v.epoch != epoch {
		v.mu.Unlock()
		return CommandResult{OK: true, Status: "connection_retired"}, nil
	}
	v.invalidateLocked("idle")
	v.addEventLocked("stop", 0, nil)
	selection := v.selectionSnapshotLocked()
	v.mu.Unlock()
	recorded := Command{Kind: CommandKindStop, Stop: &command}
	if !selection.selected {
		return v.record(recorded, nil)
	}
	return v.record(recorded, v.silence(ctx, selection))
}

// SetStrokeWindow is accepted and ignored: the stroke window is a stroker's
// physical envelope, and the vibrator follows the semantic frame.
func (v *IntifaceVibrator) SetStrokeWindow(_ context.Context, command StrokeWindowCommand) (CommandResult, error) {
	return v.record(Command{Kind: CommandKindStrokeWindow, StrokeWindow: &command}, nil)
}

// MotionTimingCapabilities declares only a buffered lead covering the
// timing offset and lookahead. The vibrator samples the frame itself, so it
// never coarsens a stroker's sampling with a point interval.
func (v *IntifaceVibrator) MotionTimingCapabilities() MotionTimingCapabilities {
	return MotionTimingCapabilities{MinimumBufferedLead: vibratorMinimumLead, MinimumMediaBufferedLead: vibratorMediaLead}
}

// PlaybackStartTime reports the local time of stream millisecond 0.
func (v *IntifaceVibrator) PlaybackStartTime() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.stream.origin.Anchor()
}

// AlignPlaybackStart re-anchors a playing stream to another owner's start.
func (v *IntifaceVibrator) AlignPlaybackStart(start time.Time) {
	v.alignPlaybackStart(start, 0)
}

func (v *IntifaceVibrator) alignPlaybackStart(start time.Time, epoch uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if (epoch == 0 || v.epoch == epoch) && v.stream.playing && !start.IsZero() {
		v.stream.origin.StartAt(start)
	}
}

// Diagnostics returns the safe transport snapshot the engine reads for
// playback health.
func (v *IntifaceVibrator) Diagnostics() TransportDiagnostics {
	v.session.mu.Lock()
	connected := v.session.connected
	v.session.mu.Unlock()
	v.mu.Lock()
	defer v.mu.Unlock()
	diagnostics := v.diag
	diagnostics.Name = vibratorTransportName
	diagnostics.Connected = connected && v.selection.selected
	diagnostics.PlaybackState = v.playbackStateLocked()
	if v.stats.lastError != "" {
		diagnostics.LastError = v.stats.lastError
	}
	return diagnostics
}

func (v *IntifaceVibrator) record(command Command, err error) (CommandResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.clock()
	v.diag.CommandCount++
	command.ID = fmt.Sprintf("vibrator-%06d", v.diag.CommandCount)
	command.IssuedAt = now.UTC().Format(time.RFC3339Nano)
	result := CommandResult{
		CommandID:   command.ID,
		Kind:        command.Kind,
		Transport:   vibratorTransportName,
		OK:          err == nil,
		Status:      v.stream.state,
		CompletedAt: now.UTC().Format(time.RFC3339Nano),
	}
	if err != nil {
		result.Error = err.Error()
		v.diag.LastError = err.Error()
	}
	v.diag.LastCommand = &command
	v.diag.LastResult = &result
	return result, err
}

// paceLoop renders one level per tick, or sooner when a lowered intensity or
// a test needs it. It ticks at the base cadence whatever the device: a slower
// device spaces its sends instead, so the watchdog and the ping always see a
// live pacer.
func (v *IntifaceVibrator) paceLoop(ctx context.Context) {
	defer v.session.wg.Done()
	ticker := time.NewTicker(v.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-v.wake:
		}
		v.paceOnce(ctx)
	}
}

// sendSpacingLocked is the shortest time between two nonzero levels: the
// selected device's advisory timing gap plus a margin, or none.
func (v *IntifaceVibrator) sendSpacingLocked() time.Duration {
	return intifaceSelection{timingGap: v.selection.timingGap}.minimumPointInterval()
}

func (v *IntifaceVibrator) paceOnce(ctx context.Context) {
	now := v.clock()
	v.lastPace.Store(now.UnixNano())
	v.mu.Lock()
	send, ok := v.composeLocked(now)
	v.mu.Unlock()
	if ok {
		v.send(ctx, send)
	}
}

// watchdog stops the device when playback runs on a stalled pacer. It reads
// atomics before Stop, so a pacer stuck holding the vibrator lock cannot
// prevent the hardware Stop. Post-Stop bookkeeping takes the owner lock.
func (v *IntifaceVibrator) watchdog(ctx context.Context) {
	defer v.session.wg.Done()
	ticker := time.NewTicker(vibratorWatchdogTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		v.stopStalledOutput(ctx)
	}
}

func (v *IntifaceVibrator) stopStalledOutput(ctx context.Context) {
	v.lifecycleMu.Lock()
	defer v.lifecycleMu.Unlock()
	if v.paceAlive(v.clock()) || v.stalled.Load() {
		return
	}
	// Admission reads this flag under the writer lock, including for an
	// already-composed command. A new Play/selection waits for this Stop.
	v.stalled.Store(true)
	stopCtx, cancel := context.WithTimeout(ctx, vibratorStopTimeout)
	err := v.session.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": v.watchedIndex.Load()})
	cancel()
	if err != nil && ctx.Err() == nil {
		v.session.setSessionFailure(fmt.Errorf("Intiface vibrator watchdog Stop could not be confirmed: %w", err))
	} else if err == nil {
		v.mu.Lock()
		v.addEventLocked("watchdog_stop", 0, nil)
		v.level = 0
		for index := range v.selection.motors {
			v.selection.motors[index].written = 0
		}
		v.mu.Unlock()
	}
}

// vibratorRun binds one engine to one selection of the vibrator.
type vibratorRun struct {
	owner *IntifaceVibrator
	epoch uint64
}

func (r *vibratorRun) current() bool {
	r.owner.mu.Lock()
	defer r.owner.mu.Unlock()
	return r.owner.epoch == r.epoch
}

func (r *vibratorRun) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	if !r.current() {
		return CommandResult{}, errVibratorStale
	}
	return r.owner.appendPoints(ctx, command, r.epoch)
}

func (r *vibratorRun) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	if !r.current() {
		return CommandResult{}, errVibratorStale
	}
	return r.owner.playBound(ctx, command, time.Time{}, r.epoch)
}

func (r *vibratorRun) PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	if !r.current() {
		return CommandResult{}, errVibratorStale
	}
	return r.owner.playBound(ctx, command, start, r.epoch)
}

// Stop on a retired binding is a no-op: the device it named was already
// stopped when it was released, reselected or lost.
func (r *vibratorRun) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	if !r.current() {
		return CommandResult{OK: true, Status: "connection_retired"}, nil
	}
	return r.owner.stopBound(ctx, command, r.epoch)
}

func (r *vibratorRun) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	return r.owner.SetStrokeWindow(ctx, command)
}

func (r *vibratorRun) Diagnostics() TransportDiagnostics {
	diagnostics := r.owner.Diagnostics()
	if !r.current() {
		diagnostics.Connected = false
		diagnostics.PlaybackState = "stale"
		diagnostics.LastError = errVibratorStale.Error()
	}
	return diagnostics
}

func (r *vibratorRun) MotionTimingCapabilities() MotionTimingCapabilities {
	return r.owner.MotionTimingCapabilities()
}

func (r *vibratorRun) PlaybackStartTime() time.Time {
	r.owner.mu.Lock()
	defer r.owner.mu.Unlock()
	if r.owner.epoch != r.epoch {
		return time.Time{}
	}
	return r.owner.stream.origin.Anchor()
}

func (r *vibratorRun) AlignPlaybackStart(start time.Time) {
	r.owner.alignPlaybackStart(start, r.epoch)
}
