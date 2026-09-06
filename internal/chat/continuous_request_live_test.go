//go:build liveeval

package chat

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// TestContinuousRequestLive uses the production service but never dispatches a
// command. Each case starts from an independent known score, so one rejection
// cannot contaminate later results. Reports retain rejected model proposals.
func TestContinuousRequestLive(t *testing.T) {
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
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 80
	rows := []map[string]any{}
	writeReport := func() {
		if path := os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"); path != "" {
			data, marshalErr := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if writeErr := os.WriteFile(path, data, 0600); writeErr != nil { // #nosec G703 -- explicit local evaluation output
				t.Fatal(writeErr)
			}
		}
	}
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	repeats = max(1, repeats)
	validCount, intentCount, total := 0, 0, 0
	reasoning := os.Getenv("MAGICHANDY_EVAL_REASONING")
	if reasoning == "" {
		reasoning = "off"
	}
	budget := 0
	if reasoning == "on" {
		budget = 384
	} else if reasoning != "off" {
		t.Fatal("reasoning must be on or off")
	}
	harness := os.Getenv("MAGICHANDY_EVAL_HARNESS")
	if harness == "" {
		harness = "production"
	}
	if harness != "production" && harness != "compact" && harness != "unguided" {
		t.Fatal("unknown evaluation harness")
	}
	for _, mode := range []MotionMode{MotionModeCreativeV2, MotionModeLayered} {
		cases := continuousRequestCases(mode)
		if os.Getenv("MAGICHANDY_EVAL_SUITE") == "holdout" {
			cases = continuousRequestHoldouts(mode)
		}
		for _, tc := range cases {
			if filter := os.Getenv("MAGICHANDY_EVAL_CASES"); filter != "" && !strings.Contains(","+filter+",", ","+tc.name+",") {
				continue
			}
			for repeat := range repeats {
				before := DefaultLayeredScore(40)
				parser := ParseLayeredReply
				if mode == MotionModeCreativeV2 {
					before, parser = FreshCreativeV2Score(40), ParseCreativeV2Reply
					before.Gesture.ReboundCount = 2
				}
				before.Seed = 123456 // Replayable evaluation fixture, never a production seed.
				state := MotionContext{Running: !tc.stopped, Paused: tc.paused, Layered: &before, MotionMode: mode, SpeedPercent: 40, SpeedMinPercent: 10, SpeedMaxPercent: 80}
				evalProvider := &continuousEvalProvider{Provider: provider, harness: harness}
				service := Service{Provider: evalProvider, Model: model, ReasoningMode: reasoning, ReasoningBudgetTokens: budget, MaxTokens: 1200,
					Capabilities: &Capabilities{Motion: true, MotionMode: mode, Voice: VoiceUtility}, MotionContext: &state}
				started := time.Now()
				result, completeErr := service.Complete(t.Context(), Request{Message: tc.message}, nil)
				_, proposed, _, parseErr := parser(result.Raw, before, limits)
				after := before
				if result.Response.Motion != nil {
					after = *result.Response.Motion.Layered
				}
				valid := completeErr == nil && !result.InitialMalformed && !result.Repaired && !result.SemanticFallback
				intent := valid && (result.Response.Motion != nil) == tc.moves && tc.check(before, after)
				row := map[string]any{"method": string(mode), "harness": harness, "reasoning": reasoning, "model": model, "recipe_name": tc.name, "repeat": repeat,
					"message": tc.message, "expected_recipe": tc.name, "valid": valid, "intent_pass": intent,
					"raw": result.Raw, "reply": result.Response.Reply, "before": before, "after": after, "limits": limits,
					"running": state.Running, "paused": state.Paused, "elapsed_ms": time.Since(started).Milliseconds()}
				if completeErr != nil {
					row["error"] = completeErr.Error()
				}
				if parseErr == nil {
					row["proposal"] = proposed
					row["proposal_intent_pass"] = tc.check(before, proposed)
				}
				rows = append(rows, row)
				total++
				if valid {
					validCount++
				}
				if intent {
					intentCount++
				}
				t.Logf("%s/%s/%d valid=%t intent=%t error=%v", mode, tc.name, repeat, valid, intent, completeErr)
				writeReport()
			}
		}
	}
	t.Logf("%s: valid=%d/%d intent=%d/%d", model, validCount, total, intentCount, total)
}

// Experimental variations are evaluation-only, never hidden production flags.
type continuousEvalProvider struct {
	llm.Provider
	harness string
}

