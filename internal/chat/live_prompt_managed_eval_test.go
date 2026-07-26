//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLivePromptParityManagedGemma(t *testing.T) {
	dataDir, err := filepath.Abs(filepath.Join("..", "..", "data"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := llm.InspectManagedLlamaRuntime(dataDir)
	if !runtime.Installed {
		t.Fatalf("managed llama.cpp is not installed: %s", runtime.Message)
	}

	metadataPaths, err := filepath.Glob(filepath.Join(dataDir, "models", "gguf", "*", "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var selected llm.ModelRecord
	for _, metadataPath := range metadataPaths {
		data, readErr := os.ReadFile(metadataPath) // #nosec G304 -- fixed app-owned model directory.
		if readErr != nil {
			continue
		}
		var candidate llm.ModelRecord
		if json.Unmarshal(data, &candidate) != nil {
			continue
		}
		description := strings.ToLower(candidate.DisplayName + " " + candidate.Family + " " + candidate.SourceName)
		if strings.Contains(description, "gemma") {
			selected = candidate
			break
		}
	}
	if selected.ID == "" || selected.ModelPath == "" {
		t.Fatal("managed model store contains no usable Gemma model metadata")
	}

	provider, err := llm.NewManagedLlamaCPPProvider(llm.ManagedLlamaCPPOptions{
		HTTPProviderOptions: llm.HTTPProviderOptions{
			BaseURL: liveEvalLlamaURL,
			Model:   selected.ID,
			Timeout: 2 * time.Minute,
		},
		RunnerPath: runtime.RunnerPath,
		ModelPath:  selected.ModelPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			t.Errorf("close managed llama.cpp: %v", closeErr)
		}
	}()
	loadContext, cancelLoad := context.WithTimeout(context.Background(), 2*time.Minute)
	status := provider.Load(loadContext)
	cancelLoad()
	if !status.Available {
		t.Fatalf("load managed model %q: %s", selected.DisplayName, status.Message)
	}
	t.Logf("managed llama.cpp model=%s id=%s base_url=%s", selected.DisplayName, selected.ID, status.BaseURL)

	promptSet, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	patterns := []PatternChoice{
		{ID: "steady_wave", Name: "Steady wave", Description: "Smooth full-range motion.", Weight: 1},
		{ID: "slow_squeeze", Name: "Slow squeeze", Description: "Gradual pressure-focused motion.", Weight: 1},
		{ID: "playful_tease", Name: "Playful tease", Description: "Variable shallow-to-mid teasing.", Weight: 1},
		{ID: "deep_roll", Name: "Deep roll", Description: "Long rolling strokes with deep emphasis.", Weight: 1},
	}
	motionContext := MotionContext{
		Running:         true,
		PatternID:       "steady_wave",
		SpeedPercent:    30,
		Area:            AreaZoneFull,
		SpeedMinPercent: 20,
		SpeedMaxPercent: 80,
	}
	conversationContext := ConversationContext{
		PersonaDescription: "An energetic and passionate partner",
		UserAnatomy:        "penis",
		CurrentMood:        MoodTeasing,
	}
	inputs := []string{
		"Start slow and talk to me while you do it.",
		"Tell me what you're doing to me right now.",
		"Tease the tip and make me beg for it.",
		"Faster - I'm getting close.",
	}

	var explicitResults []livePromptResult
	for _, level := range []VoiceLevel{VoiceWarm, VoiceIntimate, VoiceExplicit} {
		capabilities := FullCapabilities()
		capabilities.Voice = level
		capabilities.MoodTracking = true
		system := composeSystem(promptSet, nil, patterns, capabilities, &motionContext, &conversationContext)
		results := runLivePromptCases(t, provider, selected.ID, "magichandy/"+string(level), system, inputs)
		assertLiveVoiceLevel(t, level, results)
		if level == VoiceExplicit {
			explicitResults = results
		}
	}
	referenceResults := runLivePromptCases(t, provider, selected.ID, "stgpt-rv/revibed", stgptRVReferencePrompt(), inputs)
	assertLiveExplicitParity(t, explicitResults, referenceResults)
}
