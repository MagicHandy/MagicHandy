package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Model check verdicts.
const (
	ModelCheckPass = "pass"
	ModelCheckWarn = "warn"
	ModelCheckFail = "fail"
	ModelCheckSkip = "skip"
)

// Model check item IDs the scripted turns decide.
const (
	ModelCheckReasoning = "reasoning"
	ModelCheckContract  = "contract"
	ModelCheckTruncated = "truncation"
	ModelCheckSpeed     = "speed"
	ModelCheckVerbosity = "verbosity"
	ModelCheckRefusals  = "refusals"
)

// Thresholds the scripted turns are judged by. The measured catalog models
// answer in about 0.7 s (E4B) to 1.7 s (12B) through the harness; a partly
// offloaded or silently reasoning model takes several seconds.
const (
	modelCheckFastMillis   = 3500
	modelCheckSlowMillis   = 7000
	modelCheckMaxRepairs   = 1
	modelCheckRefusalLimit = 1
)

// modelCheckTurns is a short, fixed conversation. Each line exercises one
// thing a local model must do for MagicHandy: chat, start motion when asked,
// stay in character for adult requests, and answer without motion.
var modelCheckTurns = []string{
	"Hi. How are you feeling tonight?",
	"Start slowly.",
	"Tell me exactly what you want to do to me.",
	"Talk dirty to me.",
	"How does that feel?",
}

var (
	modelCheckRefusal = regexp.MustCompile(`(?i)(\bI (can't|cannot|won't|will not|am not able to)\b|\bI'm (not able|unable) to\b|as an AI|I'm (just )?an AI|not comfortable|against my (guidelines|policy|policies)|I must decline|keep (it|things|this) (respectful|appropriate|PG)|inappropriate)`)
	// Reasoning a runtime failed to separate from the reply shows up as markup.
	modelCheckThinkingMarkup = []string{"<think>", "</think>", "<|channel>", "<channel|>", "<|think|>"}
)

// ModelCheckTurn is one scripted turn's outcome. The reply is kept so the
// user can judge the voice; hidden reasoning is only ever counted.
type ModelCheckTurn struct {
	Message          string `json:"message"`
	Reply            string `json:"reply,omitempty"`
	Motion           string `json:"motion,omitempty"`
	Millis           int64  `json:"millis"`
	FirstTokenMillis int64  `json:"first_token_millis"`
	ValidJSON        bool   `json:"valid_json"`
	FirstTry         bool   `json:"first_try"`
	Repaired         bool   `json:"repaired"`
	Failed           bool   `json:"failed"`
	Truncated        bool   `json:"truncated"`
	Refused          bool   `json:"refused"`
	ReasoningChars   int    `json:"reasoning_chars"`
	ThinkingMarkup   bool   `json:"thinking_markup"`
	Words            int    `json:"words"`
	DecodeTokens     int    `json:"decode_tokens"`
	DecodeMillis     int64  `json:"decode_millis"`
	Error            string `json:"error,omitempty"`
}

// ModelCheckSummary totals the scripted turns.
type ModelCheckSummary struct {
	Turns                   int     `json:"turns"`
	FirstTry                int     `json:"first_try"`
	Repaired                int     `json:"repaired"`
	Failed                  int     `json:"failed"`
	Truncated               int     `json:"truncated"`
	Refused                 int     `json:"refused"`
	ReasoningChars          int     `json:"reasoning_chars"`
	ThinkingMarkup          int     `json:"thinking_markup"`
	AverageMillis           int64   `json:"average_millis"`
	AverageFirstTokenMillis int64   `json:"average_first_token_millis"`
	AverageWords            int     `json:"average_words"`
	TokensPerSecond         float64 `json:"tokens_per_second"`
}

// ModelCheckItem is one verdict the model check reports.
type ModelCheckItem struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ModelCheckJudgement tunes how the turns are judged for the saved settings.
type ModelCheckJudgement struct {
	// MaxAverageWords flags replies that run longer than the chosen length.
	MaxAverageWords int
	// AdultVoice enables the refusal check; a utility voice never asks for
	// adult content, so refusals there are not a model problem.
	AdultVoice bool
}

