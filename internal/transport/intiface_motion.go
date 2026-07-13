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
	i.stopMu.Lock()
	defer i.stopMu.Unlock()

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
		i.setSessionFailure(fmt.Errorf("Intiface Stop could not be confirmed: %w", err))
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
	i.stopMu.RLock()
	defer i.stopMu.RUnlock()
	if i.motionAdmissionClosed() {
		err := errors.New("Intiface owner is closed")
		return i.completeCommand(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, "rejected", started, err), err
	}

	streamID, err := cleanStreamID(command.StreamID)
	if err == nil {
		err = validateIntifacePoints(command.Points)
	}
	if err != nil {
		return i.completeCommand(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, "rejected", started, err), err
	}

	i.mu.Lock()
	timingGap := time.Duration(0)
	if i.selected {
		timingGap = i.selection.minimumPointInterval()
	}
	i.mu.Unlock()

	err = i.appendIntifacePoints(streamID, command.Points, timingGap)
	if err != nil {
		return i.completeCommand(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, "rejected", started, err), err
	}
	i.signalPacer()
	return i.completeCommand(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, "buffered", started, nil), nil
}

func (i *Intiface) appendIntifacePoints(streamID string, points []TimedPoint, timingGap time.Duration) error {
	i.paceMu.Lock()
	defer i.paceMu.Unlock()
	differentStream := i.streamID != "" && i.streamID != streamID
	if differentStream && i.playing {
		return errors.New("cannot replace an active Intiface stream")
	}
	previous := i.tail
	anchor := i.anchor
	if i.streamID != streamID {
		previous = nil
		anchor = nil
	}
	reverse := i.window.ReverseDirection
	segments, previous, anchor, err := buildIntifaceSegments(points, previous, anchor, reverse, timingGap)
	if err != nil {
		return err
	}
	baseQueueDepth := len(i.queue)
	if i.streamID != streamID {
		baseQueueDepth = 0
	}
	if baseQueueDepth+len(segments) > i.options.QueueCapacity {
		return fmt.Errorf("Intiface queue capacity %d exceeded", i.options.QueueCapacity)
	}
	if i.streamID != streamID {
		i.queue = nil
	}
	i.streamID = streamID
	i.queue = append(i.queue, segments...)
	i.tail = previous
	i.anchor = anchor
	return nil
}

func buildIntifaceSegments(points []TimedPoint, previous *TimedPoint, anchor *intifaceAnchor, reverse bool, timingGap time.Duration) ([]intifaceSegment, *TimedPoint, *intifaceAnchor, error) {
	segments := make([]intifaceSegment, 0, len(points))
	for index := range points {
		point := points[index]
		point.PositionPercent = snapshotIntifacePosition(point.PositionPercent, reverse)
		if previous == nil && anchor == nil {
			anchor = &intifaceAnchor{timeMillis: point.TimeMillis, position: point.PositionPercent}
		}
		if previous != nil {
			duration := point.TimeMillis - previous.TimeMillis
			if point.TimeMillis <= previous.TimeMillis {
				return nil, previous, anchor, fmt.Errorf("point time %d must be greater than previous time %d", point.TimeMillis, previous.TimeMillis)
			}
			if duration > int64(^uint32(0)) {
				return nil, previous, anchor, errors.New("point duration exceeds Buttplug LinearCmd bounds")
			}
			if timingGap > 0 && time.Duration(duration)*time.Millisecond < timingGap {
				return nil, previous, anchor, fmt.Errorf("point interval %dms is below the selected device timing floor of %dms", duration, timingGap.Milliseconds())
			}
			segments = append(segments, intifaceSegment{
				startMillis: previous.TimeMillis,
				duration:    duration,
				position:    point.PositionPercent,
			})
		}
		pointCopy := point
		previous = &pointCopy
	}
	return segments, previous, anchor, nil
}

