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
	diagnosticspkg "github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const (
	chatAutoQueueTargetDepth   = 2
	chatAutoLoopBridgeSeconds  = 30
	chatAutoLoopBridgeLeadMS   = 10000
	chatAutoTickInterval       = 250 * time.Millisecond
	chatAutoMinBoundaryWait    = 1 * time.Second
	chatAutoMaxTokens          = 160
	chatAutoTemperature        = 0.62
)

type chatAutoPreparedSegment struct {
	response    chatauto.Response
	llmIntent   chatauto.Intent
	session     manualqueue.Session
	stamina     float64
	intent      chatauto.Intent
	mapped      chatauto.MappedSegment
	roteiro     chatauto.Roteiro
	fromRoteiro bool
	err             error
	llmMs       int64
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
	activePlan             chatauto.ScenePlan
	activeRoteiro          chatauto.Roteiro
	pendingRoteiro         chatauto.Roteiro
	hasActiveRoteiro       bool
	hasPendingRoteiro      bool
	pendingMotionEnqueued  bool
	roteiroPrefetchActive  bool
	chatPrefetchActive     bool
	lastBridgeAt           time.Time
	lastStaminaTick        time.Time
	lastAutoReplyPublished time.Time
	lastMotionDrainAt      time.Time
	lastDrainTimelineEndMS int
}

