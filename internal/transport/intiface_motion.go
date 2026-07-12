package transport

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Stop preempts queued motion and sends Buttplug StopDeviceCmd.
func (i *Intiface) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	started := time.Now()
	i.paceMu.Lock()
	i.invalidatePlaybackLocked(true)
	i.paceMu.Unlock()
	i.signalPacer()

	selection, err := i.selectedDevice()
	if err != nil {
		i.setPlaybackState("stale")
		return i.completeCommand(Command{Kind: CommandKindStop, Stop: &command}, "stale", started, err), err
	}
	err = i.requestOK(ctx, "StopDeviceCmd", map[string]any{"DeviceIndex": selection.deviceIndex})
	status := "idle"
	if err != nil {
		status = "stale"
	}
	i.setPlaybackState(status)
	return i.completeCommand(Command{Kind: CommandKindStop, Stop: &command}, status, started, err), err
}

// SetStrokeWindow atomically updates host-side projection and direction mapping.
func (i *Intiface) SetStrokeWindow(_ context.Context, command StrokeWindowCommand) (CommandResult, error) {
	started := time.Now()
	if err := validateStrokeWindow(command); err != nil {
		return i.completeCommand(Command{Kind: CommandKindStrokeWindow, StrokeWindow: &command}, "rejected", started, err), err
	}
	i.paceMu.Lock()
	i.window = command
	i.paceMu.Unlock()
	return i.completeCommand(Command{Kind: CommandKindStrokeWindow, StrokeWindow: &command}, "applied", started, nil), nil
}

// AppendPoints validates and queues point-pair LinearCmd segments without resampling.
func (i *Intiface) AppendPoints(_ context.Context, command AppendPointsCommand) (CommandResult, error) {
	started := time.Now()
	streamID, err := cleanStreamID(command.StreamID)
	if err == nil {
		err = validateIntifacePoints(command.Points)
	}
	if err != nil {
		return i.completeCommand(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, "rejected", started, err), err
	}

	i.paceMu.Lock()
	if i.streamID != "" && i.streamID != streamID {
		if i.playing {
			err = errors.New("cannot replace an active Intiface stream")
		} else {
			i.queue = nil
			i.tail = nil
		}
	}
	segments := make([]intifaceSegment, 0, len(command.Points))
	previous := i.tail
	if i.streamID != streamID {
		previous = nil
	}
	for index := range command.Points {
		point := command.Points[index]
		if previous != nil {
			if point.TimeMillis <= previous.TimeMillis {
				err = fmt.Errorf("point time %d must be greater than previous time %d", point.TimeMillis, previous.TimeMillis)
				break
			}
			if point.TimeMillis-previous.TimeMillis > int64(^uint32(0)) {
				err = errors.New("point duration exceeds Buttplug LinearCmd bounds")
				break
			}
			segments = append(segments, intifaceSegment{
				startMillis: previous.TimeMillis,
				duration:    point.TimeMillis - previous.TimeMillis,
				position:    point.PositionPercent,
			})
		}
		pointCopy := point
		previous = &pointCopy
	}
	if err == nil && len(i.queue)+len(segments) > i.options.QueueCapacity {
		err = fmt.Errorf("Intiface queue capacity %d exceeded", i.options.QueueCapacity)
	}
	if err == nil {
		i.streamID = streamID
		i.queue = append(i.queue, segments...)
		tail := command.Points[len(command.Points)-1]
		i.tail = &tail
	}
	i.paceMu.Unlock()
	if err != nil {
		return i.completeCommand(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, "rejected", started, err), err
	}
	i.signalPacer()
	return i.completeCommand(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, "buffered", started, nil), nil
}

// Play starts immediate-mode pacing for the selected device and queued stream.
func (i *Intiface) Play(_ context.Context, command PlayCommand) (CommandResult, error) {
	started := time.Now()
	streamID, err := cleanStreamID(command.StreamID)
	if err == nil && command.StartTimeMillis < 0 {
		err = errors.New("Intiface play start time must be non-negative")
	}
	if err == nil && command.StartTimeMillis > maxIntifaceScheduleMillis {
		err = errors.New("Intiface play start time is too large")
	}
	if err == nil {
		_, err = i.selectedDevice()
	}
	if err != nil {
		return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "rejected", started, err), err
	}

	i.mu.Lock()
	sessionCtx := i.sessionCtx
	i.mu.Unlock()
	i.paceMu.Lock()
	if i.streamID != streamID {
		err = errors.New("Intiface play stream has no queued points")
	} else if len(i.queue) == 0 {
		err = errors.New("Intiface play requires at least one point pair")
	} else if i.playing {
		err = errors.New("Intiface stream is already playing")
	}
	if err == nil {
		i.generation++
		i.playing = true
		i.playBase = time.Now().Add(-time.Duration(command.StartTimeMillis) * time.Millisecond)
		i.coverageEnd = time.Time{}
		i.playCtx, i.playStop = context.WithCancel(sessionCtx)
	}
	i.paceMu.Unlock()
	if err != nil {
		return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "rejected", started, err), err
	}
	i.setPlaybackState("playing")
	i.signalPacer()
	return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "playing", started, nil), nil
}

