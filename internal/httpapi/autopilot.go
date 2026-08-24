package httpapi

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

const maxAutopilotSayRunes = 150

func autopilotCosmeticFeedback(input modes.DecisionInput) string {
	return fmt.Sprintf(
		"The previous update compiled as continuity. Motion change rate is %d/8; the same felt phrase has persisted for %d seconds across %d reconsiderations, with %d consecutive holds. Choose none only for genuinely fitting continuity; otherwise reconsider the motion concept and encode a materially different destination in one step. Another nearby numerical variant of the same idea is still continuity.",
		input.MotionChangeLevel, input.SecondsAtCurrentPhrase,
		input.DecisionsAtCurrentPhrase, input.ConsecutiveHolds,
	)
}

// autopilotDecide runs the strict motion-only model contract. It never asks for
// or publishes a chat line.
func (s *Server) autopilotDecide(ctx context.Context, input modes.DecisionInput) (modes.Decision, error) {
	response, err := s.autopilotModelTurn(ctx, input, chat.AutopilotKindMotion)
	if err != nil {
		return modes.Decision{}, err
	}
	decision, err := s.mapAutopilotResponse(response, input)
	if err != nil {
		return modes.Decision{}, err
	}
	settings, _ := s.store.Snapshot()
	admitted, cosmetic := curateAutopilotPerceptualChange(decision, input, settings.Motion)
	if !cosmetic || input.MotionChangeLevel < 6 {
		return admitted, nil
	}
	retryInput := input
	retryInput.MotionFeedback = autopilotCosmeticFeedback(input)
	retryResponse, retryErr := s.autopilotModelTurn(ctx, retryInput, chat.AutopilotKindMotion)
	if retryErr != nil {
		return admitted, nil
	}
	retryDecision, retryErr := s.mapAutopilotResponse(retryResponse, retryInput)
	if retryErr != nil {
		return admitted, nil
	}
	retryAdmitted, _ := curateAutopilotPerceptualChange(retryDecision, retryInput, settings.Motion)
	return retryAdmitted, nil
}

// autopilotDecideSpeech runs the independent spoken-check-in contract. Its
// motion capability is reduced by the saved speech authority before prompt
// composition and again by the strict parser.
func (s *Server) autopilotDecideSpeech(ctx context.Context, input modes.DecisionInput) (modes.Decision, error) {
	response, err := s.autopilotModelTurn(ctx, input, chat.AutopilotKindSpeech)
	if err != nil {
		return modes.Decision{}, err
	}
	decision, err := s.mapAutopilotResponse(response, input)
	if err != nil {
		return modes.Decision{}, err
	}
	settings, _ := s.store.Snapshot()
	admitted, _ := curateAutopilotPerceptualChange(decision, input, settings.Motion)
	return admitted, nil
}

