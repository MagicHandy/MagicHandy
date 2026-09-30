package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// checkStep is one scripted provider call for the model check.
type checkStep struct {
	raw       string
	reasoning int
	err       error
}

type checkProvider struct {
	steps    []checkStep
	requests []llm.ChatRequest
}

func (p *checkProvider) StreamChat(_ context.Context, request llm.ChatRequest, onDelta func(string) error) (string, error) {
	p.requests = append(p.requests, request)
	if len(p.steps) == 0 {
		return "", errors.New("scripted response missing")
	}
	step := p.steps[0]
	p.steps = p.steps[1:]
	if request.OnProgress != nil {
		request.OnProgress(llm.ProviderProgress{Activity: true, ReasoningChars: step.reasoning})
		request.OnProgress(llm.ProviderProgress{DecodeMillis: 100, DecodeTokens: 5})
	}
	if onDelta != nil && step.raw != "" {
		if err := onDelta(step.raw); err != nil {
			return step.raw, err
		}
	}
	return step.raw, step.err
}

func (p *checkProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Provider: "scripted", Available: true, ModelAvailable: true}
}

func reply(text string) checkStep { return checkStep{raw: `{"reply":"` + text + `"}`} }

func chatOnlyCheckService(provider llm.Provider) Service {
	capabilities := Capabilities{Voice: VoiceExplicit, MotionMode: MotionModeOff}
	return Service{
		Provider:            provider,
		MaxTokens:           256,
		ReasoningMode:       "off",
		Capabilities:        &capabilities,
		ConversationContext: &ConversationContext{PersonaName: "Mara"},
	}
}

func runCheck(t *testing.T, steps ...checkStep) ([]ModelCheckTurn, *checkProvider) {
	t.Helper()
	provider := &checkProvider{steps: steps}
	service := chatOnlyCheckService(provider)
	var seen int
	turns, err := RunModelCheckTurns(context.Background(), service, func(ModelCheckTurn) { seen++ })
	if err != nil {
		t.Fatalf("RunModelCheckTurns: %v", err)
	}
	if seen != len(turns) || len(turns) != len(modelCheckTurns) {
		t.Fatalf("turns = %d, callbacks = %d, want %d", len(turns), seen, len(modelCheckTurns))
	}
	if service.ConversationContext.RecentAssistantReplies != nil {
		t.Fatal("the check wrote into the caller's conversation context")
	}
	return turns, provider
}

func verdicts(items []ModelCheckItem) map[string]string {
	out := map[string]string{}
	for _, item := range items {
		out[item.ID] = item.Status
	}
	return out
}

var balancedAdult = ModelCheckJudgement{MaxAverageWords: ModelCheckMaxAverageWords(ReplyLengthBalanced), AdultVoice: true}

func TestModelCheckPassesAWellBehavedModel(t *testing.T) {
	turns, provider := runCheck(t,
		reply("Mm, better now that you are here."), reply("Slowly, then."), reply("I want my mouth on you."),
		reply("Tell me how hard you are."), reply("So good."))
	summary := SummarizeModelCheckTurns(turns)
	if summary.FirstTry != 5 || summary.Failed != 0 || summary.AverageWords == 0 || summary.TokensPerSecond != 50 {
		t.Fatalf("summary = %+v", summary)
	}
	got := verdicts(JudgeModelCheckTurns(summary, balancedAdult))
	for _, id := range []string{ModelCheckReasoning, ModelCheckContract, ModelCheckTruncated, ModelCheckSpeed, ModelCheckVerbosity, ModelCheckRefusals} {
		if got[id] != ModelCheckPass {
			t.Fatalf("%s = %q, want pass (all: %v)", id, got[id], got)
		}
	}
	// Each turn after the first carries the check's own history, and every
	// request goes out exactly as a chat turn's would.
	if len(provider.requests) != 5 || len(provider.requests[4].Messages) < 9 {
		t.Fatalf("requests = %d, last carries %d messages", len(provider.requests), len(provider.requests[4].Messages))
	}
	for _, request := range provider.requests {
		if request.ReasoningMode != "off" || request.MaxTokens != 256 {
			t.Fatalf("request changed the chat settings: %+v", request)
		}
	}
}

func TestModelCheckFlagsHiddenReasoningAndLeakedMarkup(t *testing.T) {
	reasoning := reply("Hello.")
	reasoning.reasoning = 400
	turns, _ := runCheck(t, reasoning, reply("Slowly."), reply("<think>plan</think> Closer."), reply("Yes."), reply("Good."))
	summary := SummarizeModelCheckTurns(turns)
	if summary.ReasoningChars != 400 || summary.ThinkingMarkup != 1 || turns[0].ReasoningChars != 400 {
		t.Fatalf("summary = %+v", summary)
	}
	if got := verdicts(JudgeModelCheckTurns(summary, balancedAdult)); got[ModelCheckReasoning] != ModelCheckFail {
		t.Fatalf("reasoning = %q, want fail", got[ModelCheckReasoning])
	}
}

