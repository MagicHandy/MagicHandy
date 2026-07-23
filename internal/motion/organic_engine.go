package motion

import (
	"math"
	"math/rand"
)

const (
	organicMaxWaypoints      = 4096
	organicStreamLeadMillis  = 300
	organicMinSignal         = 0.03
	organicMaxSignal         = 0.97
)

// OrganicConfig drives Perlin-based continuous stroke generation.
type OrganicConfig struct {
	// BaseVelocity is overall stroke cadence (1–100).
	BaseVelocity float64
	// StrokeMin and StrokeMax are Handy axis bounds (0–100).
	StrokeMin float64
	StrokeMax float64
	// NoiseWeight controls Perlin influence on path shape (0–1).
	NoiseWeight float64
	// Intensity shapes timing acceleration near stroke peaks (0–100).
	Intensity float64
	// Asymmetry biases ascent vs descent imperfection (0–1).
	Asymmetry float64
	// WaveFrequencyHz is the carrier frequency; zero picks from velocity.
	WaveFrequencyHz float64
	// SampleIntervalMS is the base delay between waypoints (ms).
	SampleIntervalMS int
	// StrokeProfile drives asymmetric down/up timing and bottom bounce.
	StrokeProfile StrokeProfile
	// TipoBatida selects cycle pacing tweaks.
	TipoBatida string
}

// OrganicConfigFromPhysics maps legacy chaotic physics into organic parameters.
func OrganicConfigFromPhysics(physics ChaoticPhysics) OrganicConfig {
	region := effectiveSemanticRegion(physics)
	strokeMin := float64(region.Min)
	strokeMax := float64(region.Max)

	vel := float64(clampInt(physics.Velocidade, 1, 100))
	intensity := float64(clampInt(physics.Intensidade, 1, 100))
	noise := 0.22 + intensity/220.0
	asym := 0.45 + intensity/250.0

	switch normalizeTipoBatida(physics.TipoBatida) {
	case "vibrate", "turbo":
		noise = 0.12 + intensity/400.0
		asym = 0.35
	case "alto":
		noise = 0.28 + intensity/180.0
		asym = 0.55
	case "leve", "simples":
		noise = 0.18 + intensity/280.0
		asym = 0.40
	case "lento":
		noise = 0.20 + intensity/240.0
		asym = 0.38
		vel *= 0.82
	}

	return OrganicConfig{
		BaseVelocity:     vel,
		StrokeMin:        clampFloat(strokeMin, 0, 100),
		StrokeMax:        clampFloat(strokeMax, 0, 100),
		NoiseWeight:      clampFloat(noise, 0.08, 0.55),
		Intensity:        intensity,
		Asymmetry:        clampFloat(asym, 0.2, 0.75),
		SampleIntervalMS: ResolveAtrasoMS(physics),
		StrokeProfile:    resolveStrokeProfile(physics),
		TipoBatida:       normalizeTipoBatida(physics.TipoBatida),
	}
}

func resolveStrokeProfile(physics ChaoticPhysics) StrokeProfile {
	if physics.StrokeProfile.DownstrokeRatio > 0 || physics.StrokeProfile.UpstrokeRatio > 0 || physics.StrokeProfile.HasBottomBounce {
		return NormalizeStrokeProfile(physics.StrokeProfile)
	}
	if physics.Action != "" {
		return StrokeProfileFromAction(physics.Action)
	}
	return DefaultStrokeProfile()
}

// GenerateOrganicWaypoints builds a continuous Perlin-modulated stroke for durationMS.
func GenerateOrganicWaypoints(
	cfg OrganicConfig,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	return generateOrganicWaypoints(cfg, durationMS, hardwareSafetyLock, rng, -1)
}

// GenerateOrganicWaypointsFromPosition continues from a device position (0..100).
func GenerateOrganicWaypointsFromPosition(
	cfg OrganicConfig,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	return generateOrganicWaypoints(cfg, durationMS, hardwareSafetyLock, rng, continueFrom)
}

// GenerateOrganicStream fills targetDurationMillis with one continuous organic wave.
func GenerateOrganicStream(
	cfg OrganicConfig,
	targetDurationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	return generateOrganicStream(cfg, targetDurationMillis, hardwareSafetyLock, rng, -1)
}

