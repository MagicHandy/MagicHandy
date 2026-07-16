package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/memory"
)

func personalizationRequest(t *testing.T, server *Server, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	// Match postChatStream's client ID so one test client owns the lease.
	request.Header.Set("X-MagicHandy-Client-ID", "test-controller")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestMemoryEndpointsRoundTrip(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	added := personalizationRequest(t, server, http.MethodPost, "/api/memory", `{"text":"Likes slow starts."}`)
	if added.Code != http.StatusOK {
		t.Fatalf("add status = %d: %s", added.Code, added.Body.String())
	}
	var snapshot memory.Snapshot
	if err := json.Unmarshal(added.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snapshot.Memories) != 1 || !snapshot.Enabled {
		t.Fatalf("snapshot after add = %+v", snapshot)
	}
	id := snapshot.Memories[0].ID

	toggled := personalizationRequest(t, server, http.MethodPatch, "/api/memory/"+id, `{"enabled":false}`)
	if toggled.Code != http.StatusOK || !strings.Contains(toggled.Body.String(), `"enabled":false`) {
		t.Fatalf("toggle = %d: %s", toggled.Code, toggled.Body.String())
	}

	globalOff := personalizationRequest(t, server, http.MethodPost, "/api/memory/enabled", `{"enabled":false}`)
	if globalOff.Code != http.StatusOK {
		t.Fatalf("global off = %d: %s", globalOff.Code, globalOff.Body.String())
	}

	removed := personalizationRequest(t, server, http.MethodDelete, "/api/memory/"+id, "")
	if removed.Code != http.StatusOK {
		t.Fatalf("remove = %d: %s", removed.Code, removed.Body.String())
	}
	missing := personalizationRequest(t, server, http.MethodDelete, "/api/memory/"+id, "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("remove missing = %d, want 404", missing.Code)
	}

	cleared := personalizationRequest(t, server, http.MethodPost, "/api/memory/clear", "")
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear = %d: %s", cleared.Code, cleared.Body.String())
	}
}

func TestChatSystemPromptIncludesMemoriesOnlyWhenEnabled(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Hi.","motion":{"action":"none"}}`,
		`{"reply":"Hi again.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)

	added := personalizationRequest(t, server, http.MethodPost, "/api/memory", `{"text":"Prefers the tease pattern."}`)
	if added.Code != http.StatusOK {
		t.Fatalf("add memory = %d: %s", added.Code, added.Body.String())
	}

	postChatStream(t, server, `{"message":"hello"}`)
	if provider.callCount() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.callCount())
	}
	system := provider.requests[0].Messages[0].Content
	if !strings.Contains(system, "Prefers the tease pattern.") {
		t.Fatalf("system prompt missing enabled memory:\n%s", system)
	}
	if !strings.Contains(system, "Choose one valid base shape") {
		t.Fatalf("system prompt missing code-owned contract:\n%s", system)
	}

	// Chat keeps working identically with the memory switch off: same contract,
	// no memory block.
	off := personalizationRequest(t, server, http.MethodPost, "/api/memory/enabled", `{"enabled":false}`)
	if off.Code != http.StatusOK {
		t.Fatalf("disable memory = %d: %s", off.Code, off.Body.String())
	}
	body := postChatStream(t, server, `{"message":"hello again"}`)
	if !strings.Contains(body, `"reply":"Hi again."`) {
		t.Fatalf("chat with memory disabled did not complete:\n%s", body)
	}
	system = provider.requests[1].Messages[0].Content
	if strings.Contains(system, "Prefers the tease pattern.") || strings.Contains(system, "Saved user memories") {
		t.Fatalf("system prompt leaked memories while disabled:\n%s", system)
	}
	if !strings.Contains(system, "Choose one valid base shape") {
		t.Fatalf("contract missing with memory disabled:\n%s", system)
	}
}

func TestPromptSetCRUDProtectsBuiltinsAndResetsSelection(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	// Built-ins are read-only.
	editBuiltin := personalizationRequest(t, server, http.MethodPut,
		"/api/prompt-sets/"+chat.DefaultPromptSetID, `{"name":"Hack","system":"Rewritten."}`)
	if editBuiltin.Code != http.StatusForbidden {
		t.Fatalf("edit builtin = %d, want 403: %s", editBuiltin.Code, editBuiltin.Body.String())
	}
	deleteBuiltin := personalizationRequest(t, server, http.MethodDelete,
		"/api/prompt-sets/"+chat.DefaultPromptSetID, "")
	if deleteBuiltin.Code != http.StatusForbidden {
		t.Fatalf("delete builtin = %d, want 403", deleteBuiltin.Code)
	}

	created := personalizationRequest(t, server, http.MethodPost, "/api/prompt-sets",
		`{"name":"Gentle","system":"Be gentle."}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var payload struct {
		Set  chat.PromptSet   `json:"set"`
		Sets []chat.PromptSet `json:"sets"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create payload: %v", err)
	}
	if payload.Set.ID == "" || payload.Set.Builtin {
		t.Fatalf("created set = %+v", payload.Set)
	}

	// Select the new set through the normal settings path.
	settings, _ := server.store.Snapshot()
	settings.LLM.PromptSet = payload.Set.ID
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("select user set: %v", err)
	}

	updated := personalizationRequest(t, server, http.MethodPut,
		"/api/prompt-sets/"+payload.Set.ID, `{"name":"Gentler","system":"Be gentler."}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", updated.Code, updated.Body.String())
	}

	// Deleting the selected set resets the selection to the default.
	deleted := personalizationRequest(t, server, http.MethodDelete,
		"/api/prompt-sets/"+payload.Set.ID, "")
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", deleted.Code, deleted.Body.String())
	}
	if !strings.Contains(deleted.Body.String(), fmt.Sprintf(`"selected":%q`, chat.DefaultPromptSetID)) {
		t.Fatalf("delete payload did not reset selection: %s", deleted.Body.String())
	}
	settings, _ = server.store.Snapshot()
	if settings.LLM.PromptSet != chat.DefaultPromptSetID {
		t.Fatalf("selection after delete = %q, want default", settings.LLM.PromptSet)
	}
}

