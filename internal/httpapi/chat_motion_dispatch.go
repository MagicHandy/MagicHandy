package httpapi

import (
	"context"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func (s *Server) dispatchChatMotionStart(ctx context.Context, command *chat.MotionCommand, stopSequence *uint64) (chatMotionDispatch, error) {
	engine, admission, err := s.motionEngineForStart()
	if err != nil {
		return chatMotionDispatch{Action: command.Action}, err
	}
	if stopSequence != nil && s.stopSequence.Load() != *stopSequence {
		return chatMotionDispatch{Action: command.Action}, errors.New("chat motion canceled by Emergency Stop")
	}
	current := engine.Snapshot()
	target, err := s.chatMotionTarget(command, current)
	if err != nil {
		return chatMotionDispatch{Action: command.Action}, err
	}
	if current.Running {
		state, applyErr := s.applyChatTargetHandoff(target, func() (motion.ActiveMotionState, error) {
			return s.applyChatTargetAtRepairBase(ctx, engine, target, "chat_start_retarget")
		})
		return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, applyErr
	}
	settings, _ := s.store.Snapshot()
	state, startErr := s.applyChatTargetHandoff(target, func() (motion.ActiveMotionState, error) {
		return engine.StartAtGeneration(ctx, target, settings.EffectiveMotion(), admission)
	})
	return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, startErr
}

func (s *Server) dispatchChatMotionUpdate(ctx context.Context, command *chat.MotionCommand) (chatMotionDispatch, error) {
	engine := s.currentMotionEngine()
	if engine == nil {
		return chatMotionDispatch{Action: command.Action}, errors.New("motion is not running; use start to begin")
	}
	current := engine.Snapshot()
	if !current.Running {
		return chatMotionDispatch{Action: command.Action, Engine: current}, errors.New("motion is not running; use start to begin")
	}
	target, err := s.chatMotionTarget(command, current)
	if err != nil {
		return chatMotionDispatch{Action: command.Action}, err
	}
	state, applyErr := s.applyChatTargetHandoff(target, func() (motion.ActiveMotionState, error) {
		return s.applyChatTargetAtRepairBase(ctx, engine, target, "chat_target")
	})
	return chatMotionDispatch{Applied: true, Action: command.Action, Engine: state}, applyErr
}
