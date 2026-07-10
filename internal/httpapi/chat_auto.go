package httpapi

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chatauto"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
)

const (
	chatAutoPrefetchLead      = 15 * time.Second
	chatAutoQueueTargetDepth  = 2
	chatAutoLoopBridgeSeconds = 15
	chatAutoTickInterval      = 250 * time.Millisecond
	chatAutoMinBoundaryWait   = 1 * time.Second
	chatAutoMaxTokens         = 160
	chatAutoTemperature       = 0.5
)

type chatAutoPreparedSegment struct {
	response chatauto.Response
	session  manualqueue.Session
	stamina  float64
	intent   chatauto.Intent
	mapped   chatauto.MappedSegment
	err      error
	llmMs    int64
}

type chatAutoQueuedSegment struct {
	prepared chatAutoPreparedSegment
	fromUser bool
}

type chatAutoRuntime struct {
	mu               sync.Mutex
	state            chatauto.State
	segmentEndsAt    time.Time
	sessionStartedAt time.Time
	player           *manualqueue.Player
	generation       uint64
	cancel           context.CancelFunc
	done             chan struct{}

	playbackQueue    []chatAutoQueuedSegment
	pendingUserTexts []string
	prefetchActive   bool
	lastBridgeAt     time.Time
}

func (s *Server) chatAutoStateSnapshot() chatauto.State {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	state := s.chatAuto.state
	if !s.chatAuto.sessionStartedAt.IsZero() {
		settings, _ := s.store.Snapshot()
		elapsed := time.Since(s.chatAuto.sessionStartedAt)
		progress, humor := chatauto.MoodAtElapsed(
			elapsed,
			settings.AutoDom.DominatrixRampMinutes,
			settings.AutoDom.AllowDominatrix,
		)
		state.MoodProgress = progress
		state.Humor = chatauto.EffectiveHumor(state.Humor, humor, settings.AutoDom.AllowDominatrix)
	}
	if !s.chatAuto.segmentEndsAt.IsZero() {
		remaining := time.Until(s.chatAuto.segmentEndsAt).Milliseconds()
		if remaining > 0 {
			state.SegmentEndsInMS = remaining
		}
	}
	return state
}

func (s *Server) chatAutoPublicState() map[string]any {
	state := s.chatAutoStateSnapshot()
	return map[string]any{
		"active":             state.Active,
		"stamina":            state.Stamina,
		"humor":              state.Humor,
		"mood_progress":      state.MoodProgress,
		"posicao":            string(state.Posicao),
		"motion": map[string]any{
			"action":      state.Motion.Action,
			"velocidade":  state.Motion.Velocidade,
			"intensidade": state.Motion.Intensidade,
			"regiao":      state.Motion.Regiao,
			"tipo_batida": state.Motion.TipoBatida,
			"atraso_ms":   state.Motion.AtrasoMS,
		},
		"last_reply":         state.LastReply,
		"reply_partial":      state.ReplyPartial,
		"segment_ends_in_ms": state.SegmentEndsInMS,
		"llm_busy":           state.LLMBusy,
		"error":              state.Error,
		"queue_depth":        s.chatAutoQueueDepth(),
	}
}

func (s *Server) chatAutoQueueDepth() int {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	return len(s.chatAuto.playbackQueue)
}

func (s *Server) chatAutoActive() bool {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if s.chatAuto.player != nil && s.chatAuto.player.Snapshot().Running {
		return true
	}
	return s.chatAuto.state.Active && s.chatAuto.cancel != nil
}

func (s *Server) operationModeAuto() bool {
	state, err := s.store.DB().LoadAppState()
	if err != nil {
		return false
	}
	return state.OperationMode == "auto"
}

func (s *Server) startChatAutoLoop(parentCtx context.Context) {
	if !s.operationModeAuto() {
		return
	}
	s.chatAuto.mu.Lock()
	if s.chatAuto.cancel != nil {
		s.chatAuto.mu.Unlock()
		return
	}
	s.chatAuto.generation++
	generation := s.chatAuto.generation
	s.chatAuto.state = chatauto.NewInitialState()
	s.chatAuto.state.Active = true
	s.chatAuto.sessionStartedAt = time.Now()
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(parentCtx))
	s.chatAuto.cancel = cancel
	s.chatAuto.done = make(chan struct{})
	s.chatAuto.mu.Unlock()

	go s.runChatAutoLoop(loopCtx, generation)
}