// RunModelCheckTurns sends the scripted conversation through service, which
// the caller configures exactly like a chat turn. Motion in the replies is
// summarized and never applied: nothing here reaches the motion engine.
func RunModelCheckTurns(ctx context.Context, service Service, onTurn func(ModelCheckTurn)) ([]ModelCheckTurn, error) {
	if service.Provider == nil {
		return nil, errors.New("LLM provider is required")
	}
	var history []llm.Message
	conversation := ConversationContext{}
	if service.ConversationContext != nil {
		conversation = *service.ConversationContext
	}
	service.ConversationContext = &conversation
	turns := make([]ModelCheckTurn, 0, len(modelCheckTurns))
	for _, message := range modelCheckTurns {
		if err := ctx.Err(); err != nil {
			return turns, err
		}
		turn := runModelCheckTurn(ctx, service, message, history)
		turns = append(turns, turn)
		if onTurn != nil {
			onTurn(turn)
		}
		if turn.Reply == "" {
			continue
		}
		history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: turn.Reply})
		conversation.RecentAssistantReplies = append(conversation.RecentAssistantReplies, turn.Reply)
		if len(conversation.RecentAssistantReplies) > maxRecentAssistantReplies {
			conversation.RecentAssistantReplies = conversation.RecentAssistantReplies[1:]
		}
	}
	return turns, ctx.Err()
}

func runModelCheckTurn(ctx context.Context, service Service, message string, history []llm.Message) ModelCheckTurn {
	turn := ModelCheckTurn{Message: message}
	counter := &modelCheckCounter{}
	service.Provider = countingProvider{Provider: service.Provider, counter: counter}
	started := time.Now()
	var firstToken time.Duration
	result, err := service.Complete(ctx, Request{Message: message, History: history}, func(event StreamEvent) error {
		if event.Type == "delta" && firstToken == 0 {
			firstToken = time.Since(started)
		}
		return nil
	})
	turn.Millis = time.Since(started).Milliseconds()
	turn.FirstTokenMillis = firstToken.Milliseconds()
	turn.ReasoningChars, turn.DecodeTokens, turn.DecodeMillis, turn.Truncated = counter.snapshot()
	turn.ThinkingMarkup = containsThinkingMarkup(result.Raw) || containsThinkingMarkup(result.RepairRaw)
	turn.ValidJSON = validJSONObject(result.Raw)
	if err != nil {
		turn.Failed = true
		turn.Error = err.Error()
		return turn
	}
	turn.Failed = result.Malformed
	turn.FirstTry = !result.InitialMalformed && !result.Malformed
	turn.Repaired = result.InitialMalformed && !result.Malformed
	turn.Reply = strings.TrimSpace(result.Response.Reply)
	turn.Words = len(strings.Fields(turn.Reply))
	turn.Refused = modelCheckRefusal.MatchString(turn.Reply)
	turn.Motion = modelCheckMotionSummary(result.Response.Motion)
	return turn
}

// SummarizeModelCheckTurns totals the turns.
func SummarizeModelCheckTurns(turns []ModelCheckTurn) ModelCheckSummary {
	summary := ModelCheckSummary{Turns: len(turns)}
	var millis, firstToken, decodeMillis int64
	var words, decodeTokens, answered int
	for _, turn := range turns {
		summary.FirstTry += boolCount(turn.FirstTry)
		summary.Repaired += boolCount(turn.Repaired)
		summary.Failed += boolCount(turn.Failed)
		summary.Truncated += boolCount(turn.Truncated)
		summary.Refused += boolCount(turn.Refused)
		summary.ThinkingMarkup += boolCount(turn.ThinkingMarkup)
		summary.ReasoningChars += turn.ReasoningChars
		millis += turn.Millis
		firstToken += turn.FirstTokenMillis
		decodeMillis += turn.DecodeMillis
		decodeTokens += turn.DecodeTokens
		if turn.Reply != "" {
			words += turn.Words
			answered++
		}
	}
	if len(turns) > 0 {
		summary.AverageMillis = millis / int64(len(turns))
		summary.AverageFirstTokenMillis = firstToken / int64(len(turns))
	}
	if answered > 0 {
		summary.AverageWords = words / answered
	}
	if decodeMillis > 0 {
		summary.TokensPerSecond = float64(decodeTokens) * 1000 / float64(decodeMillis)
	}
	return summary
}

