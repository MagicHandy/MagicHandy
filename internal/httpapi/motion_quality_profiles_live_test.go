//go:build liveeval && magichandy_labs

package httpapi

import (
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Hold the same model, capability, context and provider for every scenario.
// Local catalog selection uses the installed digest and the app's template fix.
type qualityDefaultProfile struct {
	settings config.LLMSettings
	provider llm.Provider
	window   int
	messages int
	digest   string
	template string
}

func loadQualityDefault(t *testing.T, name string) *qualityDefaultProfile {
	t.Helper()
	if name == "terra" {
		return qualityTerraDefault(t)
	}
	ids := map[string]string{"gemma-e4b": "gemma-4-e4b-qat-heretic-q4-0", "gemma-12b": "gemma-4-12b-heretic-q4km"}
	entry, ok := llm.FindCatalogModel(ids[name])
	if !ok {
		t.Fatalf("unknown default profile %q (choose gemma-e4b, gemma-12b or terra)", name)
	}
	return qualityGemmaDefault(t, entry)
}

func qualityTerraDefault(t *testing.T) *qualityDefaultProfile {
	t.Helper()
	settings := config.DefaultSettings().LLM
	const model = "gpt-6.1-terra"
	directory := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR")
	if directory == "" {
		t.Fatal("Terra requires an existing signed-in MAGICHANDY_HOSTED_DATA_DIR")
	}
	options := creativeReviewChatGPTOptions(t, directory, model)
	discovery, err := llm.NewOpenAIResponsesProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	models, err := discovery.Models(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(models, func(item llm.CloudModel) bool { return item.Slug == model })
	if i < 0 {
		t.Fatal("Terra is unavailable in the signed-in account catalog")
	}
	// New ChatGPT connections use Advanced. Setup offers 128K when the
	// catalog advertises it, and selects Low when the model supports it.
	connection := config.ModelConnection{ID: "quality", Name: "ChatGPT", Provider: config.LLMProviderChatGPT, Model: model}
	if models[i].ContextWindow >= 131072 {
		connection.ContextWindow = 131072
	}
	if slices.ContainsFunc(models[i].SupportedReasoningLevels, func(level llm.ReasoningLevel) bool { return level.Effort == "low" }) {
		connection.ReasoningEffort = "low"
	}
	connection = connection.Normalize()
	window, messages, _ := connection.ConversationWindow()
	options.ContextSize, options.ReasoningEffort = window, connection.ReasoningEffort
	provider, err := llm.NewOpenAIResponsesProvider(options)
	if err != nil {
		t.Fatal(err)
	}
	settings.Connections, settings.ConversationConnectionID = []config.ModelConnection{connection}, connection.ID
	return &qualityDefaultProfile{settings: settings, provider: provider, window: window, messages: messages}
}

func qualityGemmaDefault(t *testing.T, entry llm.CatalogModel) *qualityDefaultProfile {
	t.Helper()
	settings := config.DefaultSettings().LLM
	dir, runner := os.Getenv("MAGICHANDY_QUALITY_MODEL_DIR"), os.Getenv("MAGICHANDY_QUALITY_RUNNER")
	if dir == "" || runner == "" {
		t.Fatal("local defaults require MAGICHANDY_QUALITY_MODEL_DIR and MAGICHANDY_QUALITY_RUNNER")
	}
	manager, err := llm.OpenModelManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	})
	id, err := manager.CatalogModelInstalled(t.Context(), entry)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatalf("the evaluation model store needs the catalog download %s (%s)", entry.DisplayName, entry.SHA256)
	}
	record, err := manager.Model(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var template string
	if record.TemplateFix != "" {
		template, err = manager.TemplateFixFile(record.TemplateFix)
		if err != nil {
			t.Fatal(err)
		}
	}
	settings.Model, settings.LocalCapabilityLevel = id, entry.RecommendedCapability
	settings.LlamaCPPContextSize = settings.LocalContextSize()
	settings.MaxOutputTokens = max(settings.MaxOutputTokens, config.MinimumLocalOutput(settings.LocalCapabilityLevel))
	opts := llm.ManagedLlamaCPPOptions{
		HTTPProviderOptions: llm.HTTPProviderOptions{
			BaseURL: settings.LlamaCPPBaseURL, Model: id,
			Timeout: time.Duration(settings.RequestTimeoutMillis) * time.Millisecond,
		},
		RunnerPath: runner, ModelPath: record.ModelPath,
		ChatTemplateFile: template, ContextSize: settings.LocalContextSize(),
	}
	provider, err := llm.NewManagedLlamaCPPProvider(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Close(); err != nil {
			t.Error(err)
		}
	})
	if status := provider.Load(t.Context()); !status.Available {
		t.Fatal(status.Message)
	}
	messages := max(chat.PromptHistoryLimit, chatCapabilities(settings, nil).HistoryMessages)
	return &qualityDefaultProfile{settings: settings, provider: provider, window: settings.LocalContextSize(), messages: messages, digest: record.SHA256, template: record.TemplateFix}
}

// Put a corrected reference near the end of the default local history, and
// use a separate negative control beyond the selected profile's actual cap.
func (p *qualityDefaultProfile) historyPairs(scenario string, pairs int) int {
	switch scenario {
	case "recall_outside":
		return max(pairs, p.messages/2+4)
	case "recall_long", "recall_readback", "long_unrelated":
		if os.Getenv("MAGICHANDY_QUALITY_HISTORY_PAIRS") == "" {
			return min(pairs, (p.messages-12)/2)
		}
	}
	return pairs
}

func qualityAcknowledgementCases(mirror bool) []qualityTurn {
	turns, _ := qualityRecallCases(mirror)
	name, lo, hi, speed := qualityCue(mirror)
	turns[0].request = "For later, call smooth full sweeps from 10 to 85 at overall speed 44 Harbor. Please acknowledge only and keep the simulator idle."
	turns[1].request = "Correction to Harbor: the lower endpoint is 24, the upper endpoint is 66 and the overall speed is 31. Keep that definition for later and stay idle."
	if mirror {
		turns[0].request = "I'm writing a card for a later session. Under Orchard, write full sweeps from 10 to 85 at pace 44. Just confirm you've noted it. Nothing should run now."
		turns[1].request = fmt.Sprintf("Revise the card for %s to %d through %d at overall pace %d. This is a note, so leave playback stopped.", name, lo, hi, speed)
	}
	// This is a short conversation (no seeded intervening messages).
	turns[3].id = "apply"
	return turns
}
