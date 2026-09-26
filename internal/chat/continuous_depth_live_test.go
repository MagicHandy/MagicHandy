//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// TestContinuousDepthLanguageLive checks that a partner's words about depth
// reach the matching end of the slider. Each accepted score is compiled and
// sampled, so a reply that claims depth cannot pass while the strokes stay
// near the tip. The fixture mirrors an interactive session: Explicit voice,
// running Autopilot and narrow saved speed limits. Chat turns and Autopilot
// planning turns share the same contracts and are both measured.
func TestContinuousDepthLanguageLive(t *testing.T) {
	eval := newDepthEval(t)
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	rows := []map[string]any{}
	passes := 0
	for _, mode := range []MotionMode{MotionModeCreativeV2, MotionModeLayered} {
		for _, tc := range depthCases() {
			if filter := os.Getenv("MAGICHANDY_EVAL_CASES"); filter != "" && !strings.Contains(","+filter+",", ","+tc.name+",") {
				continue
			}
			for repeat := range max(1, repeats) {
				row, pass := eval.turn(t.Context(), mode, tc)
				row["repeat"] = repeat
				rows = append(rows, row)
				if pass {
					passes++
				}
				b, a := row["reach_before"].(depthMetrics), row["reach_after"].(depthMetrics)
				t.Logf("%s/%s/%d pass=%t deepest %.0f->%.0f turn-median %.0f->%.0f deep-share %.2f->%.2f err=%v",
					mode, tc.name, repeat, pass, b.Deepest, a.Deepest, b.LowerTurnMedian, a.LowerTurnMedian, b.DeepShare, a.DeepShare, row["error"])
				writeDepthReport(t, rows)
			}
		}
	}
	t.Logf("%s: depth intent %d/%d", eval.model, passes, len(rows))
}

type depthEval struct {
	provider llm.Provider
	model    string
	prompt   PromptSet
	voice    VoiceLevel
	limits   config.MotionSettings
}

