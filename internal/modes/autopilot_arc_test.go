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
		t.Fatal("session buildup must be opt-in")
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
	manager := swayTestManager(t, arcSettings(true, true, 1))
	start := time.Unix(1000, 0)
	manager.mu.Lock()
	manager.arc.startedAt = start
	quarter := manager.arcPercentLocked(start.Add(15 * time.Second))
	full := manager.arcPercentLocked(start.Add(time.Minute))
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

func TestThirtyMinuteArcHonorsConfiguredDuration(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 30))
	start := time.Unix(1500, 0)
	manager.mu.Lock()
	manager.arc.startedAt = start
	quarter := manager.arcPercentLocked(start.Add(7*time.Minute + 30*time.Second))
	half := manager.arcPercentLocked(start.Add(15 * time.Minute))
	full := manager.arcPercentLocked(start.Add(30 * time.Minute))
	manager.mu.Unlock()
	if quarter != 25 || half != 50 || full != 100 {
		t.Fatalf("30-minute buildup = %d/%d/%d%%, want 25/50/100%%", quarter, half, full)
	}
}

func TestMaximumArcDurationDoesNotOverflow(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, config.AutopilotMaximumArcMinutes))
	start := time.Unix(0, 0)
	duration := time.Duration(config.AutopilotMaximumArcMinutes) * time.Minute
	manager.mu.Lock()
	manager.arc.startedAt = start
	half := manager.arcPercentLocked(start.Add(duration / 2))
	manager.mu.Unlock()
	if half < 49 || half > 50 {
		t.Fatalf("maximum-duration midpoint = %d%%, want about 50%%", half)
	}
}

// The configured duration is authoritative. Planning churn must not accelerate
// the bar: the model may react to visible progress, but cannot rewrite it.
func TestAcceptedDecisionsDoNotAccelerateBuildup(t *testing.T) {
	manager := swayTestManager(t, arcSettings(true, true, 60))
	now := manager.options.Now()
	manager.mu.Lock()
	manager.mode = ModeAutopilot
	manager.generation = 7
	manager.segment = Segment{PatternID: "steady", SpeedPercent: 30}
	manager.arc.startedAt = now.Add(-15 * time.Minute)
	manager.mu.Unlock()

	before := manager.SessionArcSnapshot().Percent
	for range 20 {
		choice := segmentChoice{
			segment: Segment{PatternID: "steady", SpeedPercent: 30},
			source:  "model", timing: TimingSoon, variability: VariabilityRestless,
		}
		if !manager.armAutopilotChoice(ModeAutopilot, &choice, 7) {
			t.Fatal("current decision was not admitted")
		}
	}
	after := manager.SessionArcSnapshot().Percent
	if before != 25 || after != before {
		t.Fatalf("decision churn moved buildup from %d%% to %d%%, want elapsed-time value 25%%", before, after)
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