// Play starts immediate-mode pacing for the selected device and queued stream.
func (i *Intiface) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	started := time.Now()
	i.stopMu.RLock()
	if i.motionAdmissionClosed() {
		i.stopMu.RUnlock()
		err := errors.New("Intiface owner is closed or disconnected")
		return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "rejected", started, err), err
	}

	streamID, err := validateIntifacePlayCommand(command)
	var selection intifaceSelection
	if err == nil {
		selection, err = i.selectedDevice()
	}
	if err != nil {
		i.stopMu.RUnlock()
		return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "rejected", started, err), err
	}

	i.mu.Lock()
	sessionCtx := i.sessionCtx
	i.mu.Unlock()
	playStart, err := i.beginPlay(sessionCtx, streamID, command.StartTimeMillis, selection)
	if err != nil {
		i.stopMu.RUnlock()
		return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "rejected", started, err), err
	}
	i.setPlaybackState("anchoring")
	i.signalPacer()
	i.stopMu.RUnlock()
	select {
	case err = <-playStart.anchorDone:
	case <-ctx.Done():
		err = ctx.Err()
		i.recoverGeneration(playStart.generation, "rejected", "play_canceled")
	}
	if err != nil {
		return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "rejected", started, err), err
	}
	return i.completeCommand(Command{Kind: CommandKindPointsPlay, PointsPlay: &command}, "playing", started, nil), nil
}

func validateIntifacePlayCommand(command PlayCommand) (string, error) {
	streamID, err := cleanStreamID(command.StreamID)
	if err != nil {
		return "", err
	}
	if command.StartTimeMillis < 0 {
		return "", errors.New("Intiface play start time must be non-negative")
	}
	if command.StartTimeMillis > maxIntifaceScheduleMillis {
		return "", errors.New("Intiface play start time is too large")
	}
	return streamID, nil
}

func (i *Intiface) beginPlay(sessionCtx context.Context, streamID string, startTimeMillis int64, selection intifaceSelection) (intifacePlayStart, error) {
	i.paceMu.Lock()
	defer i.paceMu.Unlock()
	if i.streamID != streamID {
		return intifacePlayStart{}, errors.New("Intiface play stream has no queued points")
	}
	if len(i.queue) == 0 || i.anchor == nil {
		return intifacePlayStart{}, errors.New("Intiface play requires at least one point pair")
	}
	if i.playing {
		return intifacePlayStart{}, errors.New("Intiface stream is already playing")
	}
	if err := validateIntifaceQueueTiming(i.queue, selection.minimumPointInterval()); err != nil {
		return intifacePlayStart{}, err
	}

	i.generation++
	anchorDone := make(chan error, 1)
	i.playing = true
	i.playBase = time.Time{}
	i.playOffset = time.Duration(startTimeMillis) * time.Millisecond
	i.coverageEnd = time.Time{}
	i.playCtx, i.playStop = context.WithCancel(sessionCtx)
	i.anchoring = true
	i.anchorInFlight = false
	i.anchorDone = anchorDone
	i.lastPacerFailure = ""
	return intifacePlayStart{generation: i.generation, anchorDone: anchorDone}, nil
}

func validateIntifaceQueueTiming(queue []intifaceSegment, minimumPointInterval time.Duration) error {
	if minimumPointInterval <= 0 {
		return nil
	}
	for _, segment := range queue {
		if time.Duration(segment.duration)*time.Millisecond < minimumPointInterval {
			return fmt.Errorf("queued point interval %dms is below the selected device timing floor of %dms", segment.duration, minimumPointInterval.Milliseconds())
		}
	}
	return nil
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
		if i.anchoring {
			i.paceMu.Unlock()
			if !i.paceAnchorIteration(ctx, playCtx, generation) {
				return
			}
			continue
		}
		if len(i.queue) == 0 {
			coverageEnd := i.coverageEnd
			i.paceMu.Unlock()
			i.handleEmptyPacerQueue(playCtx, generation, coverageEnd)
			continue
		}

		decision := decideIntifacePace(time.Now(), i.playBase, i.queue)
		if decision.coalesced > 0 {
			i.queue = i.queue[decision.coalesced:]
			i.coalescedSegments += uint64(decision.coalesced)
		}
		if decision.failure != "" {
			i.paceMu.Unlock()
			i.recoverGeneration(generation, "starved", decision.failure)
			continue
		}
		if decision.wait > 0 {
			i.paceMu.Unlock()
			if !i.waitForPacer(playCtx, decision.wait) && ctx.Err() != nil {
				return
			}
			continue
		}
		segment := decision.segment
		playBase := i.playBase
		i.paceMu.Unlock()

		if err := i.sendLinear(playCtx, generation, playBase, segment, false); err != nil {
			if errors.Is(err, errPacerSuperseded) || playCtx.Err() != nil {
				continue
			}
			i.recoverGeneration(generation, pacerStateForError(err), pacerCategoryForError(err))
			continue
		}
		i.paceMu.Lock()
		if i.playing && i.generation == generation && len(i.queue) > 0 && i.queue[0] == segment {
			i.queue = i.queue[1:]
			i.coverageEnd = playBase.Add(time.Duration(segment.startMillis+segment.duration) * time.Millisecond)
		}
		i.paceMu.Unlock()
	}
}

