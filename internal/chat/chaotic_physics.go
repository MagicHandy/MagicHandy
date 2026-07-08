package chat

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultChaoticRegiao     = "meio_cabeca"
	defaultChaoticTipoBatida = "fluido"
	defaultChaoticVelocity   = 50
	defaultChaoticIntensity  = 50
)

var validChaoticRegioes = map[string]struct{}{
	"cabeca":      {},
	"meio":        {},
	"base":        {},
	"cabeca_base": {},
	"meio_cabeca": {},
	"meio_base":   {},
	"full":        {},
	"completo":    {},
	"aleatoria":   {},
}

var validChaoticTiposBatida = map[string]struct{}{
	"simples":    {},
	"leve":       {},
	"moderado":   {},
	"alto":       {},
	"fluido":     {},
	"lento":      {},
	"very_fast":  {},
	"vibrate":    {},
	"turbo":      {},
}

// NormalizeChaoticPhysics fills safe defaults for procedural chaotic motion fields.
func NormalizeChaoticPhysics(command *MotionCommand) {
	if command == nil {
		return
	}
	command.Regiao = normalizeChaoticRegiao(command.Regiao)
	command.TipoBatida = strings.ToLower(strings.TrimSpace(command.TipoBatida))

	if _, ok := validChaoticRegioes[command.Regiao]; !ok {
		command.Regiao = defaultChaoticRegiao
	}
	if _, ok := validChaoticTiposBatida[command.TipoBatida]; !ok {
		command.TipoBatida = defaultChaoticTipoBatida
	}
	if command.Velocidade < 0 {
		command.Velocidade = 0
	}
	if command.Velocidade > 100 {
		command.Velocidade = 100
	}
	if command.Velocidade == 0 {
		command.Velocidade = defaultChaoticVelocity
	}
	if command.Intensidade < 0 {
		command.Intensidade = 0
	}
	if command.Intensidade > 100 {
		command.Intensidade = 100
	}
	if command.Intensidade == 0 {
		command.Intensidade = defaultChaoticIntensity
	}
	if command.AtrasoMS < 0 {
		command.AtrasoMS = 0
	}
	if command.AtrasoMS > 500 {
		command.AtrasoMS = 500
	}
}

// HasChaoticPhysicsIntent reports whether a motion command carries procedural physics.
func HasChaoticPhysicsIntent(command *MotionCommand) bool {
	if command == nil {
		return false
	}
	switch command.Action {
	case MotionActionStart, MotionActionTarget:
		return true
	default:
		return false
	}
}

func validateChaoticPhysics(command *MotionCommand) error {
	NormalizeChaoticPhysics(command)
	if command.Velocidade < 1 || command.Velocidade > 100 {
		return fmt.Errorf("motion velocidade must be between 1 and 100")
	}
	if command.Intensidade < 1 || command.Intensidade > 100 {
		return fmt.Errorf("motion intensidade must be between 1 and 100")
	}
	if _, ok := validChaoticRegioes[command.Regiao]; !ok {
		return fmt.Errorf("unknown motion regiao %q", command.Regiao)
	}
	if _, ok := validChaoticTiposBatida[command.TipoBatida]; !ok {
		return fmt.Errorf("unknown motion tipo_batida %q", command.TipoBatida)
	}
	return nil
}

func decodeMotionIntensidade(raw json.RawMessage) (physics int, legacy string, err error) {
	if len(raw) == 0 {
		return 0, "", nil
	}
	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return asInt, "", nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return 0, strings.ToLower(strings.TrimSpace(asString)), nil
	}
	return 0, "", fmt.Errorf("motion intensidade must be an integer or legacy semantic string")
}

func parseIntensidadeLegacyToPhysics(value string) int {
	switch value {
	case "baixa":
		return 25
	case "media":
		return 50
	case "alta":
		return 75
	case "caos":
		return 95
	default:
		return 0
	}
}

func normalizeChaoticRegiao(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"á", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o",
		"ú", "u",
		"ç", "c",
	).Replace(value)

	aliases := map[string]string{
		"cabeca": "cabeca", "cabeça": "cabeca",
		"head": "cabeca", "tip": "cabeca", "topo": "cabeca", "ponta": "cabeca", "top": "cabeca",
		"shaft": "meio", "middle": "meio", "mid": "meio",
		"fundo": "base", "bottom": "base",
		"completo": "full", "stroke_completo": "full", "fullstroke": "full",
		"random": "aleatoria", "aleatorio": "aleatoria",
	}
	if mapped, ok := aliases[value]; ok {
		return mapped
	}
	return value
}

func parseLooseInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
