//go:build liveeval

package httpapi

import (
	"context"
	"encoding/json"
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
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{
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
	input := modes.DecisionInput{
		Style: "balanced", SpeedMinPercent: 20, SpeedMaxPercent: 80,
		CurrentSpeed: speed, CurrentDynamic: &definition, CurrentPerceptual: &perceptual,
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
		if summary.MateriallyDifferent(*input.CurrentPerceptual) {
			materialChanges++
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
	return baseURL, strings.TrimSpace(payload.Data[0].ID)
}
