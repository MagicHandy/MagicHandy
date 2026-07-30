package modes

import (
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// The whole reason PR #158 removed intra-segment variation was retarget churn:
// roughly 4-9 engine retargets per minute. Restoring texture is only defensible
// if the combined rate stays under that, so this measures it rather than
// asserting it, and prints the table the docs quote.
//
// One boundary per segment plus the waypoints that segment earns is the whole
// budget. Holds cost nothing, which is what buys room for the waypoints.
func TestCombinedRetargetRateStaysUnderThePreChangeChurn(t *testing.T) {
	const preChangeWorstCase = 9.0

	cases := []struct {
		label    string
		cadence  string
		shortest time.Duration
		longest  time.Duration
	}{
		{"Dynamic", config.AutopilotMotionDynamic, 10 * time.Second, 35 * time.Second},
		{"Natural", config.AutopilotMotionNatural, 20 * time.Second, 60 * time.Second},
		{"Steady", config.AutopilotMotionSteady, 45 * time.Second, 120 * time.Second},
	}

	for _, testCase := range cases {
		settings := config.DefaultAutopilotSettings()
		settings.MotionCadence = testCase.cadence
		manager := swayTestManager(t, settings)

		for _, variability := range []VariabilityPreference{
			VariabilitySettled, VariabilityNormal, VariabilityRestless,
		} {
			// The shortest segment in the window is the worst case: the boundary
			// cost is amortized over the least time.
			for _, duration := range []time.Duration{testCase.shortest, testCase.longest} {
				manager.mu.Lock()
				waypoints := manager.swayAllowanceLocked(duration, variability)
				manager.mu.Unlock()
				changes := float64(1 + waypoints)
				perMinute := changes / duration.Minutes()
				t.Logf("%-8s %-8s %5s segment: %d boundary + %d sway = %.1f retargets/min",
					testCase.label, variability, duration, 1, waypoints, perMinute)
				if perMinute > preChangeWorstCase {
					t.Fatalf("%s/%s at %s reaches %.1f retargets/min, over the %.0f the old loop produced",
						testCase.label, variability, duration, perMinute, preChangeWorstCase)
				}
			}
		}
	}
}

// Dynamic already changes target constantly, so it must not also get waypoints at
// the short end — that is where the budget would blow out. The coupling is
// deliberate: fast cadence is already varied, slow cadence is what needs texture.
func TestFastCadenceGetsTextureOnlyWhereThereIsRoom(t *testing.T) {
	settings := config.DefaultAutopilotSettings()
	settings.MotionCadence = config.AutopilotMotionDynamic
	manager := swayTestManager(t, settings)

	manager.mu.Lock()
	shortest := manager.swayAllowanceLocked(10*time.Second, VariabilityRestless)
	longest := manager.swayAllowanceLocked(35*time.Second, VariabilityRestless)
	manager.mu.Unlock()

	if shortest != 0 {
		t.Fatalf("a 10s Dynamic segment earned %d waypoints, want none", shortest)
	}
	if longest == 0 {
		t.Fatal("a 35s segment has room and should earn at least one waypoint")
	}
}

// Steady is the preset most at risk of feeling static, since a target can hold for
// two minutes. It must be the one that earns the most texture.
func TestSteadyCadenceEarnsTheMostTexture(t *testing.T) {
	rates := map[string]int{}
	for _, cadence := range []string{
		config.AutopilotMotionDynamic,
		config.AutopilotMotionNatural,
		config.AutopilotMotionSteady,
	} {
		settings := config.DefaultAutopilotSettings()
		settings.MotionCadence = cadence
		manager := swayTestManager(t, settings)
		minimum, maximum := settings.MotionWindow()
		middle := time.Duration((minimum+maximum)/2) * time.Second
		manager.mu.Lock()
		rates[cadence] = manager.swayAllowanceLocked(middle, VariabilityRestless)
		manager.mu.Unlock()
	}
	if rates[config.AutopilotMotionSteady] < rates[config.AutopilotMotionDynamic] {
		t.Fatalf("Steady earned %d waypoints and Dynamic %d; the slow preset needs the most texture",
			rates[config.AutopilotMotionSteady], rates[config.AutopilotMotionDynamic])
	}
}
