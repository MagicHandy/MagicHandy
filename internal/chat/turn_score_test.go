package chat

import (
	"slices"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

func TestBandGuidePreservesDialogueAndBudget(t *testing.T) {
	req := llm.ChatRequest{HistoryMessagesDropped: 4, Messages: []llm.Message{{Role: "system", Content: "Contract."}, {Role: "assistant", Content: "Earlier reply."}, {Role: "user", Content: "Move the lower end five points higher."}}, MaxTokens: 256}
	score := DefaultLayeredScore(22)
	score.MinPercent, score.MaxPercent = 14, 72
	state := MotionContext{Layered: &score, SpeedMinPercent: 10, SpeedMaxPercent: 24}
	got := (Service{PromptBudget: PromptBudgetSettings{ContextSize: 16384}}).withMotionHistoryGuide(req, state, Capabilities{LocalInference: true, SimpleMotion: true, Motion: true, MotionMode: MotionModeLayered}, DefaultPromptSetID)
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
	if got := (Service{PromptBudget: PromptBudgetSettings{ContextSize: 16384}}).withMotionHistoryGuide(req, state, Capabilities{LocalInference: true, SimpleMotion: true, Motion: true, MotionMode: MotionModeLayered}, DefaultPromptSetID); !slices.Equal(got.Messages, req.Messages) {
		t.Fatal("optional example displaced conversation history")
	}
}

func TestBandGuideRespectsPlaybackAuthority(t *testing.T) {
	score := DefaultLayeredScore(33)
	req := llm.ChatRequest{HistoryMessagesDropped: 4, Messages: []llm.Message{{Role: "system", Content: "Contract."}, {Role: "user", Content: "Move an endpoint."}}}
	for _, state := range []MotionContext{
		{Layered: &score, Paused: true}, {Layered: &score, ManualRestartAvailable: true},
		{Layered: &score, Outputs: &OutputContext{Strokers: 0}},
		{Layered: &score, Outputs: &OutputContext{Strokers: 1, PaceOnly: 1}},
	} {
		if got := (Service{}).withMotionHistoryGuide(req, state, Capabilities{LocalInference: true, SimpleMotion: true, Motion: true, MotionMode: MotionModeLayered}, DefaultPromptSetID); !slices.Equal(got.Messages, req.Messages) {
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
	got := service.withMotionHistoryGuide(req, state, caps, DefaultPromptSetID)
	if len(got.Messages) != len(req.Messages)+1 || got.Messages[len(got.Messages)-1] != req.Messages[len(req.Messages)-1] ||
		!strings.Contains(got.Messages[len(got.Messages)-2].Content, `"action":"none"`) || !strings.Contains(got.Messages[len(got.Messages)-2].Content, `"action":"update"`) {
		t.Fatal("long conversation lost the read-back/apply distinction or the original request")
	}
	hosted := caps
	hosted.LocalInference = false
	if h := service.withMotionHistoryGuide(req, state, hosted, DefaultPromptSetID); !slices.Equal(h.Messages, req.Messages) {
		t.Fatal("local guide changed a hosted request")
	}
	if translated := service.withMotionHistoryGuide(req, state, caps, PromptSetIDJapanese); !slices.Equal(translated.Messages, req.Messages) {
		t.Fatal("English guide changed the Japanese prompt")
	}
	service.TrustedMotionInput = true
	if auto := service.withMotionHistoryGuide(req, state, caps, DefaultPromptSetID); !slices.Equal(auto.Messages, req.Messages) {
		t.Fatal("conversation guide leaked into an autonomous planner")
	}
	if composedHistoryDropped(req.Messages[1:len(req.Messages)-1], got.Messages) != 0 {
		t.Fatal("a turn-specific system message was counted as dialogue")
	}
}
