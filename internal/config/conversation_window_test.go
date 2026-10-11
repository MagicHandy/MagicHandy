package config

import "testing"

func TestConversationWindowIsPerConnectionAndBounded(t *testing.T) {
	for _, size := range []int{0, 32768, 131072, 262144} {
		c := ModelConnection{ID: "example", Name: "Example", Provider: LLMProviderChatGPT, Model: "test", ContextWindow: size}.Normalize()
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		window, messages, bytes := c.ConversationWindow()
		if messages > 512 || bytes > 192000 || window < bytes {
			t.Fatal(window, messages, bytes)
		}
		s := DefaultSettings().LLM
		s.Connections = []ModelConnection{c}
		s.ConversationConnectionID = c.ID
		hosted := s.ConversationSettings()
		if !hosted.IsHosted() || hosted.ActiveConnection.ContextWindow != size || s.ActiveConnection != nil {
			t.Fatal("role view changed local settings")
		}
		if s.LocalSettings().LlamaCPPContextSize != s.LlamaCPPContextSize {
			t.Fatal("local context changed")
		}
	}
	for _, size := range []int{-1, 1, 64000, 999999999} {
		c := ModelConnection{ID: "example", Name: "Example", Provider: LLMProviderChatGPT, ContextWindow: size}.Normalize()
		if c.Validate() == nil {
			t.Fatal("unbounded context admitted", size)
		}
	}
}
