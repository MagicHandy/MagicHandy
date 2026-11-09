//go:build liveeval

package chat

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Synthetic sessions for the follow-through review: a build requested now with
// a later cue, the cue itself, invented strokes that must not ping-pong between
// two recent scores, and an ordered widening progression. Autopilot turns run
// with simulated time at change rate 8 (8-16 s reference stretches), as the
// scheduler would. No transport or engine runs; every planning and chat result
// is retained for motion-atlas review. MAGICHANDY_EVAL_REPEATS repeats each case.
func TestHostedFollowThroughLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	keep := func(scenario []map[string]any, pass bool) {
		for _, row := range scenario {
			row["intent_pass"] = pass && row["valid"] == true
		}
		rows = append(rows, scenario...)
		saveHostedLayeredRows(t, rows)
	}
	build := "Build them toward the edge now. I'll tell you when they're nearly there, and then ease right off."
	for repeat := 1; repeat <= followthroughRepeats(); repeat++ {
		t.Run(fmt.Sprintf("cued_build_chat_%d", repeat), func(t *testing.T) {
			before := followthroughSlowScore()
			state := MotionContext{Running: true, Autopilot: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: before.SpeedPercent,
				SpeedMinPercent: 1, SpeedMaxPercent: 100}
			started := time.Now()
			result, after, err := followthroughChat(t, provider, model, state, nil, build)
			// The cue is for later; building toward the edge starts now, mostly
			// through pace. Edge describes arousal, not the tip of the range.
			pass := err == nil && after.SpeedPercent >= before.SpeedPercent+10
			t.Logf("speed %d->%d pass=%t reply=%q err=%v", before.SpeedPercent, after.SpeedPercent, pass, result.Response.Reply, err)
			keep([]map[string]any{hostedLayeredLiveRow("cued_build_chat", model, build, result.Raw, before, result.Response.Motion, followthroughSettings(), started, err, pass)}, pass)
			if !pass {
				t.Error("the requested build did not start in this turn")
			}
		})
		t.Run(fmt.Sprintf("cued_build_autopilot_%d", repeat), func(t *testing.T) {
			session := newFollowthroughSession(followthroughSlowScore(), 55)
			session.ask(build, "Building toward the edge gradually. Tell me when they're nearly there.", 6)
			scores, scenario, valid := followthroughPlanner(t, provider, model, "cued_build_autopilot", session, 5)
			drops := 0
			previous := followthroughSlowScore().SpeedPercent
			for _, score := range scores {
				if score.SpeedPercent < previous {
					drops++
				}
				previous = score.SpeedPercent
			}
			last := scores[len(scores)-1].SpeedPercent
			pass := valid && len(scores) == 5 && last >= followthroughSlowScore().SpeedPercent+25 && drops <= 1
			t.Logf("trajectory=%s drops=%d pass=%t", followthroughSpeeds(scores), drops, pass)
			keep(scenario, pass)
			if !pass {
				t.Error("Autopilot did not keep building toward the named destination")
			}
		})
		t.Run(fmt.Sprintf("cue_arrives_%d", repeat), func(t *testing.T) {
			slow := followthroughSlowScore()
			slow.SpeedPercent = 18
			session := newFollowthroughSession(slow, 5)
			session.speeds = []SpeedStep{{SpeedPercent: 60, SecondsAgo: 90, Interactive: true}, {SpeedPercent: 18, SecondsAgo: 5, Interactive: true}}
			session.ask(build, "Building toward the edge.", 120)
			session.ask("They're nearly there. Ease right off.", "Easing right off.", 6)
			scores, scenario, valid := followthroughPlanner(t, provider, model, "cue_arrives", session, 4)
			pass := valid && len(scores) == 4
			for _, score := range scores[:min(3, len(scores))] {
				pass = pass && score.SpeedPercent <= 30
			}
			t.Logf("trajectory=%s pass=%t", followthroughSpeeds(scores), pass)
			keep(scenario, pass)
			if !pass {
				t.Error("Autopilot rebuilt right after the cue to ease off")
			}
		})
		t.Run(fmt.Sprintf("invented_strokes_%d", repeat), func(t *testing.T) {
			session := newFollowthroughSession(followthroughFastScore(), 20)
			session.speeds = []SpeedStep{{SpeedPercent: 100, SecondsAgo: 20, Interactive: true}}
			session.ask("Keep coming up with new strokes on your own. Nothing fancy, and stay at top speed.",
				"I'll keep inventing plain strokes at top speed.", 8)
			scores, scenario, valid := followthroughPlanner(t, provider, model, "invented_strokes", session, 6)
			distinct, returns, slowest := map[string]bool{}, 0, 100
			geometries := []string{autopilotGeometry(followthroughFastScore())}
			for _, score := range scores {
				geometry := autopilotGeometry(score)
				distinct[geometry] = true
				// A return to the score two stretches back is the A/B ping-pong.
				if n := len(geometries); n >= 2 && geometry == geometries[n-2] && geometry != geometries[n-1] {
					returns++
				}
				geometries = append(geometries, geometry)
				slowest = min(slowest, score.SpeedPercent)
			}
			pass := valid && len(scores) == 6 && len(distinct) >= 4 && returns <= 1 && slowest >= 90
			t.Logf("distinct=%d returns=%d slowest=%d pass=%t", len(distinct), returns, slowest, pass)
			keep(scenario, pass)
			if !pass {
				t.Error("invented strokes repeated recent scores or left the requested pace")
			}
		})
		t.Run(fmt.Sprintf("widening_steps_%d", repeat), func(t *testing.T) {
			before := followthroughSlowScore()
			before.SpeedPercent = 18
			state := MotionContext{Running: true, Autopilot: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: before.SpeedPercent,
				SpeedMinPercent: 1, SpeedMaxPercent: 100}
			message := "Begin with short strokes around the middle and make each stroke a little wider than the one before."
			started := time.Now()
			result, after, err := followthroughChat(t, provider, model, state, nil, message)
			pass := err == nil && widensFromMiddle(after.Phrase)
			t.Logf("phrase=%+v pass=%t err=%v", after.Phrase, pass, err)
			keep([]map[string]any{hostedLayeredLiveRow("widening_steps", model, message, result.Raw, before, result.Response.Motion, followthroughSettings(), started, err, pass)}, pass)
			if !pass {
				t.Error("the ordered progression was not counted step by step")
			}
		})
	}
}

