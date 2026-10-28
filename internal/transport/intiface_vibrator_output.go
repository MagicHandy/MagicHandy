package transport

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/companion"
	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

// composeLocked picks this tick's level and returns a command only when a
// motor's step changes. A nonzero level waits for the command in flight and
// for the device's timing gap; a silent level is always sent at once.
func (v *IntifaceVibrator) composeLocked(now time.Time) (vibratorSend, bool) {
	if !v.selection.selected {
		return vibratorSend{}, false
	}
	level := v.targetLocked(now)
	if v.stalled.Load() {
		// The watchdog stopped this run: only silence until the next Play.
		level = 0
	}
	silent := level <= 0
	if !silent && (v.inFlight || (!v.loweredLimit && !v.lastSent.IsZero() && now.Sub(v.lastSent) < v.sendSpacingLocked())) {
		return vibratorSend{}, false
	}
	send := vibratorSend{deviceIndex: v.selection.deviceIndex, generation: v.stream.generation, epoch: v.epoch, run: v.runGeneration, steps: make([]int, len(v.selection.motors))}
	changed := false
	levels := make([]scalarLevel, 0, len(v.selection.motors))
	for index, motor := range v.selection.motors {
		step := vibration.Quantize(level, motor.StepCount, motor.last, v.outputLimitLocked(now))
		send.steps[index] = step
		changed = changed || step != motor.last
		levels = append(levels, scalarLevel{index: motor.Index, level: float64(step) / float64(motor.StepCount)})
	}
	if !changed {
		return vibratorSend{}, false
	}
	v.nextSend++
	send.serial = v.nextSend
	send.fields = encodeScalarCmd(send.deviceIndex, vibratorActuatorType, levels)
	for index := range v.selection.motors {
		v.selection.motors[index].last = send.steps[index]
	}
	v.inFlight = true
	v.inFlightID = send.serial
	v.loweredLimit = false
	return send, true
}

// targetLocked returns the current level; every composed command carries the
// current output generation, including tests and silence. Independent
// envelopes and an Advanced follow range are percentages of Max, applied
// exactly. Ordinary following reaches Minimum..level, where the level is Max
// or the model's share of it (ADR 0061).
func (v *IntifaceVibrator) targetLocked(now time.Time) float64 {
	if !v.testUntil.IsZero() {
		if now.Before(v.testUntil) {
			return min(v.config.Ceiling, vibratorTestCeiling)
		}
		v.testUntil = time.Time{}
		v.testDeadline.Store(0)
		v.playingFlag.Store(false)
		v.addEventLocked("test_ended", 0, nil)
		return 0
	}
	if !v.stream.playing {
		return 0
	}
	at := v.stream.origin.StreamMillis(now) - float64(v.config.OffsetMillis)
	drive, covered := v.stream.renderer.Drive(&v.stream.timeline, at)
	if v.stream.independent && !v.config.FollowOnly {
		drive, covered = v.stream.renderer.DriveEnvelope(&v.stream.vibration, at)
	}
	previous := v.stream.state
	v.stream.state = v.stream.coverage.Observe(now, covered)
	if v.stream.state != previous {
		v.addEventLocked("coverage", 0, nil)
	}
	v.stream.timeline.Prune(int64(at) - vibration.SpeedWindowMillis - vibratorPruneMillis)
	if v.stream.state == companion.StateStarved {
		// Stop and report: the engine reads the state and recovers.
		v.stream.playing = false
		v.playingFlag.Store(false)
		return 0
	}
	switch {
	case v.suspended:
		return 0
	case v.stream.independent && !v.config.FollowOnly:
		return drive * v.config.Ceiling
	case v.stream.followRange && !v.config.FollowOnly:
		if drive <= 0 {
			return 0
		}
		return v.config.Ceiling * (v.stream.followLow + drive*(v.stream.followHigh-v.stream.followLow))
	default:
		return vibration.Output(drive, v.config.Minimum, v.followLimitLocked(now))
	}
}

// followLimitLocked is the level that ordinary following reaches: Max, or the
// model's share of it while the model controls the level.
func (v *IntifaceVibrator) followLimitLocked(now time.Time) float64 {
	if !v.config.ModelControl {
		return v.config.Ceiling
	}
	return v.config.Ceiling * v.glide.at(now, v.effectiveLevelLocked())
}

// levelGlide moves a Simple or Intermediate level change toward its target at
// a bounded rate. The zero value applies the target at once.
type levelGlide struct {
	from  float64
	start time.Time
}

