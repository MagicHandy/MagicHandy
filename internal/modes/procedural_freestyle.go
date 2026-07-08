package modes

import (
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// ProceduralFreestyleSegment is one bounded procedural chaos step for freestyle.
type ProceduralFreestyleSegment struct {
	Physics        motion.ChaoticPhysics
	DurationMillis int64
}

// NextProceduralFreestyleSegment maps the saved freestyle style into chaotic physics.
func NextProceduralFreestyleSegment(planner *Planner, settings config.MotionSettings) ProceduralFreestyleSegment {
	profile, ok := styleProfiles[settings.Style]
	if !ok {
		profile = styleProfiles[config.MotionStyleBalanced]
	}
	if planner == nil {
		planner = NewPlanner(0)
	}

	physics := motion.ChaoticPhysics{
		Velocidade:  planner.speedInBand(profile, settings),
		Intensidade: planner.intensityInBand(profile, settings),
		Regiao:      proceduralRegiaoForStyle(settings.Style, planner),
		TipoBatida:  proceduralTipoBatidaForStyle(settings.Style, planner),
	}
	return ProceduralFreestyleSegment{
		Physics:        physics,
		DurationMillis: planner.durationFor(profile),
	}
}

func (p *Planner) intensityInBand(profile styleProfile, settings config.MotionSettings) int {
	minimum := float64(settings.SpeedMinPercent)
	maximum := float64(settings.SpeedMaxPercent)
	if maximum <= minimum {
		return settings.SpeedMinPercent
	}
	band := maximum - minimum
	center := minimum + band*(profile.speedBias*0.85+0.1)
	jitter := (p.rng.Float64()*2 - 1) * band * speedJitterBandPortion
	return clampInt(int(center+jitter), settings.SpeedMinPercent, settings.SpeedMaxPercent)
}

func proceduralRegiaoForStyle(style string, planner *Planner) string {
	switch style {
	case config.MotionStyleGentle:
		return pickPlannerRegiao(planner, []string{"meio", "meio_cabeca"}, []float64{0.35, 0.65})
	case config.MotionStyleIntense:
		return pickPlannerRegiao(planner, []string{"cabeca", "meio_cabeca", "meio"}, []float64{0.45, 0.40, 0.15})
	default:
		return pickPlannerRegiao(planner, []string{"meio_cabeca", "cabeca", "meio"}, []float64{0.45, 0.35, 0.20})
	}
}

func pickPlannerRegiao(planner *Planner, values []string, weights []float64) string {
	if planner == nil || len(values) == 0 {
		return "meio_cabeca"
	}
	if len(weights) != len(values) {
		return values[planner.rng.Intn(len(values))]
	}
	total := 0.0
	for _, w := range weights {
		total += w
	}
	roll := planner.rng.Float64() * total
	for i, value := range values {
		roll -= weights[i]
		if roll <= 0 {
			return value
		}
	}
	return values[len(values)-1]
}

func proceduralTipoBatidaForStyle(style string, planner *Planner) string {
	switch style {
	case config.MotionStyleGentle:
		if planner.rng.Float64() < 0.45 {
			return "fluido"
		}
		if planner.rng.Float64() < 0.70 {
			return "lento"
		}
		return "leve"
	case config.MotionStyleIntense:
		if planner.rng.Float64() < 0.35 {
			return "alto"
		}
		if planner.rng.Float64() < 0.55 {
			return "moderado"
		}
		return "fluido"
	default:
		if planner.rng.Float64() < 0.40 {
			return "fluido"
		}
		if planner.rng.Float64() < 0.60 {
			return "leve"
		}
		return "moderado"
	}
}
