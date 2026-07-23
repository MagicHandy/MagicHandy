package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/voice"
)

// These tests cover the ADR 0003 delivery-ordering rules: the shared chat
// log with per-client cursors, lockstep chat-emit/TTS-enqueue
// (spoken-equals-shown), the single-owner audio lease, and the model-error
// path staying out of history and TTS.

func getChatMessages(t *testing.T, server *Server, clientID string) (messages []chat.LogMessage, latest int64, cursor int64) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/chat/messages", nil)
	if clientID != "" {
		request = withControllerID(request, clientID)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("chat messages status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Messages  []chat.LogMessage `json:"messages"`
		LatestSeq int64             `json:"latest_seq"`
		Cursor    int64             `json:"cursor"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chat messages: %v", err)
	}
	return payload.Messages, payload.LatestSeq, payload.Cursor
}

func TestChatStreamAppendsToSharedLog(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Logged reply.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"message":"log me"}`)
	if !strings.Contains(body, `"user_seq":`) {
		t.Fatalf("status event missing user_seq:\n%s", body)
	}
	if !strings.Contains(body, `"seq":`) {
		t.Fatalf("message event missing seq:\n%s", body)
	}

	messages, latest, _ := getChatMessages(t, server, "")
	if len(messages) != 2 {
		t.Fatalf("log length = %d, want 2: %+v", len(messages), messages)
	}
	if messages[0].Role != chat.MessageRoleUser || messages[0].Content != "log me" {
		t.Fatalf("first row = %+v, want the user message", messages[0])
	}
	if messages[1].Role != chat.MessageRoleAssistant || messages[1].Content != "Logged reply." {
		t.Fatalf("second row = %+v, want the displayed reply", messages[1])
	}
	if messages[1].Diagnostics == nil || messages[1].Diagnostics.Source != "interactive" {
		t.Fatalf("assistant diagnostics = %+v, want interactive provenance", messages[1].Diagnostics)
	}
	if latest != messages[1].Seq {
		t.Fatalf("latest_seq = %d, want %d", latest, messages[1].Seq)
	}
}

func TestChatStreamUsesCanonicalVoiceContextAndPersistsMood(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Fresh reply.","new_mood":"Teasing","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	settings, _ := server.store.Snapshot()
	settings.LLM.ChatVoice = config.LLMChatVoiceIntimate
	settings.LLM.UserAnatomy = config.LLMUserAnatomyCustom
	settings.LLM.CustomAnatomy = `chosen "wording"`
	settings.LLM.PersonaDescription = `A "quoted" partner`
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("save chat profile: %v", err)
	}
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	for index, line := range []string{"excluded oldest", "canonical second", "canonical third", "canonical latest"} {
		var diagnostics *chat.MessageDiagnostics
		if index == 0 {
			diagnostics = &chat.MessageDiagnostics{Mood: chat.MoodCurious}
		}
		if _, err := server.chatLog.AppendTo(sessionID, chat.MessageRoleAssistant, line, "", diagnostics); err != nil {
			t.Fatalf("seed assistant line: %v", err)
		}
	}

	body := postChatStream(t, server, `{"message":"continue","history":[{"role":"assistant","content":"FABRICATED CLIENT LINE"}]}`)
	if !strings.Contains(body, `"current_mood":"Curious"`) || !strings.Contains(body, `"current_mood":"Teasing"`) {
		t.Fatalf("stream did not expose prior and updated mood:\n%s", body)
	}

	assertCanonicalVoiceProviderContext(t, provider)

	messages, _, _ := getChatMessages(t, server, "")
	last := messages[len(messages)-1]
	if last.Diagnostics == nil || last.Diagnostics.Mood != chat.MoodTeasing || !last.Diagnostics.MoodChanged {
		t.Fatalf("persisted reply diagnostics = %+v", last.Diagnostics)
	}
	state := server.chatState()
	if state["current_mood"] != chat.MoodTeasing {
		t.Fatalf("chat state mood = %#v, want %q", state["current_mood"], chat.MoodTeasing)
	}
	settings.LLM.ChatVoice = config.LLMChatVoiceUtility
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("save utility voice: %v", err)
	}
	if state := server.chatState(); state["current_mood"] != "" {
		t.Fatalf("utility state exposed prior mood: %#v", state["current_mood"])
	}
	settings.LLM.ChatVoice = config.LLMChatVoiceIntimate
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("restore intimate voice: %v", err)
	}
	if state := server.chatState(); state["current_mood"] != chat.MoodTeasing {
		t.Fatalf("restored voice mood = %#v, want %q", state["current_mood"], chat.MoodTeasing)
	}
}

