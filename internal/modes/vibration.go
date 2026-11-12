package modes

import (
	"context"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type vibrationEngine interface {
	AdmissionGeneration() uint64
	ApplyVibration(context.Context, motion.VibrationSpec, uint64) (motion.ActiveMotionState, error)
}

// A linear hold can still contain an independent scalar change. It uses the
// same engine and operation cancellation, without reapplying the held score.
func applyHeldVibration(ctx context.Context, engine Engine, spec *motion.VibrationSpec) error {
	if spec == nil {
		return nil
	}
	owner, ok := engine.(vibrationEngine)
	if !ok {
		return errors.New("engine does not support independent vibration")
	}
	_, err := owner.ApplyVibration(ctx, *spec, owner.AdmissionGeneration())
	return err
}

type tempoEngine interface {
	AdmissionGeneration() uint64
	ApplyTempo(context.Context, motion.TempoSpec, uint64) (motion.ActiveMotionState, error)
}

// applyHeldTempo changes only the tempo channel of a held choice, through the
// same engine and operation cancellation (ADR 0071).
func applyHeldTempo(ctx context.Context, engine Engine, spec *motion.TempoSpec) error {
	if spec == nil {
		return nil
	}
	owner, ok := engine.(tempoEngine)
	if !ok {
		return errors.New("engine does not support tempo")
	}
	_, err := owner.ApplyTempo(ctx, *spec, owner.AdmissionGeneration())
	return err
}

// applyHeldChannels applies a held choice's vibration, tempo and air.
func (m *Manager) applyHeldChannels(ctx context.Context, engine Engine, choice segmentChoice) error {
	if err := applyHeldVibration(ctx, engine, choice.vibration); err != nil {
		return err
	}
	if err := applyHeldTempo(ctx, engine, choice.tempo); err != nil {
		return err
	}
	return applyHeldAir(ctx, engine, choice.airSteps)
}

type airEngine interface {
	AdmissionGeneration() uint64
	ApplyAir(context.Context, int, uint64) (motion.ActiveMotionState, error)
}

func applyHeldAir(ctx context.Context, engine Engine, steps int) error {
	if steps == 0 {
		return nil
	}
	owner, ok := engine.(airEngine)
	if !ok {
		return errors.New("engine does not support air adjustments")
	}
	_, err := owner.ApplyAir(ctx, steps, owner.AdmissionGeneration())
	return err
}
