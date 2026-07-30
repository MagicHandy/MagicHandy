package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/modes"
)

func TestPersonaStorageFailureDoesNotFallBackToGlobalSettings(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	sessionID, err := server.chatLog.ActiveSessionID()
	if err != nil {
		t.Fatalf("active session: %v", err)
	}
	if err := server.store.Datastore().Close(); err != nil {
		t.Fatalf("close datastore fixture: %v", err)
	}

	if _, err := server.sessionPersona(sessionID); err == nil {
		t.Fatal("closed persona datastore silently resolved to the global persona")
	}
}

func TestClearingToDefaultPersistsForNewAndRestartCreatedSessions(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)

	personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})
	recorder, _ := personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": ""})
	if recorder.Code != http.StatusOK {
		t.Fatalf("clear persona: status %d body %s", recorder.Code, recorder.Body.String())
	}

	fresh, err := server.chatLog.CreateSession(true)
	if err != nil {
		t.Fatalf("create new session: %v", err)
	}
	if fresh.PersonaID != "" {
		t.Fatalf("new session resurrected persona %q after default was selected", fresh.PersonaID)
	}
	restarted, err := server.chatLog.ReconcileStartup("new", false)
	if err != nil {
		t.Fatalf("reconcile startup: %v", err)
	}
	if restarted.PersonaID != "" {
		t.Fatalf("restart-created session resurrected persona %q after default was selected", restarted.PersonaID)
	}
}

func TestPersonaSelectionIsLockedWhileAutopilotRuns(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	_, payload := personaRequest(t, server, http.MethodGet, "/api/personas", nil)

	if _, err := server.modes.Start(t.Context(), modes.ModeAutopilot); err != nil {
		t.Fatalf("start Autopilot: %v", err)
	}
	t.Cleanup(func() { server.modes.Stop("test cleanup") })

	recorder, _ := personaRequest(t, server, http.MethodPut,
		"/api/chat/sessions/"+payload.ActiveSessionID+"/persona",
		map[string]any{"persona_id": created.ID})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409; body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "stop Autopilot") {
		t.Fatalf("response did not explain the lock: %s", recorder.Body.String())
	}
	recorder, _ = personaRequest(t, server, http.MethodPatch,
		"/api/personas/"+created.ID, map[string]any{"name": "Changed in flight"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("persona edit status %d, want 409 while Autopilot runs", recorder.Code)
	}
	storedID, err := server.chatLog.SessionPersona(payload.ActiveSessionID)
	if err != nil {
		t.Fatalf("read session persona: %v", err)
	}
	if storedID != "" {
		t.Fatalf("persona changed to %q while Autopilot was active", storedID)
	}
}

func TestPersonaMutationIsLockedWhileAReplyRuns(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")

	server.chatCancelMu.Lock()
	if server.chatCancels == nil {
		server.chatCancels = make(map[uint64]context.CancelFunc)
	}
	server.chatCancels[1] = func() {}
	server.chatCancelMu.Unlock()
	t.Cleanup(server.cancelActiveChats)

	recorder, _ := personaRequest(t, server, http.MethodPatch,
		"/api/personas/"+created.ID, map[string]any{"name": "Changed in flight"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("persona edit status %d, want 409 while a reply runs", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "active reply") {
		t.Fatalf("response did not explain the lock: %s", recorder.Body.String())
	}
	stored, err := server.personas.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("read persona after rejected edit: %v", err)
	}
	if stored.Name != created.Name {
		t.Fatalf("persona changed to %q while a reply was active", stored.Name)
	}
}

func TestLoreCreateDoesNotDecodeTheCommittedRowForItsResponse(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(server.Close)
	created := createPersonaVia(t, server, "Rowan")
	if _, err := server.store.Datastore().SQL().Exec(`
		CREATE TRIGGER corrupt_lore_after_insert
		AFTER INSERT ON persona_lore
		BEGIN
			UPDATE persona_lore SET keywords_json = '{broken' WHERE id = NEW.id;
		END
	`); err != nil {
		t.Fatalf("install fault trigger: %v", err)
	}

	recorder, _ := personaRequest(t, server, http.MethodPost,
		"/api/personas/"+created.ID+"/lore",
		map[string]any{"text": "Blue velvet is familiar.", "keywords": []string{"velvet"}})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create lore status %d body %s", recorder.Code, recorder.Body.String())
	}

	recorder, _ = personaRequest(t, server, http.MethodGet,
		"/api/personas/"+created.ID+"/lore", nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("fault fixture did not break a later lore read: status %d", recorder.Code)
	}
}
