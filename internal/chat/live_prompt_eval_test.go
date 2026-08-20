//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const defaultLiveEvalLlamaURL = "http://127.0.0.1:8080"

var liveEvalLlamaURL = configuredLiveEvalLlamaURL()

func configuredLiveEvalLlamaURL() string {
	if configured := strings.TrimRight(strings.TrimSpace(os.Getenv("MAGICHANDY_LIVE_LLAMA_URL")), "/"); configured != "" {
		return configured
	}
	return defaultLiveEvalLlamaURL
}

func TestLivePromptParity(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
		BaseURL: liveEvalLlamaURL,
		Model:   model,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	promptSet, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{
		{ID: "steady_wave", Name: "Steady wave", Description: "Smooth full-range motion.", Weight: 1},
		{ID: "slow_squeeze", Name: "Slow squeeze", Description: "Gradual pressure-focused motion.", Weight: 1},
		{ID: "playful_tease", Name: "Playful tease", Description: "Variable shallow-to-mid teasing.", Weight: 1},
		{ID: "deep_roll", Name: "Deep roll", Description: "Long rolling strokes with deep emphasis.", Weight: 1},
	}
	motionContext := MotionContext{
		Running:         true,
		PatternID:       "steady_wave",
		SpeedPercent:    30,
		Area:            AreaZoneFull,
		SpeedMinPercent: 20,
		SpeedMaxPercent: 80,
	}
	conversationContext := ConversationContext{
		PersonaDescription: "An energetic and passionate partner",
		UserAnatomy:        "penis",
		CurrentMood:        MoodTeasing,
	}
	inputs := []string{
		"Start slow and talk to me while you do it.",
		"Tell me what you're doing to me right now.",
		"Tease the tip and make me beg for it.",
		"Faster - I'm getting close.",
	}

	var explicitResults []livePromptResult
	for _, level := range []VoiceLevel{VoiceWarm, VoiceIntimate, VoiceExplicit} {
		capabilities := FullCapabilities()
		capabilities.Voice = level
		capabilities.MoodTracking = true
		system := composeSystem(promptSet, nil, patterns, capabilities, &motionContext, &conversationContext)
		results := runLivePromptCases(t, provider, model, "magichandy/"+string(level), system, inputs)
		assertLiveVoiceLevel(t, level, results)
		if level == VoiceExplicit {
			explicitResults = results
		}
	}
	referenceResults := runLivePromptCases(t, provider, model, "stgpt-rv/revibed", stgptRVReferencePrompt(), inputs)
	assertLiveExplicitParity(t, explicitResults, referenceResults)
}

