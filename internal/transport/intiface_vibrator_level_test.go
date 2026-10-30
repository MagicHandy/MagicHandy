package transport

import (
	"context"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/vibration"
)

// playSteady starts a covered steady run with model control on at Max .4.
func (f *vibratorFixture) playSteady(frame *VibrationFrame) {
	f.t.Helper()
	run := f.selectAndArm(.4, vibration.FollowSteady)
	f.vibrator.Configure(VibratorConfig{Profile: vibration.Profile{Follow: vibration.FollowSteady, DepthEnd: vibration.DepthBase}, Ceiling: .4, ModelControl: true})
	command := strokeCommand("level", 20_000)
	command.Vibration = frame
	if _, err := run.AppendPoints(context.Background(), command); err != nil {
		f.t.Fatal(err)
	}
	if _, err := run.Play(context.Background(), PlayCommand{StreamID: command.StreamID}); err != nil {
		f.t.Fatal(err)
	}
	// Past the two-second fade-in.
	f.pace(3 * time.Second)
}

func TestVibratorModelLevelIsAShareOfMax(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.playSteady(nil)
	// Before the model chooses, the level follows The Handy; with no
	// commanded speed that is Max.
	if status := f.vibrator.Status(); lastScalars(t, f.server)[0] != .4 || status.LevelSource != "handy" || status.ModelLevelPercent != 100 {
		t.Fatalf("before a choice: %v %+v", lastScalars(t, f.server), status)
	}
	// An Advanced change applies at once.
	if err := f.vibrator.SetModelLevel(.5, false); err != nil {
		t.Fatal(err)
	}
	f.pace(0)
	if scalars := lastScalars(t, f.server); scalars[0] != .2 {
		t.Fatalf("half level = %v, want 50%% of the 40%% Max", scalars)
	}
	if status := f.vibrator.Status(); !status.ModelControl || status.ModelLevelPercent != 50 || status.LevelSource != "model" || status.CeilingPercent != 40 {
		t.Fatalf("status = %+v", status)
	}
	// Model control off follows at Max whatever the model chose.
	if err := f.vibrator.SetModelLevel(.25, false); err != nil {
		t.Fatal(err)
	}
	f.vibrator.Configure(VibratorConfig{Profile: vibration.Profile{Follow: vibration.FollowSteady, DepthEnd: vibration.DepthBase}, Ceiling: .4})
	f.pace(100 * time.Millisecond)
	if scalars := lastScalars(t, f.server); scalars[0] != .4 {
		t.Fatalf("manual control = %v, want Max", scalars)
	}
}