// GenerateOrganicStreamFromPosition continues a long organic stream from device position.
func GenerateOrganicStreamFromPosition(
	cfg OrganicConfig,
	targetDurationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	return generateOrganicStream(cfg, targetDurationMillis, hardwareSafetyLock, rng, continueFrom)
}

func generateOrganicStream(
	cfg OrganicConfig,
	targetDurationMillis int64,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	if targetDurationMillis <= 0 {
		return generateOrganicWaypoints(cfg, 0, hardwareSafetyLock, rng, continueFrom)
	}
	return generateOrganicWaypoints(cfg, int(targetDurationMillis), hardwareSafetyLock, rng, continueFrom)
}

func generateOrganicWaypoints(
	cfg OrganicConfig,
	durationMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	continueFrom int,
) []ChaosWaypoint {
	if rng == nil {
		rng = rand.New(rand.NewSource(1)) // #nosec G404
	}
	if durationMS <= 0 {
		durationMS = 5_000
	}

	cfg = normalizeOrganicConfig(cfg)
	seed := rng.Int63()
	perlin := NewPerlinNoise(seed)

	baseDelta := cfg.SampleIntervalMS
	if baseDelta <= 0 {
		baseDelta = velocityToBaseDeltaMillis(int(cfg.BaseVelocity))
	}

	freq := cfg.WaveFrequencyHz
	if freq <= 0 {
		freq = 0.45 + cfg.BaseVelocity/95.0
	}

	span := cfg.StrokeMax - cfg.StrokeMin
	if span < 2 {
		span = 2
	}
	maxStep := int(span/3) + 2
	if maxStep < 4 {
		maxStep = 4
	}

	phase := rng.Float64() * 4.0
	noiseY := rng.Float64() * 200.0
	ascentY := noiseY + 17.3
	descentY := noiseY + 53.7

	var out []ChaosWaypoint
	elapsed := 0
	startPos := (cfg.StrokeMin + cfg.StrokeMax) / 2
	if continueFrom >= 0 {
		rawFrom := continueFrom
		if float64(rawFrom) < cfg.StrokeMin || float64(rawFrom) > cfg.StrokeMax {
			entry := int(math.Round(cfg.StrokeMin))
			if float64(rawFrom) > cfg.StrokeMax {
				entry = int(math.Round(cfg.StrokeMax))
			}
			bridge := organicZoneBridge(rawFrom, entry, baseDelta, maxStep, cfg, hardwareSafetyLock, rng)
			out = append(out, bridge...)
			startPos = float64(entry)
			for _, wp := range bridge {
				elapsed += wp.TimeDelta
			}
		} else {
			startPos = float64(rawFrom)
			out = append(out, ChaosWaypoint{
				TimeDelta: organicJitteredDelta(baseDelta, cfg, hardwareSafetyLock, rng, 0),
				Position:  rawFrom,
			})
			elapsed += out[len(out)-1].TimeDelta
		}
	}

	prevPos := startPos
	if len(out) > 0 {
		prevPos = float64(out[len(out)-1].Position)
	}

	block := generateOrganicBlock(
		cfg,
		durationMS-elapsed,
		hardwareSafetyLock,
		rng,
		perlin,
		phase,
		noiseY,
		ascentY,
		descentY,
		prevPos,
		maxStep,
		baseDelta,
		freq,
		span,
	)
	for _, wp := range block {
		out = append(out, wp)
		elapsed += wp.TimeDelta
	}

	if len(out) > 0 {
		MotionDebugLog("ORG", "organic_engine.go:generateOrganicWaypoints", "perlin organic stats", map[string]any{
			"stats":       organicMotionStats(out),
			"noiseWeight": cfg.NoiseWeight,
			"freqHz":      freq,
			"strokeMin":   cfg.StrokeMin,
			"strokeMax":   cfg.StrokeMax,
		})
	}
	return out
}

