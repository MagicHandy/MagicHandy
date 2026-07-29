package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/persona"
)

type personasResponse struct {
	Personas        []persona.Persona `json:"personas"`
	ActivePersonaID string            `json:"active_persona_id"`
	ActiveSessionID string            `json:"active_session_id"`
	Persona         *persona.Persona  `json:"persona"`
	Options         struct {
		ChatVoices     []string `json:"chat_voices"`
		ReactionStyles []string `json:"reaction_styles"`
		FocusAreas     []string `json:"focus_areas"`
		MaxName        int      `json:"max_name"`
		MaxDescription int      `json:"max_description"`
	} `json:"options"`
}

func personaRequest(t *testing.T, server *Server, method, path string, body any) (*httptest.ResponseRecorder, personasResponse) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	if method != http.MethodGet {
		request = withController(request)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	var decoded personasResponse
	if strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	}
	return recorder, decoded
}

func createPersonaVia(t *testing.T, server *Server, name string) persona.Persona {
	t.Helper()
	recorder, decoded := personaRequest(t, server, http.MethodPost, "/api/personas",
		map[string]any{"name": name})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create %q: status %d body %s", name, recorder.Code, recorder.Body.String())
	}
	if decoded.Persona == nil {
		t.Fatalf("create %q returned no persona", name)
	}
	return *decoded.Persona
}

func portraitJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x30, A: 0xFF})
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, canvas, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode portrait: %v", err)
	}
	return buffer.Bytes()
}

func TestPersonasEndpointReportsTheServersOwnVocabulary(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	recorder, decoded := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	if len(decoded.Personas) != 0 {
		t.Fatalf("a fresh install has %d personas, want none", len(decoded.Personas))
	}
	// The page must never offer a value the server would reject, so the option
	// lists come from the same source that validates them.
	if strings.Join(decoded.Options.ChatVoices, ",") != strings.Join(config.LLMChatVoices(), ",") {
		t.Fatalf("chat voices = %v, want %v", decoded.Options.ChatVoices, config.LLMChatVoices())
	}
	if strings.Join(decoded.Options.ReactionStyles, ",") != strings.Join(config.LLMReactionStyles(), ",") {
		t.Fatalf("reaction styles = %v, want %v", decoded.Options.ReactionStyles, config.LLMReactionStyles())
	}
	if strings.Join(decoded.Options.FocusAreas, ",") != strings.Join(chat.AreaZones(), ",") {
		t.Fatalf("focus areas = %v, want %v", decoded.Options.FocusAreas, chat.AreaZones())
	}
	if decoded.Options.MaxName != persona.MaxNameChars || decoded.Options.MaxDescription != persona.MaxDescriptionChars {
		t.Fatal("bounds were not reported to the client")
	}
	if decoded.ActiveSessionID == "" {
		t.Fatal("the payload must name the active session so the page can bind to it")
	}
}

