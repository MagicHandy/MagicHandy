package httpapi

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	chatChaosDispatchMinInterval = 450 * time.Millisecond
)

type chatChaosRuntime struct {
	mu               sync.Mutex
	player           *manualqueue.Player
	generation       uint64
	lastDispatchTime time.Time
}

func (s *Server) shouldUseChaoticChatMotion(command *chat.MotionCommand, settings config.Settings) bool {
	if settings.Motion.MotionGenerationMode != config.MotionGenerationModeProcedural {
		return false
	}
	if command == nil {
		return false
	}
	switch command.Action {
	case chat.MotionActionStart, chat.MotionActionTarget, chat.MotionActionStop:
		return true
	default:
		return false
	}
}

func (s *Server) dispatchChatChaoticMotionAsync(
	parentCtx context.Context,
	command *chat.MotionCommand,
	settings config.Settings,
) chatMotionDispatch {
	if command == nil {
		return chatMotionDispatch{Action: chat.MotionActionNone}
	}
	if command.Action == chat.MotionActionStop {
		return s.dispatchChatChaoticStop(parentCtx, command)
	}
	if !chat.HasChaoticPhysicsIntent(command) {
		return chatMotionDispatch{Action: command.Action}
	}

	now := time.Now()
	s.chatChaos.mu.Lock()
	if !s.chatChaos.lastDispatchTime.IsZero() &&
		now.Sub(s.chatChaos.lastDispatchTime) < chatChaosDispatchMinInterval {
		s.chatChaos.mu.Unlock()
		return chatMotionDispatch{Action: command.Action}
	}
	s.chatChaos.lastDispatchTime = now
	s.chatChaos.generation++
	generation := s.chatChaos.generation
	s.chatChaos.mu.Unlock()

	physics := cloneMotionCommand(command)
	aiRaw := map[string]any{
		"action": command.Action, "velocidade": command.Velocidade,
		"intensidade": command.Intensidade, "regiao": command.Regiao,
		"tipo_batida": command.TipoBatida, "atraso_ms": command.AtrasoMS,
	}
	chat.NormalizeChaoticPhysics(physics)
	// #region agent log
	agentDebugLog("M0", "chat_chaos.go:dispatch", "ai physics normalized", map[string]any{
		"ai_raw": aiRaw,
		"normalized": map[string]any{
			"action": physics.Action, "velocidade": physics.Velocidade,
			"intensidade": physics.Intensidade, "regiao": physics.Regiao,
			"tipo_batida": physics.TipoBatida, "atraso_ms": physics.AtrasoMS,
		},
	})
	// #endregion

	go func() {
		ctx := context.WithoutCancel(parentCtx)
		if err := s.playChatChaoticMotion(ctx, physics, settings, generation); err != nil {
			s.logger.Warn("chat chaotic motion failed",
				"action", physics.Action,
				"generation", generation,
				"error", err,
			)
		}
	}()

	return chatMotionDispatch{
		Applied: true,
		Action:  command.Action,
	}
}

func (s *Server) dispatchChatChaoticStop(ctx context.Context, command *chat.MotionCommand) chatMotionDispatch {
	if s.modes != nil {
		s.modes.NotifyChatStop()
		s.modes.NotifyUserStop()
	}
	s.cancelChatChaosMotion(ctx)
	s.stopAndClearMotionEngine(ctx, "chat_stop")
	return chatMotionDispatch{
		Applied: true,
		Action:  command.Action,
	}
}

func (s *Server) playChatChaoticMotion(
	ctx context.Context,
	command *chat.MotionCommand,
	settings config.Settings,
	generation uint64,
) error {
	if !s.chatChaosGenerationCurrent(generation) {
		return nil
	}

	commandTransport, err := s.newSelectedMotionTransport()
	if err != nil {
		return err
	}

	// #nosec G404 -- chaotic motion requires non-deterministic micro-variance.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	continueFrom := -1
	s.chatChaos.mu.Lock()
	if existing := s.chatChaos.player; existing != nil {
		if snap := existing.Snapshot(); snap.Running {
			continueFrom = int(math.Round(snap.PositionPct))
		}
	}
	s.chatChaos.mu.Unlock()

	session := buildChaosSessionFromPosition(
		motion.ChaoticPhysics{
			Velocidade:  command.Velocidade,
			Intensidade: command.Intensidade,
			Regiao:      command.Regiao,
			TipoBatida:  command.TipoBatida,
			AtrasoMS:    command.AtrasoMS,
		},
		settings.Motion,
		settings.Motion.HardwareSafetyLock,
		rng,
		continueFrom,
	)
	if len(session.Points) == 0 {
		return nil
	}
	// #region agent log
	if len(session.Points) > 0 {
		last := session.Points[len(session.Points)-1]
		agentDebugLog("M2", "chat_chaos.go:playChatChaoticMotion", "session built", map[string]any{
			"continue_from": continueFrom,
			"point_count":   len(session.Points),
			"duration_ms": session.DurationMS,
			"first_x":       session.Points[0].PositionPercent,
			"first_t":       session.Points[0].TimeMillis,
			"last_x":        last.PositionPercent,
			"last_t":        last.TimeMillis,
			"stroke_min":    session.StrokeMin,
			"stroke_max":    session.StrokeMax,
		})
	}
	// #endregion

	s.stopAndClearMotionEngine(ctx, "chat_chaos_prepare")
	s.stopManualQueuePlayer(ctx)
	s.stopChatChaosPlayer(ctx)

	if !s.chatChaosGenerationCurrent(generation) {
		return nil
	}

	player := manualqueue.NewPlayer(commandTransport)
	player.SetOnFinished(func() {
		s.chatChaos.mu.Lock()
		if s.chatChaos.player == player {
			s.chatChaos.player = nil
		}
		s.chatChaos.mu.Unlock()
	})

	if err := player.Start(ctx, session, 0); err != nil {
		return err
	}

	s.chatChaos.mu.Lock()
	if generation != s.chatChaos.generation {
		playerToStop := player
		s.chatChaos.mu.Unlock()
		playerToStop.Stop(ctx)
		return nil
	}
	s.chatChaos.player = player
	s.chatChaos.mu.Unlock()

	if s.modes != nil {
		s.modes.NotifyChatStop()
	}
	return nil
}

func (s *Server) stopChatChaosPlayer(ctx context.Context) {
	s.chatChaos.mu.Lock()
	player := s.chatChaos.player
	s.chatChaos.player = nil
	s.chatChaos.mu.Unlock()

	if player != nil {
		player.Stop(ctx)
	}
}

func (s *Server) cancelChatChaosMotion(ctx context.Context) {
	s.chatChaos.mu.Lock()
	s.chatChaos.generation++
	player := s.chatChaos.player
	s.chatChaos.player = nil
	s.chatChaos.mu.Unlock()

	if player != nil {
		player.Stop(ctx)
	}
}

func (s *Server) chatChaosGenerationCurrent(generation uint64) bool {
	s.chatChaos.mu.Lock()
	defer s.chatChaos.mu.Unlock()
	return generation == s.chatChaos.generation
}

func cloneMotionCommand(command *chat.MotionCommand) *chat.MotionCommand {
	if command == nil {
		return nil
	}
	cloned := *command
	return &cloned
}