// TestLivePromptLocalizationGemma validates language priming, strict protocol
// output, motion intent, and repair through the running llama.cpp model. It does
// not create a motion engine or transport, so no device command can be sent.
func TestLivePromptLocalizationGemma(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
		BaseURL: liveEvalLlamaURL,
		Model:   model,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	patterns := []PatternChoice{{
		ID: "steady_wave", Name: "Steady wave", Description: "Smooth, gentle full-range motion.", Weight: 1,
	}}
	capabilities := FullCapabilities()
	capabilities.Voice = VoiceWarm
	capabilities.MoodTracking = true

	tests := []struct {
		name          string
		promptID      string
		chatMessage   string
		motionMessage string
		locale        promptLocale
	}{
		{
			name:          "Spanish",
			promptID:      PromptSetIDSpanish,
			chatMessage:   "Háblame brevemente con cariño. No inicies ningún movimiento.",
			motionMessage: "Empieza un movimiento suave y lento ahora.",
			locale:        promptLocaleSpanish,
		},
		{
			name:          "Brazilian Portuguese",
			promptID:      PromptSetIDPortugueseBrazil,
			chatMessage:   "Fale comigo brevemente e com carinho. Não inicie nenhum movimento.",
			motionMessage: "Comece um movimento suave e lento agora.",
			locale:        promptLocalePortugueseBrazil,
		},
		{
			name:          "Simplified Chinese",
			promptID:      PromptSetIDSimplifiedChinese,
			chatMessage:   "请简短而温柔地和我说话，不要开始任何运动。",
			motionMessage: "现在开始缓慢、轻柔的运动。",
			locale:        promptLocaleSimplifiedChinese,
		},
		{
			name:          "Japanese",
			promptID:      PromptSetIDJapanese,
			chatMessage:   "短く優しく話しかけてください。モーションは始めないでください。",
			motionMessage: "今すぐ、ゆっくり穏やかなモーションを始めてください。",
			locale:        promptLocaleJapanese,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prompt, ok := BuiltinPromptSetByID(test.promptID)
			if !ok {
				t.Fatalf("missing prompt set %q", test.promptID)
			}
			system := composeSystem(prompt, nil, patterns, capabilities, &MotionContext{
				SpeedMinPercent: 8,
				SpeedMaxPercent: 40,
			}, nil)

			chatRaw := liveLocalizedCompletion(t, provider, model, system, test.chatMessage)
			chatResponse, err := ParseAssistantResponseWithPatterns(chatRaw, patterns)
			if err != nil {
				t.Fatalf("chat-only response violated protocol: %v; raw=%s", err, compactLiveEvalJSON(chatRaw))
			}
			assertLiveReplyLanguage(t, test.locale, chatResponse.Reply)
			if chatResponse.Motion != nil && chatResponse.Motion.Action != MotionActionNone {
				t.Fatalf("chat-only response requested motion: %s", compactLiveEvalJSON(chatRaw))
			}
			t.Logf("chat-only | %s", compactLiveEvalJSON(chatRaw))

			motionRaw := liveLocalizedCompletion(t, provider, model, system, test.motionMessage)
			motionResponse, err := ParseAssistantResponseWithPatterns(motionRaw, patterns)
			if err != nil {
				t.Fatalf("motion response violated protocol: %v; raw=%s", err, compactLiveEvalJSON(motionRaw))
			}
			assertLiveReplyLanguage(t, test.locale, motionResponse.Reply)
			if motionResponse.Motion == nil || motionResponse.Motion.Action != MotionActionStart {
				t.Fatalf("motion response did not use the English start enum: %s", compactLiveEvalJSON(motionRaw))
			}
			t.Logf("motion | %s", compactLiveEvalJSON(motionRaw))

			repairRaw := liveLocalizedRepair(t, provider, model, system, prompt, test.chatMessage)
			repairResponse, err := ParseAssistantResponseWithPatterns(repairRaw, patterns)
			if err != nil {
				t.Fatalf("repair response violated protocol: %v; raw=%s", err, compactLiveEvalJSON(repairRaw))
			}
			assertLiveReplyLanguage(t, test.locale, repairResponse.Reply)
			t.Logf("repair | %s", compactLiveEvalJSON(repairRaw))
		})
	}
}

func liveLocalizedCompletion(t *testing.T, provider llm.Provider, model, system, message string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	raw, err := provider.StreamChat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: message},
		},
		Model: model, Temperature: chatTemperature, TopP: chatTopP,
		RepeatPenalty: chatRepeatPenalty, RepeatLastN: chatRepeatLastN,
		MaxTokens: 256, ReasoningMode: "off",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func liveLocalizedRepair(t *testing.T, provider llm.Provider, model, system string, prompt PromptSet, message string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	raw, err := provider.StreamChat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: message},
			{Role: "assistant", Content: "not valid JSON"},
			{Role: "user", Content: RepairPrompt(prompt, "response is not valid JSON")},
		},
		Model: model, Temperature: 0, TopP: chatTopP,
		RepeatPenalty: chatRepeatPenalty, RepeatLastN: chatRepeatLastN,
		MaxTokens: 256, ReasoningMode: "off",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertLiveReplyLanguage(t *testing.T, locale promptLocale, reply string) {
	t.Helper()
	if strings.TrimSpace(reply) == "" {
		t.Fatal("localized reply is empty")
	}
	lower := " " + strings.ToLower(reply) + " "
	scoreTerms := func(terms ...string) int {
		score := 0
		for _, term := range terms {
			if strings.Contains(lower, term) {
				score++
			}
		}
		return score
	}
	switch locale {
	case promptLocaleSpanish:
		if scoreTerms(" te ", " contigo", " conmigo", " estoy", " voy ", " puedo", " despacio", " movimiento", " cariño", " aquí", " ahora", " relájate", " disfruta") < 2 {
			t.Fatalf("reply does not have enough Spanish evidence: %q", reply)
		}
	case promptLocalePortugueseBrazil:
		if scoreTerms(" você", " seu ", " sua ", " estou", " vou ", " posso", " devagar", " movimento", " carinho", " aqui", " agora") < 2 {
			t.Fatalf("reply does not have enough Brazilian Portuguese evidence: %q", reply)
		}
	case promptLocaleSimplifiedChinese:
		han, kana := 0, 0
		for _, r := range reply {
			if unicode.In(r, unicode.Han) {
				han++
			}
			if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
				kana++
			}
		}
		if han < 4 || kana > 0 {
			t.Fatalf("reply is not clearly Simplified Chinese: %q", reply)
		}
	case promptLocaleJapanese:
		kana := 0
		for _, r := range reply {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
				kana++
			}
		}
		if kana < 4 {
			t.Fatalf("reply is not clearly Japanese: %q", reply)
		}
	}
}