// widensFromMiddle reports at least three counted groups, the first centered
// near the middle, each wider than the one before.
func widensFromMiddle(phrase *motion.FlowPhrase) bool {
	if phrase == nil {
		return false
	}
	groups := phrase.Strokes
	if len(groups) == 0 {
		for _, region := range phrase.Regions {
			groups = append(groups, region.PhraseStroke)
		}
	}
	if len(groups) < 3 {
		return false
	}
	if center := (groups[0].BottomPercent + groups[0].TopPercent) / 2; center < 35 || center > 65 {
		return false
	}
	for i := 1; i < len(groups); i++ {
		if groups[i].TopPercent-groups[i].BottomPercent <= groups[i-1].TopPercent-groups[i-1].BottomPercent {
			return false
		}
	}
	return true
}

func followthroughRepeats() int {
	repeats, _ := strconv.Atoi(os.Getenv("MAGICHANDY_EVAL_REPEATS"))
	return max(1, repeats)
}

func followthroughCaps() Capabilities {
	return Capabilities{Motion: true, MotionMode: MotionModeLayered, AdvancedMotion: true, PreserveConversationText: true,
		Voice: VoiceUtility, ReplyLength: ReplyLengthBalanced, HistoryMessages: 256, HistoryBytes: 96000}
}

