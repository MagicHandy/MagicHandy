//go:build liveeval

package chat

import (
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Synthetic Edge-button requests through production adapters. The baseline
// uses precisely the same mechanical state with the prior technical prompt.
func TestLiveEdgeFeedback(t *testing.T) {
	if os.Getenv("MAGICHANDY_EDGE_FEEDBACK_EVAL") == "" {
		t.Skip("set MAGICHANDY_EDGE_FEEDBACK_EVAL to evaluate Edge feedback")
	}
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	var provider llm.Provider
	if os.Getenv("MAGICHANDY_LIVE_LLAMA_URL") == "" {
		provider, model = hostedLayeredLiveProvider(t)
	} else {
		var err error
		provider, err = llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{BaseURL: liveEvalLlamaURL, Model: model, Timeout: 90 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
	}
	state := EdgeContext{PreviousSpeed: 50, HoldingSpeed: 20, SpeedMin: 20, SpeedMax: 50, RangeMin: 0, RangeMax: 100}
	earlier := EdgeObservation{Kind: "moving", HoldingPace: 20, PlannedSeconds: 45, ReturnPace: 35, ReturnStyle: "previous"}
	cases := []struct {
		name    string
		remarks []string
		want    func(EdgePlan) bool
	}{
		{"no_feedback", nil, func(EdgePlan) bool { return true }},
		{"close", []string{"That was too close."}, func(p EdgePlan) bool { return p.HoldSeconds > 45 && p.ResumeSpeed < 35 && p.ResumeStyle == "gentle" }},
		{"specific", []string{"Next Edge, give me 92 seconds and return at 22 percent."}, func(p EdgePlan) bool { return p.HoldSeconds == 92 && p.ResumeSpeed == 22 }},
		{"correction", []string{"Next time wait 90 seconds.", "Actually, next hold only 14 seconds, then return to my earlier pattern at 40 percent."}, func(p EdgePlan) bool {
			return p.HoldSeconds == 14 && p.ResumeSpeed == 40 && p.ResumeStyle == "previous"
		}},
		{"unrelated", []string{"The concert was too short. My train takes 137 seconds to cross the bridge."}, func(p EdgePlan) bool { return p.HoldSeconds != 137 }},
		{"recovery", []string{"I hadn't settled down before you picked back up. Give me more breathing room on the next one and ease back in."}, func(p EdgePlan) bool { return p.HoldSeconds > 45 && p.ResumeSpeed < 35 && p.ResumeStyle == "gentle" }},
		{"mixed", []string{"At the next button press, wait 76 seconds and come back at 24 percent.", "What pace did the last press use?", "Blue and yellow make green."}, func(p EdgePlan) bool { return p.HoldSeconds == 76 && p.ResumeSpeed == 24 }},
		{"other_topic", []string{"My coffee steeps for 113 seconds. That lid was too tight."}, func(p EdgePlan) bool { return p.HoldSeconds != 113 }},
	}
	var rows []map[string]any
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for repeat := 0; repeat < 3; repeat++ {
		for _, tc := range cases {
			service := EdgeService{Provider: provider, Model: model, ReasoningMode: "off"}
			if len(tc.remarks) > 0 && os.Getenv("MAGICHANDY_EDGE_BASELINE") == "" {
				service.Feedback = &EdgeFeedback{Observation: earlier, Remarks: tc.remarks}
			}
			start := time.Now()
			plan, raw, err := service.Complete(t.Context(), state)
			valid := err == nil
			pass := valid && tc.want(plan)
			rows = append(rows, map[string]any{"model": model, "wording": tc.name, "context": state, "plan": plan, "raw": raw, "valid": valid, "matched": pass, "elapsed_ms": time.Since(start).Milliseconds()})
			t.Logf("%s %d: %d seconds / %d%% / %s, matched=%t", tc.name, repeat+1, plan.HoldSeconds, plan.ResumeSpeed, plan.ResumeStyle, pass)
			if !valid {
				t.Errorf("%s: %v", tc.name, err)
			}
			if !pass && os.Getenv("MAGICHANDY_EDGE_REQUIRE_QUALITY") != "" {
				t.Errorf("quality miss for %s", tc.name)
			}
		}
	}
}
