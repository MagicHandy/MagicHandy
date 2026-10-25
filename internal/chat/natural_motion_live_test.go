//go:build liveeval

package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// These opt-in conversations use the production prompt and parser. Assertions
// check explicit requests, not preferred speeds, ranges, wording or a mandatory
// pattern rotation. Captures retain every failed attempt for qualitative and
// shared-engine review; there is no transport or hardware in this fixture.
func naturalLiveProvider(t *testing.T) (*outputLiveProvider, string, bool) {
	t.Helper()
	if os.Getenv("MAGICHANDY_NATURAL_EVAL") != "1" {
		t.Skip("opt-in natural motion conversation evaluation")
	}
	model := os.Getenv("MAGICHANDY_LOCAL_MODEL")
	if model == "" {
		provider, name := hostedLayeredLiveProvider(t)
		return &outputLiveProvider{Provider: provider}, name, true
	}
	provider, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second, Client: &http.Client{Transport: local16KTransport{}}})
	if level := os.Getenv("MAGICHANDY_NATURAL_LEVEL"); level == config.ModelCapabilityIntermediate || level == config.ModelCapabilityAdvanced {
		contextSize := config.MinimumLocalContext(level)
		if explicit, parseErr := strconv.Atoi(os.Getenv("MAGICHANDY_NATURAL_CONTEXT")); parseErr == nil {
			contextSize = max(contextSize, explicit)
		}
		provider, err = llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: model, Timeout: 90 * time.Second, ContextSize: contextSize, Client: &http.Client{Transport: naturalSeedTransport{}}})
	}
	if err != nil {
		t.Fatal(err)
	}
	return &outputLiveProvider{Provider: provider}, model, os.Getenv("MAGICHANDY_NATURAL_LEVEL") == config.ModelCapabilityAdvanced
}

// Reproducible comparisons may pin sampling, without overriding the production
// context allocation or changing the provider's shipped defaults.
type naturalSeedTransport struct{}

func (naturalSeedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	seed, err := strconv.Atoi(os.Getenv("MAGICHANDY_NATURAL_SEED"))
	if err != nil {
		return http.DefaultTransport.RoundTrip(request)
	}
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	_ = request.Body.Close()
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	options, _ := body["options"].(map[string]any)
	if options == nil {
		options = map[string]any{}
	}
	options["seed"], body["options"] = seed, options
	data, err = json.Marshal(body)
	if err != nil {
		return nil, err
	}
	cloned := request.Clone(request.Context())
	cloned.Body, cloned.ContentLength = io.NopCloser(bytes.NewReader(data)), int64(len(data))
	return http.DefaultTransport.RoundTrip(cloned)
}

func naturalIntermediate() bool {
	return os.Getenv("MAGICHANDY_LOCAL_MODEL") != "" && os.Getenv("MAGICHANDY_NATURAL_LEVEL") == config.ModelCapabilityIntermediate
}

func naturalMaxTokens() int {
	if value, err := strconv.Atoi(os.Getenv("MAGICHANDY_NATURAL_OUTPUT")); err == nil && value > 0 {
		return value
	}
	return 1536
}

func naturalPromptBudget() PromptBudgetSettings {
	level := os.Getenv("MAGICHANDY_NATURAL_LEVEL")
	if os.Getenv("MAGICHANDY_LOCAL_MODEL") != "" && (level == config.ModelCapabilityIntermediate || level == config.ModelCapabilityAdvanced) {
		return PromptBudgetSettings{ContextSize: config.MinimumLocalContext(level), MaxOutputTokens: naturalMaxTokens()}
	}
	return PromptBudgetSettings{}
}

