package httpapi

import (
	"github.com/mapledaemon/MagicHandy/internal/config"
	"testing"
)

func TestReasoningChangeInvalidatesProviderAndGenerationProof(t *testing.T) {
	connection := config.ModelConnection{ID: "chatgpt", Name: "ChatGPT", Provider: config.LLMProviderChatGPT, Model: "account-model"}.Normalize()
	settings := config.DefaultSettings().LLM.WithConnection(connection)
	before := hostedProviderKey(settings)
	proof := connectionTestKey(connection, 7)
	connection.ReasoningEffort = "low"
	if hostedProviderKey(settings.WithConnection(connection)) == before || connectionTestKey(connection, 7) == proof {
		t.Fatal("reasoning change reused a provider or generation proof")
	}
}
