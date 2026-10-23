package motion

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestPhraseNarrowingDoesNotAccelerateToDeviceLimit(t *testing.T) {
	for _, profile := range []string{config.HandyModelOriginal, config.HandyModel2Standard, config.HandyModel2Pro} {
		for _, speed := range []int{7, 46, 100} {
			for _, anchor := range []int{0, 100} {
				t.Run(fmt.Sprintf("%s/%d/%d", profile, speed, anchor), func(t *testing.T) {
					settings := config.DefaultSettings().Motion
					settings.HandyModel, settings.SpeedMinPercent, settings.SpeedMaxPercent = profile, 1, speed
					s := phraseFixture(anchor, 70, 2, speed)
					if anchor == 100 {
						s.Phrase.Strokes[0].BottomPercent = 30
					}
					s.PaceVariationPercent = 0
					a, err := FlowTarget(s, settings)
					if err != nil {
						t.Fatal(err)
					}
					previous := NewMotionPlan("before", a, settings, 0, 0, time.Unix(0, 0))
					if anchor == 100 {
						s.Phrase.Strokes[0].BottomPercent = 84
					} else {
						s.Phrase.Strokes[0].TopPercent = 16
					}
					b, err := FlowTarget(s, settings)
					if err != nil {
						t.Fatal(err)
					}
					for leg := 1; leg <= 4; leg++ {
						for _, fraction := range []float64{.1, .4, .7, .9} {
							left, right := previous.curve.authoredKnots[leg-1].TimeMillis, previous.curve.authoredKnots[leg].TimeMillis
							at := int64((float64(left) + fraction*float64(right-left)) * float64(previous.PeriodMillis) / float64(previous.curve.duration))
							next := previous.Retarget("after", b, settings, at, time.Unix(0, 0))
							peak := max(previous.curve.maximumVelocityPerMillis()*float64(previous.curve.duration)/float64(previous.PeriodMillis), next.curve.maximumVelocityPerMillis()*float64(next.curve.duration)/float64(next.PeriodMillis))
							bridge := layeredPaceTransition(previous, nil, &next, at)
							if bridge == nil {
								t.Fatalf("lost monotone handoff at leg%d/%v", leg, fraction)
							}
							if bridge.stroke.maximumVelocity() > peak*1.00001 {
								t.Fatalf("narrowing accelerated beyond either selected plan: %f > %f at leg%d/%v", bridge.stroke.maximumVelocity()*1000, peak*1000, leg, fraction)
							}
							assertLayeredSplice(t, previous, next, bridge, settings)
						}
					}
				})
			}
		}
	}
}

func TestPhraseSlowSweepMirrorsBothTravelDirections(t *testing.T) {
	for _, anchor := range []int{0, 100} {
		s := phraseFixture(anchor, 28, 3, 46)
		if anchor == 100 {
			s.Phrase.Strokes[0].BottomPercent = 72
		}
		s.PaceVariationPercent = 0
		s.Phrase.Strokes[1].RatePercent = 40
		legs, durations := phraseLegs(s, config.HandyModelOriginal)
		fullTimes := []int64{}
		for i, leg := range legs[:8] {
			if math.Abs(leg.to-leg.from) == 100 {
				fullTimes = append(fullTimes, durations[i])
			}
		}
		if len(fullTimes) != 2 || fullTimes[0] != fullTimes[1] {
			t.Fatalf("full sweep rates asymmetric for anchor%d: %v", anchor, fullTimes)
		}
	}
}

func TestPhraseHorizonContainsWholeMotifsWithoutShrinking(t *testing.T) {
	for _, count := range []int{3, 17, 22, 24} {
		s := phraseFixture(100, 70, 1, 30)
		s.Phrase.Strokes = nil
		for remaining := count; remaining > 0; remaining -= min(8, remaining) {
			s.Phrase.Strokes = append(s.Phrase.Strokes, PhraseStroke{BottomPercent: 70, TopPercent: 100, Count: min(8, remaining), RatePercent: 100})
		}
		legs, _ := phraseLegs(s, config.HandyModelOriginal)
		cycles := len(legs) / 2
		if cycles < 64 || cycles >= 64+count || cycles%count != 0 {
			t.Fatalf("motif %d has wrong horizon %d", count, cycles)
		}
	}
}
