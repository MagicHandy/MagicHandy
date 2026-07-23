package chat

import (
	"fmt"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

// LibraryPatternTags are the physical pattern tags available for library-mode LLM selection.
var LibraryPatternTags = []string{
	"rapido_curto",
	"lento_profundo",
	"caotico",
	"constante",
	"vibrato_intenso",
	"pulsante",
	"aleatorio",
}

func normalizeMotionGenerationMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case config.MotionGenerationModeLibrary:
		return config.MotionGenerationModeLibrary
	case config.MotionGenerationModeSynsual:
		return config.MotionGenerationModeSynsual
	default:
		return config.MotionGenerationModeProcedural
	}
}

func motionInstructionsForMode(mode string) string {
	switch normalizeMotionGenerationMode(mode) {
	case config.MotionGenerationModeLibrary:
		return libraryMotionInstructions()
	default:
		return proceduralMotionInstructions()
	}
}

func proceduralMotionInstructions() string {
	return strings.TrimSpace(`Você é o Diretor de Cena. NÃO calcule posições, waypoints, arrays de movimento nem comandos de device.
Retorne APENAS um objeto JSON com a física semântica da cena. O backend Go gera todo o movimento orgânico.

JSON contract (strict):
{
  "action": "ride|tease|pause|stop|none",
  "intensity": 1-10,
  "stroke_range": [0.1, 0.9],
  "dialogue": "short in-character reply"
}

Fields:
- "action":
  - "ride" — movimento contínuo principal (iniciar ou ajustar cena ativa)
  - "tease" — provocação leve, curto alcance
  - "pause" — desacelerar / respirar (NÃO use stop)
  - "stop" — parar de verdade quando o usuário pedir
  - "none" — só diálogo, sem mudar movimento
- "intensity" (1–10): energia da cena. 1 = muito suave, 10 = intenso.
- "stroke_range": [min, max] frações normalizadas 0..1 do eixo do device.
  Ex.: [0.1, 0.9] = stroke longo; [0.7, 0.95] = foco na cabeça.
- "dialogue": resposta curta para o usuário.

Rules:
- NEVER output position arrays, timestamps, velocidade, regiao, tipo_batida, or motion blocks.
- NEVER output legacy {"reply","motion"} shape.
- Rest/pause in narrative = "pause", not "stop".
- Prefer stroke_range changes over stop/start when shifting mood mid-scene.`)
}

func libraryMotionInstructions() string {
	tags := strings.Join(LibraryPatternTags, ", ")
	return strings.TrimSpace(fmt.Sprintf(`Instrução de Movimento: O usuário optou pela biblioteca de padrões importados. No objeto JSON de 'Movimento', você deve escolher um padrão real pré-programado. Use a chave 'padrao_id' e preencha exclusivamente com uma das seguintes tags disponíveis que melhor se adeque à cena física gerada: %s.

JSON contract (library):
{
  "reply": "short user-facing reply",
  "motion": {
    "action": "none|start|target|stop",
    "padrao_id": "one_of_the_tags_above"
  }
}

Rules:
- Omit "motion" or set {"action":"none"} when the user is only chatting.
- Use "start" only when the user asks to begin motion.
- Use "target" only to adjust active motion.
- Use "stop" when the user asks to stop, pause, or end motion.
- padrao_id must be exactly one of the listed tags; never invent new tags.`, tags))
}