func TestPersonaCRUDRoundTripsThroughTheAPI(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")

	recorder, decoded := personaRequest(t, server, http.MethodPatch, "/api/personas/"+created.ID,
		map[string]any{"chat_voice": config.LLMChatVoiceIntimate, "reaction_style": config.LLMReactionStyleTender})
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if decoded.Persona.ChatVoice != config.LLMChatVoiceIntimate {
		t.Fatalf("voice = %q", decoded.Persona.ChatVoice)
	}

	recorder, duplicated := personaRequest(t, server, http.MethodPost, "/api/personas/"+created.ID+"/duplicate", nil)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("duplicate: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if len(duplicated.Personas) != 2 {
		t.Fatalf("library has %d personas after duplicating, want 2", len(duplicated.Personas))
	}

	recorder, remaining := personaRequest(t, server, http.MethodDelete, "/api/personas/"+created.ID, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if len(remaining.Personas) != 1 {
		t.Fatalf("library has %d personas after deleting, want 1", len(remaining.Personas))
	}
}

func TestPersonaAPIMapsFailuresToDistinctStatuses(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	missing := "persona-0123456789ab"

	cases := []struct {
		label  string
		method string
		path   string
		body   any
		want   int
	}{
		{"unknown persona", http.MethodPatch, "/api/personas/" + missing, map[string]any{"name": "x"}, http.StatusNotFound},
		{"malformed id", http.MethodDelete, "/api/personas/not-an-id", nil, http.StatusNotFound},
		{"empty name", http.MethodPost, "/api/personas", map[string]any{"name": "  "}, http.StatusBadRequest},
		{"unknown register", http.MethodPatch, "/api/personas/" + created.ID, map[string]any{"chat_voice": "seductive"}, http.StatusBadRequest},
		{"unknown style", http.MethodPatch, "/api/personas/" + created.ID, map[string]any{"reaction_style": "bratty"}, http.StatusBadRequest},
		{"long name", http.MethodPatch, "/api/personas/" + created.ID, map[string]any{"name": strings.Repeat("n", persona.MaxNameChars+1)}, http.StatusBadRequest},
	}
	for _, testCase := range cases {
		recorder, _ := personaRequest(t, server, testCase.method, testCase.path, testCase.body)
		if recorder.Code != testCase.want {
			t.Fatalf("%s: status %d, want %d (body %s)", testCase.label, recorder.Code, testCase.want, recorder.Body.String())
		}
	}
}

func TestPortraitUploadServeAndClear(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	portrait := portraitJPEG(t, 96, 128)

	request := withController(httptest.NewRequest(http.MethodPost,
		"/api/personas/"+created.ID+"/portrait", bytes.NewReader(portrait)))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload: status %d body %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/personas/"+created.ID+"/portrait", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("serve: status %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q", got)
	}
	// nosniff matters here: this is user-supplied bytes served back from the app's
	// own origin, and a sniffed content type would be an XSS surface.
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), portrait) {
		t.Fatal("served portrait differs from the uploaded bytes")
	}

	_, cleared := personaRequest(t, server, http.MethodDelete, "/api/personas/"+created.ID+"/portrait", nil)
	if cleared.Persona == nil || cleared.Persona.HasPortrait {
		t.Fatal("portrait was not cleared")
	}
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/personas/"+created.ID+"/portrait", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("serve after clear: status %d, want 404", recorder.Code)
	}
}

func TestPortraitUploadRejectsNonImageBytes(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")

	request := withController(httptest.NewRequest(http.MethodPost,
		"/api/personas/"+created.ID+"/portrait", strings.NewReader("<html>not an image</html>")))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d, want 415 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestSelectingAPersonaBindsTheSessionAndNotTheSettings(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.ChatVoice = config.LLMChatVoiceUtility
		settings.LLM.PersonaDescription = "the global description"
		return settings
	})
	created := createPersonaVia(t, server, "Rowan")
	if _, decoded := personaRequest(t, server, http.MethodPatch, "/api/personas/"+created.ID,
		map[string]any{"chat_voice": config.LLMChatVoiceIntimate}); decoded.Persona == nil {
		t.Fatal("configure returned no persona")
	}

	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	recorder, bound := personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+payload.ActiveSessionID+"/persona", map[string]any{"persona_id": created.ID})
	if recorder.Code != http.StatusOK {
		t.Fatalf("bind: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if bound.ActivePersonaID != created.ID {
		t.Fatalf("active persona = %q, want %q", bound.ActivePersonaID, created.ID)
	}

	// The whole point of binding to the session: Settings is untouched, so a user
	// who clears the selection gets exactly what they had before.
	settings, _ := server.store.Snapshot()
	if settings.LLM.ChatVoice != config.LLMChatVoiceUtility {
		t.Fatalf("selecting a persona wrote settings: chat voice is now %q", settings.LLM.ChatVoice)
	}
	if settings.LLM.PersonaDescription != "the global description" {
		t.Fatal("selecting a persona overwrote the saved description")
	}
}

func TestSelectingAnUnknownPersonaIsRejected(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)

	recorder, _ := personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": "persona-0123456789ab"})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", recorder.Code)
	}
}

func TestClearingTheSelectionRestoresTheGlobalAxes(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	sessionID := payload.ActiveSessionID

	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+sessionID+"/persona",
		map[string]any{"persona_id": created.ID})
	recorder, cleared := personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+sessionID+"/persona", map[string]any{"persona_id": ""})
	if recorder.Code != http.StatusOK {
		t.Fatalf("clear: status %d body %s", recorder.Code, recorder.Body.String())
	}
	if cleared.ActivePersonaID != "" {
		t.Fatalf("active persona = %q, want empty", cleared.ActivePersonaID)
	}
	if server.sessionPersona(sessionID) != nil {
		t.Fatal("a cleared session still resolves a persona")
	}
}

