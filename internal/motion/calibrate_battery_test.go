package motion

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

type calibrateCase struct {
	name       string
	profile    string
	physics    ChaoticPhysics
	wantZone   regionRange
	minInZone  float64
	maxMinDelta int
	maxMaxDelta int
}

func freestyleProfilePhysics(profile string, rng *rand.Rand) ChaoticPhysics {
	switch profile {
	case "gentle":
		return ChaoticPhysics{
			Velocidade:  28,
			Intensidade: 32,
			Regiao:      pickProfileRegiao([]string{"meio", "meio_cabeca"}, rng),
			TipoBatida:  pickProfileTipo([]string{"fluido", "lento", "leve"}, rng),
			AtrasoMS:    160,
		}
	case "intense":
		return ChaoticPhysics{
			Velocidade:  82,
			Intensidade: 78,
			Regiao:      pickProfileRegiao([]string{"cabeca", "meio_cabeca", "meio"}, rng),
			TipoBatida:  pickProfileTipo([]string{"alto", "moderado", "fluido"}, rng),
			AtrasoMS:    80,
		}
	default: // balanced
		return ChaoticPhysics{
			Velocidade:  52,
			Intensidade: 55,
			Regiao:      pickProfileRegiao([]string{"meio_cabeca", "cabeca", "meio"}, rng),
			TipoBatida:  pickProfileTipo([]string{"fluido", "leve", "moderado"}, rng),
			AtrasoMS:    160,
		}
	}
}

func pickProfileRegiao(values []string, rng *rand.Rand) string {
	return values[rng.Intn(len(values))]
}

func pickProfileTipo(values []string, rng *rand.Rand) string {
	return values[rng.Intn(len(values))]
}

func allCalibrationCases() []calibrateCase {
	rng := rand.New(rand.NewSource(42))
	cases := make([]calibrateCase, 0, 128)

	regions := []struct {
		name string
		want regionRange
		min  float64
	}{
		{"cabeca", regionRange{70, 100}, 90},
		{"meio", regionRange{30, 69}, 90},
		{"base", regionRange{0, 29}, 90},
		{"meio_cabeca", regionRange{30, 100}, 88},
		{"meio_base", regionRange{0, 69}, 88},
		{"full", regionRange{0, 100}, 85},
	}

	tipos := []string{"fluido", "lento", "leve", "moderado", "alto", "simples", "very_fast"}
	velocities := []int{25, 50, 75, 95}

	for _, profile := range []string{"gentle", "balanced", "intense"} {
		p := freestyleProfilePhysics(profile, rng)
		region, _ := chaosRegionRange(p.Regiao)
		cases = append(cases, calibrateCase{
			name:       fmt.Sprintf("profile_%s", profile),
			profile:    profile,
			physics:    p,
			wantZone:   region,
			minInZone:  85,
			maxMinDelta: maxMinDeltaForTipo(p.TipoBatida, false),
			maxMaxDelta: maxMaxDeltaForTipo(p.TipoBatida),
		})
	}

	for _, reg := range regions {
		for _, tipo := range tipos {
			for _, vel := range velocities {
				atraso := 160
				if tipo == "very_fast" {
					atraso = 0
				}
				cases = append(cases, calibrateCase{
					name: fmt.Sprintf("reg_%s_tipo_%s_vel_%d", reg.name, tipo, vel),
					physics: ChaoticPhysics{
						Regiao:      reg.name,
						TipoBatida:  tipo,
						Velocidade:  vel,
						Intensidade: 50 + vel/5,
						AtrasoMS:    atraso,
					},
					wantZone:    reg.want,
					minInZone:   reg.min,
					maxMinDelta: maxMinDeltaForTipo(tipo, false),
					maxMaxDelta: maxMaxDeltaForTipo(tipo),
				})
			}
		}
	}

	return cases
}

func maxMinDeltaForTipo(tipo string, safetyLock bool) int {
	if safetyLock {
		return 30
	}
	switch normalizeTipoBatida(tipo) {
	case "very_fast":
		return veryFastStepMaxMS + 5
	case "vibrate", "turbo":
		return turboVibrateHalfCycleMaxMS + 10
	case "alto":
		return 45
	default:
		return 120
	}
}