// Diagnostics returns a redacted transport diagnostics snapshot.
func (i *Intiface) Diagnostics() TransportDiagnostics {
	i.mu.Lock()
	defer i.mu.Unlock()
	diagnostics := i.diagnosis
	if diagnostics.LastCommand != nil {
		command := SafeCommand(*diagnostics.LastCommand)
		diagnostics.LastCommand = &command
	}
	if diagnostics.LastResult != nil {
		result := SafeCommandResult(*diagnostics.LastResult)
		diagnostics.LastResult = &result
	}
	if diagnostics.LastError != "" {
		diagnostics.LastError = "redacted"
	}
	return diagnostics
}

func (i *Intiface) paceLoop(ctx context.Context) {
	defer i.wg.Done()
	for {
		i.paceMu.Lock()
		if !i.playing {
			i.paceMu.Unlock()
			if !i.waitForPacer(ctx, 0) {
				return
			}
			continue
		}
		generation := i.generation
		playCtx := i.playCtx
		if len(i.queue) == 0 {
			coverageEnd := i.coverageEnd
			i.paceMu.Unlock()
			i.handleEmptyPacerQueue(ctx, playCtx, generation, coverageEnd)
			continue
		}

		segment := i.queue[0]
		playBase := i.playBase
		i.paceMu.Unlock()
		if !i.paceSegment(ctx, playCtx, generation, playBase, segment) && ctx.Err() != nil {
			return
		}
	}
}

func (i *Intiface) handleEmptyPacerQueue(ctx, playCtx context.Context, generation uint64, coverageEnd time.Time) {
	if coverageEnd.IsZero() {
		coverageEnd = time.Now()
	}
	wait := time.Until(coverageEnd)
	if wait > 0 && i.waitForPacer(playCtx, wait) {
		return
	}
	if playCtx.Err() != nil {
		return
	}
	i.paceMu.Lock()
	if i.playing && i.generation == generation && len(i.queue) == 0 {
		i.invalidatePlaybackLocked(false)
		i.paceMu.Unlock()
		i.setPlaybackState("starved")
		i.stopSelectedDevice(ctx)
		return
	}
	i.paceMu.Unlock()
}

func (i *Intiface) paceSegment(ctx, playCtx context.Context, generation uint64, playBase time.Time, segment intifaceSegment) bool {
	wait := time.Until(playBase.Add(time.Duration(segment.startMillis) * time.Millisecond))
	if wait > 0 {
		return i.waitForPacer(playCtx, wait)
	}
	if -wait > intifaceLateTolerance {
		i.paceMu.Lock()
		i.invalidatePlaybackLocked(false)
		i.paceMu.Unlock()
		i.setPlaybackState("starved")
		i.stopSelectedDevice(ctx)
		return true
	}

	err := i.sendLinear(playCtx, generation, segment)
	if err != nil {
		if errors.Is(err, errPacerSuperseded) || playCtx.Err() != nil {
			return true
		}
		i.cancelPlayback("rejected")
		i.stopSelectedDevice(ctx)
		return true
	}
	i.paceMu.Lock()
	if i.playing && i.generation == generation && len(i.queue) > 0 && i.queue[0] == segment {
		i.queue = i.queue[1:]
		i.coverageEnd = i.playBase.Add(time.Duration(segment.startMillis+segment.duration) * time.Millisecond)
	}
	i.paceMu.Unlock()
	return true
}

func (i *Intiface) sendLinear(ctx context.Context, generation uint64, segment intifaceSegment) error {
	selection, err := i.selectedDevice()
	if err != nil {
		return err
	}
	i.paceMu.Lock()
	window := i.window
	i.paceMu.Unlock()
	position := projectIntifacePosition(segment.position, window)
	payload := map[string]any{
		"DeviceIndex": selection.deviceIndex,
		"Vectors": []map[string]any{{
			"Index":    selection.actuatorIndex,
			"Duration": segment.duration,
			"Position": position,
		}},
	}
	message, err := i.requestGuarded(ctx, "LinearCmd", payload, generation)
	if err != nil {
		return err
	}
	if message.kind != "Ok" {
		return fmt.Errorf("expected Buttplug Ok for LinearCmd, received %s", message.kind)
	}
	return nil
}
