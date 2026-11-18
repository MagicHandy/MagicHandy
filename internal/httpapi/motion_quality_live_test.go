//go:build liveeval && magichandy_labs

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
	"github.com/mapledaemon/MagicHandy/internal/motion"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

type qualityAttempt struct {
	Raw               string          `json:"raw"`
	History           int             `json:"history_messages"`
	InputBytes        int             `json:"input_bytes"`
	InputLimitBytes   int             `json:"input_limit_bytes"`
	PriorOmissions    int             `json:"omitted_before_provider_budget"`
	MaxTokens         int             `json:"request_max_tokens"`
	Dropped           int             `json:"history_dropped"`
	CorrectionPresent bool            `json:"correction_present"`
	Sampling          qualitySampling `json:"requested_sampling"`
}

type qualitySampling struct {
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	RepeatPenalty float64 `json:"repeat_penalty"`
	RepeatLastN   int     `json:"repeat_last_n"`
}

type qualityProvider struct {
	llm.Provider
	window     int
	correction string
	attempts   []qualityAttempt
}

func (p *qualityProvider) StreamChat(ctx context.Context, req llm.ChatRequest, delta func(string) error) (string, error) {
	bounded, budget, admissionErr := llm.BudgetChatRequest(req, p.window)
	review := qualityAttempt{InputBytes: budget.InputBytes, InputLimitBytes: budget.LimitBytes,
		PriorOmissions: req.HistoryMessagesDropped, Dropped: budget.HistoryMessagesDropped, MaxTokens: req.MaxTokens}
	review.Sampling = qualitySampling{Temperature: req.Temperature, TopP: req.TopP, RepeatPenalty: req.RepeatPenalty, RepeatLastN: req.RepeatLastN}
	if admissionErr == nil {
		for i, m := range bounded.Messages {
			if i < len(bounded.Messages)-1 && (m.Role == "user" || m.Role == "assistant") {
				review.History++
			}
			if p.correction != "" && m.Role == "user" && m.Content == p.correction {
				review.CorrectionPresent = true
			}
		}
	}
	raw, err := p.Provider.StreamChat(ctx, req, delta)
	review.Raw = raw
	p.attempts = append(p.attempts, review)
	return raw, err
}

type qualityEvents struct {
	Action      string                  `json:"action"`
	OK          bool                    `json:"ok"`
	Reply       string                  `json:"reply"`
	Repaired    bool                    `json:"repaired"`
	Fallback    bool                    `json:"fallback"`
	Diagnostics chat.MessageDiagnostics `json:"diagnostics"`
}

func qualityChat(t *testing.T, s *Server, message string) (qualityEvents, string) {
	t.Helper()
	data, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		t.Fatal(err)
	}
	req := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(string(data))))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(stopSequenceHeader, fmt.Sprint(s.stopSequence.Load()))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	stream := rec.Body.String()
	out := qualityEvents{Action: "none"}
	for _, block := range strings.Split(strings.ReplaceAll(stream, "\r\n", "\n"), "\n\n") {
		var name, body string
		for _, line := range strings.Split(block, "\n") {
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				name = v
			}
			if v, ok := strings.CutPrefix(line, "data: "); ok {
				body = v
			}
		}
		switch name {
		case "message":
			var msg struct {
				Reply       string
				Diagnostics chat.MessageDiagnostics
			}
			if err := json.Unmarshal([]byte(body), &msg); err != nil {
				t.Fatal(err)
			}
			out.Reply, out.Diagnostics = msg.Reply, msg.Diagnostics
		case "motion":
			var dispatch struct {
				Action  string
				Applied bool
			}
			if err := json.Unmarshal([]byte(body), &dispatch); err != nil {
				t.Fatal(err)
			}
			if dispatch.Applied {
				out.Action = dispatch.Action
			}
		case "done":
			var done struct {
				OK, Repaired bool
				Fallback     bool `json:"semantic_fallback"`
			}
			if err := json.Unmarshal([]byte(body), &done); err != nil {
				t.Fatal(err)
			}
			out.OK, out.Repaired, out.Fallback = done.OK, done.Repaired, done.Fallback
		}
	}
	return out, stream
}