// generateOrganicBlock produces waypoints with Perlin bounds and asymmetric stroke profiles.
func generateOrganicBlock(
	cfg OrganicConfig,
	remainingMS int,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	perlin *PerlinNoise,
	phase float64,
	noiseY, ascentY, descentY float64,
	startPos float64,
	maxStep int,
	baseDelta int,
	freq float64,
	span float64,
) []ChaosWaypoint {
	if remainingMS <= 0 {
		return nil
	}
	profile := NormalizeStrokeProfile(cfg.StrokeProfile)
	cycleMS := estimateStrokeCycleMS(cfg, baseDelta)
	if cycleMS < baseDelta*4 {
		cycleMS = baseDelta * 4
	}

	var out []ChaosWaypoint
	elapsed := 0
	prevPos := startPos
	cycleProgress := 0.0
	cycleElapsed := 0
	bounceDone := false

	for elapsed < remainingMS && len(out) < organicMaxWaypoints {
		if cycleElapsed == 0 {
			bounceDone = false
		}

		progress := float64(elapsed) / float64(remainingMS)
		delta := organicJitteredDelta(baseDelta, cfg, hardwareSafetyLock, rng, progress)
		if elapsed+delta > remainingMS {
			delta = remainingMS - elapsed
			if delta < 1 {
				break
			}
		}

		phase += freq * float64(delta) / 1000.0
		cycleElapsed += delta
		cycleProgress = float64(cycleElapsed) / float64(cycleMS)
		if cycleProgress > 1 {
			cycleProgress = 1
		}

		inDownstroke := cycleProgress < profile.DownstrokeRatio
		var signal float64
		if inDownstroke {
			local := cycleProgress / profile.DownstrokeRatio
			signal = 1.0 - easeInQuad(local)
		} else {
			local := (cycleProgress - profile.DownstrokeRatio) / profile.UpstrokeRatio
			if local < 0 {
				local = 0
			}
			if local > 1 {
				local = 1
			}
			signal = easeOutCubic(local)
		}

		nAscent := perlin.Noise2D(phase*1.85, ascentY)
		nDescent := perlin.Noise2D(phase*1.45, descentY)
		nDrift := perlin.Noise2D(float64(elapsed)/700.0+phase*0.2, noiseY+91.0)
		nMicro := perlin.Noise2D(phase*5.2, noiseY+phase*0.4)

		var asymNoise float64
		if inDownstroke {
			asymNoise = nDescent*cfg.Asymmetry + nMicro*(1-cfg.Asymmetry)*0.35
		} else {
			asymNoise = nAscent*(1-cfg.Asymmetry*0.65) + nMicro*cfg.Asymmetry*0.28
		}
		signal += asymNoise*cfg.NoiseWeight*0.22 + nDrift*cfg.NoiseWeight*0.10
		signal = clampFloat(signal, organicMinSignal, organicMaxSignal)

		pos := cfg.StrokeMin + signal*span
		pos = clampFloat(pos, cfg.StrokeMin, cfg.StrokeMax)
		posI := int(math.Round(pos))
		posI = clampOrganicStep(int(math.Round(prevPos)), posI, maxStep)
		posI = enforceMinOrganicStep(int(math.Round(prevPos)), posI, int(cfg.StrokeMin), int(cfg.StrokeMax))

		out = append(out, ChaosWaypoint{TimeDelta: delta, Position: posI})
		prevPos = float64(posI)
		elapsed += delta

		atBottom := !inDownstroke && cycleProgress >= profile.DownstrokeRatio && cycleProgress < profile.DownstrokeRatio+0.04
		if profile.HasBottomBounce && atBottom && !bounceDone {
			botPos := cfg.StrokeMin + organicMinSignal*span
			bounce := buildBottomBounce(botPos, cfg.StrokeMin, cfg.StrokeMax, perlin, phase, rng)
			for _, wp := range bounce {
				if elapsed+wp.TimeDelta > remainingMS {
					break
				}
				pos := enforceMinOrganicStep(int(math.Round(prevPos)), wp.Position, int(cfg.StrokeMin), int(cfg.StrokeMax))
				wp.Position = pos
				out = append(out, wp)
				elapsed += wp.TimeDelta
				prevPos = float64(wp.Position)
			}
			bounceDone = true
		}

		if cycleElapsed >= cycleMS {
			cycleElapsed = 0
			cycleProgress = 0
		}
	}
	return out
}

