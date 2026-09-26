package chat

import (
	"math"
	"slices"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Groove feels and plain words are vocabularies over one stroke score. Each
// maps to exact turns, so the model sees back the words it chose; a score from
// another vocabulary is described by the nearest words.

var grooveFeels = []string{"steady", "breathing", "wandering"}

// grooveTurns realizes a groove from where its strokes bottom out and turn
// back and how the stroke varies, with the variation it breathes by. Breathing
// lets the length swell and ease; wandering moves the whole stroke toward
// whichever end has more room, or contracts a stroke that already spans the
// slider.
func grooveTurns(bottomAt, topAt int, feel string) (motion.StrokeTurn, motion.StrokeTurn, int) {
	bottom := motion.StrokeTurn{AtPercent: bottomAt, VaryToPercent: bottomAt, Character: "steady"}
	top := motion.StrokeTurn{AtPercent: topAt, VaryToPercent: topAt, Character: "steady"}
	switch feel {
	case "breathing":
		amount := min(15, max(3, (topAt-bottomAt)/4))
		bottom.VaryToPercent, bottom.Character = min(90, bottomAt+amount), "drift"
		top.VaryToPercent, top.Character = max(10, topAt-amount), "drift"
		return bottom, top, 50
	case "wandering":
		shift := min(25, max(bottomAt, 100-topAt))
		if shift < 5 {
			amount := (topAt - bottomAt) / 3
			bottom.VaryToPercent, bottom.Character = min(90, bottomAt+amount), "drift"
			top.VaryToPercent, top.Character = max(10, topAt-amount), "drift"
			return bottom, top, 60
		}
		if 100-topAt < bottomAt {
			shift = -shift
		}
		bottom.VaryToPercent, bottom.Character = bottomAt+shift, "roam"
		top.VaryToPercent, top.Character = topAt+shift, "roam"
		return bottom, top, 60
	}
	return bottom, top, 20
}

// grooveOf describes a stroke score as a groove: where it bottoms out, where
// it turns back and its feel.
func grooveOf(spec motion.StrokeSpec) (int, int, string) {
	for _, feel := range grooveFeels {
		bottom, top, _ := grooveTurns(spec.Bottom.AtPercent, spec.Top.AtPercent, feel)
		if bottom == spec.Bottom && top == spec.Top {
			return spec.Bottom.AtPercent, spec.Top.AtPercent, feel
		}
	}
	feel := "steady"
	switch {
	case spec.Bottom.Character == "roam" || spec.Top.Character == "roam":
		feel = "wandering"
	case spec.Bottom.VaryToPercent != spec.Bottom.AtPercent || spec.Top.VaryToPercent != spec.Top.AtPercent:
		feel = "breathing"
	}
	return spec.Bottom.AtPercent, spec.Top.AtPercent, feel
}

// Plain words name each end of every stroke: depth is where it bottoms out
// and pull back is where it turns back toward the tip. Every combination is a
// valid stroke, so a depth word always takes effect on its own.
var (
	plainDepths      = []string{"tip", "upper", "middle", "lower", "base"}
	plainPullBacks   = []string{"tip", "upper", "middle", "lower"}
	plainPaces       = []string{"slowest", "slow", "medium", "fast", "fastest"}
	plainAccentWords = []string{"plunges", "full strokes", "tip flicks", "deep grinding", "pauses", "slow strokes"}
	// plainAccentMoves pairs each accent word with its motif.
	plainAccentMoves = []string{"plunge", "full", "tip_flicks", "deep_grind", "pause", "slow"}
)

var (
	plainDepthPoints    = map[string]int{"base": 0, "lower": 20, "middle": 40, "upper": 60, "tip": 80}
	plainPullBackPoints = map[string]int{"lower": 40, "middle": 60, "upper": 80, "tip": 100}
)

// plainMinimumTravel is the shortest stroke. A pull back at or below the depth
// turns back this far above it.
const plainMinimumTravel = 20

type plainWords struct {
	Depth      string   `json:"depth"`
	PullBack   string   `json:"pull_back"`
	Variety    string   `json:"variety"`
	Accents    []string `json:"accents"`
	AccentRate string   `json:"accent_rate"`
	Pace       string   `json:"pace"`
}

// plainTurns realizes depth, pull back and variety. A stroke that reaches the
// base or pulls back to the tip keeps that end still while it breathes, so
// every stroke still gets there.
func plainTurns(words plainWords) (motion.StrokeTurn, motion.StrokeTurn, int) {
	low := plainDepthPoints[words.Depth]
	high := min(100, max(plainPullBackPoints[words.PullBack], low+plainMinimumTravel))
	bottom, top, variation := grooveTurns(low, high, words.Variety)
	if words.Variety == "breathing" {
		if low == 0 {
			bottom = motion.StrokeTurn{AtPercent: 0, VaryToPercent: 0, Character: "steady"}
		}
		if high == 100 {
			top = motion.StrokeTurn{AtPercent: 100, VaryToPercent: 100, Character: "steady"}
		}
	}
	return bottom, top, variation
}

var plainPaceShares = map[string]float64{"slowest": 0, "slow": 0.25, "medium": 0.5, "fast": 0.75, "fastest": 1}

func plainSpeed(pace string, limits config.MotionSettings) int {
	span := float64(limits.SpeedMaxPercent - limits.SpeedMinPercent)
	return limits.SpeedMinPercent + int(math.Round(plainPaceShares[pace]*span))
}

func plainPaceOf(speed int, limits config.MotionSettings) string {
	best, nearest := math.MaxFloat64, "medium"
	for _, pace := range plainPaces {
		if distance := math.Abs(float64(plainSpeed(pace, limits) - speed)); distance < best {
			best, nearest = distance, pace
		}
	}
	return nearest
}

// plainWordsOf describes a stroke score by its nearest words. When several
// pull-back words give the same strokes, the one nearest the tip wins.
func plainWordsOf(spec motion.FlowSpec, limits config.MotionSettings) plainWords {
	k := spec.Strokes
	words := plainWords{Accents: []string{}, AccentRate: "sometimes", Pace: plainPaceOf(spec.SpeedPercent, limits)}
	best := math.MaxFloat64
	for _, depth := range plainDepths {
		for _, pullBack := range plainPullBacks {
			for _, variety := range grooveFeels {
				bottom, top, variation := plainTurns(plainWords{Depth: depth, PullBack: pullBack, Variety: variety})
				distance := turnDistance(bottom, k.Bottom) + turnDistance(top, k.Top) + math.Abs(float64(variation-k.VariationPercent))/10
				if distance < best {
					best, words.Depth, words.PullBack, words.Variety = distance, depth, pullBack, variety
				}
			}
		}
	}
	for _, accent := range k.Accents {
		if index := slices.Index(plainAccentMoves, accent.Move); index >= 0 {
			words.Accents = append(words.Accents, plainAccentWords[index])
		}
	}
	if len(k.Accents) > 0 {
		words.AccentRate = k.Accents[0].Rate
	}
	return words
}

func turnDistance(a, b motion.StrokeTurn) float64 {
	distance := math.Abs(float64(a.AtPercent-b.AtPercent)) + math.Abs(float64(a.VaryToPercent-b.VaryToPercent))
	if a.Character != b.Character {
		distance += 5
	}
	return distance
}
