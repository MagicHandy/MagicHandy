package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const (
	maxUserMessageBytes = 4096
	maxHistoryMessages  = 12
	emptyRepairContext  = `{"_malformed":"empty_or_truncated_output"}`
)

// StreamEvent describes chat orchestration progress.
type StreamEvent struct {
	Type  string
	Phase string
	Text  string
	Error string
}

// Request is one local chat turn.
type Request struct {
	Message string
	History []llm.Message
}

// Result is the validated chat turn outcome.
type Result struct {
	Response         AssistantResponse
	Raw              string
	RepairRaw        string
	InitialMalformed bool
	Malformed        bool
	MalformedError   string
	Repaired         bool
	SemanticFallback bool
}

var (
	errMotionNoChange        = errors.New("motion target repeats the current content, speed, and area; change one allowed target field or use action none")
	errMotionPatternStale    = errors.New("explicit variation selected a recently used pattern; select a fresh enabled pattern")
	errMotionVariationAbsent = errors.New("explicit variation requires a different enabled pattern")
	errMotionSpeedBand       = errors.New("motion speed is outside the explicitly requested speed band")
)

// ValidateUserMessage normalizes one user turn before either persistence or
// model generation so both paths enforce the same byte limit.
func ValidateUserMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("chat message is required")
	}
	if len(message) > maxUserMessageBytes {
		return "", fmt.Errorf("chat message must be at most %d bytes", maxUserMessageBytes)
	}
	return message, nil
}

// Service runs chat prompts, strict validation, and repair over an LLM provider.
// Prompt is the resolved behavior profile; Memories are the enabled memory
// texts (empty when the memory switch is off; chat must work without them).
type Service struct {
	Provider              llm.Provider
	Prompt                PromptSet
	Model                 string
	MaxTokens             int
	ReasoningMode         string
	ReasoningBudgetTokens int
	Memories              []string
	Patterns              []PatternChoice
	// MotionContext is the authoritative semantic snapshot for this turn.
	// Nil is retained for non-interactive callers such as legacy tests.
	MotionContext *MotionContext
	// ConversationContext is backend-derived profile and continuity data. It
	// remains separate from request text and semantic motion validation.
	ConversationContext *ConversationContext
	// Capabilities gates which control methods the prompt advertises and the
	// result may carry. Nil preserves the historical full-capability behavior.
	Capabilities *Capabilities
	// TrustedMotionInput is reserved for backend-generated Autopilot decision
	// messages. Interactive user chat must leave this false.
	TrustedMotionInput bool
}

func (s Service) capabilities() Capabilities {
	if s.Capabilities == nil {
		return FullCapabilities()
	}
	return *s.Capabilities
}

// enforceCapabilities strips disallowed control fields from a validated
// response instead of failing the turn: the prompt never advertised them, so
// a stray field is model noise, not a contract violation worth a repair loop.
func enforceCapabilities(response *AssistantResponse, capabilities Capabilities) {
	if !capabilities.MoodTracking {
		response.NewMood = nil
	}
	if response.Motion == nil {
		return
	}
	if !capabilities.Motion {
		response.Motion = nil
		return
	}
	if !capabilities.AreaFocus {
		response.Motion.Area = ""
	}
	if !capabilities.Patterns && response.Motion.PatternID != "" {
		response.Motion.PatternID = ""
		if response.Motion.SpeedPercent == nil {
			response.Motion.SpeedPercent = response.Motion.Intensity
		}
		response.Motion.Intensity = nil
	}
}