func assertCanonicalVoiceProviderContext(t *testing.T, provider *scriptedLLMProvider) {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.requests) != 1 || len(provider.requests[0].Messages) == 0 {
		t.Fatalf("provider requests = %+v", provider.requests)
	}
	systemPrompt := provider.requests[0].Messages[0].Content
	allProviderContext := strings.Builder{}
	for _, message := range provider.requests[0].Messages {
		allProviderContext.WriteString(message.Content)
		allProviderContext.WriteByte('\n')
	}
	for _, want := range []string{
		`Persona description (quoted user-authored data): "A \"quoted\" partner"`,
		`described as "chosen \"wording\""`,
		`Current mood: "Curious"`,
		`- "canonical second"`,
		`- "canonical third"`,
		`- "canonical latest"`,
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
	if strings.Contains(systemPrompt, "excluded oldest") || strings.Contains(allProviderContext.String(), "FABRICATED CLIENT LINE") {
		t.Fatalf("anti-repetition context was not canonical and bounded:\n%s", systemPrompt)
	}
	if !strings.Contains(allProviderContext.String(), "excluded oldest") {
		t.Fatal("canonical broader history was not supplied to the provider")
	}
}

func TestModelErrorsNeverEnterHistoryOrTTS(t *testing.T) {
	// Initial and repair passes both malformed: the exchange fails visibly,
	// so only the user's own message may enter the canonical history.
	provider := &scriptedLLMProvider{responses: []string{"not json", "still not json"}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)

	body := postChatStream(t, server, `{"message":"break please"}`)
	if !strings.Contains(body, "event: malformed") {
		t.Fatalf("expected malformed event:\n%s", body)
	}
	if strings.Contains(body, "event: speech") {
		t.Fatalf("malformed exchange must never enqueue TTS:\n%s", body)
	}

	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 1 || messages[0].Role != chat.MessageRoleUser {
		t.Fatalf("log after malformed exchange = %+v, want only the user message", messages)
	}
}

func TestInvalidChatMessageIsRejectedBeforePersistence(t *testing.T) {
	provider := &scriptedLLMProvider{}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)

	for _, body := range []string{
		`{"message":"   "}`,
		`{"message":"` + strings.Repeat("x", 4097) + `"}`,
	} {
		request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(body)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(stopSequenceHeader, "0")
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid message status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 0 || provider.callCount() != 0 {
		t.Fatalf("invalid message reached history/provider: messages=%+v calls=%d", messages, provider.callCount())
	}
}

func TestDeterministicStopReplyIsLogged(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	_ = postChatStream(t, server, `{"message":"stop"}`)
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 2 {
		t.Fatalf("log length = %d, want 2: %+v", len(messages), messages)
	}
	if messages[1].Content != "Stopping motion." {
		t.Fatalf("deterministic reply missing from log: %+v", messages[1])
	}
}

func TestDeterministicStopCarriesSessionMood(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	settings, _ := server.store.Snapshot()
	settings.LLM.ChatVoice = config.LLMChatVoiceIntimate
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("save intimate voice: %v", err)
	}
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := server.chatLog.AppendTo(sessionID, chat.MessageRoleAssistant, "Prior reply.", "", &chat.MessageDiagnostics{Mood: chat.MoodConfident}); err != nil {
		t.Fatalf("seed mood: %v", err)
	}

	_ = postChatStream(t, server, `{"message":"stop"}`)
	messages, _, _ := getChatMessages(t, server, "")
	last := messages[len(messages)-1]
	if last.Diagnostics == nil || last.Diagnostics.Mood != chat.MoodConfident {
		t.Fatalf("deterministic Stop diagnostics = %+v, want carried mood", last.Diagnostics)
	}
	if state := server.chatState(); state["current_mood"] != chat.MoodConfident {
		t.Fatalf("chat state mood = %#v, want %q", state["current_mood"], chat.MoodConfident)
	}
}

