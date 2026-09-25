//go:build liveeval

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/modes"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// continuousSessionTurn is one scheduled decision, chat reply or spoken
// check-in in a simulated Autopilot session. Samples are the shared plan's
// semantic positions for the stretch a motion decision governs, so a report
// renders without a transport.
type continuousSessionTurn struct {
	Turn       int                       `json:"turn"`
	Kind       string                    `json:"kind"`
	At         int                       `json:"session_seconds"`
	Raw        string                    `json:"raw"`
	Reply      string                    `json:"reply,omitempty"`
	Error      string                    `json:"error,omitempty"`
	Held       bool                      `json:"held"`
	Requested  bool                      `json:"requested_hold,omitempty"`
	Stay       *bool                     `json:"stay_unchanged,omitempty"`
	Changed    []string                  `json:"changed,omitempty"`
	Score      *motion.FlowSpec          `json:"score,omitempty"`
	Summary    *motion.PerceptualSummary `json:"summary,omitempty"`
	Samples    [][2]float64              `json:"samples,omitempty"`
	DurationMS int64                     `json:"duration_ms"`
}

type continuousSessionRun struct {
	Mode     string                  `json:"mode"`
	Model    string                  `json:"model"`
	Persona  string                  `json:"persona"`
	Settings *config.MotionSettings  `json:"settings,omitempty"`
	Requests map[int]string          `json:"requests,omitempty"`
	Turns    []continuousSessionTurn `json:"turns"`
}

// sessionStretch approximates the cadence of a real Creative v2 session: its
// retained trace planned a new decision every 13-15 seconds.
const sessionStretch = 14 * time.Second

