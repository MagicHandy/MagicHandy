package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestLocalBandExamplesUseTheirTierAndPlaybackState(t *testing.T) {
	for _, level := range []string{config.ModelCapabilitySimple, config.ModelCapabilityIntermediate} {
		for _, action := range []string{MotionActionStart, MotionActionUpdate} {
			t.Run(level+"/"+action, func(t *testing.T) {
				caps := Capabilities{LocalInference: true, Motion: true, MotionMode: MotionModeLayered, SimpleMotion: level == config.ModelCapabilitySimple, IntermediateMotion: level == config.ModelCapabilityIntermediate}
				score := DefaultLayeredScore(23)
				state := MotionContext{Running: action == MotionActionUpdate, Layered: &score, SpeedMinPercent: 20, SpeedMaxPercent: 27}
				req := llm.ChatRequest{Messages: []llm.Message{{Role: "system", Content: "The contract."}, {Role: "user", Content: "Use both marks."}}, MaxTokens: 256}
				got := (Service{PromptBudget: PromptBudgetSettings{ContextSize: 32768}}).withLocalMotionGuide(req, state, caps, DefaultPromptSetID)
				if len(got.Messages) != 3 || got.Messages[1].Role != "system" || got.Messages[2] != req.Messages[1] {
					t.Fatal("guide displaced or rewrote the request")
				}
				guide := got.Messages[1].Content
				pos := strings.Index(guide, `{"action":"`+action+`",`)
				if pos < 0 {
					t.Fatal("example missing")
				}
				var example json.RawMessage
				if err := json.NewDecoder(strings.NewReader(guide[pos:])).Decode(&example); err != nil {
					t.Fatal(err)
				}
				var result struct {
					Action string
					Edits  json.RawMessage
				}
				if err := json.Unmarshal(example, &result); err != nil {
					t.Fatal(err)
				}
				if result.Action != action || result.Edits[0] != '[' {
					t.Fatalf("wrong action or edits shape: %s", example)
				}
				limits := config.DefaultSettings().Motion
				limits.SpeedMinPercent, limits.SpeedMaxPercent = state.SpeedMinPercent, state.SpeedMaxPercent
				raw := string(example)
				if caps.forInteractiveDialect(DefaultPromptSetID, &state).usesGroupedLayeredEdits() {
					normalized, err := normalizeIntermediateLayeredReply(raw)
					if err != nil {
						t.Fatal(err)
					}
					raw = normalized
				}
				_, next, _, err := ParseLayeredReply(raw, score, limits)
				if err != nil || next.MinPercent != 17 || next.MaxPercent != 69 || next.SpeedPercent != 27 {
					t.Fatalf("example did not apply within the saved speed range: %v", err)
				}
			})
		}
	}
}

func TestSimpleRepairPreservesTheUserTurnWhenRemovingGeometryGuidance(t *testing.T) {
	caps := Capabilities{SimpleMotion: true, MotionMode: MotionModeLayered}
	for _, guide := range []string{localRequestedMotionGuide(caps, MotionContext{}), compactBandGuide} {
		human := llm.Message{Role: "user", Content: "Change the lower mark."}
		feedback := llm.Message{Role: "user", Content: "Repair the JSON."}
		req := llm.ChatRequest{Messages: []llm.Message{{Role: "system", Content: "Contract."}, {Role: "system", Content: guide}, human, feedback}}
		got := simpleRepairInput(req)
		if len(got.Messages) != 3 || got.Messages[1] != human || got.Messages[2] != feedback || len(req.Messages) != 4 {
			t.Fatal("repair lost the user turn or retained the optional example")
		}
	}
}

func TestCompactBandGuidePreservesTheProviderWindow(t *testing.T) {
	const window = 16384
	req := llm.ChatRequest{MaxTokens: 256, HistoryMessagesDropped: 2, Messages: []llm.Message{
		{Role: "system", Content: "Contract."}, {Role: "user", Content: strings.Repeat("old ", 250)},
		{Role: "assistant", Content: "Recent."}, {Role: "user", Content: "Use my corrected marks."},
	}}
	// Use the same byte accounting as the provider. The oldest row must
	// already be removed, and the surviving rows leave room only for a short hint.
	_, measure, err := llm.BudgetChatRequest(req, window)
	if err != nil {
		t.Fatal(err)
	}
	req.Messages[2].Content += strings.Repeat("x", measure.LimitBytes-measure.InputBytes+len(req.Messages[1].Content)+64-160)
	before, b, err := llm.BudgetChatRequest(req, window)
	if err != nil || b.HistoryMessagesDropped != 3 {
		t.Fatalf("invalid pressure fixture: %+v %v", b, err)
	}
	got := withOptionalTurnGuide(req, strings.Repeat("full example ", 80), compactBandGuide, window, b.HistoryMessagesDropped)
	after, a, err := llm.BudgetChatRequest(got, window)
	if err != nil || a.HistoryMessagesDropped != b.HistoryMessagesDropped || len(after.Messages) != len(before.Messages)+1 {
		t.Fatalf("guide displaced admitted history: %+v %v", a, err)
	}
	m := after.Messages
	if m[len(m)-2].Role != "system" || m[len(m)-2].Content != compactBandGuide || m[len(m)-1] != before.Messages[len(before.Messages)-1] || m[1] != before.Messages[1] {
		t.Fatal("compact hint lost the recent dialogue or the original human request")
	}
}

