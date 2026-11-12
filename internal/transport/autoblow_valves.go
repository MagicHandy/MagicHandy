package transport

import (
	"context"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

// StrokeStepAt opens one valve for a short pulse: longer draws air out of
// the sleeve for a longer, tighter stroke; shorter lets air in. It works
// whether or not motion plays, and Stop closes the valve if it is still open.
func (v *VacuGlide) StrokeStepAt(generation uint64, longer bool) error {
	if generation == 0 {
		generation = v.ControlGeneration()
	}
	return v.pulse(generation, longer, false)
}

// pulse opens a valve, waits, and always closes it again.
func (v *VacuGlide) pulse(generation uint64, longer, motion bool) error {
	v.valveMu.Lock()
	defer v.valveMu.Unlock()
	return v.pulseLocked(context.Background(), generation, longer, motion)
}

func (v *VacuGlide) pulseLocked(parent context.Context, generation uint64, longer, motion bool) error {
	if err := parent.Err(); err != nil {
		return err
	}
	client, loops, err := v.admitValveStep(generation, longer, motion)
	if err != nil {
		return err
	}
	defer loops.done.Done()
	valve, step := autoblow.ValveStrokeMinus, -1
	if longer {
		valve, step = autoblow.ValveStrokePlus, 1
	}
	attempted := false
	err = v.commandBound(client, generation, autoblowCommandTimeout, func(ctx context.Context, client *autoblow.Client) error {
		if err := parent.Err(); err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(ctx)
		stopCancellation := context.AfterFunc(parent, cancel)
		defer func() { stopCancellation(); cancel() }()
		attempted = true
		v.mu.Lock()
		v.valveOpen = valve
		// Count an attempted opening conservatively: a timeout or Stop may
		// arrive after the device accepted it. Neither permits another step.
		v.steps += step
		if motion {
			v.airSteps += step
		}
		v.mu.Unlock()
		_, err := client.Valve(ctx, valve, true)
		return err
	})
	if !attempted {
		return err
	}
	if err == nil {
		timer := time.NewTimer(vacuglideValvePulse)
		select {
		case <-timer.C:
		case <-loops.stop:
		case <-parent.Done():
		}
		timer.Stop()
	}
	// A timeout can mean the cloud applied the opening but lost the reply.
	// Cleanup must still run, and must never target a replacement connection.
	closeErr := v.commandBound(client, 0, autoblowStopTimeout, func(ctx context.Context, client *autoblow.Client) error {
		_, err := client.Valve(ctx, valve, false)
		return err
	})
	v.mu.Lock()
	if v.client == client {
		if closeErr == nil {
			v.valveOpen = ""
		}
		if combined := errors.Join(err, closeErr); combined != nil {
			v.lastError = combined.Error()
			go v.failConnection(client, combined)
		}
	}
	v.mu.Unlock()
	return errors.Join(err, closeErr, parent.Err())
}

// Admit the entire pulse before retirement can join the connection workers.
func (v *VacuGlide) admitValveStep(generation uint64, longer, motion bool) (*autoblow.Client, *autoblowLoops, error) {
	v.mu.Lock()
	if v.client == nil {
		v.mu.Unlock()
		return nil, nil, errAutoblowNotConnected
	}
	if v.generation != generation || v.stopHolds > 0 || v.state == "stale" {
		v.mu.Unlock()
		return nil, nil, errAutoblowSuperseded
	}
	if longer && v.steps >= vacuglideMaxSteps || !longer && v.steps <= -vacuglideMaxSteps {
		v.mu.Unlock()
		return nil, nil, errVacuGlideStrokeLimit
	}
	if err := v.admitMotionAirLocked(longer, motion); err != nil {
		v.mu.Unlock()
		return nil, nil, err
	}
	client, loops := v.client, v.loops
	loops.done.Add(1)
	v.mu.Unlock()
	return client, loops, nil
}

// admitMotionAirLocked keeps motion's valve steps inside the person's
// permission and the session's bound. Manual steps are not limited here.
func (v *VacuGlide) admitMotionAirLocked(longer, motion bool) error {
	switch {
	case !motion:
		return nil
	case !v.config.MotionAir:
		return errVacuGlideAirOff
	case !v.stream.origin.Playing():
		return errVacuGlideNotPlaying
	case longer && v.airSteps >= vacuglideMotionAirSteps, !longer && v.airSteps <= -vacuglideMotionAirSteps:
		return errVacuGlideAirLimit
	}
	return nil
}