func (i *Intiface) paceAnchorIteration(ctx, playCtx context.Context, generation uint64) bool {
	i.paceMu.Lock()
	if !i.playing || i.generation != generation || !i.anchoring {
		i.paceMu.Unlock()
		return true
	}
	if i.anchorInFlight {
		i.paceMu.Unlock()
		return i.waitForPacer(playCtx, 0) || ctx.Err() == nil
	}
	anchor := *i.anchor
	i.anchorInFlight = true
	i.paceMu.Unlock()
	err := i.startAnchor(playCtx, generation, anchor)
	if err == nil || errors.Is(err, errPacerSuperseded) || playCtx.Err() != nil {
		return true
	}
	i.recoverGeneration(generation, pacerStateForError(err), pacerCategoryForError(err))
	return true
}

func (i *Intiface) startAnchor(playCtx context.Context, generation uint64, anchor intifaceAnchor) error {
	selection, err := i.selectedDevice()
	if err != nil {
		return err
	}
	duration := intifaceAnchorDuration
	if selection.timingGap > duration {
		duration = selection.timingGap
	}
	segment := intifaceSegment{
		startMillis: anchor.timeMillis,
		duration:    duration.Milliseconds(),
		position:    anchor.position,
	}
	return i.sendLinear(playCtx, generation, time.Time{}, segment, true)
}

func (i *Intiface) handleEmptyPacerQueue(playCtx context.Context, generation uint64, coverageEnd time.Time) {
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
	stillEmpty := i.playing && i.generation == generation && len(i.queue) == 0 && !time.Now().Before(i.coverageEnd)
	i.paceMu.Unlock()
	if !stillEmpty {
		return
	}
	i.recoverGeneration(generation, "starved", "queue_starved")
}

type intifacePaceDecision struct {
	segment   intifaceSegment
	wait      time.Duration
	coalesced int
	failure   string
}

type intifacePlayStart struct {
	generation uint64
	anchorDone chan error
}

func decideIntifacePace(now, playBase time.Time, queue []intifaceSegment) intifacePaceDecision {
	decision := intifacePaceDecision{}
	for decision.coalesced < len(queue) {
		segment := queue[decision.coalesced]
		end := playBase.Add(time.Duration(segment.startMillis+segment.duration) * time.Millisecond)
		if now.Before(end) {
			break
		}
		decision.coalesced++
	}
	if decision.coalesced == len(queue) {
		decision.failure = "queue_expired"
		return decision
	}
	decision.segment = queue[decision.coalesced]
	scheduled := playBase.Add(time.Duration(decision.segment.startMillis) * time.Millisecond)
	if now.Before(scheduled) {
		decision.wait = scheduled.Sub(now)
	}
	return decision
}