// curateAutopilotPerceptualChange turns cosmetic geometry updates into honest
// holds. It does not choose replacement geometry or require a change: the model
// may hold, but an update must compile to a felt shape difference. A simultaneous
// pace change remains valid while the ineffective geometry is stripped away.
func curateAutopilotPerceptualChange(
	decision modes.Decision,
	input modes.DecisionInput,
	settings config.MotionSettings,
) (modes.Decision, bool) {
	if decision.Hold || decision.Segment.Dynamic == nil || input.CurrentDynamic == nil ||
		input.CurrentPerceptual == nil {
		return decision, false
	}
	plan := motion.NewMotionPlan("autopilot-quality", motion.MotionTarget{
		Label: "Autopilot", Source: "autopilot",
		SpeedPercent: decision.Segment.SpeedPercent, Dynamic: decision.Segment.Dynamic,
	}, settings, 0, 0, time.Unix(0, 0))
	if plan.Perceptual.CommandedPeakVelocityPerSecond <= 0 ||
		plan.Perceptual.MateriallyDifferent(*input.CurrentPerceptual) {
		return decision, false
	}
	if decision.Segment.SpeedPercent != input.CurrentSpeed {
		current := motion.NormalizeDynamicDefinition(*input.CurrentDynamic)
		decision.Segment.Dynamic = &current
		return decision, true
	}
	return modes.Decision{
		Hold: true, Say: decision.Say, Next: decision.Next, Variability: decision.Variability,
	}, true
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
	if kind == chat.AutopilotKindMotion {
		patternChoices = withoutRecentPatterns(patternChoices, input.RecentPatternIDs)
	}
	provider, err := s.newLLMProvider(ctx, settings.LLM)
	if err != nil {
		return chat.AutopilotResponse{}, err
	}
	motionContext := s.chatMotionContext(settings.Motion, settings.LLM)
	// During engine recovery the scheduler can still own a valid current
	// segment while the transport-facing snapshot is temporarily idle. Keep the
	// autonomous validator aligned with that semantic state: a hold can restart
	// an owned segment, but it cannot start a brand-new Autopilot run.
	if input.CurrentSpeed > 0 && (input.CurrentDynamic != nil || input.CurrentPatternID != "") {
		motionContext.Running = true
		motionContext.Paused = false
		motionContext.SpeedPercent = input.CurrentSpeed
	}
	temperature := autopilotTemperature(kind, input.MotionChangeLevel)
	if kind == chat.AutopilotKindMotion && strings.TrimSpace(input.MotionFeedback) != "" && temperature < 0.98 {
		temperature = 0.98
	}
	service := chat.AutopilotService{
		Provider:              provider,
		Prompt:                prompt,
		Model:                 settings.LLM.Model,
		MaxTokens:             settings.LLM.MaxOutputTokens,
		ReasoningMode:         settings.LLM.ReasoningMode,
		ReasoningBudgetTokens: managedLlamaReasoningBudget(settings.LLM, s.managedLLM.Snapshot().Runtime.Current),
		Temperature:           temperature,
		Memories:              memories,
		Patterns:              patternChoices,
		MotionContext:         &motionContext,
		ConversationContext:   promptContext.ConversationContext,
		Capabilities:          capabilities,
	}
	modelContext := autopilotPromptContext(input, capabilities)
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

// autopilotTemperature turns Motion change rate into genuine sampling breadth
// as well as cadence. It is deliberately probabilistic: the model still authors
// the geometry, while a high rate is less likely to collapse onto its safest
// repeated numbers. Speech uses one independent moderate value so it can vary
// its language without inheriting the motion slider.
func autopilotTemperature(kind chat.AutopilotKind, level int) float64 {
	if kind != chat.AutopilotKindMotion {
		return 0.55
	}
	if level < 1 || level > 8 {
		level = 4
	}
	return 0.32 + float64(level-1)*0.07
}

// autopilotPromptContext is the single adapter from scheduler state to the
// bounded model-visible contract. Keeping it pure lets transport-free evals
// exercise the exact context the app sends rather than reconstructing it.
func autopilotPromptContext(input modes.DecisionInput, capabilities chat.Capabilities) chat.AutopilotContext {
	modelContext := chat.AutopilotContext{
		Style:             input.Style,
		SegmentIndex:      input.SegmentIndex,
		RecentPatternIDs:  input.RecentPatternIDs,
		SpeedMinPercent:   input.SpeedMinPercent,
		SpeedMaxPercent:   input.SpeedMaxPercent,
		LastSay:           input.LastSay,
		CurrentPatternID:  string(input.CurrentPatternID),
		CurrentSpeed:      input.CurrentSpeed,
		CurrentArea:       chatAreaZone(input.CurrentAreaFocus),
		AreaFocusEnabled:  capabilities.AreaFocus,
		MotionMode:        capabilities.MotionMode,
		MotionMinSeconds:  input.MotionMinSeconds,
		MotionMaxSeconds:  input.MotionMaxSeconds,
		MotionChangeLevel: input.MotionChangeLevel,
		// The manager already applied the tracking and arc switches when it built
		// the input, so a disabled switch arrives as false here and the prompt
		// omits the section entirely.
		SessionTracking:          input.SessionTracking,
		SessionSeconds:           input.SessionSeconds,
		SecondsAtCurrentSpeed:    input.SecondsAtCurrentSpeed,
		SecondsAtCurrentPhrase:   input.SecondsAtCurrentPhrase,
		DecisionsAtCurrentPhrase: input.DecisionsAtCurrentPhrase,
		ConsecutiveHolds:         input.ConsecutiveHolds,
		SpeedTrend:               input.SpeedTrend,
		ArcEnabled:               input.ArcEnabled,
		ArcPercent:               input.ArcPercent,
		MotionFeedback:           input.MotionFeedback,
	}
	if input.CurrentPerceptual != nil {
		modelContext.CommandedPositionMin = int(math.Round(input.CurrentPerceptual.PositionMinPercent))
		modelContext.CommandedPositionMax = int(math.Round(input.CurrentPerceptual.PositionMaxPercent))
		modelContext.CommandedMeanTravel = int(math.Round(input.CurrentPerceptual.CommandedMeanTravelPerSecond))
		modelContext.CommandedPeakSpeed = int(math.Round(input.CurrentPerceptual.CommandedPeakVelocityPerSecond))
		modelContext.MeanStrokeLength = int(math.Round(input.CurrentPerceptual.MeanStrokePercent))
		modelContext.LocalStrokeCV = int(math.Round(input.CurrentPerceptual.MinimumLocalStrokeCV * 100))
		modelContext.LocalStrokeRange = int(math.Round(input.CurrentPerceptual.MinimumLocalStrokeRange))
	}
	for _, band := range input.RecentPositionBands {
		modelContext.RecentPositionBands = append(modelContext.RecentPositionBands, chat.PositionBand{
			Minimum: int(math.Round(band.MinimumPercent)),
			Maximum: int(math.Round(band.MaximumPercent)),
		})
	}
	if input.CurrentDynamic != nil {
		dynamic := motion.NormalizeDynamicDefinition(*input.CurrentDynamic)
		modelContext.CurrentCenter = dynamic.CenterPercent
		modelContext.CurrentSpan = dynamic.SpanPercent
		modelContext.CurrentSpanMin = dynamic.SpanMinPercent
		modelContext.CurrentSpanProfile = dynamic.SpanProfile
		modelContext.CurrentVariation = dynamic.VariationPercent
		modelContext.CurrentSegment = dynamic.SegmentSeconds
		modelContext.CurrentSectionCount = len(dynamic.Sections)
		for _, anchor := range dynamic.Anchors {
			modelContext.CurrentAnchors = append(modelContext.CurrentAnchors, anchor.Name)
		}
	}
	return modelContext
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
	command := response.Motion
	if command == nil || command.Action == chat.MotionActionNone ||
		command.Action == chat.MotionActionStop || command.Action == chat.MotionActionStart {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
	}
	settings, _ := s.store.Snapshot()
	if settings.LLM.MotionGenerationMode == config.LLMMotionModeDynamic {
		return mapDynamicAutopilotCommand(command, input, say, next, variability), nil
	}

	patternID := strings.TrimSpace(command.PatternID)
	if patternID == "" {
		patternID = string(input.CurrentPatternID)
	}
	speed := input.CurrentSpeed
	if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	} else if command.Intensity != nil {
		speed = *command.Intensity
	}
	var areaFocus *motion.AreaFocus
	if input.CurrentAreaFocus != nil {
		focus := *input.CurrentAreaFocus
		areaFocus = &focus
	}
	if command.Area != "" {
		focus, ok := zoneAreaFocus(command.Area)
		if !ok {
			return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
		}
		areaFocus = focus
	}
	if patternID == "" || speed <= 0 {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
	}
	if strings.EqualFold(patternID, string(input.CurrentPatternID)) &&
		speed == input.CurrentSpeed && sameAreaFocus(areaFocus, input.CurrentAreaFocus) {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
	}

	resolved, found, err := s.patterns.ResolveEnabled(patternID)
	if err != nil {
		return modes.Decision{}, fmt.Errorf("resolve Autopilot pattern: %w", err)
	}
	if !found {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}, nil
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
	}, nil
}

