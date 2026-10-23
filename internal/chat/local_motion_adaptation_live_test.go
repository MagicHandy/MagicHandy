//go:build liveeval

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

const localRegionTrialGuide = `Geometry stays inside the outer band. For full-device reach widen controls.min_percent/max_percent to 0/100, or use full_slider when every stroke should cover it. Tip/base are band ends. Choose regional coverage through endpoints, anchor and stroke_width; narrow does not mean minimum possible. Uncounted regional alternation uses alternate_ends. Match pace and center shape, period_cycles and phase_percent to make upper strokes faster; offset pace phase by 50 for faster lower strokes. Geometry's new alternate layer defaults are period 8, phase 0. Encode every requested control; omit the rest.`

const localStartTrialGuide = `When stopped, execute a fresh motion request now with start and edits; do not ask again. Questions, quoted examples and explicit future/settings-only requests use none.`

const localFullMixOldExample = `User: switch to full and base strokes and slow to 20 percent
{"edits":{"controls":{"speed_percent":20},"geometry":"full_and_base"},"reply":"Full and base strokes at 20 percent."}`

const localFullMixTrialExample = `User: mix full-device sweeps and base strokes at 20 percent
{"edits":{"controls":{"min_percent":0,"max_percent":100,"speed_percent":20},"geometry":"full_and_base"},"reply":"Full-device and base strokes."}`

type localMotionTrial struct {
	hostedLifecycleCase
	running bool
	check   string
}

// This experiment changes only code-owned prompts/schema in its wrapper. The
// optional state interpretation is evaluated against the captured output with
// the existing hosted state validator; production local behavior stays intact
// until the comparative evidence supports an explicit implementation change.
func TestLocalMotionAdaptationsLive(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LOCAL_MODEL")
	if model == "" {
		t.Skip("set MAGICHANDY_LOCAL_MODEL for the local Ollama evaluation")
	}
	variant := os.Getenv("MAGICHANDY_LOCAL_VARIANT")
	if variant == "" {
		variant = "baseline"
	}
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second, Client: &http.Client{Transport: local16KTransport{}}})
	if err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range localMotionTrials() {
		if os.Getenv("MAGICHANDY_LOCAL_RUNNING_ONLY") == "1" && !tc.running {
			continue
		}
		if mode := os.Getenv("MAGICHANDY_LIFECYCLE_MODE"); mode != "" && mode != string(tc.mode) {
			continue
		}
		t.Run(string(tc.mode)+"/"+tc.name, func(t *testing.T) {
			row := evaluateLocalMotionTrial(t, provider, model, variant, tc)
			rows = append(rows, row)
			saveHostedLayeredRows(t, rows)
			t.Logf("pass=%v calls=%v elapsed_ms=%v error=%v", row["intent_pass"], row["provider_calls"], row["elapsed_ms"], row["error"])
		})
	}
}

func evaluateLocalMotionTrial(t *testing.T, provider llm.Provider, model, variant string, tc localMotionTrial) map[string]any {
	t.Helper()
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
	before := DefaultLayeredScore(40)
	if tc.mode == MotionModeCreativeV2 {
		before = FreshCreativeV2Score(40)
	}
	before.Seed = 913
	if tc.running {
		before.MinPercent, before.MaxPercent, before.RangeCeilingPercent = 20, 85, 65
	}
	state := MotionContext{MotionMode: tc.mode, Layered: &before, Running: tc.running, Paused: tc.paused, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80, Envelope: motion.CurrentPlanningEnvelope(limits)}
	caps := Capabilities{Motion: !tc.off, MotionMode: tc.mode, Patterns: tc.mode == MotionModePattern, MotionHolder: tc.holder}
	wrapped := &localAdaptationProvider{Provider: provider, variant: variant, state: state, caps: caps, patterns: hostedLifecyclePatterns(tc.mode)}
	svc := Service{Provider: wrapped, Model: model, Capabilities: &caps, MotionContext: &state, Patterns: wrapped.patterns, ReasoningMode: "off", MaxTokens: 256}
	request := Request{Message: tc.message}
	if tc.afterStop {
		request.History = []llm.Message{{Role: "user", Content: "Use a smooth rhythm."}, {Role: "assistant", Content: "Playing a smooth rhythm."}, {Role: "user", Content: "Stop."}, {Role: "assistant", Content: "The device is stopped."}}
	}
	started := time.Now()
	result, err := svc.Complete(t.Context(), request, nil)
	if (variant == "combined" || variant == "state" || variant == "guide") && (tc.mode == MotionModeDynamic || tc.mode == MotionModePattern) && !tc.off && err == nil {
		// Evaluate model intent after the ordinary local path, without asking
		// another model or rewriting its command. No target is dispatched.
		interpreted := caps
		interpreted.HostedMotion = true
		result.Response, err = svc.parseAndValidateResponse(result.Raw, interpreted, request.Message)
	}
	command := result.Response.Motion
	moves := command != nil && command.Action != MotionActionNone
	pass := err == nil && moves == (tc.start || tc.running) && strings.TrimSpace(result.Response.Reply) != ""
	if tc.start {
		pass = pass && command != nil && command.Action == MotionActionStart
	}
	if tc.running && pass {
		pass = command != nil && command.Layered != nil && localMotionIntent(tc.check, before, *command.Layered, limits)
	}
	row := hostedLayeredLiveRow(tc.name, model, tc.message, result.Raw, before, command, limits, started, err, pass)
	row["valid"], row["repaired"], row["malformed"] = err == nil && !result.Malformed, result.Repaired, result.Malformed
	if result.MalformedError != "" {
		row["error"] = result.MalformedError
	}
	row["method"], row["variant"], row["motion_command"], row["reply"], row["provider_calls"] = tc.mode, variant, command, result.Response.Reply, wrapped.calls
	row["schema_bytes"], row["input_bytes"], row["context_size"], row["max_tokens"], row["raw_attempts"] = wrapped.schemaBytes, wrapped.inputBytes, 16384, 256, wrapped.raws
	row["running"], row["paused"], row["motion_enabled"], row["motion_holder"], row["history"] = tc.running, tc.paused, !tc.off, tc.holder, request.History
	if command == nil || command.Layered == nil {
		delete(row, "after")
	}
	return row
}

