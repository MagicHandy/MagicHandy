package modes

import (
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func arcSettings(enabled bool, tracking bool, minutes int) config.AutopilotSettings {
	settings := config.DefaultAutopilotSettings()
	settings.SessionArc = enabled
	settings.SessionTracking = tracking
	settings.SessionArcMinutes = minutes
	return settings
}

// The arc is off by default because it changes what the model is encouraged to
// do, unlike tracking which is inert context.
func TestSessionArcIsOffAndTrackingIsOnByDefault(t *testing.T) {
	defaults := config.DefaultAutopilotSettings()
	if defaults.SessionArc {
		t.Fatal("the session arc must be opt-in")
	}
	if !defaults.SessionTracking {
		t.Fatal("session tracking is inert context and should default on")
	}
}

// Disabled means absent, not zero: the model must not receive an arc it could
// reason about while the user has the bar switched off.
func TestDisabledArcReportsNothing(t *testing.T) {
	manager := swayTestManager(t, arcSettings(false, true, 30))
	manager.mu.Lock()
	percent := manager.arcPercentLocked(time.Unix(0, 0))
	manager.mu.Unlock()
	if percent != 0 {
		t.Fatalf("disabled arc reported %d%%", percent)
	}
	if snapshot := manager.SessionArcSnapshot(); snapshot.Enabled {
		t.Fatal("snapshot reports the arc enabled while the switch is off")
	}
}

// Time is the floor so a session that is simply left running still progresses.
func TestArcAdvancesWithElapsedTime(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 10))
	start := time.Unix(1000, 0)
	manager.mu.Lock()
	manager.arc.startedAt = start
	quarter := manager.arcPercentLocked(start.Add(150 * time.Second))
	full := manager.arcPercentLocked(start.Add(10 * time.Minute))
	over := manager.arcPercentLocked(start.Add(40 * time.Minute))
	manager.mu.Unlock()
	if quarter < 23 || quarter > 27 {
		t.Fatalf("quarter of the way = %d%%, want about 25", quarter)
	}
	if full != 100 {
		t.Fatalf("end of the arc = %d%%, want 100", full)
	}
	if over != 100 {
		t.Fatalf("past the end = %d%%, want it clamped to 100", over)
	}
}

// The model may ask, never write. One turn moves the bar by at most one clamped
// step, which is what stops an eager model sprinting it to full.
func TestArcNudgeIsClampedPerTurn(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 60))
	now := time.Unix(2000, 0)
	manager.mu.Lock()
	manager.arc.startedAt = now
	before := manager.arcPercentLocked(now)
	manager.applyArcIntentLocked(now, config.AutopilotArcAdvance)
	after := manager.arcPercentLocked(now)
	manager.mu.Unlock()
	if moved := after - before; moved > config.AutopilotArcNudgePercent {
		t.Fatalf("one advance moved the bar %d points, over the %d cap",
			moved, config.AutopilotArcNudgePercent)
	}
	if after <= before {
		t.Fatal("advance did not move the bar at all")
	}
}

// Winding down has to be expressible, or the bar is a ratchet.
func TestArcCanEaseBack(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 60))
	now := time.Unix(3000, 0)
	manager.mu.Lock()
	manager.arc.startedAt = now.Add(-30 * time.Minute)
	before := manager.arcPercentLocked(now)
	manager.applyArcIntentLocked(now, config.AutopilotArcEase)
	after := manager.arc.percent
	manager.mu.Unlock()
	if after >= before {
		t.Fatalf("ease moved the bar from %d%% to %d%%", before, after)
	}
}

func TestUnknownArcIntentHolds(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 60))
	now := time.Unix(4000, 0)
	manager.mu.Lock()
	manager.arc.startedAt = now
	before := manager.arcPercentLocked(now)
	manager.applyArcIntentLocked(now, "sprint")
	after := manager.arc.percent
	manager.mu.Unlock()
	if after != before {
		t.Fatalf("an unrecognized intent moved the bar from %d%% to %d%%", before, after)
	}
}

// An arc nudge must never reach the settings that own the limits. The bar is
// where in the band to aim, not how wide the band is.
func TestArcNudgeDoesNotTouchSpeedLimits(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 30))
	before := manager.options.Settings()
	now := time.Unix(5000, 0)
	manager.mu.Lock()
	for range 40 {
		manager.applyArcIntentLocked(now, config.AutopilotArcAdvance)
	}
	percent := manager.arc.percent
	manager.mu.Unlock()
	after := manager.options.Settings()
	if percent != 100 {
		t.Fatalf("repeated advances reached %d%%, want the bar pinned at 100", percent)
	}
	if before.SpeedMinPercent != after.SpeedMinPercent || before.SpeedMaxPercent != after.SpeedMaxPercent {
		t.Fatal("the arc changed the user's speed limits")
	}
}

// A bar the user cannot pull back is not really an override.
func TestUserCanPlaceAndResetTheArc(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 30))
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.mu.Unlock()

	manager.SetSessionArcPercent(70)
	if got := manager.SessionArcSnapshot().Percent; got < 68 || got > 72 {
		t.Fatalf("placed the bar at 70%% but it reads %d%%", got)
	}
	manager.ResetSessionArc()
	if got := manager.SessionArcSnapshot().Percent; got != 0 {
		t.Fatalf("reset left the bar at %d%%", got)
	}
}

// Start clears the arc for a fresh run, so accepting a placement while no session
// exists would store a value that is discarded a moment later. Found live: the
// endpoint returned 200 and reported 0%.
func TestArcPlacementIsRefusedWithoutASession(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 30))
	if manager.SetSessionArcPercent(55) {
		t.Fatal("placing the arc should be refused while Autopilot is not running")
	}
	if manager.ResetSessionArc() {
		t.Fatal("resetting the arc should be refused while Autopilot is not running")
	}
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.mu.Unlock()
	if !manager.SetSessionArcPercent(55) {
		t.Fatal("placing the arc should succeed once Autopilot is running")
	}
	if got := manager.SessionArcSnapshot().Percent; got < 53 || got > 57 {
		t.Fatalf("placed at 55%% but the snapshot reads %d%%", got)
	}
}
