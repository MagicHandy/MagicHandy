//go:build liveeval

package httpapi

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/openaiauth"
)

// timingLine is a human line said at a session second. Chat answers it in
// words, and the planner decides again at once, as ReconsiderAfterChat does.
type timingLine struct {
	at   int
	text string
}

type timingScenario struct {
	name  string
	turns int
	lines []timingLine
}

// The lines are neutral pacing requests, and the planner gets no persona: the
// hosted run uses a real account, and timing judgment needs no more.
var timingScenarios = []timingScenario{
	{name: "open", turns: 12},
	{name: "slow_build", turns: 12, lines: []timingLine{{0, "Build the pace up gradually over the next few minutes."}}},
	{name: "too_fast", turns: 12, lines: []timingLine{{0, "Faster, please."}, {150, "That is too fast now, ease off."}}},
	{name: "keep", turns: 10, lines: []timingLine{{0, "This is good. Keep it exactly like this for a while."}}},
}

// timingTurn is one planning decision in a simulated auto-timing session.
type timingTurn struct {
	At        int      `json:"session_seconds"`
	Human     string   `json:"human,omitempty"`
	Raw       string   `json:"raw"`
	Error     string   `json:"error,omitempty"`
	Held      bool     `json:"held"`
	Changed   []string `json:"changed,omitempty"`
	Speed     int      `json:"speed_percent"`
	NextCheck int      `json:"next_check_seconds"`
	Played    int      `json:"played_seconds"`
	LatencyMS int64    `json:"latency_ms"`
}

type timingRun struct {
	Scenario string       `json:"scenario"`
	Model    string       `json:"model"`
	Hosted   bool         `json:"hosted"`
	Turns    []timingTurn `json:"turns"`
}