type qualityRow struct {
	Scenario   string           `json:"scenario"`
	ID         string           `json:"id"`
	Request    string           `json:"request"`
	Response   string           `json:"response"`
	ElapsedMS  int64            `json:"elapsed_ms"`
	Expected   string           `json:"expected_action"`
	Events     qualityEvents    `json:"events"`
	Attempts   []qualityAttempt `json:"attempts"`
	Before     *motion.FlowSpec `json:"before,omitempty"`
	After      *motion.FlowSpec `json:"after,omitempty"`
	Motion     *motion.Review   `json:"motion,omitempty"`
	Failures   []string         `json:"failures,omitempty"`
	IntentPass bool             `json:"intent_pass"`
}

func qualityState(s *Server) motion.ActiveMotionState {
	if engine := s.currentMotionEngine(); engine != nil {
		return engine.Snapshot()
	}
	return motion.ActiveMotionState{}
}

func qualityRunTurn(t *testing.T, s *Server, p *qualityProvider, scenario string, tc qualityTurn) qualityRow {
	t.Helper()
	before := qualityState(s)
	transportBefore := s.transport.Diagnostics().CommandCount
	start := time.Now()
	p.attempts = nil
	events, stream := qualityChat(t, s, tc.request)
	elapsed := time.Since(start)
	after := qualityState(s)
	row := qualityRow{Scenario: scenario, ID: tc.id, Request: tc.request, Response: stream, ElapsedMS: elapsed.Milliseconds(),
		Expected: tc.action, Events: events, Attempts: p.attempts, Before: before.Target.Flow, After: after.Target.Flow}
	if after.Running && after.Target.Flow != nil {
		settings, _ := s.store.Snapshot()
		render := motion.ReviewMotionOutput(after.Target, settings.EffectiveMotion())
		row.Motion = &render
		if render.Error != "" {
			row.Failures = append(row.Failures, "shared compiler or sampler")
		}
	}
	calls := 1
	if tc.action == "stop" {
		calls = 0
	}
	if !events.OK || strings.TrimSpace(events.Reply) == "" || events.Repaired || events.Fallback || events.Diagnostics.RefusalFallbackFrom != "" || len(row.Attempts) != calls {
		row.Failures = append(row.Failures, "completion or provider calls")
	}
	if events.Action != tc.action {
		row.Failures = append(row.Failures, "action")
	}
	if failure := qualityPlaybackFailure(tc.action, before, after, transportBefore, s.transport.Diagnostics()); failure != "" {
		row.Failures = append(row.Failures, failure)
	}
	if tc.check != nil && !tc.check(row.Before, row.After, row.Motion) {
		row.Failures = append(row.Failures, "requested trajectory or preservation")
	}
	row.IntentPass = len(row.Failures) == 0
	return row
}