func decideIntifaceLiveDuration(now, scheduledAt, scheduledEnd time.Time, originalDuration, minimumDuration time.Duration) (time.Duration, time.Duration, error) {
	if !now.Before(scheduledEnd) {
		return 0, 0, &intifacePacerError{category: "segment_expired"}
	}
	lateness := now.Sub(scheduledAt)
	if lateness < 0 {
		lateness = 0
	}
	if lateness > intifaceLateTolerance {
		return lateness, 0, &intifacePacerError{category: "live_late"}
	}
	if lateness > originalDuration/4 {
		return lateness, 0, &intifacePacerError{category: "duration_compression"}
	}
	remaining := scheduledEnd.Sub(now)
	wireSteps := (remaining + time.Millisecond - 1) / time.Millisecond
	wireDuration := wireSteps * time.Millisecond
	if wireDuration > originalDuration {
		wireDuration = originalDuration
	}
	if wireDuration <= 0 {
		return lateness, 0, &intifacePacerError{category: "segment_expired"}
	}
	if minimumDuration > 0 && wireDuration < minimumDuration {
		return lateness, 0, &intifacePacerError{category: "timing_gap"}
	}
	return lateness, wireDuration, nil
}

func (i *Intiface) sendLinear(playCtx context.Context, generation uint64, playBase time.Time, segment intifaceSegment, anchor bool) error {
	selection, err := i.selectedDevice()
	if err != nil {
		return err
	}
	duration := time.Duration(segment.duration) * time.Millisecond
	paced := intifacePacedRequest{
		position:         segment.position,
		enforceSchedule:  !anchor,
		originalDuration: duration,
		minimumDuration:  selection.timingGap,
	}
	if !anchor {
		paced.scheduledAt = playBase.Add(time.Duration(segment.startMillis) * time.Millisecond)
		paced.scheduledEnd = paced.scheduledAt.Add(duration)
	}
	payload := map[string]any{
		"DeviceIndex": selection.deviceIndex,
		"Vectors": []map[string]any{{
			"Index":    selection.actuatorIndex,
			"Duration": segment.duration,
			"Position": 0.0,
		}},
	}

	i.workerMu.Lock()
	defer i.workerMu.Unlock()
	if i.workersClosed {
		return errPacerSuperseded
	}
	request, err := i.startPacedRequest(playCtx, payload, generation, paced)
	if !request.writtenAt.IsZero() {
		i.recordLinearSent(request, segment.startMillis, selection, anchor)
		if err != nil {
			i.updateDispatch(request.id, "write_late", nil)
		}
	}
	if err != nil {
		return err
	}
	i.wg.Add(1)
	go i.awaitLinearACK(playCtx, generation, request, anchor)
	return nil
}

func (i *Intiface) awaitLinearACK(playCtx context.Context, generation uint64, request intifaceRequest, anchor bool) {
	defer i.wg.Done()
	defer i.releasePendingACK(request.linear)

	message, err := i.awaitResponse(playCtx, request)
	latency := time.Since(request.writtenAt)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, errPacerSuperseded) || playCtx.Err() != nil {
			i.updateDispatch(request.id, "superseded", nil)
			return
		}
		if errors.Is(err, errIntifaceResponseTime) {
			i.paceMu.Lock()
			i.linearTimeouts++
			i.paceMu.Unlock()
			i.updateDispatch(request.id, "timeout", nil)
			i.recoverGeneration(generation, "stale", "ack_timeout")
			return
		}
		i.updateDispatch(request.id, "stale", nil)
		i.recoverGeneration(generation, "stale", "session_stale")
		return
	}

	i.recordACKLatency(latency)
	latencyMillis := latency.Milliseconds()
	switch message.kind {
	case "Ok":
		i.paceMu.Lock()
		i.linearACKed++
		i.paceMu.Unlock()
		i.updateDispatch(request.id, "acked", &latencyMillis)
		if anchor {
			i.completeAnchor(playCtx, generation, request.writtenAt.Add(request.wireDuration))
		}
	case "Error":
		i.paceMu.Lock()
		i.linearRejected++
		i.paceMu.Unlock()
		i.updateDispatch(request.id, "rejected", &latencyMillis)
		i.recoverGeneration(generation, "rejected", "ack_rejected")
	default:
		i.paceMu.Lock()
		i.linearRejected++
		i.paceMu.Unlock()
		i.updateDispatch(request.id, "unexpected", &latencyMillis)
		i.recoverGeneration(generation, "rejected", "unexpected_ack")
	}
}