func mapDynamicAutopilotCommand(
	command *chat.MotionCommand,
	input modes.DecisionInput,
	say string,
	next modes.TimingPreference,
	variability modes.VariabilityPreference,
) modes.Decision {
	dynamic := dynamicDefinitionFromCommand(command, input.CurrentDynamic)
	if input.CurrentDynamic != nil && len(command.Sections) == 0 &&
		commandChangesSingleDynamicPhrase(command) &&
		!sameDynamicPhraseSemantics(dynamic, *input.CurrentDynamic) {
		dynamic = motion.AdvanceDynamicPhraseSeed(dynamic, input.CurrentDynamic.PhraseSeed)
	}
	speed := input.CurrentSpeed
	if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	}
	if speed <= 0 {
		return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}
	}
	if input.CurrentDynamic != nil && speed == input.CurrentSpeed && len(command.Sections) == 0 &&
		sameDynamicPhraseSemantics(dynamic, *input.CurrentDynamic) {
		current := motion.NormalizeDynamicDefinition(*input.CurrentDynamic)
		if dynamic.SegmentSeconds == current.SegmentSeconds {
			return modes.Decision{Hold: true, Say: say, Next: next, Variability: variability}
		}
	}
	return modes.Decision{
		Segment: modes.Segment{SpeedPercent: speed, Dynamic: &dynamic, DurationMillis: int64(dynamic.SegmentSeconds) * 1000},
		Say:     say, Next: next, Variability: variability,
	}
}