func (s *Server) stopChatAutoLoop(ctx context.Context) {
	s.chatAuto.mu.Lock()
	cancel := s.chatAuto.cancel
	player := s.chatAuto.player
	done := s.chatAuto.done
	s.chatAuto.generation++
	s.chatAuto.state.Active = false
	s.chatAuto.sessionStartedAt = time.Time{}
	s.chatAuto.cancel = nil
	s.chatAuto.player = nil
	s.chatAuto.playbackQueue = nil
	s.chatAuto.pendingUserTexts = nil
	s.chatAuto.prefetchActive = false
	s.chatAuto.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if player != nil {
		player.Stop(ctx)
	}
	if done != nil {
		<-done
	}

	s.chatAuto.mu.Lock()
	s.chatAuto.done = nil
	s.chatAuto.mu.Unlock()
}

func (s *Server) runChatAutoLoop(ctx context.Context, generation uint64) {
	defer func() {
		s.chatAuto.mu.Lock()
		done := s.chatAuto.done
		s.chatAuto.state.Active = false
		s.chatAuto.mu.Unlock()
		if done != nil {
			close(done)
		}
	}()

	settings, _ := s.store.Snapshot()
	if err := s.runChatAutoFirstTurn(ctx, generation, settings); err != nil {
		return
	}

	go s.runChatAutoPrefetchLoop(ctx, generation, settings)

	ticker := time.NewTicker(chatAutoTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.chatAutoGenerationCurrent(generation) {
				return
			}
			for s.chatChaosActive() {
				time.Sleep(300 * time.Millisecond)
				if !s.chatAutoGenerationCurrent(generation) {
					return
				}
			}
			s.chatAutoDrainPlaybackQueue(ctx, generation, settings)
			s.chatAutoMaybeLoopBridge(ctx, generation, settings)
		}
	}
}

func (s *Server) enqueueChatAutoUserText(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	s.chatAuto.mu.Lock()
	s.chatAuto.pendingUserTexts = append(s.chatAuto.pendingUserTexts, trimmed)
	s.chatAuto.mu.Unlock()
}

func (s *Server) runChatAutoPrefetchLoop(ctx context.Context, generation uint64, settings config.Settings) {
	for {
		if ctx.Err() != nil || !s.chatAutoGenerationCurrent(generation) {
			return
		}
		if s.chatAutoShouldThrottlePrefetch() {
			time.Sleep(chatAutoTickInterval)
			continue
		}

		prefetchStart := time.Now()
		userText := s.chatAutoPopPendingUser()

		s.chatAuto.mu.Lock()
		s.chatAuto.prefetchActive = true
		s.chatAuto.mu.Unlock()

		prepared, err := s.prepareChatAutoSegment(ctx, settings, userText)

		s.chatAuto.mu.Lock()
		s.chatAuto.prefetchActive = false
		s.chatAuto.mu.Unlock()

		if ctx.Err() != nil || !s.chatAutoGenerationCurrent(generation) {
			return
		}

		if err != nil {
			s.setChatAutoError(err.Error())
			fallback, fbErr := s.fallbackChatAutoResponse()
			if fbErr != nil {
				time.Sleep(time.Second)
				continue
			}
			session, stamina, intent, mapped, buildErr := s.buildChatAutoSession(settings, fallback)
			if buildErr != nil {
				time.Sleep(time.Second)
				continue
			}
			fallback.AutoDom = intent
			prepared = chatAutoPreparedSegment{
				response: fallback,
				session:  session,
				stamina:  stamina,
				intent:   intent,
				mapped:   mapped,
			}
		}
		prepared.llmMs = time.Since(prefetchStart).Milliseconds()

		s.chatAutoEnqueue(chatAutoQueuedSegment{
			prepared: prepared,
			fromUser: userText != "",
		})

		// #region agent log
		agentDebugLog("CA1", "chat_auto.go:runChatAutoPrefetchLoop", "segment_enqueued", map[string]any{
			"fromUser": userText != "", "llmMs": prepared.llmMs,
			"duracao": prepared.response.AutoDom.DuracaoSegundos,
			"queueDepth": s.chatAutoQueueDepth(), "points": len(prepared.session.Points),
		})
		// #endregion

		s.chatAutoDrainPlaybackQueue(ctx, generation, settings)
	}
}

func (s *Server) chatAutoShouldThrottlePrefetch() bool {
	s.chatAuto.mu.Lock()
	prefetching := s.chatAuto.prefetchActive
	depth := len(s.chatAuto.playbackQueue)
	pendingUsers := len(s.chatAuto.pendingUserTexts)
	s.chatAuto.mu.Unlock()

	if prefetching {
		return true
	}
	if pendingUsers > 0 {
		return false
	}
	if depth >= chatAutoQueueTargetDepth {
		return true
	}
	untilPrefetch := time.Until(s.chatAutoSegmentEnd()) - chatAutoPrefetchLead
	return untilPrefetch > 0
}