// Complete streams a model response, repairs malformed JSON once, and returns a validated result.
func (s Service) Complete(ctx context.Context, request Request, emit func(StreamEvent) error) (Result, error) {
	if s.Provider == nil {
		return Result{}, errors.New("LLM provider is required")
	}
	userMessage, err := ValidateUserMessage(request.Message)
	if err != nil {
		return Result{}, err
	}

	prompt := s.Prompt
	if strings.TrimSpace(prompt.ID) == "" {
		prompt, _ = BuiltinPromptSetByID(DefaultPromptSetID)
	}
	capabilities := s.capabilities()
	systemPrompt := composeSystem(prompt, s.Memories, s.Patterns, capabilities, s.MotionContext, s.ConversationContext)

	messages := buildMessages(systemPrompt, request.History, userMessage)
	raw, err := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:              messages,
		Model:                 s.Model,
		Temperature:           0.2,
		MaxTokens:             s.MaxTokens,
		ReasoningMode:         s.ReasoningMode,
		ReasoningBudgetTokens: s.ReasoningBudgetTokens,
	}, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "delta", Phase: "initial", Text: text})
	})
	truncated := errors.Is(err, llm.ErrOutputTruncated)
	if err != nil && !truncated {
		return Result{}, err
	}

	response, parseErr := s.parseAndValidateResponse(raw, capabilities, userMessage)
	if parseErr == nil {
		return Result{Response: response, Raw: raw}, nil
	}
	if truncated {
		parseErr = fmt.Errorf("assistant response was truncated before valid JSON: %w", parseErr)
	}

	result := Result{
		Raw:              raw,
		InitialMalformed: true,
		Malformed:        true,
		MalformedError:   parseErr.Error(),
	}
	if err := emitEvent(emit, StreamEvent{Type: "malformed", Phase: "initial", Error: parseErr.Error()}); err != nil {
		return result, err
	}

	repairMessages := append([]llm.Message(nil), messages...)
	repairContext := strings.TrimSpace(raw)
	if repairContext == "" {
		repairContext = emptyRepairContext
	}
	repairMessages = append(repairMessages, llm.Message{Role: "assistant", Content: repairContext})
	repairMessages = append(repairMessages, llm.Message{Role: "user", Content: RepairPrompt(prompt, parseErr.Error())})
	repairRaw, repairErr := s.Provider.StreamChat(ctx, llm.ChatRequest{
		Messages:      repairMessages,
		Model:         s.Model,
		Temperature:   0,
		MaxTokens:     s.MaxTokens,
		ReasoningMode: "off",
	}, func(text string) error {
		return emitEvent(emit, StreamEvent{Type: "repair_delta", Phase: "repair", Text: text})
	})
	result.RepairRaw = repairRaw
	repairTruncated := errors.Is(repairErr, llm.ErrOutputTruncated)
	if repairErr != nil && !repairTruncated {
		result.MalformedError = repairErr.Error()
		return result, fmt.Errorf("repair assistant response: %w", repairErr)
	}

	repaired, repairParseErr := s.parseAndValidateResponse(repairRaw, capabilities, userMessage)
	if repairParseErr != nil {
		if fallback, ok := s.recoverSemanticRepair(repaired, capabilities, userMessage, repairParseErr); ok {
			result.Response = fallback
			result.Malformed = false
			result.Repaired = true
			result.SemanticFallback = true
			return result, nil
		}
		if repairTruncated {
			repairParseErr = fmt.Errorf("repaired response was truncated before valid JSON: %w", repairParseErr)
		}
		result.MalformedError = repairParseErr.Error()
		return result, nil
	}

	result.Response = repaired
	result.Malformed = false
	result.Repaired = true
	return result, nil
}

func (s Service) recoverSemanticRepair(response AssistantResponse, capabilities Capabilities, userMessage string, repairErr error) (AssistantResponse, bool) {
	if errors.Is(repairErr, errMotionNoChange) && !requestsPatternVariation(userMessage) && strings.TrimSpace(response.Reply) != "" {
		response.Motion = nil
		return response, true
	}
	if !requestsPatternVariation(userMessage) || !isSemanticMotionError(repairErr) {
		return AssistantResponse{}, false
	}
	if !s.TrustedMotionInput && !userAuthorizesMotion(userMessage, MotionActionTarget) {
		return AssistantResponse{}, false
	}
	return s.semanticPatternFallback(response, capabilities, userMessage)
}

