package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

var errVibrationUnavailable = errors.New("vibration is unavailable")

func (s *Server) vibrationReadiness() error {
	settings, _ := s.store.Snapshot()
	if !settings.Device.Plays(config.DeviceVibrator) {
		return fmt.Errorf("%w: include the vibrator in playback first", errVibrationUnavailable)
	}
	owner, err := s.currentVibrator()
	if err != nil {
		return fmt.Errorf("%w: connect and select the vibrator first", errVibrationUnavailable)
	}
	state := owner.Status()
	if !state.Selected || state.PlaybackState == "stale" {
		return fmt.Errorf("%w: connect and select the vibrator first", errVibrationUnavailable)
	}
	if s.vibratorRetired() {
		return fmt.Errorf("%w: it left this run; start a new session to rejoin", errVibrationUnavailable)
	}
	return nil
}

// This explicit UI action returns to the saved profile without granting the
// model arming authority or starting an idle engine.
func (s *Server) handleVibrationFollow(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	finishUnreadBody(w, r)
	if err := s.vibrationReadiness(); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	release, err := s.beginDeferredMotion(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	defer release()
	engine := s.currentMotionEngine()
	if engine == nil {
		writeError(w, http.StatusConflict, errors.New("playback is not running"))
		return
	}
	spec := motion.VibrationSpec{Mode: "follow"}
	target := engine.Snapshot().Target
	target.Vibration = &spec
	state, err := s.applyChatTargetHandoff(target, func() (motion.ActiveMotionState, error) {
		return engine.ApplyVibration(r.Context(), spec, engine.AdmissionGeneration())
	})
	s.writeMotionResult(w, r, state, err)
}

func (s *Server) validateChatVibration(spec *motion.VibrationSpec) error {
	settings, _ := s.store.Snapshot()
	return s.validateModelVibration(spec, settings.LLM.ConversationSettings())
}

func (s *Server) validateModelVibration(spec *motion.VibrationSpec, model config.LLMSettings) error {
	if spec == nil {
		return nil
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	settings, _ := s.store.Snapshot()
	if !model.ModelFeatures().IndependentVibration || !settings.Device.Plays(config.DeviceVibrator) || !settings.Vibration.ModelSetsLevel() {
		return errors.New("independent vibration is unavailable in the current playback/model configuration")
	}
	if err := s.vibrationReadiness(); err != nil {
		return err
	}
	owner, _ := s.currentVibrator()
	status := owner.Status()
	if spec.Mode != "off" && status.Suspended {
		return fmt.Errorf("%w: Stop paused it; send a new message or choose Resume now in the device panel", errVibrationUnavailable)
	}
	if spec.Mode != "off" && status.EffectiveCeilingPercent == 0 {
		return fmt.Errorf("%w: raise its Max in the device panel first; the model cannot raise it", errVibrationUnavailable)
	}
	return nil
}

// applyChatVibrationLevel records a Simple or Intermediate level. It glides
// toward the new value and never starts or reshapes motion (ADR 0061).
func (s *Server) applyChatVibrationLevel(level int) error {
	if level < 0 || level > 100 {
		return errors.New("vibration_level must be between 0 and 100")
	}
	settings, _ := s.store.Snapshot()
	if !settings.LLM.ConversationSettings().ModelFeatures().VibrationLevel || !settings.Vibration.ModelSetsLevel() {
		return fmt.Errorf("%w: the chat model cannot set the vibration level", errVibrationUnavailable)
	}
	if err := s.vibrationReadiness(); err != nil {
		return err
	}
	if engine := s.currentMotionEngine(); engine != nil && engine.Snapshot().Paused {
		return errors.New("playback is paused; resume it in the app before changing vibration")
	}
	owner, err := s.currentVibrator()
	if err != nil {
		return fmt.Errorf("%w: connect and select the vibrator first", errVibrationUnavailable)
	}
	if err := owner.SetModelLevel(float64(level)/100, true); err != nil {
		return fmt.Errorf("%w: %v", errVibrationUnavailable, err)
	}
	return nil
}

func (s *Server) dispatchChatVibration(ctx context.Context, spec motion.VibrationSpec, stop *uint64) (chatMotionDispatch, error) {
	result := chatMotionDispatch{Action: "vibration"}
	if err := s.validateChatVibration(&spec); err != nil {
		return result, err
	}
	engine := s.currentMotionEngine()
	if engine != nil {
		current := engine.Snapshot()
		if current.Paused {
			return result, errors.New("playback is paused; resume it in the app first")
		}
		if current.Running {
			admission := engine.AdmissionGeneration()
			if stop != nil && s.stopSequence.Load() != *stop {
				return result, errors.New("vibration change canceled by Stop")
			}
			target := current.Target
			target.Vibration = &spec
			state, err := s.applyChatTargetHandoff(target, func() (motion.ActiveMotionState, error) {
				if candidate, ok := ctx.Value(chatRepairContextKey{}).(*chatRepairCandidate); ok && candidate.active.Load() {
					return engine.ApplyVibrationIfCurrent(ctx, spec, admission, candidate.state.Generation)
				}
				return engine.ApplyVibration(ctx, spec, admission)
			})
			result.Applied, result.Engine = err == nil, state
			return result, err
		}
	}
	// Off and follow do not implicitly start an idle session.
	if spec.Mode == "off" || spec.Mode == "follow" {
		result.Applied = true
		return result, nil
	}
	engine, admission, err := s.motionEngineForStart()
	if err != nil {
		return result, err
	}
	if stop != nil && s.stopSequence.Load() != *stop {
		return result, errors.New("vibration start canceled by Stop")
	}
	settings, _ := s.store.Snapshot()
	flow := chat.DefaultLayeredScore(settings.Motion.SpeedMinPercent)
	target, err := motion.FlowTarget(flow, settings.EffectiveMotion())
	if err != nil {
		return result, err
	}
	target.Source, target.Label = "chat", "Vibration"
	target.VibrationOnly, target.Vibration = true, &spec
	state, err := s.applyChatTargetHandoff(target, func() (motion.ActiveMotionState, error) {
		return engine.StartAtGeneration(ctx, target, settings.EffectiveMotion(), admission)
	})
	result.Applied, result.Engine = err == nil, state
	return result, err
}