// Deleting a persona must not rewrite history. The session keeps its recorded id
// and resolution falls back to the global axis values.
func TestDeletingABoundPersonaLeavesTheSessionReadable(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	sessionID := payload.ActiveSessionID
	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+sessionID+"/persona",
		map[string]any{"persona_id": created.ID})

	recorder, _ := personaRequest(t, server, http.MethodDelete, "/api/personas/"+created.ID, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete: status %d", recorder.Code)
	}
	storedID, err := server.chatLog.SessionPersona(sessionID)
	if err != nil {
		t.Fatalf("read session persona: %v", err)
	}
	if storedID != created.ID {
		t.Fatalf("session persona = %q, want the deleted id retained", storedID)
	}
	if server.sessionPersona(sessionID) != nil {
		t.Fatal("a deleted persona must resolve to nil, not a phantom row")
	}
}

// The guardrail that matters most: a persona is a picture with a name, and it
// must never be able to switch on a motion capability the user left off.
func TestAPersonaCannotGrantAMotionCapability(t *testing.T) {
	settings := config.LLMSettings{ChatVoice: config.LLMChatVoiceUtility}
	dominant := &persona.Persona{
		Name:          "Vesper",
		ChatVoice:     config.LLMChatVoiceExplicit,
		ReactionStyle: config.LLMReactionStyleDominant,
	}

	baseline := chatCapabilities(settings, nil)
	withPersona := chatCapabilities(settings, dominant)

	if withPersona.Motion != baseline.Motion ||
		withPersona.Patterns != baseline.Patterns ||
		withPersona.AreaFocus != baseline.AreaFocus ||
		withPersona.ExperimentalPatterns != baseline.ExperimentalPatterns {
		t.Fatalf("a persona changed the capability gates: %+v vs %+v", withPersona, baseline)
	}
	// It does override the two reply-shaping axes, which is its whole job.
	if withPersona.Voice != chat.VoiceExplicit {
		t.Fatalf("voice = %q, want the persona's register", withPersona.Voice)
	}
	if withPersona.Style != chat.StyleDominant {
		t.Fatalf("style = %q, want the persona's style", withPersona.Style)
	}
}

func TestPersonaWithoutADescriptionKeepsTheGlobalOne(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.ChatVoice = config.LLMChatVoiceIntimate
		settings.LLM.PersonaDescription = "the global description"
		return settings
	})
	created := createPersonaVia(t, server, "Rowan")
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})

	settings, _ := server.store.Snapshot()
	resolved, err := server.loadInteractiveChatPromptContext(payload.ActiveSessionID, settings.LLM)
	if err != nil {
		t.Fatalf("load prompt context: %v", err)
	}
	if resolved.ConversationContext == nil {
		t.Fatal("no conversation context was composed")
	}
	// Selecting a persona that has no description of its own must not silently
	// discard what the user wrote in Settings.
	if resolved.ConversationContext.PersonaDescription != "the global description" {
		t.Fatalf("description = %q, want the global fallback", resolved.ConversationContext.PersonaDescription)
	}
	if resolved.ConversationContext.PersonaName != "Rowan" {
		t.Fatalf("persona name = %q", resolved.ConversationContext.PersonaName)
	}
}

func TestPersonaDescriptionReplacesRatherThanJoinsTheGlobalOne(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.ChatVoice = config.LLMChatVoiceIntimate
		settings.LLM.PersonaDescription = "the global description"
		return settings
	})
	created := createPersonaVia(t, server, "Rowan")
	personaRequest(t, server, http.MethodPatch, "/api/personas/"+created.ID,
		map[string]any{"description": "steady and low-voiced"})
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})

	settings, _ := server.store.Snapshot()
	resolved, err := server.loadInteractiveChatPromptContext(payload.ActiveSessionID, settings.LLM)
	if err != nil {
		t.Fatalf("load prompt context: %v", err)
	}
	// Two partner descriptions in one prompt is a contradiction, not extra context.
	if resolved.ConversationContext.PersonaDescription != "steady and low-voiced" {
		t.Fatalf("description = %q, want only the persona's", resolved.ConversationContext.PersonaDescription)
	}
}