func (s *Server) chatAutoStateSnapshot() chatauto.State {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	state := s.chatAuto.state
	settings, _ := s.store.Snapshot()
	if !s.chatAuto.sessionStartedAt.IsZero() {
		elapsed := time.Since(s.chatAuto.sessionStartedAt)
		progress, humor := chatauto.MoodAtElapsed(
			elapsed,
			settings.AutoDom.DominatrixRampMinutes,
			settings.AutoDom.AllowDominatrix,
		)
		state.MoodProgress = progress
		state.Humor = chatauto.EffectiveHumor(state.Humor, humor, settings.AutoDom.AllowDominatrix)
	}
	state.SpiceLevel = chatauto.ResolveSpiceLevel(
		state.Humor,
		state.MoodProgress,
		settings.AutoDom.AllowDominatrix,
	)
	if player := s.chatAuto.player; player != nil && player.Running() {
		snap := player.Snapshot()
		remaining := player.TimelineEndMS() - snap.PlayheadMS
		if remaining > 0 {
			state.SegmentEndsInMS = int64(remaining)
		}
	} else if !s.chatAuto.segmentEndsAt.IsZero() {
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
		"spice_level":        state.SpiceLevel,
		"mood_progress":      state.MoodProgress,
		"posicao":            string(state.Posicao),
		"scene_intensidade":  state.SceneIntensidade,
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

func (s *Server) chatAutoProjectedStamina() float64 {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	stamina := s.chatAuto.state.Stamina
	for _, item := range s.chatAuto.playbackQueue {
		intent := item.prepared.intent
		if intent == (chatauto.Intent{}) {
			intent = item.prepared.response.AutoDom
		}
		before := stamina
		after := chatauto.ApplyProceduralStamina(stamina, intent, float64(intent.DuracaoSegundos))
		if chatauto.ProceduralCycleCompleted(before, after, intent) {
			stamina = 100
		} else {
			stamina = after
		}
	}
	return stamina
}

func (s *Server) commitChatAutoSegmentState(prepared *chatAutoPreparedSegment, useRecover bool) {
	s.chatAuto.mu.Lock()
	s.commitChatAutoSegmentStateLocked(prepared, useRecover)
	s.chatAuto.mu.Unlock()
}

func (s *Server) commitChatAutoSegmentStateLocked(prepared *chatAutoPreparedSegment, useRecover bool) {
	var stamina float64
	var intent chatauto.Intent
	if prepared.intent != (chatauto.Intent{}) {
		intent = prepared.intent
		if useRecover {
			stamina = prepared.stamina
		} else {
			// Bridge filler: keep live stamina and roteiro pose; only extend motion.
			stamina = s.chatAuto.state.Stamina
			intent.Posicao = s.chatAuto.state.Posicao
			intent.Humor = s.chatAuto.state.Humor
			if scene := s.chatAuto.state.SceneIntensidade; scene > 0 {
				intent.Intensidade = scene
			}
		}
	} else {
		staminaBefore := s.chatAuto.state.Stamina
		currentPose := s.chatAuto.state.Posicao
		llmIntent := prepared.llmIntent
		if llmIntent == (chatauto.Intent{}) {
			llmIntent = prepared.response.AutoDom
		}
		if useRecover {
			stamina, intent = chatauto.ApplyStaminaCommit(staminaBefore, currentPose, llmIntent)
		} else {
			stamina, intent = chatauto.ApplyStaminaForBridge(staminaBefore, currentPose, llmIntent)
		}
	}
	prepared.response.AutoDom = intent
	prepared.stamina = stamina
	prepared.intent = intent
	if prepared.fromRoteiro {
		s.chatAuto.activePlan = chatauto.PlanFromRoteiro(prepared.roteiro)
		s.chatAuto.lastStaminaTick = time.Now()
	} else if useRecover {
		s.chatAuto.activePlan = chatauto.PlanFromIntent(intent)
	}
	s.applyChatAutoMotionStateLocked(prepared.response, stamina, prepared.mapped, intent)
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
	s.chatAuto.hasActiveRoteiro = false
	s.chatAuto.hasPendingRoteiro = false
	s.chatAuto.pendingMotionEnqueued = false
	s.chatAuto.roteiroPrefetchActive = false
	s.chatAuto.chatPrefetchActive = false
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
	if s.chatAutoShouldDeferAutonomousStart(settings) {
		go s.runChatAutoRoteiroPrefetchLoop(ctx, generation, settings)
		go s.runChatAutoChatPrefetchLoop(ctx, generation, settings)
	} else if err := s.runChatAutoFirstTurn(ctx, generation, settings); err != nil {
		return
	} else {
		go s.runChatAutoRoteiroPrefetchLoop(ctx, generation, settings)
		go s.runChatAutoChatPrefetchLoop(ctx, generation, settings)
	}

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
			s.chatAuto.mu.Lock()
			s.syncChatAutoSegmentEndLocked()
			s.chatAuto.mu.Unlock()
			s.chatAutoDrainPlaybackQueue(ctx, generation, settings)
			s.chatAutoMaybeLoopBridge(ctx, generation, settings)
			s.chatAutoTickPlaybackStamina(settings)
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
			fallbackLLM := fallback.AutoDom
			session, stamina, intent, mapped, buildErr := s.buildChatAutoSession(
				settings, fallback, chatauto.PlanFromIntent(fallback.AutoDom),
				s.chatAutoProceduralBlockSec(settings), s.chatAutoProjectedStamina(), true, false,
			)
			if buildErr != nil {
				time.Sleep(time.Second)
				continue
			}
			fallback.AutoDom = intent
			prepared = chatAutoPreparedSegment{
				response:  fallback,
				llmIntent: fallbackLLM,
				session:   session,
				stamina:   stamina,
				intent:    intent,
				mapped:    mapped,
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

func (s *Server) chatAutoSegmentBounds(settings config.Settings) chatauto.SegmentDurationBounds {
	minSec, maxSec := settings.AutoDom.SegmentDurationBounds()
	return chatauto.SegmentDurationBounds{MinSec: minSec, MaxSec: maxSec}
}

func (s *Server) chatAutoProceduralBlockSec(settings config.Settings) int {
	return s.chatAutoSegmentBounds(settings).MaxSec
}

func (s *Server) chatAutoActivePlan() chatauto.ScenePlan {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if s.chatAuto.activePlan.Posicao != "" {
		return s.chatAuto.activePlan
	}
	return chatauto.PlanFromState(s.chatAuto.state)
}

func (s *Server) chatAutoPrefetchLead(settings config.Settings) time.Duration {
	return settings.AutoDom.PrefetchLead()
}

func (s *Server) chatAutoShouldDeferAutonomousStart(settings config.Settings) bool {
	if !settings.AutoDom.ShouldWaitForUserMessage() {
		return false
	}
	return !s.chatAutoHasUserEngagement()
}

func (s *Server) chatAutoHasUserEngagement() bool {
	s.chatAuto.mu.Lock()
	pending := len(s.chatAuto.pendingUserTexts)
	s.chatAuto.mu.Unlock()
	if pending > 0 {
		return true
	}
	s.lsoCompat.mu.RLock()
	defer s.lsoCompat.mu.RUnlock()
	for _, msg := range s.lsoCompat.messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "user") && strings.TrimSpace(msg.Content) != "" {
			return true
		}
	}
	return false
}

func (s *Server) chatAutoShouldThrottlePrefetch() bool {
	settings, _ := s.store.Snapshot()
	if s.chatAutoShouldDeferAutonomousStart(settings) {
		return true
	}

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
	leadMS := int(s.chatAutoPrefetchLead(settings).Milliseconds())
	if s.chatAutoTimelineRemainingMS() > leadMS {
		return true
	}
	prefetchLead := s.chatAutoPrefetchLead(settings)
	untilPrefetch := time.Until(s.chatAutoSegmentEnd()) - prefetchLead
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
	leadMS := int(s.chatAutoPrefetchLead(settings).Milliseconds())
	if remainingMS > leadMS && !front.fromUser {
		return
	}

	s.chatAuto.mu.Lock()
	player := s.chatAuto.player
	timelineEnd := 0
	if player != nil {
		timelineEnd = player.TimelineEndMS()
	}
	lastDrainEnd := s.chatAuto.lastDrainTimelineEndMS
	lastDrainAt := s.chatAuto.lastMotionDrainAt
	s.chatAuto.mu.Unlock()
	if !front.fromUser && remainingMS <= leadMS && timelineEnd > 0 && timelineEnd <= lastDrainEnd {
		return
	}
	if !front.fromUser && remainingMS <= 0 && !lastDrainAt.IsZero() && time.Since(lastDrainAt) < 5*time.Second {
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
	if s.shouldPublishChatAutoReply(item, remainingMS) {
		s.publishChatAutoReply(item.prepared.response)
	}
	s.commitChatAutoSegmentState(&item.prepared, true)

	s.chatAuto.mu.Lock()
	staminaAfter := s.chatAuto.state.Stamina
	if player := s.chatAuto.player; player != nil {
		s.chatAuto.lastDrainTimelineEndMS = player.TimelineEndMS()
	}
	s.chatAuto.lastMotionDrainAt = time.Now()
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
	if remainingMS > chatAutoLoopBridgeLeadMS {
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
	if !lastBridge.IsZero() && time.Since(lastBridge) < 5*time.Second {
		return
	}

	prepared, err := s.buildChatAutoLoopBridge(settings)
	if err != nil {
		agentDebugLog("CA9", "chat_auto.go:chatAutoMaybeLoopBridge", "bridge_build_failed", map[string]any{
			"err": err.Error(),
		})
		return
	}
	mode, err := s.appendOrRestartChatAutoMotion(ctx, generation, settings, prepared)
	if err != nil {
		agentDebugLog("CA9", "chat_auto.go:chatAutoMaybeLoopBridge", "bridge_append_failed", map[string]any{
			"err": err.Error(), "mode": mode,
		})
		return
	}
	s.commitChatAutoSegmentState(&prepared, false)

	s.chatAuto.mu.Lock()
	staminaAfter := s.chatAuto.state.Stamina
	posicao := s.chatAuto.state.Posicao
	s.chatAuto.lastBridgeAt = time.Now()
	s.chatAuto.mu.Unlock()

	// #region agent log
	agentDebugLog("CA9", "chat_auto.go:chatAutoMaybeLoopBridge", "loop_bridge_appended", map[string]any{
		"remainingMs": remainingMS, "points": len(prepared.session.Points),
		"staminaAfter": staminaAfter, "posicao": posicao, "blockSec": chatAutoLoopBridgeSeconds,
		"intMin": prepared.intent.IntensidadeMin, "intMax": prepared.intent.IntensidadeMax,
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
	llmIntent := response.AutoDom
	state := s.chatAutoStateSnapshot()
	llmIntent.Posicao = chatauto.ResolvePose(state.Posicao, llmIntent.Posicao)
	response.AutoDom = llmIntent
	plan := chatauto.PlanFromIntent(llmIntent)
	blockSec := s.chatAutoProceduralBlockSec(settings)
	session, stamina, intent, mapped, err := s.buildChatAutoSession(
		settings, response, plan, blockSec, s.chatAutoProjectedStamina(), true, false,
	)
	if err != nil {
		return chatAutoPreparedSegment{err: err, llmMs: time.Since(start).Milliseconds()}, err
	}
	response.AutoDom = intent
	return chatAutoPreparedSegment{
		response:  response,
		llmIntent: llmIntent,
		session:   session,
		stamina:   stamina,
		intent:    intent,
		mapped:    mapped,
		llmMs:     time.Since(start).Milliseconds(),
	}, nil
}

func (s *Server) buildChatAutoLoopBridge(settings config.Settings) (chatAutoPreparedSegment, error) {
	state := s.chatAutoStateSnapshot()
	plan := s.chatAutoActivePlan()
	plan.Posicao = state.Posicao
	staminaBefore := state.Stamina
	blockIntent := chatauto.PlanToIntent(plan, staminaBefore, chatAutoLoopBridgeSeconds)
	response := chatauto.Response{Reply: "", AutoDom: blockIntent}
	llmIntent := blockIntent
	session, stamina, intent, mapped, err := s.buildChatAutoSession(
		settings, response, plan, chatAutoLoopBridgeSeconds, staminaBefore, false, false,
	)
	if err != nil {
		return chatAutoPreparedSegment{}, err
	}
	response.AutoDom = intent
	return chatAutoPreparedSegment{
		response:  response,
		llmIntent: llmIntent,
		session:   session,
		stamina:   stamina,
		intent:    intent,
		mapped:    mapped,
	}, nil
}

func (s *Server) chatAutoTickPlaybackStamina(settings config.Settings) {
	if !s.chatAutoPlayerRunning() {
		// #region agent log
		s.chatAuto.mu.Lock()
		playerNil := s.chatAuto.player == nil
		s.chatAuto.mu.Unlock()
		agentDebugLog("CA10", "chat_auto.go:chatAutoTickPlaybackStamina", "stamina_tick_skipped", map[string]any{
			"hypothesisId": "H12",
			"player_nil":   playerNil,
			"active":       s.chatAutoActive(),
		})
		// #endregion
		return
	}
	s.chatAuto.mu.Lock()
	last := s.chatAuto.lastStaminaTick
	player := s.chatAuto.player
	hasRoteiro := s.chatAuto.hasActiveRoteiro
	roteiro := s.chatAuto.activeRoteiro
	s.chatAuto.mu.Unlock()
	if player == nil || !player.Running() {
		return
	}

	now := time.Now()
	if last.IsZero() {
		s.chatAuto.mu.Lock()
		s.chatAuto.lastStaminaTick = now
		s.chatAuto.mu.Unlock()
		return
	}
	elapsed := now.Sub(last).Seconds()
	if elapsed < 0.2 {
		return
	}

	intent := s.chatAutoPlaybackIntent(hasRoteiro, roteiro)
	s.chatAuto.mu.Lock()
	before := s.chatAuto.state.Stamina
	after := chatauto.ApplyProceduralStamina(before, intent, elapsed)
	s.chatAuto.state.Stamina = after
	s.chatAuto.lastStaminaTick = now
	cycleDone := chatauto.ProceduralCycleCompleted(before, after, intent)
	if cycleDone {
		s.chatAuto.state.Stamina = 100
		s.chatAutoActivatePendingRoteiroLocked()
	}
	s.chatAuto.mu.Unlock()

	// #region agent log
	agentDebugLog("CA10", "chat_auto.go:chatAutoTickPlaybackStamina", "playback_stamina_tick", map[string]any{
		"before": before, "after": after, "elapsedSec": elapsed,
		"rate": chatauto.StaminaNetRatePerSecond(intent, before), "recovering": chatauto.IsRecoveringIntent(intent),
		"intensidade": intent.Intensidade, "cycleDone": cycleDone,
	})
	// #endregion
}

func (s *Server) chatAutoPlaybackIntent(hasRoteiro bool, roteiro chatauto.Roteiro) chatauto.Intent {
	if hasRoteiro {
		return chatauto.RoteiroToIntent(roteiro, 1)
	}
	state := s.chatAutoStateSnapshot()
	intensity := state.SceneIntensidade
	if intensity <= 0 {
		intensity = 4
	}
	return chatauto.Intent{
		Humor:       state.Humor,
		Posicao:     state.Posicao,
		Intensidade: intensity,
	}
}

func (s *Server) shouldPublishChatAutoReply(item chatAutoQueuedSegment, remainingBeforeMS int) bool {
	if item.fromUser {
		return strings.TrimSpace(item.prepared.response.Reply) != ""
	}
	if strings.TrimSpace(item.prepared.response.Reply) == "" {
		return false
	}
	if remainingBeforeMS <= 500 {
		return false
	}
	s.chatAuto.mu.Lock()
	lastAt := s.chatAuto.lastAutoReplyPublished
	s.chatAuto.mu.Unlock()
	if !lastAt.IsZero() && time.Since(lastAt) < 42*time.Second {
		return false
	}
	return true
}

func (s *Server) publishChatAutoReply(response chatauto.Response) {
	reply := strings.TrimSpace(response.Reply)
	if reply == "" {
		return
	}
	s.chatAuto.mu.Lock()
	s.chatAuto.lastAutoReplyPublished = time.Now()
	s.chatAuto.mu.Unlock()
	s.appendChatMessage("assistant", reply)
}

func (s *Server) runChatAutoFirstTurn(ctx context.Context, generation uint64, settings config.Settings) error {
	loopStart := time.Now()
	// #region agent log
	agentDebugLog("CA2", "chat_auto.go:runChatAutoFirstTurn", "turn_start", map[string]any{
		"generation": generation, "stamina": s.chatAutoStateSnapshot().Stamina,
	})
	// #endregion

	roteiro, err := s.fetchChatAutoRoteiro(ctx, settings, false)
	roteiroMs := time.Since(loopStart).Milliseconds()
	if err != nil {
		roteiro = s.fallbackChatAutoRoteiro()
	}
	s.chatAuto.mu.Lock()
	s.chatAuto.activeRoteiro = roteiro
	s.chatAuto.hasActiveRoteiro = true
	s.chatAuto.activePlan = chatauto.PlanFromRoteiro(roteiro)
	s.chatAuto.mu.Unlock()

	prepared, err := s.prepareChatAutoMotionFromRoteiro(settings, roteiro, s.chatAutoStateSnapshot().Stamina, true)
	if err != nil {
		return err
	}

	segStart := time.Now()
	segErr := s.startChatAutoPreparedSegment(ctx, generation, prepared)
	if segErr != nil {
		return segErr
	}
	s.commitChatAutoSegmentState(&prepared, true)

	go func() {
		reply, replyErr := s.fetchChatAutoReply(ctx, settings, roteiro, "")
		if replyErr == nil && strings.TrimSpace(reply) != "" {
			s.publishChatAutoReply(chatauto.Response{Reply: reply})
		}
	}()

	// #region agent log
	agentDebugLog("CA2", "chat_auto.go:runChatAutoFirstTurn", "segment_scheduled", map[string]any{
		"segSetupMs": time.Since(segStart).Milliseconds(),
		"roteiroMs": roteiroMs, "duracaoSec": prepared.intent.DuracaoSegundos,
		"posicao": roteiro.Posicao, "staminaAfter": s.chatAutoStateSnapshot().Stamina,
	})
	// #endregion
	return nil
}

func (s *Server) startChatAutoPreparedSegment(ctx context.Context, generation uint64, prepared chatAutoPreparedSegment) error {
	commandTransport, err := s.newMotionCommandTransport()
	if err != nil {
		return err
	}
	s.stopAndClearMotionEngine(ctx, "chat_auto_prepare")
	s.stopManualQueuePlayer(ctx)
	s.cancelChatChaosMotion(ctx)
	s.stopFreestyleChaosPlayer(ctx)
	s.stopChatAutoPlayer(ctx)

	player := manualqueue.NewPlayer(commandTransport)
	s.configureChatAutoPlayer(player)
	player.SetOnFinished(func() {
		s.chatAutoPlayerFinished(player)
	})
	if err := player.Start(ctx, prepared.session, 0); err != nil {
		return err
	}

	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if generation != s.chatAuto.generation {
		player.Stop(ctx)
		return errors.New("chat auto generation changed during start")
	}
	s.chatAuto.player = player
	s.syncChatAutoSegmentEndLocked()
	s.chatAuto.state.SegmentEndsInMS = 0
	s.chatAuto.segmentEndsAt = time.Now().Add(time.Duration(prepared.intent.DuracaoSegundos) * time.Second)
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
		return time.Now()
	}
	// Player idle — never throttle prefetch on a stale wall-clock deadline.
	return time.Now()
}

func (s *Server) syncChatAutoSegmentEndLocked() {
	player := s.chatAuto.player
	if player == nil || !player.Running() {
		return
	}
	snap := player.Snapshot()
	remaining := player.TimelineEndMS() - snap.PlayheadMS
	if remaining < 0 {
		remaining = 0
	}
	s.chatAuto.segmentEndsAt = time.Now().Add(time.Duration(remaining) * time.Millisecond)
}

func (s *Server) chatAutoPlayerFinished(player *manualqueue.Player) {
	s.chatAuto.mu.Lock()
	if s.chatAuto.player != player {
		s.chatAuto.mu.Unlock()
		return
	}
	generation := s.chatAuto.generation
	recover := s.chatAuto.cancel != nil
	s.chatAuto.player = nil
	s.chatAuto.segmentEndsAt = time.Now()
	s.chatAuto.mu.Unlock()
	// #region agent log
	agentDebugLog("H6", "chat_auto.go:chatAutoPlayerFinished", "segment_end_reset", map[string]any{
		"reason":  "player_finished",
		"recover": recover,
	})
	// #endregion
	if !recover || !s.chatAutoGenerationCurrent(generation) {
		return
	}
	settings, _ := s.store.Snapshot()
	go func() {
		prepared, err := s.buildChatAutoLoopBridge(settings)
		if err != nil {
			agentDebugLog("H11", "chat_auto.go:chatAutoPlayerFinished", "recover_bridge_failed", map[string]any{
				"err": err.Error(),
			})
			return
		}
		mode, err := s.appendOrRestartChatAutoMotion(context.Background(), generation, settings, prepared)
		agentDebugLog("H11", "chat_auto.go:chatAutoPlayerFinished", "recover_motion", map[string]any{
			"mode": mode, "err": errString(err),
		})
	}()
}

func (s *Server) chatAutoRegionChanged(nextRegiao string) bool {
	s.chatAuto.mu.Lock()
	lastRegiao := s.chatAuto.state.Motion.Regiao
	s.chatAuto.mu.Unlock()
	return lastRegiao != "" && lastRegiao != nextRegiao
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
		regionChanged := s.chatAutoRegionChanged(prepared.mapped.Physics.Regiao)
		if !regionChanged {
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
	s.syncChatAutoSegmentEndLocked()
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
	s.configureChatAutoPlayer(player)
	player.SetOnFinished(func() {
		s.chatAutoPlayerFinished(player)
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
	s.syncChatAutoSegmentEndLocked()
	s.chatAuto.state.SegmentEndsInMS = 0
	s.chatAuto.segmentEndsAt = time.Now().Add(time.Duration(prepared.response.AutoDom.DuracaoSegundos) * time.Second)
	return nil
}

func (s *Server) fetchChatAutoTurn(ctx context.Context, settings config.Settings, userText string) (chatauto.Response, error) {
	userInitiated := strings.TrimSpace(userText) != ""
	if userInitiated {
		s.setChatAutoLLMBusy(true, "")
		defer s.setChatAutoLLMBusy(false, "")
	}

	provider, err := s.ensureLLMReady(ctx)
	if err != nil {
		return chatauto.Response{}, err
	}

	state := s.chatAutoStateSnapshot()
	transcript := s.chatAutoTranscript()
	recentAssistant := s.chatAutoRecentAssistantReplies(4)
	prompt, _ := chat.BuiltinPromptSetByID(config.PromptSetAutoDomV1PTBR)
	bounds := s.chatAutoSegmentBounds(settings)
	systemPrompt := strings.TrimSpace(prompt.System) + "\n\n" + chatauto.FormatSpiceSystemBlock() + "\n\n" + chatauto.FormatAutoSessionContract(bounds)
	systemPrompt = chat.AppendUserProfile(systemPrompt, settings.UserProfile, config.PromptSetAutoDomV1PTBR)

	userPrompt := chatauto.FormatUserTurn(state, transcript, recentAssistant, settings.AutoDom.AllowDominatrix)
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
	response, err := chatauto.ParseResponseWithBounds(raw, bounds)
	if err != nil {
		// #region agent log
		agentDebugLog("CA1", "chat_auto.go:fetchChatAutoTurn", "parse_failed", map[string]any{
			"err": err.Error(), "rawPreview": truncateForLog(raw, 300),
		})
		// #endregion
		return chatauto.Response{}, err
	}
	llmPose := response.AutoDom.Posicao
	s.applyChatAutoRamp(&response)
	if s.handyMotionLogEnabled() {
		logChatAutoPoseDecision(
			s,
			"fetch_turn",
			string(llmPose),
			string(llmPose),
			string(response.AutoDom.Posicao),
			s.chatAutoStateSnapshot().Stamina,
			s.chatAutoStateSnapshot().Stamina,
		)
	}
	return response, nil
}

func (s *Server) fallbackChatAutoResponse() (chatauto.Response, error) {
	state := s.chatAutoStateSnapshot()
	settings, _ := s.store.Snapshot()
	minSec, maxSec := settings.AutoDom.SegmentDurationBounds()
	bounds := chatauto.SegmentDurationBounds{MinSec: minSec, MaxSec: maxSec}
	prefer := minSec + (maxSec-minSec)/2
	intent := chatauto.Intent{
		Humor:          chatauto.HumorDesejando,
		Posicao:        chatauto.PoseHandjob,
		IntensidadeMin: 2,
		IntensidadeMax: 4,
		DuracaoSegundos: prefer,
	}
	if state.Humor != "" {
		intent.Humor = state.Humor
	}
	if state.Posicao != "" {
		intent.Posicao = state.Posicao
	}
	intent, err := chatauto.ValidateIntentWithBounds(intent, bounds)
	if err != nil {
		return chatauto.Response{}, err
	}
	spice := chatauto.ResolveSpiceLevel(
		intent.Humor,
		state.MoodProgress,
		settings.AutoDom.AllowDominatrix,
	)
	response := chatauto.Response{
		Reply:   chatauto.FallbackReply(spice),
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
	response.AutoDom.IntensidadeMin = chatauto.BoostIntensity(response.AutoDom.IntensidadeMin, progress)
	response.AutoDom.IntensidadeMax = chatauto.BoostIntensity(response.AutoDom.IntensidadeMax, progress)
	if response.AutoDom.IntensidadeMax < response.AutoDom.IntensidadeMin {
		response.AutoDom.IntensidadeMax = response.AutoDom.IntensidadeMin
	}
	response.AutoDom.Intensidade = (response.AutoDom.IntensidadeMin + response.AutoDom.IntensidadeMax) / 2
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
	llmIntent := response.AutoDom
	s.chatAuto.mu.Lock()
	staminaBefore := s.chatAuto.state.Stamina
	s.chatAuto.mu.Unlock()
	session, _, intent, mapped, err := s.buildChatAutoSession(
		settings, response, chatauto.PlanFromIntent(response.AutoDom),
		s.chatAutoProceduralBlockSec(settings), staminaBefore, true, false,
	)
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
	s.configureChatAutoPlayer(player)
	player.SetOnFinished(func() {
		s.chatAutoPlayerFinished(player)
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
	s.syncChatAutoSegmentEndLocked()
	s.chatAuto.state.SegmentEndsInMS = 0
	s.chatAuto.segmentEndsAt = time.Now().Add(time.Duration(response.AutoDom.DuracaoSegundos) * time.Second)
	prepared := chatAutoPreparedSegment{
		response:  response,
		llmIntent: llmIntent,
		session:   session,
		intent:    intent,
		mapped:    mapped,
	}
	s.commitChatAutoSegmentStateLocked(&prepared, true)
	s.chatAuto.mu.Unlock()
	return nil
}

func (s *Server) appendChatAutoSegment(
	ctx context.Context,
	generation uint64,
	settings config.Settings,
	response chatauto.Response,
) error {
	llmIntent := response.AutoDom
	session, _, intent, mapped, err := s.buildChatAutoSession(
		settings, response, chatauto.PlanFromIntent(response.AutoDom),
		s.chatAutoProceduralBlockSec(settings), s.chatAutoProjectedStamina(), true, false,
	)
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
	prepared := chatAutoPreparedSegment{
		response:  response,
		llmIntent: llmIntent,
		session:   session,
		intent:    intent,
		mapped:    mapped,
	}
	s.commitChatAutoSegmentStateLocked(&prepared, true)
	s.syncChatAutoSegmentEndLocked()
	s.chatAuto.mu.Unlock()
	return nil
}

func (s *Server) buildChatAutoSession(
	settings config.Settings,
	response chatauto.Response,
	plan chatauto.ScenePlan,
	blockDurationSec int,
	staminaBefore float64,
	useRecover bool,
	fromRoteiro bool,
) (manualqueue.Session, float64, chatauto.Intent, chatauto.MappedSegment, error) {
	if blockDurationSec < 1 {
		blockDurationSec = s.chatAutoProceduralBlockSec(settings)
	}
	s.chatAuto.mu.Lock()
	currentPose := s.chatAuto.state.Posicao
	s.chatAuto.mu.Unlock()

	blockIntent := chatauto.PlanToIntent(plan, staminaBefore, blockDurationSec)
	var stamina float64
	var intent chatauto.Intent
	if fromRoteiro && useRecover {
		stamina, intent = chatauto.ApplyRoteiroStaminaCommit(staminaBefore, currentPose, blockIntent)
	} else if useRecover {
		stamina, intent = chatauto.ApplyStaminaCommit(staminaBefore, currentPose, blockIntent)
	} else if !fromRoteiro {
		// Loop bridge filler: pose and stamina are owned by the active roteiro + playback ticks.
		stamina = staminaBefore
		intent = blockIntent
		intent.Posicao = currentPose
	} else {
		stamina, intent = chatauto.ApplyStaminaForBridge(staminaBefore, currentPose, blockIntent)
	}
	staminaBeforeLog := staminaBefore

	logChatAutoPoseDecision(
		s,
		"build_session",
		string(plan.Posicao),
		string(intent.Posicao),
		string(intent.Posicao),
		staminaBeforeLog,
		stamina,
	)

	// #region agent log
	agentDebugLog("CA5", "chat_auto.go:buildChatAutoSession", "stamina_drain", map[string]any{
		"before": staminaBeforeLog, "after": stamina, "intensidade": intent.Intensidade,
		"intMin": intent.IntensidadeMin, "intMax": intent.IntensidadeMax,
		"netRate": chatauto.StaminaNetRatePerSecond(intent, staminaBeforeLog), "recovering": chatauto.IsRecoveringIntent(intent),
		"duracao": intent.DuracaoSegundos, "posicao": intent.Posicao,
		"useRecover": useRecover, "fromRoteiro": fromRoteiro,
	})
	// #endregion

	mapped := chatauto.MapIntent(intent, stamina)
	var blendFrom *motion.MotionBlendState
	continueFrom := -1
	s.chatAuto.mu.Lock()
	lastRegiao := s.chatAuto.state.Motion.Regiao
	if existing := s.chatAuto.player; existing != nil {
		if snap := existing.Snapshot(); snap.Running {
			regionChanged := lastRegiao != "" && lastRegiao != mapped.Physics.Regiao
			if regionChanged && motion.IsTurboTipo(mapped.Physics.TipoBatida) {
				continueFrom = -1
			} else if !regionChanged {
				state := estimateBlendStateFromPlayer(existing)
				blendFrom = &state
				continueFrom = int(math.Round(state.Position))
			} else {
				state := estimateBlendStateFromPlayer(existing)
				continueFrom = int(math.Round(state.Position))
			}
		}
	}
	s.chatAuto.mu.Unlock()

	// #nosec G404 -- procedural auto segments need non-deterministic micro-variance.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var session manualqueue.Session
	if blendFrom != nil {
		session = buildChaosSessionForDurationFromBlend(
			mapped.Physics,
			settings.Motion,
			settings.Motion.HardwareSafetyLock,
			rng,
			mapped.DurationMillis,
			*blendFrom,
		)
	} else {
		session = buildChaosSessionForDurationFromPosition(
			mapped.Physics,
			settings.Motion,
			settings.Motion.HardwareSafetyLock,
			rng,
			mapped.DurationMillis,
			continueFrom,
		)
	}
	if len(session.Points) == 0 {
		return manualqueue.Session{}, 0, intent, mapped, errors.New("chat auto session has no points")
	}
	session.Continuous = true
	if s.handyMotionLogEnabled() {
		first := session.Points[0]
		last := session.Points[len(session.Points)-1]
		durationMS := int(last.TimeMillis)
		s.recordHandyMotionLog("chat_auto_segment_built", "chat_auto", diagnosticspkg.HandyLogEntry{
			DurationMS: &durationMS,
			Details: map[string]any{
				"posicao":          intent.Posicao,
				"humor":            intent.Humor,
				"intensidade":      intent.Intensidade,
				"duracao_segundos": intent.DuracaoSegundos,
				"points":           len(session.Points),
				"continue_from":    continueFrom,
				"first_time_ms":    first.TimeMillis,
				"last_time_ms":     last.TimeMillis,
				"first_pos":        first.PositionPercent,
				"last_pos":         last.PositionPercent,
				"regiao":           mapped.Physics.Regiao,
				"tipo_batida":      mapped.Physics.TipoBatida,
				"velocidade":       mapped.Physics.Velocidade,
				"atraso_ms":        mapped.Physics.AtrasoMS,
			},
		})
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

func (s *Server) applyChatAutoMotionStateLocked(response chatauto.Response, stamina float64, mapped chatauto.MappedSegment, intent chatauto.Intent) {
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
	s.chatAuto.state.Humor = intent.Humor
	s.chatAuto.state.Posicao = intent.Posicao
	s.chatAuto.state.SceneIntensidade = intent.Intensidade
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

func (s *Server) chatAutoRecentAssistantReplies(limit int) []string {
	if limit <= 0 {
		return nil
	}
	s.lsoCompat.mu.RLock()
	defer s.lsoCompat.mu.RUnlock()
	out := make([]string, 0, limit)
	for i := len(s.lsoCompat.messages) - 1; i >= 0 && len(out) < limit; i-- {
		msg := s.lsoCompat.messages[i]
		if msg.Role != "assistant" {
			continue
		}
		trimmed := strings.TrimSpace(msg.Content)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (s *Server) configureChatAutoPlayer(player *manualqueue.Player) {
	if player == nil || !s.handyMotionLogEnabled() {
		return
	}
	player.SetDispatchDebug(func(event string, data map[string]any) {
		entry := diagnosticspkg.HandyLogEntry{Details: data}
		if raw, ok := data["buffer_ahead_ms"]; ok {
			switch value := raw.(type) {
			case int64:
				entry.BufferAheadMS = &value
			case int:
				converted := int64(value)
				entry.BufferAheadMS = &converted
			}
		}
		if starvation, ok := data["starvation_risk"].(bool); ok && starvation {
			s.recordHandyMotionLog("player_starvation_risk", "chat_auto", entry)
			agentDebugLog("H1", "chat_auto.go:configureChatAutoPlayer", "player_starvation_risk", data)
			return
		}
		if event == "player_finish_triggered" {
			agentDebugLog("H4", "chat_auto.go:configureChatAutoPlayer", "player_finish_triggered", data)
		}
		s.recordHandyMotionLog(event, "chat_auto", entry)
	})
}
