package chat

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestSmallLocalWindowsAdmitTheLiveLayeredContract(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	const correction = "Cedar now means full strokes from 26 to 62 at speed 30."
	for _, tc := range []struct {
		name                  string
		window, repetitions   int
		intermediate, bounded bool
	}{
		{"E4B ordinary default", 32768, 5, false, false},
		{"E4B minimum", 16384, 65, false, true},
		{"E4B verbose default", 32768, 88, false, true},
		{"12B verbose default", 32768, 88, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := Capabilities{LocalInference: true, Motion: true, MotionMode: MotionModeLayered, SimpleMotion: !tc.intermediate, IntermediateMotion: tc.intermediate}
			score := DefaultLayeredScore(45)
			state := MotionContext{MotionMode: MotionModeLayered, Layered: &score, SpeedMinPercent: 20, SpeedMaxPercent: 80}
			old := strings.Repeat("Archived display note. ", tc.repetitions)
			var history []llm.Message
			for i := range 7 {
				state.UserRequests = append(state.UserRequests, old)
				state.UserRequestSecondsAgo = append(state.UserRequestSecondsAgo, 100-i)
				history = append(history, llm.Message{Role: "user", Content: old}, llm.Message{Role: "assistant", Content: "Noted."})
			}
			history = append(history, llm.Message{Role: "user", Content: correction}, llm.Message{Role: "assistant", Content: "Noted."})
			state.UserRequests = append(state.UserRequests, correction)
			state.UserRequestSecondsAgo = append(state.UserRequestSecondsAgo, 3)
			tokens := 256
			if tc.intermediate {
				tokens = 512
			}
			budget := PromptBudgetSettings{ContextSize: tc.window, MaxOutputTokens: tokens}
			for _, running := range []bool{false, true} {
				state.Running = running
				provider := &layeredTestProvider{raw: `{"action":"none","edits":[],"reply":"Ready."}`}
				service := Service{Provider: provider, Prompt: set, Capabilities: &caps, MotionContext: &state, PromptBudget: budget, MaxTokens: tokens}
				_, err := service.Complete(t.Context(), Request{Message: "Please read back my latest correction.", History: history}, nil)
				if err != nil {
					t.Fatal(err)
				}
				req := provider.request
				comp := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil, budget)
				if comp.Prompt != req.Messages[0].Content || string(req.JSONSchema) != string(InteractiveResponseSchema(nil, caps, &state, set.ID)) {
					t.Fatal("prompt inspection and live contract differ")
				}
				if strings.Contains(comp.Prompt, old) == tc.bounded || !strings.Contains(comp.Prompt, correction) || !strings.Contains(comp.Prompt, `"seconds_ago":3`) {
					t.Fatal("wrong request copies or ages in engine snapshot")
				}
				if strings.Contains(comp.Prompt, `"older_request_copies_omitted":7`) != tc.bounded || strings.Contains(comp.Prompt, "If a named motion's definition is missing") != (tc.bounded && !tc.intermediate) {
					t.Fatal("incorrect reference or omitted-copy count for the selected window")
				}
				admitted, _, err := llm.BudgetChatRequest(req, tc.window)
				if err != nil || !slices.Contains(admitted.Messages, llm.Message{Role: "user", Content: correction}) {
					t.Fatalf("original latest correction did not reach the provider: %v", err)
				}
				if len(state.UserRequests) != 8 || len(state.UserRequestSecondsAgo) != 8 {
					t.Fatal("mutated full engine timeline")
				}
			}
		})
	}
}

func TestBoundedRequestTimelineCountsEscapedJSON(t *testing.T) {
	escaped := strings.Repeat("<&", 100)
	state := MotionContext{UserRequests: []string{"earlier", escaped}, UserRequestSecondsAgo: []int{10, 3}}
	if got := state.withBoundedRequestTimeline(1024); len(got.UserRequests) != 0 || len(got.UserRequestSecondsAgo) != 0 {
		t.Fatal("included a partial or oversized timeline copy")
	}
	state.UserRequests = append(state.UserRequests, "newest")
	state.UserRequestSecondsAgo = append(state.UserRequestSecondsAgo, 1)
	got := state.withBoundedRequestTimeline(1024)
	if !slices.Equal(got.UserRequests, []string{"newest"}) || !slices.Equal(got.UserRequestSecondsAgo, []int{1}) {
		t.Fatal("lost the newest complete timeline entry or its age")
	}
	untimed := MotionContext{UserRequests: []string{"first", "é"}, UserRequestSecondsAgo: []int{1}}
	encoded, _ := json.Marshal([]string{"é"})
	got = untimed.withBoundedRequestTimeline(len(encoded))
	if !slices.Equal(got.UserRequests, []string{"é"}) || got.UserRequestSecondsAgo != nil {
		t.Fatal("exact JSON boundary lost a line or attached a mismatched age")
	}
}

