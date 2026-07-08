package httpapi

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

type freestyleChaosRuntime struct {
	mu         sync.Mutex
	player     *manualqueue.Player
	generation uint64
}

func (s *Server) usesProceduralFreestyle(settings config.MotionSettings) bool {
	return settings.MotionGenerationMode == config.MotionGenerationModeProcedural
}

func (s *Server) freestyleChaosActive() bool {
	s.freestyleChaos.mu.Lock()
	player := s.freestyleChaos.player
	s.freestyleChaos.mu.Unlock()
	if player == nil {
		return false
	}
	return player.Snapshot().Running
}

func (s *Server) startFreestyleChaosSegment(
	ctx context.Context,
	segment modes.ProceduralFreestyleSegment,
) error {
	settings, _ := s.store.Snapshot()
	if !s.usesProceduralFreestyle(settings.Motion) {
		return nil
	}

	commandTransport, err := s.newSelectedMotionTransport()
	if err != nil {
		return err
	}

	s.freestyleChaos.mu.Lock()
	continueFrom := -1
	if existing := s.freestyleChaos.player; existing != nil {
		if snap := existing.Snapshot(); snap.Running {
			continueFrom = int(math.Round(snap.PositionPct))
		}
	}
	s.freestyleChaos.generation++
	generation := s.freestyleChaos.generation
	s.freestyleChaos.mu.Unlock()

	// #nosec G404 -- freestyle procedural segments need non-deterministic micro-variance.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	session := buildChaosSessionForDurationFromPosition(
		segment.Physics,
		settings.Motion,
		false,
		rng,
		segment.DurationMillis,
		continueFrom,
	)
	if len(session.Points) == 0 {
		return nil
	}

	s.stopAndClearMotionEngine(ctx, "freestyle_chaos_prepare")
	s.stopManualQueuePlayer(ctx)
	s.cancelChatChaosMotion(ctx)
	s.stopFreestyleChaosPlayer(ctx)

	if !s.freestyleChaosGenerationCurrent(generation) {
		return nil
	}

	player := manualqueue.NewPlayer(commandTransport)
	player.SetOnFinished(func() {
		s.freestyleChaos.mu.Lock()
		if s.freestyleChaos.player == player {
			s.freestyleChaos.player = nil
		}
		s.freestyleChaos.mu.Unlock()
	})

	if err := player.Start(ctx, session, 0); err != nil {
		return err
	}

	s.freestyleChaos.mu.Lock()
	if generation != s.freestyleChaos.generation {
		playerToStop := player
		s.freestyleChaos.mu.Unlock()
		playerToStop.Stop(ctx)
		return nil
	}
	s.freestyleChaos.player = player
	s.freestyleChaos.mu.Unlock()
	return nil
}

func (s *Server) stopFreestyleChaosPlayer(ctx context.Context) {
	s.freestyleChaos.mu.Lock()
	player := s.freestyleChaos.player
	s.freestyleChaos.player = nil
	s.freestyleChaos.mu.Unlock()
	if player != nil {
		player.Stop(ctx)
	}
}

func (s *Server) cancelFreestyleChaosMotion(ctx context.Context) {
	s.freestyleChaos.mu.Lock()
	s.freestyleChaos.generation++
	player := s.freestyleChaos.player
	s.freestyleChaos.player = nil
	s.freestyleChaos.mu.Unlock()
	if player != nil {
		player.Stop(ctx)
	}
}

func (s *Server) freestyleChaosGenerationCurrent(generation uint64) bool {
	s.freestyleChaos.mu.Lock()
	defer s.freestyleChaos.mu.Unlock()
	return generation == s.freestyleChaos.generation
}
