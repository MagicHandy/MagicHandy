package httpapi

import (
	"context"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chatauto"
	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/manualqueue"
)

const chatAutoRoteiroPrefetchStamina = 20.0

func (s *Server) runChatAutoRoteiroPrefetchLoop(ctx context.Context, generation uint64, settings config.Settings) {
	for {
		if ctx.Err() != nil || !s.chatAutoGenerationCurrent(generation) {
			return
		}
		if s.chatAutoShouldPrefetchRoteiro() {
			s.chatAutoPrefetchRoteiro(ctx, generation, settings)
		}
		if s.chatAutoShouldEnqueueRoteiroMotion(settings) {
			s.chatAutoEnqueuePendingRoteiroMotion(ctx, generation, settings)
		}
		time.Sleep(chatAutoTickInterval)
	}
}

func (s *Server) runChatAutoChatPrefetchLoop(ctx context.Context, generation uint64, settings config.Settings) {
	for {
		if ctx.Err() != nil || !s.chatAutoGenerationCurrent(generation) {
			return
		}
		userText := s.chatAutoPopPendingUser()
		if s.chatAutoShouldFetchChatReply(userText != "") {
			s.chatAutoFetchAndPublishReply(ctx, settings, userText)
		}
		time.Sleep(chatAutoTickInterval)
	}
}

func (s *Server) chatAutoShouldPrefetchRoteiro() bool {
	s.chatAuto.mu.Lock()
	defer s.chatAuto.mu.Unlock()
	if s.chatAuto.roteiroPrefetchActive {
		return false
	}
	if !s.chatAuto.hasActiveRoteiro {
		return true
	}
	if s.chatAuto.hasPendingRoteiro {
		return false
	}
	return s.chatAuto.state.Stamina < chatAutoRoteiroPrefetchStamina
}

func (s *Server) chatAutoShouldEnqueueRoteiroMotion(settings config.Settings) bool {
	s.chatAuto.mu.Lock()
	hasPending := s.chatAuto.hasPendingRoteiro
	enqueued := s.chatAuto.pendingMotionEnqueued
	depth := len(s.chatAuto.playbackQueue)
	prefetching := s.chatAuto.roteiroPrefetchActive
	s.chatAuto.mu.Unlock()
	if !hasPending || enqueued || prefetching || depth >= chatAutoQueueTargetDepth {
		return false
	}
	if !s.chatAutoPlayerRunning() {
		return true
	}
	return s.chatAutoTimelineRemainingMS() <= int(s.chatAutoPrefetchLead(settings).Milliseconds())
}

func (s *Server) chatAutoShouldFetchChatReply(userPending bool) bool {
	if userPending {
		return true
	}
	s.chatAuto.mu.Lock()
	hasRoteiro := s.chatAuto.hasActiveRoteiro
	chatBusy := s.chatAuto.chatPrefetchActive
	lastAt := s.chatAuto.lastAutoReplyPublished
	s.chatAuto.mu.Unlock()
	if !hasRoteiro || chatBusy {
		return false
	}
	if !s.chatAutoActive() {
		return false
	}
	if !lastAt.IsZero() && time.Since(lastAt) < 42*time.Second {
		return false
	}
	return true
}

func (s *Server) chatAutoPrefetchRoteiro(ctx context.Context, generation uint64, settings config.Settings) {
	s.chatAuto.mu.Lock()
	nextCycle := s.chatAuto.hasActiveRoteiro
	s.chatAuto.roteiroPrefetchActive = true
	s.chatAuto.mu.Unlock()

	start := time.Now()
	roteiro, err := s.fetchChatAutoRoteiro(ctx, settings, nextCycle)

	s.chatAuto.mu.Lock()
	s.chatAuto.roteiroPrefetchActive = false
	if ctx.Err() != nil || !s.chatAutoGenerationCurrent(generation) {
		s.chatAuto.mu.Unlock()
		return
	}
	if err != nil {
		roteiro = s.fallbackChatAutoRoteiro()
	}
	if nextCycle {
		s.chatAuto.pendingRoteiro = roteiro
		s.chatAuto.hasPendingRoteiro = true
		s.chatAuto.pendingMotionEnqueued = false
	} else {
		s.chatAuto.activeRoteiro = roteiro
		s.chatAuto.hasActiveRoteiro = true
		s.chatAuto.activePlan = chatauto.PlanFromRoteiro(roteiro)
	}
	s.chatAuto.mu.Unlock()

	// #region agent log
	agentDebugLog("CA11", "chat_auto_roteiro.go:chatAutoPrefetchRoteiro", "roteiro_cached", map[string]any{
		"nextCycle": nextCycle, "llmMs": time.Since(start).Milliseconds(),
		"posicao": roteiro.Posicao, "intensidade": roteiro.Intensidade,
		"velocidade": roteiro.Velocidade, "humor": roteiro.Humor,
		"stamina": s.chatAutoStateSnapshot().Stamina,
	})
	// #endregion
}

