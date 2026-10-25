package motion

import (
	"context"
	"errors"
	"math"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// VibrationSpec is relative drive inside the human's armed intensity, never a
// hardware level or permission to raise that intensity. It drives all motors.
type VibrationSpec struct {
	Mode          string  `json:"mode"`
	MinPercent    int     `json:"min_percent"`
	MaxPercent    int     `json:"max_percent"`
	PeriodSeconds float64 `json:"period_seconds"`
}

// Validate rejects malformed envelopes without changing their requested shape.
func (s VibrationSpec) Validate() error {
	if s.MinPercent < 0 || s.MaxPercent > 100 || s.MinPercent > s.MaxPercent || math.IsNaN(s.PeriodSeconds) || math.IsInf(s.PeriodSeconds, 0) {
		return errors.New("vibration requires 0 <= min_percent <= max_percent <= 100 and a finite period_seconds")
	}
	switch s.Mode {
	case "follow", "off":
		if s.MinPercent != 0 || s.MaxPercent != 0 || s.PeriodSeconds != 0 {
			return errors.New("vibration follow/off requires min_percent, max_percent and period_seconds all 0")
		}
	case "steady":
		if s.MinPercent != s.MaxPercent || s.PeriodSeconds != 0 {
			return errors.New("steady vibration requires equal min/max_percent and period_seconds 0")
		}
	case "wave", "pulse":
		if s.PeriodSeconds < 0.5 || s.PeriodSeconds > 30 {
			return errors.New("vibration wave/pulse period_seconds must be 0.5..30")
		}
	default:
		return errors.New("vibration mode must be follow, off, steady, wave or pulse")
	}
	return nil
}

type vibrationPlan struct {
	spec       VibrationSpec
	fromMillis int64
	fromLevel  float64
	revision   uint64
}

// LevelAt samples smooth, bounded drive. Pulse has a plateau and rounded
// edges; wave is a continuous sinusoidal swell. Off is immediately silent.
func (p vibrationPlan) LevelAt(at int64) float64 {
	if p.spec.Mode == "off" || p.spec.Mode == "follow" {
		return 0
	}
	level := float64(p.spec.MaxPercent)
	if p.spec.Mode == "wave" || p.spec.Mode == "pulse" {
		phase := math.Mod(math.Max(0, float64(at-p.fromMillis)), p.spec.PeriodSeconds*1000) / (p.spec.PeriodSeconds * 1000)
		x := (1 - math.Cos(2*math.Pi*phase)) / 2
		if p.spec.Mode == "pulse" {
			x = smoothVibration(clampVibration((x - 0.25) * 2))
		}
		level = float64(p.spec.MinPercent) + float64(p.spec.MaxPercent-p.spec.MinPercent)*x
	}
	weight := smoothVibration(clampVibration(float64(at-p.fromMillis) / 600))
	return p.fromLevel + (level-p.fromLevel)*weight
}

func clampVibration(x float64) float64  { return math.Max(0, math.Min(1, x)) }
func smoothVibration(x float64) float64 { return x * x * (3 - 2*x) }

func (e *Engine) setVibrationLocked(spec VibrationSpec, at int64) {
	from := 0.0
	if e.vibration != nil {
		from = e.vibration.LevelAt(at)
	}
	e.vibrationRevision++
	e.vibration = &vibrationPlan{spec: spec, fromMillis: at, fromLevel: from, revision: e.vibrationRevision}
	snapshot := spec
	e.plan.Target.Vibration = &snapshot
}

func (e *Engine) vibrationFrameLocked(at int64) *transport.VibrationFrame {
	if e.vibration == nil {
		return nil
	}
	frame := &transport.VibrationFrame{Revision: e.vibration.revision, Follow: e.vibration.spec.Mode == "follow"}
	if !frame.Follow {
		// Cover the saved +/-300ms timing offset and pacer scheduling jitter.
		for t := max(int64(0), at/100*100-500); t <= max(int64(0), at)+2500; t += 100 {
			frame.Points = append(frame.Points, transport.TimedLevel{TimeMillis: t, LevelPercent: math.Max(0, math.Min(100, e.vibration.LevelAt(t)))})
		}
	}
	e.vibrationTail = at + 2500
	return frame
}

// ApplyVibration changes only the scalar envelope. It neither retargets the
// linear plan nor starts idle outputs; admission and Stop use the same engine.
func (e *Engine) ApplyVibration(ctx context.Context, spec VibrationSpec, admission uint64) (ActiveMotionState, error) {
	return e.applyVibration(ctx, spec, admission, nil)
}

// ApplyVibrationIfCurrent rejects a delayed repair after any newer motion edit.
func (e *Engine) ApplyVibrationIfCurrent(ctx context.Context, spec VibrationSpec, admission, generation uint64) (ActiveMotionState, error) {
	return e.applyVibration(ctx, spec, admission, &generation)
}

func (e *Engine) applyVibration(ctx context.Context, spec VibrationSpec, admission uint64, generation *uint64) (ActiveMotionState, error) {
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
	epoch := e.runEpoch
	e.generation++
	e.setVibrationLocked(spec, e.estimatedPlaybackMillisLocked(e.now()))
	e.mu.Unlock()
	// Once admitted, this local buffer replacement completes on the run's
	// context. A canceled chat/planner turn is not a hardware failure; Stop
	// still cancels the run context and fences every outgoing command.
	err := e.sendVibrationLocked(context.WithoutCancel(ctx), epoch)
	e.commandMu.Unlock()
	if err != nil && !errors.Is(err, transport.ErrVibrationUnavailable) && e.validateRun(epoch) == nil {
		err = errors.Join(err, e.stopForRecovery(ctx, epoch, "vibration_dispatch_failed", err.Error()))
	}
	return e.Snapshot(), err
}

// Called by the ordinary engine loop, not by another motion scheduler.
func (e *Engine) dispatchVibrationIfNeeded(ctx context.Context, epoch uint64) error {
	e.commandMu.Lock()
	e.mu.Lock()
	needed := e.vibration != nil && e.vibration.spec.Mode != "follow" && e.vibrationTail-e.estimatedPlaybackMillisLocked(e.now()) < 1500
	e.mu.Unlock()
	if !needed {
		e.commandMu.Unlock()
		return nil
	}
	err := e.sendVibrationLocked(ctx, epoch)
	e.commandMu.Unlock()
	if errors.Is(err, transport.ErrVibrationUnavailable) {
		return nil
	}
	if err != nil && e.validateRun(epoch) == nil {
		return errors.Join(err, e.stopForRecovery(ctx, epoch, "vibration_dispatch_failed", err.Error()))
	}
	return err
}

func (e *Engine) sendVibrationLocked(ctx context.Context, epoch uint64) error {
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
	command := transport.AppendPointsCommand{StreamID: e.streamID, Vibration: e.vibrationFrameLocked(e.estimatedPlaybackMillisLocked(e.now()))}
	e.mu.Unlock()
	result, err := e.transport.AppendPoints(commandCtx, command)
	if errors.Is(err, transport.ErrVibrationUnavailable) {
		e.mu.Lock()
		if e.runEpoch == epoch {
			e.vibration = nil
			e.plan.Target.Vibration = nil
		}
		e.mu.Unlock()
	}
	e.recordTransportResult("vibration_envelope", nil, transport.Command{Kind: transport.CommandKindPointsAdd, PointsAdd: &command}, result, err)
	e.rememberResult(result, err)
	return err
}
