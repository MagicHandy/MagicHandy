package chatauto

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SegmentDurationBounds clamps auto segment durations.
type SegmentDurationBounds struct {
	MinSec int
	MaxSec int
}

// DefaultSegmentDurationBounds returns the built-in 45–60 second window.
func DefaultSegmentDurationBounds() SegmentDurationBounds {
	return SegmentDurationBounds{MinSec: 45, MaxSec: 60}
}

// FormatAutoSessionContract builds the JSON contract for the given duration window.
func FormatAutoSessionContract(bounds SegmentDurationBounds) string {
	if bounds.MinSec <= 0 {
		bounds = DefaultSegmentDurationBounds()
	}
	if bounds.MaxSec < bounds.MinSec {
		bounds.MaxSec = bounds.MinSec
	}
	return fmt.Sprintf(`Return exactly one JSON object and no markdown.

JSON contract:
{
  "reply": "short user-facing reply (max 2 short sentences)",
  "autodom": {
    "humor": "desejando|tesao|intensa|dominatrix",
    "posicao": "handjob|oral|cavalgando|deepthroat",
    "intensidade_min": 3,
    "intensidade_max": 7
  }
}

Rules:
- You define the SCENE PLAN (roteiro) only — a procedural motor generates motion and drains stamina.
- Do NOT output duracao_segundos; block length is handled by the motor.
- intensidade_min and intensidade_max must be between 1 and 10 (min <= max).
- Plan the NEXT beat while the CURRENT block is still playing.
- reply: follow the spice_level tier from the user turn (see spice ladder in system prompt).
- NEVER repeat phrases from recent replies.
- posicao must match the physical action described in reply.
- When stamina is above 60, change posicao when the scene calls for it.`)
}

// ParseResponse validates one auto-session model response with default bounds.
func ParseResponse(raw string) (Response, error) {
	return ParseResponseWithBounds(raw, DefaultSegmentDurationBounds())
}

// ParseResponseWithBounds validates one auto-session model response.
func ParseResponseWithBounds(raw string, bounds SegmentDurationBounds) (Response, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Response{}, fmt.Errorf("empty auto response")
	}
	var response Response
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return Response{}, err
	}
	intent, err := ValidateIntentWithBounds(response.AutoDom, bounds)
	if err != nil {
		return Response{}, err
	}
	response.AutoDom = intent
	response.Reply = strings.TrimSpace(response.Reply)
	return response, nil
}

// ValidateIntent clamps and validates one scene intent with default bounds.
func ValidateIntent(intent Intent) (Intent, error) {
	return ValidateIntentWithBounds(intent, DefaultSegmentDurationBounds())
}

// ValidateIntentWithBounds clamps and validates one scene intent.
func ValidateIntentWithBounds(intent Intent, bounds SegmentDurationBounds) (Intent, error) {
	if bounds.MinSec <= 0 {
		bounds = DefaultSegmentDurationBounds()
	}
	if bounds.MaxSec < bounds.MinSec {
		bounds.MaxSec = bounds.MinSec
	}
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
	if intent.IntensidadeMin <= 0 && intent.IntensidadeMax <= 0 {
		if intent.Intensidade > 0 {
			intent.IntensidadeMin = intent.Intensidade
			intent.IntensidadeMax = intent.Intensidade
		} else {
			intent.IntensidadeMin = 4
			intent.IntensidadeMax = 6
		}
	}
	if intent.IntensidadeMin < 1 {
		intent.IntensidadeMin = 1
	}
	if intent.IntensidadeMax > 10 {
		intent.IntensidadeMax = 10
	}
	if intent.IntensidadeMin > intent.IntensidadeMax {
		intent.IntensidadeMin, intent.IntensidadeMax = intent.IntensidadeMax, intent.IntensidadeMin
	}
	intent.Intensidade = (intent.IntensidadeMin + intent.IntensidadeMax) / 2
	if intent.Intensidade < 1 {
		intent.Intensidade = 1
	}
	if intent.Intensidade > 10 {
		intent.Intensidade = 10
	}
	// Procedural motor owns block length; LLM duracao is ignored.
	intent.DuracaoSegundos = bounds.MaxSec
	if intent.DuracaoSegundos < bounds.MinSec {
		intent.DuracaoSegundos = bounds.MinSec
	}
	return intent, nil
}

// FormatUserTurn builds the user message for an autonomous turn.
func FormatUserTurn(state State, transcript []string, recentAssistant []string, allowDominatrix bool) string {
	var b strings.Builder
	spice := ResolveSpiceLevel(state.Humor, state.MoodProgress, allowDominatrix)
	b.WriteString(fmt.Sprintf("stamina=%.0f humor=%s spice_level=%s posicao=%s mood_progress=%.0f\n",
		state.Stamina, state.Humor, spice, state.Posicao, state.MoodProgress))
	b.WriteString(FormatSpiceTurnInstruction(spice))
	b.WriteString(FormatPoseInstruction(state))
	b.WriteString("The CURRENT procedural block is still playing; your JSON defines the NEXT scene plan (roteiro).\n")
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
	b.WriteString("Write a fresh reply (new verbs, new angle). Output the next scene plan JSON (posicao + intensidade_min/max).\n")
	return b.String()
}

// FormatPoseInstruction nudges the model to vary pose and intensity with stamina.
func FormatPoseInstruction(state State) string {
	next := string(NextPose(state.Posicao))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Scene: posicao=%s stamina=%.0f. ", state.Posicao, state.Stamina))
	if state.Stamina > 70 {
		b.WriteString(fmt.Sprintf("Stamina alta — considere mudar posicao (ex: %s) ou subir intensidade. ", next))
	} else if state.Stamina < 25 {
		b.WriteString("Stamina baixa — use intensidade 1-4 e ritmo mais suave. ")
	}
	b.WriteString("Alinhe posicao com a acao do reply; varie entre handjob, oral, cavalgando e deepthroat.\n")
	return b.String()
}