// followthroughSettings are full speed and stroke limits on a current Handy.
func followthroughSettings() config.MotionSettings {
	settings := config.DefaultSettings().Motion
	settings.SpeedMinPercent, settings.SpeedMaxPercent = 1, 100
	return settings
}

// followthroughSlowScore is a slow, wandering score with no counted phrase.
func followthroughSlowScore() motion.FlowSpec {
	return motion.FlowSpec{MinPercent: 0, MaxPercent: 100, SpeedPercent: 20, RangeFloorPercent: 10, RangeCeilingPercent: 60,
		AnchorPercent: 50, MemoryCycles: 10, PaceVariationPercent: 10, VariationMode: "waves", Seed: 901,
		Layers: []motion.FlowLayer{
			{Axis: "range", AmountPercent: 100, PeriodCycles: 6, Shape: "alternate"},
			{Axis: "center", AmountPercent: 70, PeriodCycles: 14, Shape: "wave"},
		}}
}

// followthroughFastScore is a simple counted motif at top speed.
func followthroughFastScore() motion.FlowSpec {
	return motion.FlowSpec{MinPercent: 0, MaxPercent: 100, SpeedPercent: 100, RangeFloorPercent: 20, RangeCeilingPercent: 100,
		AnchorPercent: 100, MemoryCycles: 12, VariationMode: "waves", Seed: 902,
		Phrase: &motion.FlowPhrase{Strokes: []motion.PhraseStroke{{BottomPercent: 80, TopPercent: 100, Count: 2, RatePercent: 100}, {BottomPercent: 0, TopPercent: 100, Count: 1, RatePercent: 100}}}}
}

func followthroughChat(t *testing.T, provider *hostedLayeredCapture, model string, state MotionContext, history []llm.Message, message string) (Result, motion.FlowSpec, error) {
	caps := followthroughCaps()
	state = withFollowthroughFacts(t, state)
	service := Service{Provider: provider, Model: model, ReasoningMode: "off", Capabilities: &caps, MotionContext: &state,
		MaxTokens: 1024, PromptBudget: PromptBudgetSettings{ContextSize: 131072, MaxOutputTokens: 1024}}
	result, err := service.Complete(t.Context(), Request{Message: message, History: history}, nil)
	after := *motion.CloneFlowSpec(state.Layered)
	if command := result.Response.Motion; command != nil && command.Layered != nil {
		after = *motion.CloneFlowSpec(command.Layered)
	}
	return result, after, err
}

type followthroughRequest struct {
	text string
	at   time.Time
}

type followthroughPlayed struct {
	flow         motion.FlowSpec
	since, until time.Time
}

// followthroughSession mirrors the scheduler state Autopilot sees: the
// person's requests, recent speeds and check intervals, and the distinct
// scores played before the current one.
type followthroughSession struct {
	history                    []llm.Message
	requests                   []followthroughRequest
	start, now, since, speedAt time.Time
	current                    motion.FlowSpec
	speeds                     []SpeedStep
	checks                     []int
	played                     []followthroughPlayed
	segment                    int
}

func newFollowthroughSession(current motion.FlowSpec, atSpeedSeconds int) *followthroughSession {
	start := time.Unix(1_800_000_000, 0)
	now := start.Add(20 * time.Minute)
	return &followthroughSession{start: start, now: now, since: now.Add(-10 * time.Second),
		speedAt: now.Add(-time.Duration(atSpeedSeconds) * time.Second), current: current, segment: 60}
}

// ask records a request made secondsAgo and the chat reply that answered it.
func (s *followthroughSession) ask(request, reply string, secondsAgo int) {
	s.requests = append(s.requests, followthroughRequest{text: request, at: s.now.Add(-time.Duration(secondsAgo) * time.Second)})
	s.history = append(s.history, llm.Message{Role: "user", Content: request}, llm.Message{Role: "assistant", Content: reply})
}