type localAdaptationProvider struct {
	llm.Provider
	variant           string
	state             MotionContext
	caps              Capabilities
	patterns          []PatternChoice
	calls, inputBytes int
	schemaBytes       int
	raws              []string
}

func (p *localAdaptationProvider) StreamChat(ctx context.Context, request llm.ChatRequest, emit func(string) error) (string, error) {
	if p.variant != "baseline" && p.variant != "narrow" && p.caps.Motion {
		if p.caps.MotionMode == MotionModePattern || p.caps.MotionMode == MotionModeDynamic {
			caps := p.caps
			caps.HostedMotion = true
			request.Messages[0].Content = hostedStoppedGuidance(hostedStartContract(request.Messages[0].Content, caps), caps)
			request.JSONSchema = PatternResponseSchema(p.patterns, caps, &p.state)
		}
		if (p.variant == "guide" || p.variant == "combined") && p.caps.MotionMode == MotionModeLayered {
			original := strings.Split(strings.Split(layeredContract, "Geometry works inside")[1], "\n\n")[0]
			request.Messages[0].Content = strings.Replace(request.Messages[0].Content, "Geometry works inside"+original, localRegionTrialGuide, 1)
		}
	}
	if p.variant == "narrow" && p.caps.Motion {
		p.narrowTrial(&request)
	}
	if p.variant == "reach" && p.caps.MotionMode == MotionModeLayered && p.caps.Motion {
		text := strings.Replace(request.Messages[0].Content, "Geometry works inside the outer band.",
			"full_and_tip/full_and_base use current bounds; full-device sweeps also need controls:{min_percent:0,max_percent:100}.", 1)
		request.Messages[0].Content = strings.Replace(text, "Geometry's base/tip are the BAND's ends, not slider 0/100 unless the band reaches them. ", "", 1)
	}
	bounded, budget, err := llm.BudgetChatRequest(request, 16384)
	p.inputBytes, p.schemaBytes = budget.InputBytes, len(request.JSONSchema)
	if err != nil {
		return "", err
	}
	p.calls++
	raw, err := p.Provider.StreamChat(ctx, bounded, emit)
	p.raws = append(p.raws, raw)
	return raw, err
}

func (p *localAdaptationProvider) narrowTrial(request *llm.ChatRequest) {
	if p.caps.MotionMode == MotionModeDynamic && p.state.Paused {
		request.JSONSchema = chatOnlyResponseSchema(p.caps)
	}
	if p.caps.MotionMode == MotionModePattern {
		caps := p.caps
		caps.HostedMotion = true
		request.JSONSchema = PatternResponseSchema(p.patterns, caps, &p.state)
	}
	if (p.caps.MotionMode == MotionModeLayered || p.caps.MotionMode == MotionModeCreativeV2) && !p.state.Running && !p.state.Paused {
		request.Messages[0].Content = strings.Replace(request.Messages[0].Content,
			"Apply requested edits with start. A fresh motion request can start after an earlier Stop; questions and explicitly future/settings-only requests use none.", localStartTrialGuide, 1)
	}
	if p.caps.MotionMode == MotionModeLayered {
		original := strings.Replace(localFullMixOldExample, `{"edits":`, `{"action":"update","edits":`, 1)
		replacement := strings.Replace(localFullMixTrialExample, `{"edits":`, `{"action":"update","edits":`, 1)
		request.Messages[0].Content = strings.Replace(request.Messages[0].Content, original, replacement, 1)
	}
}

type local16KTransport struct{}

func (local16KTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	_ = request.Body.Close()
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	options := body["options"].(map[string]any)
	options["num_ctx"], options["seed"] = 16384, 10232026
	data, err = json.Marshal(body)
	if err != nil {
		return nil, err
	}
	cloned := request.Clone(request.Context())
	cloned.Body, cloned.ContentLength = io.NopCloser(bytes.NewReader(data)), int64(len(data))
	return http.DefaultTransport.RoundTrip(cloned)
}

