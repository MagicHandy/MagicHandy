package chat

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestHostedPaceFollowingUsesResultingGeometry(t *testing.T) {
	before := DefaultLayeredScore(37)
	for _, relation := range []string{"same", "opposite"} {
		raw := fmt.Sprintf(`{"edits":{"composition":"geometry","geometry":"alternate_ends","layers":{"pace":{"op":"follow","value":{"axis":"center","relation":%q,"amount_percent":100,"low_rate_percent":19}}}},"reply":"Test."}`, relation)
		_, after, _, err := parseHostedLayeredReply(raw, before, config.DefaultSettings().Motion)
		if err != nil {
			t.Fatal(err)
		}
		var center, pace motion.FlowLayer
		for _, layer := range after.Layers {
			if layer.Axis == "center" {
				center = layer
			}
			if layer.Axis == "pace" {
				pace = layer
			}
		}
		if pace.PeriodCycles != center.PeriodCycles || pace.PhasePercent != center.PhasePercent || pace.Shape != center.Shape || pace.Inverted != (relation == "opposite") || pace.LowRatePercent != 19 || after.SpeedPercent != before.SpeedPercent {
			t.Fatal("explicit timing relationship lost the resulting geometry or changed base pace")
		}
	}
}

func TestPaceFollowingResolvesAfterAllLayerEdits(t *testing.T) {
	for _, order := range []string{
		`[{"axis":"pace","follow":{"axis":"range","relation":"opposite"}},{"axis":"range","amount_percent":100,"period_cycles":17,"phase_percent":23,"shape":"wave","inverted":true}]`,
		`[{"axis":"range","amount_percent":100,"period_cycles":17,"phase_percent":23,"shape":"wave","inverted":true},{"axis":"pace","follow":{"axis":"range","relation":"opposite"}}]`,
	} {
		_, after, _, err := ParseLayeredReply(`{"edits":{"layers":`+order+`},"reply":"Test."}`, DefaultLayeredScore(37), config.DefaultSettings().Motion)
		if err != nil {
			t.Fatal(err)
		}
		for _, layer := range after.Layers {
			if layer.Axis == "pace" && (layer.Shape != "wave" || layer.PeriodCycles != 17 || layer.PhasePercent != 23 || layer.Inverted) {
				t.Fatal("follow depended on field order or lost source polarity")
			}
		}
	}
}

func TestPaceFollowingRejectsMissingSourceAndCompetingTiming(t *testing.T) {
	before := DefaultLayeredScore(37)
	for _, edits := range []string{
		`{"layers":[{"axis":"pace","follow":{"axis":"range","relation":"same"}}]}`,
		`{"layers":[{"axis":"pace","follow":{"axis":"center","relation":"same"}}]}`,
		`{"geometry":"alternate_ends","layers":[{"axis":"pace","follow":{"axis":"center","relation":"same"},"phase_percent":25}]}`,
		`{"geometry":"alternate_ends","layers":[{"axis":"pace","follow":{"axis":"center","relation":"invalid"}}]}`,
		`{"geometry":"alternate_ends","layers":[{"axis":"center","follow":{"axis":"pace","relation":"same"}}]}`,
	} {
		_, after, _, err := ParseLayeredReply(`{"edits":`+edits+`,"reply":"Test."}`, before, config.DefaultSettings().Motion)
		if err == nil || !reflect.DeepEqual(after, before) {
			t.Fatal("ambiguous timing relationship was not rejected atomically")
		}
	}
}
