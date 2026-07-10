package chatauto

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AutoSessionContract is appended to the autodom prompt during chat auto mode.
const AutoSessionContract = `Return exactly one JSON object and no markdown.

JSON contract:
{
  "reply": "short user-facing reply (max 2 short sentences)",
  "autodom": {
    "humor": "desejando|tesao|intensa|dominatrix",
    "posicao": "handjob|oral|cavalgando|deepthroat",
    "intensidade": 1,
    "duracao_segundos": 50
  }
}

Rules:
- duracao_segundos must be between 45 and 60 (prefer 50–60 when rhythm is flowing).
- intensidade must be between 1 and 10.
- Plan the NEXT segment while the CURRENT segment is still playing; output is queued for seamless HSP playback.
- Keep the scene flowing; do not stop unless stamina is very low.`

// ParseResponse validates one auto-session model response.
func ParseResponse(raw string) (Response, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Response{}, fmt.Errorf("empty auto response")
	}
	var response Response
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return Response{}, err
	}
	intent, err := ValidateIntent(response.AutoDom)
	if err != nil {
		return Response{}, err
	}
	response.AutoDom = intent
	response.Reply = strings.TrimSpace(response.Reply)
	return response, nil
}

// ValidateIntent clamps and validates one scene intent.
func ValidateIntent(intent Intent) (Intent, error) {
	switch intent.Humor {
	case HumorDesejando, HumorTesao, HumorIntensa, HumorDominatrix:
	default:
		intent.Humor = HumorDesejando
	}
	switch intent.Posicao {
	case PoseHandjob, PoseOral, PoseCavalgando, PoseDeepthroat:
	default:
		intent.Posicao = PoseHandjob
	}
	if intent.Intensidade < 1 {
		intent.Intensidade = 1
	}
	if intent.Intensidade > 10 {
		intent.Intensidade = 10
	}
	if intent.DuracaoSegundos < 45 {
		intent.DuracaoSegundos = 45
	}
	if intent.DuracaoSegundos > 60 {
		intent.DuracaoSegundos = 60
	}
	return intent, nil
}

// FormatUserTurn builds the user message for an autonomous turn.
func FormatUserTurn(state State, transcript []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("stamina=%.0f humor=%s posicao=%s\n", state.Stamina, state.Humor, state.Posicao))
	b.WriteString("Continue the autonomous scene. The CURRENT segment is still playing on the device; your JSON defines the NEXT queued segment.\n")
	if len(transcript) > 0 {
		b.WriteString("transcript:\n")
		for _, line := range transcript {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("Plan the next seamless segment JSON now.\n")
	return b.String()
}
