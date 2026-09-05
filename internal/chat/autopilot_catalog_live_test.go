//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type catalogLiveProvider struct {
	llm.Provider
	raws []string
}

func (p *catalogLiveProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	raw, err := p.Provider.StreamChat(ctx, request, delta)
	p.raws = append(p.raws, raw)
	return raw, err
}

func TestAutopilotContinuousCatalogLiveContract(t *testing.T) {
	models := strings.Split(os.Getenv("MAGICHANDY_LAB_MODELS"), "|")
	if models[0] == "" {
		t.Skip("set MAGICHANDY_LAB_MODELS")
	}
	rows := []map[string]any{}
	for _, model := range models {
		native, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []AutopilotKind{AutopilotKindMotion, AutopilotKindSpeech} {
			provider := &catalogLiveProvider{Provider: native}
			service := AutopilotService{Provider: provider, Model: model, Patterns: continuousTestChoices(), Capabilities: FullCapabilities(), MaxTokens: 768, ReasoningMode: "off", Temperature: .1,
				MotionContext: &MotionContext{Running: true, PatternID: string(motion.PatternFullSweeps), SpeedPercent: 25, SpeedMinPercent: 10, SpeedMaxPercent: 43, Area: AreaZoneFull}}
			result, err := service.Complete(t.Context(), kind, Request{Message: "The user requests smooth variable reach returning to the base each stroke. Keep speed at 25. Choose that movement inside the saved limits."})
			ok := err == nil && len(provider.raws) == 1 && result.Motion != nil && result.Motion.PatternID == string(motion.PatternBaseVariation)
			rows = append(rows, map[string]any{"model": model, "kind": kind, "pass": ok, "result": result, "raw": provider.raws, "error": err})
			if !ok {
				t.Errorf("%s %s: calls=%d response=%+v error=%v", model, kind, len(provider.raws), result, err)
			}
		}
	}
	if path := os.Getenv("MAGICHANDY_LAB_REPORT"); path != "" {
		data, _ := json.MarshalIndent(rows, "", "  ")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