func (s *Server) chatAutoEnqueuePendingRoteiroMotion(ctx context.Context, generation uint64, settings config.Settings) {
	s.chatAuto.mu.Lock()
	if !s.chatAuto.hasPendingRoteiro {
		s.chatAuto.mu.Unlock()
		return
	}
	roteiro := s.chatAuto.pendingRoteiro
	s.chatAuto.mu.Unlock()

	prepared, err := s.prepareChatAutoMotionFromRoteiro(settings, roteiro, s.chatAutoProjectedStamina(), true)
	if err != nil {
		return
	}
	s.chatAutoEnqueue(chatAutoQueuedSegment{prepared: prepared, fromUser: false})
	s.chatAuto.mu.Lock()
	s.chatAuto.pendingMotionEnqueued = true
	s.chatAuto.mu.Unlock()
	// #region agent log
	agentDebugLog("CA12", "chat_auto_roteiro.go:chatAutoEnqueuePendingRoteiroMotion", "roteiro_motion_enqueued", map[string]any{
		"duracao": prepared.intent.DuracaoSegundos, "posicao": roteiro.Posicao,
		"queueDepth": s.chatAutoQueueDepth(),
	})
	// #endregion
	s.chatAutoDrainPlaybackQueue(ctx, generation, settings)
}

func (s *Server) chatAutoFetchAndPublishReply(ctx context.Context, settings config.Settings, userText string) {
	s.chatAuto.mu.Lock()
	if !s.chatAuto.hasActiveRoteiro {
		s.chatAuto.mu.Unlock()
		return
	}
	roteiro := s.chatAuto.activeRoteiro
	s.chatAuto.chatPrefetchActive = true
	s.chatAuto.mu.Unlock()

	start := time.Now()
	reply, err := s.fetchChatAutoReply(ctx, settings, roteiro, userText)

	s.chatAuto.mu.Lock()
	s.chatAuto.chatPrefetchActive = false
	s.chatAuto.mu.Unlock()

	if err != nil || strings.TrimSpace(reply) == "" {
		return
	}
	s.publishChatAutoReply(chatauto.Response{Reply: reply})
	// #region agent log
	agentDebugLog("CA13", "chat_auto_roteiro.go:chatAutoFetchAndPublishReply", "chat_reply_published", map[string]any{
		"llmMs": time.Since(start).Milliseconds(), "fromUser": userText != "",
		"replyPreview": truncateForLog(reply, 80), "posicao": roteiro.Posicao,
	})
	// #endregion
}

func (s *Server) chatAutoActivatePendingRoteiroLocked() {
	if !s.chatAuto.hasPendingRoteiro {
		return
	}
	s.chatAuto.activeRoteiro = s.chatAuto.pendingRoteiro
	s.chatAuto.activePlan = chatauto.PlanFromRoteiro(s.chatAuto.pendingRoteiro)
	s.chatAuto.pendingRoteiro = chatauto.Roteiro{}
	s.chatAuto.hasPendingRoteiro = false
	s.chatAuto.pendingMotionEnqueued = false
	// #region agent log
	agentDebugLog("CA14", "chat_auto_roteiro.go:chatAutoActivatePendingRoteiroLocked", "roteiro_rotated", map[string]any{
		"posicao": s.chatAuto.activeRoteiro.Posicao,
		"intensidade": s.chatAuto.activeRoteiro.Intensidade,
	})
	// #endregion
}

func (s *Server) fetchChatAutoRoteiro(ctx context.Context, settings config.Settings, nextCycle bool) (chatauto.Roteiro, error) {
	provider, err := s.ensureLLMReady(ctx)
	if err != nil {
		return chatauto.Roteiro{}, err
	}
	state := s.chatAutoStateSnapshot()
	transcript := s.chatAutoTranscript()
	prompt, _ := chat.BuiltinPromptSetByID(config.PromptSetAutoDomV1PTBR)
	systemPrompt := strings.TrimSpace(prompt.System) + "\n\n" + chatauto.FormatSpiceSystemBlock() + "\n\n" + chatauto.FormatRoteiroContract()
	systemPrompt = chat.AppendUserProfile(systemPrompt, settings.UserProfile, config.PromptSetAutoDomV1PTBR)
	userPrompt := chatauto.FormatRoteiroUserTurn(state, transcript, nextCycle)

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
		return chatauto.Roteiro{}, err
	}
	roteiro, err := chatauto.ParseRoteiroResponse(raw)
	if err != nil {
		return chatauto.Roteiro{}, err
	}
	state = s.chatAutoStateSnapshot()
	roteiro.Posicao = chatauto.ResolvePose(state.Posicao, roteiro.Posicao)
	s.applyChatAutoRampRoteiro(&roteiro)
	return roteiro, nil
}

