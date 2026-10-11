package httpapi

import (
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func TestAutopilotSpeechRespectsModelChoiceWithoutFilteringWords(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		raw := `{"intent":"contribute","reply":"Still here with you.","next":"later"}`
		if quiet {
			raw = `{"intent":"quiet","reply":"","next":"later"}`
		}
		provider := &scriptedLLMProvider{responses: []string{raw}}
		server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
		t.Cleanup(server.Close)
		if _, err := server.chatLog.Append(chat.MessageRoleAssistant, "Still here with you.", ""); err != nil {
			t.Fatal(err)
		}
		decision, err := server.autopilotDecideSpeech(t.Context(), modes.DecisionInput{CurrentSpeed: 30, LastSay: "Still here with you.", SpeedMinPercent: 20, SpeedMaxPercent: 80})
		if err != nil || !decision.Hold || decision.Abstain != quiet || decision.Next != modes.TimingLater {
			t.Fatalf("quiet=%t decision=%+v err=%v", quiet, decision, err)
		}
		if !quiet && decision.Say != "Still here with you." {
			t.Fatalf("model-authored wording was filtered: %+v", decision)
		}
		if server.currentMotionEngine() != nil || len(provider.requests) != 1 {
			t.Fatal("speech choice created motion or another model request")
		}
	}
}
