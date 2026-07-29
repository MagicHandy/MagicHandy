package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/persona"
)

func TestPromptInspectorReturnsTheExactProductionComposition(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	set, err := server.personalization.prompts.Create("Persona behavior", "INSPECTOR BEHAVIOR SENTINEL")
	if err != nil {
		t.Fatalf("create prompt set: %v", err)
	}
	created := createPersonaVia(t, server, "Rowan")
	recorder, _ := personaRequest(t, server, http.MethodPatch, "/api/personas/"+created.ID,
		map[string]any{
			"chat_voice":    config.LLMChatVoiceIntimate,
			"prompt_set_id": set.ID,
			"lore_mode":     persona.LoreModeFull,
		})
	if recorder.Code != http.StatusOK {
		t.Fatalf("configure persona: status %d body %s", recorder.Code, recorder.Body.String())
	}
	recorder, _ = personaRequest(t, server, http.MethodPost, "/api/personas/"+created.ID+"/lore",
		map[string]any{
			"text":     "Blue velvet is familiar.",
			"keywords": []string{"velvet"},
		})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create lore: status %d body %s", recorder.Code, recorder.Body.String())
	}
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})

	// No controller header: diagnostics inspection is deliberately available to
	// a read-only tab.
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics/prompt-composition", nil)
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("inspector: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cache)
	}

	var inspected struct {
		SessionID   string                 `json:"session_id"`
		PromptSet   string                 `json:"prompt_set"`
		PersonaID   string                 `json:"persona_id"`
		PersonaName string                 `json:"persona_name"`
		LoreMode    string                 `json:"lore_mode"`
		Lore        persona.LoreSelection  `json:"lore"`
		Composition chat.PromptComposition `json:"composition"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &inspected); err != nil {
		t.Fatalf("decode inspector: %v", err)
	}
	if inspected.SessionID != payload.ActiveSessionID ||
		inspected.PromptSet != set.ID ||
		inspected.PersonaID != created.ID ||
		inspected.PersonaName != "Rowan" ||
		inspected.LoreMode != persona.LoreModeFull {
		t.Fatalf("inspector provenance = %+v", inspected)
	}
	if len(inspected.Lore.EntryIDs) != 1 || inspected.Lore.Characters != len([]rune("Blue velvet is familiar.")) {
		t.Fatalf("inspector lore selection = %+v", inspected.Lore)
	}
	if !strings.Contains(inspected.Composition.Prompt, "INSPECTOR BEHAVIOR SENTINEL") ||
		!strings.Contains(inspected.Composition.Prompt, "Blue velvet is familiar.") {
		t.Fatalf("inspector prompt omitted active personalization:\n%s", inspected.Composition.Prompt)
	}

	settings, _ := server.store.Snapshot()
	context, err := server.loadInteractiveChatPromptContext(payload.ActiveSessionID, settings.LLM)
	if err != nil {
		t.Fatalf("load production context: %v", err)
	}
	prompt, memories, _, err := server.resolveInteractiveChatPersonalization(
		effectivePersonaPromptSet(settings.LLM.PromptSet, context.Persona),
	)
	if err != nil {
		t.Fatalf("resolve production personalization: %v", err)
	}
	patterns, err := server.chatPatternChoicesFor(context.Capabilities)
	if err != nil {
		t.Fatalf("resolve pattern choices: %v", err)
	}
	motionContext := server.chatMotionContext(settings.Motion)
	expected := chat.ComposePrompt(
		prompt,
		memories,
		patterns,
		context.Capabilities,
		&motionContext,
		context.ConversationContext,
	)
	if !reflect.DeepEqual(inspected.Composition, expected) {
		t.Fatalf("inspector composition drifted from production\ninspected: %+v\nexpected: %+v",
			inspected.Composition, expected)
	}
}