func (s *Server) chatAutoPopPendingUser() string {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if len(s.chatAuto.pendingUserTexts) == 0 {
		return ""
	}
	text := s.chatAuto.pendingUserTexts[0]
	s.chatAuto.pendingUserTexts = s.chatAuto.pendingUserTexts[1:]
	return text
}

func (s *Server) chatAutoEnqueue(item chatAutoQueuedSegment) {
	s.chatAuto.mu.Lock()
	s.chatAuto.playbackQueue = append(s.chatAuto.playbackQueue, item)
	s.chatAuto.mu.Unlock()
}

func (s *Server) chatAutoDequeue() (chatAutoQueuedSegment, bool) {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if len(s.chatAuto.playbackQueue) == 0 {
		return chatAutoQueuedSegment{}, false
	}
	item := s.chatAuto.playbackQueue[0]
	s.chatAuto.playbackQueue = s.chatAuto.playbackQueue[1:]
	return item, true
}

func (s *Server) chatAutoPeekQueue() (chatAutoQueuedSegment, bool) {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if len(s.chatAuto.playbackQueue) == 0 {
		return chatAutoQueuedSegment{}, false
	}
	return s.chatAuto.playbackQueue[0], true
}

func (s *Server) chatAutoTimelineRemainingMS() int {
	s.chatAuto.mu.Lock()
	player := s.chatAuto.player
	s.chatAuto.mu.Unlock()
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

func (s *Server) chatAutoDrainPlaybackQueue(ctx context.Context, generation uint64, settings config.Settings) {
	front, ok := s.chatAutoPeekQueue()
	if !ok {
		return
	}

	remainingMS := s.chatAutoTimelineRemainingMS()
	leadMS := int(chatAutoPrefetchLead.Milliseconds())
	if remainingMS > leadMS && !front.fromUser {
		return
	}

	item, ok := s.chatAutoDequeue()
	if !ok {
		return
	}

	segStart := time.Now()
	segPath, segErr := s.appendOrRestartChatAutoMotion(ctx, generation, settings, item.prepared)
	if segErr != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:chatAutoDrainPlaybackQueue", "segment_failed", map[string]any{
			"segPath": segPath, "err": segErr.Error(), "fromUser": item.fromUser,
		})
		// #endregion
		s.setChatAutoError(segErr.Error())
		s.chatAutoEnqueue(item)
		return
	}

	s.applyChatAutoUI(item.prepared.response)
	s.publishChatAutoReply(item.prepared.response)

	s.chatAuto.mu.Lock()
	staminaAfter := s.chatAuto.state.Stamina
	s.chatAuto.mu.Unlock()
	// #region agent log
	agentDebugLog("CA7", "chat_auto.go:chatAutoDrainPlaybackQueue", "motion_continued", map[string]any{
		"segPath": segPath, "segSetupMs": time.Since(segStart).Milliseconds(),
		"llmMs": item.prepared.llmMs, "fromUser": item.fromUser,
		"playerAfter": s.chatAutoPlayerRunning(), "duracaoSec": item.prepared.response.AutoDom.DuracaoSegundos,
		"staminaAfter": staminaAfter, "queueDepth": s.chatAutoQueueDepth(),
		"remainingBeforeMs": remainingMS,
	})
	// #endregion
}

func (s *Server) chatAutoMaybeLoopBridge(ctx context.Context, generation uint64, settings config.Settings) {
	remainingMS := s.chatAutoTimelineRemainingMS()
	if remainingMS > 5000 {
		return
	}

	s.chatAuto.mu.Lock()
	queueEmpty := len(s.chatAuto.playbackQueue) == 0
	prefetching := s.chatAuto.prefetchActive
	lastBridge := s.chatAuto.lastBridgeAt
	s.chatAuto.mu.Unlock()

	if !queueEmpty {
		return
	}
	if prefetching && remainingMS > 3000 {
		return
	}
	if !lastBridge.IsZero() && time.Since(lastBridge) < 8*time.Second {
		return
	}
	if !s.chatAutoPlayerRunning() {
		return
	}

	prepared, err := s.buildChatAutoLoopBridge(settings)
	if err != nil {
		return
	}
	if err := s.appendChatAutoSegmentPrepared(ctx, generation, prepared); err != nil {
		return
	}

	s.chatAuto.mu.Lock()
	s.chatAuto.lastBridgeAt = time.Now()
	s.chatAuto.mu.Unlock()

	// #region agent log
	agentDebugLog("CA9", "chat_auto.go:chatAutoMaybeLoopBridge", "loop_bridge_appended", map[string]any{
		"remainingMs": remainingMS, "points": len(prepared.session.Points),
	})
	// #endregion
}

