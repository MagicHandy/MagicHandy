package httpapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func (s *Server) prepareManualQueueSession(ctx context.Context) (manualqueue.Session, error) {
	if s.library == nil {
		return manualqueue.Session{}, errLibraryUnavailable
	}
	s.manualQueue.mu.Lock()
	items := append([]manualQueueItem(nil), s.manualQueue.items...)
	autoloop := s.manualQueue.autoloop
	s.manualQueue.mu.Unlock()
	if len(items) == 0 {
		return manualqueue.Session{}, errors.New("manual queue is empty")
	}

	blocks := make(map[string][]manualqueue.Action)
	queueItems := make([]manualqueue.Item, 0, len(items))
	for _, item := range items {
		block, err := s.library.Store().GetMotionBlock(ctx, item.BlockID)
		if err != nil {
			return manualqueue.Session{}, fmt.Errorf("block %s: %w", item.BlockID, err)
		}
		if _, ok := blocks[item.BlockID]; !ok {
			actions := make([]manualqueue.Action, len(block.Actions))
			for i, action := range block.Actions {
				actions[i] = manualqueue.Action{At: action.At, Pos: int(action.Pos + 0.5)}
			}
			blocks[item.BlockID] = actions
		}
		queueItems = append(queueItems, manualqueue.Item{
			BlockID:     item.BlockID,
			DurationSec: item.DurationSec,
			Loop:        item.Loop,
		})
	}

	actions, durationMS := manualqueue.ConcatManualQueueItems(queueItems, blocks)
	if len(actions) == 0 {
		return manualqueue.Session{}, errors.New("manual queue script has no actions")
	}

	settings, _ := s.store.Snapshot()
	opts := manualqueue.TimelineOptions{
		StrokeMinPercent: settings.Motion.StrokeMinPercent,
		StrokeMaxPercent: settings.Motion.StrokeMaxPercent,
		RemapStroke:      true,
	}
	return manualqueue.Session{
		Actions:       actions,
		Points:        manualqueue.ActionsToTimedPoints(actions, opts),
		DurationMS:    durationMS,
		SegmentStarts: manualqueue.SegmentStarts(queueItems),
		Autoloop:      autoloop,
		StrokeMin:     settings.Motion.StrokeMinPercent,
		StrokeMax:     settings.Motion.StrokeMaxPercent,
	}, nil
}

func (s *Server) stopManualQueuePlayer(ctx context.Context) {
	s.manualQueue.mu.Lock()
	player := s.manualQueue.player
	s.manualQueue.player = nil
	s.manualQueue.mu.Unlock()
	if player != nil {
		player.Stop(ctx)
	}
}

func (s *Server) manualQueuePlayerSnapshot() (manualqueue.Snapshot, []manualqueue.Action) {
	s.manualQueue.mu.Lock()
	player := s.manualQueue.player
	s.manualQueue.mu.Unlock()
	if player == nil {
		return manualqueue.Snapshot{}, nil
	}
	return player.Snapshot(), player.Actions()
}

func (s *Server) startManualQueuePlayback(ctx context.Context) (manualqueue.Session, error) {
	session, err := s.prepareManualQueueSession(ctx)
	if err != nil {
		return manualqueue.Session{}, err
	}

	commandTransport, err := s.newMotionCommandTransport()
	if err != nil {
		return manualqueue.Session{}, err
	}

	s.stopAndClearMotionEngine(ctx, "manual_queue_play")
	s.stopDirectControl()

	player := manualqueue.NewPlayer(commandTransport)
	player.SetOnFinished(func() {
		s.manualQueue.mu.Lock()
		s.manualQueue.playing = false
		s.manualQueue.paused = false
		s.manualQueue.playStarted = time.Time{}
		s.manualQueue.player = nil
		s.manualQueue.mu.Unlock()
	})

	if err := player.Start(ctx, session, 0); err != nil {
		return manualqueue.Session{}, err
	}

	s.manualQueue.mu.Lock()
	s.manualQueue.player = player
	s.manualQueue.playing = true
	s.manualQueue.paused = false
	s.manualQueue.playStarted = time.Now()
	s.manualQueue.mu.Unlock()

	return session, nil
}

func (s *Server) stopDirectControl() {
	s.direct.mu.Lock()
	wasActive := s.direct.active
	s.direct.active = false
	s.direct.mu.Unlock()
	if !wasActive {
		return
	}
	if commandTransport, err := s.newSelectedMotionTransport(); err == nil {
		if direct, ok := commandTransport.(transport.DirectMotionCapable); ok {
			_ = direct.StopDirectMotion(context.Background())
		}
	}
}

func (s *Server) buildManualQueuePreviewFromSession(session manualqueue.Session) ([]map[string]any, []map[string]any, int, int) {
	preview := make([]map[string]any, 0, len(session.Actions))
	for _, action := range session.Actions {
		preview = append(preview, map[string]any{
			"t_ms": action.At,
			"pos":  action.Pos,
		})
	}
	segments := make([]map[string]any, 0, len(session.SegmentStarts))
	for index, startMS := range session.SegmentStarts {
		endMS := session.DurationMS
		if index+1 < len(session.SegmentStarts) {
			endMS = session.SegmentStarts[index+1]
		}
		segments = append(segments, map[string]any{
			"index":       index,
			"start_ms":    startMS,
			"duration_ms": endMS - startMS,
		})
	}
	return preview, segments, session.DurationMS, len(session.Actions)
}