func TestDeterministicStopSuppressesMoodInUtilityVoice(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := server.chatLog.AppendTo(sessionID, chat.MessageRoleAssistant, "Prior reply.", "", &chat.MessageDiagnostics{
		Mood: chat.MoodConfident, MoodChanged: true,
	}); err != nil {
		t.Fatalf("seed mood: %v", err)
	}

	body := postChatStream(t, server, `{"message":"stop"}`)
	if strings.Contains(body, `"current_mood":"Confident"`) {
		t.Fatalf("utility Stop exposed prior mood: %s", body)
	}
	messages, _, _ := getChatMessages(t, server, "")
	last := messages[len(messages)-1]
	if last.Diagnostics == nil || last.Diagnostics.Mood != "" {
		t.Fatalf("utility Stop diagnostics exposed mood: %+v", last.Diagnostics)
	}
}

func TestDeterministicStopIgnoresUncommittedMood(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	settings, _ := server.store.Snapshot()
	settings.LLM.ChatVoice = config.LLMChatVoiceIntimate
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("save intimate voice: %v", err)
	}
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := server.chatLog.AppendTo(sessionID, chat.MessageRoleAssistant, "Committed mood.", "", &chat.MessageDiagnostics{
		Mood: chat.MoodCurious, MoodChanged: true,
	}); err != nil {
		t.Fatalf("seed committed mood: %v", err)
	}
	pending, err := server.chatLog.AppendPendingAssistantTo(sessionID, "Canceled mood.", &chat.MessageDiagnostics{
		Mood: chat.MoodTeasing, MoodChanged: true,
	})
	if err != nil {
		t.Fatalf("stage canceled mood: %v", err)
	}
	t.Cleanup(func() { _ = server.chatLog.Delete(pending) })

	_ = postChatStream(t, server, `{"message":"stop"}`)
	messages, _, _ := getChatMessages(t, server, "")
	last := messages[len(messages)-1]
	if last.Diagnostics == nil || last.Diagnostics.Mood != chat.MoodCurious {
		t.Fatalf("deterministic Stop copied uncommitted mood: %+v", last.Diagnostics)
	}
	context, err := server.chatLog.PromptContext(sessionID)
	if err != nil || context.CurrentMood != chat.MoodCurious {
		t.Fatalf("effective mood after Stop = %+v, err=%v", context, err)
	}
}

