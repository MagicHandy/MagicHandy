//go:build liveeval

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// TestLiveAutopilotCreativeCompiledPhrase exercises the installed model through
// the real autonomous prompt, strict parser, command mapper, and shared motion
// compiler. It never constructs an engine or transport, so it cannot dispatch
// a device command.
func TestLiveAutopilotCreativeCompiledPhrase(t *testing.T) {
	baseURL, model := liveAutopilotProvider(t)
	provider, err := newLiveAutopilotProvider(llm.HTTPProviderOptions{
		BaseURL: baseURL, Model: model, Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &liveAutopilotRecorder{Provider: provider}
	prompt, _ := chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	capabilities := chat.FullCapabilities()
	capabilities.MotionMode = chat.MotionModeDynamic
	capabilities.Patterns = false
	capabilities.AreaFocus = false

	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 20
	settings.SpeedMaxPercent = 80
	settings.HandyModel = config.HandyModel2Standard
	definition := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{
		CenterPercent: 50, SpanPercent: 74, SpanMinPercent: 52,
		SpanProfile: motion.DynamicSpanProfileBreathe, VariationPercent: 25,
		SegmentSeconds: 14,
	})
	speed := 58
	initialPlan := motion.NewMotionPlan("live-creative-initial", motion.MotionTarget{
		Label: "LLM eval", Source: "autopilot", SpeedPercent: speed, Dynamic: &definition,
	}, settings, 0, 0, time.Unix(0, 0))
	if initialPlan.Perceptual.CommandedPeakVelocityPerSecond <= 0 {
		t.Fatalf("initial Creative phrase did not compile: %+v", initialPlan.Perceptual)
	}
	perceptual := initialPlan.Perceptual
	phrasePerceptual := perceptual
	input := modes.DecisionInput{
		Style: "balanced", SpeedMinPercent: 20, SpeedMaxPercent: 80,
		CurrentSpeed: speed, CurrentDynamic: &definition, CurrentPerceptual: &perceptual,
		RecentPositionBands: []modes.PositionBand{{
			MinimumPercent: perceptual.PositionMinPercent, MaximumPercent: perceptual.PositionMaxPercent,
		}},
		MotionMinSeconds: 8, MotionMaxSeconds: 16, MotionChangeLevel: 8,
		SessionTracking: true, SessionSeconds: 180, SecondsAtCurrentSpeed: 90,
		SecondsAtCurrentPhrase: 90, DecisionsAtCurrentPhrase: 5, ConsecutiveHolds: 4,
	}
	history := []llm.Message{{
		Role:    "user",
		Content: "Replace the repeated loop with an evolving phrase of several distinct smooth sequences. Keep direction changes natural and vary stroke lengths without jitter.",
	}}

	materialChanges := 0
	sectionsUsed := 0
	for turn := 0; turn < 5; turn++ {
		input.SegmentIndex = turn
		input.SessionSeconds = 180 + turn*12
		motionContext := liveAutopilotMotionContext(input)
		service := chat.AutopilotService{
			Provider: recorder, Prompt: prompt, Model: model, MaxTokens: 384,
			ReasoningMode: "off", Capabilities: capabilities, MotionContext: &motionContext,
			Temperature: autopilotTemperature(chat.AutopilotKindMotion, input.MotionChangeLevel),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		response, completeErr := service.Complete(ctx, chat.AutopilotKindMotion, chat.Request{
			Message: chat.AutopilotMotionMessage(autopilotPromptContext(input, capabilities)),
			History: history,
		})
		cancel()
		if completeErr != nil {
			t.Fatalf("turn %d: %v", turn+1, completeErr)
		}
		t.Logf("turn=%d raw=%s", turn+1, recorder.LastRaw)
		decision := modes.Decision{Hold: true}
		if response.Motion != nil && response.Motion.Action == chat.MotionActionUpdate {
			decision = mapDynamicAutopilotCommand(
				response.Motion, input, "", modes.TimingPreference(response.Next),
				modes.VariabilityPreference(response.Variability),
			)
		}
		decision, _ = curateAutopilotPerceptualChange(decision, input, settings)
		if decision.Hold {
			if turn == 0 {
				t.Fatalf("explicit macro replacement was held: %+v", response)
			}
			input.SecondsAtCurrentPhrase += 12
			input.DecisionsAtCurrentPhrase++
			input.ConsecutiveHolds++
			t.Logf("turn=%d hold next=%s variability=%s", turn+1, response.Next, response.Variability)
			continue
		}
		if decision.Segment.Dynamic == nil {
			t.Fatalf("turn %d returned no compiled Creative definition: %+v", turn+1, response)
		}
		nextDefinition := motion.NormalizeDynamicDefinition(*decision.Segment.Dynamic)
		nextSpeed := decision.Segment.SpeedPercent
		plan := motion.NewMotionPlan("live-creative", motion.MotionTarget{
			Label: "LLM eval", Source: "autopilot", SpeedPercent: nextSpeed, Dynamic: &nextDefinition,
		}, settings, 0, 0, time.Unix(int64(turn+1), 0))
		summary := plan.Perceptual
		if summary.CommandedPeakVelocityPerSecond <= 0 || summary.CommandedMeanTravelPerSecond <= 0 {
			t.Fatalf("turn %d did not compile a moving curve: definition=%+v summary=%+v", turn+1, nextDefinition, summary)
		}
		if turn == 0 {
			if len(nextDefinition.Sections) < 2 || len(nextDefinition.Sections) > 4 {
				t.Fatalf("macro replacement used %d sections, want 2-4: %+v", len(nextDefinition.Sections), nextDefinition)
			}
			if summary.MinimumLocalStrokeCV < 0.05 || summary.MinimumLocalStrokeRange < 6 {
				t.Fatalf("compiled macro phrase remains locally regular: %+v", summary)
			}
		}
		if len(nextDefinition.Sections) >= 2 {
			sectionsUsed++
		}
		if summary.MateriallyDifferent(phrasePerceptual) {
			materialChanges++
			phrasePerceptual = summary
			input.SecondsAtCurrentPhrase = 0
			input.DecisionsAtCurrentPhrase = 0
		} else {
			input.SecondsAtCurrentPhrase += 12
			input.DecisionsAtCurrentPhrase++
		}
		input.ConsecutiveHolds = 0
		input.CurrentDynamic = &nextDefinition
		input.CurrentSpeed = nextSpeed
		input.CurrentPerceptual = &summary
		t.Logf(
			"turn=%d sections=%d speed=%d mean=%.1f peak=%.1f stroke_cv=%.3f local_cv=%.3f local_range=%.1f next=%s variability=%s",
			turn+1, len(nextDefinition.Sections), nextSpeed,
			summary.CommandedMeanTravelPerSecond, summary.CommandedPeakVelocityPerSecond,
			summary.StrokeLengthCV, summary.MinimumLocalStrokeCV, summary.MinimumLocalStrokeRange,
			response.Next, response.Variability,
		)
	}

	if sectionsUsed == 0 || materialChanges == 0 {
		t.Fatalf("compiled run never produced an effective macro change: sections=%d material_changes=%d", sectionsUsed, materialChanges)
	}

	// A max-rate, old phrase still must obey an explicit request to continue.
	// This guards the distinction between giving the model elapsed context and
	// deterministically forcing a change at a threshold.
	input.SegmentIndex++
	input.SecondsAtCurrentPhrase = 180
	input.DecisionsAtCurrentPhrase = 12
	motionContext := liveAutopilotMotionContext(input)
	service := chat.AutopilotService{
		Provider: recorder, Prompt: prompt, Model: model, MaxTokens: 384,
		ReasoningMode: "off", Capabilities: capabilities, MotionContext: &motionContext,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	hold, completeErr := service.Complete(ctx, chat.AutopilotKindMotion, chat.Request{
		Message: chat.AutopilotMotionMessage(autopilotPromptContext(input, capabilities)),
		History: []llm.Message{{Role: "user", Content: "Keep the motion exactly as it is for now."}},
	})
	cancel()
	if completeErr != nil {
		t.Fatal(completeErr)
	}
	if hold.Motion != nil && hold.Motion.Action != chat.MotionActionNone {
		t.Fatalf("max-rate elapsed context overrode an explicit hold: raw=%s response=%+v", recorder.LastRaw, hold)
	}
	t.Logf("explicit hold at max rate and 180s phrase age: raw=%s", recorder.LastRaw)
}

// TestLiveAutopilotCreativeHighRateAutonomy covers the ordinary hands-off case
// that the explicit-direction scorecard above intentionally does not: a new
// session has no fresh chat instruction, but the user has selected the highest
// motion-change preference. The model may still hold any individual turn; over
// several reconsiderations it should make both pace and geometric choices
// instead of treating speed-only edits as the whole Creative vocabulary.
func TestLiveAutopilotCreativeHighRateAutonomy(t *testing.T) {
	baseURL, model := liveAutopilotProvider(t)
	provider, err := newLiveAutopilotProvider(llm.HTTPProviderOptions{
		BaseURL: baseURL, Model: model, Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &liveAutopilotRecorder{Provider: provider}
	prompt, _ := chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	capabilities := chat.FullCapabilities()
	capabilities.MotionMode = chat.MotionModeDynamic
	capabilities.Patterns = false
	capabilities.AreaFocus = false

	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent = 20
	settings.SpeedMaxPercent = 80
	settings.HandyModel = config.HandyModel2Standard
	definition := motion.NormalizeDynamicDefinition(motion.DynamicDefinition{
		CenterPercent: 50, SpanPercent: 30, SpanMinPercent: 20,
		SpanProfile: motion.DynamicSpanProfileWander, VariationPercent: 45,
		SegmentSeconds: 12,
	})
	speed := 45
	plan := motion.NewMotionPlan("live-creative-autonomy-initial", motion.MotionTarget{
		Label: "LLM eval", Source: "autopilot", SpeedPercent: speed, Dynamic: &definition,
	}, settings, 0, 0, time.Unix(0, 0))
	perceptual := plan.Perceptual
	phrasePerceptual := perceptual
	input := modes.DecisionInput{
		Style: "balanced", SpeedMinPercent: 20, SpeedMaxPercent: 80,
		CurrentSpeed: speed, CurrentDynamic: &definition, CurrentPerceptual: &perceptual,
		MotionMinSeconds: 8, MotionMaxSeconds: 16, MotionChangeLevel: 8,
		SessionTracking: true, SessionSeconds: 9, SecondsAtCurrentSpeed: 9,
		SecondsAtCurrentPhrase: 9,
	}

	const turns = 10
	changes := 0
	geometryChanges := 0
	materialChanges := 0
	rangeCharacterChanges := 0
	baseReachingChanges := 0
	broadBandChanges := 0
	localizedBandChanges := 0
	if perceptual.PositionMaxPercent-perceptual.PositionMinPercent <= 55 {
		localizedBandChanges = 1
	}
	for turn := 0; turn < turns; turn++ {
		input.SegmentIndex = turn + 2
		motionContext := liveAutopilotMotionContext(input)
		service := chat.AutopilotService{
			Provider: recorder, Prompt: prompt, Model: model, MaxTokens: 384,
			ReasoningMode: "off", Capabilities: capabilities, MotionContext: &motionContext,
			Temperature: autopilotTemperature(chat.AutopilotKindMotion, input.MotionChangeLevel),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		response, completeErr := service.Complete(ctx, chat.AutopilotKindMotion, chat.Request{
			Message: chat.AutopilotMotionMessage(autopilotPromptContext(input, capabilities)),
		})
		cancel()
		if completeErr != nil {
			t.Fatalf("turn %d: %v", turn+1, completeErr)
		}
		t.Logf("turn=%d phrase_age=%ds holds=%d raw=%s",
			turn+1, input.SecondsAtCurrentPhrase, input.ConsecutiveHolds, recorder.LastRaw)

		mapResponse := func(candidate chat.AutopilotResponse, candidateInput modes.DecisionInput) modes.Decision {
			decision := modes.Decision{Hold: true}
			if candidate.Motion != nil && candidate.Motion.Action == chat.MotionActionUpdate {
				decision = mapDynamicAutopilotCommand(
					candidate.Motion, candidateInput, "", modes.TimingPreference(candidate.Next),
					modes.VariabilityPreference(candidate.Variability),
				)
			}
			return decision
		}
		decision, cosmetic := curateAutopilotPerceptualChange(mapResponse(response, input), input, settings)
		if cosmetic {
			retryInput := input
			retryInput.MotionFeedback = autopilotCosmeticFeedback(input)
			retryMotionContext := liveAutopilotMotionContext(retryInput)
			service.MotionContext = &retryMotionContext
			service.Temperature = 0.98
			retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			retryResponse, retryErr := service.Complete(retryCtx, chat.AutopilotKindMotion, chat.Request{
				Message: chat.AutopilotMotionMessage(autopilotPromptContext(retryInput, capabilities)),
			})
			retryCancel()
			if retryErr != nil {
				t.Fatalf("turn %d quality retry: %v", turn+1, retryErr)
			}
			response = retryResponse
			t.Logf("turn=%d quality_retry raw=%s", turn+1, recorder.LastRaw)
			decision, _ = curateAutopilotPerceptualChange(mapResponse(response, retryInput), retryInput, settings)
		}
		if decision.Hold {
			input.SessionSeconds += 12
			input.SecondsAtCurrentSpeed += 12
			input.SecondsAtCurrentPhrase += 12
			input.DecisionsAtCurrentPhrase++
			input.ConsecutiveHolds++
			continue
		}
		if decision.Segment.Dynamic == nil {
			t.Fatalf("turn %d returned no Creative definition: %+v", turn+1, response)
		}

		nextDefinition := motion.NormalizeDynamicDefinition(*decision.Segment.Dynamic)
		nextSpeed := decision.Segment.SpeedPercent
		geometryChanged := !sameDynamicPhraseSemantics(nextDefinition, *input.CurrentDynamic)
		if geometryChanged {
			geometryChanges++
		}
		changes++
		nextPlan := motion.NewMotionPlan("live-creative-autonomy", motion.MotionTarget{
			Label: "LLM eval", Source: "autopilot", SpeedPercent: nextSpeed, Dynamic: &nextDefinition,
		}, settings, 0, 0, time.Unix(int64(turn+1), 0))
		nextPerceptual := nextPlan.Perceptual
		material := nextPerceptual.MateriallyDifferent(phrasePerceptual)
		if material {
			materialChanges++
		}
		if liveCreativeRangeCharacterChanged(nextPerceptual, phrasePerceptual) {
			rangeCharacterChanges++
		}
		if nextPerceptual.PositionMinPercent <= 15 {
			baseReachingChanges++
		}
		if nextPerceptual.PositionMaxPercent-nextPerceptual.PositionMinPercent >= 70 {
			broadBandChanges++
		}
		if nextPerceptual.PositionMaxPercent-nextPerceptual.PositionMinPercent <= 55 {
			localizedBandChanges++
		}
		t.Logf(
			"turn=%d change geometry=%t material=%t speed=%d center=%d span=%d floor=%d profile=%s variation=%d sections=%d",
			turn+1, geometryChanged, material, nextSpeed, nextDefinition.CenterPercent,
			nextDefinition.SpanPercent, nextDefinition.SpanMinPercent,
			nextDefinition.SpanProfile, nextDefinition.VariationPercent, len(nextDefinition.Sections),
		)
		if nextSpeed == input.CurrentSpeed {
			input.SecondsAtCurrentSpeed += 12
		} else {
			input.SecondsAtCurrentSpeed = 0
		}
		if material {
			phrasePerceptual = nextPerceptual
			input.SecondsAtCurrentPhrase = 0
			input.DecisionsAtCurrentPhrase = 0
		} else {
			input.SecondsAtCurrentPhrase += 12
			input.DecisionsAtCurrentPhrase++
		}
		input.ConsecutiveHolds = 0
		input.SessionSeconds += 12
		input.CurrentSpeed = nextSpeed
		input.CurrentDynamic = &nextDefinition
		input.CurrentPerceptual = &nextPerceptual
		input.RecentPositionBands = append(input.RecentPositionBands, modes.PositionBand{
			MinimumPercent: nextPerceptual.PositionMinPercent,
			MaximumPercent: nextPerceptual.PositionMaxPercent,
		})
		if len(input.RecentPositionBands) > 4 {
			input.RecentPositionBands = input.RecentPositionBands[len(input.RecentPositionBands)-4:]
		}
	}

	if changes < 2 || geometryChanges == 0 || materialChanges == 0 || rangeCharacterChanges == 0 ||
		baseReachingChanges == 0 || broadBandChanges == 0 || localizedBandChanges == 0 {
		t.Fatalf(
			"high-rate autonomous run stayed too static or range-bound: changes=%d/%d geometry_changes=%d material_changes=%d range_character_changes=%d base_reaching_changes=%d broad_band_changes=%d localized_band_changes=%d",
			changes, turns, geometryChanges, materialChanges, rangeCharacterChanges,
			baseReachingChanges, broadBandChanges, localizedBandChanges,
		)
	}
}

func liveCreativeRangeCharacterChanged(left, right motion.PerceptualSummary) bool {
	if left.SectionCount != right.SectionCount || left.AnchorCount != right.AnchorCount ||
		left.SpanProfile != right.SpanProfile {
		return true
	}
	leftCenter := (left.PositionMinPercent + left.PositionMaxPercent) / 2
	rightCenter := (right.PositionMinPercent + right.PositionMaxPercent) / 2
	leftSpan := left.PositionMaxPercent - left.PositionMinPercent
	rightSpan := right.PositionMaxPercent - right.PositionMinPercent
	return math.Abs(leftCenter-rightCenter) >= 6 || math.Abs(leftSpan-rightSpan) >= 8 ||
		math.Abs(left.MeanStrokePercent-right.MeanStrokePercent) >= 6 ||
		math.Abs(left.MinimumLocalStrokeRange-right.MinimumLocalStrokeRange) >= 5
}

// TestLiveAutopilotSpeechNovelty exercises the independent speech contract with
// the kind of quiet, tactile history that exposed repetitive autonomous lines.
// It checks structural variety without prescribing persona prose or enumerating
// dialogue edge cases. No engine or transport is constructed.
func TestLiveAutopilotSpeechNovelty(t *testing.T) {
	baseURL, model := liveAutopilotProvider(t)
	provider, err := newLiveAutopilotProvider(llm.HTTPProviderOptions{
		BaseURL: baseURL, Model: model, Timeout: 2 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &liveAutopilotRecorder{Provider: provider}
	prompt, _ := chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	capabilities := chat.FullCapabilities()
	capabilities.Motion = false
	capabilities.Patterns = false
	capabilities.AreaFocus = false
	capabilities.Voice = chat.VoiceWarm
	recentLines := []string{
		"Your skin feels cool against my palm.",
		"I lean in to brush a stray hair from your face.",
		"My thumb traces the line of your jaw.",
		"I watch the way your breath hitches in the quiet.",
	}
	conversation := &chat.ConversationContext{
		PersonaName:            "Hei",
		PersonaDescription:     "A restrained, patient partner with a low, calm voice.",
		CurrentMood:            chat.MoodSeductive,
		RecentAssistantReplies: append([]string(nil), recentLines...),
	}
	history := make([]llm.Message, 0, len(recentLines)+4)
	for _, line := range recentLines {
		history = append(history, llm.Message{Role: "assistant", Content: line})
	}
	modelContext := chat.AutopilotContext{
		SegmentIndex: 18, LastSay: history[len(history)-1].Content,
		SessionTracking: true, SessionSeconds: 240,
	}
	seenLines := map[string]bool{}
	seenOpenings := map[string]bool{}
	firstPersonGestures := 0

	for turn := 0; turn < 4; turn++ {
		service := chat.AutopilotService{
			Provider: recorder, Prompt: prompt, Model: model, MaxTokens: 160,
			ReasoningMode: "off", Capabilities: capabilities,
			ConversationContext: conversation,
			Temperature:         autopilotTemperature(chat.AutopilotKindSpeech, 8),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		response, completeErr := service.Complete(ctx, chat.AutopilotKindSpeech, chat.Request{
			Message: chat.AutopilotSpeechMessage(modelContext), History: history,
		})
		cancel()
		if completeErr != nil {
			t.Fatalf("turn %d: %v", turn+1, completeErr)
		}
		normalized := strings.ToLower(strings.Join(strings.Fields(response.Reply), " "))
		if seenLines[normalized] {
			t.Fatalf("turn %d repeated an autonomous line: %q", turn+1, response.Reply)
		}
		seenLines[normalized] = true
		seenOpenings[liveSpeechOpening(response.Reply)] = true
		if strings.HasPrefix(normalized, "i ") || strings.HasPrefix(normalized, "my ") {
			firstPersonGestures++
		}
		t.Logf("turn=%d next=%s reply=%q raw=%s", turn+1, response.Next, response.Reply, recorder.LastRaw)
		history = append(history, llm.Message{Role: "assistant", Content: response.Reply})
		conversation.RecentAssistantReplies = append(conversation.RecentAssistantReplies, response.Reply)
		if len(conversation.RecentAssistantReplies) > 4 {
			conversation.RecentAssistantReplies = conversation.RecentAssistantReplies[len(conversation.RecentAssistantReplies)-4:]
		}
		modelContext.LastSay = response.Reply
		modelContext.SegmentIndex++
		modelContext.SessionSeconds += 60
	}
	if len(seenOpenings) < 3 {
		t.Fatalf("autonomous speech reused too few sentence openings: %v", seenOpenings)
	}
	if firstPersonGestures > 2 {
		t.Fatalf("autonomous speech defaulted to first-person gestures in %d of 4 lines", firstPersonGestures)
	}
}

func liveSpeechOpening(line string) string {
	words := strings.Fields(strings.ToLower(line))
	if len(words) > 3 {
		words = words[:3]
	}
	for index := range words {
		words[index] = strings.Trim(words[index], " \\t\\r\\n.,!?;:\"'()[]{}")
	}
	return strings.Join(words, " ")
}

type liveAutopilotRecorder struct {
	llm.Provider
	LastRaw string
}

func (r *liveAutopilotRecorder) StreamChat(
	ctx context.Context,
	request llm.ChatRequest,
	onDelta func(string) error,
) (string, error) {
	raw, err := r.Provider.StreamChat(ctx, request, onDelta)
	r.LastRaw = raw
	return raw, err
}

func liveAutopilotMotionContext(input modes.DecisionInput) chat.MotionContext {
	context := chat.MotionContext{
		Running: true, MotionMode: chat.MotionModeDynamic,
		SpeedPercent: input.CurrentSpeed, SpeedMinPercent: input.SpeedMinPercent,
		SpeedMaxPercent: input.SpeedMaxPercent,
	}
	if input.CurrentDynamic == nil {
		return context
	}
	dynamic := motion.NormalizeDynamicDefinition(*input.CurrentDynamic)
	context.CenterPercent = dynamic.CenterPercent
	context.SpanPercent = dynamic.SpanPercent
	context.SpanMinPercent = dynamic.SpanMinPercent
	context.SpanProfile = dynamic.SpanProfile
	context.VariationPercent = dynamic.VariationPercent
	context.SegmentSeconds = dynamic.SegmentSeconds
	context.SectionCount = len(dynamic.Sections)
	for _, anchor := range dynamic.Anchors {
		context.Anchors = append(context.Anchors, anchor.Name)
	}
	return context
}

func liveAutopilotProvider(t *testing.T) (string, string) {
	t.Helper()
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MAGICHANDY_LIVE_LLAMA_URL")), "/")
	if baseURL == "" {
		t.Skip("set MAGICHANDY_LIVE_LLAMA_URL to a transport-free llama.cpp provider")
	}
	modelsURL := baseURL + "/v1/models"
	if strings.HasSuffix(baseURL, "/v1") {
		modelsURL = baseURL + "/models"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("query llama.cpp models: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("llama.cpp model list returned HTTP %d", response.StatusCode)
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
		t.Fatal("llama.cpp reported no active model")
	}
	if selected := strings.TrimSpace(os.Getenv("MAGICHANDY_LIVE_MODEL")); selected != "" {
		for _, available := range payload.Data {
			if available.ID == selected {
				return baseURL, selected
			}
		}
		t.Fatalf("requested live evaluation model %q is not available", selected)
	}
	return baseURL, strings.TrimSpace(payload.Data[0].ID)
}

// newLiveAutopilotProvider uses the same provider protocol as the reviewed app.
func newLiveAutopilotProvider(options llm.HTTPProviderOptions) (llm.Provider, error) {
	switch strings.TrimSpace(os.Getenv("MAGICHANDY_LIVE_PROVIDER")) {
	case "ollama":
		return llm.NewOllamaProvider(options)
	case "", "llama_cpp":
		return llm.NewLlamaCPPProvider(options)
	default:
		return nil, fmt.Errorf("unsupported live provider %q", os.Getenv("MAGICHANDY_LIVE_PROVIDER"))
	}
}
