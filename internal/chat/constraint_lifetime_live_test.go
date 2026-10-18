//go:build liveeval

package chat

import (
	"math"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

type constraintEpisode struct {
	name         string
	lines        []string
	initialSpeed int
	band         bool
	keepSpeed    int
	exactHold    bool
	// An explicit timed request must last this long. Vague "brief" requests
	// have no invented deadline, and release is assessed over the episode.
	releaseAfter int
}

func constraintEpisodes() []constraintEpisode {
	return []constraintEpisode{
		{name: "open", initialSpeed: 25},
		{name: "one_time_speed", lines: []string{"Set the speed to 25 percent now."}, initialSpeed: 25},
		{name: "praise", lines: []string{"That feels good."}, initialSpeed: 25},
		{name: "brief_easing", lines: []string{"Give me a brief easy stretch at 15 percent."}, initialSpeed: 15},
		{name: "timed_easing", lines: []string{"Use 15 percent for the next minute, then continue developing the session."}, initialSpeed: 15, releaseAfter: 60},
		{name: "released_speed", lines: []string{"Keep the base speed at 25 percent until I say otherwise.", "You can take over again now, including changing the pace."}, initialSpeed: 25, releaseAfter: 0},
		{name: "lasting_speed", lines: []string{"Keep the base speed at 25 percent throughout this session. Develop the reach, location and texture naturally."}, initialSpeed: 25, keepSpeed: 25},
		{name: "lasting_band", lines: []string{"Keep all travel between 20 and 70 percent throughout this session. Develop the pace and texture naturally."}, initialSpeed: 25, band: true},
		{name: "exact_hold", lines: []string{"Keep this exact pattern and speed repeating until I tell you otherwise."}, initialSpeed: 25, exactHold: true},
	}
}

// The same model, schema, parser, initial score and clock are used for both
// conditions. Only the hosted planning paragraph differs: previous uses the
// prior hosted advice; prior restores the original pace-development paragraph.
// All output is compiled without an engine, transport or device connection.
func TestConstraintLifetimeLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, mode := range []MotionMode{MotionModeLayered, MotionModeCreativeV2} {
		if !constraintFilter("MODES", string(mode)) {
			continue
		}
		for _, episode := range constraintEpisodes() {
			if !constraintFilter("CASES", episode.name) {
				continue
			}
			initial := newConstraintSession(mode, episode)
			caps := Capabilities{Motion: true, MotionMode: mode, HostedMotion: true, PreserveConversationText: true}
			if os.Getenv("MAGICHANDY_CONSTRAINT_CHAT_SETUP") == "1" && !prepareConstraintChat(t, provider, model, &initial, caps, "shared_setup", &rows) {
				continue
			}
			for _, variant := range []string{"current", "previous", "prior"} {
				if !constraintFilter("VARIANTS", variant) {
					continue
				}
				t.Run(string(mode)+"/"+episode.name+"/"+variant, func(t *testing.T) {
					session := initial
					session.score = *motion.CloneFlowSpec(&initial.score)
					session.speeds, session.bands = slices.Clone(initial.speeds), slices.Clone(initial.bands)
					runConstraintEpisode(t, provider, model, session, variant, &rows)
				})
			}
		}
	}
}

func constraintFilter(key, value string) bool {
	selected := os.Getenv("MAGICHANDY_CONSTRAINT_" + key)
	if key == "VARIANTS" && selected == "" {
		selected = "current,previous"
	}
	return selected == "" || strings.Contains(","+selected+",", ","+value+",")
}

type constraintSession struct {
	score                                     motion.FlowSpec
	limits                                    config.MotionSettings
	mode                                      MotionMode
	episode                                   constraintEpisode
	history                                   []llm.Message
	historyAt                                 []time.Time
	userLines                                 []RecentUserRequest
	speeds                                    []constraintSpeedMark
	bands                                     []PositionBand
	speedSince, phraseSince, unchanged, holds int
}

type constraintSpeedMark struct {
	speed, at int
}

