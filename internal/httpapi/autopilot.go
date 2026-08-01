package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

const maxAutopilotSayRunes = 150

// autopilotDecide runs the strict motion-only model contract. It never asks for
// or publishes a chat line.
func (s *Server) autopilotDecide(ctx context.Context, input modes.DecisionInput) (modes.Decision, error) {
	response, err := s.autopilotModelTurn(ctx, input, chat.AutopilotKindMotion)
	if err != nil {
		return modes.Decision{}, err
	}
	return s.mapAutopilotResponse(response, input)
}

// autopilotDecideSpeech runs the independent spoken-check-in contract. Its
// motion capability is reduced by the saved speech authority before prompt
// composition and again by the strict parser.
func (s *Server) autopilotDecideSpeech(ctx context.Context, input modes.DecisionInput) (modes.Decision, error) {
	response, err := s.autopilotModelTurn(ctx, input, chat.AutopilotKindSpeech)
	if err != nil {
		return modes.Decision{}, err
	}
	return s.mapAutopilotResponse(response, input)
}

func (s *Server) autopilotModelTurn(
	ctx context.Context,
	input modes.DecisionInput,
	kind chat.AutopilotKind,
) (chat.AutopilotResponse, error) {
	settings, _ := s.store.Snapshot()
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		return chat.AutopilotResponse{}, fmt.Errorf("resolve active chat: %w", err)
	}
	promptContext, err := s.loadInteractiveChatPromptContext(sessionID, settings.LLM)
	if err != nil {
		return chat.AutopilotResponse{}, fmt.Errorf("resolve conversation context: %w", err)
	}
	promptID := effectivePersonaPromptSet(settings.LLM.PromptSet, promptContext.Persona)
	prompt, memories, _, err := s.resolveInteractiveChatPersonalization(promptID)
	if err != nil {
		return chat.AutopilotResponse{}, fmt.Errorf("resolve personalization: %w", err)
	}
	capabilities := promptContext.Capabilities
	if kind == chat.AutopilotKindSpeech {
		capabilities = autopilotSpeechCapabilities(capabilities, settings.Autopilot.SpeechMotionAuthority)
	}
	patternChoices, err := s.chatPatternChoicesFor(capabilities)
	if err != nil {
		return chat.AutopilotResponse{}, fmt.Errorf("resolve pattern catalog: %w", err)
	}
	provider, err := s.newLLMProvider(ctx, settings.LLM)
	if err != nil {
		return chat.AutopilotResponse{}, err
	}
	motionContext := s.chatMotionContext(settings.Motion)
	service := chat.AutopilotService{
		Provider:              provider,
		Prompt:                prompt,
		Model:                 settings.LLM.Model,
		MaxTokens:             settings.LLM.MaxOutputTokens,
		ReasoningMode:         settings.LLM.ReasoningMode,
		ReasoningBudgetTokens: managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current),
		Memories:              memories,
		Patterns:              patternChoices,
		MotionContext:         &motionContext,
		ConversationContext:   promptContext.ConversationContext,
		Capabilities:          capabilities,
	}
	modelContext := chat.AutopilotContext{
		Style:            input.Style,
		SegmentIndex:     input.SegmentIndex,
		RecentPatternIDs: input.RecentPatternIDs,
		SpeedMinPercent:  input.SpeedMinPercent,
		SpeedMaxPercent:  input.SpeedMaxPercent,
		LastSay:          input.LastSay,
		CurrentPatternID: string(input.CurrentPatternID),
		CurrentSpeed:     input.CurrentSpeed,
		CurrentArea:      chatAreaZone(input.CurrentAreaFocus),
		AreaFocusEnabled: capabilities.AreaFocus,
		// The manager already applied the tracking and arc switches when it built
		// the input, so a disabled switch arrives as false here and the prompt
		// omits the section entirely.
		SessionTracking:       input.SessionTracking,
		SessionSeconds:        input.SessionSeconds,
		SecondsAtCurrentSpeed: input.SecondsAtCurrentSpeed,
		SpeedTrend:            input.SpeedTrend,
		ArcEnabled:            input.ArcEnabled,
		ArcPercent:            input.ArcPercent,
	}
	message := chat.AutopilotMotionMessage(modelContext)
	if kind == chat.AutopilotKindSpeech {
		message = chat.AutopilotSpeechMessage(modelContext)
	}
	providerCtx, _, releaseLLM, err := s.llmRequests.acquire(ctx, llmRequestAutonomous)
	if err != nil {
		return chat.AutopilotResponse{}, err
	}
	defer releaseLLM()
	return service.Complete(providerCtx, kind, chat.Request{
		Message: message,
		History: promptContext.History,
	})
}