func (s *Server) prepareChatAutoSegment(
	ctx context.Context,
	settings config.Settings,
	userText string,
) (chatAutoPreparedSegment, error) {
	start := time.Now()
	response, err := s.fetchChatAutoTurn(ctx, settings, userText)
	if err != nil {
		return chatAutoPreparedSegment{err: err, llmMs: time.Since(start).Milliseconds()}, err
	}
	session, stamina, intent, mapped, err := s.buildChatAutoSession(settings, response)
	if err != nil {
		return chatAutoPreparedSegment{err: err, llmMs: time.Since(start).Milliseconds()}, err
	}
	response.AutoDom = intent
	return chatAutoPreparedSegment{
		response: response,
		session:  session,
		stamina:  stamina,
		intent:   intent,
		mapped:   mapped,
		llmMs:    time.Since(start).Milliseconds(),
	}, nil
}

func (s *Server) buildChatAutoLoopBridge(settings config.Settings) (chatAutoPreparedSegment, error) {
	state := s.chatAutoStateSnapshot()
	intent := chatauto.Intent{
		Humor:           state.Humor,
		Posicao:         state.Posicao,
		Intensidade:     4,
		DuracaoSegundos: chatAutoLoopBridgeSeconds,
	}
	if intent.Humor == "" {
		intent.Humor = chatauto.HumorDesejando
	}
	if intent.Posicao == "" {
		intent.Posicao = chatauto.PoseHandjob
	}
	if state.Motion.Intensidade > 0 {
		intent.Intensidade = max(2, min(8, state.Motion.Intensidade/10))
	}
	intent, _ = chatauto.ValidateIntent(intent)
	response := chatauto.Response{
		Reply:   "",
		AutoDom: intent,
	}
	session, stamina, intent, mapped, err := s.buildChatAutoSession(settings, response)
	if err != nil {
		return chatAutoPreparedSegment{}, err
	}
	response.AutoDom = intent
	return chatAutoPreparedSegment{
		response: response,
		session:  session,
		stamina:  stamina,
		intent:   intent,
		mapped:   mapped,
	}, nil
}

func (s *Server) publishChatAutoReply(response chatauto.Response) {
	reply := strings.TrimSpace(response.Reply)
	if reply == "" {
		return
	}
	s.appendChatMessage("assistant", reply)
}

func (s *Server) runChatAutoFirstTurn(ctx context.Context, generation uint64, settings config.Settings) error {
	loopStart := time.Now()
	// #region agent log
	agentDebugLog("CA2", "chat_auto.go:runChatAutoLoop", "turn_start", map[string]any{
		"generation": generation, "stamina": s.chatAutoStateSnapshot().Stamina,
		"posicao": s.chatAutoStateSnapshot().Posicao, "chatChaosActive": s.chatChaosActive(),
		"playerRunning": s.chatAutoPlayerRunning(),
	})
	// #endregion

	response, err := s.fetchChatAutoTurn(ctx, settings, "")
	llmMs := time.Since(loopStart).Milliseconds()
	if err != nil {
		// #region agent log
		agentDebugLog("CA1", "chat_auto.go:runChatAutoLoop", "fetch_failed_fallback", map[string]any{
			"err": err.Error(), "llmMs": llmMs,
		})
		// #endregion
		s.setChatAutoError(err.Error())
		response, err = s.fallbackChatAutoResponse()
		if err != nil {
			return err
		}
	}
	// #region agent log
	agentDebugLog("CA1", "chat_auto.go:runChatAutoLoop", "fetch_ok", map[string]any{
		"llmMs": llmMs, "reply": truncateForLog(response.Reply, 120),
		"humor": response.AutoDom.Humor, "posicao": response.AutoDom.Posicao,
		"intensidade": response.AutoDom.Intensidade, "duracao": response.AutoDom.DuracaoSegundos,
	})
	// #endregion

	segStart := time.Now()
	segErr := s.startChatAutoSegment(ctx, generation, settings, response)
	if segErr != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:runChatAutoLoop", "segment_failed", map[string]any{
			"segPath": "start", "err": segErr.Error(),
		})
		// #endregion
		return segErr
	}
	s.applyChatAutoUI(response)
	s.publishChatAutoReply(response)
	// #region agent log
	agentDebugLog("CA2", "chat_auto.go:runChatAutoLoop", "segment_scheduled", map[string]any{
		"segPath": "start", "segSetupMs": time.Since(segStart).Milliseconds(),
		"duracaoSec": response.AutoDom.DuracaoSegundos, "llmMs": llmMs, "staminaAfter": s.chatAutoStateSnapshot().Stamina,
	})
	// #endregion
	return nil
}