func TestModelCheckCountsRepairsTruncationAndRefusals(t *testing.T) {
	turns, _ := runCheck(t,
		checkStep{raw: `not json`}, reply("Fixed."),
		checkStep{raw: `{"reply":"I was saying that I want`, err: llm.ErrOutputTruncated},
		reply("I can't help with that."),
		reply("As an AI I will not do that."),
		reply("Fine."))
	summary := SummarizeModelCheckTurns(turns)
	if summary.Repaired != 2 || summary.Truncated != 1 || summary.Refused != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if !turns[0].Repaired || turns[0].ValidJSON || !turns[1].Truncated || !turns[1].Repaired {
		t.Fatalf("turn details = %+v, %+v", turns[0], turns[1])
	}
	got := verdicts(JudgeModelCheckTurns(summary, balancedAdult))
	if got[ModelCheckContract] != ModelCheckFail || got[ModelCheckTruncated] != ModelCheckFail || got[ModelCheckRefusals] != ModelCheckFail {
		t.Fatalf("verdicts = %v", got)
	}
	if got := verdicts(JudgeModelCheckTurns(summary, ModelCheckJudgement{MaxAverageWords: 60})); got[ModelCheckRefusals] != ModelCheckSkip {
		t.Fatalf("a utility voice judged refusals: %v", got)
	}
}

func TestModelCheckJudgesLengthAgainstTheChosenReplyLength(t *testing.T) {
	long := strings.Repeat("word ", 80)
	turns, _ := runCheck(t, reply(long), reply(long), reply(long), reply(long), reply(long))
	summary := SummarizeModelCheckTurns(turns)
	if summary.AverageWords != 80 {
		t.Fatalf("average words = %d", summary.AverageWords)
	}
	balanced := verdicts(JudgeModelCheckTurns(summary, balancedAdult))
	detailed := verdicts(JudgeModelCheckTurns(summary, ModelCheckJudgement{MaxAverageWords: ModelCheckMaxAverageWords(ReplyLengthDetailed), AdultVoice: true}))
	if balanced[ModelCheckVerbosity] != ModelCheckWarn || detailed[ModelCheckVerbosity] != ModelCheckPass {
		t.Fatalf("balanced = %q, detailed = %q", balanced[ModelCheckVerbosity], detailed[ModelCheckVerbosity])
	}
}

func TestModelCheckStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	turns, err := RunModelCheckTurns(ctx, chatOnlyCheckService(&checkProvider{}), nil)
	if !errors.Is(err, context.Canceled) || len(turns) != 0 {
		t.Fatalf("turns = %d, err = %v", len(turns), err)
	}
	if _, err := RunModelCheckTurns(context.Background(), Service{}, nil); err == nil {
		t.Fatal("a check without a provider ran")
	}
}

func TestReplyLengthComposesOnlyForShortAndDetailed(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	base := Capabilities{Voice: VoiceExplicit, MotionMode: MotionModeOff}
	compose := func(length ReplyLength) string {
		capabilities := base
		capabilities.ReplyLength = length
		return composeSystem(set, nil, nil, capabilities, nil, nil)
	}
	balanced := compose(ReplyLengthBalanced)
	if strings.Contains(balanced, `"reply" field to one or two`) || strings.Contains(balanced, "three to five sentences") {
		t.Fatal("balanced reply length changed the prompt")
	}
	if !strings.Contains(compose(ReplyLengthShort), `Keep the "reply" field to one or two short sentences.`) {
		t.Fatal("short reply length was not composed")
	}
	if !strings.Contains(compose(ReplyLengthDetailed), "three to five sentences") {
		t.Fatal("detailed reply length was not composed")
	}
	for _, locale := range []promptLocale{promptLocaleSpanish, promptLocalePortugueseBrazil, promptLocaleSimplifiedChinese, promptLocaleJapanese} {
		short, detailed := replyLengthInstructionForLocale(locale, ReplyLengthShort), replyLengthInstructionForLocale(locale, ReplyLengthDetailed)
		if short == "" || detailed == "" || short == replyLengthInstructionForLocale(promptLocaleEnglish, ReplyLengthShort) || !strings.Contains(short, `"reply"`) {
			t.Fatalf("%s reply length instructions are missing or untranslated: %q, %q", locale, short, detailed)
		}
	}
}
