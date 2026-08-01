package config

import (
	"strings"
	"testing"
)

func TestLatencyPoliciesDefaultForExistingSettings(t *testing.T) {
	settings, migrated, err := loadSettingsFromBytes([]byte(`{"version":2,"llm":{"provider":"llama_cpp","llama_cpp_mode":"managed"},"voice":{"tts_provider":"none","asr_provider":"none"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if migrated {
		t.Fatal("additive latency policies should not require a schema migration")
	}
	if settings.LLM.ManagedLoadPolicy != LLMManagedLoadStartup {
		t.Fatalf("managed load policy = %q, want startup", settings.LLM.ManagedLoadPolicy)
	}
	if settings.Voice.ChatSpeechPolicy != ChatSpeechInterrupt {
		t.Fatalf("chat speech policy = %q, want interrupt", settings.Voice.ChatSpeechPolicy)
	}
}

func TestLatencyPolicyUpdatesPreserveOmittedValues(t *testing.T) {
	current := DefaultSettings()
	current.LLM.ManagedLoadPolicy = LLMManagedLoadOnDemand
	current.Voice.ChatSpeechPolicy = ChatSpeechFinishCurrent

	llmUpdate := LLMUpdateFromSettings(current.LLM)
	llmUpdate.ManagedLoadPolicy = nil
	nextLLM, err := applyLLMUpdate(current.LLM, llmUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if nextLLM.ManagedLoadPolicy != LLMManagedLoadOnDemand {
		t.Fatalf("omitted managed load policy = %q", nextLLM.ManagedLoadPolicy)
	}
	nextVoice := applyVoiceUpdate(current.Voice, VoiceUpdate{})
	if nextVoice.ChatSpeechPolicy != ChatSpeechFinishCurrent {
		t.Fatalf("omitted chat speech policy = %q", nextVoice.ChatSpeechPolicy)
	}

	startup := " startup "
	interrupt := " INTERRUPT "
	llmUpdate.ManagedLoadPolicy = &startup
	nextLLM, err = applyLLMUpdate(current.LLM, llmUpdate)
	if err != nil {
		t.Fatal(err)
	}
	nextVoice = applyVoiceUpdate(current.Voice, VoiceUpdate{ChatSpeechPolicy: &interrupt})
	if nextLLM.ManagedLoadPolicy != LLMManagedLoadStartup || nextVoice.ChatSpeechPolicy != ChatSpeechInterrupt {
		t.Fatalf("normalized policies = %q, %q", nextLLM.ManagedLoadPolicy, nextVoice.ChatSpeechPolicy)
	}
}

func TestLatencyPolicyValidationAndOptions(t *testing.T) {
	settings := DefaultSettings()
	settings.LLM.ManagedLoadPolicy = "sometimes"
	if _, err := NormalizeSettings(settings); err == nil || !strings.Contains(err.Error(), "managed LLM load policy") {
		t.Fatalf("invalid load policy error = %v", err)
	}
	settings = DefaultSettings()
	settings.Voice.ChatSpeechPolicy = "queue_forever"
	if _, err := NormalizeSettings(settings); err == nil || !strings.Contains(err.Error(), "chat speech policy") {
		t.Fatalf("invalid speech policy error = %v", err)
	}

	options := publicSettingsOptionHints()
	if len(options.LLMManagedLoadPolicies) != 2 || options.LLMManagedLoadPolicies[0] != LLMManagedLoadStartup {
		t.Fatalf("managed load options = %v", options.LLMManagedLoadPolicies)
	}
	if len(options.ChatSpeechPolicies) != 2 || options.ChatSpeechPolicies[0] != ChatSpeechInterrupt {
		t.Fatalf("chat speech options = %v", options.ChatSpeechPolicies)
	}
}
