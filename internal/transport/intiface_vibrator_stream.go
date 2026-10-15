package transport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/companion"
	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

// vibratorSend is one composed level command.
type vibratorSend struct {
	deviceIndex uint32
	generation  uint64
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
	recorded := Command{Kind: CommandKindPointsAdd, PointsAdd: &command}
	if err := ctx.Err(); err != nil {
		return v.record(recorded, err)
	}
	points := make([]companion.Point, 0, len(command.Points))
	for _, point := range command.Points {
		points = append(points, companion.Point{TimeMillis: point.TimeMillis, Position: point.PositionPercent})
	}
	v.mu.Lock()
	if command.StreamID != v.stream.id {
		v.stream.timeline.Reset()
		v.stream.id = command.StreamID
	}
	err := v.stream.timeline.Append(points)
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
		v.stream.generation++
		v.stream.playing = true
		v.stream.coverage.Reset()
		v.stream.renderer.Restart(float64(command.StartTimeMillis))
		v.stream.state = companion.StateCovered
		v.testUntil = time.Time{}
		v.playingFlag.Store(true)
		v.stalled.Store(false)
		v.lastPace.Store(v.clock().UnixNano())
	}
	v.mu.Unlock()
	return v.record(recorded, err)
}

// Stop ends the stream and silences the device: a zero level for its motors,
// then StopDeviceCmd. Pause arrives here too. A Stop that cannot be
// confirmed fails the session, because the server then stops its devices.
func (v *IntifaceVibrator) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	v.mu.Lock()
	v.invalidateLocked("idle")
	selection := v.selection
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
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.stream.playing && !start.IsZero() {
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

// composeLocked picks this tick's level and returns a command only when a
// motor's step changes. A nonzero level waits for the command in flight and
// for the device's timing gap; a silent level is always sent at once.
func (v *IntifaceVibrator) composeLocked(now time.Time) (vibratorSend, bool) {
	if !v.selection.selected {
		return vibratorSend{}, false
	}
	level, generation := v.targetLocked(now)
	if v.stalled.Load() {
		// The watchdog stopped this run: only silence until the next Play.
		level, generation = 0, 0
	}
	v.level = level
	silent := level <= 0
	if !silent && (v.inFlight || (!v.lastSent.IsZero() && now.Sub(v.lastSent) < v.sendSpacingLocked())) {
		return vibratorSend{}, false
	}
	send := vibratorSend{deviceIndex: v.selection.deviceIndex, generation: generation, steps: make([]int, len(v.selection.motors))}
	changed := false
	levels := make([]scalarLevel, 0, len(v.selection.motors))
	for index, motor := range v.selection.motors {
		step := vibration.Quantize(level, motor.StepCount, motor.last, v.config.Ceiling)
		send.steps[index] = step
		changed = changed || step != motor.last
		levels = append(levels, scalarLevel{index: motor.Index, level: float64(step) / float64(motor.StepCount)})
	}
	if !changed {
		return vibratorSend{}, false
	}
	if silent {
		send.generation = 0
	}
	send.fields = encodeScalarCmd(send.deviceIndex, vibratorActuatorType, levels)
	for index := range v.selection.motors {
		v.selection.motors[index].last = send.steps[index]
	}
	v.inFlight = true
	v.lastSent = now
	v.stats.commanded++
	return send, true
}

// targetLocked returns the level for this tick and the generation that may
// carry it: playback levels are fenced by the playback generation, a test
// and silence by none.
func (v *IntifaceVibrator) targetLocked(now time.Time) (float64, uint64) {
	if !v.testUntil.IsZero() {
		if now.Before(v.testUntil) {
			return min(v.intensity, vibratorTestCeiling), v.testGeneration
		}
		v.testUntil = time.Time{}
		return 0, 0
	}
	if !v.stream.playing {
		return 0, 0
	}
	at := v.stream.origin.StreamMillis(now) - float64(v.config.OffsetMillis)
	drive, covered := v.stream.renderer.Drive(&v.stream.timeline, at)
	v.stream.state = v.stream.coverage.Observe(now, covered)
	v.stream.timeline.Prune(int64(at) - vibration.SpeedWindowMillis - vibratorPruneMillis)
	if v.stream.state == companion.StateStarved {
		// Stop and report: the engine reads the state and recovers.
		v.stream.playing = false
		v.playingFlag.Store(false)
		return 0, 0
	}
	return vibration.Output(drive, v.config.Minimum, v.intensity), v.stream.generation
}

// send writes one level command and correlates its acknowledgement without
// blocking the pacer. A missing or rejected acknowledgement makes the
// vibrator unhealthy and stops the device.
func (v *IntifaceVibrator) send(ctx context.Context, send vibratorSend) {
	request, err := v.session.startRequestFor(ctx, v, buttplugScalarCmd, send.fields, send.generation, nil)
	if err != nil {
		// Nothing reached the wire: a superseded generation, the emergency
		// barrier, a full request window or a lost session. The next tick
		// recomputes the level; a broken socket already failed the session.
		v.finishSend(send, false)
		return
	}
	v.session.workerMu.Lock()
	if v.session.workersClosed {
		v.session.workerMu.Unlock()
		v.finishSend(send, false)
		return
	}
	v.session.wg.Add(1)
	v.session.workerMu.Unlock()
	go func() {
		defer v.session.wg.Done()
		message, err := v.session.awaitResponse(ctx, request)
		if err == nil && message.kind != "Ok" {
			err = fmt.Errorf("Intiface rejected ScalarCmd: %s", message.kind)
		}
		v.finishSend(send, err == nil)
		if err != nil && ctx.Err() == nil {
			v.failSend(send, err, errors.Is(err, errIntifaceResponseTime))
		}
	}()
}

func (v *IntifaceVibrator) finishSend(send vibratorSend, acknowledged bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.inFlight = false
	if acknowledged {
		v.stats.acked++
		return
	}
	for index := range v.selection.motors {
		if v.selection.deviceIndex == send.deviceIndex {
			v.selection.motors[index].last = -1
		}
	}
}

func (v *IntifaceVibrator) failSend(send vibratorSend, cause error, timedOut bool) {
	v.mu.Lock()
	if timedOut {
		v.stats.timeouts++
	} else {
		v.stats.rejected++
	}
	v.stats.lastError = cause.Error()
	selection := v.selection
	current := selection.selected && selection.deviceIndex == send.deviceIndex
	if current {
		v.invalidateLocked("stale")
	}
	v.mu.Unlock()
	if current {
		ctx, cancel := context.WithTimeout(context.Background(), vibratorStopTimeout)
		_ = v.silence(ctx, selection)
		cancel()
	}
}

// silence sends a zero level for the selected motors and StopDeviceCmd. The
// zero may be refused behind an emergency barrier or a closing session,
// where StopAllDevices or StopDeviceCmd still reaches the device; only an
// unconfirmed StopDeviceCmd fails the session.
func (v *IntifaceVibrator) silence(ctx context.Context, selection vibratorSelection) error {
	err := v.silenceWithoutFailure(ctx, selection)
	if err != nil {
		v.session.setSessionFailure(fmt.Errorf("Intiface vibrator Stop could not be confirmed: %w", err))
	}
	return err
}

func (v *IntifaceVibrator) silenceWithoutFailure(ctx context.Context, selection vibratorSelection) error {
	levels := make([]scalarLevel, 0, len(selection.motors))
	for _, motor := range selection.motors {
		levels = append(levels, scalarLevel{index: motor.Index})
	}
	zeroCtx, cancel := context.WithTimeout(ctx, vibratorStopTimeout)
	if v.session.requestOK(zeroCtx, buttplugScalarCmd, encodeScalarCmd(selection.deviceIndex, vibratorActuatorType, levels)) == nil {
		v.markSilent(selection.deviceIndex)
	}
	cancel()
	stopCtx, cancel := context.WithTimeout(ctx, vibratorStopTimeout)
	defer cancel()
	return v.session.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": selection.deviceIndex})
}

// markSilent records a confirmed zero, so the pacer does not repeat it. An
// unconfirmed zero leaves the steps unknown and the pacer sends it again.
func (v *IntifaceVibrator) markSilent(deviceIndex uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.selection.selected || v.selection.deviceIndex != deviceIndex {
		return
	}
	for index := range v.selection.motors {
		v.selection.motors[index].last = 0
	}
}

// watchdog stops the device when playback runs on a stalled pacer. It reads
// only atomics, so a pacer stuck holding the vibrator lock cannot block it.
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
		if v.paceAlive(v.clock()) || v.stalled.Load() {
			continue
		}
		v.stalled.Store(true)
		stopCtx, cancel := context.WithTimeout(ctx, vibratorStopTimeout)
		err := v.session.requestOK(stopCtx, "StopDeviceCmd", map[string]any{"DeviceIndex": v.watchedIndex.Load()})
		cancel()
		if err != nil && ctx.Err() == nil {
			v.session.setSessionFailure(fmt.Errorf("Intiface vibrator watchdog Stop could not be confirmed: %w", err))
		}
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
	return r.owner.AppendPoints(ctx, command)
}

func (r *vibratorRun) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	if !r.current() {
		return CommandResult{}, errVibratorStale
	}
	return r.owner.Play(ctx, command)
}

func (r *vibratorRun) PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	if !r.current() {
		return CommandResult{}, errVibratorStale
	}
	return r.owner.PlayAligned(ctx, command, start)
}

// Stop on a retired binding is a no-op: the device it named was already
// stopped when it was released, reselected or lost.
func (r *vibratorRun) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	if !r.current() {
		return CommandResult{OK: true, Status: "connection_retired"}, nil
	}
	return r.owner.Stop(ctx, command)
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
	if !r.current() {
		return time.Time{}
	}
	return r.owner.PlaybackStartTime()
}

func (r *vibratorRun) AlignPlaybackStart(start time.Time) {
	if r.current() {
		r.owner.AlignPlaybackStart(start)
	}
}
