package httpapi

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	chatChaosLoopBridgeLeadMS   = 15_000
	chatChaosLoopBridgeSeconds  = 8
	chatChaosLoopBridgeDebounce = 400 * time.Millisecond
	chatChaosBridgePollInterval = 100 * time.Millisecond
	chatChaosBridgeEmergencyMS  = 800
)

func (s *Server) chatChaosTimelineRemainingMS() int {
	s.chatChaos.mu.Lock()
	player := s.chatChaos.player
	s.chatChaos.mu.Unlock()
	if player == nil || !player.Running() {
		return 0
	}
	snap := player.Snapshot()
	remaining := player.TimelineEndMS() - snap.PlayheadMS
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *Server) startChatChaosBridgeWatcher(parentCtx context.Context, generation uint64, settings config.Settings) {
	s.stopChatChaosBridgeWatcher()

	ctx, cancel := context.WithCancel(context.WithoutCancel(parentCtx))
	s.chatChaos.mu.Lock()
	s.chatChaos.bridgeCancel = cancel
	s.chatChaos.mu.Unlock()

	go func() {
		ticker := time.NewTicker(chatChaosBridgePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !s.chatChaosGenerationCurrent(generation) {
					return
				}
				s.chatChaosMaybeLoopBridge(ctx, generation, settings)
			}
		}
	}()
}

func (s *Server) stopChatChaosBridgeWatcher() {
	s.chatChaos.mu.Lock()
	cancel := s.chatChaos.bridgeCancel
	s.chatChaos.bridgeCancel = nil
	s.chatChaos.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Server) chatChaosMaybeLoopBridge(ctx context.Context, generation uint64, settings config.Settings) {
	_ = ctx
	_ = generation

	remainingMS := s.chatChaosTimelineRemainingMS()
	if remainingMS > chatChaosLoopBridgeLeadMS {
		return
	}

	s.chatChaos.mu.Lock()
	inFlight := s.chatChaos.dispatchInFlight
	lastBridge := s.chatChaos.lastBridgeAt
	hasPhysics := s.chatChaos.hasLastPhysics
	s.chatChaos.mu.Unlock()

	if !hasPhysics {
		return
	}
	emergency := remainingMS <= chatChaosBridgeEmergencyMS
	if inFlight && !emergency {
		return
	}
	if !emergency && !lastBridge.IsZero() && time.Since(lastBridge) < chatChaosLoopBridgeDebounce {
		return
	}

	session, err := s.buildChatChaosLoopBridge(settings)
	if err != nil {
		agentDebugLog("CB1", "chat_chaos_bridge.go:chatChaosMaybeLoopBridge", "bridge_build_failed", map[string]any{
			"err": err.Error(),
		})
		return
	}

	s.chatChaos.mu.Lock()
	player := s.chatChaos.player
	s.chatChaos.mu.Unlock()
	if player == nil || !player.Running() {
		return
	}
	if err := player.AppendExtension(session); err != nil {
		agentDebugLog("CB1", "chat_chaos_bridge.go:chatChaosMaybeLoopBridge", "bridge_append_failed", map[string]any{
			"err": err.Error(),
		})
		return
	}

	s.chatChaos.mu.Lock()
	s.chatChaos.lastBridgeAt = time.Now()
	s.chatChaos.mu.Unlock()

	motion.MotionDebugLog("CB2", "chat_chaos_bridge.go:chatChaosMaybeLoopBridge", "chat_chaos_bridge_filler", map[string]any{
		"remaining_ms": remainingMS,
		"points":       len(session.Points),
		"duration_ms":  session.DurationMS,
		"block_sec":    chatChaosLoopBridgeSeconds,
	})
}

func (s *Server) buildChatChaosLoopBridge(settings config.Settings) (manualqueue.Session, error) {
	s.chatChaos.mu.Lock()
	physics := s.chatChaos.lastPhysics
	hasPhysics := s.chatChaos.hasLastPhysics
	player := s.chatChaos.player
	s.chatChaos.mu.Unlock()
	if !hasPhysics {
		return manualqueue.Session{}, errors.New("no prior chaotic physics for bridge filler")
	}

	filler := physics
	if !motion.IsTurboTipo(filler.TipoBatida) {
		filler.TipoBatida = "fluido"
		filler.Velocidade = maxInt(20, physics.Velocidade-15)
		if filler.AtrasoMS == 0 {
			filler.AtrasoMS = 160
		}
	} else if filler.AtrasoMS <= 0 {
		filler.AtrasoMS = 1
	}

	// #nosec G404 -- bridge filler needs non-deterministic micro-variance.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	durationMS := int64(chatChaosLoopBridgeSeconds * 1000)

	var blendState motion.MotionBlendState
	if player != nil {
		blendState = estimateBlendStateFromPlayer(player)
	}
	if motion.IsTurboTipo(filler.TipoBatida) {
		continueFrom := int(math.Round(blendState.Position))
		return buildChaosSessionForDurationFromPosition(
			filler,
			settings.Motion,
			settings.Motion.HardwareSafetyLock,
			rng,
			durationMS,
			continueFrom,
		), nil
	}
	return buildChaosSessionForDurationFromBlend(
		filler,
		settings.Motion,
		settings.Motion.HardwareSafetyLock,
		rng,
		durationMS,
		blendState,
	), nil
}

func (s *Server) rememberChatChaosPhysics(physics motion.ChaoticPhysics) {
	s.chatChaos.mu.Lock()
	s.chatChaos.lastPhysics = physics
	s.chatChaos.hasLastPhysics = true
	s.chatChaos.mu.Unlock()
}

func (s *Server) resetChatChaosBridgeDebounce() {
	s.chatChaos.mu.Lock()
	s.chatChaos.lastBridgeAt = time.Time{}
	s.chatChaos.mu.Unlock()
}
