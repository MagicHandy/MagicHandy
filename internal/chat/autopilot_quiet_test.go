package chat

import "testing"

func TestAutopilotQuietReplyIsExplicitAndCannotCarryMotion(t *testing.T) {
	service := AutopilotService{Capabilities: FullCapabilities(), Patterns: defaultPatternChoices()}
	for _, raw := range []string{`{"next":"later"}`, `{"reply":null,"next":"later"}`, `{"reply":"","next":"later"}`, `{"intent":"rest","reply":" ","next":"later"}`, `{"intent":"rest","reply":"","next":"soon"}`, `{"intent":"rest","reply":"","next":"later","motion":{"action":"none"}}`} {
		if _, err := service.parse(raw, AutopilotKindSpeech); err == nil {
			t.Fatalf("ambiguous quiet reply accepted: %s", raw)
		}
	}
	response, err := service.parse(`{"intent":"The point is already covered","reply":"","next":"later"}`, AutopilotKindSpeech)
	if err != nil || response.Reply != "" || response.Motion != nil || response.Next != AutopilotTimingLater {
		t.Fatalf("quiet reply: %+v, %v", response, err)
	}
	response, err = service.parse(`{"intent":"quiet","reply":"Still speaking.","next":"later"}`, AutopilotKindSpeech)
	if err != nil || response.Reply != "Still speaking." {
		t.Fatal("planning words overrode the explicit spoken reply")
	}
}
