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
	return strings.TrimSpace(`Instrução de Movimento (Física Procedural):
No objeto JSON de "motion", defina a física da cena usando estas chaves:
- "velocidade" (int: 0 a 100): Velocidade geral da ação.
- "intensidade" (int: 0 a 100): Quanto a curva acelera no fim do golpe.
- "regiao" (string): Zona do stroke. Aceita:
  - "cabeca" (70–100%) — prefira para cenas ativas, oral, ritmo intenso no topo
  - "meio" (30–69%)
  - "base" (0–29%) — use só quando a cena pedir explicitamente fundo/base
  - "full" ou "cabeca_base" (0–100%, stroke completo — use com moderação)
  - "meio_cabeca" (30–100%) — ótimo padrão, mistura meio e cabeça
  - "meio_base" (0–69%)
  - "aleatoria"
- "tipo_batida" (string): Perfil do movimento. Aceita:
  - "fluido" (curvas longas na zona — padrão natural)
  - "lento" (micro-stroke lento na zona — use para pausa/respiração na cena, NÃO pare o device)
  - "simples" (um golpe direto na zona)
  - "leve", "moderado", "alto" (intensidade crescente dentro da zona)
  - "very_fast" ou "vibrate" (turbo/vibração rápida na zona)
- "atraso_ms" (int, opcional): Suavização entre pontos da curva, como no mouse tracker.
  - 160 = fluido (recomendado para sessões naturais)
  - 80–120 = moderado
  - 1 = turbo/vibração (somente com tipo_batida very_fast/vibrate)

JSON contract (procedural):
{
  "reply": "short user-facing reply",
  "motion": {
    "action": "none|start|target|stop",
    "velocidade": 50,
    "intensidade": 60,
    "regiao": "meio_cabeca",
    "tipo_batida": "fluido",
    "atraso_ms": 160
  }
}

Rules:
- Omit "motion" or set {"action":"none"} when the user is only chatting.
- Use "start" only when the user asks to begin motion.
- Use "target" to change ritmo/zona/perfil durante a cena.
- Use "stop" ONLY quando o usuário pedir para parar de verdade. Pausa, descanso ou respiração = "target" com "tipo_batida":"lento" ou "fluido" e "atraso_ms":160.
- Golpes devem usar a zona inteira (ex: cabeca = 70→100→70), nunca micromovimentos no meio da zona.
- Varie "regiao" entre targets: alterne cabeca, meio_cabeca e meio. Evite ficar preso em base ou full por muito tempo.
- Use only the physics keys above for start/target; never invent device commands or transport details.`)
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