func (s Service) parseAndValidateResponse(raw string, capabilities Capabilities, userMessage string) (AssistantResponse, error) {
	response, err := parseAssistantResponseForCapabilities(raw, s.Patterns, capabilities, s.MotionContext)
	if err != nil {
		return AssistantResponse{}, err
	}
	if response.Motion != nil && !s.TrustedMotionInput && !userAuthorizesMotion(userMessage, response.Motion.Action) {
		response.Motion = nil
		// An unauthorized model command is inert output. Return before semantic
		// variation repair can synthesize a replacement command.
		return response, nil
	}
	if response.Motion == nil && (!capabilities.Motion || (!s.TrustedMotionInput && !userAuthorizesMotion(userMessage, MotionActionTarget))) {
		return response, nil
	}
	if err := validateMotionChange(response, s.MotionContext, userMessage, s.Patterns); err != nil {
		return response, err
	}
	return response, nil
}

func userAuthorizesMotion(message, action string) bool {
	switch action {
	case MotionActionNone, MotionActionStop:
		return true
	case MotionActionStart, MotionActionTarget:
	default:
		return false
	}

	normalized := normalizeMotionIntent(message)
	if normalized == "" || motionIntentIsNegated(normalized) || motionIntentIsConversation(normalized) {
		return false
	}
	if action == MotionActionStart {
		return authorizesMotionStart(normalized)
	}
	return authorizesMotionTarget(normalized)
}

func normalizeMotionIntent(message string) string {
	message = strings.ToLower(strings.ReplaceAll(message, "’", "'"))
	return strings.Join(strings.FieldsFunc(message, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '\''
	}), " ")
}

func motionIntentIsNegated(message string) bool {
	if hasIntentPhrase(message,
		"no", "not", "never", "don't", "dont", "do not", "without",
		// Contracted negatives carry the same refusal as "do not" and must not
		// leave an authorizing verb ("start moving") exposed behind them.
		"didn't", "didnt", "doesn't", "doesnt", "won't", "wouldn't", "wouldnt",
		"shouldn't", "shouldnt", "can't", "cant", "cannot", "isn't", "isnt",
		"aren't", "arent", "stop", "stopped",
		"nunca", "sin", "evita", "evitar", "não", "nao", "sem",
		"tampoco", "pare", "para de", "deja de", "parar de",
	) {
		return true
	}
	return containsAny(message,
		"不要", "别", "別", "不想", "不能", "不再", "停止", "停下",
		"動かさない", "動かないで", "しないで", "ないで", "止め", "やめ",
	)
}

func motionIntentIsConversation(message string) bool {
	if motionIntentIsPermissionQuestion(message) {
		return true
	}
	if hasIntentPhrase(message,
		"tell me", "talk about", "think about", "explain", "describe", "story", "joke", "thought",
		"what is", "what are", "what does", "how does", "how do", "why is", "why does", "definition of",
		"why", "when", "where", "who",
		"wording", "reply", "speak", "say", "cuéntame", "explica", "historia", "chiste",
		"qué es", "que es", "cómo funciona", "como funciona",
		"conte me", "conta me", "explique", "história", "piada", "o que é", "o que e", "como funciona",
	) {
		return true
	}
	return containsAny(message, "告诉我", "解釋", "解释", "故事", "笑话", "笑話", "什么是", "什麼是", "教えて", "説明", "物語", "冗談", "について", "とは")
}

// motionIntentIsPermissionQuestion recognizes questions *about* moving — is it
// safe, what would happen, should I — which contain the same verbs as a real
// request. Asking whether motion is a good idea is not asking for it, and the
// safe failure here is a turn that only talks.
func motionIntentIsPermissionQuestion(message string) bool {
	if hasIntentPhrase(message,
		"is it safe", "is that safe", "is this safe", "how safe", "is it ok", "is it okay",
		"what happens", "what would happen", "what will happen",
		"should i", "should we", "may i", "do i need", "would it be", "is it a good idea",
		"es seguro", "es peligroso", "qué pasa si", "que pasa si", "debería", "deberia",
		"é seguro", "e seguro", "o que acontece", "devo", "deveria",
	) {
		return true
	}
	return containsAny(message, "安全ですか", "大丈夫ですか", "安全吗", "会怎么样", "应该吗")
}

