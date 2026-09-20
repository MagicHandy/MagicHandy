package transport

import (
	"testing"
	"time"
)

func TestIntifaceSamplingBudgetCoversUnadvertisedAndAdvertisedGaps(t *testing.T) {
	for _, test := range []struct{ gap, want time.Duration }{
		{0, 50 * time.Millisecond},
		{20 * time.Millisecond, 50 * time.Millisecond},
		{300 * time.Millisecond, 330 * time.Millisecond},
	} {
		owner := &Intiface{selected: true, selection: intifaceSelection{timingGap: test.gap}}
		if got := owner.MotionTimingCapabilities().MinimumPointInterval; got != test.want {
			t.Fatalf("gap %v: sampling interval %v, want %v", test.gap, got, test.want)
		}
	}
}