func estimateStrokeCycleMS(cfg OrganicConfig, baseDelta int) int {
	vel := int(cfg.BaseVelocity)
	if vel <= 0 {
		vel = 50
	}
	cycle := 900 + (100-vel)*12
	cycle = int(float64(cycle) * organicCycleVelocityScale(cfg.TipoBatida))
	if baseDelta > 1 {
		cycle = cycle * baseDelta / 120
	}
	if cycle < baseDelta*4 {
		cycle = baseDelta * 4
	}
	return cycle
}

func organicJitteredDelta(
	baseMS int,
	cfg OrganicConfig,
	hardwareSafetyLock bool,
	rng *rand.Rand,
	progress float64,
) int {
	intensity := int(cfg.Intensity)
	if intensity <= 0 {
		intensity = 50
	}
	physics := ChaoticPhysics{
		TipoBatida:  "fluido",
		Intensidade: intensity,
	}
	delta := jitteredTimeDelta(baseMS, progress, intensity, physics, hardwareSafetyLock, rng)
	if baseMS <= 1 {
		return clampDelta(1, hardwareSafetyLock)
	}
	// Extra human-like ms flutter independent of position jitter.
	if rng != nil {
		wobble := 1 + (rng.Float64()*2-1)*0.11*cfg.NoiseWeight
		delta = int(math.Round(float64(delta) * wobble))
		delta = clampDelta(delta, hardwareSafetyLock)
	}
	if delta < 1 {
		delta = 1
	}
	return delta
}

func normalizeOrganicConfig(cfg OrganicConfig) OrganicConfig {
	if cfg.BaseVelocity <= 0 {
		cfg.BaseVelocity = 50
	}
	if cfg.BaseVelocity > 100 {
		cfg.BaseVelocity = 100
	}
	if cfg.StrokeMax <= cfg.StrokeMin {
		cfg.StrokeMin = 0
		cfg.StrokeMax = 100
	}
	cfg.StrokeMin = clampFloat(cfg.StrokeMin, 0, 100)
	cfg.StrokeMax = clampFloat(cfg.StrokeMax, 0, 100)
	if cfg.NoiseWeight <= 0 {
		cfg.NoiseWeight = 0.25
	}
	if cfg.Intensity <= 0 {
		cfg.Intensity = 50
	}
	if cfg.Asymmetry <= 0 {
		cfg.Asymmetry = 0.45
	}
	return cfg
}

func clampFloat(v, minVal, maxVal float64) float64 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func clampOrganicStep(prev, next, maxStep int) int {
	step := next - prev
	if absIntValue(step) <= maxStep {
		return next
	}
	if step > 0 {
		return prev + maxStep
	}
	return prev - maxStep
}

func enforceMinOrganicStep(prev, next, minBound, maxBound int) int {
	step := next - prev
	absStep := absIntValue(step)
	if absStep == 0 || absStep >= 3 {
		return next
	}
	if step > 0 {
		if prev+3 <= maxBound {
			return prev + 3
		}
		if prev-3 >= minBound {
			return prev - 3
		}
	} else {
		if prev-3 >= minBound {
			return prev - 3
		}
		if prev+3 <= maxBound {
			return prev + 3
		}
	}
	return next
}

func organicZoneBridge(
	from, to int,
	baseDelta int,
	maxStep int,
	cfg OrganicConfig,
	hardwareSafetyLock bool,
	rng *rand.Rand,
) []ChaosWaypoint {
	if from == to {
		return []ChaosWaypoint{{
			TimeDelta: organicJitteredDelta(baseDelta, cfg, hardwareSafetyLock, rng, 0),
			Position:  from,
		}}
	}
	blend := CubicHermiteCrossfade(
		MotionBlendState{Position: float64(from), Velocity: 0},
		MotionBlendState{Position: float64(to), Velocity: 0},
		CrossfadeOptions{
			DurationMS:         clampInt(absIntValue(to-from)*18, crossfadeMinDurationMS/2, crossfadeMaxDurationMS),
			PointCount:         crossfadeMaxPoints,
			HardwareSafetyLock: hardwareSafetyLock,
		},
		rng,
	)
	for i := range blend {
		if i > 0 {
			blend[i].Position = clampOrganicStep(blend[i-1].Position, blend[i].Position, maxStep)
			blend[i].Position = enforceMinOrganicStep(blend[i-1].Position, blend[i].Position, int(cfg.StrokeMin), int(cfg.StrokeMax))
		} else {
			blend[i].Position = from
		}
	}
	return blend
}
