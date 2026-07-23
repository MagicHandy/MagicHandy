package chatauto

import (
	"math"
	"testing"
)

func TestDrainRatePerSecondBounds(t *testing.T) {
	if got := DrainRatePerSecond(1); math.Abs(got-StaminaMinDrainPerSec) > 0.001 {
		t.Fatalf("intensity 1 rate = %v, want %v", got, StaminaMinDrainPerSec)
	}
	if got := DrainRatePerSecond(10); math.Abs(got-StaminaMaxDrainPerSec) > 0.001 {
		t.Fatalf("intensity 10 rate = %v, want %v", got, StaminaMaxDrainPerSec)
	}
}

func TestSlowMotionRecoversStamina(t *testing.T) {
	intent := Intent{Humor: HumorDesejando, Intensidade: 2, Velocidade: 2}
	if StaminaNetRatePerSecond(intent, 40) >= 0 {
		t.Fatal("slow motion below 100 should have negative net rate (recovery)")
	}
	after := ApplyProceduralStamina(40, intent, 10)
	if after <= 40 {
		t.Fatalf("expected recovery, got %.2f", after)
	}
}

func TestDesejandoHighIntensityDrainsAtFullStamina(t *testing.T) {
	intent := Intent{Humor: HumorDesejando, Intensidade: 7, Velocidade: 6}
	rate := StaminaNetRatePerSecond(intent, 100)
	if rate <= 0 {
		t.Fatalf("full stamina with high intensity should drain, rate=%v", rate)
	}
	dur := ProceduralBlockDuration(100, intent)
	if dur < 30 {
		t.Fatalf("block duration = %d, want meaningful drain cycle", dur)
	}
}

func TestApplyDrainTimeBased(t *testing.T) {
	intent := Intent{Humor: HumorTesao, Intensidade: 5, DuracaoSegundos: 50}
	after := ApplyDrain(100, intent)
	rate := DrainRatePerSecond(5)
	want := 100 - rate*50
	if math.Abs(after-want) > 0.01 {
		t.Fatalf("after = %.2f, want %.2f", after, want)
	}
}

func TestProceduralBlockDurationDepletesStamina(t *testing.T) {
	stamina := 50.0
	intent := Intent{Humor: HumorTesao, Intensidade: 5}
	dur := ProceduralBlockDuration(stamina, intent)
	after := ApplyProceduralStamina(stamina, intent, float64(dur))
	if after > 0.01 {
		t.Fatalf("expected depletion, got %.2f after %ds", after, dur)
	}
}

func TestProceduralBlockDurationRecoversToFull(t *testing.T) {
	stamina := 40.0
	intent := Intent{Humor: HumorDesejando, Intensidade: 2, Velocidade: 2}
	dur := ProceduralBlockDuration(stamina, intent)
	after := ApplyProceduralStamina(stamina, intent, float64(dur))
	if after < 99.9 {
		t.Fatalf("expected full recovery, got %.2f after %ds", after, dur)
	}
}
