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
	lastPhysics      motion.ChaoticPhysics
	hasLastPhysics   bool
	lastBridgeAt     time.Time
	dispatchInFlight bool
	bridgeCancel     context.CancelFunc
}

func (s *Server) shouldUseChaoticChatMotion(command *chat.MotionCommand, settings config.Settings) bool {
	if !config.UsesProceduralMotionGeneration(settings.Motion.MotionGenerationMode) {
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
	s.chatChaos.mu.Lock()
	s.chatChaos.dispatchInFlight = true
	s.chatChaos.mu.Unlock()
	defer func() {
		s.chatChaos.mu.Lock()
		s.chatChaos.dispatchInFlight = false
		s.chatChaos.mu.Unlock()
	}()

	if !s.chatChaosGenerationCurrent(generation) {
		return nil
	}

	commandTransport, err := s.newMotionCommandTransport()
	if err != nil {
		return err
	}

	// #nosec G404 -- chaotic motion requires non-deterministic micro-variance.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var appendToExisting bool
	var blendState motion.MotionBlendState
	var existingPlayer *manualqueue.Player
	var regionChanged bool

	s.chatChaos.mu.Lock()
	if existing := s.chatChaos.player; existing != nil {
		if snap := existing.Snapshot(); snap.Running {
			appendToExisting = true
			existingPlayer = existing
			blendState = estimateBlendStateFromPlayer(existing)
		}
	}
	if s.chatChaos.hasLastPhysics {
		regionChanged = s.chatChaos.lastPhysics.Regiao != command.Regiao
	}
	s.chatChaos.mu.Unlock()

	physics := chat.ChaoticPhysicsFromCommand(command)
	if command.Action == chat.MotionActionStart && appendToExisting {
		command.Action = chat.MotionActionTarget
	}
	durationMS := int64(motion.EstimateChatMotionDurationMS(physics))

	var session manualqueue.Session
	var forcePlayerRestart bool
	if appendToExisting && !regionChanged {
		if motion.IsTurboTipo(physics.TipoBatida) {
			continueFrom := int(math.Round(blendState.Position))
			session = buildChaosSessionForDurationFromPosition(
				physics,
				settings.Motion,
				settings.Motion.HardwareSafetyLock,
				rng,
				durationMS,
				continueFrom,
			)
		} else {
			session = buildChaosSessionForDurationFromBlend(
				physics,
				settings.Motion,
				settings.Motion.HardwareSafetyLock,
				rng,
				durationMS,
				blendState,
			)
		}
	} else if appendToExisting && regionChanged {
		// Splicing leaves pre-cut meio points in the HSP buffer; restart with a
		// fresh timeline so the device actually moves in the new regiao envelope.
		continueFrom := int(math.Round(blendState.Position))
		if motion.IsTurboTipo(physics.TipoBatida) {
			continueFrom = -1
		}
		session = buildChaosSessionForDurationFromPosition(
			physics,
			settings.Motion,
			settings.Motion.HardwareSafetyLock,
			rng,
			durationMS,
			continueFrom,
		)
		forcePlayerRestart = true
	} else {
		session = buildChaosSessionForDurationFromPosition(
			physics,
			settings.Motion,
			settings.Motion.HardwareSafetyLock,
			rng,
			durationMS,
			-1,
		)
	}
	if len(session.Points) == 0 {
		return nil
	}
	s.rememberChatChaosPhysics(physics)
	s.resetChatChaosBridgeDebounce()
	// #region agent log
	if len(session.Points) > 0 {
		last := session.Points[len(session.Points)-1]
		agentDebugLog("M2", "chat_chaos.go:playChatChaoticMotion", "session built", map[string]any{
			"append":            appendToExisting,
			"force_restart":     forcePlayerRestart,
			"region_changed":    regionChanged,
			"blend_pos":         blendState.Position,
			"blend_vel":     blendState.Velocity,
			"point_count":   len(session.Points),
			"duration_ms":   session.DurationMS,
			"target_ms":     durationMS,
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

	if appendToExisting && existingPlayer != nil && !forcePlayerRestart {
		if !s.chatChaosGenerationCurrent(generation) {
			return nil
		}
		appended := false
		if err := existingPlayer.SpliceExtensionAtPlayhead(session, int(chaosChainLeadMillis)); err == nil {
			appended = true
		} else {
			s.logger.Warn("chat chaos splice failed, falling back to append", "error", err)
			if err := existingPlayer.AppendExtension(session); err == nil {
				appended = true
			} else {
				s.logger.Warn("chat chaos append failed, restarting with crossfade", "error", err)
				appendToExisting = false
			}
		}
		if appended {
			if s.modes != nil {
				s.modes.NotifyChatStop()
			}
			s.startChatChaosBridgeWatcher(ctx, generation, settings)
			return nil
		}
	}

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

	s.startChatChaosBridgeWatcher(ctx, generation, settings)

	if s.modes != nil {
		s.modes.NotifyChatStop()
	}
	return nil
}

func (s *Server) stopChatChaosPlayer(ctx context.Context) {
	s.stopChatChaosBridgeWatcher()
	s.chatChaos.mu.Lock()
	player := s.chatChaos.player
	s.chatChaos.player = nil
	s.chatChaos.mu.Unlock()

	if player != nil {
		player.Stop(ctx)
	}
}

func (s *Server) cancelChatChaosMotion(ctx context.Context) {
	s.stopChatChaosBridgeWatcher()
	s.chatChaos.mu.Lock()
	s.chatChaos.generation++
	player := s.chatChaos.player
	s.chatChaos.player = nil
	s.chatChaos.hasLastPhysics = false
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