func (p *continuousEvalProvider) StreamChat(ctx context.Context, request llm.ChatRequest, delta func(string) error) (string, error) {
	switch p.harness {
	case "compact":
		request.Messages[0].Content = strings.ReplaceAll(request.Messages[0].Content, continuousActionGuide, strings.Split(continuousActionGuide, "\nInterpret")[0])
	case "unguided":
		request.JSONSchema = nil
	}
	return p.Provider.StreamChat(ctx, request, delta)
}

type continuousRequestCase struct {
	name, message          string
	stopped, paused, moves bool
	check                  func(motion.FlowSpec, motion.FlowSpec) bool
}

func continuousRequestCases(mode MotionMode) []continuousRequestCase {
	same := func(a, b motion.FlowSpec) bool { return reflect.DeepEqual(a, b) }
	pace := func(delta int) func(motion.FlowSpec, motion.FlowSpec) bool {
		return func(a, b motion.FlowSpec) bool { a.SpeedPercent += delta; return same(a, b) }
	}
	width := func(a, b motion.FlowSpec) bool {
		if mode == MotionModeCreativeV2 {
			a = *motion.CloneFlowSpec(&a)
			a.Gesture.FocusWidthPercent = 30
		} else {
			a.RangeFloorPercent, a.RangeCeilingPercent = 30, 30
		}
		return same(a, b)
	}
	cases := []continuousRequestCase{
		{name: "pace-relative", message: "Increase overall speed by exactly five percentage points. Preserve everything else.", moves: true, check: pace(5)},
		{name: "pace-paraphrase", message: "Cut the overall rate by a quarter. Leave the rest alone.", moves: true, check: pace(-10)},
		{name: "pace-scoped-negative", message: "Make the pace five percentage points slower without changing the reach or location.", moves: true, check: pace(-5)},
		{name: "pace-spanish", message: "Baja la velocidad cinco puntos porcentuales y conserva todo lo demás.", moves: true, check: pace(-5)},
		{name: "question", message: "How does faster motion change the feel of this pattern?", check: same},
		{name: "permission-question", message: "Would it be a good idea to increase the speed?", check: same},
		{name: "preserve-compound", message: "Do not change the pace or range. Keep everything as it is.", check: same},
		{name: "feedback", message: "That drift feels good. I like the current pace.", check: same},
		{name: "hold", message: "Keep this exact motion repeating with no changes.", check: same},
		{name: "evolve", message: "Keep varying within this same character.", moves: true, check: func(a, b motion.FlowSpec) bool {
			changed := a.Seed != b.Seed
			a.Seed = b.Seed
			return changed && same(a, b)
		}},
		{name: "paused", message: "Make the pace faster.", paused: true, check: same},
		{name: "stopped-question", message: "What would starting at a gentle pace do?", stopped: true, check: same},
		{name: "stopped-refusal", message: "Do not start moving; just explain the controls.", stopped: true, check: same},
		{name: "start", message: "Start moving at the current settings.", stopped: true, moves: true, check: same},
	}
	widthRequest := "Set the shortest and widest stroke to exactly 30 percentage points."
	if mode == MotionModeCreativeV2 {
		widthRequest = "Set local stroke width to exactly 30 percentage points."
	}
	cases = append(cases,
		continuousRequestCase{name: "width", message: widthRequest + " Preserve the other settings.", moves: true, check: width},
		continuousRequestCase{name: "width-scoped-negative", message: widthRequest + " Do not change the speed or location.", moves: true, check: width},
		continuousRequestCase{name: "width-explanation", message: widthRequest + " Then explain why that changes the feel. Keep everything else.", moves: true, check: width},
	)
	if mode == MotionModeCreativeV2 {
		cases = append(cases, continuousRequestCase{name: "remove-rebounds", message: "No more rebounds, please. Preserve all other controls.", moves: true, check: func(a, b motion.FlowSpec) bool {
			a = *motion.CloneFlowSpec(&a)
			a.Gesture.ReboundCount = 0
			return same(a, b)
		}},
			continuousRequestCase{name: "directional-timing", message: "Make the sweep toward the upper end quicker than the return, with 60 percent timing contrast. Preserve overall pace and other controls.", moves: true, check: func(a, b motion.FlowSpec) bool {
				a = *motion.CloneFlowSpec(&a)
				a.Gesture.FasterDirection = "tip"
				a.Gesture.ContrastPercent = 60
				return same(a, b)
			}})
	} else {
		cases = append(cases, continuousRequestCase{name: "layer-timing", message: "Make only the pace layer develop more gradually by increasing its period by four cycles.", moves: true, check: func(a, b motion.FlowSpec) bool {
			a = *motion.CloneFlowSpec(&a)
			a.Layers[1].PeriodCycles += 4
			return same(a, b)
		}})
	}
	return cases
}

