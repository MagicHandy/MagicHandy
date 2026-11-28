package chat

import (
	"errors"
	"strings"
	"testing"
)

func TestEdgePlanStrictContract(t *testing.T) {
	state := EdgeContext{PreviousSpeed: 60, HoldingSpeed: 20, SpeedMin: 5, SpeedMax: 60, RangeMin: 25, RangeMax: 65}
	for _, raw := range []string{
		`{"hold_seconds":0,"resume_speed_percent":30,"resume_style":"gentle"}`,
		`{"hold_seconds":20,"resume_speed_percent":61,"resume_style":"previous"}`,
		`{"hold_seconds":20,"resume_speed_percent":30,"resume_style":"start"}`,
		`{"hold_seconds":20,"resume_speed_percent":30,"resume_style":"gentle","transport":{}}`,
		`{"hold_seconds":20,"resume_speed_percent":30,"resume_style":"gentle"} {}`,
		`{"hold_seconds":20,"resume_speed_percent":30}`,
		`I cannot help with that request.`,
	} {
		provider := &scriptedProvider{responses: []string{raw}}
		if _, _, err := (EdgeService{Provider: provider}).Complete(t.Context(), state); err == nil {
			t.Fatalf("accepted %s", raw)
		}
		if len(provider.requests) != 1 {
			t.Fatal("hold errors must not start repair loops")
		}
	}
	provider := &scriptedProvider{responses: []string{`{"hold_seconds":37,"resume_speed_percent":25,"resume_style":"previous"}`}}
	plan, _, err := (EdgeService{Provider: provider}).Complete(t.Context(), state)
	if err != nil || plan.HoldSeconds != 37 || plan.ResumeSpeed != 25 {
		t.Fatalf("%+v %v", plan, err)
	}
	request := provider.requests[0]
	if len(request.Messages) != 2 || request.MaxTokens > 256 || !strings.Contains(request.Messages[0].Content, "adult device") {
		t.Fatal("expected a compact, truthful technical contract")
	}
	refusal := errors.New("provider refusal")
	provider = &scriptedProvider{responses: []string{""}, responseErrors: []error{refusal}}
	if _, _, err := (EdgeService{Provider: provider}).Complete(t.Context(), state); !errors.Is(err, refusal) {
		t.Fatalf("lost refusal: %v", err)
	}
}

func TestEdgeFeedbackLeavesAcceptedDurationAndReturnUntouched(t *testing.T) {
	raw := `{"hold_seconds":45,"resume_speed_percent":35,"resume_style":"previous"}`
	provider := &scriptedProvider{responses: []string{raw, raw}}
	service := EdgeService{Provider: provider, Feedback: &EdgeFeedback{Observation: EdgeObservation{Kind: "moving", PlannedSeconds: 45, ReturnPace: 35}, Remarks: []string{"That timing worked. Keep choosing what fits."}}}
	for range 2 {
		plan, _, err := service.Complete(t.Context(), EdgeContext{PreviousSpeed: 50, HoldingSpeed: 20, SpeedMin: 20, SpeedMax: 50})
		if err != nil || plan != (EdgePlan{HoldSeconds: 45, ResumeSpeed: 35, ResumeStyle: "previous"}) {
			t.Fatalf("altered model choice %+v %v", plan, err)
		}
	}
}

func TestEdgeChatAcknowledgementReflectsSelectedPlanner(t *testing.T) {
	state := MotionContext{EdgePlannerReadsFeedback: true}
	if state.edgeInstructions() != "" {
		t.Fatal("injected a button cue before the first press")
	}
	state.RecentEdge = &EdgeObservation{Kind: "moving", HoldingPace: 22}
	if prompt := state.edgeInstructions(); !strings.Contains(prompt, "NEXT press") || !strings.Contains(prompt, `"holding_pace_percent":22`) {
		t.Fatal("missing future feedback or actual holding pace")
	}
	state.EdgePlannerReadsFeedback = false
	if prompt := state.edgeInstructions(); !strings.Contains(prompt, "mechanical state only") || !strings.Contains(prompt, `"holding_pace_percent":22`) {
		t.Fatal("chat must still know the observed action and its planner's limits")
	}
}
