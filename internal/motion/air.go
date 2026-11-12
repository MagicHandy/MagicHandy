package motion

import (
	"context"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// ApplyAir admits a one-time semantic air adjustment through the running
// engine. It never starts an idle output and cannot cross Stop or Pause.
func (e *Engine) ApplyAir(ctx context.Context, steps int, admission uint64) (ActiveMotionState, error) {
	if steps == 0 || steps < -2 || steps > 2 {
		return e.Snapshot(), errors.New("air adjustment must be one or two relative steps")
	}
	e.targetMu.Lock()
	defer e.targetMu.Unlock()
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	if err := ctx.Err(); err != nil {
		return e.Snapshot(), err
	}
	e.mu.Lock()
	if !e.running || e.paused || e.starting || e.stopGeneration != admission {
		e.mu.Unlock()
		return e.Snapshot(), errRunInvalidated
	}
	if !e.tempoOutput {
		e.mu.Unlock()
		return e.Snapshot(), transport.ErrTempoUnavailable
	}
	e.generation++
	epoch := e.runEpoch
	command := transport.AppendPointsCommand{StreamID: e.streamID, TempoOnly: e.plan.Target.TempoOnly,
		Air: &transport.AirAdjustment{Revision: e.generation, Steps: steps}}
	e.mu.Unlock()
	commandCtx, cleanup, err := e.contextForRun(ctx, epoch)
	if err != nil {
		return e.Snapshot(), err
	}
	defer cleanup()
	result, err := e.transport.AppendPoints(commandCtx, command)
	e.recordTransportResult("air_adjustment", nil, transport.Command{Kind: transport.CommandKindPointsAdd, PointsAdd: &command}, result, err)
	e.rememberResult(result, err)
	return e.Snapshot(), err
}
