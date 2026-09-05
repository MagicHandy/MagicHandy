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
		SecondsAtCurrentPhrase: 240, DecisionsAtCurrentPhrase: 6, ConsecutiveHolds: 3,
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
	if !strings.Contains(tracking, "motion phrase has remained materially unchanged for 4 minutes across 6 reconsiderations") {
		t.Fatalf("motion phrase age missing:\n%s", tracking)
	}
	if !strings.Contains(tracking, "last 3 motion outcomes were holds") {
		t.Fatalf("hold streak missing:\n%s", tracking)
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
	if !strings.Contains(arc, "clock and allowed range never move because of your response") {
		t.Fatalf("the arc must state that model output cannot move its clock or limits:\n%s", arc)
	}
	if strings.Contains(arc, "Set arc") {
		t.Fatalf("the prompt still lets the model accelerate buildup:\n%s", arc)
	}
	if !strings.Contains(arc, "variability") {
		t.Fatalf("the variability instruction is missing:\n%s", arc)
	}
}

func TestAutopilotSpeechMessageDoesNotExposePatternStorageID(t *testing.T) {
	message := AutopilotSpeechMessage(AutopilotContext{
		CurrentPatternID: "curated-intense-drive-16",
		CurrentSpeed:     40,
		CurrentArea:      AreaZoneFull,
		LastSay:          "I stay close in the quiet.",
	})
	if strings.Contains(message, "curated-intense-drive-16") {
		t.Fatalf("speech prompt leaked persisted pattern ID:\n%s", message)
	}
	if !strings.Contains(message, "40%") || !strings.Contains(message, "catalog pattern") {
		t.Fatalf("speech prompt lost useful motion context:\n%s", message)
	}
	for _, want := range []string{
		"Continue the same conversation naturally",
		"without repeating this line or restarting the session",
		"Do not invent a scene",
		"one specific current fact",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("speech prompt is missing anti-repetition guidance %q:\n%s", want, message)
		}
	}
}

func TestAutopilotContractKeepsPatternAndSpeedIndependent(t *testing.T) {
	contract := autopilotContract(AutopilotKindMotion, FullCapabilities())
	if !strings.Contains(contract, "pattern_id may omit speed_percent to preserve the live pace") {
		t.Fatalf("autopilot contract does not explain shape-only changes:\n%s", contract)
	}
	if strings.Contains(contract, "include \"pattern_id\" and \"speed_percent\" together") {
		t.Fatalf("autopilot contract still couples pattern and pace:\n%s", contract)
	}
}

func TestDynamicAutopilotContractAdvertisesEveryAcceptedCreativePhraseField(t *testing.T) {
	capabilities := FullCapabilities()
	capabilities.MotionMode = MotionModeDynamic
	contract := autopilotContract(AutopilotKindMotion, capabilities)
	for _, field := range []string{
		"span_min_percent", "span_profile", "variation_percent", "segment_seconds", "sections", "cycles",
	} {
		if !strings.Contains(contract, field) {
			t.Fatalf("dynamic Autopilot contract omitted %q:\n%s", field, contract)
		}
	}
	if !strings.Contains(contract, "Sections replace single-phrase geometry") {
		t.Fatalf("dynamic Autopilot contract does not explain section exclusivity:\n%s", contract)
	}
}

