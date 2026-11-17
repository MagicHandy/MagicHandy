package diagnostics

import "testing"

func TestTraceOwnsAllTargetSlicesAcrossReads(t *testing.T) {
	ring := NewTraceRing(2)
	input := MotionTraceRow{Target: &MotionTraceTarget{DynamicAnchors: []string{"a"}, PaceLimiters: []string{"speed"}}}
	input.Retarget = &MotionTraceRetarget{PreviousTarget: input.Target, NextTarget: input.Target}
	returned := ring.Add(input)
	ring.Record(input)
	input.Target.DynamicAnchors[0] = "input"
	returned.Target.DynamicAnchors[0] = "return"
	returned.Retarget.PreviousTarget.PaceLimiters[0] = "return"
	exported := ring.Export()
	exported.Rows[0].Target.DynamicAnchors[0] = "export"
	exported.Rows[1].Retarget.NextTarget.DynamicAnchors[0] = "export"
	for _, row := range ring.Rows() {
		for _, target := range []*MotionTraceTarget{row.Target, row.Retarget.PreviousTarget, row.Retarget.NextTarget} {
			if target.DynamicAnchors[0] != "a" || target.PaceLimiters[0] != "speed" {
				t.Fatal("trace target aliases another owner")
			}
		}
	}
}
