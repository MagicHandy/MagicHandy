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

const localDecisionTrialGuide = `Decide the requested action before writing the reply. A request to perform a rhythm is permission to do it now; do not ask for confirmation. Asking how motion feels, quoting a command, or discussing a future possibility is only conversation: emit none, not a demonstration. Never add start merely to illustrate an explanation. For example, "Give me a rolling rhythm" asks for motion now; "What would a rolling rhythm feel like?" asks only for an explanation. When paused, explain that the app must resume it.`

const localCoverageTrialGuide = `Full-device sweeps mixed with short strokes need BOTH controls:{min_percent:0,max_percent:100} and geometry full_and_base/full_and_tip. Those geometries alone keep the old outer band. Short strokes alternating at both ends use alternate_ends, not full_and_base/full_and_tip. For quicker upper strokes, add a pace layer with shape alternate, period_cycles 8, phase_percent 0; for quicker lower strokes use phase_percent 50. This matches alternate_ends' center layer. Choose the pace amount for the requested contrast; base speed is independent. Width changes alone never widen travel bounds. Custom widths use stroke_width; meaningful regional coverage need not use the minimum width. Encode every requested control and omit unrequested changes.`

const compactLocalLayeredTrial = `Control one persistent Layered score with {"action":"none|start|update","edits":{...},"reply":"..."}. The current state determines permitted actions. Questions use none and empty edits. For a request to perform motion, apply every requested edit now, then describe only what the edits actually do. Never ask for confirmation of an already requested action.
Make a partial patch: omit every unrequested field. Outer endpoints, stroke width, location, base speed and pace variation are independent. The current outer band is existing motion, not a safety limit. Full-device travel means 0..100, including BOTH endpoints even if the current band is narrower. Width alone cannot expand the band.
Geometry operations preserve pace:
- full_slider makes every stroke span 0..100, removes range/center layers.
- full_band fills the CURRENT band every stroke, removes range/center layers.
- full_and_base/full_and_tip mix full-band strokes with shorter strokes anchored at that end, using a range layer. For full-DEVICE sweeps also set controls:{min_percent:0,max_percent:100}.
- alternate_ends moves short strokes between the two band ends, using a center layer. Its center layer has amount100, shape alternate, period_cycles8, phase_percent0. It does NOT mix in full strokes.
- base_anchor/tip_anchor/centered preserve width variation and remove center movement. The anchor is relative within the band; 0 is its lower end, 100 its upper end.
- wander varies width and location irregularly. It does not fix stroke width.
For quicker upper short strokes and slower lower short strokes use alternate_ends plus a pace layer with matching shape alternate, period_cycles8 and phase_percent0. For quicker lower strokes use pace phase_percent50 instead. Choose the amount of contrast for the request. Base speed stays separate.
edits supports:
- controls: partial absolute min_percent/max_percent (outer band 0..100), speed_percent (saved speed limits), anchor_percent (0..100), memory_cycles (2..32), pace_variation_percent (0..40), variation_mode (drift/waves).
- change_by: signed changes to those numeric controls. Never set and adjust the same control.
- stroke_width:{min_percent,max_percent}: shortest/widest travel, each 10..outer band width. Equal values fix every stroke's length. After geometry, these override its default widths.
- layers:[{axis,partial fields}]: PATCH range/center/pace. amount_percent0..100; period_cycles2..32; phase_percent0..100; shape drift/alternate/wave. Omitted fields/layers persist; new defaults amount30,period12,phase0,drift. Use remove_layers:[axis] to remove a layer.
- period_change_cycles in a layer adds a signed number to its current period; never combine with period_cycles. Longer period slows development, not base speed.
- evolve:true changes seeded details inside the same character, not geometry. Exact repetition uses no edits.
Range layers vary width; center layers move its location; pace layers vary travel rate. Independent layers can have independent timing. Location/width requests do not authorize new pace variation. Fixed width with roaming needs equal stroke_width bounds, a drifting center layer and removal of an old range layer. A count cannot be encoded by memory_cycles; this local grammar has no exact counted phrases.
For a new character, change geometry, widths, anchor or layers; evolve alone only freshens details. Encode every part of a compound request. No timestamps, points, device commands or physical-smoothness claims.
Example: full-device sweeps mixed with short tip strokes
{"action":"update","edits":{"controls":{"min_percent":0,"max_percent":100},"geometry":"full_and_tip"},"reply":"Full-device sweeps alternate with shorter tip strokes."}
Example: short strokes at both ends, quicker at the lower end, preserve base speed
{"action":"update","edits":{"geometry":"alternate_ends","layers":[{"axis":"pace","amount_percent":65,"period_cycles":8,"phase_percent":50,"shape":"alternate"}]},"reply":"Short strokes alternate ends, quicker at the lower end."}`

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
	var provider llm.Provider
	var err error
	if baseURL := os.Getenv("MAGICHANDY_NATURAL_LLAMA_URL"); baseURL != "" {
		provider, err = llm.NewLlamaCPPProvider(llm.HTTPProviderOptions{BaseURL: baseURL, Model: model, Timeout: 90 * time.Second, Client: &http.Client{Transport: naturalOpenAISeedTransport{}}})
	} else {
		provider, err = llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second, Client: &http.Client{Transport: local16KTransport{}}})
	}
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
	caps := Capabilities{Motion: !tc.off, MotionMode: tc.mode, SimpleMotion: naturalSimple(), Patterns: tc.mode == MotionModePattern, MotionHolder: tc.holder}
	wrapped := &localAdaptationProvider{Provider: provider, variant: variant, state: state, caps: caps, patterns: hostedLifecyclePatterns(tc.mode)}
	svc := Service{Provider: wrapped, Model: model, Capabilities: &caps, MotionContext: &state, Patterns: wrapped.patterns, ReasoningMode: "off", MaxTokens: 256}
	request := Request{Message: tc.message}
	if tc.afterStop {
		request.History = []llm.Message{{Role: "user", Content: "Use a smooth rhythm."}, {Role: "assistant", Content: "Playing a smooth rhythm."}, {Role: "user", Content: "Stop."}, {Role: "assistant", Content: "The device is stopped."}}
	}
	started := time.Now()
	result, err := svc.Complete(t.Context(), request, nil)
	if (variant == "intent_only" || variant == "compact" || variant == "coverage" || variant == "intent" || variant == "combined" || variant == "state" || variant == "guide") && (tc.mode == MotionModeDynamic || tc.mode == MotionModePattern) && !tc.off && err == nil {
		// Evaluate model intent after the ordinary local path, without asking
		// another model or rewriting its command. No target is dispatched.
		interpreted := caps
		interpreted.AdvancedMotion = true
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
			caps.AdvancedMotion = true
			request.Messages[0].Content = hostedStoppedGuidance(hostedStartContract(request.Messages[0].Content, caps), caps)
			request.JSONSchema = PatternResponseSchema(p.patterns, caps, &p.state)
		}
		if (p.variant == "coverage" || p.variant == "intent" || p.variant == "guide" || p.variant == "combined") && p.caps.MotionMode == MotionModeLayered {
			original := strings.Split(strings.Split(layeredContract, "Geometry works inside")[1], "\n\n")[0]
			guide := localRegionTrialGuide
			if p.variant == "coverage" {
				guide = localCoverageTrialGuide
			}
			request.Messages[0].Content = strings.Replace(request.Messages[0].Content, "Geometry works inside"+original, guide, 1)
		}
	}
	if (p.variant == "intent_only" || p.variant == "compact" || p.variant == "coverage" || p.variant == "intent") && p.caps.Motion {
		if p.variant != "intent_only" {
			p.narrowTrial(&request)
		}
		request.Messages[0].Content += "\n" + localDecisionTrialGuide
	}
	if p.variant == "compact" && p.caps.Motion && p.caps.MotionMode == MotionModeLayered {
		text := request.Messages[0].Content
		start := strings.Index(text, "Control one persistent layered motion score.")
		endText := "Never claim sudden acceleration or physical improvement from a plotted estimate."
		end := strings.Index(text, endText)
		if start >= 0 && end >= start {
			trial := compactLocalLayeredTrial
			if !p.state.Running {
				trial = strings.Split(trial, "\nExample:")[0]
			}
			request.Messages[0].Content = text[:start] + trial + text[end+len(endText):]
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
		caps.AdvancedMotion = true
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
	if os.Getenv("MAGICHANDY_LOCAL_SUITE") == "intent_holdouts" {
		return localMotionIntentHoldouts()
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

func localMotionIntentHoldouts() []localMotionTrial {
	trials := []localMotionTrial{}
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2, MotionModeDynamic, MotionModePattern} {
		for _, tc := range []hostedLifecycleCase{
			{name: "polite_performance", message: "Could you give me a slow rolling rhythm now?", start: true},
			{name: "perform_demonstration", message: "Please demonstrate a rolling rhythm now.", start: true},
			{name: "translated_performance", message: "Usa un ritmo suave ahora.", start: true},
			{name: "resume_after_stop", message: "Try a gently changing rhythm now.", start: true, afterStop: true},
			{name: "explain_hypothetical", message: "What would happen if I asked for a slower rolling rhythm?"},
			{name: "remembered_command", message: `I remember you saying "start a rolling rhythm". Explain what you meant.`},
			{name: "possible_future", message: "I might want more variation later. For now, just tell me what my options are."},
			{name: "translated_question", message: "¿Cómo se sentiría un ritmo suave? Solo explícalo, sin mover el dispositivo."},
			{name: "paused_performance", message: "Give me a slow rolling rhythm now.", paused: true},
			{name: "disabled_performance", message: "Give me a slow rolling rhythm now.", off: true},
		} {
			tc.mode = mode
			trials = append(trials, localMotionTrial{hostedLifecycleCase: tc})
		}
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
