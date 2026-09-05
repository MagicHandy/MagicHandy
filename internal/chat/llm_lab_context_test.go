package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
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
