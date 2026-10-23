//go:build liveeval

package chat

import (
	"os"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// Synthetic mechanical-state evaluation only: no user conversation, transport
// or device access. Retain rejected output alongside accepted decisions.
func TestEdgeWordingLive(t *testing.T) {
	var provider llm.Provider
	var model string
	if os.Getenv("MAGICHANDY_EDGE_LOCAL") != "" {
		model = os.Getenv("MAGICHANDY_LIVE_MODEL")
		var err error
		provider, err = llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 45 * time.Second, ContextSize: 8192})
		if err != nil {
			t.Fatal(err)
		}
	} else {
		provider, model = hostedLayeredLiveProvider(t)
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, wording := range []struct{ name, purpose string }{
		{"edging", "The person pressed Edge to request edging: keep a gentle holding interval before Autopilot resumes."},
		{"counterpoint", edgePurpose},
		{"backup", "The person requested the slow, gentle backup holding pattern. Decide its duration and a measured return."},
	} {
		for _, state := range []EdgeContext{
			{PreviousSpeed: 15, HoldingSpeed: 6, SpeedMin: 5, SpeedMax: 15, RangeMin: 50, RangeMax: 70},
			{PreviousSpeed: 50, HoldingSpeed: 20, SpeedMin: 5, SpeedMax: 50, RangeMin: 25, RangeMax: 75},
			{PreviousSpeed: 90, HoldingSpeed: 20, SpeedMin: 10, SpeedMax: 90, RangeMin: 10, RangeMax: 50},
		} {
			started := time.Now()
			plan, raw, err := (EdgeService{Provider: provider, Model: model, ReasoningMode: "off"}).complete(t.Context(), state, wording.purpose)
			row := map[string]any{"wording": wording.name, "model": model, "context": state, "raw": raw, "elapsed_ms": time.Since(started).Milliseconds(), "plan": plan, "valid": err == nil}
			if err != nil {
				row["error"] = err.Error()
				t.Errorf("%s previous=%d: %v", wording.name, state.PreviousSpeed, err)
			}
			rows = append(rows, row)
			t.Logf("%s previous=%d valid=%t ms=%v", wording.name, state.PreviousSpeed, err == nil, row["elapsed_ms"])
		}
	}
}
