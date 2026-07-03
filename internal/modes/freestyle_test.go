package modes

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func styleSettings(style string) config.MotionSettings {
	settings := config.DefaultSettings().Motion
	settings.Style = style
	return settings
}

func TestPlannerIsDeterministicForSeed(t *testing.T) {
	first := NewPlanner(42)
	second := NewPlanner(42)
	settings := styleSettings(config.MotionStyleBalanced)

	for index := range 12 {
		a, _ := first.NextSegment(settings)
		b, _ := second.NextSegment(settings)
		if a != b && (a.AreaFocus == nil) == (b.AreaFocus == nil) {
			// Segments with focus pointers compare by value below.
		}
		if a.PatternID != b.PatternID || a.SpeedPercent != b.SpeedPercent ||
			a.DurationMillis != b.DurationMillis || a.DriftToSpeedPercent != b.DriftToSpeedPercent {
			t.Fatalf("segment %d diverged for identical seeds: %+v vs %+v", index, a, b)
		}
	}
	if first.Seed() != 42 {
		t.Fatalf("seed = %d, want 42", first.Seed())
	}
}

func TestPlannerRecordsScoresAndAvoidsLongRepeats(t *testing.T) {
	for _, style := range []string{config.MotionStyleGentle, config.MotionStyleBalanced, config.MotionStyleIntense} {
		planner := NewPlanner(7)
		settings := styleSettings(style)
		var previous motion.PatternID
		repeats := 0
		for range 24 {
			segment, scores := planner.NextSegment(settings)
			if len(scores) != 3 {
				t.Fatalf("style %s: scores = %d entries, want 3", style, len(scores))
			}
			chosen := 0
			for _, score := range scores {
				if score.Chosen {
					chosen++
					if score.PatternIdentifier != string(segment.PatternID) {
						t.Fatalf("style %s: chosen score %q != segment %q", style, score.PatternIdentifier, segment.PatternID)
					}
				}
			}
			if chosen != 1 {
				t.Fatalf("style %s: chosen entries = %d, want exactly 1", style, chosen)
			}
			if segment.PatternID == previous {
				repeats++
				if repeats >= 3 {
					t.Fatalf("style %s: pattern %q repeated %d times consecutively", style, segment.PatternID, repeats+1)
				}
			} else {
				repeats = 0
			}
			previous = segment.PatternID
			if segment.DurationMillis < minSegmentMillis || segment.DurationMillis > maxSegmentMillis {
				t.Fatalf("style %s: duration %d outside bounds", style, segment.DurationMillis)
			}
		}
	}
}

func TestStyleScoringBiasesSpeedAndPace(t *testing.T) {
	gentle := NewPlanner(11)
	intense := NewPlanner(11)
	gentleSettings := styleSettings(config.MotionStyleGentle)
	intenseSettings := styleSettings(config.MotionStyleIntense)

	gentleSpeed, gentleDuration := 0, int64(0)
	intenseSpeed, intenseDuration := 0, int64(0)
	const samples = 32
	for range samples {
		segment, _ := gentle.NextSegment(gentleSettings)
		gentleSpeed += segment.SpeedPercent
		gentleDuration += segment.DurationMillis
		segment, _ = intense.NextSegment(intenseSettings)
		intenseSpeed += segment.SpeedPercent
		intenseDuration += segment.DurationMillis
	}
	if gentleSpeed >= intenseSpeed {
		t.Fatalf("gentle total speed %d should be below intense %d", gentleSpeed, intenseSpeed)
	}
	if gentleDuration <= intenseDuration {
		t.Fatalf("gentle segments should run longer: %d vs %d", gentleDuration, intenseDuration)
	}
}

func TestPlannerRespectsUserSpeedBand(t *testing.T) {
	planner := NewPlanner(3)
	settings := styleSettings(config.MotionStyleIntense)
	settings.SpeedMinPercent = 30
	settings.SpeedMaxPercent = 40
	for range 24 {
		segment, _ := planner.NextSegment(settings)
		if segment.SpeedPercent < 30 || segment.SpeedPercent > 40 {
			t.Fatalf("segment speed %d escaped the 30..40 band", segment.SpeedPercent)
		}
		if segment.DriftToSpeedPercent != 0 &&
			(segment.DriftToSpeedPercent < 30 || segment.DriftToSpeedPercent > 40) {
			t.Fatalf("drift speed %d escaped the 30..40 band", segment.DriftToSpeedPercent)
		}
	}
}

func TestNormalizeArrangementBoundsSegments(t *testing.T) {
	if _, err := NormalizeArrangement(Arrangement{}); err == nil {
		t.Fatal("empty arrangement accepted")
	}
	tooMany := Arrangement{Segments: make([]Segment, maxSegments+1)}
	if _, err := NormalizeArrangement(tooMany); err == nil {
		t.Fatal("oversized arrangement accepted")
	}
	arrangement, err := NormalizeArrangement(Arrangement{Segments: []Segment{
		{DurationMillis: 1, SpeedPercent: 500},
	}})
	if err != nil {
		t.Fatalf("NormalizeArrangement: %v", err)
	}
	segment := arrangement.Segments[0]
	if segment.DurationMillis != minSegmentMillis || segment.SpeedPercent != 100 || segment.PatternID != motion.PatternStroke {
		t.Fatalf("segment not normalized: %+v", segment)
	}
}
