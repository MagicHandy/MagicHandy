package chat

import (
	"strings"
	"testing"
)

func TestSessionFactsAndArcAppearOnlyWhenEnabled(t *testing.T) {
	off := AutopilotMotionMessage(AutopilotContext{
		Style: "balanced", SpeedMinPercent: 20, SpeedMaxPercent: 40,
		SessionSeconds: 900, SecondsAtCurrentSpeed: 200, SpeedTrend: "rising",
		ArcPercent: 60,
	})
	for _, fragment := range []string{"Session so far", "Session buildup", "minutes", "rising"} {
		if strings.Contains(off, fragment) {
			t.Fatalf("tracking off still leaked %q into the prompt:\n%s", fragment, off)
		}
	}

	tracking := AutopilotMotionMessage(AutopilotContext{
		Style: "balanced", SpeedMinPercent: 20, SpeedMaxPercent: 40,
		SessionTracking: true, SessionSeconds: 900, SecondsAtCurrentSpeed: 200, SpeedTrend: "rising",
		ArcPercent: 60,
	})
	if !strings.Contains(tracking, "Session so far: 15 minutes") {
		t.Fatalf("elapsed session time missing:\n%s", tracking)
	}
	if !strings.Contains(tracking, "held for 3 minutes") {
		t.Fatalf("time at current speed missing:\n%s", tracking)
	}
	if !strings.Contains(tracking, "Speed has been rising") {
		t.Fatalf("speed trend missing:\n%s", tracking)
	}
	if strings.Contains(tracking, "Session buildup") {
		t.Fatalf("the arc leaked in while its own switch is off:\n%s", tracking)
	}

	arc := AutopilotMotionMessage(AutopilotContext{
		Style: "balanced", SpeedMinPercent: 20, SpeedMaxPercent: 40,
		SessionTracking: true, SessionSeconds: 900, ArcEnabled: true, ArcPercent: 60,
	})
	if !strings.Contains(arc, "Session buildup: 60%") {
		t.Fatalf("arc percent missing:\n%s", arc)
	}
	if !strings.Contains(arc, "allowed range itself never moves") {
		t.Fatalf("the arc must state that limits do not move:\n%s", arc)
	}
	if !strings.Contains(arc, "variability") {
		t.Fatalf("the variability instruction is missing:\n%s", arc)
	}
}