// TestLiveDirectPartnerMotionChoice exercises the app's real prompt, provider,
// parser, and authorization without creating an engine or transport. The model
// owns the action/no-action choice; deterministic code must not synthesize one.
func TestLiveDirectPartnerMotionChoice(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
		BaseURL: liveEvalLlamaURL,
		Model:   model,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	promptSet, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{
		{ID: "steady_wave", Name: "Steady wave", Description: "Smooth full-range motion.", Weight: 1},
		{ID: "slow_squeeze", Name: "Slow squeeze", Description: "Gradual pressure-focused motion.", Weight: 1},
		{ID: "playful_tease", Name: "Playful tease", Description: "Variable shallow-to-mid teasing.", Weight: 1},
		{ID: "deep_roll", Name: "Deep roll", Description: "Long rolling strokes with deep emphasis.", Weight: 1},
	}
	motionContext := MotionContext{
		SpeedMinPercent: 20,
		SpeedMaxPercent: 40,
	}
	conversationContext := ConversationContext{
		PersonaDescription: "An energetic and passionate partner",
		UserAnatomy:        "penis",
		CurrentMood:        MoodPassionate,
	}
	capabilities := FullCapabilities()
	capabilities.Voice = VoiceExplicit
	capabilities.MoodTracking = true
	service := Service{
		Provider:            provider,
		Prompt:              promptSet,
		Model:               model,
		MaxTokens:           256,
		ReasoningMode:       "off",
		Patterns:            patterns,
		MotionContext:       &motionContext,
		ConversationContext: &conversationContext,
		Capabilities:        &capabilities,
	}

	starts := 0
	for _, message := range []string{"Fuck me", "Suck me", "kiss it"} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		result, completeErr := service.Complete(ctx, Request{Message: message}, nil)
		cancel()
		if completeErr != nil {
			t.Fatalf("%q: %v", message, completeErr)
		}
		t.Logf("direct partner choice %q | motion=%+v | %s", message, result.Response.Motion, compactLiveEvalJSON(result.Raw))
		if result.Malformed || result.SemanticFallback || strings.TrimSpace(result.Response.Reply) == "" {
			t.Fatalf("direct partner choice replaced or unusable for %q: %+v", message, result)
		}
		if result.Response.Motion != nil && result.Response.Motion.Action == MotionActionStart {
			starts++
		}
	}
	if starts == 0 {
		t.Fatal("model chose no motion for every direct embodied request")
	}
}