func naturalState(score *motion.FlowSpec, mode MotionMode, limits config.MotionSettings) MotionContext {
	plan := motion.NewMotionPlan("review", motion.MotionTarget{Flow: score, SpeedPercent: score.SpeedPercent}, limits, 0, 0, time.Unix(0, 0))
	return MotionContext{MotionMode: mode, Layered: score, Running: true, SpeedPercent: score.SpeedPercent,
		SpeedMinPercent: limits.SpeedMinPercent, SpeedMaxPercent: limits.SpeedMaxPercent,
		Envelope: motion.CurrentPlanningEnvelope(limits), CommandedSummary: &plan.Perceptual}
}

func TestNaturalMotionConversationLive(t *testing.T) {
	provider, model, hosted := naturalLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 100
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		t.Run(string(mode), func(t *testing.T) {
			score := DefaultLayeredScore(43)
			if mode == MotionModeCreativeV2 {
				score = FreshCreativeV2Score(43)
			}
			score.Seed = 20261025
			history := []llm.Message{}
			requests := []string{}
			for i, message := range []string{
				"For this stretch, keep every stroke in the upper band from 65 to 100. Let the stroke lengths vary naturally there. Keep the current pace.",
				"Release that upper-only restriction now. Mix full-device sweeps with shorter strokes at the tip, flowing between them, without changing the pace.",
				"Let those shorter strokes cover more of the upper region. Preserve the full sweeps, the anchored tip and the pace.",
				"Forget the tip focus now. Let both location and stroke length wander freely, without a counted routine. Keep the current pace.",
				"Ease the overall pace for a gentle stretch. Preserve exactly this reach and pattern character.",
				"That gentle stretch is over. Bring the pace up to something brisk again, keeping the rest exactly as it is.",
				"Now make every stroke travel the entire slider, all the way to the base and back to the tip. Keep the pace.",
				"Keep this exact motion repeating. No changes to anything.",
				"Keep all controls and the same character, but refresh its seeded details once.",
				"What is the difference between changing travel range and changing speed? Explain only; keep the current motion.",
				"Increase just the speed by seven percentage points, staying inside my saved limits. Nothing else.",
			} {
				t.Run(fmt.Sprintf("turn_%02d", i+1), func(t *testing.T) {
					before := *motion.CloneFlowSpec(&score)
					state := naturalState(&score, mode, limits)
					// Production reads committed prior requests before logging the
					// current message. Keep the same bounded, aged context here.
					state.UserRequests = SelectRecentUserRequests(requests)
					for n := range state.UserRequests {
						state.UserRequestSecondsAgo = append(state.UserRequestSecondsAgo, (len(state.UserRequests)-n)*20)
					}
					caps := Capabilities{Motion: true, MotionMode: mode, AdvancedMotion: hosted, IntermediateMotion: naturalIntermediate(), PreserveConversationText: hosted}
					svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}
					started, calls := time.Now(), provider.calls
					result, err := svc.Complete(t.Context(), Request{Message: message, History: history}, nil)
					command := result.Response.Motion
					if err == nil && command != nil && command.Layered != nil {
						score = *motion.CloneFlowSpec(command.Layered)
					}
					pass := err == nil && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback && provider.calls-calls == 1
					pass = pass && naturalConversationIntent(i, before, score)
					row := hostedLayeredLiveRow(fmt.Sprintf("conversation/%s/%02d", mode, i+1), model, message, result.Raw, before, command, limits, started, err, pass)
					row["method"], row["reply"], row["provider_calls"] = mode, result.Response.Reply, provider.calls-calls
					row["repaired"], row["repair_raw"], row["fallback"], row["history"] = result.Repaired, result.RepairRaw, result.SemanticFallback, append([]llm.Message{}, history...)
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: result.Response.Reply})
					requests = append(requests, message)
					t.Logf("intent=%t calls=%d elapsed=%v error=%v", pass, provider.calls-calls, row["elapsed_ms"], err)
					if !pass {
						t.Error("conversation intent failed; retained for review")
					}
				})
			}
		})
	}
}

