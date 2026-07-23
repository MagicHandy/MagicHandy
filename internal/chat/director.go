package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/motion/semantic"
)

const directorSystemPrompt = `You classify the user's message into physical motion intent.
Reply with strict JSON only: action, location, intensity (1-10).
Actions: oral, handjob, riding, titjob, deepthroat.
Locations: base, shaft, tip, full.`

// DirectorResponseFormat returns JSON-schema constrained output for director calls.
func DirectorResponseFormat() *llm.ResponseFormat {
	return &llm.ResponseFormat{
		Type:   "json_schema",
		Name:   "motion_director",
		Strict: true,
		JSONSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{
						string(semantic.ActionOral),
						string(semantic.ActionHandjob),
						string(semantic.ActionRiding),
						string(semantic.ActionTitjob),
						string(semantic.ActionDeepthroat),
					},
				},
				"location": map[string]any{
					"type": "string",
					"enum": []string{
						string(semantic.LocationBase),
						string(semantic.LocationShaft),
						string(semantic.LocationTip),
						string(semantic.LocationFull),
					},
				},
				"intensity": map[string]any{
					"type":    "integer",
					"minimum": 1,
					"maximum": 10,
				},
			},
			"required":             []string{"action", "location", "intensity"},
			"additionalProperties": false,
		},
	}
}

// AskDirector performs a fast constrained JSON call and returns validated intent.
func AskDirector(
	ctx context.Context,
	provider llm.Provider,
	model string,
	userMessage string,
	history []llm.Message,
) (semantic.LLMIntent, time.Duration, error) {
	start := time.Now()
	if provider == nil {
		return semantic.LLMIntent{}, 0, errors.New("LLM provider is required")
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return semantic.LLMIntent{}, 0, errors.New("chat message is required")
	}

	messages := []llm.Message{{Role: "system", Content: directorSystemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	request := llm.ChatRequest{
		Messages:       messages,
		Model:          model,
		Temperature:    0.1,
		MaxTokens:      64,
		ResponseFormat: DirectorResponseFormat(),
	}
	raw, err := provider.StreamChat(ctx, request, nil)
	latency := time.Since(start)
	if err != nil {
		return semantic.LLMIntent{}, latency, err
	}

	intent, err := parseDirectorResponse(raw)
	if err == nil {
		return intent, latency, nil
	}

	repairMessages := []llm.Message{
		{Role: "system", Content: directorSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("Fix this JSON:\n%s\nError: %s", raw, err.Error())},
	}
	repairRequest := llm.ChatRequest{
		Messages:       repairMessages,
		Model:          model,
		Temperature:    0,
		MaxTokens:      64,
		ResponseFormat: DirectorResponseFormat(),
	}
	repairRaw, repairErr := provider.StreamChat(ctx, repairRequest, nil)
	latency = time.Since(start)
	if repairErr != nil {
		return semantic.LLMIntent{}, latency, err
	}
	intent, parseErr := parseDirectorResponse(repairRaw)
	if parseErr != nil {
		return semantic.LLMIntent{}, latency, parseErr
	}
	return intent, latency, nil
}

func parseDirectorResponse(raw string) (semantic.LLMIntent, error) {
	raw = strings.TrimSpace(raw)
	var intent semantic.LLMIntent
	if err := json.Unmarshal([]byte(raw), &intent); err != nil {
		return semantic.LLMIntent{}, fmt.Errorf("director response must be JSON: %w", err)
	}
	if err := semantic.NormalizeLLMIntent(&intent); err != nil {
		return semantic.LLMIntent{}, err
	}
	return intent, nil
}

// MotionCommandFromLLMIntent maps director intent into the procedural dispatch contract.
func MotionCommandFromLLMIntent(
	intent semantic.LLMIntent,
	prefs semantic.MotionPreferences,
	motionRunning bool,
) (*MotionCommand, error) {
	min, max, err := semantic.ResolveMotionBounds(intent, prefs)
	if err != nil {
		return nil, err
	}
	vel := 22 + intent.Intensity*8
	physInt := 28 + intent.Intensity*7
	action := MotionActionStart
	if motionRunning {
		action = MotionActionTarget
	}
	return &MotionCommand{
		Action:         action,
		PhysicalAction: string(intent.Action),
		Velocidade:     vel,
		Intensidade:    physInt,
		Regiao:         semantic.LocationToRegiao(intent.Location),
		TipoBatida:     directorTipoBatida(intent.Action),
		AtrasoMS:       170 - intent.Intensity*8,
		StrokeRange:    []float64{min, max},
	}, nil
}

func directorTipoBatida(action semantic.ActionName) string {
	switch action {
	case semantic.ActionHandjob:
		return "simples"
	case semantic.ActionTitjob:
		return "leve"
	case semantic.ActionRiding:
		return "fluido"
	default:
		return "fluido"
	}
}

// BuildChaoticPhysicsFromIntent builds motion.ChaoticPhysics for organic generation.
func BuildChaoticPhysicsFromIntent(
	intent semantic.LLMIntent,
	prefs semantic.MotionPreferences,
) (motion.ChaoticPhysics, error) {
	min, max, err := semantic.ResolveMotionBounds(intent, prefs)
	if err != nil {
		return motion.ChaoticPhysics{}, err
	}
	cmd, err := MotionCommandFromLLMIntent(intent, prefs, false)
	if err != nil {
		return motion.ChaoticPhysics{}, err
	}
	physics := ChaoticPhysicsFromCommand(cmd)
	physics.StrokeRangeMin = min
	physics.StrokeRangeMax = max
	physics.StrokeProfile = semantic.ResolveStrokeProfile(intent)
	return physics, nil
}
