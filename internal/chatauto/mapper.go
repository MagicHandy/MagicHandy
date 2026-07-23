package chatauto

import (
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// MappedSegment is procedural physics for one auto segment.
type MappedSegment struct {
	Physics        motion.ChaoticPhysics
	DurationMillis int64
}

// MapIntent converts scene intent into procedural chaotic physics.
func MapIntent(intent Intent, stamina float64) MappedSegment {
	intensity := intent.Intensidade
	if stamina < 30 && intensity > 4 {
		intensity = 4
	}
	if stamina < 15 && intensity > 2 {
		intensity = 2
	}

	velocidade := 25 + intensity*7
	if intent.Velocidade > 0 {
		velocidade = 15 + intent.Velocidade*8
	}
	physicsIntensity := 30 + intensity*6
	regiao := "meio_cabeca"
	tipo := "fluido"
	atraso := 150 - intensity*4
	if atraso < 95 {
		atraso = 95
	}
	if atraso > 210 {
		atraso = 210
	}

	switch intent.Posicao {
	case PoseOral, PoseDeepthroat:
		regiao = "cabeca"
	case PoseCavalgando:
		regiao = "meio_cabeca"
	case PoseHandjob:
		regiao = "meio_cabeca"
	}

	switch intent.Humor {
	case HumorDesejando:
		tipo = "lento"
		velocidade = max(20, velocidade-15)
		atraso = min(280, atraso+50)
	case HumorTesao:
		tipo = "fluido"
		atraso = min(220, atraso+20)
	case HumorIntensa:
		tipo = "moderado"
		velocidade = min(100, velocidade+10)
		atraso = max(100, atraso-25)
	case HumorDominatrix:
		tipo = "alto"
		velocidade = min(100, velocidade+25)
		physicsIntensity = min(100, physicsIntensity+20)
		atraso = max(70, atraso-45)
	}

	if intent.Intensidade >= 8 {
		tipo = "alto"
		atraso = max(55, atraso-30)
	}

	return MappedSegment{
		Physics: motion.ChaoticPhysics{
			Velocidade:  velocidade,
			Intensidade: physicsIntensity,
			Regiao:      regiao,
			TipoBatida:  tipo,
			AtrasoMS:    atraso,
		},
		DurationMillis: int64(intent.DuracaoSegundos) * 1000,
	}
}

// MotionFromMapped builds public motion choice metadata for the UI.
func MotionFromMapped(mapped MappedSegment) MotionChoice {
	return MotionChoice{
		Action:      "start",
		Velocidade:  mapped.Physics.Velocidade,
		Intensidade: mapped.Physics.Intensidade,
		Regiao:      mapped.Physics.Regiao,
		TipoBatida:  mapped.Physics.TipoBatida,
		AtrasoMS:    mapped.Physics.AtrasoMS,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