// Seed only authored, inert dialogue between measured requests. The correction,
// competing motion, delayed reference and subsequent edits go through real
// inference and the same HTTP handler as interactive chat.
func qualityFillHistory(t *testing.T, s *Server, pairs int) {
	t.Helper()
	objects := []string{"trail map", "thermos", "notebook", "folding stool", "lantern", "field guide", "rain jacket"}
	places := []string{"north shelf", "window bench", "supply table", "hall cabinet", "rear cupboard", "blue crate", "west drawer"}
	for index := range pairs {
		object, place := objects[index%len(objects)], places[(index/len(objects)+index)%len(places)]
		question := fmt.Sprintf("Field log entry %d. Where did we leave the %s during the observatory visit?", index+1, object)
		answer := fmt.Sprintf("For entry %d the %s was on the %s. The volunteer noted it before the evening walk.", index+1, object, place)
		if pairs > 40 {
			question += " We are keeping an inventory floor by floor so the next volunteer can retrace the route."
			answer += " The name and location went into the field log alongside sketches of the garden and the old clock tower."
		}
		if _, err := s.chatLog.Append(chat.MessageRoleUser, question, "quality-fixture"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.chatLog.Append(chat.MessageRoleAssistant, answer, ""); err != nil {
			t.Fatal(err)
		}
	}
}

// Use -run TestLiveMotionRequestSequence/<scenario> to select a sequence.
// MAGICHANDY_QUALITY_MIRROR=1 selects the alternate vocabulary and numerical
// targets. Each run writes its own ignored capture. Continue past wrong replies
// so that later recovery and all accepted geometry can be reviewed.
func TestLiveMotionRequestSequence(t *testing.T) {
	model := os.Getenv("MAGICHANDY_LIVE_MODEL")
	profileName := os.Getenv("MAGICHANDY_QUALITY_PROFILE")
	if (model == "" && profileName == "") || os.Getenv("MAGICHANDY_EXPERIMENT_CAPTURE") == "" {
		t.Skip("select a default profile or a live model and a private capture path")
	}
	hosted := os.Getenv("MAGICHANDY_HOSTED_DATA_DIR") != ""
	mirror := os.Getenv("MAGICHANDY_QUALITY_MIRROR") == "1"
	rows := []qualityRow{}
	commands := map[string][]transport.Command{}
	profiles := map[string]any{}
	providerName := "ollama"
	if hosted {
		providerName = "chatgpt"
	}
	var fixed *qualityDefaultProfile
	if profileName != "" {
		fixed = loadQualityDefault(t, profileName)
		active := fixed.settings.ConversationSettings()
		model, providerName, hosted = active.Model, active.Provider, active.IsHosted()
	}
	defer func() {
		exportLabExperimentCapture(t, map[string]any{"schema": "motion_quality.v1", "model": model, "provider": providerName, "profiles": profiles, "mirrored": mirror, "turns": rows, "session_commands": commands})
	}()
	starts := qualityIndependentStarts()
	scenarios := []string{"creative", "layered", "recall_recent", "recall_long", "holdout", "counted", "recall_readback", "recall_outside", "long_unrelated", "acknowledge_idle", "restated"}
	for _, scenario := range append(scenarios, slices.Sorted(maps.Keys(starts))...) {
		t.Run(scenario, func(t *testing.T) {
			mode, pairs, window, correction, cases := qualityScenario(t, scenario, hosted, mirror, starts)
			if fixed != nil {
				if scenario == "counted" && !fixed.settings.ConversationSettings().AdvancedMotion() {
					const reason = "an exact counted phrase requires the Advanced motion vocabulary"
					profiles[scenario] = map[string]any{"profile": profileName, "mode": mode, "capability": fixed.settings.ConversationSettings().ModelCapabilityLevel(), "context_size": fixed.window, "passed": 0, "turns": 0, "skipped": reason}
					t.Skip(reason)
				}
				window = fixed.window
				pairs = fixed.historyPairs(scenario, pairs)
			}
			s, p, fake := qualityServer(t, model, mode, correction, window, hosted, fixed)
			passes := 0
			for _, tc := range cases {
				row := qualityMemoryTurn(t, s, p, scenario, tc, pairs, mirror, fixed)
				rows = append(rows, row)
				if row.IntentPass {
					passes++
				} else {
					t.Errorf("%s: %s", tc.id, strings.Join(row.Failures, ", "))
				}
			}
			commands[scenario] = fake.Commands()
			run := map[string]any{"mode": mode, "context_size": window, "filler_pairs": pairs, "passed": passes, "turns": len(cases)}
			if fixed != nil {
				active := fixed.settings.ConversationSettings()
				run["profile"], run["capability"], run["history_limit"] = profileName, active.ModelCapabilityLevel(), fixed.messages
				if hosted {
					run["reasoning_effort"] = active.ActiveConnection.ReasoningEffort
				} else {
					run["max_output_tokens"], run["gguf_sha256"], run["template_fix"] = active.MaxOutputTokens, fixed.digest, fixed.template
				}
			}
			profiles[scenario] = run
			t.Logf("%d/%d exact first-pass outcomes", passes, len(cases))
		})
	}
}

func qualityPlaybackFailure(action string, before, after motion.ActiveMotionState, transportBefore int, owner transport.TransportDiagnostics) string {
	switch action {
	case "none":
		if before.Running != after.Running || before.PlanID != after.PlanID || (!before.Running && owner.CommandCount != transportBefore) {
			return "unexpected change to playback"
		}
	case "start", "update":
		if !after.Running || after.Target.Flow == nil || (action == "update" && (before.StreamID != after.StreamID || before.PlanID == after.PlanID)) {
			return "engine admission or stream continuity"
		}
	case "stop":
		if after.Running || owner.CommandCount <= transportBefore || owner.LastResult == nil || !owner.LastResult.ConfirmsStop() {
			return "Stop acknowledgement"
		}
	}
	return ""
}

func qualityScenario(t *testing.T, scenario string, hosted, mirror bool, starts map[string]qualityTurn) (string, int, int, string, []qualityTurn) {
	t.Helper()
	mode := config.LLMMotionModeLayered
	var pairs, window int
	var correction string
	cases := qualityLayeredCases(mirror)
	if hosted {
		window = config.MinimumExternalContext
	}
	if tc, ok := starts[scenario]; ok {
		cases = []qualityTurn{tc, {"stop", "Stop.", "stop", nil}}
	}
	switch scenario {
	case "restated":
		cases, correction = qualityRestatedCases(mirror)
		pairs = 112
	case "acknowledge_idle":
		cases = qualityAcknowledgementCases(mirror)
	case "holdout":
		cases = qualityHoldoutCases(mirror)
	case "counted":
		window = config.MinimumExternalContext
		cases = qualityPhraseCases(mirror)
	case "creative":
		mode = config.LLMMotionModeCreativeV2
		cases = qualityCreativeCases(mirror)
	case "recall_recent", "recall_long", "recall_readback", "recall_outside", "long_unrelated":
		cases, correction = qualityRecallCases(mirror)
		pairs = 12
		if scenario != "recall_recent" {
			pairs = 40
		}
		if scenario == "recall_long" || scenario == "recall_readback" || scenario == "long_unrelated" {
			window = 131072
			if n := os.Getenv("MAGICHANDY_QUALITY_HISTORY_PAIRS"); n != "" {
				length, err := strconv.Atoi(n)
				if err != nil || length < 1 || length > 500 {
					t.Fatal("history pairs must be between 1 and 500")
				}
				pairs = length
			}
		}
	}
	return mode, pairs, window, correction, cases
}

func qualityServer(t *testing.T, model, mode, correction string, window int, hosted bool, fixed *qualityDefaultProfile) (*Server, *qualityProvider, *transport.Fake) {
	t.Helper()
	var native llm.Provider
	if fixed != nil {
		native = fixed.provider
	} else {
		native, _ = creativeReviewProviderAtContext(t, model, window)
	}
	p := &qualityProvider{Provider: native, window: window, correction: correction}
	fake := transport.NewFake()
	s := newTestServerWithRuntime(t, Runtime{LLMProvider: p, Transport: fake, MotionTransport: fake, MotionSimulated: true})
	t.Cleanup(s.Close)
	saveSettings(t, s.store, func(cfg config.Settings) config.Settings {
		if fixed != nil {
			cfg.LLM = fixed.settings
			cfg.LLM.MotionGenerationMode = mode
			return cfg
		}
		cfg.LLM.RetryRefusalLocally = false
		cfg.LLM.Provider, cfg.LLM.Model = config.LLMProviderOllama, model
		cfg.LLM.ReasoningMode, cfg.LLM.ChatVoice, cfg.LLM.MotionGenerationMode = "off", "utility", mode
		cfg.Motion.SpeedMinPercent, cfg.Motion.SpeedMaxPercent = 10, 80
		if hosted {
			c := config.ModelConnection{ID: "quality", Name: "Quality", Provider: config.LLMProviderChatGPT, Model: model, ReasoningEffort: "low", ContextWindow: window}.Normalize()
			cfg.LLM.Connections, cfg.LLM.ConversationConnectionID = []config.ModelConnection{c}, c.ID
		} else if window > 0 {
			cfg.LLM.LocalCapabilityLevel = config.ModelCapabilityAdvanced
			cfg.LLM.LlamaCPPContextSize = window
			cfg.LLM.MaxOutputTokens = config.MinimumLocalOutput(config.ModelCapabilityAdvanced)
		}
		return cfg
	})
	return s, p, fake
}