func (s *Server) chatAutoSegmentEnd() time.Time {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if player := s.chatAuto.player; player != nil && player.Running() {
		snap := player.Snapshot()
		remaining := player.TimelineEndMS() - snap.PlayheadMS
		if remaining > 0 {
			return time.Now().Add(time.Duration(remaining) * time.Millisecond)
		}
	}
	if !s.chatAuto.segmentEndsAt.IsZero() {
		return s.chatAuto.segmentEndsAt
	}
	return time.Now()
}

func (s *Server) appendOrRestartChatAutoMotion(
	ctx context.Context,
	generation uint64,
	settings config.Settings,
	prepared chatAutoPreparedSegment,
) (string, error) {
	if !s.chatAutoGenerationCurrent(generation) {
		return "", errors.New("chat auto generation changed")
	}

	s.chatAuto.mu.Lock()
	player := s.chatAuto.player
	s.chatAuto.mu.Unlock()

	if player != nil && player.Running() {
		if err := s.appendChatAutoSegmentPrepared(ctx, generation, prepared); err == nil {
			return "append", nil
		} else {
			timelineEnd := 0
			if player != nil {
				timelineEnd = player.TimelineEndMS()
			}
			// #region agent log
			agentDebugLog("CA8", "chat_auto.go:appendOrRestartChatAutoMotion", "append_skipped", map[string]any{
				"err": err.Error(), "playerNil": player == nil,
				"playerRunning": player != nil && player.Running(),
				"timelineEndMs": timelineEnd,
			})
			// #endregion
		}
	} else {
		// #region agent log
		agentDebugLog("CA8", "chat_auto.go:appendOrRestartChatAutoMotion", "append_skipped", map[string]any{
			"err": "chat auto player is not running", "playerNil": player == nil,
			"playerRunning": player != nil && player.Running(),
		})
		// #endregion
	}

	if err := s.restartChatAutoPlayer(ctx, generation, settings, prepared); err != nil {
		return "start", err
	}
	return "start", nil
}

func (s *Server) appendChatAutoSegmentPrepared(
	ctx context.Context,
	generation uint64,
	prepared chatAutoPreparedSegment,
) error {
	s.chatAuto.mu.Lock()
	player := s.chatAuto.player
	s.chatAuto.mu.Unlock()
	if player == nil || !player.Running() {
		return errors.New("chat auto player is not running")
	}

	if err := player.AppendExtension(prepared.session); err != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:appendChatAutoSegmentPrepared", "append_failed", map[string]any{
			"err": err.Error(), "points": len(prepared.session.Points),
		})
		// #endregion
		return err
	}

	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if generation != s.chatAuto.generation {
		return errors.New("chat auto generation changed during append")
	}
	s.applyChatAutoMotionStateLocked(prepared.response, prepared.stamina, prepared.mapped)
	remaining := time.Until(s.chatAuto.segmentEndsAt)
	if remaining < 0 {
		remaining = 0
	}
	s.chatAuto.segmentEndsAt = time.Now().Add(remaining + time.Duration(prepared.response.AutoDom.DuracaoSegundos)*time.Second)
	return nil
}

func (s *Server) restartChatAutoPlayer(
	ctx context.Context,
	generation uint64,
	settings config.Settings,
	prepared chatAutoPreparedSegment,
) error {
	commandTransport, err := s.newMotionCommandTransport()
	if err != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:restartChatAutoPlayer", "transport_failed", map[string]any{
			"err": err.Error(),
		})
		// #endregion
		return err
	}

	s.stopAndClearMotionEngine(ctx, "chat_auto_prepare")
	s.stopManualQueuePlayer(ctx)
	s.cancelChatChaosMotion(ctx)
	s.stopFreestyleChaosPlayer(ctx)
	s.stopChatAutoPlayer(ctx)

	player := manualqueue.NewPlayer(commandTransport)
	player.SetOnFinished(func() {
		s.chatAuto.mu.Lock()
		if s.chatAuto.player == player {
			s.chatAuto.player = nil
		}
		s.chatAuto.mu.Unlock()
	})
	if err := player.Start(ctx, prepared.session, 0); err != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:restartChatAutoPlayer", "player_start_failed", map[string]any{
			"err": err.Error(), "points": len(prepared.session.Points),
		})
		// #endregion
		return err
	}

	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if generation != s.chatAuto.generation {
		player.Stop(ctx)
		return errors.New("chat auto generation changed during restart")
	}
	s.chatAuto.player = player
	s.applyChatAutoMotionStateLocked(prepared.response, prepared.stamina, prepared.mapped)
	s.chatAuto.state.SegmentEndsInMS = 0
	s.chatAuto.segmentEndsAt = time.Now().Add(time.Duration(prepared.response.AutoDom.DuracaoSegundos) * time.Second)
	return nil
}

