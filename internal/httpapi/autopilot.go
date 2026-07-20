package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

// maxAutopilotSayRunes bounds one announced line; the model is asked for a
// short line, and an over-long one is truncated rather than dropped.
const (
	maxAutopilotSayRunes  = 150
	autopilotHistoryLimit = 12
)

// autopilotDecide is the mode manager's injected LLM curation step. It runs one
// bounded turn with recent canonical conversation context through the strict
// JSON contract,
// repair pass, and enabled-pattern curation as interactive chat, then maps the
// validated result onto the modes decision. Every error return makes the
// manager fall back to its deterministic planner — motion never stalls or
// stops because a model call failed.
func (s *Server) autopilotDecide(ctx context.Context, input modes.DecisionInput) (modes.Decision, error) {
	settings, _ := s.store.Snapshot()
	prompt, ok, err := s.personalization.prompts.Resolve(settings.LLM.PromptSet)
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve prompt set: %w", err)
	}
	if !ok {
		prompt, _ = chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	}
	memories, err := s.personalization.memory.PromptTexts()
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve memories: %w", err)
	}
	capabilities := chatCapabilities(settings.LLM)
	patternChoices, err := s.chatPatternChoicesFor(capabilities)
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve pattern catalog: %w", err)
	}
	history, err := s.autopilotHistory()
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve conversation context: %w", err)
	}
	provider, err := s.newLLMProvider(ctx, settings.LLM)
	if err != nil {
		return modes.Decision{}, err
	}
	service := chat.Service{
		Provider:              provider,
		Prompt:                prompt,
		Model:                 settings.LLM.Model,
		MaxTokens:             settings.LLM.MaxOutputTokens,
		ReasoningMode:         settings.LLM.ReasoningMode,
		ReasoningBudgetTokens: managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current),
		Memories:              memories,
		Patterns:              patternChoices,
		Capabilities:          &capabilities,
	}

	message := chat.AutopilotDecisionMessage(chat.AutopilotContext{
		Style:            input.Style,
		SegmentIndex:     input.SegmentIndex,
		RecentPatternIDs: input.RecentPatternIDs,
		SpeedMinPercent:  input.SpeedMinPercent,
		SpeedMaxPercent:  input.SpeedMaxPercent,
		LastSay:          input.LastSay,
	})
	result, err := service.Complete(ctx, chat.Request{Message: message, History: history}, nil)
	if err != nil {
		return modes.Decision{}, err
	}
	if result.Malformed {
		return modes.Decision{}, errors.New("autopilot decision stayed malformed: " + result.MalformedError)
	}
	return s.mapAutopilotResult(result)
}

func (s *Server) autopilotHistory() ([]llm.Message, error) {
	if s.chatLog == nil {
		return nil, nil
	}
	messages, err := s.chatLog.Recent(autopilotHistoryLimit)
	if err != nil {
		return nil, err
	}
	history := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		history = append(history, llm.Message{Role: message.Role, Content: message.Content})
	}
	return history, nil
}

// mapAutopilotResult converts one validated chat result into a modes decision.
func (s *Server) mapAutopilotResult(result chat.Result) (modes.Decision, error) {
	say := strings.TrimSpace(result.Response.Reply)
	command := result.Response.Motion
	if command == nil || command.Action == chat.MotionActionNone || command.Action == chat.MotionActionStop {
		// No motion change — including a model-requested stop, which is
		// deliberately ignored: stopping the device belongs to the user.
		return modes.Decision{Hold: true, Say: say}, nil
	}

	speed := 0
	if command.Intensity != nil {
		speed = *command.Intensity
	} else if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	}
	if command.PatternID == "" || speed <= 0 {
		// A motion change without a curated pattern and intensity holds the
		// current segment instead of guessing a new one.
		return modes.Decision{Hold: true, Say: say}, nil
	}

	resolved, found, err := s.patterns.ResolveEnabled(command.PatternID)
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve autopilot pattern: %w", err)
	}
	if !found {
		// The enabled set changed while the model was deciding; never apply a
		// now-disabled selection.
		return modes.Decision{Hold: true, Say: say}, nil
	}
	segment := modes.Segment{
		PatternID:    motion.PatternID(resolved.ID),
		SpeedPercent: speed,
	}
	if command.Area != "" {
		if focus, ok := zoneAreaFocus(command.Area); ok {
			segment.AreaFocus = focus
		}
	}
	return modes.Decision{
		Segment: segment,
		Pattern: &resolved,
		Say:     say,
	}, nil
}

// autopilotAnnounce publishes one Autopilot line with the same lockstep ordering
// as interactive chat (ADR 0003): the line enters the shared log first, then
// optionally enters TTS. A raced Emergency Stop deletes the log row and cancels
// the speech so a stopped session never keeps talking.
func (s *Server) autopilotAnnounce(ctx context.Context, say string) {
	say = strings.TrimSpace(say)
	if say == "" || ctx.Err() != nil {
		return
	}
	if runes := []rune(say); len(runes) > maxAutopilotSayRunes {
		say = string(runes[:maxAutopilotSayRunes])
	}
	stopSequence := s.stopSequence.Load()
	s.chatSpeechMu.Lock()
	defer s.chatSpeechMu.Unlock()
	if s.autopilotAnnouncementInvalidated(ctx, stopSequence) {
		return
	}
	sessionID, err := s.chatLog.ActiveSessionID()
	if err != nil {
		s.logger.Warn("Autopilot chat session unavailable", "error", err)
		return
	}
	settings, _ := s.store.Snapshot()
	diagnostics := &chat.MessageDiagnostics{
		Source:    "autopilot",
		Provider:  settings.LLM.Provider,
		Model:     settings.LLM.Model,
		PromptSet: settings.LLM.PromptSet,
	}
	replySeq := s.appendChatMessageTo(sessionID, chat.MessageRoleAssistant, say, "", diagnostics)
	if s.autopilotAnnouncementInvalidated(ctx, stopSequence) {
		s.deleteAutopilotLine(replySeq)
		return
	}
	if replySeq == 0 {
		// Unlike an interactive SSE reply, an autonomous line has no other
		// display channel. Never speak text that failed to enter the log.
		return
	}
	worker := s.voice.Worker(voice.RoleTTS)
	if autopilotSpeechBacklogged(worker.Status()) {
		// Autonomous lines remain visible in Chat, but they never deepen an
		// existing speech backlog. The next idle line may speak normally.
		return
	}
	speech := s.enqueueSpeech(say)
	if speech == nil {
		return
	}
	if s.autopilotAnnouncementInvalidated(ctx, stopSequence) {
		worker.Cancel(speech)
		s.deleteAutopilotLine(replySeq)
		return
	}
	s.rememberAutopilotSpeech(replySeq, speech.ID)
}

func (s *Server) autopilotAnnouncementInvalidated(ctx context.Context, stopSequence uint64) bool {
	return ctx.Err() != nil || s.stopSequence.Load() != stopSequence
}

func (s *Server) deleteAutopilotLine(seq int64) {
	if seq <= 0 || s.chatLog == nil {
		return
	}
	if err := s.chatLog.Delete(seq); err != nil {
		s.logger.Warn("delete Stop-invalidated autopilot line", "seq", seq, "error", err)
	}
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
