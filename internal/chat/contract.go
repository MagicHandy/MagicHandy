// Package chat orchestrates local LLM turns into app-level semantic actions.
//
// Motion dispatch uses two stacks (see docs/procedural-chat-motion-analysis.md):
//   - Library / semantic retarget: motion.Engine via httpapi.dispatchChatMotion
//   - Procedural physics: manualqueue.Player + HSP via httpapi.dispatchChatChaoticMotionAsync
//
// MotionCommand is shared JSON; procedural fields feed the organic stroke generator.
// MotionTargetFromCommand maps engine-oriented metadata for library mode and diagnostics.
package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

const (
	// MotionActionNone leaves motion unchanged.
	MotionActionNone = "none"
	// MotionActionStart starts motion (engine or procedural player depending on mode).
	MotionActionStart = "start"
	// MotionActionTarget retargets running motion.
	MotionActionTarget = "target"
	// MotionActionStop stops motion.
	MotionActionStop = "stop"
)

// AssistantResponse is the only model output shape accepted by MagicHandy.
type AssistantResponse struct {
	Reply  string         `json:"reply"`
	Motion *MotionCommand `json:"motion,omitempty"`
}

// MotionCommand is semantic motion intent, not a transport command.
type MotionCommand struct {
	Action         string `json:"action"`
	PatternID      string `json:"pattern_id,omitempty"`
	LibraryBlockID string `json:"library_block_id,omitempty"`
	SpeedPercent   *int   `json:"speed_percent,omitempty"`
	Estilo         string `json:"estilo,omitempty"`
	PadraoID       string `json:"padrao_id,omitempty"`

	// IntensidadeLegacy preserves the older semantic string contract during repair.
	IntensidadeLegacy string `json:"-"`

	// PhysicalAction carries director semantic action (oral, riding, deepthroat, …).
	// Distinct from Action (start/target/stop dispatch control).
	PhysicalAction string `json:"physical_action,omitempty"`

	// Chaotic procedural physics (procedural mode).
	Velocidade  int    `json:"velocidade,omitempty"`
	Intensidade int    `json:"intensidade,omitempty"`
	Regiao      string `json:"regiao,omitempty"`
	TipoBatida  string `json:"tipo_batida,omitempty"`
	AtrasoMS    int    `json:"atraso_ms,omitempty"`
	// StrokeRange carries scene-director normalized bounds [0..1, 0..1].
	StrokeRange []float64 `json:"stroke_range,omitempty"`
}

// UnmarshalJSON accepts the new integer intensidade physics field and the
// legacy semantic string intensidade used by older prompt contracts.
func (command *MotionCommand) UnmarshalJSON(data []byte) error {
	type motionCommandJSON struct {
		Action         string          `json:"action"`
		PatternID      string          `json:"pattern_id,omitempty"`
		LibraryBlockID string          `json:"library_block_id,omitempty"`
		SpeedPercent   *int            `json:"speed_percent,omitempty"`
		Estilo         string          `json:"estilo,omitempty"`
		PadraoID       string          `json:"padrao_id,omitempty"`
		Velocidade     int             `json:"velocidade,omitempty"`
		Regiao         string          `json:"regiao,omitempty"`
		TipoBatida     string          `json:"tipo_batida,omitempty"`
		AtrasoMS       int             `json:"atraso_ms,omitempty"`
		IntensidadeRaw json.RawMessage `json:"intensidade,omitempty"`
	}

	var payload motionCommandJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	physics, legacy, err := decodeMotionIntensidade(payload.IntensidadeRaw)
	if err != nil {
		return err
	}

	command.Action = payload.Action
	command.PatternID = payload.PatternID
	command.LibraryBlockID = payload.LibraryBlockID
	command.SpeedPercent = payload.SpeedPercent
	command.Estilo = payload.Estilo
	command.PadraoID = payload.PadraoID
	command.Velocidade = payload.Velocidade
	command.Regiao = payload.Regiao
	command.TipoBatida = payload.TipoBatida
	command.AtrasoMS = payload.AtrasoMS
	command.Intensidade = physics
	command.IntensidadeLegacy = legacy
	return nil
}

// ParseAssistantResponse validates one strict JSON response from the model.
func ParseAssistantResponse(raw string) (AssistantResponse, error) {
	return ParseAssistantResponseForMode(raw, config.MotionGenerationModeProcedural)
}