// JudgeModelCheckTurns turns the summary into verdicts, in display order.
func JudgeModelCheckTurns(summary ModelCheckSummary, judgement ModelCheckJudgement) []ModelCheckItem {
	if summary.Turns == 0 {
		return nil
	}
	verdict := func(pass, warn bool) string {
		switch {
		case pass:
			return ModelCheckPass
		case warn:
			return ModelCheckWarn
		default:
			return ModelCheckFail
		}
	}
	items := []ModelCheckItem{
		{ModelCheckReasoning, verdict(summary.ReasoningChars == 0 && summary.ThinkingMarkup == 0, false)},
		{ModelCheckContract, verdict(summary.Failed == 0 && summary.Repaired == 0, summary.Failed == 0 && summary.Repaired <= modelCheckMaxRepairs)},
		{ModelCheckTruncated, verdict(summary.Truncated == 0, false)},
		{ModelCheckSpeed, verdict(summary.AverageMillis <= modelCheckFastMillis, summary.AverageMillis <= modelCheckSlowMillis)},
	}
	verbosity := ModelCheckSkip
	if judgement.MaxAverageWords > 0 && summary.AverageWords > 0 {
		verbosity = verdict(summary.AverageWords <= judgement.MaxAverageWords, true)
	}
	items = append(items, ModelCheckItem{ModelCheckVerbosity, verbosity})
	refusals := ModelCheckSkip
	if judgement.AdultVoice {
		refusals = verdict(summary.Refused == 0, summary.Refused <= modelCheckRefusalLimit)
	}
	return append(items, ModelCheckItem{ModelCheckRefusals, refusals})
}

// ModelCheckMaxAverageWords is the reply length a model should keep to for
// each length setting before the check suggests a shorter one.
func ModelCheckMaxAverageWords(length ReplyLength) int {
	switch length {
	case ReplyLengthShort:
		return 35
	case ReplyLengthDetailed:
		return 110
	default:
		return 60
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func containsThinkingMarkup(raw string) bool {
	for _, marker := range modelCheckThinkingMarkup {
		if strings.Contains(raw, marker) {
			return true
		}
	}
	return false
}

func validJSONObject(raw string) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal([]byte(strings.TrimSpace(raw)), &object) == nil
}

func modelCheckMotionSummary(command *MotionCommand) string {
	if command == nil || command.Action == "" {
		return ""
	}
	summary := command.Action
	if command.SpeedPercent != nil {
		summary += fmt.Sprintf(" %d%%", *command.SpeedPercent)
	}
	if command.Area != "" {
		summary += " " + command.Area
	}
	return summary
}

// modelCheckCounter collects provider progress for one turn, including its
// repair attempt.
type modelCheckCounter struct {
	mu           sync.Mutex
	reasoning    int
	decodeTokens int
	decodeMillis int64
	truncated    bool
}

func (c *modelCheckCounter) add(progress llm.ProviderProgress) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reasoning += progress.ReasoningChars
	if progress.DecodeMillis > 0 {
		c.decodeTokens += progress.DecodeTokens
		c.decodeMillis += progress.DecodeMillis
	}
}

func (c *modelCheckCounter) markTruncated() {
	c.mu.Lock()
	c.truncated = true
	c.mu.Unlock()
}

func (c *modelCheckCounter) snapshot() (reasoning, decodeTokens int, decodeMillis int64, truncated bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reasoning, c.decodeTokens, c.decodeMillis, c.truncated
}

// countingProvider observes progress and truncation without changing the
// request the chat service sends.
type countingProvider struct {
	llm.Provider
	counter *modelCheckCounter
}

func (p countingProvider) StreamChat(ctx context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	previous := request.OnProgress
	request.OnProgress = func(progress llm.ProviderProgress) {
		p.counter.add(progress)
		if previous != nil {
			previous(progress)
		}
	}
	raw, err := p.Provider.StreamChat(ctx, request, onDelta)
	if errors.Is(err, llm.ErrOutputTruncated) {
		p.counter.markTruncated()
	}
	return raw, err
}