func (s *Server) fetchChatAutoTurn(ctx context.Context, settings config.Settings, userText string) (chatauto.Response, error) {
	s.setChatAutoLLMBusy(true, "")
	defer s.setChatAutoLLMBusy(false, "")

	provider, err := s.ensureLLMReady(ctx)
	if err != nil {
		return chatauto.Response{}, err
	}

	state := s.chatAutoStateSnapshot()
	transcript := s.chatAutoTranscript()
	prompt, _ := chat.BuiltinPromptSetByID(config.PromptSetAutoDomV1PTBR)
	systemPrompt := strings.TrimSpace(prompt.System) + "\n\n" + chatauto.AutoSessionContract

	userPrompt := chatauto.FormatUserTurn(state, transcript)
	if trimmed := strings.TrimSpace(userText); trimmed != "" {
		userPrompt = trimmed + "\n\n" + userPrompt
	}

	raw, err := provider.StreamChat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Model:       settings.LLM.Model,
		Temperature: chatAutoTemperature,
		MaxTokens:   chatAutoMaxTokens,
	}, nil)
	if err != nil {
		return chatauto.Response{}, err
	}
	// #region agent log
	agentDebugLog("CA1", "chat_auto.go:fetchChatAutoTurn", "raw_llm", map[string]any{
		"rawLen": len(raw), "rawPreview": truncateForLog(raw, 300),
	})
	// #endregion
	response, err := chatauto.ParseResponse(raw)
	if err != nil {
		// #region agent log
		agentDebugLog("CA1", "chat_auto.go:fetchChatAutoTurn", "parse_failed", map[string]any{
			"err": err.Error(), "rawPreview": truncateForLog(raw, 300),
		})
		// #endregion
		return chatauto.Response{}, err
	}
	s.applyChatAutoRamp(&response)
	return response, nil
}

func (s *Server) fallbackChatAutoResponse() (chatauto.Response, error) {
	state := s.chatAutoStateSnapshot()
	intent := chatauto.Intent{
		Humor:           chatauto.HumorDesejando,
		Posicao:         chatauto.PoseHandjob,
		Intensidade:     3,
		DuracaoSegundos: 50,
	}
	if state.Humor != "" {
		intent.Humor = state.Humor
	}
	if state.Posicao != "" {
		intent.Posicao = state.Posicao
	}
	intent, err := chatauto.ValidateIntent(intent)
	if err != nil {
		return chatauto.Response{}, err
	}
	response := chatauto.Response{
		Reply:   "Continuo no ritmo com você.",
		AutoDom: intent,
	}
	s.applyChatAutoRamp(&response)
	return response, nil
}

func (s *Server) applyChatAutoRamp(response *chatauto.Response) {
	settings, _ := s.store.Snapshot()
	s.chatAuto.mu.Lock()
	startedAt := s.chatAuto.sessionStartedAt
	s.chatAuto.mu.Unlock()
	if startedAt.IsZero() {
		return
	}
	progress, humor := chatauto.MoodAtElapsed(
		time.Since(startedAt),
		settings.AutoDom.DominatrixRampMinutes,
		settings.AutoDom.AllowDominatrix,
	)
	response.AutoDom.Humor = chatauto.EffectiveHumor(
		response.AutoDom.Humor,
		humor,
		settings.AutoDom.AllowDominatrix,
	)
	response.AutoDom.Intensidade = chatauto.BoostIntensity(response.AutoDom.Intensidade, progress)
}

func (s *Server) chatAutoPlayerRunning() bool {
	s.chatAuto.mu.Lock()
	player := s.chatAuto.player
	s.chatAuto.mu.Unlock()
	if player == nil {
		return false
	}
	return player.Snapshot().Running
}