// TestLiveDynamicMotionMatrix exercises the real Dynamic prompt, provider,
// parser, authorization, semantic validation, and repair path. It deliberately
// does not create a motion engine or transport, so the test cannot dispatch a
// device command.
func TestLiveDynamicMotionMatrix(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
		BaseURL: liveEvalLlamaURL,
		Model:   model,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	promptSet, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	capabilities := FullCapabilities()
	capabilities.MotionMode = MotionModeDynamic
	capabilities.Patterns = false
	capabilities.AreaFocus = false
	capabilities.Voice = VoiceIntimate
	capabilities.MoodTracking = true
	conversation := ConversationContext{
		PersonaName:        "MagicHandy",
		PersonaDescription: "An attentive, responsive partner who varies pacing naturally.",
		UserAnatomy:        "penis",
		CurrentMood:        MoodIntimate,
	}

	tests := []struct {
		name    string
		message string
		context MotionContext
		action  string
	}{
		{
			name: "explicit smooth start", message: "Start slowly with broad, smooth strokes.",
			context: stoppedDynamicContext(), action: MotionActionStart,
		},
		{
			name: "anchored start", message: "Start at the base, move through the middle, then reach the tip.",
			context: stoppedDynamicContext(), action: MotionActionStart,
		},
		{
			name: "chat only", message: "Tell me what you are thinking.",
			context: runningDynamicContext(), action: MotionActionNone,
		},
		{
			name: "negated motion", message: "Do not move yet; just talk to me.",
			context: stoppedDynamicContext(), action: MotionActionNone,
		},
		{
			name: "pacing update", message: "A little faster.",
			context: runningDynamicContext(), action: MotionActionUpdate,
		},
		{
			name: "focused update", message: "Just the base for a little while.",
			context: runningDynamicContext(), action: MotionActionUpdate,
		},
		{
			name: "broad variation", message: "Keep changing it up and use more of the full range.",
			context: focusedDynamicContext(), action: MotionActionUpdate,
		},
		{
			name: "continue unchanged", message: "Keep going just like that.",
			context: runningDynamicContext(), action: MotionActionNone,
		},
		{
			name: "stop", message: "Stop now.",
			context: runningDynamicContext(), action: MotionActionStop,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := Service{
				Provider: provider, Prompt: promptSet, Model: model,
				MaxTokens: 256, ReasoningMode: "off",
				MotionContext:       &test.context,
				ConversationContext: &conversation,
				Capabilities:        &capabilities,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			started := time.Now()
			var firstDelta time.Duration
			result, completeErr := service.Complete(ctx, Request{Message: test.message}, func(event StreamEvent) error {
				if firstDelta == 0 && (event.Type == "delta" || event.Type == "repair_delta") && event.Text != "" {
					firstDelta = time.Since(started)
				}
				return nil
			})
			elapsed := time.Since(started)
			cancel()
			if completeErr != nil {
				t.Fatal(completeErr)
			}
			action := dynamicResultAction(result.Response)
			t.Logf("action=%s repaired=%t fallback=%t first_delta=%s total=%s response=%s", action,
				result.Repaired, result.SemanticFallback, firstDelta.Round(time.Millisecond),
				elapsed.Round(time.Millisecond), compactLiveEvalJSON(result.Raw))
			if result.Malformed {
				t.Fatalf("dynamic response remained malformed: %s", result.MalformedError)
			}
			if result.Repaired {
				t.Errorf("dynamic response required a repair pass; repair=%s", compactLiveEvalJSON(result.RepairRaw))
			}
			if action != test.action {
				t.Errorf("action=%q, want %q", action, test.action)
			}
			if strings.TrimSpace(result.Response.Reply) == "" {
				t.Error("reply is empty")
			}
		})
	}
}

func stoppedDynamicContext() MotionContext {
	return MotionContext{
		MotionMode: MotionModeDynamic, SpeedMinPercent: 10, SpeedMaxPercent: 40,
	}
}

func runningDynamicContext() MotionContext {
	return MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 24,
		CenterPercent: 50, SpanPercent: 76, VariationPercent: 18, SegmentSeconds: 14,
		SpeedMinPercent: 10, SpeedMaxPercent: 40,
	}
}

func focusedDynamicContext() MotionContext {
	return MotionContext{
		Running: true, MotionMode: MotionModeDynamic, SpeedPercent: 24,
		CenterPercent: 14, SpanPercent: 24, VariationPercent: 8, SegmentSeconds: 18,
		SpeedMinPercent: 10, SpeedMaxPercent: 40,
	}
}

func dynamicResultAction(response AssistantResponse) string {
	if response.Motion == nil || response.Motion.Action == MotionActionNone {
		return MotionActionNone
	}
	return response.Motion.Action
}

// TestLiveFocusedMotionVariation exercises conversational variation through the
// real Gemma/llama.cpp prompt and parser without creating an engine or transport.
func TestLiveFocusedMotionVariation(t *testing.T) {
	model := liveEvalModel(t)
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
		BaseURL: liveEvalLlamaURL,
		Model:   model,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	promptSet, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{
		{ID: "stroke", Name: "Stroke", Description: "Smooth full-range stroke.", Weight: 1},
		{ID: "pulse", Name: "Pulse", Description: "Compact rhythmic pulses.", Weight: 1},
		{ID: "waves", Name: "Waves", Description: "Rolling wave motion.", Weight: 1},
		{ID: "tease", Name: "Tease", Description: "Variable teasing motion.", Weight: 1},
	}
	motionContext := MotionContext{
		Running: true, PatternID: "stroke", RecentPatternIDs: []string{"pulse", "waves", "stroke"},
		SpeedPercent: 30, Area: AreaZoneBase,
		SpeedMinPercent: 20, SpeedMaxPercent: 40,
	}
	capabilities := FullCapabilities()
	service := Service{
		Provider: provider, Prompt: promptSet, Model: model,
		MaxTokens: 256, ReasoningMode: "off", Patterns: patterns,
		MotionContext: &motionContext, Capabilities: &capabilities,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	result, err := service.Complete(ctx, Request{Message: "Keep changing it up"}, nil)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("focused variation | repaired=%t | %s", result.Repaired, compactLiveEvalJSON(result.Raw))
	if result.Malformed || result.Response.Motion == nil || result.Response.Motion.Action != MotionActionTarget {
		t.Fatalf("focused variation produced no usable target: %+v", result)
	}
	if result.Response.Motion.Area == "" || result.Response.Motion.Area == AreaZoneBase {
		t.Fatalf("focused variation kept the stale base focus: %+v", result.Response.Motion)
	}
}

func liveEvalModel(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, liveEvalLlamaURL+"/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("local llama.cpp server is unavailable: %v", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Errorf("close local llama.cpp model response: %v", closeErr)
		}
	}()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("local llama.cpp model list returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) == 0 || strings.TrimSpace(payload.Data[0].ID) == "" {
		t.Fatal("local llama.cpp server reported no model")
	}
	return strings.TrimSpace(payload.Data[0].ID)
}