func TestLongLatestCorrectionSurvivesAnOmittedTimelineCopy(t *testing.T) {
	set, _ := BuiltinPromptSetByID(DefaultPromptSetID)
	caps := Capabilities{LocalInference: true, Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true}
	correction := strings.Repeat("Session note. ", 100) + "Replace the earlier marks with 26 and 62 at pace 30."
	state := MotionContext{Running: true, MotionMode: MotionModeLayered, UserRequests: []string{correction}}
	provider := &layeredTestProvider{raw: `{"action":"none","edits":[],"reply":"Ready."}`}
	service := Service{Prompt: set, Provider: provider, Capabilities: &caps, MotionContext: &state, MaxTokens: 256,
		PromptBudget: PromptBudgetSettings{ContextSize: 16384, MaxOutputTokens: 256}}
	prior := llm.Message{Role: "user", Content: correction}
	if _, err := service.Complete(t.Context(), Request{Message: "Read those settings back.", History: []llm.Message{prior}}, nil); err != nil {
		t.Fatal(err)
	}
	actual, _, err := llm.BudgetChatRequest(provider.request, 16384)
	if err != nil || strings.Contains(actual.Messages[0].Content, correction) || !slices.Contains(actual.Messages, prior) {
		t.Fatalf("lost or truncated the newest correction in dialogue: %v", err)
	}
}

func TestEmptySimpleChatFitsMinimumWindow(t *testing.T) {
	for _, set := range BuiltinPromptSets() {
		for _, mood := range []bool{false, true} {
			caps := Capabilities{LocalInference: true, Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true, MoodTracking: mood}
			state := MotionContext{MotionMode: MotionModeLayered}
			budget := PromptBudgetSettings{ContextSize: 16384, MaxOutputTokens: 256}
			comp := ComposeInteractivePrompt(set, nil, nil, caps, &state, nil, budget)
			req := llm.ChatRequest{Messages: []llm.Message{{Role: "system", Content: comp.Prompt}, {Role: "user", Content: "Hello."}}, MaxTokens: 256, JSONSchema: InteractiveResponseSchema(nil, caps, &state, set.ID)}
			if _, _, err := llm.BudgetChatRequest(req, budget.ContextSize); err != nil {
				t.Fatalf("%s, mood %t: %v", set.ID, mood, err)
			}
		}
	}
}