func (g levelGlide) at(now time.Time, target float64) float64 {
	if g.start.IsZero() {
		return target
	}
	moved := float64(now.Sub(g.start)) / float64(vibratorGlideFullScale)
	if g.from < target {
		return math.Min(target, g.from+moved)
	}
	return math.Max(target, g.from-moved)
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
	v.recordWritten(send)
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
	current := v.currentSendLocked(send)
	if current && v.inFlightID == send.serial {
		v.inFlight = false
	}
	if acknowledged {
		v.stats.acked++
		if current {
			v.addEventLocked("acknowledged", send.serial, nil)
		}
		return
	}
	if !current || v.inFlightID != send.serial {
		return
	}
	v.addEventLocked("not_acknowledged", send.serial, nil)
	for index := range v.selection.motors {
		if v.selection.deviceIndex == send.deviceIndex {
			v.selection.motors[index].last = -1
		}
	}
}

func (v *IntifaceVibrator) failSend(send vibratorSend, cause error, timedOut bool) {
	v.lifecycleMu.Lock()
	defer v.lifecycleMu.Unlock()
	v.mu.Lock()
	if timedOut {
		v.stats.timeouts++
	} else {
		v.stats.rejected++
	}
	selection := v.selectionSnapshotLocked()
	current := v.selection.selected && v.selection.deviceIndex == send.deviceIndex && v.epoch == send.epoch && v.runGeneration == send.run
	if current {
		v.stats.lastError = cause.Error()
		v.invalidateLocked("stale")
		v.addEventLocked("command_failed", send.serial, nil)
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
		v.selection.motors[index].written = 0
	}
	v.level = 0
	v.addEventLocked("silenced", 0, make([]int, len(v.selection.motors)))
}

// Policy edits fence prepared output just like a new run, without changing
// its timeline. The writer lock serializes limit changes with actual writes.
func (v *IntifaceVibrator) invalidateOutputLocked() {
	v.stream.generation++
	v.inFlight = false
	v.inFlightID = 0
	for index := range v.selection.motors {
		v.selection.motors[index].last = -1
	}
}

func (v *IntifaceVibrator) currentSendLocked(send vibratorSend) bool {
	return v.selection.selected && v.selection.deviceIndex == send.deviceIndex && v.epoch == send.epoch && v.stream.generation == send.generation
}

func (v *IntifaceVibrator) outputLimitLocked(now time.Time) float64 {
	limit := v.config.Ceiling
	if !v.testUntil.IsZero() {
		if !now.Before(v.testUntil) {
			return 0
		}
		return min(limit, vibratorTestCeiling)
	}
	if !v.stream.playing || v.suspended {
		return 0
	}
	return limit
}

// effectiveLimitLocked is the highest motor level at or below limit and Max.
func (v *IntifaceVibrator) effectiveLimitLocked(limit float64) float64 {
	maximum := 0.0
	for _, motor := range v.selection.motors {
		steps := float64(motor.StepCount)
		maximum = max(maximum, math.Floor(min(limit, v.config.Ceiling)*steps+1e-9)/steps)
	}
	return maximum
}

func (v *IntifaceVibrator) admitLevelsLocked(fields map[string]any) error {
	levels, ok := fields["Scalars"].([]map[string]any)
	if !ok || len(levels) != len(v.selection.motors) {
		return errPacerSuperseded
	}
	limit := v.outputLimitLocked(v.clock())
	for index, level := range levels {
		motor := v.selection.motors[index]
		value, ok := level["Scalar"].(float64)
		maximum := math.Floor(limit*float64(motor.StepCount)+1e-9) / float64(motor.StepCount)
		if !ok || math.IsNaN(value) || value < 0 || value > maximum || level["Index"] != motor.Index || level["ActuatorType"] != vibratorActuatorType || (value > 0 && v.stalled.Load()) {
			return errPacerSuperseded
		}
	}
	return nil
}

func (v *IntifaceVibrator) recordWritten(send vibratorSend) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.stats.commanded++
	if !v.currentSendLocked(send) {
		return
	}
	v.lastSent = v.clock()
	v.addEventLocked("scalar_written", send.serial, send.steps)
	v.level = 0
	for index, step := range send.steps {
		v.selection.motors[index].written = step
		v.level = max(v.level, float64(step)/float64(v.selection.motors[index].StepCount))
	}
}

func (v *IntifaceVibrator) silentReasonLocked() string {
	switch {
	case !v.selection.selected:
		return "not_selected"
	case v.stalled.Load():
		return "watchdog_stopped"
	case v.stream.state == "starving" || v.stream.state == "starved":
		return "buffer_empty"
	case v.suspended:
		return "stopped"
	case v.config.Ceiling == 0:
		return "max_zero"
	case v.effectiveLimitLocked(v.config.Ceiling) == 0:
		return "below_motor_step"
	case v.config.ModelControl && v.levelChosen && v.modelLevel == 0 && !v.stream.independent && !v.stream.followRange:
		return "level_zero"
	case !v.stream.playing && v.testUntil.IsZero():
		return "idle"
	case v.level == 0:
		return "pattern_zero"
	default:
		return ""
	}
}