func naturalConversationIntent(turn int, before, after motion.FlowSpec) bool {
	samePace := before.SpeedPercent == after.SpeedPercent
	fullBand := after.MinPercent == 0 && after.MaxPercent == 100
	switch turn {
	case 0:
		return after.MinPercent >= 65 && after.MaxPercent == 100 && samePace
	case 1:
		if after.Gesture != nil {
			return fullBand && samePace && after.Gesture.FocusMixPercent > 0 && after.Gesture.FocusMixPercent < 100 && after.Gesture.FocusPercent == 100 && after.Gesture.FocusRoamPercent == 0
		}
		return fullBand && samePace && after.AnchorPercent == 100 && effectiveWidth(after).RangeCeilingPercent == 100 && after.RangeFloorPercent < 100
	case 2:
		if after.Gesture != nil {
			return fullBand && samePace && after.Gesture.FocusWidthPercent > before.Gesture.FocusWidthPercent && after.Gesture.FocusPercent == 100 && after.Gesture.FocusRoamPercent == 0
		}
		return fullBand && samePace && after.RangeFloorPercent > before.RangeFloorPercent && effectiveWidth(after).RangeCeilingPercent == 100 && after.AnchorPercent == 100
	case 3:
		if after.Gesture != nil {
			return samePace && after.Gesture.FocusRoamPercent > 0 && after.Gesture.FocusMixPercent > 0 && after.Gesture.FocusMixPercent < 100
		}
		center := false
		for _, layer := range after.Layers {
			center = center || layer.Axis == "center" && layer.AmountPercent > 0
		}
		// The carrier already varies width between the floor and ceiling;
		// a range layer is optional modulation, not a requirement for variation.
		return samePace && after.Phrase == nil && center && after.RangeFloorPercent < effectiveWidth(after).RangeCeilingPercent
	case 4, 5, 10:
		a, b := before, after
		a.SpeedPercent, b.SpeedPercent = 0, 0
		if !reflect.DeepEqual(a, b) {
			return false
		}
		if turn == 10 {
			return after.SpeedPercent == min(100, before.SpeedPercent+7)
		}
		return (turn == 4 && after.SpeedPercent < before.SpeedPercent) || (turn == 5 && after.SpeedPercent > before.SpeedPercent)
	case 6:
		if after.Gesture != nil {
			return samePace && fullBand && after.Gesture.FocusMixPercent == 0
		}
		return samePace && fullBand && after.RangeFloorPercent == 100 && effectiveWidth(after).RangeCeilingPercent == 100
	case 7, 9:
		return reflect.DeepEqual(before, after)
	case 8:
		a, b := before, after
		a.Seed, b.Seed = 0, 0
		return before.Seed != after.Seed && reflect.DeepEqual(a, b)
	}
	return false
}