func localMotionTrials() []localMotionTrial {
	if os.Getenv("MAGICHANDY_LOCAL_SUITE") == "negatives" {
		return localMotionNegativeHoldouts()
	}
	trials := []localMotionTrial{}
	for _, tc := range hostedLifecycleCases() {
		if !tc.counted {
			trials = append(trials, localMotionTrial{hostedLifecycleCase: tc})
		}
	}
	for _, mode := range []MotionMode{MotionModeDynamic, MotionModePattern} {
		for _, tc := range []hostedLifecycleCase{
			{name: "quoted_motion", message: `Explain the phrase "start a syncopated rhythm". Don't move.`},
			{name: "conditional_motion", message: "If we tried syncopation later, what would it do?"},
			{name: "after_stop_chat", message: "Thank you, that was helpful.", afterStop: true},
			{name: "script_owner", message: "Use syncopation.", off: true, holder: MotionHolderVideoScript},
		} {
			tc.mode = mode
			trials = append(trials, localMotionTrial{hostedLifecycleCase: tc})
		}
	}
	for _, tc := range []struct{ name, message string }{
		{"full_range", "Use full-device strokes all the way from 0 to 100. Keep speed the same."},
		{"full_base_mix", "Alternate full-device sweeps and short strokes at the base. Keep pace unchanged."},
		{"full_tip_mix", "Mix full-device sweeps with short strokes near the tip. Keep pace unchanged."},
		{"regional_upper", "Alternate quick strokes near the tip with slow strokes near the base. Keep the overall speed at 40 percent."},
		{"regional_lower", "Alternate quick strokes near the base with slow strokes near the tip. Keep the overall speed at 40 percent."},
		{"compound", "Increase overall speed by five percentage points and use full-device strokes, then explain what you changed."},
		{"fixed_width_roam", "Keep speed at 40. Set travel bounds from 0 to 100 and hold every stroke at 40 percentage points while moving its location irregularly."},
		{"layer_period", "Make only the pace layer develop more gradually by increasing its period by four cycles. Preserve overall speed and reach."},
	} {
		trials = append(trials, localMotionTrial{hostedLifecycleCase: hostedLifecycleCase{name: tc.name, message: tc.message, mode: MotionModeLayered}, running: true, check: tc.name})
	}
	return trials
}

func localMotionNegativeHoldouts() []localMotionTrial {
	trials := []localMotionTrial{}
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2, MotionModeDynamic, MotionModePattern} {
		for _, tc := range []hostedLifecycleCase{
			{name: "negated_imperative", message: "Don't use syncopation; explain it instead."},
			{name: "quoted_imperative", message: `Repeat the words "start moving now" as an example, and keep the device stopped.`},
			{name: "future_full_range", message: "If I later ask for full strokes, what will change? For now keep it stopped.", afterStop: true},
			{name: "future_configuration", message: "I want to configure rhythm for next time only; don't perform it now."},
			{name: "reported_imperative", message: `The previous person said "Use a rolling rhythm". Tell me what that meant.`},
			{name: "comparison_only", message: "Could you compare two patterns without playing either?"},
		} {
			tc.mode = mode
			trials = append(trials, localMotionTrial{hostedLifecycleCase: tc})
		}
	}
	return trials
}

func localMotionIntent(name string, before, after motion.FlowSpec, limits config.MotionSettings) bool {
	full := after.MinPercent == 0 && after.MaxPercent == 100
	switch name {
	case "full_range":
		return full && after.RangeFloorPercent == 100 && after.RangeCeilingPercent == 100 && after.SpeedPercent == before.SpeedPercent
	case "full_base_mix", "full_tip_mix":
		anchor := 0
		if name == "full_tip_mix" {
			anchor = 100
		}
		return full && after.AnchorPercent == anchor && after.RangeCeilingPercent == 100 && after.RangeFloorPercent < 100 && after.SpeedPercent == before.SpeedPercent
	case "compound":
		return full && after.RangeFloorPercent == 100 && after.SpeedPercent == before.SpeedPercent+5
	case "fixed_width_roam":
		return full && after.RangeFloorPercent == 40 && after.RangeCeilingPercent == 40 && after.SpeedPercent == 40
	case "layer_period":
		return pacePeriod(&after) == pacePeriod(&before)+4 && after.SpeedPercent == before.SpeedPercent && after.MinPercent == before.MinPercent && after.MaxPercent == before.MaxPercent
	case "regional_upper", "regional_lower":
		var center, pace motion.FlowLayer
		for _, layer := range after.Layers {
			if layer.Axis == "center" {
				center = layer
			}
			if layer.Axis == "pace" {
				pace = layer
			}
		}
		phase := 0
		if name == "regional_lower" {
			phase = 50
		}
		return center.AmountPercent > 0 && pace.AmountPercent > 0 && center.Shape != "drift" && center.Shape == pace.Shape && center.PeriodCycles == pace.PeriodCycles && (pace.PhasePercent-center.PhasePercent+100)%100 == phase && hasCompiledRegionalTurns(after, limits)
	}
	return false
}
