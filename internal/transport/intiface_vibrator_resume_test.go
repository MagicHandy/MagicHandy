package transport

import (
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
	if !ok || !f.vibrator.Status().Suspended {
		t.Fatalf("Stop failed to suspend or offer bounded manual permission: %+v", f.vibrator.Status())
	}
	return f, permit
}

func TestVibratorManualResumeIsOneUseAndDoesNotPlay(t *testing.T) {
	f, permit := stoppedResumeFixture(t)
	before := countKind(f.server, "ScalarCmd")
	rollback, err := f.vibrator.ResumeForManualRequest(permit)
	if err != nil || f.vibrator.Status().Suspended || f.vibrator.Status().PlaybackState != "idle" {
		t.Fatalf("resume: %+v %v", f.vibrator.Status(), err)
	}
	if countKind(f.server, "ScalarCmd") != before || f.vibrator.ResumePermitCurrent(permit) {
		t.Fatal("lifting the suspension played output or left the old permit reusable")
	}
	rollback()
	if !f.vibrator.Status().Suspended {
		t.Fatal("a failed start did not suspend the vibrator again")
	}
	if _, err := f.vibrator.ResumeForManualRequest(permit); err != nil {
		t.Fatalf("the rolled-back permit should stay usable for the same stopped device: %v", err)
	}
}

func TestVibratorManualResumeRejectsNewerHumanOrDeviceChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*vibratorFixture)
	}{
		{"another Stop", func(f *vibratorFixture) { _ = f.hub.EmergencyStop(t.Context()) }},
		{"Resume now", func(f *vibratorFixture) { _ = f.vibrator.ResumeAt(f.vibrator.ControlGeneration()) }},
		{"reselection", func(f *vibratorFixture) { _ = f.vibrator.Select(t.Context(), 3) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, permit := stoppedResumeFixture(t)
			tc.change(f)
			if _, err := f.vibrator.ResumeForManualRequest(permit); err == nil {
				t.Fatal("a stale manual permit lifted the suspension")
			}
		})
	}
}

func TestVibratorRollbackSurvivesAModelLevelChange(t *testing.T) {
	f, permit := stoppedResumeFixture(t)
	rollback, err := f.vibrator.ResumeForManualRequest(permit)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.vibrator.SetModelLevel(.8, false); err != nil {
		t.Fatal(err)
	}
	rollback()
	if !f.vibrator.Status().Suspended {
		t.Fatal("a model level change prevented rollback after a failed start")
	}
}

func TestVibratorRollbackKeepsANewerHumanResume(t *testing.T) {
	f, permit := stoppedResumeFixture(t)
	rollback, err := f.vibrator.ResumeForManualRequest(permit)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.vibrator.ResumeAt(f.vibrator.ControlGeneration()); err != nil {
		t.Fatal(err)
	}
	rollback()
	if f.vibrator.Status().Suspended {
		t.Fatal("rollback undid a newer human resume")
	}
}