func (s *Server) waitForChatAutoSegmentEnd(ctx context.Context, generation uint64) {
	for {
		if ctx.Err() != nil || !s.chatAutoGenerationCurrent(generation) {
			return
		}
		s.chatAuto.mu.Lock()
		end := s.chatAuto.segmentEndsAt
		s.chatAuto.mu.Unlock()
		if end.IsZero() {
			return
		}
		wait := time.Until(end)
		if wait <= 0 {
			return
		}
		s.waitChatAutoSegment(ctx, generation, wait)
		if !s.chatAutoPlayerRunning() {
			return
		}
	}
}

func (s *Server) dispatchChatAutoSegment(
	ctx context.Context,
	generation uint64,
	settings config.Settings,
	response chatauto.Response,
) (string, error) {
	if s.chatAutoPlayerRunning() {
		return "append", s.appendChatAutoSegment(ctx, generation, settings, response)
	}
	return "start", s.startChatAutoSegment(ctx, generation, settings, response)
}

func (s *Server) startChatAutoSegment(
	ctx context.Context,
	generation uint64,
	settings config.Settings,
	response chatauto.Response,
) error {
	session, stamina, intent, mapped, err := s.buildChatAutoSession(settings, response)
	if err != nil {
		return err
	}
	response.AutoDom = intent
	commandTransport, err := s.newMotionCommandTransport()
	if err != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:startChatAutoSegment", "transport_failed", map[string]any{
			"err": err.Error(),
		})
		// #endregion
		return err
	}

	s.stopAndClearMotionEngine(ctx, "chat_auto_prepare")
	s.stopManualQueuePlayer(ctx)
	s.cancelChatChaosMotion(ctx)
	s.stopFreestyleChaosPlayer(ctx)
	s.stopChatAutoPlayer(ctx)

	player := manualqueue.NewPlayer(commandTransport)
	player.SetOnFinished(func() {
		s.chatAuto.mu.Lock()
		if s.chatAuto.player == player {
			s.chatAuto.player = nil
		}
		s.chatAuto.mu.Unlock()
	})
	if err := player.Start(ctx, session, 0); err != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:startChatAutoSegment", "player_start_failed", map[string]any{
			"err": err.Error(), "points": len(session.Points),
		})
		// #endregion
		return err
	}

	s.chatAuto.mu.Lock()
	if generation != s.chatAuto.generation {
		player.Stop(ctx)
		s.chatAuto.mu.Unlock()
		return errors.New("chat auto generation changed during start")
	}
	s.chatAuto.player = player
	s.applyChatAutoMotionStateLocked(response, stamina, mapped)
	s.chatAuto.state.SegmentEndsInMS = 0
	s.chatAuto.segmentEndsAt = time.Now().Add(time.Duration(response.AutoDom.DuracaoSegundos) * time.Second)
	s.chatAuto.mu.Unlock()
	return nil
}

func (s *Server) appendChatAutoSegment(
	ctx context.Context,
	generation uint64,
	settings config.Settings,
	response chatauto.Response,
) error {
	session, stamina, intent, mapped, err := s.buildChatAutoSession(settings, response)
	if err != nil {
		return err
	}
	response.AutoDom = intent
	s.chatAuto.mu.Lock()
	player := s.chatAuto.player
	s.chatAuto.mu.Unlock()
	if player == nil || !player.Snapshot().Running {
		return errors.New("chat auto player is not running")
	}
	if err := player.AppendExtension(session); err != nil {
		// #region agent log
		agentDebugLog("CA6", "chat_auto.go:appendChatAutoSegment", "append_failed", map[string]any{
			"err": err.Error(),
		})
		// #endregion
		return err
	}
	s.chatAuto.mu.Lock()
	if generation != s.chatAuto.generation {
		s.chatAuto.mu.Unlock()
		return errors.New("chat auto generation changed during append")
	}
	s.applyChatAutoMotionStateLocked(response, stamina, mapped)
	remaining := time.Until(s.chatAuto.segmentEndsAt)
	if remaining < 0 {
		remaining = 0
	}
	s.chatAuto.segmentEndsAt = time.Now().Add(remaining + time.Duration(response.AutoDom.DuracaoSegundos)*time.Second)
	s.chatAuto.mu.Unlock()
	return nil
}

