package chatauto

// ScenePlan is the AI "roteiro" — procedural motor executes motion from this.
type ScenePlan struct {
	Humor          Humor
	Posicao        Pose
	IntensidadeMin int
	IntensidadeMax int
	Velocidade     int
}

// PlanFromIntent normalizes an LLM intent into a scene plan.
func PlanFromIntent(intent Intent) ScenePlan {
	minI, maxI := intent.IntensidadeMin, intent.IntensidadeMax
	if minI <= 0 && maxI <= 0 {
		minI = intent.Intensidade
		maxI = intent.Intensidade
	}
	if minI <= 0 {
		minI = 1
	}
	if maxI <= 0 {
		maxI = minI
	}
	if minI > maxI {
		minI, maxI = maxI, minI
	}
	if intent.Intensidade < minI {
		intent.Intensidade = minI
	}
	if intent.Intensidade > maxI {
		intent.Intensidade = maxI
	}
	return ScenePlan{
		Humor:          intent.Humor,
		Posicao:        intent.Posicao,
		IntensidadeMin: minI,
		IntensidadeMax: maxI,
	}
}

// PlanFromState builds a plan from live session state (bridge fallback).
func PlanFromState(state State) ScenePlan {
	intensity := state.SceneIntensidade
	if intensity <= 0 && state.Motion.Intensidade > 0 {
		intensity = max(2, min(10, state.Motion.Intensidade/10))
	}
	if intensity <= 0 {
		intensity = 4
	}
	return ScenePlan{
		Humor:          state.Humor,
		Posicao:        state.Posicao,
		IntensidadeMin: max(1, intensity-1),
		IntensidadeMax: min(10, intensity+1),
	}
}

// EffectiveIntensity picks runtime intensity from plan + stamina.
func EffectiveIntensity(plan ScenePlan, stamina float64) int {
	span := plan.IntensidadeMax - plan.IntensidadeMin
	if span < 0 {
		span = 0
	}
	ratio := stamina / 100
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	// High stamina → upper band; low stamina → lower band.
	intensity := plan.IntensidadeMin + int(float64(span)*ratio+0.5)
	if stamina < 30 && intensity > 4 {
		intensity = 4
	}
	if stamina < 15 && intensity > 2 {
		intensity = 2
	}
	if intensity < 1 {
		return 1
	}
	if intensity > 10 {
		return 10
	}
	return intensity
}

// PlanToIntent materializes one procedural block intent for mapping and stamina drain.
func PlanToIntent(plan ScenePlan, stamina float64, durationSec int) Intent {
	if durationSec < 1 {
		durationSec = 1
	}
	intensity := plan.IntensidadeMin
	if plan.IntensidadeMin != plan.IntensidadeMax {
		intensity = EffectiveIntensity(plan, stamina)
	}
	return Intent{
		Humor:           plan.Humor,
		Posicao:         plan.Posicao,
		Intensidade:     intensity,
		DuracaoSegundos: durationSec,
		IntensidadeMin:  plan.IntensidadeMin,
		IntensidadeMax:  plan.IntensidadeMax,
		Velocidade:      plan.Velocidade,
	}
}
