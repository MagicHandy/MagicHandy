package motion

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func tempoOnlyMotionPlan(id string, target MotionTarget, createdAt time.Time) MotionPlan {
	if id == "" {
		id = fmt.Sprintf("tempo-%d", createdAt.UnixNano())
	}
	// The normal engine clock, coverage and Stop still own the run. Its
	// inert position channel never becomes a phantom Handy pattern.
	return MotionPlan{ID: id, Target: target, Loop: true, PeriodMillis: minimumBurstCycleMillis,
		CreatedAt: createdAt.UTC().Format(time.RFC3339Nano), curve: (resolvedContent{}).stationaryFallbackCurve(),
		focus: focusProjection{sourceMin: 0, sourceSpan: 100, targetMin: 0, targetSpan: 100}}
}

const (
	tempoFrameStepMillis   = 100
	tempoFrameBehindMillis = 500
	tempoFrameAheadMillis  = 2500
	tempoRefillMillis      = 1500
	// Follow reads travel over a two-second window centered on each point,
	// then averages that over a second: the trapezoid neither lags the motion
	// nor hunts between strokes whose period does not divide the window.
	tempoFollowHalfWindowMillis = 1000
	tempoFollowSmoothMillis     = 500
	tempoFollowSampleMillis     = 25
	tempoReadoutAheadSeconds    = 8
)

// TempoReadout is the compact backend-authoritative tempo for clients: the
// level now and, for an envelope, the next seconds. It carries no payload.
type TempoReadout struct {
	Mode         string    `json:"mode"`
	Routine      string    `json:"routine,omitempty"`
	LevelPercent float64   `json:"level_percent"`
	Ahead        []float64 `json:"ahead,omitempty"`
}

func takesTempo(owner transport.Transport) bool {
	output, ok := owner.(transport.TempoOutput)
	return ok && output.TakesTempo()
}

// installTempoLocked prepares the run's tempo at stream time 0. Positions
// lead by default, so a run with a tempo device and no explicit tempo
// follows them; a tempo-only run must say what to play.
func (e *Engine) installTempoLocked(target *MotionTarget) error {
	e.tempo, e.tempoTail = nil, 0
	e.tempoOutput = takesTempo(e.transport)
	if target.TempoOnly && !e.tempoOutput {
		return transport.ErrTempoUnavailable
	}
	if !e.tempoOutput {
		return nil
	}
	spec := TempoSpec{Mode: TempoModeFollow}
	if target.Tempo != nil {
		spec = target.Tempo.Clone()
	}
	if err := validateRunTempo(spec, target.TempoOnly); err != nil {
		return err
	}
	e.setTempoLocked(spec, 0, 0)
	return nil
}

// validateTargetTempo checks a start before the engine changes any state.
func validateTargetTempo(target MotionTarget, output bool) error {
	switch {
	case target.TempoOnly && !output:
		return transport.ErrTempoUnavailable
	case target.TempoOnly && target.Tempo == nil:
		return errors.New("a tempo-only run needs a tempo")
	case target.Tempo != nil && output:
		return validateRunTempo(*target.Tempo, target.TempoOnly)
	}
	return nil
}

func validateRunTempo(spec TempoSpec, tempoOnly bool) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if tempoOnly && spec.Mode == TempoModeFollow {
		return errors.New("tempo can follow movement only while positions play")
	}
	return nil
}

func (e *Engine) setTempoLocked(spec TempoSpec, at int64, startLevel float64) {
	e.tempoRevision++
	e.tempo = newTempoPlan(spec, at, startLevel, e.tempoRevision)
	e.tempoTail = 0
	snapshot := spec.Clone()
	e.plan.Target.Tempo = &snapshot
}

// tempoLevelLocked is the level the current tempo plays at stream time at.
func (e *Engine) tempoLevelLocked(at int64) float64 {
	switch {
	case e.tempo == nil:
		return 0
	case e.tempo.follow():
		levels := e.followTempoLevelsLocked(at, at, tempoFollowSampleMillis*2)
		if len(levels) == 0 {
			return 0
		}
		return levels[0].LevelPercent
	default:
		return e.tempo.LevelAt(at)
	}
}