func authorizesMotionStart(message string) bool {
	if containsAnyExact(message,
		"start", "begin", "move", "please start", "start please", "please begin", "begin please",
		"start slowly", "start gently", "please start slowly", "please start gently",
		"empieza", "comienza", "inicia", "por favor empieza", "por favor inicia",
		"começa", "comece", "inicia", "por favor comece", "por favor inicia",
		"开始", "请开始", "開始", "始めて",
	) {
		return true
	}
	if hasIntentPhrase(message, "start", "begin") && hasIntentPhrase(message,
		"move", "moving", "motion", "movement", "stroke", "strokes", "stroking", "device", "pattern", "pulse",
	) {
		return true
	}
	if hasIntentPhrase(message, "empieza", "comienza", "inicia", "iniciar", "empezar", "comenzar") &&
		hasIntentPhrase(message, "mover", "moviendo", "movimiento", "dispositivo", "patrón", "patron", "pulsar") {
		return true
	}
	if hasIntentPhrase(message, "começa", "comece", "inicia", "iniciar", "começar") &&
		hasIntentPhrase(message, "mover", "movendo", "movimento", "dispositivo", "padrão", "padrao", "pulsar") {
		return true
	}
	chineseStart := containsAny(message, "开始", "启动") && containsAny(message, "运动", "移动", "动起来", "模式")
	japaneseStart := containsAny(message, "始め", "開始") && containsAny(message, "動か", "動き", "モーション", "パターン")
	return chineseStart || japaneseStart
}

func authorizesMotionTarget(message string) bool {
	if containsAnyExact(message,
		"faster", "slower", "quicker", "gentler", "harder", "deeper", "shallower",
		"go faster", "go slower", "a little faster", "a little slower", "move faster", "move slower",
		"please go faster", "please go slower", "please go a little faster", "please go a little slower",
		"faster please", "slower please", "more gently",
		"más rápido", "más rápida", "más despacio", "más lento", "más lenta", "un poco más rápido", "un poco más despacio", "más rápido por favor", "más despacio por favor",
		"mais rápido", "mais rápida", "mais devagar", "mais lento", "mais lenta", "um pouco mais rápido", "um pouco mais devagar", "mais rápido por favor", "mais devagar por favor",
		"快一点", "慢一点", "请快一点", "请慢一点", "加快速度", "放慢速度",
		"もっと速く", "もっと遅く", "もっとゆっくり", "少し速く", "少し遅く", "ゆっくりして",
	) {
		return true
	}
	if hasIntentPrefix(message,
		"a little faster ", "a little slower ", "more gently ",
		"un poco más rápido ", "un poco más despacio ",
		"um pouco mais rápido ", "um pouco mais devagar ",
	) {
		return true
	}
	directive := motionIntentIsDirective(message)
	if hasIntentPhrase(message,
		"motion", "movement", "stroke", "strokes", "speed", "pace", "pattern", "rhythm", "intensity", "range",
		"movimiento", "velocidad", "ritmo", "patrón", "patron",
		"movimento", "velocidade", "ritmo", "padrão", "padrao",
	) && directive && hasIntentPhrase(message,
		"change", "switch", "different", "new", "another", "faster", "slower", "quicker", "gentler", "harder", "deeper", "shallower", "shorter", "longer", "focus", "vary", "mix",
		"cambia", "cambiar", "otro", "diferente", "rápido", "rápida", "despacio", "lento", "lenta", "profundo", "profunda", "corto", "corta", "largo", "larga", "enfoca",
		"muda", "mudar", "outro", "diferente", "rápido", "rápida", "devagar", "lento", "lenta", "profundo", "profunda", "curto", "curta", "longo", "longa", "foque",
	) {
		return true
	}
	if directive && hasIntentPhrase(message,
		"focus on the tip", "focus on the shaft", "focus on the base", "use the full range", "use full range",
		"enfócate en la punta", "enfócate en el eje", "enfócate en la base",
		"foque na ponta", "foque na haste", "foque na base",
	) {
		return true
	}
	chineseTarget := containsAny(message, "运动", "移动", "速度", "节奏", "模式", "尖端", "根部") &&
		containsAny(message, "加快", "减慢", "放慢", "改变", "更换", "聚焦", "快一点", "慢一点", "深入", "变浅", "缩短", "加长")
	japaneseTarget := containsAny(message, "動き", "モーション", "速度", "リズム", "パターン", "先端", "シャフト", "根元") &&
		containsAny(message, "速く", "遅く", "ゆっくり", "変え", "変更", "集中", "深く", "浅く", "短く", "長く")
	return chineseTarget || japaneseTarget || (directive && requestsPatternVariation(message))
}

