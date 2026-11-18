package chat

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

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