func TestChatFallsBackToDefaultPromptSetWhenSelectionMissing(t *testing.T) {
	provider := &scriptedLLMProvider{responses: []string{
		`{"reply":"Still chatting.","motion":{"action":"none"}}`,
	}}
	server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
	t.Cleanup(server.Close)

	settings, _ := server.store.Snapshot()
	settings.LLM.PromptSet = "user-deleted-long-ago"
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("save dangling selection: %v", err)
	}

	body := postChatStream(t, server, `{"message":"hello"}`)
	if !strings.Contains(body, `"reply":"Still chatting."`) {
		t.Fatalf("chat with dangling prompt selection failed:\n%s", body)
	}
	if !strings.Contains(body, fmt.Sprintf(`"prompt_set":%q`, chat.DefaultPromptSetID)) {
		t.Fatalf("status event did not report the fallback set:\n%s", body)
	}
}

func TestReadOnlyClientCannotEditPersonalization(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	// First client takes the controller lease.
	_ = controllerFromState(t, server, "controller-a")

	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/memory",
		strings.NewReader(`{"text":"Sneaky write."}`)), "reader-b")
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("read-only memory add = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = withControllerID(httptest.NewRequest(http.MethodPost, "/api/prompt-sets",
		strings.NewReader(`{"name":"X","system":"Y"}`)), "reader-b")
	request.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("read-only prompt create = %d, want %d", recorder.Code, http.StatusConflict)
	}
}

func TestMotionPauseResumeRoundTrip(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	started := personalizationRequest(t, server, http.MethodPost, "/api/motion/start", `{"speed_percent":30}`)
	if started.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", started.Code, started.Body.String())
	}

	paused := personalizationRequest(t, server, http.MethodPost, "/api/motion/pause", "")
	if paused.Code != http.StatusOK {
		t.Fatalf("pause = %d: %s", paused.Code, paused.Body.String())
	}
	if !strings.Contains(paused.Body.String(), `"paused":true`) {
		t.Fatalf("pause payload = %s, want paused state", paused.Body.String())
	}

	resumed := personalizationRequest(t, server, http.MethodPost, "/api/motion/resume", "")
	if resumed.Code != http.StatusOK {
		t.Fatalf("resume = %d: %s", resumed.Code, resumed.Body.String())
	}
	if !strings.Contains(resumed.Body.String(), `"running":true`) {
		t.Fatalf("resume payload = %s, want running state", resumed.Body.String())
	}

	stopped := personalizationRequest(t, server, http.MethodPost, "/api/motion/stop", "")
	if stopped.Code != http.StatusOK {
		t.Fatalf("stop = %d: %s", stopped.Code, stopped.Body.String())
	}
	if !strings.Contains(stopped.Body.String(), `"running_ms":0`) {
		t.Fatalf("stop payload = %s, want reset run clock", stopped.Body.String())
	}

	// Read-only clients cannot pause (control action), matching start/target.
	recorder := httptest.NewRecorder()
	request := withControllerID(httptest.NewRequest(http.MethodPost, "/api/motion/pause", nil), "reader-b")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("read-only pause = %d, want %d", recorder.Code, http.StatusConflict)
	}
}

