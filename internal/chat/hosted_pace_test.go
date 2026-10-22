package chat

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestHostedPaceFloorEditsAreAtomicAndExplicit(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(37)
	for _, floor := range []int{1, 23, 100, 0} {
		raw := fmt.Sprintf(`{"edits":{"composition":"manual","layers":{"pace":{"op":"edit","value":{"low_rate_percent":%d}}}},"reply":"Pace contrast changed."}`, floor)
		_, after, _, err := parseHostedLayeredReply(raw, before, limits)
		if err != nil {
			t.Fatal(err)
		}
		want := *motion.CloneFlowSpec(&before)
		want.Layers[1].LowRatePercent = floor
		if !reflect.DeepEqual(after, want) || before.Layers[1].LowRatePercent != 0 {
			t.Fatal("rate floor edit changed unrelated controls or aliased the score")
		}
	}
	for _, edit := range []string{
		`{"pace":{"op":"edit","value":{"low_rate_percent":-1}}}`,
		`{"pace":{"op":"edit","value":{"low_rate_percent":101}}}`,
		`{"center":{"op":"edit","value":{"low_rate_percent":20}}}`,
		`{"range":{"op":"edit","value":{"low_rate_percent":20}}}`,
	} {
		_, after, _, err := parseHostedLayeredReply(`{"edits":{"composition":"manual","layers":`+edit+`},"reply":"Test."}`, before, limits)
		if err == nil || !reflect.DeepEqual(after, before) {
			t.Fatal("invalid floor edit was not atomic")
		}
	}
}

func TestHostedPaceGuidanceKeepsLocalContractAndSchema(t *testing.T) {
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		caps := Capabilities{Motion: true, MotionMode: mode}
		local := contractInstructions(caps)
		want := layeredContract
		if mode == MotionModeCreativeV2 {
			want = creativeV2ReachGuide + "\n\n" + creativeV2Contract
		}
		if local != want {
			t.Fatal("local prompt changed")
		}
		caps.HostedMotion = true
		hosted := contractInstructions(caps)
		if strings.Count(hosted, hostedPaceGuide) != 1 {
			t.Fatal("missing or duplicated pace guidance")
		}
		if strings.Contains(hosted, hostedLayeredCoordinationGuide) != (mode == MotionModeLayered) {
			t.Fatal("coordination guide advertised for the wrong mode")
		}
	}
	limits := config.DefaultSettings().Motion
	if strings.Contains(string(LayeredResponseSchema(limits, false)), "low_rate_percent") {
		t.Fatal("local grammar gained hosted fields")
	}
	var root map[string]any
	_ = json.Unmarshal(layeredResponseSchema(limits, false, true), &root)
	choices := root["properties"].(map[string]any)["edits"].(map[string]any)["oneOf"].([]any)
	manual := choices[1].(map[string]any)["properties"].(map[string]any)
	axes := manual["layers"].(map[string]any)["properties"].(map[string]any)
	for _, axis := range []string{"center", "range", "pace"} {
		encoded, _ := json.Marshal(axes[axis])
		if strings.Contains(string(encoded), "low_rate_percent") != (axis == "pace") {
			t.Fatal("rate floor advertised outside the pace layer")
		}
	}
}
