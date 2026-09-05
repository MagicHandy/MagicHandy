package chat

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLLMLabPreservesUnmentionedAxesAndRejectsInvalidScores(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	current := motion.DefaultFlowSpec()
	_, next, changed, err := ParseLLMLab(`{"reply":"Tip reference.","controls":{"anchor_percent":100}}`, "controls", current, limits)
	if err != nil || next.AnchorPercent != 100 || next.SpeedPercent != current.SpeedPercent || next.RangeFloorPercent != current.RangeFloorPercent || len(changed) != 1 {
		t.Fatalf("partial update: %+v %v %v", next, changed, err)
	}
	for _, raw := range []string{
		`{"reply":"Move now","action":"start"}`,
		`{"reply":"Fast","controls":{"speed_percent":44}}`,
		`{"reply":"Too narrow","controls":{"min_percent":90}}`,
		`{"reply":"Seed","controls":{"seed":2}}`,
		`{"reply":"Null","controls":{"speed_percent":null}}`,
		`{"reply":"Unknown","layers":[{"axis":"position","amount_percent":20,"period_cycles":8,"phase_percent":0}]}`,
		`{"reply":"Sequence","steps":[{"min_percent":0,"max_percent":100,"speed_percent":25,"cycles":4}]}`,
		`{"reply":"Okay"} {"reply":"Again"}`,
	} {
		if _, _, _, err := ParseLLMLab(raw, "controls", current, limits); err == nil {
			t.Fatalf("accepted unsupported result: %s", raw)
		}
	}
}

func TestLLMLabRequiresCompleteSequenceAndLayerItems(t *testing.T) {
	limits := config.DefaultSettings().Motion
	current := motion.DefaultFlowSpec()
	for _, test := range []struct{ method, raw string }{
		{"sequence", `{"reply":"Incomplete","steps":[{"max_percent":100,"speed_percent":25,"cycles":4}]}`},
		{"sequence", `{"reply":"Clear","steps":null}`},
		{"layers", `{"reply":"Incomplete","layers":[{"axis":"range","amount_percent":20,"period_cycles":8}]}`},
		{"layers", `{"reply":"Clear","layers":null}`},
	} {
		if _, _, _, err := ParseLLMLab(test.raw, test.method, current, limits); err == nil {
			t.Fatalf("accepted incomplete %s result: %s", test.method, test.raw)
		}
	}
	_, _, changed, err := ParseLLMLab(`{"reply":"No layers","layers":[]}`, "layers", current, limits)
	if err != nil || len(changed) != 0 {
		t.Fatalf("empty stack should preserve an empty score: %v %v", changed, err)
	}
	current.Layers = []motion.FlowLayer{{Axis: "range", AmountPercent: 20, PeriodCycles: 8}}
	_, next, changed, err := ParseLLMLab(`{"reply":"Clear layers","layers":[]}`, "layers", current, limits)
	if err != nil || len(next.Layers) != 0 || len(changed) != 1 || changed[0] != "layers" {
		t.Fatalf("explicit clearing lost: %+v %v %v", next, changed, err)
	}
}
