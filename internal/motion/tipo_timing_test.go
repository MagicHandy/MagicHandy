package motion

import "testing"

func TestTipoBaseAtrasoMSLadder(t *testing.T) {
	ladder := []struct {
		tipo string
		want int
	}{
		{"lento", 200},
		{"fluido", 168},
		{"simples", 138},
		{"leve", 118},
		{"moderado", 92},
		{"alto", 68},
		{"very_fast", 38},
	}
	prev := 999
	for _, step := range ladder {
		got := tipoBaseAtrasoMS(step.tipo)
		if got != step.want {
			t.Fatalf("%s base = %d, want %d", step.tipo, got, step.want)
		}
		if got >= prev {
			t.Fatalf("%s base %d not faster than previous %d", step.tipo, got, prev)
		}
		prev = got
	}
}

func TestScaleAtrasoByVelocityGentle(t *testing.T) {
	base := tipoBaseAtrasoMS("fluido")
	slow := scaleAtrasoByVelocity(base, 30)
	mid := scaleAtrasoByVelocity(base, 50)
	fast := scaleAtrasoByVelocity(base, 90)
	if slow <= mid || mid <= fast {
		t.Fatalf("velocity scale not monotonic: slow=%d mid=%d fast=%d", slow, mid, fast)
	}
	// Avoid extreme jumps (old curve could halve in one tipo step).
	if float64(slow)/float64(fast) > 2.8 {
		t.Fatalf("velocity ratio too steep: slow=%d fast=%d", slow, fast)
	}
}

func TestResolveAtrasoMSOrganicProgression(t *testing.T) {
	alto := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "alto", Velocidade: 50})
	moderado := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "moderado", Velocidade: 50})
	fluido := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: "fluido", Velocidade: 50})
	if alto >= moderado || moderado >= fluido {
		t.Fatalf("tipo ladder broken: alto=%d moderado=%d fluido=%d", alto, moderado, fluido)
	}
}
