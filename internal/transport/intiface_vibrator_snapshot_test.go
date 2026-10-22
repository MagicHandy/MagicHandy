package transport

import "testing"

func TestVibratorStopSnapshotDoesNotAliasPacerState(t *testing.T) {
	v := &IntifaceVibrator{selection: vibratorSelection{selected: true, deviceIndex: 3,
		motors: []vibratorMotor{{IntifaceVibratorMotor: IntifaceVibratorMotor{Index: 2, StepCount: 20}, last: 4}}}}
	v.mu.Lock()
	selection := v.selectionSnapshotLocked()
	v.mu.Unlock()
	// A completed in-flight send updates the last level after Stop released mu.
	v.markSilent(3)
	if selection.motors[0].last != 4 || selection.motors[0].Index != 2 || v.selection.motors[0].last != 0 {
		t.Fatal("stop snapshot shares mutable motor state with the pacer")
	}
}
