package motion

import (
	"testing"
	"time"
)

func TestRefillLeadBudgetsDeliveryWithoutExtendingEditOrMediaLead(t *testing.T) {
	e := &Engine{dispatchInterval: 200 * time.Millisecond, minimumBufferedLeadMillis: 1500,
		minimumMediaBufferedLeadMillis: 10000, refillArrivalMarginMillis: 1500}
	for _, tc := range []struct{ latency, want int64 }{{0, 1700}, {150, 1850}, {750, 2000}} {
		e.latencyMillis = []int64{tc.latency}
		if got := e.refillLeadMillisLocked(); got != tc.want || e.leadMillisLocked() != 1500 {
			t.Fatalf("latency %d: refill %d, edit lead %d", tc.latency, got, e.leadMillisLocked())
		}
	}
	e.plan.Target.Media = &MediaTimelineDefinition{}
	if e.refillLeadMillisLocked() != 10000 {
		t.Fatal("interactive arrival margin changed fixed-media buffering")
	}
	e.plan.Target.Media = nil
	e.refillArrivalMarginMillis = 0
	if e.refillLeadMillisLocked() != 1500 {
		t.Fatal("owner without an arrival margin changed")
	}
}
