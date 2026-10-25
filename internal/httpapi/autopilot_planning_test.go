package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func TestIntermediatePlanningToggleReachesNextDecision(t *testing.T) {
	for _, mode := range []string{config.LLMMotionModeLayered, config.LLMMotionModeCreativeV2} {
		t.Run(mode, func(t *testing.T) {
			edits := `{}`
			flow := chat.DefaultLayeredScore(30)
			if mode == config.LLMMotionModeCreativeV2 {
				edits, flow = `[]`, chat.FreshCreativeV2Score(30)
			}
			provider := &scriptedLLMProvider{responses: []string{
				`{"edits":` + edits + `,"reply":"Keeping this rhythm."}`,
				`{"basis":"Keep the exact rhythm.","edits":` + edits + `,"reply":"Keeping this rhythm."}`,
				`{"edits":` + edits + `,"reply":"Keeping this rhythm."}`,
			}}
			s := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
			t.Cleanup(s.Close)
			saveSettings(t, s.store, func(settings config.Settings) config.Settings {
				settings.LLM.LocalCapabilityLevel = config.ModelCapabilityIntermediate
				settings.LLM.MotionGenerationMode = mode
				settings.LLM.LlamaCPPContextSize, settings.LLM.MaxOutputTokens = 32768, 512
				return settings
			})
			for i, enabled := range []bool{false, true, false} {
				saved, _ := s.store.Snapshot()
				preferences := saved.Autopilot
				preferences.IntermediatePlanning = enabled
				data, _ := json.Marshal(preferences)
				putAutopilotPlanning(t, s, data)
				// An older client's unrelated edit must preserve this preference.
				var older map[string]any
				_ = json.Unmarshal(data, &older)
				delete(older, "intermediate_planning")
				data, _ = json.Marshal(older)
				putAutopilotPlanning(t, s, data)
				saved, _ = s.store.Snapshot()
				if saved.Autopilot.IntermediatePlanning != enabled {
					t.Fatal("save or older client reset the planning preference")
				}
				_, err := s.autopilotDecide(t.Context(), modes.DecisionInput{CurrentFlow: &flow, CurrentSpeed: 30, SpeedMinPercent: 10, SpeedMaxPercent: 80})
				if err != nil {
					t.Fatal(err)
				}
				provider.mu.Lock()
				count, request := len(provider.requests), provider.requests[i]
				provider.mu.Unlock()
				if count != i+1 || strings.Contains(string(request.JSONSchema), `"basis"`) != enabled || request.MaxTokens != 512 {
					t.Fatalf("toggle not applied to next request, enabled=%t count=%d budget=%d", enabled, count, request.MaxTokens)
				}
				if strings.Contains(request.Messages[0].Content, "Before edits, write basis:") != enabled {
					t.Fatal("planning instructions survived the disabled toggle")
				}
			}
		})
	}
}

func putAutopilotPlanning(t *testing.T, s *Server, body []byte) {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, withController(httptest.NewRequest(http.MethodPut, "/api/modes/autopilot/preferences", strings.NewReader(string(body)))))
	if w.Code != http.StatusOK {
		t.Fatalf("save planning = %d: %s", w.Code, w.Body.String())
	}
}
