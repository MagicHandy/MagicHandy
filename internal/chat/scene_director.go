package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

const (
	SceneActionRide  = "ride"
	SceneActionTease = "tease"
	SceneActionPause = "pause"
	SceneActionStop  = "stop"
	SceneActionNone  = "none"
)

// SceneDirectorResponse is the strict LLM output for procedural chat (director only).
type SceneDirectorResponse struct {
	Action      string    `json:"action"`
	Intensity   int       `json:"intensity"`
	StrokeRange []float64 `json:"stroke_range"`
	Dialogue    string    `json:"dialogue"`
}

// SceneDirectorResponseFormat returns JSON-schema constrained output for llama.cpp / Ollama.
func SceneDirectorResponseFormat() *llm.ResponseFormat {
	return &llm.ResponseFormat{
		Type:   "json_schema",
		Name:   "scene_director",
		Strict: true,
		JSONSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{
						SceneActionRide,
						SceneActionTease,
						SceneActionPause,
						SceneActionStop,
						SceneActionNone,
					},
				},
				"intensity": map[string]any{
					"type":    "integer",
					"minimum": 1,
					"maximum": 10,
				},
				"stroke_range": map[string]any{
					"type":     "array",
					"minItems": 2,
					"maxItems": 2,
					"items": map[string]any{
						"type":    "number",
						"minimum": 0,
						"maximum": 1,
					},
				},
				"dialogue": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
			},
			"required":             []string{"action", "intensity", "stroke_range", "dialogue"},
			"additionalProperties": false,
		},
	}
}

// ParseSceneDirectorResponse validates director JSON from the model.
func ParseSceneDirectorResponse(raw string) (SceneDirectorResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SceneDirectorResponse{}, errors.New("scene director response is empty")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response SceneDirectorResponse
	if err := decoder.Decode(&response); err != nil {
		return SceneDirectorResponse{}, fmt.Errorf("scene director response must be strict JSON: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return SceneDirectorResponse{}, errors.New("scene director response must contain exactly one JSON object")
	}
	if err := validateSceneDirectorResponse(&response); err != nil {
		return SceneDirectorResponse{}, err
	}
	return response, nil
}

func validateSceneDirectorResponse(response *SceneDirectorResponse) error {
	response.Action = strings.ToLower(strings.TrimSpace(response.Action))
	response.Dialogue = strings.TrimSpace(response.Dialogue)
	if response.Dialogue == "" {
		return errors.New("dialogue is required")
	}
	switch response.Action {
	case SceneActionRide, SceneActionTease, SceneActionPause, SceneActionStop, SceneActionNone:
	default:
		return fmt.Errorf("unknown scene action %q", response.Action)
	}
	if response.Intensity < 1 || response.Intensity > 10 {
		return errors.New("intensity must be between 1 and 10")
	}
	if len(response.StrokeRange) != 2 {
		return errors.New("stroke_range must contain exactly two values")
	}
	low, high := response.StrokeRange[0], response.StrokeRange[1]
	if low < 0 || high > 1 || low >= high {
		return errors.New("stroke_range must be ascending values within 0..1")
	}
	return nil
}

// ToAssistantResponse maps director JSON into the legacy dispatch contract.
func (response SceneDirectorResponse) ToAssistantResponse(motionRunning bool) AssistantResponse {
	motion := response.ToMotionCommand(motionRunning)
	if motion == nil {
		return AssistantResponse{Reply: response.Dialogue}
	}
	return AssistantResponse{
		Reply:  response.Dialogue,
		Motion: motion,
	}
}

// ToMotionCommand converts semantic scene direction into procedural physics for Go.
func (response SceneDirectorResponse) ToMotionCommand(motionRunning bool) *MotionCommand {
	action := sceneDirectorMotionAction(response.Action, motionRunning)
	if action == MotionActionNone && response.Action != SceneActionStop {
		return nil
	}
	if action == MotionActionStop {
		return &MotionCommand{Action: MotionActionStop}
	}

	physics := sceneDirectorPhysics(response)
	return &MotionCommand{
		Action:      action,
		Velocidade:  physics.Velocidade,
		Intensidade: physics.Intensidade,
		Regiao:      physics.Regiao,
		TipoBatida:  physics.TipoBatida,
		AtrasoMS:    physics.AtrasoMS,
		StrokeRange: append([]float64(nil), response.StrokeRange...),
	}
}

type scenePhysics struct {
	Velocidade  int
	Intensidade int
	Regiao      string
	TipoBatida  string
	AtrasoMS    int
}

func sceneDirectorPhysics(response SceneDirectorResponse) scenePhysics {
	intensity := response.Intensity
	vel := 22 + intensity*8
	physInt := 28 + intensity*7
	tipo := "fluido"
	atraso := 170 - intensity*8
	regiao := regiaoFromStrokeRange(response.StrokeRange)

	switch response.Action {
	case SceneActionTease:
		tipo = "leve"
		vel = maxInt(18, vel-18)
		atraso = minInt(240, atraso+35)
	case SceneActionPause:
		tipo = "lento"
		vel = maxInt(15, vel/2)
		atraso = minInt(280, atraso+60)
		intensity = maxInt(1, intensity-2)
	case SceneActionRide:
		if intensity >= 8 {
			tipo = "alto"
			atraso = maxInt(70, atraso-25)
		} else if intensity >= 5 {
			tipo = "moderado"
		}
	}

	if atraso < 60 {
		atraso = 60
	}
	if atraso > 280 {
		atraso = 280
	}

	return scenePhysics{
		Velocidade:  clampInt(vel, 1, 100),
		Intensidade: clampInt(physInt, 1, 100),
		Regiao:      regiao,
		TipoBatida:  tipo,
		AtrasoMS:    atraso,
	}
}

func sceneDirectorMotionAction(action string, motionRunning bool) string {
	switch action {
	case SceneActionStop:
		return MotionActionStop
	case SceneActionNone:
		return MotionActionNone
	case SceneActionRide, SceneActionTease, SceneActionPause:
		if motionRunning {
			return MotionActionTarget
		}
		return MotionActionStart
	default:
		return MotionActionNone
	}
}

func regiaoFromStrokeRange(strokeRange []float64) string {
	if len(strokeRange) != 2 {
		return "meio_cabeca"
	}
	center := (strokeRange[0] + strokeRange[1]) / 2
	span := strokeRange[1] - strokeRange[0]
	if span >= 0.85 {
		return "full"
	}
	switch {
	case center >= 0.72:
		return "cabeca"
	case center >= 0.45:
		return "meio_cabeca"
	case center >= 0.28:
		return "meio"
	default:
		return "base"
	}
}

// StrokeRangePercents converts normalized stroke_range into Handy 0..100 bounds.
func StrokeRangePercents(strokeRange []float64) (int, int) {
	if len(strokeRange) != 2 {
		return 0, 100
	}
	low := int(math.Round(strokeRange[0] * 100))
	high := int(math.Round(strokeRange[1] * 100))
	return clampInt(low, 0, 100), clampInt(high, 0, 100)
}

func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