// ParseAssistantResponseForMode validates JSON against the active motion generation mode.
func ParseAssistantResponseForMode(raw string, motionGenerationMode string) (AssistantResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AssistantResponse{}, errors.New("assistant response is empty")
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response AssistantResponse
	if err := decoder.Decode(&response); err != nil {
		if normalizeMotionGenerationMode(motionGenerationMode) == config.MotionGenerationModeProcedural {
			if director, derr := ParseSceneDirectorResponse(raw); derr == nil {
				return director.ToAssistantResponse(false), nil
			}
		}
		return AssistantResponse{}, fmt.Errorf("assistant response must be strict JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return AssistantResponse{}, errors.New("assistant response must contain exactly one JSON object")
	}
	if err := validateAssistantResponse(&response, motionGenerationMode); err != nil {
		return AssistantResponse{}, err
	}
	return response, nil
}

func validateAssistantResponse(response *AssistantResponse, motionGenerationMode string) error {
	response.Reply = strings.TrimSpace(response.Reply)
	if response.Reply == "" {
		return errors.New("assistant response reply is required")
	}
	if response.Motion == nil {
		return nil
	}

	response.Motion.Action = strings.ToLower(strings.TrimSpace(response.Motion.Action))
	response.Motion.PatternID = strings.ToLower(strings.TrimSpace(response.Motion.PatternID))
	response.Motion.IntensidadeLegacy = strings.ToLower(strings.TrimSpace(response.Motion.IntensidadeLegacy))
	response.Motion.Estilo = strings.ToLower(strings.TrimSpace(response.Motion.Estilo))
	response.Motion.PadraoID = strings.ToLower(strings.TrimSpace(response.Motion.PadraoID))
	response.Motion.Regiao = strings.ToLower(strings.TrimSpace(response.Motion.Regiao))
	response.Motion.TipoBatida = strings.ToLower(strings.TrimSpace(response.Motion.TipoBatida))

	switch response.Motion.Action {
	case MotionActionNone, MotionActionStart, MotionActionTarget, MotionActionStop:
	default:
		return fmt.Errorf("unknown motion action %q", response.Motion.Action)
	}

	mode := normalizeMotionGenerationMode(motionGenerationMode)
	switch mode {
	case config.MotionGenerationModeLibrary:
		return validateLibraryMotion(response.Motion)
	case config.MotionGenerationModeSynsual:
		return validateProceduralMotion(response.Motion)
	default:
		return validateProceduralMotion(response.Motion)
	}
}

func validateProceduralMotion(command *MotionCommand) error {
	if strings.TrimSpace(command.LibraryBlockID) != "" || strings.TrimSpace(command.PadraoID) != "" {
		return errors.New("procedural motion cannot include library fields")
	}
	if command.PatternID != "" && !validPatternID(command.PatternID) {
		return fmt.Errorf("unknown motion pattern %q", command.PatternID)
	}
	if command.IntensidadeLegacy != "" && !validProceduralIntensidade(command.IntensidadeLegacy) {
		return fmt.Errorf("unknown motion intensidade %q", command.IntensidadeLegacy)
	}
	if command.Estilo != "" && !validProceduralEstilo(command.Estilo) {
		return fmt.Errorf("unknown motion estilo %q", command.Estilo)
	}
	if command.SpeedPercent != nil {
		speed := *command.SpeedPercent
		if speed < 1 || speed > 100 {
			return errors.New("motion speed_percent must be between 1 and 100")
		}
	}
	if command.Action == MotionActionNone {
		clearProceduralTargetFields(command)
		return nil
	}
	if command.Action == MotionActionStart || command.Action == MotionActionTarget {
		applyLegacyPhysicsFallback(command)
		return validateChaoticPhysics(command)
	}
	return nil
}

func validateLibraryMotion(command *MotionCommand) error {
	if command.PatternID != "" ||
		command.Intensidade != 0 ||
		command.Velocidade != 0 ||
		command.Regiao != "" ||
		command.TipoBatida != "" ||
		command.Estilo != "" ||
		command.SpeedPercent != nil {
		return errors.New("library motion cannot include procedural fields")
	}
	if strings.TrimSpace(command.LibraryBlockID) != "" {
		return nil
	}
	if command.PadraoID == "" {
		if command.Action == MotionActionNone {
			return nil
		}
		return errors.New("library motion requires padrao_id")
	}
	if !validLibraryPatternTag(command.PadraoID) {
		return fmt.Errorf("unknown motion padrao_id %q", command.PadraoID)
	}
	return nil
}

func hasProceduralTargetFields(command *MotionCommand) bool {
	return command.PatternID != "" ||
		command.IntensidadeLegacy != "" ||
		command.Estilo != "" ||
		command.SpeedPercent != nil ||
		command.Velocidade != 0 ||
		command.Intensidade != 0 ||
		command.Regiao != "" ||
		command.TipoBatida != "" ||
		command.AtrasoMS != 0
}

func clearProceduralTargetFields(command *MotionCommand) {
	command.PatternID = ""
	command.IntensidadeLegacy = ""
	command.Estilo = ""
	command.SpeedPercent = nil
	command.Velocidade = 0
	command.Intensidade = 0
	command.Regiao = ""
	command.TipoBatida = ""
	command.AtrasoMS = 0
	command.PadraoID = ""
	command.LibraryBlockID = ""
}

func applyLegacyPhysicsFallback(command *MotionCommand) {
	if command.Intensidade == 0 && command.IntensidadeLegacy != "" {
		command.Intensidade = parseIntensidadeLegacyToPhysics(command.IntensidadeLegacy)
	}
	if command.Velocidade == 0 && command.SpeedPercent != nil {
		command.Velocidade = *command.SpeedPercent
	}
}

func validProceduralIntensidade(value string) bool {
	switch value {
	case "baixa", "media", "alta", "caos":
		return true
	default:
		return false
	}
}

func validProceduralEstilo(value string) bool {
	switch value {
	case "constante", "vibrato", "pulsante":
		return true
	default:
		return false
	}
}

func validLibraryPatternTag(tag string) bool {
	for _, candidate := range LibraryPatternTags {
		if candidate == tag {
			return true
		}
	}
	return false
}

func validPatternID(patternID string) bool {
	switch patternID {
	case "stroke", "pulse", "tease":
		return true
	default:
		return false
	}
}
