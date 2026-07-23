package chatauto

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Roteiro is the cached scene script: one pose, speed, intensity and mood.
type Roteiro struct {
	Humor       Humor `json:"humor"`
	Posicao     Pose  `json:"posicao"`
	Intensidade int   `json:"intensidade"`
	Velocidade  int   `json:"velocidade"`
}

// RoteiroResponse is the strict JSON shape for a roteiro-only LLM turn.
type RoteiroResponse struct {
	Roteiro Roteiro `json:"autodom"`
}

// ReplyResponse is the strict JSON shape for a chat-only LLM turn.
type ReplyResponse struct {
	Reply string `json:"reply"`
}

// FormatRoteiroContract builds the JSON contract for roteiro generation.
func FormatRoteiroContract() string {
	return `Return exactly one JSON object and no markdown.

JSON contract:
{
  "autodom": {
    "humor": "desejando|tesao|intensa|dominatrix",
    "posicao": "handjob|oral|cavalgando|deepthroat",
    "intensidade": 5,
    "velocidade": 5
  }
}

Rules:
- Define ONE scene script (roteiro) for the next stamina cycle.
- intensidade and velocidade must be between 1 and 10.
- posicao must match the physical action you want for this beat.
- When stamina is high, vary posicao from the current scene when it fits.
- Do NOT output a chat reply — motion is generated procedurally from this script.`
}

// FormatReplyContract builds the JSON contract for chat-only generation.
func FormatReplyContract(roteiro Roteiro, spice SpiceLevel) string {
	return fmt.Sprintf(`Return exactly one JSON object and no markdown.

JSON contract:
{
  "reply": "short user-facing reply (max 2 short sentences)"
}

Active scene script (roteiro) — stay consistent with this:
- humor: %s
- posicao: %s
- intensidade: %d
- velocidade: %d
- spice_level: %s

Rules:
- Write ONLY the chat reply; do NOT change the roteiro.
- Describe actions matching posicao=%s.
- NEVER repeat phrases from recent replies.
- Follow the spice_level tier from the user turn.`,
		roteiro.Humor, roteiro.Posicao, roteiro.Intensidade, roteiro.Velocidade, spice,
		roteiro.Posicao,
	)
}

// ParseRoteiroResponse validates one roteiro-only model response.
func ParseRoteiroResponse(raw string) (Roteiro, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Roteiro{}, fmt.Errorf("empty roteiro response")
	}
	var response RoteiroResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return Roteiro{}, err
	}
	return ValidateRoteiro(response.Roteiro)
}

// ParseReplyResponse validates one chat-only model response.
func ParseReplyResponse(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty reply response")
	}
	var response ReplyResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return "", err
	}
	reply := strings.TrimSpace(response.Reply)
	if reply == "" {
		return "", fmt.Errorf("empty reply")
	}
	return reply, nil
}

// ValidateRoteiro clamps and validates one roteiro.
func ValidateRoteiro(roteiro Roteiro) (Roteiro, error) {
	switch roteiro.Humor {
	case HumorDesejando, HumorTesao, HumorIntensa, HumorDominatrix:
	default:
		roteiro.Humor = HumorDesejando
	}
	switch roteiro.Posicao {
	case PoseHandjob, PoseOral, PoseCavalgando, PoseDeepthroat:
	default:
		roteiro.Posicao = PoseHandjob
	}
	if roteiro.Intensidade < 1 {
		roteiro.Intensidade = 1
	}
	if roteiro.Intensidade > 10 {
		roteiro.Intensidade = 10
	}
	if roteiro.Velocidade < 1 {
		roteiro.Velocidade = 1
	}
	if roteiro.Velocidade > 10 {
		roteiro.Velocidade = 10
	}
	return roteiro, nil
}

// RoteiroToIntent builds a motion intent from roteiro + computed block duration.
func RoteiroToIntent(roteiro Roteiro, durationSec int) Intent {
	if durationSec < 1 {
		durationSec = 1
	}
	return Intent{
		Humor:           roteiro.Humor,
		Posicao:         roteiro.Posicao,
		Intensidade:     roteiro.Intensidade,
		IntensidadeMin:  roteiro.Intensidade,
		IntensidadeMax:  roteiro.Intensidade,
		DuracaoSegundos: durationSec,
	}
}

// PlanFromRoteiro converts roteiro into a procedural scene plan.
func PlanFromRoteiro(roteiro Roteiro) ScenePlan {
	return ScenePlan{
		Humor:          roteiro.Humor,
		Posicao:        roteiro.Posicao,
		IntensidadeMin: roteiro.Intensidade,
		IntensidadeMax: roteiro.Intensidade,
		Velocidade:     roteiro.Velocidade,
	}
}

// FormatRoteiroUserTurn builds the user message for roteiro generation.
func FormatRoteiroUserTurn(state State, transcript []string, nextCycle bool) string {
	var b strings.Builder
	spice := ResolveSpiceLevel(state.Humor, state.MoodProgress, true)
	b.WriteString(fmt.Sprintf("stamina=%.0f humor=%s spice_level=%s posicao=%s mood_progress=%.0f\n",
		state.Stamina, state.Humor, spice, state.Posicao, state.MoodProgress))
	if nextCycle {
		b.WriteString("Plan the NEXT roteiro for when the current stamina cycle ends.\n")
	} else {
		b.WriteString("Plan the roteiro for the current stamina cycle.\n")
	}
	b.WriteString(FormatPoseInstruction(state))
	if len(transcript) > 0 {
		b.WriteString("transcript:\n")
		for _, line := range transcript {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// FormatReplyUserTurn builds the user message for chat-only generation.
func FormatReplyUserTurn(state State, roteiro Roteiro, transcript []string, recentAssistant []string) string {
	var b strings.Builder
	spice := ResolveSpiceLevel(roteiro.Humor, state.MoodProgress, true)
	b.WriteString(fmt.Sprintf("stamina=%.0f mood_progress=%.0f\n", state.Stamina, state.MoodProgress))
	b.WriteString(FormatSpiceTurnInstruction(spice))
	if len(recentAssistant) > 0 {
		b.WriteString("Do NOT echo these recent assistant lines:\n")
		for _, line := range recentAssistant {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if len(trimmed) > 100 {
				trimmed = trimmed[:100] + "…"
			}
			b.WriteString("- ")
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
	}
	if len(transcript) > 0 {
		b.WriteString("transcript:\n")
		for _, line := range transcript {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("Write a fresh reply aligned with the active roteiro.\n")
	return b.String()
}