func TestSimpleGroupedDialectMatchesLiveSchemaAndKeepsAuthority(t *testing.T) {
	local := Capabilities{LocalInference: true, Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true}
	remote := local
	remote.LocalInference = false
	for _, tc := range []struct {
		name, prompt string
		caps         Capabilities
		state        MotionContext
		grouped      bool
	}{
		{"local stopped", DefaultPromptSetID, local, MotionContext{}, true},
		{"local running", DefaultPromptSetID, local, MotionContext{Running: true}, true},
		{"translated", PromptSetIDJapanese, local, MotionContext{}, false},
		{"connection", DefaultPromptSetID, remote, MotionContext{}, false},
		{"paused", DefaultPromptSetID, local, MotionContext{Paused: true}, false},
		{"autopilot", DefaultPromptSetID, local, MotionContext{Autopilot: true}, false},
		{"restart", DefaultPromptSetID, local, MotionContext{ManualRestartAvailable: true}, false},
		{"pace only", DefaultPromptSetID, local, MotionContext{Outputs: &OutputContext{Strokers: 1, PaceOnly: 1}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, _ := BuiltinPromptSetByID(tc.prompt)
			tc.state.MotionMode = MotionModeLayered
			cfg := tc.caps.forInteractiveDialect(tc.prompt, &tc.state)
			if cfg.usesGroupedLayeredEdits() != tc.grouped || !cfg.SimpleMotion || cfg.IntermediateMotion || cfg.AdvancedMotion {
				t.Fatal("dialect changed the model's capability or eligibility")
			}
			composed := ComposeInteractivePrompt(set, nil, nil, tc.caps, &tc.state, nil)
			if strings.Contains(composed.Prompt, intermediateLayeredListGuide) != tc.grouped {
				t.Fatal("prompt and tier disagree")
			}
			emptyEdits := "{}"
			if tc.grouped {
				emptyEdits = "[]"
				if strings.Contains(composed.Prompt, `"edits":{}`) {
					t.Fatal("grouped prompt demonstrates object-shaped empty edits")
				}
			}
			if tc.prompt == DefaultPromptSetID && !strings.Contains(composed.Prompt, `Conversation requires "edits":`+emptyEdits) {
				t.Fatal("final output guard disagrees with the no-motion schema")
			}
			schema := string(InteractiveResponseSchema(nil, tc.caps, &tc.state, tc.prompt))
			if !strings.Contains(schema, `"const":`+emptyEdits) || strings.Contains(schema, `"phrase"`) {
				t.Fatal("unexpected live actions or Advanced vocabulary")
			}
		})
	}
	state := MotionContext{MotionMode: MotionModeLayered, Running: true}
	current := DefaultLayeredScore(35)
	service := Service{Capabilities: &local}
	limits := config.DefaultSettings().Motion
	// Previously captured Simple object edits stay readable. Both forms retain
	// the same active score and go through the usual authorization and limits.
	for _, raw := range []string{`{"action":"update","edits":{"change_by":{"speed_percent":4}},"reply":"Adjusting."}`, `{"action":"update","edits":[{"change_by":{"speed_percent":4}}],"reply":"Adjusting."}`} {
		reply, err := service.parseLayeredResponse(raw, current, state, limits, false)
		if err != nil || reply.Motion == nil || reply.Motion.Layered.SpeedPercent != 39 {
			t.Fatalf("compatible edit failed: %v", err)
		}
	}
	for _, raw := range []string{`{"action":"update","edits":[{"range":{"min_percent":12}}],"reply":"Changing."}`, `{"action":"update","edits":[{"controls":{"speed_percent":99}}],"reply":"Changing."}`} {
		if _, err := service.parseLayeredResponse(raw, current, state, limits, false); err == nil {
			t.Fatal("invalid paired edit bypassed validation")
		}
	}
	service.TrustedMotionInput = true
	if _, err := service.parseLayeredResponse(`{"edits":[{"change_by":{"speed_percent":4}}],"reply":"Changing."}`, current, state, limits, false); err == nil {
		t.Fatal("interactive dialect reached autonomous Simple planning")
	}
}