// recentScores mirrors recentPlanningScores over the simulated history.
func (s *followthroughSession) recentScores() []EarlierScore {
	out := []EarlierScore{}
	for i := len(s.played) - 1; i >= 0 && len(out) < 3; i-- {
		mark := s.played[i]
		started := int(s.now.Sub(mark.since).Seconds())
		if int(s.now.Sub(mark.until).Seconds()) > 180 {
			break
		}
		if autopilotGeometry(mark.flow) == autopilotGeometry(s.current) {
			continue
		}
		duplicate := false
		for _, kept := range out {
			duplicate = duplicate || autopilotGeometry(kept.Flow) == autopilotGeometry(mark.flow)
		}
		if !duplicate {
			out = append(out, EarlierScore{Flow: mark.flow, StartedSecondsAgo: started, PlayedSeconds: int(mark.until.Sub(mark.since).Seconds())})
		}
	}
	return out
}

// followthroughPlanner runs consecutive Autopilot motion turns, advancing
// simulated time by each turn's own horizon.
func followthroughPlanner(t *testing.T, provider *hostedLayeredCapture, model, name string, s *followthroughSession, turns int) ([]motion.FlowSpec, []map[string]any, bool) {
	caps := followthroughCaps()
	caps.AutoTiming = true
	scores, rows := []motion.FlowSpec{}, []map[string]any{}
	valid := true
	for turn := 0; turn < turns; turn++ {
		requests, ages := []string{}, []int{}
		for _, request := range s.requests {
			requests, ages = append(requests, request.text), append(ages, int(s.now.Sub(request.at).Seconds()))
		}
		lastHuman := ages[len(ages)-1]
		before := *motion.CloneFlowSpec(&s.current)
		state := withFollowthroughFacts(t, MotionContext{Running: true, MotionMode: MotionModeLayered, Layered: &before, SpeedPercent: before.SpeedPercent,
			SpeedMinPercent: 1, SpeedMaxPercent: 100, UserRequests: requests, UserRequestSecondsAgo: ages})
		context := withFollowthroughFeel(AutopilotContext{Hosted: true, Style: "balanced", SegmentIndex: s.segment, MotionMode: MotionModeLayered,
			CurrentFlow: &before, CurrentSpeed: before.SpeedPercent, SpeedMinPercent: 1, SpeedMaxPercent: 100,
			MotionMinSeconds: 8, MotionMaxSeconds: 16, MotionChangeLevel: 8, AutoTiming: true,
			SessionTracking: true, SessionSeconds: int(s.now.Sub(s.start).Seconds()),
			SecondsAtCurrentSpeed: int(s.now.Sub(s.speedAt).Seconds()), LastHumanSecondsAgo: &lastHuman,
			RecentSpeeds: append([]SpeedStep(nil), s.speeds...), RecentCheckSeconds: append([]int(nil), s.checks...),
			RecentScores: s.recentScores()}, state.CommandedSummary)
		service := AutopilotService{Provider: provider, Model: model, Capabilities: caps, MotionContext: &state, MaxTokens: 1024,
			ReasoningMode: "off", Temperature: 0.81, PromptBudget: PromptBudgetSettings{ContextSize: 131072, MaxOutputTokens: 1024}}
		started := time.Now()
		response, err := service.Complete(t.Context(), AutopilotKindMotion, Request{Message: AutopilotMotionMessage(context), History: s.history})
		valid = valid && err == nil
		rows = append(rows, hostedLayeredLiveRow(fmt.Sprintf("%s_%d", name, turn+1), model, requests[len(requests)-1], provider.raw, before, response.Motion, followthroughSettings(), started, err, false))
		after := before
		if err == nil && response.Motion != nil && response.Motion.Layered != nil {
			after = *motion.CloneFlowSpec(response.Motion.Layered)
		}
		t.Logf("turn %d next=%d speed %d->%d err=%v", turn+1, response.NextCheckSeconds, before.SpeedPercent, after.SpeedPercent, err)
		if after.SpeedPercent != before.SpeedPercent {
			s.speedAt = s.now
		}
		if autopilotGeometry(after) != autopilotGeometry(before) {
			s.played = append(s.played, followthroughPlayed{flow: before, since: s.since, until: s.now})
			s.since = s.now
		}
		s.current = after
		scores = append(scores, after)
		horizon := min(max(response.NextCheckSeconds, config.AutopilotAutoTimingMinSeconds), config.AutopilotAutoTimingMaxSeconds)
		s.speeds = append(s.speeds, SpeedStep{SpeedPercent: after.SpeedPercent})
		for i := range s.speeds {
			s.speeds[i].SecondsAgo += horizon
		}
		s.speeds = s.speeds[max(0, len(s.speeds)-6):]
		s.checks = append(s.checks, horizon)
		s.checks = s.checks[max(0, len(s.checks)-6):]
		s.now = s.now.Add(time.Duration(horizon) * time.Second)
		s.segment++
	}
	return scores, rows, valid
}

