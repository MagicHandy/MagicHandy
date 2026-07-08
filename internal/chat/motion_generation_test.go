package chat

import (
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestComposeSystemForLibraryMode(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	composed := ComposeSystemForMode(set, nil, config.MotionGenerationModeLibrary)

	if !strings.Contains(composed, "padrao_id") {
		t.Fatalf("library instructions missing padrao_id:\n%s", composed)
	}
	if !strings.Contains(composed, "rapido_curto") {
		t.Fatalf("library instructions missing tags:\n%s", composed)
	}
}

func TestComposeSystemForProceduralModeIncludesChaoticPhysics(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	composed := ComposeSystemForMode(set, nil, config.MotionGenerationModeProcedural)

	for _, key := range []string{"velocidade", "intensidade", "regiao", "tipo_batida"} {
		if !strings.Contains(composed, key) {
			t.Fatalf("procedural instructions missing %q:\n%s", key, composed)
		}
	}
}

func TestParseAssistantResponseProceduralMode(t *testing.T) {
	raw := `{"reply":"ok","motion":{"action":"start","velocidade":70,"intensidade":80,"regiao":"cabeca","tipo_batida":"moderado"}}`
	response, err := ParseAssistantResponseForMode(raw, config.MotionGenerationModeProcedural)
	if err != nil {
		t.Fatalf("parse procedural response: %v", err)
	}
	if response.Motion.Velocidade != 70 ||
		response.Motion.Intensidade != 80 ||
		response.Motion.Regiao != "cabeca" ||
		response.Motion.TipoBatida != "moderado" {
		t.Fatalf("motion = %+v", response.Motion)
	}
}

func TestParseAssistantResponseProceduralModeLegacyIntensidade(t *testing.T) {
	raw := `{"reply":"ok","motion":{"action":"start","intensidade":"alta","estilo":"vibrato"}}`
	response, err := ParseAssistantResponseForMode(raw, config.MotionGenerationModeProcedural)
	if err != nil {
		t.Fatalf("parse legacy procedural response: %v", err)
	}
	if response.Motion.IntensidadeLegacy != "alta" {
		t.Fatalf("legacy intensidade = %q", response.Motion.IntensidadeLegacy)
	}
	if response.Motion.Intensidade != 75 {
		t.Fatalf("physics intensidade = %d, want 75", response.Motion.Intensidade)
	}
}

func TestParseAssistantResponseProceduralModeFallsBackInvalidRegion(t *testing.T) {
	raw := `{"reply":"ok","motion":{"action":"target","velocidade":50,"intensidade":50,"regiao":"invalida","tipo_batida":"desconhecido"}}`
	response, err := ParseAssistantResponseForMode(raw, config.MotionGenerationModeProcedural)
	if err != nil {
		t.Fatalf("parse procedural response with invalid physics: %v", err)
	}
	if response.Motion.Regiao != defaultChaoticRegiao {
		t.Fatalf("regiao = %q, want %q", response.Motion.Regiao, defaultChaoticRegiao)
	}
	if response.Motion.TipoBatida != defaultChaoticTipoBatida {
		t.Fatalf("tipo_batida = %q, want %q", response.Motion.TipoBatida, defaultChaoticTipoBatida)
	}
}

func TestParseAssistantResponseLibraryMode(t *testing.T) {
	raw := `{"reply":"ok","motion":{"action":"start","padrao_id":"caotico"}}`
	response, err := ParseAssistantResponseForMode(raw, config.MotionGenerationModeLibrary)
	if err != nil {
		t.Fatalf("parse library response: %v", err)
	}
	if response.Motion.PadraoID != "caotico" {
		t.Fatalf("padrao_id = %q", response.Motion.PadraoID)
	}
}

func TestMotionTargetFromCommandProcedural(t *testing.T) {
	command := &MotionCommand{IntensidadeLegacy: "alta", Estilo: "pulsante"}
	target := MotionTargetFromCommand(command, motion.ActiveMotionState{}, config.MotionGenerationModeProcedural)
	if target.SpeedPercent != 75 {
		t.Fatalf("speed = %d, want 75", target.SpeedPercent)
	}
	if target.PatternID != motion.PatternTease {
		t.Fatalf("pattern = %q, want tease", target.PatternID)
	}
}
