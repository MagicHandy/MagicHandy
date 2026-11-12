package transport

import (
	"context"
	"errors"
)

// appendAir is a one-time adjustment on the engine's admitted stream, never a
// second motion path. The pulse's cleanup still runs when its caller cancels.
func (v *VacuGlide) appendAir(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	v.valveMu.Lock()
	defer v.valveMu.Unlock()
	v.mu.Lock()
	recorded := Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}
	generation, err := v.admitAirLocked(ctx, command)
	if err != nil || generation == 0 {
		result, err := v.recordLocked(recorded, err)
		v.mu.Unlock()
		return result, err
	}
	v.mu.Unlock()
	for range max(command.Air.Steps, -command.Air.Steps) {
		if err = v.pulseLocked(ctx, generation, command.Air.Steps > 0, true); err != nil {
			break
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.recordLocked(recorded, err)
}

func (v *VacuGlide) admitAirLocked(ctx context.Context, command AppendPointsCommand) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := v.checkSessionLocked(ctx); err != nil {
		return 0, err
	}
	if v.client == nil {
		return 0, errAutoblowNotConnected
	}
	air := command.Air
	if air == nil || air.Revision == 0 || air.Steps == 0 || air.Steps < -2 || air.Steps > 2 || command.Tempo != nil || len(command.Points) != 0 {
		return 0, errors.Join(ErrAirRejected, errors.New("an air adjustment needs one or two relative steps and its own revision"))
	}
	if v.stopHolds > 0 || v.state == "stale" || command.StreamID != v.stream.id {
		return 0, errAutoblowSuperseded
	}
	if air.Revision <= v.stream.airRevision {
		return 0, nil
	}
	if err := v.admitMotionAirLocked(air.Steps > 0, true); err != nil {
		return 0, errors.Join(ErrAirRejected, err)
	}
	if next := v.airSteps + air.Steps; next < -vacuglideMotionAirSteps || next > vacuglideMotionAirSteps {
		return 0, errors.Join(ErrAirRejected, errVacuGlideAirLimit)
	}
	if next := v.steps + air.Steps; next < -vacuglideMaxSteps || next > vacuglideMaxSteps {
		return 0, errors.Join(ErrAirRejected, errVacuGlideStrokeLimit)
	}
	// Consume the revision before any write: an uncertain reply must not cause
	// the same adjustment to be sent again.
	v.stream.airRevision = air.Revision
	return v.generation, nil
}
