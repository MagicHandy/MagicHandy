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
