package chatauto

import "math"

const (
	// StaminaMinDrainPerSec: 1 point every 2 seconds (intensity 1).
	StaminaMinDrainPerSec = 0.5
	// StaminaMaxDrainPerSec: 1 point every 500ms (intensity 10).
	StaminaMaxDrainPerSec = 2.0
)

// DrainRatePerSecond maps intensity 1–10 to stamina points per second (drain magnitude).
func DrainRatePerSecond(intensity int) float64 {
	if intensity < 1 {
		intensity = 1
	}
	if intensity > 10 {
		intensity = 10
	}
	span := StaminaMaxDrainPerSec - StaminaMinDrainPerSec
	return StaminaMinDrainPerSec + (float64(intensity)-1)*span/9
}

// IsRecoveringIntent reports slow/gentle motion that can recover stamina while below 100.
func IsRecoveringIntent(intent Intent) bool {
	if intent.Intensidade > 0 && intent.Intensidade <= 3 {
		return true
	}
	if intent.Velocidade > 0 && intent.Velocidade <= 3 {
		return true
	}
	if intent.Humor == HumorDesejando && intent.Intensidade <= 4 {
		if intent.Velocidade == 0 || intent.Velocidade <= 4 {
			return true
		}
	}
	return false
}

// StaminaNetRatePerSecond is positive when draining, negative when recovering.
// At stamina 100 recovery is impossible — always drain so blocks have real duration.
func StaminaNetRatePerSecond(intent Intent, stamina float64) float64 {
	rate := DrainRatePerSecond(intent.Intensidade)
	if stamina < 100 && IsRecoveringIntent(intent) {
		return -rate
	}
	return rate
}

// ApplyProceduralStamina applies net stamina change over durationSec seconds.
func ApplyProceduralStamina(stamina float64, intent Intent, durationSec float64) float64 {
	if durationSec <= 0 {
		return stamina
	}
	delta := StaminaNetRatePerSecond(intent, stamina) * durationSec
	next := stamina - delta
	if next < 0 {
		return 0
	}
	if next > 100 {
		return 100
	}
	return next
}

// ProceduralBlockDuration returns seconds until stamina hits 0 (drain) or 100 (recovery).
func ProceduralBlockDuration(stamina float64, intent Intent) int {
	rate := StaminaNetRatePerSecond(intent, stamina)
	if rate > 0 {
		if stamina <= 0 {
			return 1
		}
		return int(math.Ceil(stamina / rate))
	}
	if rate < 0 {
		need := 100 - stamina
		if need <= 0 {
			return int(math.Ceil(100 / DrainRatePerSecond(intent.Intensidade)))
		}
		return int(math.Ceil(need / (-rate)))
	}
	return int(math.Ceil(100 / DrainRatePerSecond(intent.Intensidade)))
}

// ProceduralCycleCompleted reports whether a block reached a stamina boundary.
func ProceduralCycleCompleted(staminaBefore, staminaAfter float64, intent Intent) bool {
	rate := StaminaNetRatePerSecond(intent, staminaBefore)
	if rate > 0 && staminaBefore > 0 && staminaAfter <= 0 {
		return true
	}
	if rate < 0 && staminaBefore < 100 && staminaAfter >= 100 {
		return true
	}
	return false
}

// ApplyProceduralDrain applies stamina change over the intent's block duration.
func ApplyProceduralDrain(stamina float64, intent Intent) float64 {
	return ApplyProceduralStamina(stamina, intent, float64(intent.DuracaoSegundos))
}

// ApplyDrain reduces stamina using the time-based rate model.
func ApplyDrain(stamina float64, intent Intent) float64 {
	return ApplyProceduralStamina(stamina, intent, float64(intent.DuracaoSegundos))
}

// ApplyRecover adds stamina after gentle segments (legacy bonus; slow motion uses net rate).
func ApplyRecover(stamina float64, intent Intent) float64 {
	bonus := 0.0
	if intent.Humor == HumorDesejando {
		bonus += 3
	}
	if intent.Humor == HumorTesao && intent.Intensidade <= 4 {
		bonus += 4
	}
	if intent.Posicao == PoseOral && intent.Intensidade <= 3 {
		bonus += 8
	}
	next := stamina + bonus
	if next > 100 {
		return 100
	}
	return next
}
