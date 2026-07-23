package chatauto

import (
	"math"
	"time"
)

const (
	DefaultDominatrixRampMinutes = 10
	MaxDominatrixRampMinutes     = 10
	moodEaseExponent             = 0.82
)

var moodStageOrder = []Humor{
	HumorDesejando,
	HumorTesao,
	HumorIntensa,
	HumorDominatrix,
}

// MoodAtElapsed returns progress (0–100) and the mood stage for elapsed session time.
func MoodAtElapsed(elapsed time.Duration, rampMinutes int, allowDominatrix bool) (progress float64, humor Humor) {
	rampMinutes = clampRampMinutes(rampMinutes)
	if rampMinutes <= 0 {
		return 100, peakHumor(allowDominatrix)
	}
	ramp := time.Duration(rampMinutes) * time.Minute
	if elapsed < 0 {
		elapsed = 0
	}
	ratio := float64(elapsed) / float64(ramp)
	if ratio > 1 {
		ratio = 1
	}
	eased := math.Pow(ratio, moodEaseExponent)
	progress = eased * 100
	return progress, HumorFromProgress(progress, allowDominatrix)
}

// HumorFromProgress maps a 0–100 progress value to a mood stage.
func HumorFromProgress(progress float64, allowDominatrix bool) Humor {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	stages := moodStages(allowDominatrix)
	if len(stages) == 0 {
		return HumorDesejando
	}
	thresholds := moodThresholds(allowDominatrix)
	for i := len(thresholds) - 2; i >= 0; i-- {
		if progress >= thresholds[i] {
			if i >= len(stages) {
				return stages[len(stages)-1]
			}
			return stages[i]
		}
	}
	return stages[0]
}

func moodThresholds(allowDominatrix bool) []float64 {
	if allowDominatrix {
		return []float64{0, 28, 54, 78, 100}
	}
	return []float64{0, 30, 58, 100}
}

// EffectiveHumor returns the higher of two mood stages on the ramp ladder.
func EffectiveHumor(current, candidate Humor, allowDominatrix bool) Humor {
	stages := moodStages(allowDominatrix)
	currentIdx := humorIndex(current, stages)
	candidateIdx := humorIndex(candidate, stages)
	if candidateIdx > currentIdx {
		return candidate
	}
	return current
}

// BoostIntensity nudges intensity upward as mood progress increases.
func BoostIntensity(base int, progress float64) int {
	if base < 1 {
		base = 1
	}
	boost := int(progress / 38)
	next := base + boost
	if next > 10 {
		return 10
	}
	return next
}

func moodStages(allowDominatrix bool) []Humor {
	if allowDominatrix {
		return moodStageOrder
	}
	return moodStageOrder[:len(moodStageOrder)-1]
}

func peakHumor(allowDominatrix bool) Humor {
	stages := moodStages(allowDominatrix)
	return stages[len(stages)-1]
}

func humorIndex(humor Humor, stages []Humor) int {
	for i, stage := range stages {
		if stage == humor {
			return i
		}
	}
	return 0
}

func clampRampMinutes(minutes int) int {
	if minutes <= 0 {
		return DefaultDominatrixRampMinutes
	}
	if minutes > MaxDominatrixRampMinutes {
		return MaxDominatrixRampMinutes
	}
	return minutes
}