// TestLiveContinuousAutopilotSession drives consecutive Autopilot decisions in
// a continuous mode through the production AutopilotService, parser, validator
// and shared compiler. It carries the score, compiled feel, recent position
// bands, spoken check-ins and human messages between turns the way the
// scheduler and chat log do. Motion-turn replies are recorded but, as in the
// app, never published. A standing wish that a chat reply declares holds
// Autopilot without a model call until a later reply releases it, as the
// server does. It builds no engine or transport, so it cannot move a device.
//
// MAGICHANDY_LIVE_LLAMA_URL selects a loopback llama.cpp server.
// MAGICHANDY_SESSION_MODE is creative_v2 (default) or layered.
// MAGICHANDY_SESSION_TURNS sets motion decisions per run (default 20).
// MAGICHANDY_SESSION_RUNS sets independent runs (default 1).
// MAGICHANDY_SESSION_REQUESTS injects human lines as "turn:text|turn:text".
// MAGICHANDY_EXPERIMENT_CAPTURE writes the JSON report.
func TestLiveContinuousAutopilotSession(t *testing.T) {
	baseURL, model := liveAutopilotProvider(t)
	provider, err := newLiveAutopilotProvider(llm.HTTPProviderOptions{BaseURL: baseURL, Model: model, Timeout: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &liveAutopilotRecorder{Provider: provider}
	mode := chat.MotionModeCreativeV2
	if strings.TrimSpace(os.Getenv("MAGICHANDY_SESSION_MODE")) == string(chat.MotionModeLayered) {
		mode = chat.MotionModeLayered
	}
	turns := liveSessionEnvInt("MAGICHANDY_SESSION_TURNS", 20)
	runs := liveSessionEnvInt("MAGICHANDY_SESSION_RUNS", 1)
	requests := liveSessionRequests(os.Getenv("MAGICHANDY_SESSION_REQUESTS"))
	personas := []chat.ConversationContext{
		{PersonaName: "Mara", PersonaDescription: "A confident, teasing woman in her thirties who enjoys taking her time and setting the pace.", CurrentMood: chat.MoodSeductive},
		{PersonaName: "Theo", PersonaDescription: "A soft-spoken, eager-to-please man who reads his partner closely and follows their lead.", CurrentMood: chat.MoodSeductive},
	}
	var report []continuousSessionRun
	for run := range runs {
		session := liveContinuousSession{t: t, recorder: recorder, model: model, mode: mode,
			persona: personas[run%len(personas)], requests: requests,
			// #nosec G404 -- reproducible evaluation cadence, not security material.
			cadence: rand.New(rand.NewSource(int64(run) + 1))}
		result := session.run(turns)
		report = append(report, result)
		logContinuousSessionSummary(t, run, result)
		if path := strings.TrimSpace(os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE")); path != "" {
			encoded, _ := json.MarshalIndent(report, "", " ")
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type liveContinuousSession struct {
	t        *testing.T
	recorder *liveAutopilotRecorder
	model    string
	mode     chat.MotionMode
	persona  chat.ConversationContext
	requests map[int]string
	cadence  *rand.Rand

	settings     config.Settings
	capabilities chat.Capabilities
	prompt       chat.PromptSet
	score        motion.FlowSpec
	history      []llm.Message
	humanLines   []string
	input        modes.DecisionInput
	offset       int64
	nextSpeech   time.Duration
	standingHold bool
}

func (s *liveContinuousSession) setup() {
	s.prompt, _ = chat.BuiltinPromptSetByID(chat.DefaultPromptSetID)
	s.settings = config.DefaultSettings()
	s.settings.LLM.MotionGenerationMode = string(s.mode)
	s.settings.Motion.SpeedMinPercent, s.settings.Motion.SpeedMaxPercent = 15, 54
	s.settings.Motion.StrokeMinPercent, s.settings.Motion.StrokeMaxPercent = 0, 100
	s.settings.Motion.HandyModel = config.HandyModelOriginal
	s.capabilities = chat.FullCapabilities()
	s.capabilities.MotionMode = s.mode
	s.capabilities.Voice = chat.VoiceExplicit
	s.capabilities.MoodTracking = true
	s.score = chat.FreshLayeredScore(25)
	if s.mode == chat.MotionModeCreativeV2 {
		s.score = chat.FreshCreativeV2Score(25)
	}
	s.input = modes.DecisionInput{Style: "balanced", SpeedMinPercent: 15, SpeedMaxPercent: 54, MotionMinSeconds: 20,
		MotionMaxSeconds: 60, MotionChangeLevel: 8, SessionTracking: true, ArcEnabled: true}
	s.nextSpeech = s.speechInterval()
}

// speechInterval mirrors the talkative preset's adaptive normal window,
// 20-80% of 35-120 seconds.
func (s *liveContinuousSession) speechInterval() time.Duration {
	return time.Duration(52+s.cadence.Intn(52)) * time.Second
}

func (s *liveContinuousSession) motionContext() chat.MotionContext {
	return chat.MotionContext{SpeedMinPercent: 15, SpeedMaxPercent: 54, MotionMode: s.mode,
		Envelope: motion.CurrentPlanningEnvelope(s.settings.Motion), Running: true, SpeedPercent: s.score.SpeedPercent,
		Layered: motion.CloneFlowSpec(&s.score), UserRequests: chat.SelectRecentUserRequests(s.humanLines)}
}

func (s *liveContinuousSession) run(turns int) continuousSessionRun {
	s.setup()
	result := continuousSessionRun{Mode: string(s.mode), Model: s.model, Persona: s.persona.PersonaName, Settings: &s.settings.Motion,
		Requests: s.requests}
	for turn := range turns {
		if text, ok := s.requests[turn]; ok {
			result.Turns = append(result.Turns, s.chatTurn(turn, text))
		}
		result.Turns = append(result.Turns, s.motionTurn(turn))
		elapsed := time.Duration(turn+1) * sessionStretch
		if elapsed >= s.nextSpeech {
			result.Turns = append(result.Turns, s.speechTurn(turn))
			s.nextSpeech = elapsed + s.speechInterval()
		}
	}
	return result
}

// chatTurn gives a human line an ordinary interactive reply, which may itself
// edit the score, before Autopilot plans again.
func (s *liveContinuousSession) chatTurn(turn int, text string) continuousSessionTurn {
	s.humanLines = append(s.humanLines, text)
	context := s.motionContext()
	context.Autopilot, context.StandingHold = true, s.standingHold
	conversation := s.persona
	interactive := chat.Service{Provider: s.recorder, Prompt: s.prompt, Model: s.model, MaxTokens: 512, ReasoningMode: "off",
		Capabilities: &s.capabilities, MotionContext: &context, ConversationContext: &conversation}
	ctx, cancel := contextWithTimeout()
	result, err := interactive.Complete(ctx, chat.Request{Message: text, History: s.history}, nil)
	cancel()
	line := continuousSessionTurn{Turn: turn, Kind: "chat", At: turn * int(sessionStretch/time.Second), Raw: s.recorder.LastRaw,
		Reply: strings.TrimSpace(result.Response.Reply), Stay: result.Response.StayUnchanged}
	if err != nil {
		line.Error = err.Error()
	} else {
		if stay := result.Response.StayUnchanged; stay != nil {
			s.standingHold = *stay
		}
		if command := result.Response.Motion; command != nil && command.Layered != nil && command.Layered.Validate(s.settings.Motion) == nil {
			line.Changed = continuousChangedGroups(s.score, *command.Layered)
			s.score, s.offset = *motion.CloneFlowSpec(command.Layered), 0
			line.Score = motion.CloneFlowSpec(&s.score)
		}
	}
	s.history = append(s.history, llm.Message{Role: "user", Content: text})
	if line.Reply != "" {
		s.history = append(s.history, llm.Message{Role: "assistant", Content: line.Reply})
	}
	s.t.Logf("chat turn=%d user=%q stay=%s changed=%v err=%q reply=%q", turn, text, liveSessionStay(line.Stay), line.Changed, line.Error, line.Reply)
	return line
}

func (s *liveContinuousSession) motionTurn(turn int) continuousSessionTurn {
	plan := motion.NewMotionPlan("session-review", motion.MotionTarget{Label: "Autopilot", Source: "autopilot",
		SpeedPercent: s.score.SpeedPercent, Flow: motion.CloneFlowSpec(&s.score)}, s.settings.Motion, 0, 0, time.Unix(0, 0))
	perceptual := plan.Perceptual
	stretch := int(sessionStretch / time.Second)
	s.input.SegmentIndex = turn
	s.input.CurrentSpeed = s.score.SpeedPercent
	s.input.CurrentFlow = motion.CloneFlowSpec(&s.score)
	s.input.CurrentPerceptual = &perceptual
	s.input.SessionSeconds = turn * stretch
	s.input.ArcPercent = min(100, s.input.SessionSeconds*100/1200)
	record := continuousSessionTurn{Turn: turn, Kind: "motion", At: s.input.SessionSeconds, DurationMS: sessionStretch.Milliseconds()}
	next := s.score
	response, err := chat.AutopilotResponse{}, error(nil)
	if s.standingHold {
		record.Requested = true
	} else {
		context := s.motionContext()
		conversation := s.persona
		service := chat.AutopilotService{Provider: s.recorder, Prompt: s.prompt, Model: s.model, MaxTokens: 512, ReasoningMode: "off",
			Capabilities: s.capabilities, MotionContext: &context, ConversationContext: &conversation,
			Temperature: autopilotTemperature(chat.AutopilotKindMotion, s.input.MotionChangeLevel)}
		ctx, cancel := contextWithTimeout()
		response, err = service.Complete(ctx, chat.AutopilotKindMotion, chat.Request{
			Message: chat.AutopilotMotionMessage(autopilotPromptContext(s.input, s.capabilities)), History: s.history})
		cancel()
		record.Raw, record.Reply = s.recorder.LastRaw, strings.TrimSpace(response.Reply)
	}
	switch {
	case record.Requested:
		record.Held = true
	case err != nil:
		record.Error, record.Held = err.Error(), true
	case response.Motion == nil || response.Motion.Layered == nil:
		record.Held = true
	default:
		decision, mapErr := mapContinuousAutopilotCommand(response.Motion, s.settings, response.Reply, modes.TimingNormal)
		if mapErr != nil {
			record.Error, record.Held = mapErr.Error(), true
		} else {
			next = *motion.CloneFlowSpec(decision.Segment.Flow)
		}
	}
	if record.Held || continuousSameScore(s.score, next) {
		record.Held = true
		s.input.ConsecutiveHolds++
		s.input.DecisionsAtCurrentPhrase++
		s.input.SecondsAtCurrentPhrase += stretch
	} else {
		record.Changed = continuousChangedGroups(s.score, next)
		s.input.ConsecutiveHolds, s.input.DecisionsAtCurrentPhrase, s.input.SecondsAtCurrentPhrase = 0, 0, 0
		if next.SpeedPercent != s.score.SpeedPercent {
			s.input.SecondsAtCurrentSpeed = 0
		}
		s.score, s.offset = next, 0
	}
	s.input.SecondsAtCurrentSpeed += stretch
	played := motion.NewMotionPlan("session-review", motion.MotionTarget{Label: "Autopilot", Source: "autopilot",
		SpeedPercent: s.score.SpeedPercent, Flow: motion.CloneFlowSpec(&s.score)}, s.settings.Motion, 0, 0, time.Unix(0, 0))
	summary := played.Perceptual
	record.Score, record.Summary = motion.CloneFlowSpec(&s.score), &summary
	for at := int64(0); at <= sessionStretch.Milliseconds(); at += 50 {
		record.Samples = append(record.Samples, [2]float64{float64(at) / 1000, played.SampleAt(s.offset + at).PositionPercent})
	}
	s.offset += sessionStretch.Milliseconds()
	s.input.RecentPositionBands = append(s.input.RecentPositionBands, modes.PositionBand{MinimumPercent: summary.PositionMinPercent, MaximumPercent: summary.PositionMaxPercent})
	if len(s.input.RecentPositionBands) > 4 {
		s.input.RecentPositionBands = s.input.RecentPositionBands[len(s.input.RecentPositionBands)-4:]
	}
	s.t.Logf("turn=%d held=%t requested=%t changed=%v character=%s speed=%d range=%d-%d err=%q", turn, record.Held, record.Requested,
		record.Changed, continuousCharacter(s.score), s.score.SpeedPercent, s.score.MinPercent, s.score.MaxPercent, record.Error)
	return record
}

// speechTurn publishes an independent spoken check-in into history, as the
// Autopilot speech clock does. Its capability follows the chat-only preset.
func (s *liveContinuousSession) speechTurn(turn int) continuousSessionTurn {
	capabilities := autopilotSpeechCapabilities(s.capabilities, "chat_only")
	modelContext := autopilotPromptContext(s.input, capabilities)
	modelContext.MotionMode = s.mode
	conversation := s.persona
	speech := chat.AutopilotService{Provider: s.recorder, Prompt: s.prompt, Model: s.model, MaxTokens: 512, ReasoningMode: "off",
		Capabilities: capabilities, ConversationContext: &conversation,
		Temperature: autopilotTemperature(chat.AutopilotKindSpeech, s.input.MotionChangeLevel)}
	ctx, cancel := contextWithTimeout()
	spoken, err := speech.Complete(ctx, chat.AutopilotKindSpeech, chat.Request{Message: chat.AutopilotSpeechMessage(modelContext), History: s.history})
	cancel()
	line := continuousSessionTurn{Turn: turn, Kind: "speech", At: s.input.SessionSeconds, Raw: s.recorder.LastRaw, Reply: strings.TrimSpace(spoken.Reply)}
	if err != nil {
		line.Error = err.Error()
	} else if line.Reply != "" {
		s.history = append(s.history, llm.Message{Role: "assistant", Content: line.Reply})
		s.persona.RecentAssistantReplies = liveSessionLastReplies(s.persona.RecentAssistantReplies, line.Reply)
		s.input.LastSay = line.Reply
	}
	s.t.Logf("speech turn=%d err=%q reply=%q", turn, line.Error, line.Reply)
	return line
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// TestReplayContinuousSession recompiles a captured session's exact scores and
// seeds with the current engine. Model decisions stay fixed, so a generator
// change can be compared on identical input. MAGICHANDY_REPLAY_INPUT names the
// captured report; MAGICHANDY_EXPERIMENT_CAPTURE receives the re-rendered one.
func TestReplayContinuousSession(t *testing.T) {
	input := strings.TrimSpace(os.Getenv("MAGICHANDY_REPLAY_INPUT"))
	if input == "" {
		t.Skip("set MAGICHANDY_REPLAY_INPUT to a captured session report")
	}
	encoded, err := os.ReadFile(input) // #nosec G304 -- developer-selected local evaluation report.
	if err != nil {
		t.Fatal(err)
	}
	var runs []continuousSessionRun
	if err := json.Unmarshal(encoded, &runs); err != nil {
		t.Fatal(err)
	}
	for r := range runs {
		// Reports before the limits were recorded used these harness values.
		settings := config.DefaultSettings().Motion
		settings.SpeedMinPercent, settings.SpeedMaxPercent = 15, 54
		settings.HandyModel = config.HandyModelOriginal
		if runs[r].Settings != nil {
			settings = *runs[r].Settings
		}
		var previous *motion.FlowSpec
		offset := int64(0)
		for i, turn := range runs[r].Turns {
			if turn.Kind != "motion" || turn.Score == nil {
				continue
			}
			if previous != nil && !continuousSameScore(*previous, *turn.Score) {
				offset = 0
			}
			plan := motion.NewMotionPlan("session-replay", motion.MotionTarget{Label: "Autopilot", Source: "autopilot",
				SpeedPercent: turn.Score.SpeedPercent, Flow: motion.CloneFlowSpec(turn.Score)}, settings, 0, 0, time.Unix(0, 0))
			summary := plan.Perceptual
			runs[r].Turns[i].Summary = &summary
			runs[r].Turns[i].Samples = nil
			for at := int64(0); at <= turn.DurationMS; at += 50 {
				runs[r].Turns[i].Samples = append(runs[r].Turns[i].Samples, [2]float64{float64(at) / 1000, plan.SampleAt(offset + at).PositionPercent})
			}
			offset += turn.DurationMS
			previous = turn.Score
		}
	}
	out, _ := json.MarshalIndent(runs, "", " ")
	if err := os.WriteFile(os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE"), out, 0o600); err != nil {
		t.Fatal(err)
	}
}

func logContinuousSessionSummary(t *testing.T, index int, run continuousSessionRun) {
	t.Helper()
	directions := map[string]int{}
	holds, requested, failures, motionTurns := 0, 0, 0, 0
	characters := map[string]bool{}
	var speeds []float64
	for _, turn := range run.Turns {
		if turn.Kind != "motion" {
			continue
		}
		motionTurns++
		if turn.Held {
			holds++
		}
		if turn.Requested {
			requested++
		}
		if turn.Error != "" {
			failures++
		}
		if turn.Score == nil {
			continue
		}
		characters[continuousCharacter(*turn.Score)] = true
		speeds = append(speeds, float64(turn.Score.SpeedPercent))
		if g := turn.Score.Gesture; g != nil {
			direction := g.FasterDirection
			if g.ContrastPercent == 0 {
				direction = "even"
			}
			directions[direction]++
		}
	}
	t.Logf("run=%d mode=%s persona=%s motion_turns=%d holds=%d requested_holds=%d errors=%d distinct_characters=%d directions=%v speed=%s",
		index, run.Mode, run.Persona, motionTurns, holds, requested, failures, len(characters), directions, liveSessionStats(speeds))
}

func continuousSameScore(a, b motion.FlowSpec) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

// continuousChangedGroups names what a decision changed, in either grammar.
func continuousChangedGroups(before, after motion.FlowSpec) []string {
	var changed []string
	add := func(name string, differs bool) {
		if differs {
			changed = append(changed, name)
		}
	}
	add("range", before.MinPercent != after.MinPercent || before.MaxPercent != after.MaxPercent)
	add("speed", before.SpeedPercent != after.SpeedPercent)
	if g, h := before.Gesture, after.Gesture; g != nil && h != nil {
		add("focus", g.FocusPercent != h.FocusPercent || g.FocusWidthPercent != h.FocusWidthPercent || g.FocusMixPercent != h.FocusMixPercent || g.FocusRoamPercent != h.FocusRoamPercent)
		add("sweep", g.FasterDirection != h.FasterDirection || g.ContrastPercent != h.ContrastPercent)
		add("rebounds", g.ReboundCount != h.ReboundCount || g.ReboundDecayPercent != h.ReboundDecayPercent)
		add("inertia", g.InertiaPercent != h.InertiaPercent)
		add("variation", g.VariationPercent != h.VariationPercent)
	} else {
		add("widths", before.RangeFloorPercent != after.RangeFloorPercent || before.RangeCeilingPercent != after.RangeCeilingPercent)
		add("anchor", before.AnchorPercent != after.AnchorPercent)
		add("pace_variation", before.PaceVariationPercent != after.PaceVariationPercent || before.VariationMode != after.VariationMode)
		layersBefore, _ := json.Marshal(before.Layers)
		layersAfter, _ := json.Marshal(after.Layers)
		add("layers", string(layersBefore) != string(layersAfter))
	}
	add("seed", before.Seed != after.Seed)
	return changed
}

func continuousCharacter(s motion.FlowSpec) string {
	if g := s.Gesture; g != nil {
		return fmt.Sprintf("focus=%d/w%d/mix%d/roam%d sweep=%s:%d inertia=%d rebounds=%d@%d var=%d",
			g.FocusPercent, g.FocusWidthPercent, g.FocusMixPercent, g.FocusRoamPercent, g.FasterDirection, g.ContrastPercent,
			g.InertiaPercent, g.ReboundCount, g.ReboundDecayPercent, g.VariationPercent)
	}
	var layers []string
	for _, layer := range s.Layers {
		layers = append(layers, fmt.Sprintf("%s:%s:%d/%d", layer.Axis, layer.Shape, layer.AmountPercent, layer.PeriodCycles))
	}
	return fmt.Sprintf("anchor=%d widths=%d..%d pace_var=%d mode=%s layers=[%s]", s.AnchorPercent, s.RangeFloorPercent,
		s.RangeCeilingPercent, s.PaceVariationPercent, s.VariationMode, strings.Join(layers, " "))
}

func liveSessionStats(values []float64) string {
	if len(values) == 0 {
		return "n/a"
	}
	lo, hi, sum := math.Inf(1), math.Inf(-1), 0.0
	for _, v := range values {
		lo, hi, sum = math.Min(lo, v), math.Max(hi, v), sum+v
	}
	return fmt.Sprintf("mean=%.0f[%.0f..%.0f]", sum/float64(len(values)), lo, hi)
}

func liveSessionLastReplies(replies []string, reply string) []string {
	replies = append(replies, reply)
	if len(replies) > 4 {
		replies = replies[len(replies)-4:]
	}
	return replies
}

func liveSessionStay(stay *bool) string {
	if stay == nil {
		return "-"
	}
	return strconv.FormatBool(*stay)
}

func liveSessionEnvInt(name string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && value > 0 {
		return value
	}
	return fallback
}

func liveSessionRequests(raw string) map[int]string {
	requests := map[int]string{}
	for _, item := range strings.Split(raw, "|") {
		turn, text, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		if index, err := strconv.Atoi(strings.TrimSpace(turn)); err == nil {
			requests[index] = strings.TrimSpace(text)
		}
	}
	return requests
}