// Failed calls leave the last score playing; that is not a successful model
// choice to hold it, even if the unchanged speed fits the requested easing.
func TestFollowthroughPlannerRejectsFailedHolds(t *testing.T) {
	slow := followthroughSlowScore()
	slow.SpeedPercent = 18
	session := newFollowthroughSession(slow, 5)
	session.ask("Ease right off.", "Easing off.", 6)
	provider := &hostedLayeredCapture{Provider: &scriptedProvider{
		responses: []string{"", ""}, responseErrors: []error{errors.New("provider unavailable"), errors.New("provider unavailable")},
	}}
	scores, rows, valid := followthroughPlanner(t, provider, "scripted", "failed_hold", session, 2)
	if valid || len(rows) != 2 || len(scores) != 2 {
		t.Fatalf("failed hold evaluation: valid=%t rows=%d scores=%d", valid, len(rows), len(scores))
	}
	for i, row := range rows {
		if row["valid"] != false || row["error"] == nil || row["intent_pass"] != false || scores[i].SpeedPercent != 18 {
			t.Fatalf("failure %d was not retained as a failed unchanged score: %+v", i, row)
		}
	}
}

func followthroughSpeeds(scores []motion.FlowSpec) string {
	out := ""
	for i, score := range scores {
		if i > 0 {
			out += " -> "
		}
		out += strconv.Itoa(score.SpeedPercent)
	}
	return out
}

// withFollowthroughFacts adds the compiled engine facts production sends.
func withFollowthroughFacts(t *testing.T, state MotionContext) MotionContext {
	if state.Layered == nil {
		return state
	}
	settings := followthroughSettings()
	state.Envelope = motion.CurrentPlanningEnvelope(settings)
	target, err := motion.FlowTarget(*state.Layered, settings)
	if err != nil {
		t.Logf("compile for facts failed: %v", err)
		return state
	}
	summary := motion.NewMotionPlan("followthrough", target, settings, 0, 0, time.Unix(0, 0)).Perceptual
	state.CommandedSummary = &summary
	return state
}

func withFollowthroughFeel(context AutopilotContext, summary *motion.PerceptualSummary) AutopilotContext {
	if summary == nil {
		return context
	}
	round := func(v float64) int { return int(v + 0.5) }
	context.CommandedPositionMin, context.CommandedPositionMax = round(summary.PositionMinPercent), round(summary.PositionMaxPercent)
	context.CommandedMeanTravel, context.CommandedPeakSpeed = round(summary.CommandedMeanTravelPerSecond), round(summary.CommandedPeakVelocityPerSecond)
	context.MeanStrokeLength = round(summary.MeanStrokePercent)
	context.LocalStrokeCV, context.LocalStrokeRange = round(summary.MinimumLocalStrokeCV*100), round(summary.MinimumLocalStrokeRange)
	return context
}
