package chatauto_test

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chatauto"
)

func TestApplyStaminaRotatesPose(t *testing.T) {
	intent := chatauto.Intent{
		Humor:           chatauto.HumorIntensa,
		Posicao:         chatauto.PoseHandjob,
		Intensidade:     10,
		DuracaoSegundos: 30,
	}
	stamina, next := chatauto.ApplyStamina(5, intent)
	if stamina != 100 {
		t.Fatalf("stamina = %v, want 100 after depletion", stamina)
	}
	if next.Posicao != chatauto.PoseOral {
		t.Fatalf("posicao = %q, want oral", next.Posicao)
	}
}