func (s *Server) buildChatAutoSession(
	settings config.Settings,
	response chatauto.Response,
) (manualqueue.Session, float64, chatauto.Intent, chatauto.MappedSegment, error) {
	s.chatAuto.mu.Lock()
	staminaBefore := s.chatAuto.state.Stamina
	intent := response.AutoDom
	stamina, intent := chatauto.ApplyStamina(staminaBefore, intent)
	staminaBeforeLog := staminaBefore
	s.chatAuto.mu.Unlock()

	// #region agent log
	agentDebugLog("CA5", "chat_auto.go:buildChatAutoSession", "stamina_drain", map[string]any{
		"before": staminaBeforeLog, "after": stamina, "intensidade": intent.Intensidade,
		"duracao": intent.DuracaoSegundos, "posicao": intent.Posicao,
		"poseRotated": staminaBeforeLog > 0 && stamina == 100 && intent.Posicao != response.AutoDom.Posicao,
	})
	// #endregion

	mapped := chatauto.MapIntent(intent, stamina)
	continueFrom := -1
	s.chatAuto.mu.Lock()
	if existing := s.chatAuto.player; existing != nil {
		if snap := existing.Snapshot(); snap.Running {
			continueFrom = int(math.Round(snap.PositionPct))
		}
	}
	s.chatAuto.mu.Unlock()

	// #nosec G404 -- procedural auto segments need non-deterministic micro-variance.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	session := buildChaosSessionForDurationFromPosition(
		mapped.Physics,
		settings.Motion,
		settings.Motion.HardwareSafetyLock,
		rng,
		mapped.DurationMillis,
		continueFrom,
	)
	if len(session.Points) == 0 {
		return manualqueue.Session{}, 0, intent, mapped, errors.New("chat auto session has no points")
	}
	// #region agent log
	agentDebugLog("CA4", "chat_auto.go:buildChatAutoSession", "session_built", map[string]any{
		"requestedMs": mapped.DurationMillis, "actualPoints": len(session.Points),
		"lastPointMs": session.Points[len(session.Points)-1].TimeMillis,
	})
	// #endregion
	return session, stamina, intent, mapped, nil
}

func (s *Server) waitChatAutoSegment(ctx context.Context, generation uint64, wait time.Duration) {
	if wait <= 0 {
		return
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}

func (s *Server) stopChatAutoPlayer(ctx context.Context) {
	s.chatAuto.mu.Lock()
	player := s.chatAuto.player
	s.chatAuto.player = nil
	s.chatAuto.mu.Unlock()
	if player != nil {
		player.Stop(ctx)
	}
}

func (s *Server) applyChatAutoMotionStateLocked(response chatauto.Response, stamina float64, mapped chatauto.MappedSegment) {
	if !s.chatAuto.sessionStartedAt.IsZero() {
		settings, _ := s.store.Snapshot()
		progress, _ := chatauto.MoodAtElapsed(
			time.Since(s.chatAuto.sessionStartedAt),
			settings.AutoDom.DominatrixRampMinutes,
			settings.AutoDom.AllowDominatrix,
		)
		s.chatAuto.state.MoodProgress = progress
	}
	s.chatAuto.state.Stamina = stamina
	s.chatAuto.state.Humor = response.AutoDom.Humor
	s.chatAuto.state.Posicao = response.AutoDom.Posicao
	s.chatAuto.state.Motion = chatauto.MotionFromMapped(mapped)
	s.chatAuto.state.Error = ""
}

func (s *Server) applyChatAutoUI(response chatauto.Response) {
	s.chatAuto.mu.Lock()
	s.chatAuto.state.LastReply = response.Reply
	s.chatAuto.state.ReplyPartial = ""
	s.chatAuto.mu.Unlock()
}

func (s *Server) setChatAutoLLMBusy(busy bool, partial string) {
	s.chatAuto.mu.Lock()
	s.chatAuto.state.LLMBusy = busy
	if partial != "" {
		s.chatAuto.state.ReplyPartial += partial
	} else if !busy {
		s.chatAuto.state.ReplyPartial = ""
	}
	s.chatAuto.mu.Unlock()
}

func (s *Server) setChatAutoError(message string) {
	s.chatAuto.mu.Lock()
	s.chatAuto.state.Error = message
	s.chatAuto.mu.Unlock()
}

func (s *Server) chatAutoGenerationCurrent(generation uint64) bool {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	return generation == s.chatAuto.generation && s.chatAuto.cancel != nil
}

func (s *Server) chatAutoTranscript() []string {
	s.lsoCompat.mu.RLock()
	defer s.lsoCompat.mu.RUnlock()
	out := make([]string, 0, len(s.lsoCompat.messages))
	for _, msg := range s.lsoCompat.messages {
		if msg.Content == "" {
			continue
		}
		out = append(out, msg.Content)
	}
	if len(out) > 12 {
		out = out[len(out)-12:]
	}
	return out
}
