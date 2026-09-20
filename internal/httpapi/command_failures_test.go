package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDelayedControlRoutesCannotApplyAfterStop(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	var delayed []*http.Request
	for _, route := range []string{"/api/motion/start", "/api/motion/target", "/api/motion/resume", "/api/modes/start", "/api/modes/stop", "/api/media/sync"} {
		delayed = append(delayed, commandTestRequest(s, cookie, route, `{}`, "delayed-command", 1))
	}
	if w := serveCommand(s, commandTestRequest(s, cookie, "/api/motion/stop", `{}`, "global-stop", 2)); w.Code != http.StatusOK {
		t.Fatalf("Stop: %d %s", w.Code, w.Body.String())
	}
	for _, request := range delayed {
		if w := serveCommand(s, request); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "controller state changed") {
			t.Fatalf("%s bypassed Stop generation: %d %s", request.URL.Path, w.Code, w.Body.String())
		}
	}
}

func TestPreviousGenerationReceiptIsReadableButCannotMasqueradeAsANewCommand(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	for _, operation := range []struct{ route, body, id string }{
		{"/api/motion/quick", `{"speed_max_percent":32}`, "before-stop"},
		{"/api/motion/stop", `{}`, "global-stop"},
	} {
		if w := serveCommand(s, commandTestRequest(s, cookie, operation.route, operation.body, operation.id, 1)); w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", operation.route, w.Code, w.Body.String())
		}
	}
	if w := serveCommand(s, commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":32}`, "before-stop", 1)); w.Code != http.StatusConflict {
		t.Fatalf("old receipt acknowledged as a new-generation command: %d %s", w.Code, w.Body.String())
	}
	if w := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/controller/commands/before-stop", "test-controller", ""); w.Code != http.StatusOK {
		t.Fatal("originating session cannot reconcile its pre-Stop result")
	}
}

func TestInterruptedHandlerCannotLeaveASuccessfulReceipt(t *testing.T) {
	s, store, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	r := commandTestRequest(s, cookie, "/api/motion/quick", "", "interrupted-command", 1)
	session, err := store.InspectSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	r = r.WithContext(context.WithValue(r.Context(), authenticatedSessionContextKey{}, authenticatedSessionState{session: session}))
	handler := s.trackCommandDelivery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireController(w, r) {
			t.Fatal("test request was not admitted")
		}
		writeJSON(w, http.StatusOK, map[string]bool{"staged": true})
		panic("interrupted after a response write")
	}))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("tracking swallowed the handler panic")
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}()
	receipt := s.commands.lookup(commandReceiptKey{session: session.Key, client: "test-controller", id: "interrupted-command"})
	if receipt == nil || receipt.State != "unknown" || receipt.Replayable || receipt.HTTPStatus != 0 || len(receipt.Response) != 0 {
		t.Fatalf("interrupted command advertised success: %+v", receipt)
	}
}