func maxMaxDeltaForTipo(tipo string) int {
	switch normalizeTipoBatida(tipo) {
	case "very_fast":
		return veryFastStepMaxMS + 5
	case "vibrate", "turbo":
		return turboVibrateHalfCycleMaxMS + 5
	case "lento", "fluido":
		return 320
	default:
		return 320
	}
}

func TestMotionCalibrateBatteryUnit(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for _, tc := range allCalibrationCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			waypoints := GenerateStrokeWaypoints(tc.physics, 2_500, false, rng)
			trace := SummarizeMotionTrace(tc.physics, waypoints, 300, -1)
			if trace.PointCount < 4 {
				t.Fatalf("point_count = %d, want meaningful stroke", trace.PointCount)
			}
			if trace.PointsInZonePct < tc.minInZone && tc.physics.Regiao != "full" {
				t.Fatalf("points_in_zone = %.1f%% (pos %d..%d zone %d..%d)",
					trace.PointsInZonePct, trace.PosMin, trace.PosMax, trace.ZoneMin, trace.ZoneMax)
			}
			zoneSpan := trace.ZoneMax - trace.ZoneMin
			if zoneSpan < 1 {
				zoneSpan = 1
			}
			maxAllowedStep := zoneSpan/3 + 16
			if IsTurboTipo(tc.physics.TipoBatida) {
				tipo := normalizeTipoBatida(tc.physics.TipoBatida)
				if tipo == "very_fast" {
					maxAllowedStep = zoneSpan
				} else {
					maxAllowedStep = turboVibrateRangeMax
				}
			} else if normalizeTipoBatida(tc.physics.TipoBatida) == "very_fast" {
				maxAllowedStep = zoneSpan/2 + 4
			}
			if trace.MaxPositionStep > maxAllowedStep {
				t.Fatalf("max_position_step = %d, want <= %d for zone span %d",
					trace.MaxPositionStep, maxAllowedStep, zoneSpan)
			}
			if trace.MinDeltaMS > tc.maxMinDelta {
				t.Fatalf("min_delta = %dms, want <= %d for %s vel=%d safety=off",
					trace.MinDeltaMS, tc.maxMinDelta, tc.physics.TipoBatida, tc.physics.Velocidade)
			}
			if trace.MaxDeltaMS > tc.maxMaxDelta {
				t.Fatalf("max_delta = %dms, want <= %d", trace.MaxDeltaMS, tc.maxMaxDelta)
			}
			if trace.EffectiveAtrasoMS <= 0 {
				t.Fatalf("effective_atraso_ms = %d", trace.EffectiveAtrasoMS)
			}
			if tc.physics.Velocidade >= 75 && normalizeTipoBatida(tc.physics.TipoBatida) != "very_fast" {
				slow := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: tc.physics.TipoBatida, Velocidade: 25, AtrasoMS: tc.physics.AtrasoMS})
				fast := ResolveAtrasoMS(ChaoticPhysics{TipoBatida: tc.physics.TipoBatida, Velocidade: 95, AtrasoMS: tc.physics.AtrasoMS})
				if fast >= slow {
					t.Fatalf("velocidade scaling: fast=%d slow=%d", fast, slow)
				}
			}
		})
	}
}

func TestMotionCalibrateBatteryZoneBounds(t *testing.T) {
	for _, reg := range []string{"cabeca", "meio", "base", "meio_cabeca", "meio_base", "full"} {
		reg := reg
		t.Run(reg, func(t *testing.T) {
			physics := ChaoticPhysics{
				Regiao:      reg,
				TipoBatida:  "fluido",
				Velocidade:  50,
				Intensidade: 50,
				AtrasoMS:    160,
			}
			stream := GenerateStrokeWaypoints(physics, 5_000, false, rand.New(rand.NewSource(7)))
			trace := SummarizeMotionTrace(physics, stream, 300, -1)
			margin := 8
			if strings.Contains(reg, "full") || strings.Contains(reg, "meio_") {
				margin = 15
			}
			if trace.PosMin > trace.ZoneMin+margin || trace.PosMax < trace.ZoneMax-margin {
				t.Fatalf("%s travel %d..%d outside zone %d..%d", reg, trace.PosMin, trace.PosMax, trace.ZoneMin, trace.ZoneMax)
			}
		})
	}
}