func motionIntentIsDirective(message string) bool {
	if containsAnyExact(message,
		"mix it up", "mix it up again", "change it up", "change it up again",
		"change things up", "change things up again", "something different", "something else",
		"surprise me", "surprise me again", "switch it up", "switch it up again",
		"variation", "more variation", "add variety", "vary it",
	) {
		return true
	}
	return hasIntentPrefix(message,
		"please ", "change ", "switch ", "mix ", "use ", "try ", "give me ", "make it ", "go ", "move ", "focus ", "surprise me", "add ",
		"por favor ", "cambia ", "cambiar ", "usa ", "prueba ", "dame ", "enfócate ",
		"muda ", "mudar ", "use ", "tente ", "foque ",
	)
}

func hasIntentPrefix(message string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

func hasIntentPhrase(message string, phrases ...string) bool {
	padded := " " + message + " "
	for _, phrase := range phrases {
		if strings.Contains(padded, " "+phrase+" ") {
			return true
		}
	}
	return false
}

func validateMotionChange(response AssistantResponse, context *MotionContext, userMessage string, patterns []PatternChoice) error {
	if context == nil {
		return nil
	}
	command := response.Motion
	if err := validateRequestedPatternVariation(command, *context, userMessage, patterns); err != nil {
		return err
	}
	if command == nil {
		return nil
	}
	if err := validateRequestedSpeedBand(*command, *context, userMessage); err != nil {
		return err
	}
	if !context.Running || command.Action != MotionActionTarget {
		return nil
	}
	if motionTargetMatchesContext(*command, *context) {
		return errMotionNoChange
	}
	return validatePatternFreshness(*command, *context, userMessage, patterns)
}

func validateRequestedPatternVariation(command *MotionCommand, context MotionContext, userMessage string, patterns []PatternChoice) error {
	if !context.Running || !requestsPatternVariation(userMessage) || !hasAlternativePattern(patterns, context.PatternID) {
		return nil
	}
	if command == nil || command.Action != MotionActionTarget || command.PatternID == "" || strings.EqualFold(command.PatternID, context.PatternID) {
		return errMotionVariationAbsent
	}
	return nil
}

func motionTargetMatchesContext(command MotionCommand, context MotionContext) bool {
	sameContent := command.PatternID == "" || strings.EqualFold(command.PatternID, context.PatternID)
	if context.ProgramID != "" && command.PatternID != "" {
		sameContent = false
	}
	currentArea := strings.ToLower(strings.TrimSpace(context.Area))
	if currentArea == "" {
		currentArea = AreaZoneFull
	}
	sameArea := command.Area == "" || strings.EqualFold(command.Area, currentArea)
	return sameContent && motionSpeedMatchesContext(command, context.SpeedPercent) && sameArea
}

func motionSpeedMatchesContext(command MotionCommand, currentSpeed int) bool {
	if command.Intensity != nil {
		return *command.Intensity == currentSpeed
	}
	if command.SpeedPercent != nil {
		return *command.SpeedPercent == currentSpeed
	}
	return true
}

func validatePatternFreshness(command MotionCommand, context MotionContext, userMessage string, patterns []PatternChoice) error {
	if requestsPatternVariation(userMessage) && command.PatternID != "" &&
		isRecentPattern(command.PatternID, context.RecentPatternIDs) &&
		hasFreshPattern(patterns, context.PatternID, context.RecentPatternIDs) {
		return errMotionPatternStale
	}
	return nil
}

func validateRequestedSpeedBand(command MotionCommand, context MotionContext, userMessage string) error {
	if command.Action != MotionActionStart && command.Action != MotionActionTarget {
		return nil
	}
	label, band, ok := requestedSpeedBand(context, userMessage)
	if !ok {
		return nil
	}
	speed := 0
	if command.Intensity != nil {
		speed = *command.Intensity
	} else if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	} else if context.Running {
		speed = context.SpeedPercent
	}
	if speed < band[0] || speed > band[1] {
		return fmt.Errorf("%w: requested %s speed must be within the supplied %d-%d band", errMotionSpeedBand, label, band[0], band[1])
	}
	return nil
}