// TestLiveAutopilotAutoTiming drives continuous Creative v2 Autopilot planning
// with auto timing through the production AutopilotService, prompt, parser and
// compiler. Each stretch lasts the planner's own next_check_seconds, and a
// human line cuts it short the way a chat turn brings the next decision
// forward. It builds no engine or transport, so it cannot move a device.
//
// MAGICHANDY_HOSTED_DATA_DIR with MAGICHANDY_LIVE_MODEL uses that data
// directory's existing ChatGPT sign-in in place: only the token closure
// reaches the provider, and no credential is copied. Otherwise
// MAGICHANDY_LIVE_LLAMA_URL selects a loopback llama.cpp server, which runs
// with auto timing forced on as a control.
// MAGICHANDY_TIMING_SCENARIOS limits scenarios (comma separated names).
// MAGICHANDY_TIMING_RUNS sets runs per scenario (default 1).
// MAGICHANDY_EXPERIMENT_CAPTURE writes the JSON report.
func TestLiveAutopilotAutoTiming(t *testing.T) {
	provider, model, hosted := timingLiveProvider(t)
	recorder := &liveAutopilotRecorder{Provider: provider}
	selected := strings.Split(strings.TrimSpace(os.Getenv("MAGICHANDY_TIMING_SCENARIOS")), ",")
	runs := liveSessionEnvInt("MAGICHANDY_TIMING_RUNS", 1)
	var report []timingRun
	for _, scenario := range timingScenarios {
		if selected[0] != "" && !slices.Contains(selected, scenario.name) {
			continue
		}
		for run := range runs {
			result := runTimingSession(t, recorder, model, hosted, scenario, int64(run)+1)
			report = append(report, result)
			t.Log(summarizeTimingRun(result))
			if path := strings.TrimSpace(os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE")); path != "" {
				encoded, _ := json.MarshalIndent(report, "", " ")
				if err := os.WriteFile(path, encoded, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func timingLiveProvider(t *testing.T) (llm.Provider, string, bool) {
	t.Helper()
	if directory := strings.TrimSpace(os.Getenv("MAGICHANDY_HOSTED_DATA_DIR")); directory != "" {
		model := strings.TrimSpace(os.Getenv("MAGICHANDY_LIVE_MODEL"))
		if model == "" {
			t.Skip("set MAGICHANDY_LIVE_MODEL with MAGICHANDY_HOSTED_DATA_DIR")
		}
		manager, err := openaiauth.Open(openaiauth.Options{DataDir: directory})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(manager.Close)
		token, err := manager.ActiveTokenSource(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		provider, err := llm.NewOpenAIResponsesProvider(llm.OpenAIOptions{Model: model, PlanUsage: true, ReasoningEffort: "low",
			Token: token, ContextSize: config.MinimumExternalContext, Timeout: 60 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		return provider, model, true
	}
	baseURL, model := liveAutopilotProvider(t)
	provider, err := newLiveAutopilotProvider(llm.HTTPProviderOptions{BaseURL: baseURL, Model: model, Timeout: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return provider, model, false
}

func runTimingSession(t *testing.T, recorder *liveAutopilotRecorder, model string, hosted bool, scenario timingScenario, seed int64) timingRun {
	t.Helper()
	settings := config.DefaultSettings()
	settings.LLM.MotionGenerationMode = config.LLMMotionModeCreativeV2
	settings.Motion.SpeedMinPercent, settings.Motion.SpeedMaxPercent = 15, 60
	settings.Motion.StrokeMinPercent, settings.Motion.StrokeMaxPercent = 0, 100
	settings.Motion.HandyModel = config.HandyModelOriginal
	llmSettings := settings.LLM
	if hosted {
		llmSettings = llmSettings.WithConnection(config.ModelConnection{ID: "timing-review", Name: "Timing review",
			Provider: config.LLMProviderChatGPT, Model: model, ReasoningEffort: "low"})
	}
	capabilities := chatCapabilities(llmSettings, nil)
	capabilities.AutoTiming = true
	capabilities.Voice, capabilities.Style = chat.VoiceUtility, chat.StyleNeutral
	prompt, _ := chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	// #nosec G404 -- reproducible fallback timing for an evaluation, not security material.
	fallback := rand.New(rand.NewSource(seed))

	score := chat.FreshCreativeV2Score(25)
	input := modes.DecisionInput{Style: "balanced", SpeedMinPercent: 15, SpeedMaxPercent: 60, MotionMinSeconds: 20,
		MotionMaxSeconds: 60, MotionChangeLevel: 4, SessionTracking: true}
	var history []llm.Message
	var historyAt []time.Time
	var humanLines []string
	var humanAt []int
	var speedMarks [][2]int
	result := timingRun{Scenario: scenario.name, Model: model, Hosted: hosted}
	clock, next := 0, 0
	for range scenario.turns {
		turn := timingTurn{At: clock}
		for next < len(scenario.lines) && scenario.lines[next].at <= clock {
			line := scenario.lines[next]
			humanLines, humanAt = append(humanLines, line.text), append(humanAt, clock)
			said := time.Unix(int64(clock), 0)
			history = append(history, llm.Message{Role: "user", Content: line.text}, llm.Message{Role: "assistant", Content: "Okay."})
			historyAt = append(historyAt, said, said)
			turn.Human = line.text
			next++
		}
		plan := motion.NewMotionPlan("timing-review", motion.MotionTarget{Label: "Autopilot", Source: "autopilot",
			SpeedPercent: score.SpeedPercent, Flow: motion.CloneFlowSpec(&score)}, settings.Motion, 0, 0, time.Unix(0, 0))
		perceptual := plan.Perceptual
		input.SegmentIndex = len(result.Turns)
		input.CurrentSpeed, input.CurrentFlow, input.CurrentPerceptual = score.SpeedPercent, motion.CloneFlowSpec(&score), &perceptual
		input.SessionSeconds = clock
		input.RecentSpeeds = input.RecentSpeeds[:0]
		for _, mark := range speedMarks {
			if clock-mark[1] <= 180 {
				input.RecentSpeeds = append(input.RecentSpeeds, modes.SpeedStep{SpeedPercent: mark[0], SecondsAgo: clock - mark[1]})
			}
		}
		base := time.Unix(0, 0)
		timeline := make([]chat.RecentUserRequest, len(humanLines))
		for i, text := range humanLines {
			timeline[i] = chat.RecentUserRequest{Text: text, At: base.Add(time.Duration(humanAt[i]) * time.Second)}
		}
		state := chat.MotionContext{SpeedMinPercent: 15, SpeedMaxPercent: 60, MotionMode: chat.MotionModeCreativeV2,
			Envelope: motion.CurrentPlanningEnvelope(settings.Motion), Running: true, SpeedPercent: score.SpeedPercent,
			Layered: motion.CloneFlowSpec(&score)}
		state.UserRequests, state.UserRequestSecondsAgo = chat.UserRequestTimeline(chat.SelectRecentUserRequestTimeline(timeline), base.Add(time.Duration(clock)*time.Second))
		planning := autopilotPromptContext(input, capabilities)
		if ages := state.UserRequestSecondsAgo; len(ages) > 0 {
			planning.LastHumanSecondsAgo = &ages[len(ages)-1]
		}
		messages := history
		if !hosted {
			messages = chat.PlanningHistory(history, historyAt, time.Unix(int64(clock), 0))
		}
		service := chat.AutopilotService{Provider: recorder, Prompt: prompt, Model: model, MaxTokens: settings.LLM.MaxOutputTokens,
			ReasoningMode: "off", Capabilities: capabilities, MotionContext: &state, ConversationContext: &chat.ConversationContext{},
			Temperature: autopilotTemperature(chat.AutopilotKindMotion, input.MotionChangeLevel)}
		ctx, cancel := contextWithTimeout()
		started := time.Now()
		response, err := service.Complete(ctx, chat.AutopilotKindMotion, chat.Request{Message: chat.AutopilotMotionMessage(planning), History: messages})
		cancel()
		turn.LatencyMS = time.Since(started).Milliseconds()
		turn.Raw, turn.NextCheck = recorder.LastRaw, response.NextCheckSeconds
		updated := score
		switch {
		case err != nil:
			turn.Error, turn.Held = err.Error(), true
		case response.Motion == nil || response.Motion.Layered == nil:
			turn.Held = true
		default:
			decision, mapErr := mapContinuousAutopilotCommand(response.Motion, settings, response.Reply, modes.TimingNormal)
			if mapErr != nil {
				turn.Error, turn.Held = mapErr.Error(), true
			} else {
				updated = *motion.CloneFlowSpec(decision.Segment.Flow)
			}
		}
		if turn.Held || continuousSameScore(score, updated) {
			turn.Held = true
			input.ConsecutiveHolds++
			input.DecisionsAtCurrentPhrase++
		} else {
			turn.Changed = continuousChangedGroups(score, updated)
			input.ConsecutiveHolds, input.DecisionsAtCurrentPhrase, input.SecondsAtCurrentPhrase = 0, 0, 0
			if updated.SpeedPercent != score.SpeedPercent {
				input.SecondsAtCurrentSpeed = 0
			}
			score = updated
		}
		turn.Speed = score.SpeedPercent
		speedMarks = append(speedMarks, [2]int{score.SpeedPercent, clock})
		horizon := turn.NextCheck
		if horizon <= 0 {
			// No horizon: the scheduler samples level 4's adaptive normal window.
			horizon = 28 + fallback.Intn(25)
		} else {
			horizon = min(max(horizon, config.AutopilotAutoTimingMinSeconds), config.AutopilotAutoTimingMaxSeconds)
			input.RecentCheckSeconds = append(input.RecentCheckSeconds, horizon)
			if len(input.RecentCheckSeconds) > 6 {
				input.RecentCheckSeconds = input.RecentCheckSeconds[len(input.RecentCheckSeconds)-6:]
			}
		}
		end := clock + horizon
		if next < len(scenario.lines) && scenario.lines[next].at < end {
			end = max(clock+1, scenario.lines[next].at)
		}
		turn.Played = end - clock
		input.SecondsAtCurrentSpeed += turn.Played
		input.SecondsAtCurrentPhrase += turn.Played
		clock = end
		result.Turns = append(result.Turns, turn)
		t.Logf("%s at=%ds held=%t changed=%v speed=%d next_check=%d played=%d latency=%dms err=%q human=%q",
			scenario.name, turn.At, turn.Held, turn.Changed, turn.Speed, turn.NextCheck, turn.Played, turn.LatencyMS, turn.Error, turn.Human)
	}
	return result
}

func summarizeTimingRun(run timingRun) string {
	var checks, holdChecks, changeChecks []int
	missing, errors, round, total := 0, 0, 0, 0
	for _, turn := range run.Turns {
		total += turn.Played
		if turn.Error != "" {
			errors++
		}
		if turn.NextCheck <= 0 {
			missing++
			continue
		}
		checks = append(checks, turn.NextCheck)
		if turn.NextCheck%15 == 0 {
			round++
		}
		if turn.Held {
			holdChecks = append(holdChecks, turn.NextCheck)
		} else {
			changeChecks = append(changeChecks, turn.NextCheck)
		}
	}
	sorted := slices.Clone(checks)
	slices.Sort(sorted)
	median := 0
	if len(sorted) > 0 {
		median = sorted[len(sorted)/2]
	}
	distinct := len(slices.Compact(slices.Clone(sorted)))
	mean := func(values []int) string {
		if len(values) == 0 {
			return "-"
		}
		sum := 0
		for _, value := range values {
			sum += value
		}
		return fmt.Sprintf("%.0f", float64(sum)/float64(len(values)))
	}
	calls := 0.0
	if total > 0 {
		calls = float64(len(run.Turns)) * 60 / float64(total)
	}
	low, high := 0, 0
	if len(sorted) > 0 {
		low, high = sorted[0], sorted[len(sorted)-1]
	}
	return fmt.Sprintf("%s model=%s decisions=%d checks=%v range=%d-%d median=%d distinct=%d multiples_of_15=%d missing=%d errors=%d hold_mean=%s change_mean=%s session=%ds calls_per_min=%.2f",
		run.Scenario, run.Model, len(run.Turns), checks, low, high, median, distinct, round, missing, errors, mean(holdChecks), mean(changeChecks), total, calls)
}