func autopilotSpeechCapabilities(capabilities chat.Capabilities, authority string) chat.Capabilities {
	switch authority {
	case config.AutopilotSpeechMotionFull:
		return capabilities
	case config.AutopilotSpeechMotionStyle:
		capabilities.Patterns = false
		return capabilities
	default:
		capabilities.Motion = false
		capabilities.Patterns = false
		capabilities.AreaFocus = false
		return capabilities
	}
}

// mapAutopilotResponse converts validated semantic model output into a bounded
// modes decision. Stop/start requests and semantic no-ops become true holds.
func (s *Server) mapAutopilotResponse(
	response chat.AutopilotResponse,
	input modes.DecisionInput,
) (modes.Decision, error) {
	say := strings.TrimSpace(response.Reply)
	next := modes.TimingPreference(response.Next)
	variability := modes.VariabilityPreference(response.Variability)
	arc := strings.TrimSpace(response.Arc)
	command := response.Motion
	if command == nil || command.Action == chat.MotionActionNone ||
		command.Action == chat.MotionActionStop || command.Action == chat.MotionActionStart {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability, ArcIntent: arc}, nil
	}

	patternID := strings.TrimSpace(command.PatternID)
	if patternID == "" {
		patternID = string(input.CurrentPatternID)
	}
	speed := input.CurrentSpeed
	if command.Intensity != nil {
		speed = *command.Intensity
	} else if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	}
	var areaFocus *motion.AreaFocus
	if input.CurrentAreaFocus != nil {
		focus := *input.CurrentAreaFocus
		areaFocus = &focus
	}
	if command.Area != "" {
		focus, ok := zoneAreaFocus(command.Area)
		if !ok {
			return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability, ArcIntent: arc}, nil
		}
		areaFocus = focus
	}
	if patternID == "" || speed <= 0 {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability, ArcIntent: arc}, nil
	}
	if strings.EqualFold(patternID, string(input.CurrentPatternID)) &&
		speed == input.CurrentSpeed && sameAreaFocus(areaFocus, input.CurrentAreaFocus) {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability, ArcIntent: arc}, nil
	}

	resolved, found, err := s.patterns.ResolveEnabled(patternID)
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve Autopilot pattern: %w", err)
	}
	if !found {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability, ArcIntent: arc}, nil
	}
	return modes.Decision{
		Segment: modes.Segment{
			PatternID:    motion.PatternID(resolved.ID),
			SpeedPercent: speed,
			AreaFocus:    areaFocus,
		},
		Pattern:     &resolved,
		Say:         say,
		Next:        next,
		Variability: variability,
		ArcIntent:   arc,
	}, nil
}

// mapAutopilotResult keeps the old test/helper surface while all production
// calls use the dedicated contract above.
func (s *Server) mapAutopilotResult(result chat.Result, input modes.DecisionInput) (modes.Decision, error) {
	return s.mapAutopilotResponse(chat.AutopilotResponse{
		Reply:  result.Response.Reply,
		Motion: result.Response.Motion,
		Next:   chat.AutopilotTimingNormal,
	}, input)
}

