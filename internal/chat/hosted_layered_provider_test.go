package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Exercise strict nullable wire output through the real Responses adapter,
// omission restoration, hosted operations, lifecycle checks and domain edits.
func TestHostedLayeredResponsesRoundTrip(t *testing.T) {
	for _, plan := range []bool{false, true} {
		t.Run(fmt.Sprintf("plan_%t", plan), func(t *testing.T) {
			calls := 0
			wire := `{"proposal":{"action":"update","stay_unchanged":false,"reply":"Slower, with gradual changes.","edits":{"composition":"manual","controls":{"anchor_percent":{"op":"set","value":0},"speed_percent":{"op":"adjust","value":-5},"min_percent":null,"max_percent":null,"memory_cycles":null,"pace_variation_percent":null,"variation_mode":null},"layers":{"range":null,"center":null,"pace":{"op":"edit","value":{"period_cycles":{"op":"adjust","value":4},"amount_percent":null,"phase_percent":null,"shape":null}}},"stroke_width":null,"evolve":null}}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct {
					Text struct {
						Format struct {
							Strict bool            `json:"strict"`
							Schema json.RawMessage `json:"schema"`
						} `json:"format"`
					} `json:"text"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				schema := string(request.Text.Format.Schema)
				if !request.Text.Format.Strict || !strings.Contains(schema, `"proposal"`) || strings.Contains(schema, `"change_by"`) || strings.Contains(schema, `"oneOf"`) || !strings.Contains(schema, `"const":"adjust"`) {
					t.Error("wrong strict operation schema on the wire")
				}
				delta, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": wire})
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", delta)
			}))
			defer server.Close()
			provider, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{BaseURL: server.URL, Model: "test-model", PlanUsage: plan, Token: func(context.Context) (string, error) { return "test", nil }})
			if err != nil {
				t.Fatal(err)
			}
			current := DefaultLayeredScore(25)
			state := MotionContext{Running: true, Autopilot: true, Layered: &current, MotionMode: MotionModeLayered}
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true}
			service := Service{Provider: provider, Capabilities: &caps, MotionContext: &state}
			got, err := service.Complete(t.Context(), Request{Message: "Lower the pace by five, move the anchor to zero, and lengthen the pace cycle by four."}, nil)
			_, want, _, domainErr := ParseLayeredReply(`{"edits":{"controls":{"anchor_percent":0},"change_by":{"speed_percent":-5},"layers":[{"axis":"pace","period_change_cycles":4}]},"reply":"Test."}`, current, config.DefaultSettings().Motion)
			if err != nil || domainErr != nil || got.Response.Motion == nil || !reflect.DeepEqual(*got.Response.Motion.Layered, want) || calls != 1 || got.Repaired || got.SemanticFallback {
				t.Fatalf("round trip changed intent or added a call: result=%+v calls=%d err=%v domain=%v", got, calls, err, domainErr)
			}
			if !strings.Contains(got.Raw, `"op":"adjust"`) || strings.Contains(got.Raw, `"change_by"`) {
				t.Fatal("diagnostic output lost the model's original operation")
			}
		})
	}
}

func TestHostedLayeredReviewerUsesSameOperations(t *testing.T) {
	current := DefaultLayeredScore(25)
	provider := &layeredTestProvider{raw: `{"verdict":"correct","confidence":"high","reason":"wrong_pace","feedback":"Requested lower pace.","correction":{"edits":{"controls":{"speed_percent":{"op":"adjust","value":-5}}},"reply":"Pace corrected."}}`}
	service := MotionReviewService{Provider: provider, Capabilities: Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true}}
	for _, source := range []string{"chat", "autopilot"} {
		result, err := service.Complete(t.Context(), MotionReviewInput{Source: source, Request: "Slower.", Limits: config.DefaultSettings().Motion, Before: motion.MotionTarget{Flow: &current}, After: motion.MotionTarget{Flow: &current}})
		if err != nil || result.Motion == nil || result.Motion.Action != MotionActionUpdate || result.Motion.Layered.SpeedPercent != 20 {
			t.Fatalf("review correction failed: %+v %v", result, err)
		}
		if strings.Contains(string(provider.request.JSONSchema), `"change_by"`) || !strings.Contains(provider.request.Messages[0].Content, `"op":"adjust"`) {
			t.Fatal("reviewer advertises a different edit contract")
		}
	}
	provider.raw = `{"verdict":"correct","confidence":"high","reason":"wrong_pace","feedback":"Test.","correction":{"edits":{"controls":{"speed_percent":25},"change_by":{"speed_percent":-5}},"reply":"Test."}}`
	if result, err := service.Complete(t.Context(), MotionReviewInput{Source: "chat", Limits: config.DefaultSettings().Motion, After: motion.MotionTarget{Flow: &current}}); err == nil || result.Motion != nil {
		t.Fatal("reviewer accepted competing instructions")
	}
}