func TestDynamicAutopilotStartupPromptRequiresDynamicTarget(t *testing.T) {
	startup := AutopilotMotionMessage(AutopilotContext{
		MotionMode: MotionModeDynamic, SpeedMinPercent: 20, SpeedMaxPercent: 40,
		MotionMinSeconds: 20, MotionMaxSeconds: 60,
	})
	for _, want := range []string{
		"No Dynamic target is active",
		"use update with speed and either center/span or anchors",
		"none leaves Autopilot waiting",
	} {
		if !strings.Contains(startup, want) {
			t.Fatalf("Dynamic startup prompt missing %q:\n%s", want, startup)
		}
	}

	running := AutopilotMotionMessage(AutopilotContext{
		MotionMode: MotionModeDynamic, CurrentSpeed: 30, CurrentCenter: 50,
		CurrentSpan: 60, CurrentSpanMin: 26, CurrentSpanProfile: DynamicSpanProfileWander,
		CurrentVariation: 10, CurrentSegment: 37, SpeedMinPercent: 20,
		SpeedMaxPercent: 40, MotionMinSeconds: 20, MotionMaxSeconds: 60,
		MotionChangeLevel:    8,
		CommandedPositionMin: 8, CommandedPositionMax: 92,
		RecentPositionBands: []PositionBand{
			{Minimum: 28, Maximum: 72}, {Minimum: 26, Maximum: 74}, {Minimum: 27, Maximum: 73},
		},
		CommandedMeanTravel: 73, CommandedPeakSpeed: 119, MeanStrokeLength: 42,
		LocalStrokeCV: 11, LocalStrokeRange: 17,
	})
	if len(running) > maxUserMessageBytes {
		t.Fatalf("running Dynamic Autopilot prompt is %d bytes, limit %d", len(running), maxUserMessageBytes)
	}
	if strings.Contains(running, "No Dynamic target is active") {
		t.Fatalf("running Dynamic prompt still claims startup state:\n%s", running)
	}
	for _, want := range []string{
		`span floor 26%, span profile "wander"`,
		"37-second decision horizon",
		"span_min_percent (at least 20 and strictly below the widest span) and choose span_profile breathe, wander, or contrast",
		"preserving the current 60% widest span, its usable span_min_percent range is 20-59",
		"variation_percent adds smooth correlated center/rhythm texture",
		"Compiled feel: position band 8-92%, about 73% travel per second on average, 119%/s peak carriage velocity, 42% mean stroke length",
		"least varied 12-second window has 17% length range with 11% coefficient of variation",
		"measured from the engine curve, not inferred from the JSON fields",
		"Use 2-4 sections only for several distinct or evolving movement ideas",
		"Autopilot authorizes bounded choices without a new chat message",
		"not a mandatory change schedule",
		"frequent-contrast bias",
		"pace alone does not change range",
		"rotate contrast among pace, outer travel band, stroke-length envelope, texture, and sections",
		"Phrase age and compiled feel are the source of truth",
		"Position 0% is base and 100% is tip",
		"Broad motion approaches the full available position range",
		"mix localized and broad routes and sometimes approach either end",
		"Recent compiled position bands, oldest to newest: 28-72%, 26-74%, 27-73%",
		"observations of actual engine curves, not required targets",
		"without requiring endpoints, full range, or alternation on a schedule",
		"materially different phrases instead of nearby scalar edits",
	} {
		if !strings.Contains(running, want) {
			t.Fatalf("running Dynamic prompt missing %q:\n%s", want, running)
		}
	}
}

func TestDynamicAutopilotPromptFitsUserMessageBudgetWithTrackedSession(t *testing.T) {
	const reserveBytes = 256
	message := AutopilotMotionMessage(AutopilotContext{
		Style: "balanced", MotionMode: MotionModeDynamic,
		SpeedMinPercent: 20, SpeedMaxPercent: 80, CurrentSpeed: 58,
		CurrentCenter: 50, CurrentSpan: 74, CurrentSpanMin: 52,
		CurrentSpanProfile: DynamicSpanProfileBreathe, CurrentVariation: 25,
		CurrentSegment: 14, MotionMinSeconds: 8, MotionMaxSeconds: 16,
		MotionChangeLevel: 8, CommandedMeanTravel: 123, CommandedPeakSpeed: 201,
		MeanStrokeLength: 42, LocalStrokeCV: 7, LocalStrokeRange: 9,
		RecentPositionBands: []PositionBand{
			{Minimum: 24, Maximum: 72}, {Minimum: 27, Maximum: 74},
			{Minimum: 25, Maximum: 75}, {Minimum: 26, Maximum: 73},
		},
		SessionTracking: true, SessionSeconds: 180, SecondsAtCurrentSpeed: 90,
		SecondsAtCurrentPhrase: 90, DecisionsAtCurrentPhrase: 5, ConsecutiveHolds: 4,
	})
	if len(message) > maxUserMessageBytes-reserveBytes {
		t.Fatalf("tracked Dynamic Autopilot prompt is %d bytes, want at most %d to preserve %d bytes of state headroom",
			len(message), maxUserMessageBytes-reserveBytes, reserveBytes)
	}
}