func sameAreaFocus(left, right *motion.AreaFocus) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// autopilotAnnounce publishes through canonical Chat first, then optionally
// queues TTS. The returned request ID lets the scheduler start its next interval
// after actual browser playback.
func (s *Server) autopilotAnnounce(ctx context.Context, say string) modes.Announcement {
	say = strings.TrimSpace(say)
	if say == "" || ctx.Err() != nil {
		return modes.Announcement{}
	}
	if runes := []rune(say); len(runes) > maxAutopilotSayRunes {
		say = string(runes[:maxAutopilotSayRunes])
	}
	stopSequence := s.stopSequence.Load()
	s.chatSpeechMu.Lock()
	defer s.chatSpeechMu.Unlock()
	if s.autopilotAnnouncementInvalidated(ctx, stopSequence) {
		return modes.Announcement{}
	}
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		s.logger.Warn("Autopilot chat session unavailable", "error", err)
		return modes.Announcement{}
	}
	settings, _ := s.store.Snapshot()
	activePersona, err := s.activeSessionPersona()
	if err != nil {
		s.logger.Warn("resolve Autopilot persona", "error", err)
		return modes.Announcement{}
	}
	promptID := effectivePersonaPromptSet(settings.LLM.PromptSet, activePersona)
	if _, found, resolveErr := s.personalization.prompts.Resolve(promptID); resolveErr != nil || !found {
		promptID = chat.DefaultPromptSetID
	}
	diagnostics := &chat.MessageDiagnostics{
		Source:    "autopilot",
		Provider:  settings.LLM.Provider,
		Model:     settings.LLM.Model,
		PromptSet: promptID,
	}
	if activePersona != nil {
		diagnostics.PersonaID = activePersona.ID
		diagnostics.PersonaName = activePersona.Name
	}
	if promptContext, promptErr := s.chatLog.PromptContext(sessionID); promptErr != nil {
		s.logger.Warn("read Autopilot chat mood", "error", promptErr)
	} else {
		diagnostics.Mood = promptContext.CurrentMood
	}
	replySeq, err := s.chatLog.AppendPendingAssistantTo(sessionID, say, diagnostics)
	if err != nil {
		s.logger.Warn("stage Autopilot chat line", "error", err)
		return modes.Announcement{}
	}
	replyCommitted := false
	defer func() {
		if !replyCommitted {
			s.deletePendingChatReply(replySeq)
		}
	}()
	if s.autopilotAnnouncementInvalidated(ctx, stopSequence) {
		return modes.Announcement{}
	}
	if err := s.chatLog.CommitPending(replySeq); err != nil {
		s.logger.Warn("commit Autopilot chat line", "error", err)
		return modes.Announcement{}
	}
	replyCommitted = true
	if s.autopilotAnnouncementInvalidated(ctx, stopSequence) {
		return modes.Announcement{}
	}

	announcement := modes.Announcement{Published: true}
	worker := s.voice.Worker(voice.RoleTTS)
	if autopilotSpeechBacklogged(worker.Status()) {
		return announcement
	}
	speech := s.enqueueSpeechAt(ctx, stopSequence, say)
	if speech == nil {
		return announcement
	}
	if s.autopilotAnnouncementInvalidated(ctx, stopSequence) {
		worker.Cancel(speech)
		return modes.Announcement{}
	}
	s.rememberAutopilotSpeech(replySeq, speech.ID)
	announcement.RequestID = speech.ID
	announcement.AwaitPlayback = true
	return announcement
}

func (s *Server) autopilotCanAnnounce() bool {
	settings, _ := s.store.Snapshot()
	if !settings.Voice.Enabled || !settings.Voice.SpeakReplies {
		return true
	}
	return !autopilotSpeechBacklogged(s.voice.Worker(voice.RoleTTS).Status())
}

func (s *Server) autopilotAnnouncementInvalidated(ctx context.Context, stopSequence uint64) bool {
	return ctx.Err() != nil || s.stopSequence.Load() != stopSequence
}

func autopilotSpeechBacklogged(status voice.WorkerStatus) bool {
	return status.QueueDepth > 0 || status.WorkerQueue > 0
}

// rememberAutopilotSpeech associates a canonical chat row with its ephemeral
// browser-playback request. The caller holds chatSpeechMu.
func (s *Server) rememberAutopilotSpeech(replySeq int64, requestID string) {
	if s.chatSpeechRequests == nil {
		s.chatSpeechRequests = make(map[int64]string)
	}
	s.chatSpeechRequests[replySeq] = requestID
	oldest := replySeq - chat.MessageLogCap
	for seq := range s.chatSpeechRequests {
		if seq <= oldest {
			delete(s.chatSpeechRequests, seq)
		}
	}
}