// followTempoLevelsLocked derives tempo from the planned positions on the
// selected Handy model's scale: 100 is that model's fastest full-span pace,
// so the Handy setting never slows the tempo device. Travel below level 1 is
// a hold, which stops the motor.
func (e *Engine) followTempoLevelsLocked(from, to, sampleMillis int64) []transport.TimedLevel {
	top := referenceTravelRateForSpeed(100, e.settings.HandyModel)
	reach := int64(tempoFollowHalfWindowMillis + tempoFollowSmoothMillis)
	start := max(int64(0), from-reach)
	end := to + reach
	count := int((end-start)/sampleMillis) + 1
	travel := make([]float64, count)
	previous := sampleMotionPath(e.plan, e.transition, start).PositionPercent
	for index := 1; index < count; index++ {
		position := sampleMotionPath(e.plan, e.transition, start+int64(index)*sampleMillis).PositionPercent
		travel[index] = travel[index-1] + math.Abs(position-previous)
		previous = position
	}
	rate := func(at int64) float64 {
		first := int((max(start, at-tempoFollowHalfWindowMillis) - start) / sampleMillis)
		last := min(count-1, int((at+tempoFollowHalfWindowMillis-start)/sampleMillis))
		span := float64(int64(last-first)*sampleMillis) / 1000
		if span <= 0 {
			return 0
		}
		return (travel[last] - travel[first]) / span
	}
	var levels []transport.TimedLevel
	for at := from; at <= to; at += tempoFrameStepMillis {
		sum, samples := 0.0, 0
		for offset := -int64(tempoFollowSmoothMillis); offset <= tempoFollowSmoothMillis; offset += tempoFrameStepMillis {
			sum += rate(max(0, at+offset))
			samples++
		}
		level := 0.0
		if top > 0 {
			level = sum / float64(samples) / top * 100
		}
		if level < 1 {
			level = 0
		}
		levels = append(levels, transport.TimedLevel{TimeMillis: at, LevelPercent: math.Min(100, level)})
	}
	return levels
}

// tempoFrameLocked samples the tempo an owner needs next. An envelope covers
// a short window around playback; follow covers the positions sent with it.
// A new revision always covers playback, so the owner replaces what it held.
func (e *Engine) tempoFrameLocked(playback int64, points []transport.TimedPoint) *transport.TempoFrame {
	if e.tempo == nil || !e.tempoOutput {
		return nil
	}
	frame := &transport.TempoFrame{Revision: e.tempo.revision}
	from := max(int64(0), playback/tempoFrameStepMillis*tempoFrameStepMillis-tempoFrameBehindMillis)
	if e.tempo.follow() {
		to := e.tempoTail
		if len(points) > 0 {
			to = max(to, points[len(points)-1].TimeMillis)
			if e.tempoTail > 0 {
				from = max(from, alignDown(points[0].TimeMillis))
			}
		}
		if to <= from {
			return nil
		}
		frame.Points = e.followTempoLevelsLocked(from, alignUp(to), tempoFollowSampleMillis)
		e.tempoTail = to
		return frame
	}
	to := playback + tempoFrameAheadMillis
	for at := from; at <= to; at += tempoFrameStepMillis {
		frame.Points = append(frame.Points, transport.TimedLevel{TimeMillis: at, LevelPercent: e.tempo.LevelAt(at)})
	}
	e.tempoTail = to
	return frame
}

func alignDown(millis int64) int64 { return millis / tempoFrameStepMillis * tempoFrameStepMillis }

func alignUp(millis int64) int64 {
	return (millis + tempoFrameStepMillis - 1) / tempoFrameStepMillis * tempoFrameStepMillis
}

// ApplyTempo changes only the tempo channel. It neither retargets positions
// nor starts idle outputs; admission and Stop use the same engine.
func (e *Engine) ApplyTempo(ctx context.Context, spec TempoSpec, admission uint64) (ActiveMotionState, error) {
	return e.applyTempo(ctx, spec, admission, nil)
}

// ApplyTempoIfCurrent rejects a delayed repair after any newer motion edit.
func (e *Engine) ApplyTempoIfCurrent(ctx context.Context, spec TempoSpec, admission, generation uint64) (ActiveMotionState, error) {
	return e.applyTempo(ctx, spec, admission, &generation)
}