func continuousRequestHoldouts(mode MotionMode) []continuousRequestCase {
	same := func(a, b motion.FlowSpec) bool { return reflect.DeepEqual(a, b) }
	paceOnly := func(a, b motion.FlowSpec) bool {
		lower := b.SpeedPercent < a.SpeedPercent
		a.SpeedPercent = b.SpeedPercent
		return lower && same(a, b)
	}
	cases := []continuousRequestCase{
		{name: "quoted-start", message: `Explain what the instruction "start moving now" would do.`, stopped: true, check: same},
		{name: "conditional-start", message: "If I decided to start later, would the current settings be used?", stopped: true, check: same},
		{name: "stopped-adjustment", message: "Reduce the pace by five percentage points.", stopped: true, check: same},
		{name: "quoted-update", message: `Someone suggested "increase the pace by ten". Explain that suggestion while keeping this motion.`, check: same},
		{name: "compare-only", message: "Compare the current motion with a much faster, narrower one. This is just a discussion.", check: same},
		{name: "start-paraphrase", message: "I'd like the device to begin moving now, using these same settings.", stopped: true, moves: true, check: same},
		{name: "pace-correction", message: "No, ease off a little; keep the reach and all variation settings as they are.", moves: true, check: paceOnly},
		{name: "pace-where", message: "Leave the location where it is and slow the overall pace by five percentage points.", moves: true, check: func(a, b motion.FlowSpec) bool { a.SpeedPercent -= 5; return same(a, b) }},
		{name: "pace-why", message: "Explain why the change will feel different, then reduce the overall speed from 40 to 35. Leave other controls alone.", moves: true, check: func(a, b motion.FlowSpec) bool { a.SpeedPercent = 35; return same(a, b) }},
		{name: "stop-before-move", message: "We can try faster motion later. For now leave everything exactly as it is.", check: same},
		{name: "paused-compound", message: "Resume and make the speed 45 percent.", paused: true, check: same},
	}
	if mode == MotionModeCreativeV2 {
		cases = append(cases,
			continuousRequestCase{name: "mixed-base", message: "Mix full strokes with shrinking rebounds at the base. Keep overall speed unchanged.", moves: true, check: func(a, b motion.FlowSpec) bool {
				return b.Gesture.FocusPercent == 0 && b.Gesture.FocusRoamPercent == 0 && b.Gesture.FocusMixPercent > 0 && b.Gesture.FocusMixPercent < 100 && b.Gesture.ReboundCount > 0 && a.SpeedPercent == b.SpeedPercent
			}},
			continuousRequestCase{name: "tip-rebounds", message: "Add rebounds at the tip, preserving the pace and outer band.", moves: true, check: func(a, b motion.FlowSpec) bool {
				return b.Gesture.FocusPercent == 100 && b.Gesture.FocusRoamPercent == 0 && b.Gesture.ReboundCount > 0 && a.SpeedPercent == b.SpeedPercent && a.MinPercent == b.MinPercent && a.MaxPercent == b.MaxPercent
			}},
			continuousRequestCase{name: "mixed-question", message: "What would mixing full strokes with base rebounds change? Keep the current controls.", check: same},
			continuousRequestCase{name: "already-roaming", message: "Let the working location roam freely without changing pace or width.", check: same},
		)
	} else {
		cases = append(cases,
			continuousRequestCase{name: "layer-correction", message: "No, make only the pace layer unfold four cycles more gradually. Preserve its amount, overall speed and the other layers.", moves: true, check: func(a, b motion.FlowSpec) bool {
				a = *motion.CloneFlowSpec(&a)
				a.Layers[1].PeriodCycles += 4
				return same(a, b)
			}},
			continuousRequestCase{name: "layer-remove", message: "Remove just the center layer. Don't change pace or reach.", moves: true, check: func(a, b motion.FlowSpec) bool {
				a = *motion.CloneFlowSpec(&a)
				a.Layers = a.Layers[1:]
				return same(a, b)
			}},
			continuousRequestCase{name: "geometry-compound", message: "Alternate full strokes with short strokes at the base, then explain the difference. Preserve pace.", moves: true, check: func(a, b motion.FlowSpec) bool {
				return b.AnchorPercent == 0 && b.SpeedPercent == a.SpeedPercent && len(b.Layers) == 2 && b.Layers[1].Axis == "range"
			}},
		)
	}
	return cases
}
