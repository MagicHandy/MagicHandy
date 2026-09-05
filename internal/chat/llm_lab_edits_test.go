package chat

import (
	"encoding/json"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLabEditsPreserveLayersAndRelativeSectionPace(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	current := motion.DefaultFlowSpec()
	current.Steps = []motion.FlowStep{{MinPercent: 0, MaxPercent: 100, SpeedPercent: 20, Cycles: 4}, {MinPercent: 60, MaxPercent: 100, SpeedPercent: 35, Cycles: 2}}
	current.Layers = []motion.FlowLayer{{Axis: "range", AmountPercent: 40, PeriodCycles: 8}, {Axis: "center", AmountPercent: 30, PeriodCycles: 16, PhasePercent: 25}}
	before, _ := json.Marshal(current)
	_, next, _, err := ParseLLMLab(`{"reply":"Slower, with the range layer preserved.","change_by":{"speed_percent":-5},"layers":[{"axis":"pace","amount_percent":20,"period_cycles":12,"phase_percent":0}],"remove_layers":["center"]}`, "edits", current, limits)
	if err != nil {
		t.Fatal(err)
	}
	if next.SpeedPercent != 20 || next.Steps[0].SpeedPercent != 15 || next.Steps[1].SpeedPercent != 30 || len(next.Layers) != 2 || next.Layers[0] != current.Layers[0] || next.Layers[1].Axis != "pace" {
		t.Fatalf("lost preserved state: %+v", next)
	}
	after, _ := json.Marshal(current)
	if string(before) != string(after) {
		t.Fatal("editing mutated the original score")
	}
	_, unchanged, changed, err := ParseLLMLab(`{"reply":"Preserved.","layers":[]}`, "edits", current, limits)
	if err != nil || len(changed) != 0 || len(unchanged.Layers) != 2 {
		t.Fatal("an empty edit silently cleared layers")
	}
}

func TestLabEditsRejectAmbiguityAndOutOfLimitResults(t *testing.T) {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 43
	current := motion.DefaultFlowSpec()
	for _, raw := range []string{
		`{"reply":"Conflict","controls":{"speed_percent":20},"change_by":{"speed_percent":-5}}`,
		`{"reply":"Too fast","change_by":{"speed_percent":30}}`,
		`{"reply":"Fraction","change_by":{"speed_percent":-2.5}}`,
		`{"reply":"Null","change_by":{"speed_percent":null}}`,
		`{"reply":"Seed","change_by":{"seed":1}}`,
		`{"reply":"Name","change_by":{"variation_mode":1}}`,
		`{"reply":"Conflict","layers":[{"axis":"range","amount_percent":40,"period_cycles":8,"phase_percent":0}],"remove_layers":["range"]}`,
		`{"reply":"Incomplete","layers":[{"axis":"pace","amount_percent":20}]}`,
		`{"reply":"Null","remove_layers":null}`,
		`{"reply":"Duplicate","remove_layers":["pace","pace"]}`,
		`{"reply":"Move","action":"start"}`,
	} {
		if _, _, _, err := ParseLLMLab(raw, "edits", current, limits); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	_, next, _, err := ParseLLMLab(`{"reply":"A smaller widest span.","change_by":{"range_ceiling_percent":-10},"controls":{"variation_mode":"drift","turn_softness_percent":70,"cadence_hold_percent":100}}`, "edits", current, limits)
	if err != nil || next.RangeCeilingPercent != 80 || next.VariationMode != "drift" {
		t.Fatalf("relative default ceiling: %+v %v", next, err)
	}
}