func TestAssistantProvenanceRecordsThePersona(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})

	settings, _ := server.store.Snapshot()
	resolved, err := server.loadInteractiveChatPromptContext(payload.ActiveSessionID, settings.LLM)
	if err != nil {
		t.Fatalf("load prompt context: %v", err)
	}
	// This is what the transcript derives a persona divider from, which is why no
	// new message role is needed for a mid-conversation switch.
	if resolved.Persona == nil || resolved.Persona.ID != created.ID {
		t.Fatal("the resolved context does not carry the persona for provenance")
	}
}

func TestANewSessionInheritsTheLastUsedPersona(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})

	session, err := server.chatLog.CreateSession(true)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	// Starting a fresh chat and finding the assistant reverted to a stranger would
	// make the persona feel forgotten, so continuity is the default.
	if session.PersonaID != created.ID {
		t.Fatalf("new session persona = %q, want %q", session.PersonaID, created.ID)
	}
}

func TestPersonaWritesRequireTheController(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/personas"},
		{http.MethodPatch, "/api/personas/" + created.ID},
		{http.MethodDelete, "/api/personas/" + created.ID},
		{http.MethodPost, "/api/personas/" + created.ID + "/duplicate"},
		{http.MethodPost, "/api/personas/" + created.ID + "/portrait"},
		{http.MethodDelete, "/api/personas/" + created.ID + "/portrait"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader("{}"))
		request.Header.Set(controllerHeaderName, "someone-else")
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("%s %s: status %d, want 409 for a non-controller",
				testCase.method, testCase.path, recorder.Code)
		}
	}
}

// The end-to-end claim: binding a persona changes the bytes the model receives.
// Every other test proves a piece of the path; this one proves the path.
func TestABoundPersonaChangesTheComposedPrompt(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.LLM.ChatVoice = config.LLMChatVoiceUtility
		return settings
	})
	created := createPersonaVia(t, server, "Rowan")
	personaRequest(t, server, http.MethodPatch, "/api/personas/"+created.ID, map[string]any{
		"chat_voice":     config.LLMChatVoiceIntimate,
		"reaction_style": config.LLMReactionStyleTender,
		"description":    "Steady and low-voiced.",
	})
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)
	sessionID := payload.ActiveSessionID

	settings, _ := server.store.Snapshot()
	promptSet, memories, _, err := server.resolveInteractiveChatPersonalization(settings.LLM.PromptSet)
	if err != nil {
		t.Fatalf("resolve personalization: %v", err)
	}
	compose := func() string {
		resolved, loadErr := server.loadInteractiveChatPromptContext(sessionID, settings.LLM)
		if loadErr != nil {
			t.Fatalf("load prompt context: %v", loadErr)
		}
		return chat.ComposeSystemForTest(promptSet, memories, resolved.Capabilities, resolved.ConversationContext)
	}

	before := compose()
	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+sessionID+"/persona",
		map[string]any{"persona_id": created.ID})
	after := compose()

	if before == after {
		t.Fatal("binding a persona did not change the composed prompt")
	}
	for _, fragment := range []string{
		"REACTION STYLE - TENDER",
		`"Rowan"`,
		"Steady and low-voiced.",
		"REPLY IDENTITY - INTIMATE PARTNER",
	} {
		if !strings.Contains(after, fragment) {
			t.Fatalf("composed prompt is missing %q:\n%s", fragment, after)
		}
	}
	// The utility register composes no style and no profile, so the baseline must
	// not have leaked any of it.
	if strings.Contains(before, "REACTION STYLE") || strings.Contains(before, "Rowan") {
		t.Fatal("the unbound baseline already contained persona content")
	}
	// And the contract still ends the prompt: recency is what small models weight.
	if styleAt, guardAt := strings.Index(after, "REACTION STYLE"), strings.LastIndex(after, "FINAL OUTPUT RULE"); styleAt > guardAt {
		t.Fatalf("the style block displaced the final guard: style %d guard %d", styleAt, guardAt)
	}

	// Clearing the selection restores the baseline exactly.
	personaRequest(t, server, http.MethodPut, "/api/chat/sessions/"+sessionID+"/persona",
		map[string]any{"persona_id": ""})
	if cleared := compose(); cleared != before {
		t.Fatal("clearing the persona did not restore the original prompt")
	}
}