func (i *Intiface) completeAnchor(playCtx context.Context, generation uint64, completion time.Time) {
	if wait := time.Until(completion); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-playCtx.Done():
			return
		case <-timer.C:
		}
	}
	i.paceMu.Lock()
	if !i.playing || i.generation != generation || !i.anchoring || !i.anchorInFlight {
		i.paceMu.Unlock()
		return
	}
	i.anchoring = false
	i.anchorInFlight = false
	i.playBase = time.Now().Add(-i.playOffset)
	i.finishAnchorLocked(nil)
	i.paceMu.Unlock()
	i.setPlaybackState("playing")
	i.signalPacer()
}

func (i *Intiface) recoverGeneration(generation uint64, state, failure string) {
	i.stopMu.Lock()
	defer i.stopMu.Unlock()
	i.paceMu.Lock()
	if !i.playing || i.generation != generation {
		i.paceMu.Unlock()
		return
	}
	i.lastPacerFailure = failure
	i.finishAnchorLocked(&intifacePacerError{category: failure})
	i.invalidatePlaybackLocked(true)
	i.paceMu.Unlock()
	i.signalPacer()
	i.setPlaybackState(state)
	if err := i.stopSelectedDevice(context.Background()); err != nil {
		i.setSessionFailure(fmt.Errorf("Intiface recovery Stop could not be confirmed: %w", err))
	}
}

func pacerCategoryForError(err error) string {
	var pacerErr *intifacePacerError
	if errors.As(err, &pacerErr) {
		return pacerErr.category
	}
	if errors.Is(err, errIntifacePendingACKCap) {
		return "pending_ack_limit"
	}
	return "write_failed"
}

func pacerStateForError(err error) string {
	var pacerErr *intifacePacerError
	if errors.As(err, &pacerErr) {
		return "starved"
	}
	return "stale"
}

func (i *Intiface) recordLinearSent(request intifaceRequest, scheduledMillis int64, selection intifaceSelection, anchor bool) {
	i.paceMu.Lock()
	defer i.paceMu.Unlock()
	i.linearSent++
	i.lastSendLateness = request.lateness
	if request.lateness > i.maxSendLateness {
		i.maxSendLateness = request.lateness
	}
	i.lastWireDuration = request.wireDuration
	i.recentDispatches = append(i.recentDispatches, intifaceDispatch{
		requestID: request.id,
		status: IntifaceDispatchStatus{
			DeviceIndex:                 selection.deviceIndex,
			ActuatorIndex:               selection.actuatorIndex,
			StartupAnchor:               anchor,
			RelativeScheduledTimeMillis: scheduledMillis,
			ActualSendTime:              request.writtenAt.UTC().Format(time.RFC3339Nano),
			LatenessMillis:              request.lateness.Milliseconds(),
			EffectiveDurationMillis:     request.wireDuration.Milliseconds(),
			Status:                      "pending",
		},
	})
	if excess := len(i.recentDispatches) - maxIntifaceRecentDispatches; excess > 0 {
		i.recentDropped += uint64(excess)
		copy(i.recentDispatches, i.recentDispatches[excess:])
		i.recentDispatches = i.recentDispatches[:maxIntifaceRecentDispatches]
	}
}

func (i *Intiface) recordACKLatency(latency time.Duration) {
	i.paceMu.Lock()
	i.lastACKLatency = latency
	if latency > i.maxACKLatency {
		i.maxACKLatency = latency
	}
	i.paceMu.Unlock()
}

func (i *Intiface) updateDispatch(requestID uint32, status string, latencyMillis *int64) {
	i.paceMu.Lock()
	defer i.paceMu.Unlock()
	for index := len(i.recentDispatches) - 1; index >= 0; index-- {
		if i.recentDispatches[index].requestID == requestID {
			i.recentDispatches[index].status.Status = status
			if latencyMillis != nil {
				latencyCopy := *latencyMillis
				i.recentDispatches[index].status.ACKLatencyMillis = &latencyCopy
			}
			return
		}
	}
}