func TestChatCursorsAreIsolatedAndMonotonicOverHTTP(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	for index := 0; index < 5; index++ {
		if _, err := server.chatLog.Append(chat.MessageRoleUser, fmt.Sprintf("message %d", index), "seed"); err != nil {
			t.Fatalf("seed chat log: %v", err)
		}
	}

	advance := func(clientID string, seq int64) int64 {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/chat/cursor",
			strings.NewReader(fmt.Sprintf(`{"seq":%d}`, seq)))
		request = withControllerID(request, clientID)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("cursor status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var payload struct {
			Cursor int64 `json:"cursor"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode cursor: %v", err)
		}
		return payload.Cursor
	}

	if got := advance("tab-a", 5); got != 5 {
		t.Fatalf("cursor = %d, want 5", got)
	}
	// Another client's cursor is untouched: reads are never destructive.
	_, _, cursorB := getChatMessages(t, server, "tab-b")
	if cursorB != 0 {
		t.Fatalf("tab-b cursor = %d, want 0", cursorB)
	}
	// Cursors never move backward.
	if got := advance("tab-a", 3); got != 5 {
		t.Fatalf("cursor moved backward to %d", got)
	}

	// A cursor requires a client identity.
	request := httptest.NewRequest(http.MethodPost, "/api/chat/cursor", strings.NewReader(`{"seq":1}`))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("cursor without client id = %d, want 400", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/chat/cursor", strings.NewReader(`{"session_id":"missing","seq":1}`))
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withControllerID(request, "tab-a"))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cursor for missing session = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
}

// --- Lockstep TTS + audio lease (stub worker) -------------------------------

var (
	chatStubOnce sync.Once
	chatStubPath string
	chatStubErr  error
)

func chatStubBinary(t *testing.T) string {
	t.Helper()
	chatStubOnce.Do(func() {
		dir, err := os.MkdirTemp("", "httpapi-voice-stub")
		if err != nil {
			chatStubErr = err
			return
		}
		name := "voice-stub-worker"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		chatStubPath = filepath.Join(dir, name)
		_, file, _, _ := runtime.Caller(0)
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		// #nosec G204 -- test-only: builds the in-repo stub into a temp dir.
		build := exec.Command("go", "build", "-o", chatStubPath, "./cmd/voice-stub-worker")
		build.Dir = repoRoot
		if output, err := build.CombinedOutput(); err != nil {
			chatStubErr = fmt.Errorf("%v: %s", err, output)
		}
	})
	if chatStubErr != nil {
		t.Fatalf("build stub worker: %v", chatStubErr)
	}
	return chatStubPath
}

func startSpeakingTTS(t *testing.T, server *Server, speakReplies bool) {
	t.Helper()
	saveSettings(t, server.store, func(settings config.Settings) config.Settings {
		settings.Voice.Enabled = true
		settings.Voice.TTSProvider = config.VoiceProviderCustom
		settings.Voice.SpeakReplies = speakReplies
		settings.Voice.TTSWorkerPath = chatStubBinary(t)
		settings.Voice.TTSWorkerArgs = []string{"-role", "tts", "-start-loaded"}
		return settings
	})
	settings, _ := server.store.Snapshot()
	server.applyVoiceSettingsTransition(settings)

	recorder := httptest.NewRecorder()
	request := withController(httptest.NewRequest(http.MethodPost, "/api/voice/workers/tts/start", nil))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("start tts worker = %d: %s", recorder.Code, recorder.Body.String())
	}
}

var speechEventPattern = regexp.MustCompile(`event: speech\ndata: \{"request_id":"([^"]+)"\}`)

func TestSpokenReplyAlwaysMatchesDisplayedReply(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Spoken and shown.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	startSpeakingTTS(t, server, true)

	body := postChatStream(t, server, `{"message":"say something"}`)
	match := speechEventPattern.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("chat stream missing speech event:\n%s", body)
	}
	requestID := match[1]

	// Ordering: the displayed reply is in the shared log...
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 2 || messages[1].Content != "Spoken and shown." {
		t.Fatalf("displayed reply missing from log: %+v", messages)
	}
	// ...and the enqueued TTS text is exactly that reply (lockstep).
	pending, ok := server.voice.Request(requestID)
	if !ok {
		t.Fatalf("speech request %q is not tracked", requestID)
	}
	if pending.Text() != messages[1].Content {
		t.Fatalf("spoken text %q != displayed text %q", pending.Text(), messages[1].Content)
	}

	// The stub completes and audio is retained for the lease owner.
	deadline := time.Now().Add(5 * time.Second)
	for pending.Snapshot().State != voice.RequestStateDone {
		if time.Now().After(deadline) {
			t.Fatalf("speech request never completed: %+v", pending.Snapshot())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Single-owner audio lease: the active controller fetches the clip...
	audioPath := "/api/voice/requests/" + requestID + "/audio"
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withController(httptest.NewRequest(http.MethodGet, audioPath, nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("controller audio fetch = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/wav" {
		t.Fatalf("audio content type = %q, want audio/wav", got)
	}
	if recorder.Body.Len() == 0 {
		t.Fatal("audio body is empty")
	}

	// ...and any other client is refused, so two tabs never speak one clip.
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, withControllerID(httptest.NewRequest(http.MethodGet, audioPath, nil), "other-tab"))
	if recorder.Code == http.StatusOK {
		t.Fatal("non-controller client must not fetch audio")
	}
}

func TestSpeakRepliesOffMeansNoTTSEnqueue(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Quiet reply.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)
	startSpeakingTTS(t, server, false)

	body := postChatStream(t, server, `{"message":"hush"}`)
	if strings.Contains(body, "event: speech") {
		t.Fatalf("speak-replies off must not enqueue TTS:\n%s", body)
	}
	messages, _, _ := getChatMessages(t, server, "")
	if len(messages) != 2 {
		t.Fatalf("reply must still be displayed/logged: %+v", messages)
	}
}

func TestChatLogStorageFailureIsExplicitAndRedacted(t *testing.T) {
	server := newTestServer(t)
	if _, err := server.store.Datastore().SQL().Exec(`DROP TABLE messages`); err != nil {
		t.Fatalf("remove chat log table: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/chat/messages", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Body.String(); !strings.Contains(got, "chat history storage is unavailable") || strings.Contains(got, "closed") {
		t.Fatalf("chat history response exposed storage details: %s", got)
	}
	state := server.chatState()
	if available, ok := state["available"].(bool); !ok || available {
		t.Fatalf("chat state availability = %#v, want false", state["available"])
	}
}

func TestChatSessionHTTPFlowKeepsOneActiveConversation(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	list := func() struct {
		ActiveID string         `json:"active_session_id"`
		Sessions []chat.Session `json:"sessions"`
	} {
		t.Helper()
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/chat/sessions", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("list sessions = %d: %s", recorder.Code, recorder.Body.String())
		}
		var payload struct {
			ActiveID string         `json:"active_session_id"`
			Sessions []chat.Session `json:"sessions"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode sessions: %v", err)
		}
		return payload
	}

	initial := list()
	if len(initial.Sessions) != 1 || !initial.Sessions[0].Active {
		t.Fatalf("initial sessions = %+v", initial)
	}
	if _, err := server.chatLog.Append(chat.MessageRoleUser, "keep this conversation", "client"); err != nil {
		t.Fatalf("append initial chat: %v", err)
	}

	rejected := httptest.NewRecorder()
	server.Handler().ServeHTTP(rejected, withController(httptest.NewRequest(
		http.MethodPost,
		"/api/chat/sessions",
		strings.NewReader(`{"discard_current_unsaved":false}`),
	)))
	if rejected.Code != http.StatusConflict || !strings.Contains(rejected.Body.String(), "save or discard") {
		t.Fatalf("unresolved draft create = %d: %s", rejected.Code, rejected.Body.String())
	}

	save := httptest.NewRecorder()
	server.Handler().ServeHTTP(save, withController(httptest.NewRequest(
		http.MethodPut,
		"/api/chat/sessions/"+initial.ActiveID+"/save",
		strings.NewReader(`{}`),
	)))
	if save.Code != http.StatusOK {
		t.Fatalf("save session = %d: %s", save.Code, save.Body.String())
	}

	create := httptest.NewRecorder()
	server.Handler().ServeHTTP(create, withController(httptest.NewRequest(
		http.MethodPost,
		"/api/chat/sessions",
		strings.NewReader(`{"discard_current_unsaved":false}`),
	)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create session = %d: %s", create.Code, create.Body.String())
	}
	created := list()
	if len(created.Sessions) != 2 || created.ActiveID == initial.ActiveID {
		t.Fatalf("created sessions = %+v", created)
	}

	activate := httptest.NewRecorder()
	server.Handler().ServeHTTP(activate, withController(httptest.NewRequest(
		http.MethodPut,
		"/api/chat/sessions/"+initial.ActiveID+"/active?discard_current_unsaved=true",
		strings.NewReader(`{}`),
	)))
	if activate.Code != http.StatusOK {
		t.Fatalf("activate session = %d: %s", activate.Code, activate.Body.String())
	}
	active := list()
	if active.ActiveID != initial.ActiveID {
		t.Fatalf("active session = %q, want %q", active.ActiveID, initial.ActiveID)
	}
	if len(active.Sessions) != 1 {
		t.Fatalf("discarded working session remains in list: %+v", active.Sessions)
	}
	activeCount := 0
	for _, session := range active.Sessions {
		if session.Active {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("active session count = %d, want 1", activeCount)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/chat/messages?session_id="+initial.ActiveID, nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "keep this conversation") {
		t.Fatalf("session messages = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestChatStreamRejectsAStaleSessionSelection(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	request := withController(httptest.NewRequest(
		http.MethodPost,
		"/api/chat/stream",
		strings.NewReader(`{"session_id":"not-active","message":"hello"}`),
	))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale chat stream = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "no longer active") {
		t.Fatalf("stale session response = %s", recorder.Body.String())
	}
}

func TestServerCloseAppliesTheDefaultUnsavedChatPolicy(t *testing.T) {
	server := newTestServer(t)
	dataDir := server.store.DataDir()
	discardedID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if _, err := server.chatLog.Append(chat.MessageRoleUser, "discard at shutdown", "client"); err != nil {
		t.Fatalf("append draft: %v", err)
	}

	server.Close()
	reopened, err := chat.OpenMessageLog(dataDir)
	if err != nil {
		t.Fatalf("reopen chat log: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err := reopened.Session(discardedID); !errors.Is(err, chat.ErrChatSessionNotFound) {
		t.Fatalf("shutdown draft error = %v, want not found", err)
	}
}