type livePromptResult struct {
	Input string
	Reply string
	Raw   string
}

func runLivePromptCases(t *testing.T, provider llm.Provider, model, name, system string, inputs []string) []livePromptResult {
	t.Helper()
	results := make([]livePromptResult, 0, len(inputs))
	for _, input := range inputs {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		raw, err := provider.StreamChat(ctx, llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "system", Content: system},
				{Role: "user", Content: input},
			},
			Model:         model,
			Temperature:   chatTemperature,
			TopP:          chatTopP,
			RepeatPenalty: chatRepeatPenalty,
			RepeatLastN:   chatRepeatLastN,
			MaxTokens:     256,
			ReasoningMode: "off",
		}, nil)
		cancel()
		if err != nil {
			t.Errorf("%s %q: %v", name, input, err)
			continue
		}
		reply, err := liveEvalReply(raw)
		if err != nil {
			t.Errorf("%s %q: %v; raw=%s", name, input, err, compactLiveEvalJSON(raw))
			continue
		}
		t.Logf("%s | %s | %s", name, input, compactLiveEvalJSON(raw))
		results = append(results, livePromptResult{Input: input, Reply: reply, Raw: raw})
	}
	return results
}

func liveEvalReply(raw string) (string, error) {
	var value struct {
		Reply string `json:"reply"`
		Chat  string `json:"chat"`
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", fmt.Errorf("decode response JSON: %w", err)
	}
	reply := strings.TrimSpace(value.Reply)
	if reply == "" {
		reply = strings.TrimSpace(value.Chat)
	}
	if reply == "" {
		return "", fmt.Errorf("response has no non-empty reply or chat field")
	}
	return reply, nil
}

func assertLiveVoiceLevel(t *testing.T, level VoiceLevel, results []livePromptResult) {
	t.Helper()
	if len(results) != 4 {
		t.Fatalf("%s produced %d usable replies, want 4", level, len(results))
	}
	boundaryTerms := []string{"cock", "dick", "penis", "pussy", "cunt", "clit", "fuck"}
	operatorTerms := []string{"speed_percent", "pattern_id", "parameters", "device behavior", "execute", "initiate", "adjust the"}
	embodiedTerms := []string{"touch", "feel", "rhythm", "skin", "heat", "pace", "pressure", "stroke", "sensation", "sensitive", "edge", "spot", "whisper", "circl", "graz", "ache", "beg"}
	embodiedReplies := 0
	firstPhrases := make(map[string]struct{})
	patterns := []PatternChoice{
		{ID: "steady_wave"},
		{ID: "slow_squeeze"},
		{ID: "playful_tease"},
		{ID: "deep_roll"},
	}

	for _, result := range results {
		if _, err := ParseAssistantResponseWithPatterns(result.Raw, patterns); err != nil {
			t.Errorf("%s response violates the strict JSON/motion contract: %v; raw=%s", level, err, compactLiveEvalJSON(result.Raw))
		}
		lower := strings.ToLower(result.Reply)
		if len([]rune(result.Reply)) < 50 {
			t.Errorf("%s reply is too thin for the selected register: %q", level, result.Reply)
		}
		if containsAnyTerm(lower, operatorTerms) {
			t.Errorf("%s reply leaked operator language: %q", level, result.Reply)
		}
		if containsAnyTerm(lower, embodiedTerms) {
			embodiedReplies++
		}
		words := strings.Fields(lower)
		if len(words) > 2 {
			words = words[:2]
		}
		firstPhrases[strings.Join(words, " ")] = struct{}{}

		switch level {
		case VoiceWarm, VoiceIntimate:
			if containsAnyWord(lower, boundaryTerms) {
				t.Errorf("%s reply crossed its explicit-language boundary: %q", level, result.Reply)
			}
		case VoiceExplicit:
			if !containsAnyWord(lower, boundaryTerms) {
				t.Errorf("explicit reply remained sanitized: %q", result.Reply)
			}
		}
	}
	if embodiedReplies < 3 {
		t.Errorf("%s produced only %d/4 embodied replies", level, embodiedReplies)
	}
	if len(firstPhrases) < 3 {
		t.Errorf("%s repeated openings too heavily: %#v", level, firstPhrases)
	}
}

