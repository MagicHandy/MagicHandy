package transport

import (
	"context"
	"testing"
)

func stoppedResumeFixture(t *testing.T) (*vibratorFixture, VibratorResumePermit) {
	t.Helper()
	f := newVibratorFixture(t, nil)
	f.selectAndArm(.35, "speed")
	if err := f.hub.EmergencyStop(t.Context()); err != nil {
		t.Fatal(err)
	}
	permit, ok := f.vibrator.ManualResumePermit()
	if !ok || f.vibrator.Intensity() != 0 || f.vibrator.Status().ResumeIntensityPercent != 35 {
		t.Fatalf("Stop failed to silence or retain bounded manual permission: %+v", f.vibrator.Status())
	}
	return f, permit
}

func TestVibratorManualResumeIsOneUseAndDoesNotPlay(t *testing.T) {
	f, permit := stoppedResumeFixture(t)
	before := countKind(f.server, "ScalarCmd")
	rollback, err := f.vibrator.ResumeIntensityForManualRequest(permit)
	if err != nil || f.vibrator.Intensity() != .35 || f.vibrator.Status().PlaybackState != "idle" {
		t.Fatalf("resume: %+v %v", f.vibrator.Status(), err)
	}
	if countKind(f.server, "ScalarCmd") != before || f.vibrator.ResumePermitCurrent(permit) {
		t.Fatal("restoring the level played output or left the old permit reusable")
	}
	rollback()
	if f.vibrator.Intensity() != 0 || f.vibrator.Status().ResumeIntensityPercent != 35 {
		t.Fatal("failed-start rollback lost the previous human level")
	}
	if _, err := f.vibrator.ResumeIntensityForManualRequest(permit); err == nil {
		t.Fatal("rollback made an old request valid again")
	}
}

func TestVibratorManualResumeInvalidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*vibratorFixture)
		newLevel int
	}{
		{"second Stop", func(f *vibratorFixture) { _ = f.hub.EmergencyStop(context.Background()) }, 35},
		{"explicit zero", func(f *vibratorFixture) { f.vibrator.SetIntensity(0) }, 0},
		{"zero from stale control", func(f *vibratorFixture) { _, _ = f.vibrator.SetIntensityAt(0, 0, false) }, 0},
		{"lower ceiling", func(f *vibratorFixture) { c := f.vibrator.config; c.Ceiling = .2; f.vibrator.Configure(c) }, 20},
		{"reselect", func(f *vibratorFixture) { _ = f.vibrator.Select(context.Background(), 3) }, 0},
		{"release", func(f *vibratorFixture) { _ = f.vibrator.Release(context.Background()) }, 0},
		{"failure", func(f *vibratorFixture) {
			f.vibrator.mu.Lock()
			f.vibrator.invalidateLocked("stale")
			f.vibrator.mu.Unlock()
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, permit := stoppedResumeFixture(t)
			tc.change(f)
			if _, err := f.vibrator.ResumeIntensityForManualRequest(permit); err == nil || f.vibrator.Intensity() != 0 {
				t.Fatal("a stale request rearmed output")
			}
			if f.vibrator.Status().ResumeIntensityPercent != tc.newLevel {
				t.Fatalf("new permission = %+v", f.vibrator.Status())
			}
		})
	}
}

func TestVibratorManualResumeRollbackPreservesNewerHumanControls(t *testing.T) {
	f, permit := stoppedResumeFixture(t)
	rollback, err := f.vibrator.ResumeIntensityForManualRequest(permit)
	if err != nil {
		t.Fatal(err)
	}
	f.vibrator.SetIntensity(.2)
	rollback()
	if f.vibrator.Intensity() != .2 || f.vibrator.Status().ResumeIntensityPercent != 0 {
		t.Fatal("rollback overwrote a newer human choice")
	}
}

func TestVibratorManualResumeCannotCrossStopWriterBarrier(t *testing.T) {
	f, permit := stoppedResumeFixture(t)
	f.hub.session.writeMu.Lock()
	result := make(chan error, 1)
	go func() { _, err := f.vibrator.ResumeIntensityForManualRequest(permit); result <- err }()
	f.vibrator.invalidateForEmergency()
	f.hub.session.writeMu.Unlock()
	if err := <-result; err == nil || f.vibrator.Intensity() != 0 {
		t.Fatal("request waiting at the writer barrier crossed Stop")
	}
}
