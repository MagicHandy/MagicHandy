package chatauto

import "testing"

func TestPlanFromIntentMinMax(t *testing.T) {
	plan := PlanFromIntent(Intent{
		Humor:           HumorTesao,
		Posicao:         PoseOral,
		Intensidade:     5,
		IntensidadeMin:  3,
		IntensidadeMax:  8,
	})
	if plan.IntensidadeMin != 3 || plan.IntensidadeMax != 8 {
		t.Fatalf("plan min/max = %d/%d", plan.IntensidadeMin, plan.IntensidadeMax)
	}
}

func TestEffectiveIntensityScalesWithStamina(t *testing.T) {
	plan := ScenePlan{IntensidadeMin: 2, IntensidadeMax: 9}
	low := EffectiveIntensity(plan, 10)
	high := EffectiveIntensity(plan, 95)
	if high <= low {
		t.Fatalf("high=%d low=%d, want high > low", high, low)
	}
}
