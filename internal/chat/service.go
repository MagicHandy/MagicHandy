package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

const (
	chatTemperature   = 0.3
	chatTopP          = 0.95
	chatRepeatPenalty = 1.2
	chatRepeatLastN   = 40
)

var (
	errMotionNoChange        = errors.New("motion target repeats the current content, speed, and area; change one allowed target field or use action none")
	errMotionVariationAbsent = errors.New("explicit variation requires a meaningful change to content, speed, or area")
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
	if capabilities.MotionMode == MotionModeDynamic {
		response.Motion.PatternID = ""
		response.Motion.Area = ""
	} else {
		clearDynamicMotionFields(response.Motion)
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

func clearDynamicMotionFields(command *MotionCommand) {
	command.CenterPercent = nil
	command.SpanPercent = nil
	command.SpanMinPercent = nil
	command.SpanProfile = ""
	command.Anchors = nil
	command.VariationPercent = nil
	command.SegmentSeconds = nil
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
		Temperature:           chatTemperature,
		TopP:                  chatTopP,
		RepeatPenalty:         chatRepeatPenalty,
		RepeatLastN:           chatRepeatLastN,
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
		// A response cut off at the output cap cannot be repaired by asking for
		// the corrected object under that same cap: the correction is longer
		// than the text that already did not fit, so it truncates again. That
		// cost a second full generation and produced a second failure, which on
		// a slow model is where the request reset. The prose the user asked for
		// is already sitting in the partial JSON, so recover it instead. No
		// motion is taken from a partial object.
		if reply := salvageTruncatedReply(raw); reply != "" {
			return Result{
				Response:         AssistantResponse{Reply: reply},
				Raw:              raw,
				InitialMalformed: true,
				Repaired:         true,
			}, nil
		}
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
	return s.repairResponse(ctx, repairInput{
		result:       result,
		messages:     messages,
		prompt:       prompt,
		raw:          raw,
		parseErr:     parseErr,
		truncated:    truncated,
		userMessage:  userMessage,
		capabilities: capabilities,
	}, emit)
}

// repairInput carries one malformed turn into the second attempt. It exists so
// the repair phase reads as its own step rather than as the tail of Complete.
type repairInput struct {
	result       Result
	messages     []llm.Message
	prompt       PromptSet
	raw          string
	parseErr     error
	truncated    bool
	userMessage  string
	capabilities Capabilities
}

// repairResponse asks the model to replace one malformed response, then applies
// the same validation and the semantic fallback to whatever comes back.
func (s Service) repairResponse(ctx context.Context, in repairInput, emit func(StreamEvent) error) (Result, error) {
	result := in.result
	repairMessages := append([]llm.Message(nil), in.messages...)
	repairContext := strings.TrimSpace(in.raw)
	if repairContext == "" {
		repairContext = emptyRepairContext
	}
	repairMessages = append(repairMessages, llm.Message{Role: "assistant", Content: repairContext})
	repairMessages = append(repairMessages, llm.Message{Role: "user", Content: repairPromptFor(in.prompt, in.parseErr.Error(), in.truncated)})
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

	repaired, repairParseErr := s.parseAndValidateResponse(repairRaw, in.capabilities, in.userMessage)
	if repairParseErr != nil {
		if fallback, ok := s.recoverSemanticRepair(repaired, in.userMessage, repairParseErr); ok {
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

// salvageTruncatedReply pulls the reply text out of a JSON object that was cut
// off mid-generation. It decodes the "reply" string manually because the object
// has no closing brace and often no closing quote, so encoding/json cannot help.
//
// It reads only the top-level "reply" key and stops at the first unescaped
// quote, so a truncated object cannot smuggle in a motion command: the caller
// discards everything else.
func salvageTruncatedReply(raw string) string {
	position := skipJSONSpace(raw, 0)
	if position >= len(raw) || raw[position] != '{' {
		return ""
	}
	position++

	for {
		position = skipJSONSpace(raw, position)
		if position >= len(raw) || raw[position] == '}' {
			return ""
		}

		decoder := json.NewDecoder(strings.NewReader(raw[position:]))
		var key string
		if err := decoder.Decode(&key); err != nil {
			return ""
		}
		position += int(decoder.InputOffset())
		position = skipJSONSpace(raw, position)
		if position >= len(raw) || raw[position] != ':' {
			return ""
		}
		position = skipJSONSpace(raw, position+1)
		if key == "reply" {
			if position >= len(raw) || raw[position] != '"' {
				return ""
			}
			return decodePartialJSONString(raw[position+1:])
		}

		// A later key can only be top-level when this value is complete. Let
		// encoding/json skip strings, arrays, and objects without confusing a
		// nested or quoted "reply" token for the field we want.
		decoder = json.NewDecoder(strings.NewReader(raw[position:]))
		var ignored json.RawMessage
		if err := decoder.Decode(&ignored); err != nil {
			return ""
		}
		position += int(decoder.InputOffset())
		position = skipJSONSpace(raw, position)
		if position >= len(raw) || raw[position] != ',' {
			return ""
		}
		position++
	}
}

func skipJSONSpace(raw string, position int) int {
	for position < len(raw) {
		switch raw[position] {
		case ' ', '\t', '\r', '\n':
			position++
		default:
			return position
		}
	}
	return position
}

func decodePartialJSONString(rest string) string {

	var out strings.Builder
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '\\':
			if i+1 >= len(rest) {
				// Trailing backslash: the escape itself was cut in half.
				return strings.TrimSpace(out.String())
			}
			// Decode escapes through encoding/json rather than maintaining a
			// second mapping. Try a full UTF-16 surrogate pair before a single
			// \uXXXX escape so non-BMP characters survive intact.
			var decoded string
			if rest[i+1] == 'u' && i+5 < len(rest) {
				unit, ok := parseJSONUTF16Unit(rest[i:])
				if !ok {
					return strings.TrimSpace(out.String())
				}
				if unit >= 0xd800 && unit <= 0xdbff {
					low, pairOK := parseJSONUTF16Unit(rest[i+6:])
					if i+11 < len(rest) && pairOK && low >= 0xdc00 && low <= 0xdfff {
						if err := json.Unmarshal([]byte(`"`+rest[i:i+12]+`"`), &decoded); err == nil {
							out.WriteString(decoded)
							i += 11
							continue
						}
					}
					return strings.TrimSpace(out.String())
				}
				if unit >= 0xdc00 && unit <= 0xdfff {
					return strings.TrimSpace(out.String())
				}
				if err := json.Unmarshal([]byte(`"`+rest[i:i+6]+`"`), &decoded); err == nil {
					out.WriteString(decoded)
					i += 5
					continue
				}
			}
			if err := json.Unmarshal([]byte(`"\`+string(rest[i+1])+`"`), &decoded); err == nil {
				out.WriteString(decoded)
				i++
				continue
			}
			return strings.TrimSpace(out.String())
		case '"':
			return strings.TrimSpace(out.String())
		default:
			out.WriteByte(rest[i])
		}
	}
	// Ran off the end: the string itself was truncated, which is the common case.
	return strings.TrimSpace(out.String())
}

func parseJSONUTF16Unit(raw string) (uint16, bool) {
	if len(raw) < 6 || raw[0] != '\\' || raw[1] != 'u' {
		return 0, false
	}
	value, err := strconv.ParseUint(raw[2:6], 16, 16)
	return uint16(value), err == nil
}

func (s Service) recoverSemanticRepair(response AssistantResponse, userMessage string, repairErr error) (AssistantResponse, bool) {
	if errors.Is(repairErr, errMotionNoChange) && !requestsMotionVariation(userMessage) && strings.TrimSpace(response.Reply) != "" {
		response.Motion = nil
		return response, true
	}
	return AssistantResponse{}, false
}

func (s Service) parseAndValidateResponse(raw string, capabilities Capabilities, userMessage string) (AssistantResponse, error) {
	response, err := parseAssistantResponseForCapabilities(raw, s.Patterns, capabilities, s.MotionContext)
	if err != nil {
		return AssistantResponse{}, err
	}
	if response.Motion != nil && !s.TrustedMotionInput &&
		!userAuthorizesMotionCommandForCapabilities(userMessage, *response.Motion, capabilities, s.MotionContext) {
		response.Motion = nil
		// An unauthorized model command is inert output. Return before semantic
		// variation repair can synthesize a replacement command.
		return response, nil
	}
	if response.Motion == nil && (!capabilities.Motion || (!s.TrustedMotionInput &&
		!userAuthorizesMotionForCapabilities(userMessage, MotionActionTarget, capabilities, s.MotionContext))) {
		return response, nil
	}
	if capabilities.MotionMode == MotionModeDynamic {
		if response.Motion != nil && s.MotionContext != nil {
			return response, validateRequestedSpeedBand(*response.Motion, *s.MotionContext, userMessage)
		}
		return response, nil
	}
	if err := validateMotionChange(response, s.MotionContext, userMessage); err != nil {
		return response, err
	}
	return response, nil
}

func userAuthorizesMotionCommandForCapabilities(message string, command MotionCommand, capabilities Capabilities, context *MotionContext) bool {
	if capabilities.MotionMode == MotionModeDynamic && command.Action == MotionActionUpdate &&
		context != nil && context.Running {
		return userAuthorizesDynamicUpdate(message, command, *context)
	}
	return userAuthorizesMotionForCapabilities(message, command.Action, capabilities, context)
}

func userAuthorizesMotionForCapabilities(message, action string, capabilities Capabilities, context *MotionContext) bool {
	if capabilities.MotionMode == MotionModeDynamic && action == MotionActionUpdate && context != nil && context.Running {
		normalized := normalizeMotionIntent(message)
		return normalized != "" && !motionIntentIsNegated(normalized)
	}
	return userAuthorizesMotion(message, action)
}

// userAuthorizesDynamicUpdate keeps the active Creative model's freedom to
// make a semantic update while respecting the scope of a user's negative
// qualifier. "Do not change the pace" preserves one axis; it does not cancel a
// simultaneous request to change stroke length. Unscoped refusals remain inert.
func userAuthorizesDynamicUpdate(message string, command MotionCommand, context MotionContext) bool {
	message = normalizeMotionIntent(message)
	if message == "" {
		return false
	}
	if !motionIntentIsNegated(message) {
		return true
	}

	changesSpeed := command.SpeedPercent != nil && *command.SpeedPercent != context.SpeedPercent
	changesRange := dynamicCommandChangesRange(command)
	changesTexture := command.VariationPercent != nil
	negatesSpeed := negatesDynamicSpeedChange(message)
	negatesRange := negatesDynamicRangeChange(message)
	if !negatesSpeed && !negatesRange {
		// Only a recognized semantic-axis qualifier narrows the ordinary
		// negation gate. A whole-motion refusal remains inert even if the model
		// also emitted a plausible target.
		return false
	}

	if changesSpeed && negatesSpeed {
		return false
	}
	if changesRange && negatesRange {
		// Negating range variation is itself an explicit request to clear an
		// envelope. Negating a range change means preserve it exactly.
		if command.SpanProfile != DynamicSpanProfileSteady ||
			!negatesAxisMutation(message, []string{"range", "the range", "stroke length", "the stroke length"}, "vary") {
			return false
		}
	}

	requestedAxis := (changesSpeed && requestsDynamicSpeedChange(message)) ||
		(changesRange && requestsDynamicRangeChange(message)) ||
		(changesTexture && requestsDynamicTextureChange(message))
	return requestedAxis
}

func dynamicCommandChangesRange(command MotionCommand) bool {
	return command.CenterPercent != nil || command.SpanPercent != nil ||
		command.SpanMinPercent != nil || command.SpanProfile != "" || len(command.Anchors) > 0
}

func negatesDynamicSpeedChange(message string) bool {
	return negatesAxisMutation(message, []string{"pace", "the pace", "speed", "the speed"}, "change")
}

func negatesDynamicRangeChange(message string) bool {
	return negatesAxisMutation(
		message,
		[]string{"range", "the range", "stroke length", "the stroke length"},
		"change", "vary",
	)
}

func negatesAxisMutation(message string, axes []string, verbs ...string) bool {
	for _, axis := range axes {
		for _, verb := range verbs {
			continuous := verb + "ing"
			switch verb {
			case "change":
				continuous = "changing"
			case "vary":
				continuous = "varying"
			}
			if hasIntentPhrase(message,
				"do not "+verb+" "+axis,
				"don't "+verb+" "+axis,
				"dont "+verb+" "+axis,
				"without "+continuous+" "+axis,
				"stop "+continuous+" "+axis,
				"no "+axis+" "+verb,
			) {
				return true
			}
		}
	}
	return false
}

func requestsDynamicSpeedChange(message string) bool {
	return hasIntentPhrase(message,
		"faster", "slower", "quicker", "gentler", "harder", "speed up", "slow down",
		"increase speed", "decrease speed", "change pace", "change speed",
	)
}

func requestsDynamicRangeChange(message string) bool {
	rangeLanguage := hasIntentPhrase(message,
		"range", "stroke length", "stroke lengths", "tight", "broad", "short", "deep",
		"shallow", "base", "middle", "tip", "anchor", "anchors",
	)
	changeLanguage := hasIntentPhrase(message,
		"change", "mix", "vary", "breathe", "wander", "contrast", "steady", "tight", "broad",
		"short", "deep", "shallow", "use", "make", "keep", "let", "focus",
	)
	return rangeLanguage && changeLanguage
}

func requestsDynamicTextureChange(message string) bool {
	return hasIntentPhrase(message,
		"variation", "vary", "rhythm", "timing", "center drift", "more organic", "less robotic",
	)
}

func userAuthorizesMotion(message, action string) bool {
	switch action {
	case MotionActionNone, MotionActionStop:
		return true
	case MotionActionStart, MotionActionTarget, MotionActionUpdate:
	default:
		return false
	}

	normalized := normalizeMotionIntent(message)
	if normalized == "" || motionIntentIsNegated(normalized) {
		return false
	}
	if authorizesDirectPartnerMotion(normalized) {
		return true
	}
	if motionIntentIsConversation(normalized) {
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
		"不要", "请勿", "請勿", "勿", "莫", "别", "別", "不想", "不能", "不再",
		"不开始", "不開始", "不启动", "不啟動", "停止", "停下", "禁止",
		"動かさない", "動かないで", "動かすな", "動くな", "しないで", "するな",
		"始めるな", "開始するな", "ないで", "止め", "やめ", "禁止",
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
	if hasIntentPhrase(message, "should", "should the", "should you") &&
		hasIntentPhrase(message, "move", "moving", "motion", "movement", "device", "pattern") {
		return true
	}
	if containsAny(message, "安全ですか", "大丈夫ですか", "安全吗", "会怎么样") {
		return true
	}
	chineseMotion := containsAny(message, "运动", "運動", "移动", "移動", "开始", "開始", "启动", "啟動", "速度", "模式")
	chineseQuestion := containsAny(message, "应该", "應該", "应不应该", "應不應該", "是否", "该不该", "該不該", "要不要") ||
		(containsAny(message, "可以", "能") && containsAny(message, "吗", "嗎"))
	if chineseMotion && chineseQuestion {
		return true
	}
	japaneseMotion := containsAny(message, "動き", "動か", "始め", "開始", "モーション", "速度", "パターン")
	japaneseQuestion := containsAny(message, "べき", "てもいい", "ほうがいい", "安全", "大丈夫", "どうなる")
	return japaneseMotion && japaneseQuestion
}

// authorizesDirectPartnerMotion grants a model-authored command permission to
// pass the current-turn gate. It does not decide or synthesize motion.
func authorizesDirectPartnerMotion(message string) bool {
	for _, prefix := range []string{
		"please ", "can you ", "could you ", "would you ", "will you ",
		"i want you to ", "i need you to ", "go ahead and ",
	} {
		if strings.HasPrefix(message, prefix) {
			message = strings.TrimSpace(strings.TrimPrefix(message, prefix))
			break
		}
	}
	message = strings.TrimSpace(strings.TrimSuffix(message, " please"))
	for _, command := range []string{
		"fuck me", "stroke me", "jerk me off", "ride me",
		"suck me", "suck it", "kiss it", "lick me", "lick it",
	} {
		if message == command {
			return true
		}
		if !strings.HasPrefix(message, command+" ") {
			continue
		}
		qualifier := strings.TrimSpace(strings.TrimPrefix(message, command))
		if containsAnyExact(qualifier,
			"now", "right now",
			"slow", "slowly", "gently", "nice and slow", "slow and gentle", "slow and deep",
			"hard", "harder", "fast", "faster", "deep", "deeper", "hard and fast", "hard and deep",
			"as hard as you can", "as fast as you can", "however you want", "like you mean it", "with it",
		) {
			return true
		}
		if hasIntentPrefix(qualifier,
			"and talk ", "and tell ", "while you ", "until i ", "until i'm ",
			"with the ", "like you ", "however you ",
		) {
			return true
		}
	}
	return false
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
	if authorizesAreaFocus(message) {
		return true
	}
	chineseTarget := containsAny(message, "运动", "移动", "速度", "节奏", "模式", "尖端", "根部") &&
		containsAny(message, "加快", "减慢", "放慢", "改变", "更换", "聚焦", "快一点", "慢一点", "深入", "变浅", "缩短", "加长")
	japaneseTarget := containsAny(message, "動き", "モーション", "速度", "リズム", "パターン", "先端", "シャフト", "根元") &&
		containsAny(message, "速く", "遅く", "ゆっくり", "変え", "変更", "集中", "深く", "浅く", "短く", "長く")
	return chineseTarget || japaneseTarget || (directive && requestsMotionVariation(message))
}

// authorizesAreaFocus recognizes a request to move motion to part of the
// stroke. It deliberately does not require the directive prefix the other
// target branches do: "just the tip" and "stay near the top" are unambiguous
// requests, and this branch can only ever re-aim motion that is already
// running. Refusing one of these silently ignores the user, while accepting a
// false one moves an existing stroke — so the naming plus a placement word is
// enough evidence here. Starting motion still needs authorizesMotionStart.
func authorizesAreaFocus(message string) bool {
	return namesMotionZone(message) && placesMotion(message)
}

func namesMotionZone(message string) bool {
	if hasIntentPhrase(message,
		"tip", "head", "top", "bottom", "base", "shaft", "middle", "mid",
		"upper", "lower", "shallow", "shallowly", "deep", "deeply",
		"full range", "whole range", "entire range", "whole stroke", "full stroke", "full strokes",
		"punta", "cabeza", "eje", "medio", "arriba", "abajo", "profundo", "rango completo",
		"ponta", "cabeça", "cabeca", "haste", "meio", "cima", "baixo", "fundo", "alcance total",
	) {
		return true
	}
	return containsAny(message, "尖端", "根部", "中间", "全程", "先端", "根元", "シャフト", "全体")
}

func placesMotion(message string) bool {
	if hasIntentPhrase(message,
		"focus", "focused", "focusing", "concentrate", "concentrating", "center", "centre",
		"stay", "stays", "staying", "keep", "keeping", "work", "working", "stick",
		"just", "only", "near", "around", "toward", "towards", "back to", "return to",
		"use", "switch to", "move to", "go to", "more", "all",
		"enfócate", "enfocate", "enfoca", "concéntrate", "concentrate", "quédate", "quedate",
		"solo", "sólo", "cerca", "vuelve", "usa",
		"foque", "concentre", "fique", "só", "so", "apenas", "perto", "volte", "use",
	) {
		return true
	}
	return containsAny(message, "集中", "聚焦", "回到", "だけ", "付近", "戻し", "戻して")
}

func motionIntentIsDirective(message string) bool {
	if containsAnyExact(message,
		"mix it up", "mix it up again", "change it up", "change it up again",
		"change things up", "change things up again", "keep changing it up", "keep mixing it up",
		"keep switching it up", "keep varying it", "something different", "something else",
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

func validateMotionChange(response AssistantResponse, context *MotionContext, userMessage string) error {
	if context == nil {
		return nil
	}
	command := response.Motion
	variationRequested := context.Running && requestsMotionVariation(userMessage)
	if command == nil {
		if variationRequested {
			return errMotionVariationAbsent
		}
		return nil
	}
	if variationRequested && command.Action == MotionActionNone {
		return errMotionVariationAbsent
	}
	if err := validateRequestedSpeedBand(*command, *context, userMessage); err != nil {
		return err
	}
	if !context.Running || (command.Action != MotionActionTarget && command.Action != MotionActionUpdate && command.Action != MotionActionStart) {
		return nil
	}
	if !motionTargetMatchesContext(*command, *context) {
		return nil
	}
	if variationRequested {
		return errMotionVariationAbsent
	}
	return errMotionNoChange
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
	if command.SpeedPercent != nil {
		return *command.SpeedPercent == currentSpeed
	}
	if command.Intensity != nil {
		return *command.Intensity == currentSpeed
	}
	return true
}

func validateRequestedSpeedBand(command MotionCommand, context MotionContext, userMessage string) error {
	if command.Action != MotionActionStart && command.Action != MotionActionTarget && command.Action != MotionActionUpdate {
		return nil
	}
	label, band, ok := requestedSpeedBand(context, userMessage)
	if !ok {
		return nil
	}
	speed := 0
	if command.SpeedPercent != nil {
		speed = *command.SpeedPercent
	} else if command.Intensity != nil {
		speed = *command.Intensity
	} else if context.Running {
		speed = context.SpeedPercent
	}
	if speed < band[0] || speed > band[1] {
		return fmt.Errorf("%w: requested %s speed must be within the supplied %d-%d band", errMotionSpeedBand, label, band[0], band[1])
	}
	return nil
}

func requestedSpeedBand(context MotionContext, message string) (string, [2]int, bool) {
	message = normalizeMotionIntent(message)
	if hasIntentPhrase(message,
		"faster", "slower", "harder", "gentler", "a little faster", "a little slower",
		"más rápido", "más rápida", "más despacio", "más lento", "más lenta",
		"mais rápido", "mais rápida", "mais devagar", "mais lento", "mais lenta",
	) || containsAny(message, "快一点", "慢一点", "もっと速く", "もっと遅く", "もっとゆっくり") {
		return "", [2]int{}, false
	}
	low := hasIntentPhrase(message,
		"gentle", "gently", "slow", "slow pace", "low speed",
		"despacio", "lentamente", "suave", "suavemente", "ritmo lento", "velocidad baja",
		"devagar", "ritmo lento", "velocidade baixa",
	) || (hasIntentPhrase(message, "slowly") && !requestsDynamicRangeChange(message)) ||
		containsAny(message, "慢速", "缓慢", "緩慢", "慢慢", "轻柔", "輕柔", "温柔", "低速", "ゆっくり", "やさしく", "優しく", "穏やか", "低速")
	middle := hasIntentPhrase(message,
		"medium", "medium pace", "medium speed", "moderate", "moderately",
		"medio", "media", "moderado", "moderada", "velocidad media",
		"médio", "média", "moderado", "moderada", "velocidade média",
	) || containsAny(message, "中速", "适中", "適中", "中等速度", "适度", "適度", "中くらい", "普通の速さ")
	high := hasIntentPhrase(message,
		"fast", "hard", "as fast as", "fastest", "full speed", "max speed", "maximum speed",
		"rápido", "rápida", "fuerte", "máxima velocidad", "a toda velocidad", "lo más rápido",
		"forte", "velocidade máxima", "o mais rápido",
	) || containsAny(message, "快速", "最快", "最快速", "全速", "最大速度", "尽可能快", "盡可能快", "用力", "高速", "最速", "できるだけ速く", "強く")
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

func requestsMotionVariation(message string) bool {
	message = normalizeMotionIntent(message)
	if motionIntentIsNegated(message) || motionIntentIsConversation(message) {
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
		"change it up", "change things up", "keep changing it up", "mix it up", "mix things up",
		"keep mixing it up", "keep switching it up", "keep varying it", "something different",
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
		"keep changing it up", "keep mixing it up", "keep switching it up", "keep varying it",
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