func (s *Server) fetchChatAutoReply(ctx context.Context, settings config.Settings, roteiro chatauto.Roteiro, userText string) (string, error) {
	userInitiated := strings.TrimSpace(userText) != ""
	if userInitiated {
		s.setChatAutoLLMBusy(true, "")
		defer s.setChatAutoLLMBusy(false, "")
	}
	provider, err := s.ensureLLMReady(ctx)
	if err != nil {
		return "", err
	}
	state := s.chatAutoStateSnapshot()
	transcript := s.chatAutoTranscript()
	recentAssistant := s.chatAutoRecentAssistantReplies(4)
	spice := chatauto.ResolveSpiceLevel(roteiro.Humor, state.MoodProgress, settings.AutoDom.AllowDominatrix)
	prompt, _ := chat.BuiltinPromptSetByID(config.PromptSetAutoDomV1PTBR)
	systemPrompt := strings.TrimSpace(prompt.System) + "\n\n" + chatauto.FormatSpiceSystemBlock() + "\n\n" + chatauto.FormatReplyContract(roteiro, spice)
	systemPrompt = chat.AppendUserProfile(systemPrompt, settings.UserProfile, config.PromptSetAutoDomV1PTBR)
	userPrompt := chatauto.FormatReplyUserTurn(state, roteiro, transcript, recentAssistant)
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
		return "", err
	}
	return chatauto.ParseReplyResponse(raw)
}

func (s *Server) applyChatAutoRampRoteiro(roteiro *chatauto.Roteiro) {
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
	roteiro.Humor = chatauto.EffectiveHumor(roteiro.Humor, humor, settings.AutoDom.AllowDominatrix)
	roteiro.Intensidade = chatauto.BoostIntensity(roteiro.Intensidade, progress)
}

func (s *Server) fallbackChatAutoRoteiro() chatauto.Roteiro {
	state := s.chatAutoStateSnapshot()
	roteiro := chatauto.Roteiro{
		Humor:       chatauto.HumorDesejando,
		Posicao:     chatauto.PoseHandjob,
		Intensidade: 3,
		Velocidade:  4,
	}
	if state.Humor != "" {
		roteiro.Humor = state.Humor
	}
	if state.Posicao != "" {
		roteiro.Posicao = state.Posicao
	}
	roteiro, _ = chatauto.ValidateRoteiro(roteiro)
	return roteiro
}

func (s *Server) prepareChatAutoMotionFromRoteiro(
	settings config.Settings,
	roteiro chatauto.Roteiro,
	staminaBefore float64,
	useRecover bool,
) (chatAutoPreparedSegment, error) {
	intent := chatauto.RoteiroToIntent(roteiro, 1)
	durationSec := chatauto.ProceduralBlockDuration(staminaBefore, intent)
	intent.DuracaoSegundos = durationSec
	session, stamina, intent, mapped, err := s.buildChatAutoSessionFromRoteiro(
		settings, roteiro, durationSec, staminaBefore, useRecover,
	)
	if err != nil {
		return chatAutoPreparedSegment{err: err}, err
	}
	response := chatauto.Response{AutoDom: intent}
	return chatAutoPreparedSegment{
		response:    response,
		llmIntent:   intent,
		session:     session,
		stamina:     stamina,
		intent:      intent,
		mapped:      mapped,
		roteiro:     roteiro,
		fromRoteiro: true,
	}, nil
}

func (s *Server) buildChatAutoSessionFromRoteiro(
	settings config.Settings,
	roteiro chatauto.Roteiro,
	blockDurationSec int,
	staminaBefore float64,
	useRecover bool,
) (manualqueue.Session, float64, chatauto.Intent, chatauto.MappedSegment, error) {
	plan := chatauto.PlanFromRoteiro(roteiro)
	return s.buildChatAutoSession(settings, chatauto.Response{}, plan, blockDurationSec, staminaBefore, useRecover, true)
}
