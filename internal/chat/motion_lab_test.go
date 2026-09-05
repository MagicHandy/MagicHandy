package chat

import (
	"errors"
	"testing"
)

func TestMotionLabProposalRejectsActionsAndDoesNotRepair(t *testing.T) {
	for _, raw := range []string{
		`{"range_anchor_percent":0,"outbound_time_percent":35,"explanation":"test","action":"start"}`,
		`{"range_anchor_percent":0,"outbound_time_percent":0,"explanation":"test"}`,
		`{"range_anchor_percent":0,"explanation":"test"}`,
		`{"method":"directional","range_anchor_percent":0,"outbound_time_percent":35,"explanation":"test"}`,
		`{"method":"creative","range_anchor_percent":50,"outbound_time_percent":50,"explanation":"test"} {}`,
	} {
		provider := &scriptedProvider{responses: []string{raw}}
		if _, err := ProposeMotionLab(t.Context(), provider, "test", "Compare timing", `{}`); err == nil {
			t.Fatalf("accepted invalid proposal %s", raw)
		}
		if len(provider.requests) != 1 {
			t.Fatal("lab proposal silently retried")
		}
	}
	provider := &scriptedProvider{responses: []string{""}, responseErrors: []error{errors.New("offline")}}
	if _, err := ProposeMotionLab(t.Context(), provider, "test", "Compare timing", `{}`); err == nil {
		t.Fatal("provider error was hidden")
	}
}

func TestMotionLabProposalUsesIsolatedPrompt(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"range_anchor_percent":0,"outbound_time_percent":35,"explanation":"A held base return with a slower return leg."}`}}
	proposal, err := ProposeMotionLab(t.Context(), provider, "chosen-model", "Compare timing", `{"speed_percent":30}`)
	if err != nil || proposal.Method != "combined" {
		t.Fatalf("proposal=%+v err=%v", proposal, err)
	}
	request := provider.requests[0]
	if len(request.Messages) != 2 || request.Messages[0].Content != MotionLabPrompt ||
		request.Model != "chosen-model" || request.ReasoningMode != "off" {
		t.Fatalf("unexpected lab provider context: %+v", request)
	}
}
