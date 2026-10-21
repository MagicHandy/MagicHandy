//go:build magichandy_labs

// Inert second-opinion evaluation. No engine, transport or device is created.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

type reviewCase struct {
	id    string
	input chat.MotionReviewInput
	check func(motion.FlowSpec) bool
	bad   bool
}

func main() {
	dataDir := flag.String("data-dir", "", "existing signed-in profile; credentials are not copied")
	captures := flag.String("captures", "", "directory containing retained gemma/sol production captures")
	output := flag.String("output", ".scratch/motion-second-opinion.json", "ignored evidence output")
	feedback := flag.Bool("feedback", false, "also ask local Gemma to implement the review feedback")
	model := flag.String("model", "gpt-6-sol", "review model from the connected ChatGPT account")
	localModel := flag.String("local-model", "igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0", "feedback model")
	flag.Parse()
	manager, err := openaiauth.Open(openaiauth.Options{DataDir: *dataDir})
	must(err)
	defer manager.Close()
	token, err := manager.ActiveTokenSource(context.Background())
	must(err)
	provider, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{Model: *model, PlanUsage: true, ReasoningEffort: "low", Token: token, Timeout: 30 * time.Second})
	must(err)
	local, err := llm.NewOllamaProvider(llm.HTTPProviderOptions{BaseURL: "http://127.0.0.1:11434", Model: *localModel, Timeout: 30 * time.Second})
	must(err)
	service := chat.MotionReviewService{Provider: provider, Model: *model, Capabilities: chat.Capabilities{Motion: true, MotionMode: chat.MotionModeCreativeV2, Voice: chat.VoiceUtility}}
	cases := productionCases(*captures)
	cases = append(cases, autopilotCases()...)
	rows := []map[string]any{}
	for _, item := range cases {
		started := time.Now()
		review, err := service.Complete(context.Background(), item.input)
		elapsed := time.Since(started).Milliseconds()
		after := *motion.CloneFlowSpec(item.input.After.Flow)
		if err == nil && review.Motion != nil {
			after = *motion.CloneFlowSpec(review.Motion.Layered)
		}
		row := map[string]any{"case": item.id, "message": item.input.Request, "source": item.input.Source, "model": *model, "method": "creative_v2", "before": item.input.After.Flow, "after": after, "limits": item.input.Limits, "expected_intervention": item.bad, "review": review, "elapsed_ms": elapsed, "valid": err == nil, "intent_pass": err == nil && item.check(after) && (review.Verdict == "correct") == item.bad, "changed": !reflect.DeepEqual(after, *item.input.After.Flow)}
		if err != nil {
			row["error"] = err.Error()
		}
		if target, e := motion.FlowTarget(after, item.input.Limits); e == nil {
			row["motion"] = motion.ReviewMotionOutput(target, item.input.Limits)
		}
		if *feedback && err == nil && review.Verdict == "correct" {
			trial := chat.RunLLMLab(context.Background(), local, *localModel, "creative_v2", chat.LLMLabPrompts()["creative_v2"], "Revise the current motion to satisfy this request: "+item.input.Request+"\nA second opinion found a material error: "+review.Feedback+"\nEmit the needed edits; preserve unrelated controls.", *item.input.After.Flow, item.input.Limits, nil, true)
			row["feedback_trial"] = trial
			row["feedback_pass"] = trial.Valid && item.check(trial.After)
		}
		rows = append(rows, row)
		encoded, e := json.MarshalIndent(map[string]any{"turns": rows}, "", "  ")
		must(e)
		must(os.WriteFile(*output, encoded, 0600))
		fmt.Printf("%s expected=%t verdict=%s reason=%s pass=%v ms=%d err=%v\n", item.id, item.bad, review.Verdict, review.Reason, row["intent_pass"], elapsed, err)
	}
}

func productionCases(directory string) []reviewCase {
	paths, err := filepath.Glob(filepath.Join(directory, "*-production*.json"))
	must(err)
	cases := []reviewCase{}
	for _, path := range paths {
		if !strings.HasPrefix(filepath.Base(path), "gemma-") && !strings.HasPrefix(filepath.Base(path), "sol-") {
			continue
		}
		var capture struct {
			Turns []struct {
				Request  string `json:"request"`
				Response string `json:"response"`
			} `json:"turns"`
		}
		data, e := os.ReadFile(path)
		must(e)
		must(json.Unmarshal(data, &capture))
		limits := config.DefaultSettings().Motion
		limits.SpeedMinPercent = 20
		limits.SpeedMaxPercent = 80
		initial := chat.FreshCreativeV2Score(25)
		before, _ := motion.FlowTarget(initial, limits)
		var history []string
		for i, turn := range capture.Turns {
			if i >= 5 {
				break
			}
			after := before
			for _, block := range strings.Split(strings.ReplaceAll(turn.Response, "\r\n", "\n"), "\n\n") {
				if !strings.HasPrefix(block, "event: motion\n") {
					continue
				}
				var event struct {
					Engine motion.ActiveMotionState `json:"engine"`
				}
				parts := strings.SplitN(block, "data: ", 2)
				if len(parts) == 2 {
					must(json.Unmarshal([]byte(parts[1]), &event))
					if event.Engine.Target.Flow != nil {
						after = event.Engine.Target
					}
				}
			}
			check := productionCheck(i, *before.Flow, *after.Flow)
			input := chat.MotionReviewInput{Source: "chat", Request: turn.Request, RecentRequests: append([]string(nil), history...), Before: before, After: after, Limits: limits}
			cases = append(cases, reviewCase{id: fmt.Sprintf("%s/%d", filepath.Base(path), i), input: input, check: check, bad: !check(*after.Flow)})
			history = append(history, turn.Request)
			before = after
		}
	}
	return cases
}

