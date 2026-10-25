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