func newConstraintSession(mode MotionMode, episode constraintEpisode) constraintSession {
	s := constraintSession{mode: mode, episode: episode, limits: config.DefaultSettings().Motion, speedSince: -120, phraseSince: -120}
	s.limits.SpeedMinPercent, s.limits.SpeedMaxPercent = 10, 50
	s.score = DefaultLayeredScore(episode.initialSpeed)
	if mode == MotionModeCreativeV2 {
		s.score = FreshCreativeV2Score(episode.initialSpeed)
	}
	s.score.Seed = 841
	if episode.band {
		s.score.MinPercent, s.score.MaxPercent, s.score.RangeCeilingPercent = 20, 70, 50
	}
	s.speeds = []constraintSpeedMark{{speed: 25, at: -120}, {speed: 25, at: -60}}
	if episode.initialSpeed != 25 {
		// The requested easing has only just begun; do not tell the model it
		// has already spent two minutes at the newly requested speed.
		s.speedSince, s.phraseSince = 0, 0
		s.speeds = append(s.speeds, constraintSpeedMark{speed: episode.initialSpeed, at: 0})
	}
	lines := append([]string{"Autopilot is in charge. Let the motion develop naturally within my saved limits."}, episode.lines...)
	for i, text := range lines {
		at := time.Unix(int64(-60*(len(lines)-1-i)), 0)
		s.userLines = append(s.userLines, RecentUserRequest{Text: text, At: at})
		s.history = append(s.history, llm.Message{Role: "user", Content: text})
		s.historyAt = append(s.historyAt, at)
	}
	return s
}

func (s constraintSession) context(at int, hosted bool) (MotionContext, AutopilotContext) {
	now := time.Unix(int64(at), 0)
	texts, ages := UserRequestTimeline(SelectRecentUserRequestTimeline(s.userLines), now)
	state := MotionContext{Running: true, Autopilot: true, MotionMode: s.mode, Layered: motion.CloneFlowSpec(&s.score),
		SpeedPercent: s.score.SpeedPercent, SpeedMinPercent: 10, SpeedMaxPercent: 50,
		Envelope: motion.CurrentPlanningEnvelope(s.limits), UserRequests: texts, UserRequestSecondsAgo: ages}
	plan := motion.NewMotionPlan("constraint-review", motion.MotionTarget{Flow: &s.score, SpeedPercent: s.score.SpeedPercent}, s.limits, 0, 0, now)
	feel := plan.Perceptual
	context := AutopilotContext{Hosted: hosted, MotionMode: s.mode, CurrentFlow: state.Layered, CurrentSpeed: s.score.SpeedPercent,
		SpeedMinPercent: 10, SpeedMaxPercent: 50, MotionChangeLevel: 5, SessionTracking: true, SessionSeconds: at + 180,
		SecondsAtCurrentSpeed: at - s.speedSince, SecondsAtCurrentPhrase: at - s.phraseSince, DecisionsAtCurrentPhrase: s.unchanged,
		ConsecutiveHolds: s.holds, LastHumanSecondsAgo: &ages[len(ages)-1], RecentPositionBands: s.bands,
		CommandedMeanTravel: int(math.Round(feel.CommandedMeanTravelPerSecond)), CommandedPeakSpeed: int(math.Round(feel.CommandedPeakVelocityPerSecond)),
		CommandedPositionMin: int(math.Round(feel.PositionMinPercent)), CommandedPositionMax: int(math.Round(feel.PositionMaxPercent)),
		MeanStrokeLength: int(math.Round(feel.MeanStrokePercent)), LocalStrokeCV: int(math.Round(feel.MinimumLocalStrokeCV * 100)), LocalStrokeRange: int(math.Round(feel.MinimumLocalStrokeRange))}
	for _, step := range s.speeds {
		if at-step.at <= 180 {
			context.RecentSpeeds = append(context.RecentSpeeds, SpeedStep{SpeedPercent: step.speed, SecondsAgo: at - step.at})
		}
	}
	return state, context
}