func newDepthEval(t *testing.T) depthEval {
	endpoint, model := os.Getenv("MAGICHANDY_EVAL_URL"), os.Getenv("MAGICHANDY_EVAL_MODEL")
	if endpoint == "" || model == "" {
		t.Skip("set MAGICHANDY_EVAL_URL and MAGICHANDY_EVAL_MODEL for a local llama.cpp worker")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		t.Fatal("live evaluation requires a loopback HTTP worker")
	}
	provider, err := llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{BaseURL: endpoint, Model: model, Timeout: 90 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	voice := VoiceLevel(os.Getenv("MAGICHANDY_EVAL_VOICE"))
	if voice == "" {
		voice = VoiceExplicit
	}
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 6, 31
	prompt, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	return depthEval{provider: provider, model: model, prompt: prompt, voice: voice, limits: limits}
}

// turn runs one case from its own starting score, so a rejection cannot
// contaminate later cases, and measures the accepted result.
func (e depthEval) turn(ctx context.Context, mode MotionMode, tc depthCase) (map[string]any, bool) {
	before := depthStart(mode, tc.start)
	state := MotionContext{Running: true, Autopilot: true, Layered: &before, MotionMode: mode,
		SpeedPercent: before.SpeedPercent, SpeedMinPercent: e.limits.SpeedMinPercent, SpeedMaxPercent: e.limits.SpeedMaxPercent}
	capabilities := Capabilities{Motion: true, MotionMode: mode, Voice: e.voice, MoodTracking: e.voice != VoiceUtility}
	profile := ConversationContext{UserAnatomy: config.LLMUserAnatomyCustom}
	started := time.Now()
	complete := e.chat
	if tc.autopilot {
		complete = e.plan
	}
	reply, raw, after, err := complete(ctx, tc.message, &state, capabilities, &profile)
	final := before
	if after != nil {
		final = *after
	}
	b, a := depthReach(before, e.limits), depthReach(final, e.limits)
	pass := err == nil && after != nil && tc.check(b, a)
	// Field names match the motion atlas's LLM report input.
	row := map[string]any{"model": e.model, "method": string(mode), "recipe_name": tc.name, "expected_recipe": tc.name,
		"message": tc.message, "autopilot": tc.autopilot, "voice": string(e.voice),
		"valid": err == nil, "intent_pass": pass, "reply": reply, "raw": raw, "limits": e.limits,
		"before": before, "after": final, "reach_before": b, "reach_after": a,
		"elapsed_ms": time.Since(started).Milliseconds()}
	if err != nil {
		row["error"] = err.Error()
	}
	return row, pass
}

func (e depthEval) chat(ctx context.Context, message string, state *MotionContext, capabilities Capabilities, profile *ConversationContext) (string, string, *motion.FlowSpec, error) {
	service := Service{Provider: e.provider, Prompt: e.prompt, Model: e.model, ReasoningMode: "off", MaxTokens: 512,
		Capabilities: &capabilities, MotionContext: state, ConversationContext: profile}
	result, err := service.Complete(ctx, Request{Message: message}, nil)
	var after *motion.FlowSpec
	if result.Response.Motion != nil {
		after = result.Response.Motion.Layered
	}
	return result.Response.Reply, result.Raw, after, err
}

// plan runs the Autopilot turn that follows a request chat has already
// answered in words, as in the reported session.
func (e depthEval) plan(ctx context.Context, message string, state *MotionContext, capabilities Capabilities, profile *ConversationContext) (string, string, *motion.FlowSpec, error) {
	spoke := 25
	state.UserRequests, state.UserRequestSecondsAgo = []string{message}, []int{spoke}
	capture := &rawCaptureProvider{Provider: e.provider}
	service := AutopilotService{Provider: capture, Prompt: e.prompt, Model: e.model, MaxTokens: 512, ReasoningMode: "off",
		MotionContext: state, ConversationContext: profile, Capabilities: capabilities}
	history := []llm.Message{{Role: "user", Content: message}, {Role: "assistant", Content: "Taking you deeper now."}}
	response, err := service.Complete(ctx, AutopilotKindMotion, Request{
		Message: depthAutopilotMessage(state.MotionMode, *state.Layered, e.limits, spoke), History: history})
	var after *motion.FlowSpec
	if response.Motion != nil {
		after = response.Motion.Layered
	}
	return response.Reply, capture.raw, after, err
}

// depthAutopilotMessage supplies the compiled facts production derives from
// the running plan.
func depthAutopilotMessage(mode MotionMode, current motion.FlowSpec, limits config.MotionSettings, spoke int) string {
	feel := motion.NewMotionPlan("depth-eval-current", motion.MotionTarget{Label: "depth eval", Source: "autopilot",
		SpeedPercent: current.SpeedPercent, Flow: motion.CloneFlowSpec(&current)}, limits, 0, 0, time.Unix(0, 0)).Perceptual
	low, high := int(math.Round(feel.PositionMinPercent)), int(math.Round(feel.PositionMaxPercent))
	return AutopilotDecisionMessage(AutopilotContext{MotionMode: mode, CurrentFlow: &current,
		CurrentSpeed: current.SpeedPercent, SpeedMinPercent: limits.SpeedMinPercent, SpeedMaxPercent: limits.SpeedMaxPercent,
		Style: "balanced", LastHumanSecondsAgo: &spoke, MotionMinSeconds: 8, MotionMaxSeconds: 16, MotionChangeLevel: 4,
		SessionTracking: true, SessionSeconds: 180, SecondsAtCurrentSpeed: spoke, SecondsAtCurrentPhrase: spoke, DecisionsAtCurrentPhrase: 1,
		CommandedPositionMin: low, CommandedPositionMax: high,
		CommandedMeanTravel: int(math.Round(feel.CommandedMeanTravelPerSecond)),
		CommandedPeakSpeed:  int(math.Round(feel.CommandedPeakVelocityPerSecond)),
		MeanStrokeLength:    int(math.Round(feel.MeanStrokePercent)),
		LocalStrokeCV:       int(math.Round(feel.MinimumLocalStrokeCV * 100)),
		LocalStrokeRange:    int(math.Round(feel.MinimumLocalStrokeRange)),
		RecentPositionBands: []PositionBand{{Minimum: low, Maximum: high}}})
}

func writeDepthReport(t *testing.T, rows []map[string]any) {
	path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE")
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil { // #nosec G703 -- explicit local evaluation output
		t.Fatal(err)
	}
}

// rawCaptureProvider keeps the last raw completion, which Autopilot results
// do not carry, so a report can show what the planner actually emitted.
type rawCaptureProvider struct {
	llm.Provider
	raw string
}

func (p *rawCaptureProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	raw, err := p.Provider.StreamChat(ctx, request, delta)
	p.raw = raw
	return raw, err
}

type depthCase struct {
	name, message, start string
	autopilot            bool
	check                func(before, after depthMetrics) bool
}

func depthCases() []depthCase {
	// Deep means most strokes turn near the base; shallow means none pass
	// halfway. Relative cases compare where strokes turn before and after.
	deep := func(_, a depthMetrics) bool { return a.Deepest <= 12 && a.DeepShare >= 0.5 }
	deeper := func(b, a depthMetrics) bool { return a.Deepest <= 12 && a.LowerTurnMedian <= b.LowerTurnMedian-10 }
	shallow := func(_, a depthMetrics) bool { return a.Deepest >= 50 }
	lessDeep := func(b, a depthMetrics) bool { return a.LowerTurnMedian >= b.LowerTurnMedian+10 }
	whole := func(_, a depthMetrics) bool { return a.Deepest <= 10 && a.Shallowest >= 90 && a.FullShare >= 0.5 }
	return []depthCase{
		{name: "deepthroat", message: "Deepthroat it", start: "upper", check: deep},
		{name: "not-deep-enough", message: "you're not going deep enough", start: "upper", check: deeper},
		{name: "all-the-way-down", message: "Take it all the way down every time", start: "upper", check: deep},
		{name: "bottom-out", message: "Bottom out on every stroke", start: "middle", check: deep},
		{name: "deeper-slower", message: "Slower and deeper", start: "upper", check: func(b, a depthMetrics) bool {
			return deeper(b, a) && a.Speed < b.Speed
		}},
		{name: "just-the-tip", message: "Just the tip for a while", start: "full", check: shallow},
		{name: "head-only", message: "Only work the head", start: "full", check: shallow},
		{name: "not-so-deep", message: "Not so deep", start: "deep", check: lessDeep},
		{name: "whole-length", message: "Use the whole length, tip to base", start: "upper", check: whole},
		// Planning after chat answered: recover depth the score still lacks,
		// and keep depth once the score has it.
		{name: "autopilot-recover-depth", message: "Deepthroat it", start: "upper", autopilot: true, check: deeper},
		{name: "autopilot-keep-depth", message: "Deepthroat it", start: "based", autopilot: true, check: deep},
	}
}

// depthStart returns a replayable running score confined to one region.
func depthStart(mode MotionMode, region string) motion.FlowSpec {
	spec := DefaultLayeredScore(20)
	if mode == MotionModeCreativeV2 {
		spec = FreshCreativeV2Score(20)
	}
	switch region {
	case "upper":
		spec.MinPercent, spec.MaxPercent = 45, 100
	case "middle":
		spec.MinPercent, spec.MaxPercent = 25, 90
	case "deep":
		spec.MinPercent, spec.MaxPercent = 0, 55
	case "based":
		// Every stroke reaches the base, as a chat turn that honored depth leaves it.
		spec.MinPercent, spec.MaxPercent = 0, 100
		if spec.Gesture != nil {
			spec.Gesture.FocusPercent, spec.Gesture.FocusWidthPercent, spec.Gesture.FocusMixPercent, spec.Gesture.FocusRoamPercent = 0, 40, 60, 0
		} else {
			spec.AnchorPercent, spec.Layers = 0, spec.Layers[1:]
		}
	}
	spec.Seed = 123456 // Replayable evaluation fixture, never a production seed.
	return spec
}

type depthMetrics struct {
	Speed           int     `json:"speed"`
	Deepest         float64 `json:"deepest"`
	Shallowest      float64 `json:"shallowest"`
	LowerTurnMedian float64 `json:"lower_turn_median"`
	DeepShare       float64 `json:"deep_share"`
	FullShare       float64 `json:"full_share"`
	Strokes         int     `json:"strokes"`
}

// depthReach compiles a score through the shared engine and measures where
// its strokes turn. DeepShare counts turns within 20 points of the base;
// FullShare counts strokes spanning at least 70 points.
func depthReach(spec motion.FlowSpec, limits config.MotionSettings) depthMetrics {
	plan := motion.NewMotionPlan("depth-eval", motion.MotionTarget{Label: "depth eval", Source: "chat",
		SpeedPercent: spec.SpeedPercent, Flow: motion.CloneFlowSpec(&spec)}, limits, 0, 0, time.Unix(0, 0))
	var positions []float64
	for ms := int64(0); ms <= 90000; ms += 20 {
		positions = append(positions, plan.SampleAt(ms).PositionPercent)
	}
	m := depthMetrics{Speed: spec.SpeedPercent, Deepest: slices.Min(positions), Shallowest: slices.Max(positions)}
	lows, turns := strokeTurns(positions)
	if len(lows) == 0 {
		return m
	}
	slices.Sort(lows)
	m.LowerTurnMedian = lows[len(lows)/2]
	deep := 0
	for _, low := range lows {
		if low <= 20 {
			deep++
		}
	}
	m.DeepShare = float64(deep) / float64(len(lows))
	full := 0
	for i := 1; i < len(turns); i++ {
		if d := turns[i] - turns[i-1]; d >= 70 || d <= -70 {
			full++
		}
	}
	m.Strokes = len(turns) - 1
	if m.Strokes > 0 {
		m.FullShare = float64(full) / float64(m.Strokes)
	}
	return m
}

// strokeTurns finds reversals with 2 points of hysteresis. lows are the
// deepest point of each stroke; turns are every reversal in order.
func strokeTurns(positions []float64) (lows, turns []float64) {
	direction, extreme := 0, positions[0]
	for _, p := range positions[1:] {
		switch {
		case direction >= 0 && p < extreme-2:
			if direction > 0 {
				turns = append(turns, extreme)
			}
			direction, extreme = -1, p
		case direction <= 0 && p > extreme+2:
			if direction < 0 {
				turns, lows = append(turns, extreme), append(lows, extreme)
			}
			direction, extreme = 1, p
		case (direction > 0 && p > extreme) || (direction < 0 && p < extreme):
			extreme = p
		}
	}
	return lows, turns
}
