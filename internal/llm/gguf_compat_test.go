package llm

import "testing"

func TestEmbeddedGGUFComponentKey(t *testing.T) {
	for _, key := range []string{
		"gemma4.vision.embedding_length",
		"vision.block_count",
		"audio.embedding_length",
		"model.projector.type",
		"clip.has_vision_encoder",
	} {
		if !embeddedGGUFComponentKey(key) {
			t.Errorf("embeddedGGUFComponentKey(%q) = false", key)
		}
	}
	for _, key := range []string{"general.architecture", "tokenizer.chat_template", "llama.context_length"} {
		if embeddedGGUFComponentKey(key) {
			t.Errorf("embeddedGGUFComponentKey(%q) = true", key)
		}
	}
}