func productionCheck(index int, before, applied motion.FlowSpec) func(motion.FlowSpec) bool {
	return func(s motion.FlowSpec) bool {
		g := s.Gesture
		if g == nil {
			return false
		}
		switch index {
		case 0:
			limits := config.DefaultSettings().Motion
			limits.SpeedMinPercent, limits.SpeedMaxPercent = 20, 80
			target, err := motion.FlowTarget(s, limits)
			if err != nil {
				return false
			}
			feel := motion.ReviewMotionOutput(target, limits).Summary
			return feel.PositionMinPercent >= 70 && feel.PositionMaxPercent <= 100 && g.FasterDirection == "tip" && g.ContrastPercent > 0 && s.SpeedPercent == applied.SpeedPercent
		case 1:
			return g.InertiaPercent > before.Gesture.InertiaPercent && s.SpeedPercent == before.SpeedPercent && s.MinPercent == before.MinPercent && s.MaxPercent == before.MaxPercent
		case 2:
			return s.MinPercent == 0 && s.MaxPercent == 100 && g.FocusPercent == 0 && g.FocusMixPercent > 0 && g.FocusMixPercent < 100 && g.FocusWidthPercent == 45 && g.ReboundCount == 3 && g.ReboundDecayPercent == 75 && s.SpeedPercent == before.SpeedPercent
		case 3:
			return g.ReboundCount == 0 && s.SpeedPercent == before.SpeedPercent
		default:
			// An already varying score is not a material failure merely because
			// its seed did not change. The reviewer should avoid cosmetic edits.
			return chat.CreativeV2CharacterUnchanged(applied, s)
		}
	}
}

func autopilotCases() []reviewCase {
	limits := config.DefaultSettings().Motion
	limits.SpeedMinPercent = 15
	limits.SpeedMaxPercent = 54
	base := chat.FreshCreativeV2Score(18)
	before, _ := motion.FlowTarget(base, limits)
	cases := []reviewCase{}
	add := func(id, request string, next motion.FlowSpec, bad bool, check func(motion.FlowSpec) bool) {
		after, err := motion.FlowTarget(next, limits)
		must(err)
		facts := chat.AutopilotContext{MotionMode: chat.MotionModeCreativeV2, CurrentFlow: &next, CurrentSpeed: next.SpeedPercent, SpeedMinPercent: 15, SpeedMaxPercent: 54, MotionChangeLevel: 5, SessionTracking: true, SessionSeconds: 240, SecondsAtCurrentPhrase: 30}
		cases = append(cases, reviewCase{id: id, bad: bad, check: check, input: chat.MotionReviewInput{Source: "autopilot", Request: "Review this autonomous continuation.", RecentRequests: []string{request}, Before: before, After: after, Limits: limits, Autopilot: &facts}})
	}
	next := *motion.CloneFlowSpec(&base)
	next.SpeedPercent = 48
	add("autopilot/lasting-slow-violated", "Keep it slow for this whole session, until I ask you to change pace.", next, true, func(s motion.FlowSpec) bool { return s.SpeedPercent <= 25 })
	next = *motion.CloneFlowSpec(&base)
	next.SpeedPercent = 20
	add("autopilot/lasting-slow-respected", "Keep it slow for this whole session, until I ask you to change pace.", next, false, func(s motion.FlowSpec) bool { return reflect.DeepEqual(s, next) })
	free := *motion.CloneFlowSpec(&base)
	free.SpeedPercent = 32
	free.Gesture.FocusPercent = 65
	free.Gesture.FocusMixPercent = 45
	add("autopilot/allowed-development", "Develop the motion naturally and use the saved pace range.", free, false, func(s motion.FlowSpec) bool { return reflect.DeepEqual(s, free) })
	held := *motion.CloneFlowSpec(&base)
	add("autopilot/requested-hold", "Keep this exact score unchanged until I ask otherwise.", held, false, func(s motion.FlowSpec) bool { return reflect.DeepEqual(s, held) })
	wrong := *motion.CloneFlowSpec(&base)
	wrong.Gesture.FocusPercent = 0
	wrong.Gesture.FocusMixPercent = 100
	add("autopilot/lasting-region-violated", "Keep every stroke inside the upper quarter, from 75 to 100, until I say otherwise.", wrong, true, func(s motion.FlowSpec) bool { return s.MinPercent >= 75 && s.MaxPercent <= 100 && s.SpeedPercent == 18 })
	question := *motion.CloneFlowSpec(&base)
	item := reviewCase{id: "chat/question-no-change", bad: false, check: func(s motion.FlowSpec) bool { return reflect.DeepEqual(s, question) }, input: chat.MotionReviewInput{Source: "chat", Request: "What does the preview estimate? Do not change motion.", Before: before, After: before, Limits: limits}}
	cases = append(cases, item)
	return cases
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
