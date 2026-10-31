package motion

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// StartupRecoveryState survives a stopped run. Its opaque ID prevents a stale
// browser action from acknowledging a different failure or replacement engine.
type StartupRecoveryState struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Resetting bool   `json:"resetting"`
}

// ErrStartupRecoveryUnavailable rejects stale or concurrent reset actions.
var ErrStartupRecoveryUnavailable = errors.New("the position error changed; refresh before resetting")

var (
	errStartupReadings = errors.New("device position or calibration readings are invalid")
	errStartupMoving   = errors.New("device slider remained in motion after Stop")
)

func startupRecoveryKind(cause error) string {
	switch {
	case errors.Is(cause, errStartupReadings):
		return "invalid_telemetry"
	case errors.Is(cause, errStartupMoving):
		return "moving_after_stop"
	case errors.Is(cause, ErrStartupArrival):
		return "arrival_not_reached"
	case errors.Is(cause, ErrUnsafeStartupState):
		return "unsafe_geometry"
	}
	return ""
}

func (e *Engine) rememberStartupRecoveryLocked(cause error) {
	kind := startupRecoveryKind(cause)
	if kind != "" {
		e.startupRecovery = &StartupRecoveryState{ID: rand.Text(), Kind: kind}
	}
}

// ResetStartupPosition stops the owner and re-reads stationary geometry. It
// neither positions the slider nor resumes the failed target, and grants no
// exemption from arrival verification on the next explicit start.
func (e *Engine) ResetStartupPosition(ctx context.Context, failureID string) (ActiveMotionState, error) {
	return e.ResetStartupPositionAtGeneration(ctx, failureID, e.AdmissionGeneration())
}

// ResetStartupPositionAtGeneration admits an idle reset without retaining the
// server's lifecycle lock during device I/O. Replacement must Stop this engine,
// which either invalidates admission or cancels an already admitted reset.
func (e *Engine) ResetStartupPositionAtGeneration(ctx context.Context, failureID string, admission uint64) (ActiveMotionState, error) {
	err := e.resetStartupPosition(ctx, failureID, admission)
	return e.Snapshot(), err
}

func (e *Engine) resetStartupPosition(ctx context.Context, failureID string, admission uint64) (err error) {
	ctx, cancel := context.WithTimeout(ctx, defaultStopTimeout)
	defer cancel()
	e.mu.Lock()
	if ctx.Err() != nil {
		e.mu.Unlock()
		return ctx.Err()
	}
	if e.stopGeneration != admission || e.startupRecovery == nil || e.startupRecovery.ID != failureID ||
		e.startupRecovery.Resetting || e.running || e.starting || e.completing || e.stopBarriers > 0 {
		e.mu.Unlock()
		return ErrStartupRecoveryUnavailable
	}
	e.startupRecovery.Resetting = true
	e.startupResetCancel = cancel
	e.stopBarriers++ // New starts cannot cross the stop-and-read operation.
	e.paused = false
	e.runMillisAccum = 0
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.startupResetCancel = nil
		e.endStopBarrierLocked()
		if e.startupRecovery != nil && e.startupRecovery.ID == failureID {
			e.startupRecovery.Resetting = false
			if e.stopGeneration != admission || ctx.Err() != nil {
				if err == nil {
					err = context.Canceled
				}
				return
			}
			if err != nil {
				e.lastError = err.Error()
				if kind := startupRecoveryKind(err); kind != "" {
					e.startupRecovery.Kind = kind
				}
				return
			}
			e.startupRecovery = nil
			e.lastError = ""
			e.traceStateLocked("startup_position_reset", "position_rechecked=true;playback_resumed=false")
		}
	}()
	return e.resetStartupOwners(ctx)
}

func (e *Engine) resetStartupOwners(ctx context.Context) error {
	// Stop can cancel the read before waiting for commandMu. It remains the
	// final wire command if it arrives during this reset.
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	command := transport.StopCommand{Reason: "startup_position_reset"}
	result, err := e.transport.Stop(ctx, command)
	if err == nil && !result.OK {
		err = errors.New("device Stop was not confirmed; position was not reset")
	}
	e.recordTransportResult("startup_reset_stop", nil, transport.Command{Kind: transport.CommandKindStop, Stop: &command}, result, err)
	e.rememberResult(result, err)
	if err != nil {
		return err
	}
	owners := []transport.LinkedParticipant{{ID: "device", Label: "Device", Transport: e.transport}}
	if group, ok := e.transport.(transport.MotionStartupParticipantsProvider); ok {
		owners = group.MotionStartupParticipants()
	}
	checked := 0
	for _, owner := range owners {
		provider, ok := owner.Transport.(transport.MotionStartupStateProvider)
		if !ok {
			continue
		}
		if err := e.recheckStartupPosition(ctx, provider, "startup_reset_"+owner.ID); err != nil {
			return fmt.Errorf("%s: %w", owner.Label, err)
		}
		checked++
	}
	if checked == 0 {
		return errors.New("the connected device cannot report its position; reconnect it before trying again")
	}
	return ctx.Err()
}

// Caller owns commandMu; unlike startup, this read is valid only while stopped.
func (e *Engine) recheckStartupPosition(ctx context.Context, provider transport.MotionStartupStateProvider, reason string) error {
	for attempt := range startupStationaryAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, results, err := provider.ReadMotionStartupState(ctx)
		e.recordStartupObservation(reason, state, results, err)
		if err != nil {
			return err
		}
		if _, err := checkedStartupCalibration(state); err != nil {
			return err
		}
		if state.SpeedAbsolute <= startupStationarySpeedAbsolute {
			return nil
		}
		if attempt+1 < startupStationaryAttempts {
			if err := e.startupWait(ctx, startupStationaryRetryDelay); err != nil {
				return err
			}
		}
	}
	return errStartupMoving
}

func checkedStartupCalibration(state transport.MotionStartupState) (startupCalibration, error) {
	if err := validateStartupState(state); err != nil {
		return startupCalibration{}, fmt.Errorf("%w: %w: %v", ErrUnsafeStartupState, errStartupReadings, err)
	}
	calibration, err := newStartupCalibration(state)
	if err != nil {
		return startupCalibration{}, fmt.Errorf("%w: %w: %v", ErrUnsafeStartupState, errStartupReadings, err)
	}
	observed := calibration.fullPercentAt(state.PositionAbsolute)
	if observed < -startupCalibrationTolerancePercent || observed > 100+startupCalibrationTolerancePercent {
		return startupCalibration{}, fmt.Errorf(
			"%w: motion startup absolute position was outside calibrated full travel "+
				"(position %.2f maps to %.1f%% of travel; device reported slide %.1f-%.1f%% as %.2f-%.2f)",
			ErrUnsafeStartupState, state.PositionAbsolute, observed,
			state.StrokeMinPercent, state.StrokeMaxPercent, state.StrokeMinAbsolute, state.StrokeMaxAbsolute)
	}
	return calibration, nil
}
