package chat

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestLayeredFullGeometryPreservesPaceAndUsesTheRequestedBand(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(25)
	original := motion.CloneFlowSpec(&before)
	for _, band := range [][2]int{{0, 100}, {30, 70}, {80, 90}} {
		raw := fmt.Sprintf(`{"edits":{"geometry":"full_band","controls":{"min_percent":%d,"max_percent":%d}},"reply":"Full strokes."}`, band[0], band[1])
		_, next, _, err := ParseLayeredReply(raw, before, limits)
		if err != nil {
			t.Fatal(err)
		}
		if next.RangeFloorPercent != band[1]-band[0] || next.RangeCeilingPercent != band[1]-band[0] || next.SpeedPercent != before.SpeedPercent || next.PaceVariationPercent != before.PaceVariationPercent || next.Seed != before.Seed {
			t.Fatalf("full geometry changed pace or failed to fill band: %+v", next)
		}
		for _, layer := range next.Layers {
			if layer.Axis != "pace" || layer != before.Layers[1] {
				t.Fatalf("geometry retained location/width variation or changed pace: %+v", layer)
			}
		}
		if _, err := motion.FlowTarget(next, limits); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, *original) {
		t.Fatal("full geometry mutated its source")
	}
}

func TestLayeredFullSliderReplacesOldBand(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(25)
	_, entire, _, err := ParseLayeredReply(`{"edits":{"geometry":"full_slider"},"reply":"Use the whole slider."}`, before, limits)
	if err != nil || entire.MinPercent != 0 || entire.MaxPercent != 100 || entire.RangeFloorPercent != 100 || entire.RangeCeilingPercent != 100 || entire.SpeedPercent != before.SpeedPercent {
		t.Fatal("full slider did not replace the old 5..95 band while keeping speed", err)
	}
}

func TestLayeredFullGeometryRejectsConflictingEdits(t *testing.T) {
	limits := config.DefaultSettings().Motion
	before := DefaultLayeredScore(25)
	for _, raw := range []string{
		`{"edits":{"geometry":"full_band","stroke_width":{"min_percent":20,"max_percent":90}},"reply":"Conflicting width."}`,
		`{"edits":{"geometry":"full_band","layers":[{"axis":"center","amount_percent":40}]},"reply":"Conflicting center."}`,
		`{"edits":{"geometry":"full_slider","controls":{"min_percent":20}},"reply":"Conflicting outer bound."}`,
		`{"edits":{"geometry":"full_slider","controls":{"max_percent":80}},"reply":"Conflicting outer bound."}`,
	} {
		_, got, _, err := ParseLayeredReply(raw, before, limits)
		if err == nil || !reflect.DeepEqual(got, before) {
			t.Fatal("conflicting full geometry did not fail transactionally")
		}
	}
}