func TestNaturalVibrationConversationLive(t *testing.T) {
	provider, model, hosted := naturalLiveProvider(t)
	if !hosted {
		t.Skip("independent control requires Advanced capability")
	}
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent, limits.SpeedMaxPercent = 10, 100
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		t.Run(string(mode), func(t *testing.T) {
			score := DefaultLayeredScore(43)
			if mode == MotionModeCreativeV2 {
				score = FreshCreativeV2Score(43)
			}
			score.Seed = 20261025
			current := motion.VibrationSpec{Mode: "wave", MinPercent: 15, MaxPercent: 70, PeriodSeconds: 8}
			history := []llm.Message{}
			for i, message := range []string{
				"Keep The Handy exactly unchanged. Change only vibration to distinct pulses from silence to 65 percent of my allowed intensity, once every 2 seconds.",
				"Make just those vibration pulses twice as slow. Keep their levels and The Handy unchanged.",
				"Keep the pulse pattern and the Handy. Make only the vibration peaks a bit softer; leave the silent lows alone.",
				"Let vibration flow in a continuous rolling swell instead of distinct pulses, preserving its current cycle length and levels. Keep The Handy unchanged.",
				"Choose a different, contrasting vibration character now; you can change the rhythm and levels. Keep The Handy exactly unchanged, and describe only what you choose.",
				"Leave vibration exactly as it is. Make every Handy stroke cover the whole slider at 56 percent base speed with no pace variation.",
				"Turn off only vibration. Leave The Handy moving exactly as it is.",
				"Bring the vibrator back with a gentle rolling swell of your choosing, without changing The Handy.",
				"Return the vibrator to its saved movement-follow profile. Preserve the Handy score.",
				"I might want short vibration bursts later. Just explain the options; keep both outputs unchanged for now.",
			} {
				t.Run(fmt.Sprintf("turn_%02d", i+1), func(t *testing.T) {
					before, beforeVibration := *motion.CloneFlowSpec(&score), current
					state := naturalState(&score, mode, limits)
					state.Outputs = &OutputContext{Strokers: 1, Vibrator: &VibrationContext{Independent: true, Ready: true, Armed: true, Follow: "speed", Motors: 2, Current: &current}}
					caps := Capabilities{Motion: true, MotionMode: mode, AdvancedMotion: true, PreserveConversationText: true}
					svc := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off", MaxTokens: naturalMaxTokens(), PromptBudget: naturalPromptBudget()}
					started, calls := time.Now(), provider.calls
					result, err := svc.Complete(t.Context(), Request{Message: message, History: history}, nil)
					command := result.Response.Motion
					var vibration *motion.VibrationSpec
					if command != nil && err == nil {
						vibration = command.Vibration
						if command.Layered != nil {
							score = *motion.CloneFlowSpec(command.Layered)
						}
						if vibration != nil {
							current = *vibration
						}
					}
					pass := err == nil && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback && provider.calls-calls == 1
					if i != 5 {
						pass = pass && reflect.DeepEqual(before, score)
					}
					switch i {
					case 0:
						pass = pass && current == (motion.VibrationSpec{Mode: "pulse", MaxPercent: 65, PeriodSeconds: 2})
					case 1:
						pass = pass && current == (motion.VibrationSpec{Mode: "pulse", MaxPercent: 65, PeriodSeconds: 4})
					case 2:
						pass = pass && current.Mode == "pulse" && current.MinPercent == 0 && current.MaxPercent < beforeVibration.MaxPercent && current.MaxPercent > 0 && current.PeriodSeconds == beforeVibration.PeriodSeconds
					case 3:
						pass = pass && current.Mode == "wave" && current.MinPercent == beforeVibration.MinPercent && current.MaxPercent == beforeVibration.MaxPercent && current.PeriodSeconds == beforeVibration.PeriodSeconds
					case 4:
						pass = pass && current != beforeVibration && current.Mode != "off" && current.Mode != "follow"
					case 5:
						pass = pass && vibration == nil && score.SpeedPercent == 56 && score.MinPercent == 0 && score.MaxPercent == 100
					case 6:
						pass = pass && current.Mode == "off"
					case 7:
						pass = pass && current.Mode == "wave" && current.MinPercent < current.MaxPercent
					case 8:
						pass = pass && current == (motion.VibrationSpec{Mode: "follow"})
					case 9:
						pass = pass && vibration == nil
					}
					row := hostedLayeredLiveRow(fmt.Sprintf("vibration_conversation/%s/%02d", mode, i+1), model, message, result.Raw, before, command, limits, started, err, pass)
					row["method"], row["reply"], row["provider_calls"] = mode, result.Response.Reply, provider.calls-calls
					row["current_vibration"], row["vibration"], row["repaired"], row["repair_raw"] = beforeVibration, vibration, result.Repaired, result.RepairRaw
					rows = append(rows, row)
					saveHostedLayeredRows(t, rows)
					history = append(history, llm.Message{Role: "user", Content: message}, llm.Message{Role: "assistant", Content: result.Response.Reply})
					t.Logf("intent=%t calls=%d elapsed=%v error=%v", pass, provider.calls-calls, row["elapsed_ms"], err)
					if !pass {
						t.Error("independent conversation intent failed; retained for review")
					}
				})
			}
		})
	}
}