func TestSettingsResetRestoresDefaults(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)

	addedMemory := personalizationRequest(t, server, http.MethodPost, "/api/memory", `{"text":"Survives settings reset."}`)
	if addedMemory.Code != http.StatusOK {
		t.Fatalf("add memory = %d: %s", addedMemory.Code, addedMemory.Body.String())
	}
	createdPrompt := personalizationRequest(t, server, http.MethodPost, "/api/prompt-sets",
		`{"name":"Reset survivor","system":"Still available after settings reset."}`)
	if createdPrompt.Code != http.StatusOK {
		t.Fatalf("create prompt set = %d: %s", createdPrompt.Code, createdPrompt.Body.String())
	}
	var promptPayload struct {
		Set chat.PromptSet `json:"set"`
	}
	if err := json.Unmarshal(createdPrompt.Body.Bytes(), &promptPayload); err != nil {
		t.Fatalf("decode created prompt set: %v", err)
	}

	settings, _ := server.store.Snapshot()
	settings.Motion.SpeedMaxPercent = 55
	settings.Device.HandyConnectionKey = "secret-key-value"
	settings.LLM.PromptSet = promptPayload.Set.ID
	if _, err := server.store.Save(settings); err != nil {
		t.Fatalf("save modified settings: %v", err)
	}

	recorder := personalizationRequest(t, server, http.MethodPost, "/api/settings/reset", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret-key-value") {
		t.Fatalf("reset response leaked the connection key: %s", recorder.Body.String())
	}

	after, _ := server.store.Snapshot()
	defaults := config.DefaultSettings()
	if after.Motion.SpeedMaxPercent != defaults.Motion.SpeedMaxPercent {
		t.Fatalf("speed max after reset = %d, want default %d",
			after.Motion.SpeedMaxPercent, defaults.Motion.SpeedMaxPercent)
	}
	if after.Device.HandyConnectionKey != "" {
		t.Fatal("connection key survived the reset")
	}
	if after.LLM.PromptSet != defaults.LLM.PromptSet {
		t.Fatalf("prompt set selection after reset = %q, want default %q", after.LLM.PromptSet, defaults.LLM.PromptSet)
	}

	memorySnapshot, err := server.personalization.memory.Snapshot()
	if err != nil {
		t.Fatalf("read memory after settings reset: %v", err)
	}
	if len(memorySnapshot.Memories) != 1 || memorySnapshot.Memories[0].Text != "Survives settings reset." {
		t.Fatalf("memory rows after settings reset = %+v, want pre-reset memory intact", memorySnapshot.Memories)
	}
	if set, ok, err := server.personalization.prompts.Resolve(promptPayload.Set.ID); err != nil || !ok || set.Name != "Reset survivor" {
		t.Fatalf("prompt set after settings reset = %+v ok=%v, want pre-reset prompt set intact", set, ok)
	}
}

func TestPersonalizationStorageFailuresAreNotReportedAsEmptyState(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		provider := &scriptedLLMProvider{responses: []string{
			`{"reply":"Must not run.","motion":{"action":"none"}}`,
		}}
		server := newTestServerWithRuntime(t, Runtime{LLMProvider: provider})
		t.Cleanup(server.Close)
		if err := server.personalization.memory.Close(); err != nil {
			t.Fatal(err)
		}

		read := personalizationRequest(t, server, http.MethodGet, "/api/memory", "")
		if read.Code != http.StatusInternalServerError ||
			!strings.Contains(read.Body.String(), "memory storage is unavailable") {
			t.Fatalf("memory read = %d: %s", read.Code, read.Body.String())
		}

		state := personalizationRequest(t, server, http.MethodGet, "/api/state", "")
		if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), `"memory":{"available":false`) {
			t.Fatalf("state with failed memory store = %d: %s", state.Code, state.Body.String())
		}

		request := withController(httptest.NewRequest(
			http.MethodPost,
			"/api/chat/stream",
			strings.NewReader(`{"message":"hello"}`),
		))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(stopSequenceHeader, strconv.FormatUint(server.stopSequence.Load(), 10))
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError || provider.callCount() != 0 {
			t.Fatalf("chat with failed memory store = %d, provider calls = %d: %s",
				recorder.Code, provider.callCount(), recorder.Body.String())
		}
		if contentType := recorder.Header().Get("Content-Type"); strings.Contains(contentType, "text/event-stream") {
			t.Fatalf("chat committed SSE headers before reading memory: %q", contentType)
		}
	})

	t.Run("prompt sets", func(t *testing.T) {
		server := newTestServer(t)
		t.Cleanup(server.Close)
		if err := server.personalization.prompts.Close(); err != nil {
			t.Fatal(err)
		}

		read := personalizationRequest(t, server, http.MethodGet, "/api/prompt-sets", "")
		if read.Code != http.StatusInternalServerError ||
			!strings.Contains(read.Body.String(), "prompt set storage is unavailable") {
			t.Fatalf("prompt set read = %d: %s", read.Code, read.Body.String())
		}
	})
}