func runConstraintEpisode(t *testing.T, provider *hostedLayeredCapture, model string, s constraintSession, variant string, rows *[]map[string]any) {
	t.Helper()
	mode, episode := s.mode, s.episode
	caps := Capabilities{Motion: true, MotionMode: mode, HostedMotion: true, PreserveConversationText: true}
	for index, at := range []int{15, 90, 180, 360} {
		state, facts := s.context(at, variant != "prior")
		planner := AutopilotService{Provider: provider, Model: model, Capabilities: caps, MotionContext: &state, ReasoningMode: "off", Temperature: 0.9}
		message := AutopilotMotionMessage(facts)
		if variant == "previous" {
			guide, err := os.ReadFile("testdata/constraint-lifetime-prior-hosted.txt")
			if err != nil {
				t.Fatal(err)
			}
			message = strings.Replace(message, hostedContinuousPlanningGuide, strings.ReplaceAll(string(guide), "\r\n", "\n"), 1)
		}
		request := Request{History: PlanningHistory(s.history, s.historyAt, time.Unix(int64(at), 0)), Message: message}
		started, calls := time.Now(), provider.calls
		decision, err := planner.Complete(t.Context(), AutopilotKindMotion, request)
		before := *motion.CloneFlowSpec(&s.score)
		if err == nil && decision.Motion != nil && decision.Motion.Layered != nil {
			s.score = *motion.CloneFlowSpec(decision.Motion.Layered)
		}
		pass := err == nil && provider.calls-calls == 1
		if episode.keepSpeed > 0 {
			pass = pass && s.score.SpeedPercent == episode.keepSpeed
		}
		if episode.band {
			pass = pass && s.score.MinPercent == 20 && s.score.MaxPercent == 70
		}
		if episode.exactHold {
			pass = pass && reflect.DeepEqual(before, s.score)
		}
		if episode.releaseAfter > 0 && at < episode.releaseAfter {
			pass = pass && s.score.SpeedPercent <= episode.initialSpeed+2
		}
		row := hostedLayeredLiveRow(string(mode)+"/"+episode.name+"/"+variant+"/"+strconv.Itoa(at), model, strings.Join(episode.lines, " | "), provider.raw, before, decision.Motion, s.limits, started, err, pass)
		row["episode"] = episode.name
		row["method"], row["variant"], row["turn"], row["request_age_seconds"] = string(mode), variant, index+1, at
		row["messages"], row["provider_calls"] = provider.request.Messages, provider.calls-calls
		_, compactBudget, compactErr := llm.BudgetChatRequest(provider.request, 32768)
		row["compact_context_budget"] = compactBudget
		if compactErr != nil {
			row["compact_context_error"] = compactErr.Error()
		}
		row["shape_changed"] = constraintShapeChanged(before, s.score)
		plan := motion.NewMotionPlan("constraint-review", motion.MotionTarget{Flow: &s.score, SpeedPercent: s.score.SpeedPercent}, s.limits, 0, 0, time.Unix(0, 0))
		row["compiled"] = plan.Perceptual
		*rows = append(*rows, row)
		saveHostedLayeredRows(t, *rows)
		t.Logf("age=%d speed=%d->%d shape=%t valid=%t pass=%t error=%v", at, before.SpeedPercent, s.score.SpeedPercent, row["shape_changed"], err == nil, pass, err)
		if !pass {
			t.Error("constraint or validity check failed; raw response retained")
		}
		s.record(before, at, plan.Perceptual)
	}
}

// An optional real interactive turn includes the model's actual acknowledgment
// in later history. This catches a planning prompt that behaves well only when
// assistant turns are absent. It still creates no engine or transport.
func prepareConstraintChat(t *testing.T, provider *hostedLayeredCapture, model string, session *constraintSession, caps Capabilities, variant string, rows *[]map[string]any) bool {
	t.Helper()
	state, _ := session.context(0, true)
	service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off"}
	index := len(session.history) - 1
	started, calls := time.Now(), provider.calls
	result, err := service.Complete(t.Context(), Request{History: session.history[:index], Message: session.history[index].Content}, nil)
	pass := err == nil && provider.calls-calls == 1 && result.Response.Reply != "" && !result.Repaired && !result.SemanticFallback && result.Response.StayUnchanged != nil && *result.Response.StayUnchanged == session.episode.exactHold
	row := hostedLayeredLiveRow(string(session.mode)+"/"+session.episode.name+"/"+variant+"/chat", model, session.history[index].Content, result.Raw, session.score, result.Response.Motion, session.limits, started, err, pass)
	row["method"], row["phase"], row["episode"], row["variant"] = string(session.mode), "chat", session.episode.name, variant
	row["stay_unchanged"], row["provider_calls"] = result.Response.StayUnchanged, provider.calls-calls
	*rows = append(*rows, row)
	if !pass {
		t.Errorf("initial chat failed or installed the wrong standing hold: %v", err)
		return false
	}
	if result.Response.Motion != nil && result.Response.Motion.Layered != nil {
		session.score = *motion.CloneFlowSpec(result.Response.Motion.Layered)
	}
	session.history = append(session.history, llm.Message{Role: "assistant", Content: result.Response.Reply})
	session.historyAt = append(session.historyAt, time.Unix(0, 0))
	// Production honors an exact standing hold without a planning request.
	return !session.episode.exactHold
}

