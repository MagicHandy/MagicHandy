//go:build liveeval

package httpapi

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// The acceptance path includes generation, strict parsing and the compiled
// curve. A valid JSON response alone cannot prove the requested control survived.
func TestLiveMotionLabControls(t *testing.T) {
	base, model := liveAutopilotProvider(t)
	provider, err := newLiveAutopilotProvider(llm.HTTPProviderOptions{BaseURL: base, Model: model, Timeout: 90 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	cases := []struct {
		name, message, method string
		anchor, outbound      int
	}{
		{"base", "Vary the stroke lengths while always returning to the base end. Keep timing balanced.", "anchored", 0, 50},
		{"tip", "Keep the tip end fixed while varying the other end. Keep both directions equally timed.", "anchored", 100, 50},
		{"timing", "Compare a quicker outbound stroke toward the tip and a slower return. Keep range contraction centered.", "directional", 50, -1},
		{"combined", "Keep returning to the base, with a quicker outbound stroke and a slower return.", "combined", 0, -1},
		{"baseline", "Use the ordinary Creative baseline without experimental controls.", "creative", 50, 50},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			for run := 0; run < 3; run++ {
				request := labAPIRequest()
				request.VariationPercent = 0
				reference := motionLabPromptReference(request)
				ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
				started := time.Now()
				proposal, err := chat.ProposeMotionLab(ctx, provider, model, test.message, reference)
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				if proposal.Method != test.method || *proposal.RangeAnchorPercent != test.anchor ||
					test.outbound > 0 && *proposal.OutboundTimePercent != test.outbound ||
					test.outbound < 0 && *proposal.OutboundTimePercent >= 50 {
					t.Fatalf("incorrect control choice: %+v", proposal)
				}
				request.RangeAnchorPercent, request.OutboundTimePercent = *proposal.RangeAnchorPercent, *proposal.OutboundTimePercent
				preview, err := motion.PreviewMotionLab(request, settings)
				if err != nil {
					t.Fatal(err)
				}
				for _, candidate := range preview.Candidates {
					if candidate.Method != proposal.Method {
						continue
					}
					if test.outbound < 0 && math.Abs(candidate.OutboundTimePercent-float64(*proposal.OutboundTimePercent)) > 3 {
						t.Fatalf("directional intent lost after fitting: %.1f vs %d", candidate.OutboundTimePercent, *proposal.OutboundTimePercent)
					}
					t.Logf("run=%d model=%s method=%s anchor=%d authored_outbound=%d actual_outbound=%.1f effective_pace=%.1f elapsed=%s explanation=%s",
						run+1, model, proposal.Method, *proposal.RangeAnchorPercent, *proposal.OutboundTimePercent,
						candidate.OutboundTimePercent, candidate.Perceptual.Pace.EffectivePercent, time.Since(started).Round(time.Millisecond), proposal.Explanation)
				}
			}
		})
	}
}
