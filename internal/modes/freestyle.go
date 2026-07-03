package modes

import (
	"math/rand"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// styleProfile is the deterministic scoring input for one motion style. The
// saved style biases planner scoring directly — it is never prompt-only bias.
type styleProfile struct {
	patternWeights map[motion.PatternID]float64
	// speedBias positions segment speeds inside the user's speed band:
	// 0 hugs the minimum, 1 hugs the maximum.
	speedBias float64
	// duration bounds keep each segment long enough to establish a feel.
	minDurationMillis int64
	maxDurationMillis int64
	// focusChance is the probability a segment emphasizes a sub-region.
	focusChance float64
	// driftChance is the probability a segment drifts to a second speed.
	driftChance float64
}

var styleProfiles = map[string]styleProfile{
	config.MotionStyleGentle: {
		patternWeights: map[motion.PatternID]float64{
			motion.PatternStroke: 0.50,
			motion.PatternTease:  0.40,
			motion.PatternPulse:  0.10,
		},
		speedBias:         0.25,
		minDurationMillis: 12_000,
		maxDurationMillis: 24_000,
		focusChance:       0.20,
		driftChance:       0.25,
	},
	config.MotionStyleBalanced: {
		patternWeights: map[motion.PatternID]float64{
			motion.PatternStroke: 0.40,
			motion.PatternPulse:  0.30,
			motion.PatternTease:  0.30,
		},
		speedBias:         0.50,
		minDurationMillis: 8_000,
		maxDurationMillis: 20_000,
		focusChance:       0.25,
		driftChance:       0.35,
	},
	config.MotionStyleIntense: {
		patternWeights: map[motion.PatternID]float64{
			motion.PatternPulse:  0.50,
			motion.PatternStroke: 0.35,
			motion.PatternTease:  0.15,
		},
		speedBias:         0.80,
		minDurationMillis: 6_000,
		maxDurationMillis: 14_000,
		focusChance:       0.30,
		driftChance:       0.45,
	},
}

const (
	recencyPenaltyLast     = 0.35
	recencyPenaltySecond   = 0.15
	scoreJitter            = 0.15
	speedJitterBandPortion = 0.20
)

// Planner deterministically builds freestyle segments. Given the same seed,
// style, and settings, the segment sequence is reproducible; the seed and all
// candidate scores are recorded in planner trace rows.
type Planner struct {
	seed    int64
	rng     *rand.Rand
	recent  []motion.PatternID
	segment int
}

// NewPlanner seeds the freestyle planner; a zero seed uses the clock and the
// effective seed stays visible in every decision row.
func NewPlanner(seed int64) *Planner {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	//nolint:gosec // Deterministic, reproducible variation; not security material.
	return &Planner{seed: seed, rng: rand.New(rand.NewSource(seed))}
}

// Seed returns the planner's effective seed.
func (p *Planner) Seed() int64 {
	return p.seed
}

// NextSegment scores every pattern for the saved style and returns the next
// bounded segment plus the full score table for the trace row.
func (p *Planner) NextSegment(settings config.MotionSettings) (Segment, []diagnostics.PlannerScore) {
	profile, ok := styleProfiles[settings.Style]
	if !ok {
		profile = styleProfiles[config.MotionStyleBalanced]
	}

	patterns := []motion.PatternID{motion.PatternStroke, motion.PatternPulse, motion.PatternTease}
	scores := make([]diagnostics.PlannerScore, 0, len(patterns))
	var chosen motion.PatternID
	best := -1.0
	for _, pattern := range patterns {
		score := profile.patternWeights[pattern]
		score -= p.recencyPenalty(pattern)
		score += p.rng.Float64() * scoreJitter
		scores = append(scores, diagnostics.PlannerScore{
			PatternIdentifier: string(pattern),
			Score:             score,
		})
		if score > best {
			best = score
			chosen = pattern
		}
	}
	for index := range scores {
		scores[index].Chosen = scores[index].PatternIdentifier == string(chosen)
	}
	p.remember(chosen)
	p.segment++

	segment := Segment{
		PatternID:      chosen,
		SpeedPercent:   p.speedInBand(profile, settings),
		DurationMillis: p.durationFor(profile),
	}
	if p.rng.Float64() < profile.driftChance {
		segment.DriftToSpeedPercent = p.speedInBand(profile, settings)
	}
	if p.rng.Float64() < profile.focusChance {
		segment.AreaFocus = p.focusWindow()
	}
	return NormalizeSegment(segment), scores
}

// SegmentIndex reports how many segments the planner has produced.
func (p *Planner) SegmentIndex() int {
	return p.segment
}

func (p *Planner) recencyPenalty(pattern motion.PatternID) float64 {
	penalty := 0.0
	if len(p.recent) >= 1 && p.recent[len(p.recent)-1] == pattern {
		penalty += recencyPenaltyLast
	}
	if len(p.recent) >= 2 && p.recent[len(p.recent)-2] == pattern {
		penalty += recencyPenaltySecond
	}
	return penalty
}

func (p *Planner) remember(pattern motion.PatternID) {
	p.recent = append(p.recent, pattern)
	if len(p.recent) > 4 {
		p.recent = p.recent[len(p.recent)-4:]
	}
}

// speedInBand biases speed selection inside the user's speed limits.
// Variation comes from changing targets between segments — never from rapid
// oscillation around one target inside a segment.
func (p *Planner) speedInBand(profile styleProfile, settings config.MotionSettings) int {
	minimum := float64(settings.SpeedMinPercent)
	maximum := float64(settings.SpeedMaxPercent)
	if maximum <= minimum {
		return settings.SpeedMinPercent
	}
	band := maximum - minimum
	center := minimum + band*profile.speedBias
	jitter := (p.rng.Float64()*2 - 1) * band * speedJitterBandPortion
	speed := int(center + jitter)
	return clampInt(speed, settings.SpeedMinPercent, settings.SpeedMaxPercent)
}

func (p *Planner) durationFor(profile styleProfile) int64 {
	spread := profile.maxDurationMillis - profile.minDurationMillis
	if spread <= 0 {
		return profile.minDurationMillis
	}
	return profile.minDurationMillis + p.rng.Int63n(spread)
}

// focusWindow emits a bounded emphasis region, never a hard lock point.
func (p *Planner) focusWindow() *motion.AreaFocus {
	width := 35 + p.rng.Intn(26) // 35..60 percent of travel
	start := p.rng.Intn(101 - width)
	return &motion.AreaFocus{MinPercent: start, MaxPercent: start + width}
}