func TestCompactGroupedReferenceNamesTheSchemaOperations(t *testing.T) {
	state := MotionContext{MotionMode: MotionModeLayered}
	caps := (Capabilities{LocalInference: true, Motion: true, MotionMode: MotionModeLayered, SimpleMotion: true}).forInteractiveDialect(DefaultPromptSetID, &state)
	_, limits := layeredContextScore(&state)
	var schema struct {
		Properties struct {
			Edits struct {
				Items struct {
					AnyOf []struct {
						Properties map[string]struct{ Enum []string }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(continuousEditSchema(limits, caps, state), &schema); err != nil {
		t.Fatal(err)
	}
	groups := schema.Properties.Edits.Items.AnyOf
	if len(groups) == 0 {
		t.Fatal("expected grouped edits")
	}
	for _, group := range groups {
		for name, field := range group.Properties {
			if !strings.Contains(compactGroupedLayeredContract, name) {
				t.Errorf("missing edit group %s", name)
			}
			for _, value := range field.Enum {
				if !strings.Contains(compactGroupedLayeredContract, value) {
					t.Errorf("missing %s value %s", name, value)
				}
			}
		}
	}
}

func TestLocalTurnGuideUsesProviderAdmittedHistory(t *testing.T) {
	const window = 16384
	for _, tc := range []struct {
		name           string
		advanced       bool
		storageOmitted int
		roomAfterTrim  int
		wantGuide      bool
	}{
		{"Simple byte pressure", false, 0, 1200, true},
		{"Simple row and byte pressure", false, 19, 1200, true},
		{"Advanced full guide fits after trimming", true, 0, 1200, true},
		{"Advanced full guide would displace dialogue", true, 0, 32, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := llm.ChatRequest{HistoryMessagesDropped: tc.storageOmitted, MaxTokens: 256,
				Messages: []llm.Message{{Role: "system", Content: "Contract."}}}
			for i := range 68 {
				role := "user"
				if i%2 != 0 {
					role = "assistant"
				}
				req.Messages = append(req.Messages, llm.Message{Role: role, Content: fmt.Sprintf("Retained line %d.", i)})
			}
			req.Messages = append(req.Messages, llm.Message{Role: "user", Content: "Read the settings back."})
			_, measured, err := llm.BudgetChatRequest(req, window)
			if err != nil {
				t.Fatal(err)
			}
			req.Messages[0].Content += strings.Repeat("x", measured.LimitBytes-measured.InputBytes-tc.roomAfterTrim)
			req.Messages = slices.Insert(req.Messages, 1, llm.Message{Role: "user", Content: "Old request."},
				llm.Message{Role: "assistant", Content: strings.Repeat("x", 2500)})
			before, base, err := llm.BudgetChatRequest(req, window)
			if err != nil || base.HistoryMessagesDropped != tc.storageOmitted+2 {
				t.Fatalf("invalid fixture: %+v, %v", base, err)
			}
			score := DefaultLayeredScore(30)
			state := MotionContext{Running: true, Layered: &score}
			caps := Capabilities{Motion: true, LocalInference: true, MotionMode: MotionModeLayered,
				SimpleMotion: !tc.advanced, AdvancedMotion: tc.advanced}
			got := (Service{PromptBudget: PromptBudgetSettings{ContextSize: window}}).withLocalMotionGuide(req, state, caps, DefaultPromptSetID)
			after, budget, err := llm.BudgetChatRequest(got, window)
			if err != nil || budget.HistoryMessagesDropped != base.HistoryMessagesDropped {
				t.Fatalf("guide changed admitted history: %+v, %v", budget, err)
			}
			messages := after.Messages
			if tc.wantGuide {
				i := len(messages) - 2
				want := "read-back"
				if !tc.advanced {
					want = fmt.Sprintf("%d earlier messages", base.HistoryMessagesDropped)
				}
				if messages[i].Role != "system" || !strings.Contains(messages[i].Content, want) {
					t.Fatalf("missing guide for admitted dialogue: wanted %q", want)
				}
				messages = slices.Delete(slices.Clone(messages), i, i+1)
			}
			if !slices.Equal(messages, before.Messages) {
				t.Fatal("guide displaced a retained role, turn or the current request")
			}
		})
	}
}

func TestBandGuidePreservesDialogueAndBudget(t *testing.T) {
	req := llm.ChatRequest{HistoryMessagesDropped: 4, Messages: []llm.Message{{Role: "system", Content: "Contract."}, {Role: "assistant", Content: "Earlier reply."}, {Role: "user", Content: "Move the lower end five points higher."}}, MaxTokens: 256}
	score := DefaultLayeredScore(22)
	score.MinPercent, score.MaxPercent = 14, 72
	state := MotionContext{Layered: &score, SpeedMinPercent: 10, SpeedMaxPercent: 24}
	got := (Service{PromptBudget: PromptBudgetSettings{ContextSize: 16384}}).withLocalMotionGuide(req, state, Capabilities{LocalInference: true, SimpleMotion: true, Motion: true, MotionMode: MotionModeLayered}, DefaultPromptSetID)
	if len(got.Messages) != len(req.Messages)+1 {
		t.Fatal("missing request-specific band example")
	}
	last := len(got.Messages) - 1
	guide := got.Messages[last-1]
	if guide.Role != "system" || !strings.Contains(guide.Content, "14..72, overall speed 22") || !strings.Contains(guide.Content, "4 earlier messages") || !slices.Equal(got.Messages[:last-1], req.Messages[:2]) || got.Messages[last] != req.Messages[2] {
		t.Fatal("guide lost the authoritative band, speed cap or original dialogue")
	}
	got.Messages[0].Content = "Changed test copy"
	if req.Messages[0].Content != "Contract." {
		t.Fatal("request shares its message slice with the guide")
	}
	// Keep all of a near-full history, even when the optional example would
	// still fit if the oldest assistant message were dropped.
	limit := llm.PromptInputByteLimit(16384, req.MaxTokens, 0, 0)
	req.Messages[1].Content = strings.Repeat("x", limit-280)
	if _, budget, err := llm.BudgetChatRequest(req, 16384); err != nil || budget.HistoryMessagesDropped != req.HistoryMessagesDropped {
		t.Fatal("fixture does not fit without the guide")
	}
	if got := (Service{PromptBudget: PromptBudgetSettings{ContextSize: 16384}}).withLocalMotionGuide(req, state, Capabilities{LocalInference: true, SimpleMotion: true, Motion: true, MotionMode: MotionModeLayered}, DefaultPromptSetID); !slices.Equal(got.Messages, req.Messages) {
		t.Fatal("optional example displaced conversation history")
	}
}

func TestBandGuideRespectsPlaybackAuthority(t *testing.T) {
	score := DefaultLayeredScore(33)
	req := llm.ChatRequest{HistoryMessagesDropped: 4, Messages: []llm.Message{{Role: "system", Content: "Contract."}, {Role: "user", Content: "Move an endpoint."}}}
	for _, state := range []MotionContext{
		{Layered: &score, Paused: true}, {Layered: &score, Autopilot: true},
		{Layered: &score, ManualRestartAvailable: true},
		{Layered: &score, Outputs: &OutputContext{Strokers: 0}},
		{Layered: &score, Outputs: &OutputContext{Strokers: 1, PaceOnly: 1}},
	} {
		if got := (Service{}).withLocalMotionGuide(req, state, Capabilities{LocalInference: true, SimpleMotion: true, Motion: true, MotionMode: MotionModeLayered}, DefaultPromptSetID); !slices.Equal(got.Messages, req.Messages) {
			t.Fatal("linear guide appeared without ordinary position control")
		}
	}
}

func TestLongLocalReadbackGuideRespectsRouteAndSession(t *testing.T) {
	score := DefaultLayeredScore(36)
	state := MotionContext{Running: true, Layered: &score}
	caps := Capabilities{Motion: true, LocalInference: true, AdvancedMotion: true, MotionMode: MotionModeLayered}
	req := llm.ChatRequest{Messages: []llm.Message{{Role: "system", Content: "Contract."}}}
	for i := range PromptHistoryLimit + 1 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		req.Messages = append(req.Messages, llm.Message{Role: role, Content: "Synthetic dialogue."})
	}
	req.Messages = append(req.Messages, llm.Message{Role: "user", Content: "Read back the corrected settings."})
	service := Service{PromptBudget: PromptBudgetSettings{ContextSize: 131072}}
	got := service.withLocalMotionGuide(req, state, caps, DefaultPromptSetID)
	if len(got.Messages) != len(req.Messages)+1 || got.Messages[len(got.Messages)-1] != req.Messages[len(req.Messages)-1] ||
		!strings.Contains(got.Messages[len(got.Messages)-2].Content, `"action":"none"`) || !strings.Contains(got.Messages[len(got.Messages)-2].Content, `"action":"update"`) {
		t.Fatal("long conversation lost the read-back/apply distinction or the original request")
	}
	hosted := caps
	hosted.LocalInference = false
	if h := service.withLocalMotionGuide(req, state, hosted, DefaultPromptSetID); !slices.Equal(h.Messages, req.Messages) {
		t.Fatal("local guide changed a hosted request")
	}
	if translated := service.withLocalMotionGuide(req, state, caps, PromptSetIDJapanese); !slices.Equal(translated.Messages, req.Messages) {
		t.Fatal("English guide changed the Japanese prompt")
	}
	service.TrustedMotionInput = true
	if auto := service.withLocalMotionGuide(req, state, caps, DefaultPromptSetID); !slices.Equal(auto.Messages, req.Messages) {
		t.Fatal("conversation guide leaked into an autonomous planner")
	}
	if composedHistoryDropped(req.Messages[1:len(req.Messages)-1], got.Messages) != 0 {
		t.Fatal("a turn-specific system message was counted as dialogue")
	}
}