func (e *Engine) applyTempo(ctx context.Context, spec TempoSpec, admission uint64, generation *uint64) (ActiveMotionState, error) {
	if err := spec.Validate(); err != nil {
		return e.Snapshot(), err
	}
	e.targetMu.Lock()
	defer e.targetMu.Unlock()
	e.commandMu.Lock()
	if err := ctx.Err(); err != nil {
		e.commandMu.Unlock()
		return e.Snapshot(), err
	}
	e.mu.Lock()
	if !e.running || e.starting || e.stopGeneration != admission || (generation != nil && e.generation != *generation) {
		e.mu.Unlock()
		e.commandMu.Unlock()
		return e.Snapshot(), errRunInvalidated
	}
	if !e.tempoOutput {
		e.mu.Unlock()
		e.commandMu.Unlock()
		return e.Snapshot(), transport.ErrTempoUnavailable
	}
	if err := validateRunTempo(spec, e.plan.Target.TempoOnly); err != nil {
		e.mu.Unlock()
		e.commandMu.Unlock()
		return e.Snapshot(), err
	}
	epoch := e.runEpoch
	e.generation++
	at := e.estimatedPlaybackMillisLocked(e.now())
	e.setTempoLocked(spec, at, e.tempoLevelLocked(at))
	if spec.Mode == TempoModeFollow {
		// Cover what is already buffered, replacing the envelope there.
		e.tempoTail = e.bufferedTailMillisLocked()
	}
	e.mu.Unlock()
	// Like vibration: once admitted, the replacement completes on the run's
	// context; Stop still cancels the run and fences every command.
	err := e.sendTempoLocked(context.WithoutCancel(ctx), epoch)
	e.commandMu.Unlock()
	if err != nil && !errors.Is(err, transport.ErrTempoUnavailable) && e.validateRun(epoch) == nil {
		err = errors.Join(err, e.stopForRecovery(ctx, epoch, "tempo_dispatch_failed", err.Error()))
	}
	return e.Snapshot(), err
}

// Called by the ordinary engine loop, not by another motion scheduler.
func (e *Engine) dispatchTempoIfNeeded(ctx context.Context, epoch uint64) error {
	e.commandMu.Lock()
	e.mu.Lock()
	needed := e.tempo != nil && e.tempoOutput && !e.tempo.follow() && e.tempoTail-e.estimatedPlaybackMillisLocked(e.now()) < tempoRefillMillis
	e.mu.Unlock()
	if !needed {
		e.commandMu.Unlock()
		return nil
	}
	err := e.sendTempoLocked(ctx, epoch)
	e.commandMu.Unlock()
	if errors.Is(err, transport.ErrTempoUnavailable) {
		return nil
	}
	if err != nil && e.validateRun(epoch) == nil {
		return errors.Join(err, e.stopForRecovery(ctx, epoch, "tempo_dispatch_failed", err.Error()))
	}
	return err
}

func (e *Engine) sendTempoLocked(ctx context.Context, epoch uint64) error {
	commandCtx, cleanup, err := e.contextForRun(ctx, epoch)
	if err != nil {
		return err
	}
	defer cleanup()
	e.mu.Lock()
	if err := e.validateRunLocked(epoch); err != nil {
		e.mu.Unlock()
		return err
	}
	frame := e.tempoFrameLocked(e.estimatedPlaybackMillisLocked(e.now()), nil)
	if frame == nil {
		e.mu.Unlock()
		return nil
	}
	command := transport.AppendPointsCommand{StreamID: e.streamID, Tempo: frame, TempoOnly: e.plan.Target.TempoOnly}
	e.mu.Unlock()
	result, err := e.transport.AppendPoints(commandCtx, command)
	if errors.Is(err, transport.ErrTempoUnavailable) {
		e.mu.Lock()
		if e.runEpoch == epoch {
			e.tempoOutput = false
		}
		e.mu.Unlock()
	}
	e.recordTransportResult("tempo_envelope", nil, transport.Command{Kind: transport.CommandKindPointsAdd, PointsAdd: &command}, result, err)
	e.rememberResult(result, err)
	return err
}

// tempoReadoutLocked is the client view of the tempo channel.
func (e *Engine) tempoReadoutLocked(playback int64) *TempoReadout {
	if e.tempo == nil || !e.tempoOutput || (!e.running && !e.paused) {
		return nil
	}
	readout := &TempoReadout{Mode: e.tempo.spec.Mode, Routine: e.tempo.spec.Routine}
	if e.paused {
		return readout
	}
	readout.LevelPercent = math.Round(e.tempoLevelLocked(playback)*10) / 10
	if !e.tempo.follow() {
		for second := int64(1); second <= tempoReadoutAheadSeconds; second++ {
			readout.Ahead = append(readout.Ahead, math.Round(e.tempo.LevelAt(playback+second*1000)*10)/10)
		}
	}
	return readout
}
