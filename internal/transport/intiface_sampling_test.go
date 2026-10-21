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
		owner := &Intiface{intifaceSession: &intifaceSession{}, selected: true, selection: intifaceSelection{timingGap: test.gap}}
		if got := owner.MotionTimingCapabilities().MinimumPointInterval; got != test.want {
			t.Fatalf("gap %v: sampling interval %v, want %v", test.gap, got, test.want)
		}
	}
}

func TestIntifaceHandyPreferenceIsIndependentOfHardTiming(t *testing.T) {
	for _, name := range []string{"The Handy (FW4+)", "Other linear device", "The Handy"} {
		for _, gap := range []uint32{100, 300} {
			owner := &Intiface{intifaceSession: &intifaceSession{devices: map[uint32]IntifaceDevice{7: {
				DeviceIndex: 7, DeviceName: name, DeviceMessageTimingGapMillis: gap,
				LinearActuators: []IntifaceLinearActuator{{Index: 0, StepCount: 100}},
			}}}}
			selection, err := owner.prepareSelectionLocked(7, 0)
			if err != nil {
				t.Fatal(err)
			}
			owner.selection, owner.selected = selection, true
			wantPreference := time.Duration(0)
			if name == "The Handy (FW4+)" {
				wantPreference = 180 * time.Millisecond
			}
			if got := owner.MotionSamplingCapabilities().PreferredPointInterval; got != wantPreference {
				t.Fatalf("%s preference %v, want %v", name, got, wantPreference)
			}
			wantFloor := time.Duration(gap+gap/10) * time.Millisecond
			if got := owner.MotionTimingCapabilities().MinimumPointInterval; got != wantFloor {
				t.Fatalf("preference changed the hard timing floor: %v, want %v", got, wantFloor)
			}
		}
	}
}
