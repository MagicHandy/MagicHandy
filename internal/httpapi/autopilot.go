package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

// maxAutopilotSayRunes bounds one announced line; the model is asked for a
// short line, and an over-long one is truncated rather than dropped.
const maxAutopilotSayRunes = 300

// autopilotDecide is the mode manager's injected LLM curation step. It runs
// one bounded, history-free chat turn through the same strict-JSON contract,
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
	patternChoices, err := s.chatPatternChoices()
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve pattern catalog: %w", err)
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
	}

	message := chat.AutopilotDecisionMessage(chat.AutopilotContext{
		Style:            input.Style,
		SegmentIndex:     input.SegmentIndex,
		RecentPatternIDs: input.RecentPatternIDs,
		SpeedMinPercent:  input.SpeedMinPercent,
		SpeedMaxPercent:  input.SpeedMaxPercent,
		LastSay:          input.LastSay,
	})
	result, err := service.Complete(ctx, chat.Request{Message: message}, nil)
	if err != nil {
		return modes.Decision{}, err
	}
	if result.Malformed {
		return modes.Decision{}, errors.New("autopilot decision stayed malformed: " + result.MalformedError)
	}
	return s.mapAutopilotResult(result)
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
	return modes.Decision{
		Segment: modes.Segment{
			PatternID:    motion.PatternID(resolved.ID),
			SpeedPercent: speed,
		},
		Pattern: &resolved,
		Say:     say,
	}, nil
}

// autopilotAnnounce publishes one Autopilot spoken line with the same
// lockstep ordering as interactive chat (ADR 0003): the line enters the
// shared log first, then TTS. A raced Emergency Stop deletes the log row and
// cancels the speech so a stopped session never keeps talking.
func (s *Server) autopilotAnnounce(say string) {
	say = strings.TrimSpace(say)
	if say == "" {
		return
	}
	if runes := []rune(say); len(runes) > maxAutopilotSayRunes {
		say = string(runes[:maxAutopilotSayRunes])
	}
	stopSequence := s.stopSequence.Load()
	replySeq := s.appendChatMessage(chat.MessageRoleAssistant, say, "")
	if s.stopSequence.Load() != stopSequence {
		if replySeq > 0 && s.chatLog != nil {
			if err := s.chatLog.Delete(replySeq); err != nil {
				s.logger.Warn("delete Stop-invalidated autopilot line", "seq", replySeq, "error", err)
			}
		}
		return
	}
	speech := s.enqueueSpeech(say)
	if speech != nil && s.stopSequence.Load() != stopSequence {
		s.voice.Worker(voice.RoleTTS).Cancel(speech)
	}
}
