package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestCreativeV2ReportsTheRejectedFieldWithoutChangingTheScore(t *testing.T) {
	before := FreshCreativeV2Score(25)
	limits := config.DefaultSettings().Motion
	for _, tc := range []struct {
		name, raw, problem string
	}{
		{"missing edits", `{"reply":"Hello."}`, "edits must be a non-null array"},
		{"null edits", `{"edits":null,"reply":"Hello."}`, "edits must be a non-null array"},
		{"too many edits", `{"edits":[{},{},{},{},{},{},{},{},{}],"reply":"Hello."}`, "edits contains 9 items; at most 8 are allowed"},
		{"missing reply", `{"edits":[]}`, "reply must contain non-whitespace text"},
		{"null reply", `{"edits":[],"reply":null}`, "reply must contain non-whitespace text"},
		{"empty reply", `{"edits":[],"reply":""}`, "reply must contain non-whitespace text"},
		{"blank reply with an edit", `{"edits":[{"speed_percent":30}],"reply":" \n\t "}`, "reply must contain non-whitespace text"},
		{"long reply with an edit", `{"edits":[{"speed_percent":30}],"reply":"` + strings.Repeat("x", 12001) + `"}`, "reply is 12001 bytes; at most 12000 are allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, after, changed, err := ParseCreativeV2Reply(tc.raw, before, limits)
			if err == nil || !strings.Contains(err.Error(), tc.problem) {
				t.Fatalf("rejection = %v, want %q", err, tc.problem)
			}
			if !reflect.DeepEqual(before, after) || len(changed) != 0 {
				t.Fatal("rejected reply changed the score")
			}
		})
	}
	// The existing limit is bytes, including for non-ASCII reply text.
	_, _, _, err := ParseCreativeV2Reply(`{"edits":[],"reply":"`+strings.Repeat("é", 6001)+`"}`, before, limits)
	if err == nil || !strings.Contains(err.Error(), "reply is 12002 bytes") {
		t.Fatalf("UTF-8 reply limit changed: %v", err)
	}
	for _, reply := range []string{"Hello.", strings.Repeat("x", 12000)} {
		_, after, changed, err := ParseCreativeV2Reply(`{"edits":[],"reply":"`+reply+`"}`, before, limits)
		if err != nil || !reflect.DeepEqual(before, after) || len(changed) != 0 {
			t.Fatalf("valid no-change reply was rejected or changed motion: %v", err)
		}
	}
}

func TestCreativeV2GuidedOutputRequiresAReplyInEveryActionBranch(t *testing.T) {
	limits := config.DefaultSettings().Motion
	for _, mood := range []bool{false, true} {
		base := CreativeV2ResponseSchema(limits, mood)
		var root struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(base, &root); err != nil {
			t.Fatal(err)
		}
		checkCreativeV2ReplySchema(t, root.Properties["reply"])
		for _, state := range []MotionContext{{}, {Running: true}, {Paused: true}, {Running: true, Autopilot: true}} {
			state.MotionMode = MotionModeCreativeV2
			// Earlier-score recall and live authority must retain reply bounds.
			var live struct {
				OneOf []struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"oneOf"`
			}
			if err := json.Unmarshal(continuousActionSchema(withRecallSchema(base, MotionModeCreativeV2, 1), state), &live); err != nil {
				t.Fatal(err)
			}
			for _, branch := range live.OneOf {
				checkCreativeV2ReplySchema(t, branch.Properties["reply"])
			}
		}
	}
}

func checkCreativeV2ReplySchema(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var reply struct {
		Type      string `json:"type"`
		MinLength int    `json:"minLength"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil || reply.Type != "string" || reply.MinLength != 1 {
		t.Fatalf("guided output permits an empty reply: %s (%v)", raw, err)
	}
}
