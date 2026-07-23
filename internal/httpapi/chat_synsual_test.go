package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
)

func TestShouldUseChaoticChatMotionSynsualMode(t *testing.T) {
	s := &Server{}
	settings := config.Settings{
		Motion: config.MotionSettings{
			MotionGenerationMode: config.MotionGenerationModeSynsual,
		},
	}
	command := &chat.MotionCommand{
		Action:      chat.MotionActionStart,
		Regiao:      "cabeca",
		TipoBatida:  "leve",
		Velocidade:  30,
		Intensidade: 40,
	}
	if !s.shouldUseChaoticChatMotion(command, settings) {
		t.Fatal("expected synsual mode to use procedural chaotic dispatch")
	}
}
