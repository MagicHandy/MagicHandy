package httpapi

import (
	"context"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type manualVibrationResumeKey struct{}

type manualVibrationResume struct {
	owner    *transport.IntifaceVibrator
	permit   transport.VibratorResumePermit
	sequence uint64
}

// Only the fresh manual chat endpoint captures this permission to lift Stop's
// suspension. Autopilot, reviewers and diagnostic previews never receive it.
// No output changes here.
func (s *Server) withManualVibrationResume(ctx context.Context, settings config.Settings, sequence uint64) context.Context {
	if !chatCapabilities(settings.LLM, nil).Motion || !settings.Device.Plays(config.DeviceVibrator) {
		return ctx
	}
	owner, err := s.currentVibrator()
	if err != nil {
		return ctx
	}
	permit, available := owner.ManualResumePermit()
	if !available {
		return ctx
	}
	return context.WithValue(ctx, manualVibrationResumeKey{}, &manualVibrationResume{owner, permit, sequence})
}

func (s *Server) manualVibrationResumeCurrent(ctx context.Context, resume *manualVibrationResume) bool {
	if resume == nil || s.chatCanceled(ctx, resume.sequence) {
		return false
	}
	owner, err := s.currentVibrator()
	return err == nil && owner == resume.owner && s.vibrationReadiness() == nil && owner.ResumePermitCurrent(resume.permit)
}

func (s *Server) addManualVibrationResumeContext(ctx context.Context, state *chat.MotionContext) {
	resume, _ := ctx.Value(manualVibrationResumeKey{}).(*manualVibrationResume)
	if !s.manualVibrationResumeCurrent(ctx, resume) {
		return
	}
	state.ManualRestartAvailable = true
	if state.Outputs != nil && state.Outputs.Vibrator != nil {
		state.Outputs.Vibrator.ResumeOnMotionRequest = true
	}
}

func resumesVibration(command *chat.MotionCommand) bool {
	if command == nil || command.Action == chat.MotionActionStop {
		return false
	}
	if command.Vibration != nil && command.Vibration.Mode == "off" {
		return false
	}
	linear := command.Action != "" && command.Action != chat.MotionActionNone
	return linear || (command.Vibration != nil && command.Vibration.MaxPercent > 0) ||
		(command.VibrationLevel != nil && *command.VibrationLevel > 0)
}

func (s *Server) dispatchManualChatMotion(ctx context.Context, command *chat.MotionCommand, sequence *uint64) (chatMotionDispatch, error) {
	resume, _ := ctx.Value(manualVibrationResumeKey{}).(*manualVibrationResume)
	if resume == nil || !resumesVibration(command) {
		return s.dispatchChatMotionLocked(ctx, command, sequence)
	}
	if !s.manualVibrationResumeCurrent(ctx, resume) {
		return chatMotionDispatch{}, errors.New("the vibrator controls changed; send your motion request again")
	}
	if command.Action != "" && command.Action != chat.MotionActionNone {
		if err := s.validateChatMotionMode(command); err != nil {
			return chatMotionDispatch{}, err
		}
	}
	rollback, err := resume.owner.ResumeForManualRequest(resume.permit)
	if err != nil {
		return chatMotionDispatch{}, err
	}
	result, err := s.dispatchChatMotionLocked(ctx, command, sequence)
	// Keep ordinary command acceptance. A level/follow echo can be a valid
	// settings-only reply, but does not start playback. Restore suspension
	// after such a reply without turning it into an error or requiring new
	// wording from the person. Actual starts retain the resume permission.
	engine := s.currentMotionEngine()
	if err != nil || !result.Applied || engine == nil || !engine.Snapshot().Running {
		rollback()
	}
	return result, err
}