func sameDynamicDefinition(left, right motion.DynamicDefinition) bool {
	left = motion.NormalizeDynamicDefinition(left)
	right = motion.NormalizeDynamicDefinition(right)
	return reflect.DeepEqual(left, right)
}

func sameDynamicPhraseSemantics(left, right motion.DynamicDefinition) bool {
	left = motion.NormalizeDynamicDefinition(left)
	right = motion.NormalizeDynamicDefinition(right)
	left.PhraseSeed = 0
	right.PhraseSeed = 0
	left.SegmentSeconds = 0
	right.SegmentSeconds = 0
	return reflect.DeepEqual(left, right)
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

// minAutopilotPatternChoices is how many patterns must remain on the menu before
// recent ones are withheld. A small enabled library would otherwise be narrowed
// to nothing, and a forced choice is worse than a repeated one.
const minAutopilotPatternChoices = 4

// withoutRecentPatterns temporarily removes patterns just played from the
// autonomous motion allow-list. The model can still deliberately hold the live
// pattern or change only its pace by omitting pattern_id; interactive chat keeps
// the complete enabled catalog so an explicit user request is never withheld.
//
// Prompting alone did not move this. Across four wordings, including replacing
// the recency list's "not a ban on deliberate reuse" with an explicit nudge, a
// live 26B held one pattern for an entire twenty-decision session with that same
// id sitting in the recent list four times over. Shaping the choice set is what
// the deterministic planner already does through recencyPenalty. It remains
// model-owned at the action boundary: action none is always valid, while a
// requested pattern change must use the current bounded menu.
func withoutRecentPatterns(choices []chat.PatternChoice, recent []string) []chat.PatternChoice {
	if len(choices) <= minAutopilotPatternChoices || len(recent) == 0 {
		return choices
	}
	withheld := make(map[string]bool, len(recent))
	for _, id := range recent {
		withheld[strings.ToLower(strings.TrimSpace(id))] = true
	}
	kept := make([]chat.PatternChoice, 0, len(choices))
	for _, choice := range choices {
		if withheld[strings.ToLower(strings.TrimSpace(choice.ID))] {
			continue
		}
		kept = append(kept, choice)
	}
	// Never narrow past the floor: fall back to the full menu rather than hand
	// the model too few options to make a sensible choice.
	if len(kept) < minAutopilotPatternChoices {
		return choices
	}
	return kept
}