func constraintShapeChanged(a, b motion.FlowSpec) bool {
	a.Seed, b.Seed, a.SpeedPercent, b.SpeedPercent = 0, 0, 0, 0
	return !reflect.DeepEqual(a, b)
}

func (s *constraintSession) record(before motion.FlowSpec, at int, feel motion.PerceptualSummary) {
	if before.SpeedPercent != s.score.SpeedPercent {
		s.speedSince = at
	}
	s.speeds = append(s.speeds, constraintSpeedMark{speed: s.score.SpeedPercent, at: at})
	if constraintShapeChanged(before, s.score) {
		s.phraseSince, s.unchanged = at, 0
	} else {
		s.unchanged++
	}
	if reflect.DeepEqual(before, s.score) {
		s.holds++
	} else {
		s.holds = 0
	}
	s.bands = append(s.bands, PositionBand{Minimum: int(math.Round(feel.PositionMinPercent)), Maximum: int(math.Round(feel.PositionMaxPercent))})
	if len(s.bands) > 6 {
		s.bands = s.bands[len(s.bands)-6:]
	}
}

// Interactive turns can accidentally freeze all subsequent planning by setting
// stay_unchanged. Probe that separately from individual motion dimensions.
func TestConstraintLifetimeChatLive(t *testing.T) {
	provider, model := hostedLayeredLiveProvider(t)
	rows := []map[string]any{}
	t.Cleanup(func() { saveHostedLayeredRows(t, rows) })
	for _, tc := range []struct {
		name, text     string
		held, wantHold bool
	}{
		{"praise", "That feels good.", false, false},
		{"continue", "Keep going.", false, false},
		{"one_time_speed", "Set the speed to 25 percent now.", false, false},
		{"brief_easing", "Give me a brief easy stretch at 15 percent.", false, false},
		{"lasting_pace", "Keep the base speed at 25 percent, but develop the rest naturally.", false, false},
		{"exact_hold", "Keep this exact pattern and speed repeating until I tell you otherwise.", false, true},
		{"release_hold", "You can take over again, including changing the pace.", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := DefaultLayeredScore(25)
			caps := Capabilities{Motion: true, MotionMode: MotionModeLayered, HostedMotion: true, PreserveConversationText: true}
			state := MotionContext{Running: true, Autopilot: true, StandingHold: tc.held, MotionMode: MotionModeLayered, Layered: &before, SpeedMinPercent: 10, SpeedMaxPercent: 50}
			service := Service{Provider: provider, Model: model, Capabilities: &caps, MotionContext: &state, ReasoningMode: "off"}
			started := time.Now()
			result, err := service.Complete(t.Context(), Request{Message: tc.text}, nil)
			pass := err == nil && result.Response.StayUnchanged != nil && *result.Response.StayUnchanged == tc.wantHold && !result.Repaired && !result.SemanticFallback
			row := hostedLayeredLiveRow("chat_"+tc.name, model, tc.text, result.Raw, before, result.Response.Motion, config.DefaultSettings().Motion, started, err, pass)
			row["stay_unchanged"], row["expected_hold"] = result.Response.StayUnchanged, tc.wantHold
			rows = append(rows, row)
			t.Logf("pass=%t hold=%v err=%v", pass, result.Response.StayUnchanged, err)
			if !pass {
				t.Error("unexpected future Autopilot hold; see capture")
			}
		})
	}
}