func TestVibratorSimpleLevelChangeGlides(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.playSteady(nil)
	if err := f.vibrator.SetModelLevel(.5, false); err != nil {
		t.Fatal(err)
	}
	f.pace(100 * time.Millisecond)
	if err := f.vibrator.SetModelLevel(1, true); err != nil {
		t.Fatal(err)
	}
	// Half the range takes 750 ms: after 400 ms the level is still between.
	f.pace(400 * time.Millisecond)
	between := lastScalars(t, f.server)[0]
	if between <= .2 || between >= .4 {
		t.Fatalf("mid-glide scalar %v, want between .2 and .4", between)
	}
	f.pace(400 * time.Millisecond)
	if scalars := lastScalars(t, f.server); scalars[0] != .4 {
		t.Fatalf("after the glide: %v", scalars)
	}
	// Stop cuts a glide short.
	if err := f.vibrator.SetModelLevel(0, true); err != nil {
		t.Fatal(err)
	}
	stops := countKind(f.server, "StopAllDevices")
	if err := f.hub.EmergencyStop(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.pace(100 * time.Millisecond)
	if status := f.vibrator.Status(); !status.Suspended || status.LevelPercent != 0 || countKind(f.server, "StopAllDevices") != stops+1 {
		t.Fatalf("after Stop: %+v", status)
	}
}

func TestVibratorLevelZeroSilencesOnlyTheVibrator(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.playSteady(nil)
	if err := f.vibrator.SetModelLevel(0, false); err != nil {
		t.Fatal(err)
	}
	f.pace(100 * time.Millisecond)
	status := f.vibrator.Status()
	if scalars := lastScalars(t, f.server); scalars[0] != 0 || status.SilentReason != "level_zero" || status.PlaybackState != "playing" {
		t.Fatalf("level 0: scalars=%v status=%+v", scalars, status)
	}
}

func TestVibratorZeroLevelGlideDoesNotReportSilenceBeforeZeroOutput(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.playSteady(nil)
	if err := f.vibrator.SetModelLevel(0, true); err != nil {
		t.Fatal(err)
	}
	f.pace(100 * time.Millisecond)
	if status := f.vibrator.Status(); status.LevelPercent <= 0 || status.SilentReason != "" {
		t.Fatalf("positive glide output was reported silent: %+v", status)
	}
	f.pace(1500 * time.Millisecond)
	if status := f.vibrator.Status(); status.LevelPercent != 0 || status.SilentReason != "level_zero" {
		t.Fatalf("completed zero glide was not silent: %+v", status)
	}
}

func TestVibratorAdvancedFollowRangeAndEnvelopeUseMax(t *testing.T) {
	f := newVibratorFixture(t, nil)
	// Steady drive is 1, so a 25..75% follow range reaches 75% of Max.
	f.playSteady(&VibrationFrame{Revision: 1, Follow: true, FollowMinPercent: 25, FollowMaxPercent: 75})
	if scalars := lastScalars(t, f.server); scalars[0] != .3 {
		t.Fatalf("follow range = %v, want 75%% of the 40%% Max", scalars)
	}
	// All zeros follow at the owner's level.
	if err := f.vibrator.SetModelLevel(.5, false); err != nil {
		t.Fatal(err)
	}
	command := AppendPointsCommand{StreamID: "level", Vibration: &VibrationFrame{Revision: 2, Follow: true}}
	run, err := f.vibrator.MotionTransport()
	if err != nil {
		t.Fatal(err)
	}
	_ = run
	f.vibrator.mu.Lock()
	f.vibrator.applyVibrationFrameLocked(command.Vibration, f.vibrator.stream.vibration)
	f.vibrator.mu.Unlock()
	f.pace(100 * time.Millisecond)
	if scalars := lastScalars(t, f.server); scalars[0] != .2 {
		t.Fatalf("plain follow = %v, want the 50%% level", scalars)
	}
}

func TestVibratorRejectsAnInvalidFollowRange(t *testing.T) {
	for _, frame := range []*VibrationFrame{
		{Revision: 1, Follow: true, FollowMinPercent: 60, FollowMaxPercent: 40},
		{Revision: 1, Follow: true, FollowMaxPercent: 101},
		{Revision: 1, FollowMaxPercent: 50, Points: []TimedLevel{{TimeMillis: 0, LevelPercent: 10}, {TimeMillis: 100, LevelPercent: 10}}},
	} {
		if _, err := vibrationFrameTimeline(frame); err == nil {
			t.Fatalf("accepted %+v", frame)
		}
	}
}

func TestVibratorManualControlOverridesAnExistingModelPattern(t *testing.T) {
	for _, frame := range []*VibrationFrame{
		{Revision: 1, Points: []TimedLevel{{TimeMillis: 0, LevelPercent: 25}, {TimeMillis: 20000, LevelPercent: 25}}},
		{Revision: 1, Follow: true, FollowMaxPercent: 25},
	} {
		t.Run(map[bool]string{true: "follow range", false: "independent"}[frame.Follow], func(t *testing.T) {
			f := newVibratorFixture(t, nil)
			f.playSteady(frame)
			if got := lastScalars(t, f.server)[0]; got != .1 {
				t.Fatalf("model pattern = %v", got)
			}
			f.vibrator.Configure(VibratorConfig{Profile: vibration.Profile{Follow: vibration.FollowSteady}, Ceiling: .4, FollowOnly: true})
			f.pace(100 * time.Millisecond)
			if got := lastScalars(t, f.server)[0]; got != .4 {
				t.Fatalf("manual following still used the model pattern: %v", got)
			}
			if err := f.hub.EmergencyStop(t.Context()); err != nil {
				t.Fatal(err)
			}
			f.vibrator.Configure(VibratorConfig{Profile: vibration.Profile{Follow: vibration.FollowSteady}, Ceiling: .4, ModelControl: true})
			f.pace(100 * time.Millisecond)
			if !f.vibrator.Status().Suspended || f.vibrator.Status().LevelPercent != 0 {
				t.Fatal("changing model control revived output after Stop")
			}
		})
	}
}

func TestVibratorReportsThePreviousLevelWhileOff(t *testing.T) {
	f := newVibratorFixture(t, nil)
	f.playSteady(nil)
	if status := f.vibrator.Status(); status.PreviousLevelPercent != 0 {
		t.Fatalf("a running level reported a previous one: %+v", status)
	}
	if err := f.vibrator.SetModelLevel(.6, false); err != nil {
		t.Fatal(err)
	}
	if err := f.vibrator.SetModelLevel(0, false); err != nil {
		t.Fatal(err)
	}
	if status := f.vibrator.Status(); status.ModelLevelPercent != 0 || status.PreviousLevelPercent != 60 {
		t.Fatalf("off from 60: %+v", status)
	}
	if err := f.vibrator.SetModelLevel(.6, true); err != nil || f.vibrator.Status().PreviousLevelPercent != 0 {
		t.Fatalf("back on: %v %+v", err, f.vibrator.Status())
	}
}

func TestVibratorFollowsTheHandysSpeedUntilTheModelChooses(t *testing.T) {
	f := newVibratorFixture(t, nil)
	run := f.selectAndArm(.4, vibration.FollowSteady)
	f.vibrator.Configure(VibratorConfig{Profile: vibration.Profile{Follow: vibration.FollowSteady, DepthEnd: vibration.DepthBase},
		Ceiling: .4, ModelControl: true, SpeedMinPercent: 20, SpeedMaxPercent: 80})
	command := strokeCommand("handy", 20_000)
	command.SpeedPercent = 20
	if _, err := run.AppendPoints(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Play(context.Background(), PlayCommand{StreamID: command.StreamID}); err != nil {
		t.Fatal(err)
	}
	f.pace(3 * time.Second)
	// The slowest saved speed is 20% of the level: .08, step .1 of a 20-step motor.
	if scalars := lastScalars(t, f.server); scalars[0] != .1 {
		t.Fatalf("slowest speed = %v", scalars)
	}
	faster := AppendPointsCommand{StreamID: command.StreamID, Points: []TimedPoint{{TimeMillis: 20_000, PositionPercent: 50}}, SpeedPercent: 80}
	if _, err := run.AppendPoints(context.Background(), faster); err != nil {
		t.Fatal(err)
	}
	f.pace(100 * time.Millisecond)
	if scalars := lastScalars(t, f.server); scalars[0] != .4 {
		t.Fatalf("fastest speed = %v", scalars)
	}
	// Once the model chooses, the speed no longer sets the level.
	if err := f.vibrator.SetModelLevel(.25, false); err != nil {
		t.Fatal(err)
	}
	f.pace(100 * time.Millisecond)
	if scalars := lastScalars(t, f.server); scalars[0] != .1 {
		t.Fatalf("chosen level = %v", scalars)
	}
}