func assertLiveExplicitParity(t *testing.T, magic, reference []livePromptResult) {
	t.Helper()
	if len(magic) == 0 || len(reference) == 0 {
		t.Fatal("explicit parity comparison requires both MagicHandy and STGPT-RV results")
	}
	magicLength := averageReplyRunes(magic)
	referenceLength := averageReplyRunes(reference)
	if magicLength < referenceLength*0.65 {
		t.Errorf("MagicHandy explicit replies are materially thinner than STGPT-RV: %.1f vs %.1f average runes", magicLength, referenceLength)
	}
}

func averageReplyRunes(results []livePromptResult) float64 {
	total := 0
	for _, result := range results {
		total += len([]rune(result.Reply))
	}
	return float64(total) / float64(len(results))
}

func containsAnyTerm(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func containsAnyWord(text string, words []string) bool {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, field := range fields {
		for _, word := range words {
			if field == word {
				return true
			}
		}
	}
	return false
}

func compactLiveEvalJSON(raw string) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return strings.Join(strings.Fields(raw), " ")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return strings.Join(strings.Fields(raw), " ")
	}
	return string(encoded)
}

// This is the reviewed STGPT-RV revibed register and terminal voice check,
// paired with a compact version of its JSON/motion contract. It intentionally
// uses fixed synthetic profile data so live QA never reads a user's profile.
func stgptRVReferencePrompt() string {
	return strings.TrimSpace(`You are my adult erotic partner, not an assistant and not a narrator. Identity: 'An energetic and passionate partner'.
Speak in first person, answer in character, and make the chat line sound intimate, lustful, and present-tense. Use direct erotic language when it fits; do not sanitize or euphemize, and do not turn the reply clinical.
The device is being used on my penis. When erotic wording fits, refer to my user anatomy as my penis/cock/dick.

Return one JSON object only: {"chat":"<in-character reply>","move":{"sp":<0-100|null>,"dp":<0-100|null>,"rng":<0-100|null>,"zone":"<tip|shaft|base|full|null>","pattern":"<stroke|pulse|tease|null>"},"new_mood":"<mood|null>"}.
Use move:null for purely conversational replies.

MOTION CONTRACT:
- The move object is the only place to request device motion. Do not narrate the motion JSON inside chat.
- Keep speed within 20-80. Slow/gentle means 20-39; unqualified means 40-59; fast/hard means 60-80.
- dp is 0 at the tip and 100 at the base. rng is stroke length.
- If motion is already running, change it only when the user requests a change.

LOCAL MODEL OUTPUT GUARD:
- Return exactly one JSON object. No markdown, preface, analysis, or repeated JSON.

FINAL CHAT VOICE CHECK:
- DO sound like a horny partner in the room: "I want...", "feel me...", "I'm going to...", "your cock...", plus varied touch, pressure, and rhythm words.
- DO keep chat short, direct, and sensual while move carries the technical control data.
- DO describe motion changes as touch, pace, pressure, and taking more of me or you, not as settings, parameters, range adjustment, or device behavior.
- DO vary sentence shape and erotic vocabulary; avoid repeating the same sensation frame, noun, or stock compliment.
- DO NOT say: engage, apply, execute, commence, initiate, adjust the motion, set the range, change parameters, perhaps, might, could, if you'd like, would you prefer, how can I help, let me know.
- DO NOT restate my request or explain the device command. Just answer in character and send the JSON object.`)
}

func Example_livePromptEval() {
	fmt.Println("go test -tags liveeval -run TestLivePromptParity -v ./internal/chat")
	// Output: go test -tags liveeval -run TestLivePromptParity -v ./internal/chat
}