func requestedSpeedBand(context MotionContext, message string) (string, [2]int, bool) {
	message = strings.ToLower(strings.TrimSpace(message))
	low := containsAny(message, "gentle", "gently", "slow pace", "slowly", "low speed")
	middle := containsAny(message, "medium pace", "medium speed", "moderate", "moderately")
	high := containsAny(message, "as fast as", "fastest", "full speed", "max speed", "maximum speed")
	if countTrue(low, middle, high) != 1 {
		return "", [2]int{}, false
	}
	bands := normalizedPromptMotionContext(context).SpeedBands
	switch {
	case low:
		return "low", bands.Low, true
	case middle:
		return "middle", bands.Middle, true
	default:
		return "high", bands.High, true
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func countTrue(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func isSemanticMotionError(err error) bool {
	return errors.Is(err, errMotionNoChange) || errors.Is(err, errMotionPatternStale) ||
		errors.Is(err, errMotionVariationAbsent) || errors.Is(err, errMotionSpeedBand)
}

func requestsPatternVariation(message string) bool {
	message = normalizeMotionIntent(message)
	if motionIntentIsNegated(message) || motionIntentIsConversation(message) {
		return false
	}
	if containsAny(message, "do not change the pattern", "don't change the pattern", "keep the same pattern", "keep this pattern", "same pattern") {
		return false
	}
	if containsAny(message,
		"change the feel", "different motion", "different movement", "different pattern",
		"fresh pattern", "new motion", "new movement", "new pattern", "another pattern",
		"switch motion", "switch pattern", "motion variation", "pattern variation",
	) {
		return true
	}
	variationPhrase := containsAny(message,
		"change it up", "change things up", "mix it up", "mix things up", "something different",
		"something else", "surprise me", "switch it up", "variation", "variety", "vary it",
	)
	if !variationPhrase {
		return false
	}
	if containsAny(message,
		"motion", "movement", "pattern", "stroke", "rhythm", "feel", "pace", "speed",
		"faster", "slower", "focus", "full range", "tip", "middle", "base",
	) {
		return true
	}
	standalone := strings.Trim(message, " .,!?:;\t\r\n")
	return containsAnyExact(standalone,
		"change it up", "change it up again", "change things up", "change things up again",
		"mix it up", "mix it up again", "mix things up", "mix things up again",
		"something different", "something else", "surprise me", "surprise me again",
		"switch it up", "switch it up again", "variation", "more variation", "add variety", "vary it",
	)
}

func containsAnyExact(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func isRecentPattern(patternID string, recentPatternIDs []string) bool {
	for _, recentID := range recentPatternIDs {
		if strings.EqualFold(strings.TrimSpace(patternID), strings.TrimSpace(recentID)) {
			return true
		}
	}
	return false
}

func hasFreshPattern(patterns []PatternChoice, currentID string, recentPatternIDs []string) bool {
	for _, pattern := range patterns {
		patternID := strings.TrimSpace(pattern.ID)
		if patternID != "" && !strings.EqualFold(patternID, currentID) && !isRecentPattern(patternID, recentPatternIDs) {
			return true
		}
	}
	return false
}

func hasAlternativePattern(patterns []PatternChoice, currentID string) bool {
	for _, pattern := range patterns {
		patternID := strings.TrimSpace(pattern.ID)
		if patternID != "" && !strings.EqualFold(patternID, currentID) {
			return true
		}
	}
	return false
}

func (s Service) semanticPatternFallback(response AssistantResponse, capabilities Capabilities, userMessage string) (AssistantResponse, bool) {
	if !capabilities.Motion || !capabilities.Patterns || s.MotionContext == nil || !s.MotionContext.Running || len(s.Patterns) < 2 {
		return AssistantResponse{}, false
	}
	patternID, ok := selectFallbackPattern(s.Patterns, s.MotionContext.PatternID, s.MotionContext.RecentPatternIDs)
	if !ok {
		return AssistantResponse{}, false
	}
	command, ok := buildFallbackPatternCommand(patternID, response.Motion, *s.MotionContext, userMessage)
	if !ok {
		return AssistantResponse{}, false
	}
	response.Motion = &command
	return response, true
}

func selectFallbackPattern(patterns []PatternChoice, currentID string, recentPatternIDs []string) (string, bool) {
	currentID = strings.TrimSpace(currentID)
	start := 0
	for index, pattern := range patterns {
		if strings.EqualFold(strings.TrimSpace(pattern.ID), currentID) {
			start = (index + 1) % len(patterns)
			break
		}
	}
	recent := make(map[string]bool, len(recentPatternIDs))
	for _, patternID := range recentPatternIDs {
		recent[strings.ToLower(strings.TrimSpace(patternID))] = true
	}
	for pass := range 2 {
		for offset := range len(patterns) {
			patternID := strings.TrimSpace(patterns[(start+offset)%len(patterns)].ID)
			normalizedID := strings.ToLower(patternID)
			if normalizedID == "" || strings.EqualFold(normalizedID, currentID) || (pass == 0 && recent[normalizedID]) {
				continue
			}
			return patternID, true
		}
	}
	return "", false
}

func buildFallbackPatternCommand(patternID string, repaired *MotionCommand, context MotionContext, userMessage string) (MotionCommand, bool) {
	command := MotionCommand{Action: MotionActionTarget, PatternID: patternID}
	if repaired != nil {
		command.Area = repaired.Area
		command.Intensity = cloneInt(repaired.Intensity)
		command.SpeedPercent = cloneInt(repaired.SpeedPercent)
	}
	if validateRequestedSpeedBand(command, context, userMessage) != nil {
		command.Intensity = nil
		command.SpeedPercent = nil
	}
	if command.Intensity == nil && command.SpeedPercent == nil {
		speed := context.SpeedPercent
		if _, band, requested := requestedSpeedBand(context, userMessage); requested && (speed < band[0] || speed > band[1]) {
			speed = band[0] + (band[1]-band[0])/2
		}
		if speed < 1 || speed > 100 {
			return MotionCommand{}, false
		}
		command.SpeedPercent = &speed
	}
	return command, true
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func buildMessages(systemPrompt string, history []llm.Message, userMessage string) []llm.Message {
	messages := []llm.Message{{Role: "system", Content: systemPrompt}}
	messages = append(messages, sanitizeHistory(history)...)
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})
	return messages
}

func sanitizeHistory(history []llm.Message) []llm.Message {
	if len(history) > maxHistoryMessages {
		history = history[len(history)-maxHistoryMessages:]
	}
	messages := make([]llm.Message, 0, len(history))
	for _, message := range history {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len(content) > maxUserMessageBytes {
			content = truncateUTF8Bytes(content, maxUserMessageBytes)
		}
		if role == "assistant" {
			content = assistantHistoryContent(content)
		}
		messages = append(messages, llm.Message{Role: role, Content: content})
	}
	return messages
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && end < len(value) && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func assistantHistoryContent(content string) string {
	var candidate AssistantResponse
	_ = json.Unmarshal([]byte(content), &candidate)
	choices := defaultPatternChoices()
	if candidate.Motion != nil && strings.TrimSpace(candidate.Motion.PatternID) != "" {
		choices = append(choices, PatternChoice{ID: candidate.Motion.PatternID})
	}
	if _, err := parseAssistantResponse(content, choices, false, nil); err == nil {
		return content
	}
	response := AssistantResponse{
		Reply: content,
		Motion: &MotionCommand{
			Action: MotionActionNone,
		},
	}
	data, err := json.Marshal(response)
	if err != nil {
		return `{"reply":"Previous assistant reply omitted.","motion":{"action":"none"}}`
	}
	return string(data)
}

func emitEvent(emit func(StreamEvent) error, event StreamEvent) error {
	if emit == nil {
		return nil
	}
	return emit(event)
}
