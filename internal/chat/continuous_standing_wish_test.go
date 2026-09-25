package chat

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// Only a live chat turn reads a standing wish. The host, not the parser,
// decides what to do with it.
func TestLiveContinuousChatDeclaresStandingWishes(t *testing.T) {
	limits := config.DefaultSettings().Motion
	creative := FreshCreativeV2Score(25)
	response, _, _, err := ParseCreativeV2Reply(`{"action":"none","stay_unchanged":true,"edits":[],"reply":"Keeping it."}`, creative, limits)
	if err != nil || response.StayUnchanged == nil || !*response.StayUnchanged {
		t.Fatalf("creative v2 wish: %+v %v", response, err)
	}
	layered := DefaultLayeredScore(25)
	response, _, _, err = ParseLayeredReply(`{"action":"none","stay_unchanged":false,"edits":{},"reply":"Changing it up."}`, layered, limits)
	if err != nil || response.StayUnchanged == nil || *response.StayUnchanged {
		t.Fatalf("layered wish: %+v %v", response, err)
	}
	response, _, _, err = ParseLayeredReply(`{"action":"none","edits":{},"reply":"Mm."}`, layered, limits)
	if err != nil || response.StayUnchanged != nil {
		t.Fatal("a reply without the field declared a standing wish")
	}
}

// The wish is offered only while continuous Autopilot composes the motion, the
// one time it has an effect. There it is required, so the grammar asks for it
// before the reply instead of as a trailing field the model tended to skip.
func TestStandingWishIsOfferedOnlyWhileAutopilotComposes(t *testing.T) {
	creative := FreshCreativeV2Score(25)
	cases := []struct {
		name            string
		trusted         bool
		state           MotionContext
		offered         bool
		autopilotStatus string
	}{
		{name: "chat without Autopilot", state: MotionContext{Running: true}},
		{name: "Autopilot composing", state: MotionContext{Running: true, Autopilot: true}, offered: true,
			autopilotStatus: `"autopilot":"composing between chat turns"`},
		{name: "Autopilot holding", state: MotionContext{Running: true, Autopilot: true, StandingHold: true}, offered: true,
			autopilotStatus: `"autopilot":"holding the motion unchanged at the user's request"`},
		// Autopilot planning cannot declare or clear a human's wish.
		{name: "planning turn", trusted: true, state: MotionContext{Running: true, Autopilot: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &layeredTestProvider{raw: `{"action":"none","edits":[],"reply":"Hi."}`}
			if tc.offered {
				provider.raw = `{"action":"none","edits":[],"stay_unchanged":false,"reply":"Hi."}`
			}
			if tc.trusted {
				provider.raw = `{"edits":[],"reply":"Holding."}`
			}
			state := tc.state
			state.Layered = &creative
			service := Service{Provider: provider, TrustedMotionInput: tc.trusted, MotionContext: &state,
				Capabilities: &Capabilities{Motion: true, MotionMode: MotionModeCreativeV2}}
			if _, err := service.Complete(t.Context(), Request{Message: "Hello"}, nil); err != nil {
				t.Fatal(err)
			}
			var schema struct {
				OneOf []struct {
					Properties map[string]json.RawMessage `json:"properties"`
					Required   []string                   `json:"required"`
				} `json:"oneOf"`
			}
			_ = json.Unmarshal(provider.request.JSONSchema, &schema)
			for _, branch := range schema.OneOf {
				_, offered := branch.Properties["stay_unchanged"]
				if offered != tc.offered || slices.Contains(branch.Required, "stay_unchanged") != tc.offered {
					t.Fatalf("wish offered=%t required=%v", offered, branch.Required)
				}
			}
			system := provider.request.Messages[0].Content
			if strings.Contains(system, "AUTOPILOT WISH") != tc.offered || !strings.Contains(system, tc.autopilotStatus) {
				t.Fatalf("Autopilot state was not described as expected:\n%s", system)
			}
		})
	}
}
