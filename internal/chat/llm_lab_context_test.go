package chat

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLabRefreshesNumericPlanningContextWithoutHardwareNames(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"reply":"Keeping the score."}`, `{"reply":"Keeping the score."}`}}
	limits := config.DefaultSettings().Motion
	for index, profile := range []string{config.HandyModelOriginal, config.HandyModel2Pro} {
		limits.HandyModel = profile
		limits.SpeedMinPercent, limits.SpeedMaxPercent = 10+index, 43-index
		limits.StrokeMinPercent, limits.StrokeMaxPercent = 20, 70
		trial := RunLLMLab(t.Context(), provider, "test", "edits", LLMLabPrompts()["edits"], "Keep the score.", motion.DefaultFlowSpec(), limits, nil, true)
		if !trial.Valid || trial.Limits != limits {
			t.Fatalf("trial must retain reproducible full settings: %+v", trial)
		}
		request := provider.requests[index]
		content := request.Messages[len(request.Messages)-1].Content
		for _, excluded := range []string{"handy_model", profile, "reverse_direction", "apply_video_speed_limit", "stroke_min_percent", "style"} {
			if strings.Contains(content, excluded) {
				t.Fatalf("model received irrelevant settings %q: %s", excluded, content)
			}
		}
		var input struct {
			Limits   map[string]int          `json:"saved_limits"`
			Envelope motion.PlanningEnvelope `json:"engine_envelope"`
		}
		encoded, _, _ := strings.Cut(content, "\nRequest:")
		if err := json.Unmarshal([]byte(encoded), &input); err != nil {
			t.Fatal(err)
		}
		if len(input.Limits) != 2 || input.Limits["speed_min_percent"] != 10+index || input.Limits["speed_max_percent"] != 43-index {
			t.Fatalf("stale or excessive planning limits: %+v", input.Limits)
		}
		wantPeak := []float64{363.6, 360}[index]
		if input.Envelope.PositionMinPercent != 0 || input.Envelope.PositionMaxPercent != 100 || input.Envelope.ProfilePeakVelocityPerSecond != wantPeak {
			t.Fatalf("profile reference or semantic coordinates incorrect: %+v", input.Envelope)
		}
	}
}

// Continuous Lab methods share production's continuation policy, which reads
// the human's latest lines from the planning context. Lab history carries the
// human turns and replies, so those lines must reach that field.
func TestLabContinuationSeesTheLatestHumanLines(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"edits":[],"reply":"Holding."}`}}
	history := []llm.Message{{Role: "user", Content: "Slow it down a little."}, {Role: "assistant", Content: `{"edits":[],"reply":"Slower."}`},
		{Role: "user", Content: "Keep it exactly like this now."}, {Role: "assistant", Content: `{"edits":[],"reply":"Keeping it."}`}}
	trial := RunLLMLab(t.Context(), provider, "test", "creative_v2", LLMLabPrompts()["creative_v2"], CreativeV2ContinuationMessage(),
		FreshCreativeV2Score(25), config.DefaultSettings().Motion, history, true)
	if !trial.Valid {
		t.Fatal(trial.Error)
	}
	content := provider.requests[0].Messages[len(provider.requests[0].Messages)-1].Content
	encoded, _, _ := strings.Cut(content, "\nRequest:")
	var input struct {
		Requests []string `json:"recent_user_requests_oldest_first"`
	}
	if err := json.Unmarshal([]byte(encoded), &input); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Slow it down a little.", "Keep it exactly like this now."}; !slices.Equal(input.Requests, want) {
		t.Fatalf("continuation saw %q, want %q", input.Requests, want)
	}
}
