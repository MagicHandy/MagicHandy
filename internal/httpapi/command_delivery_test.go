package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func commandTestRequest(s *Server, cookie *http.Cookie, route, body, id string, sequence uint64) *http.Request {
	r := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(controllerHeaderName, "test-controller")
	s.controller.mu.Lock()
	r.Header.Set(controllerEpochHeader, s.controller.epoch)
	r.Header.Set(controllerGenerationHeader, strconv.FormatUint(s.controller.generation, 10))
	if len(s.controller.tickets) > 0 {
		r.Header.Set(commandTicketHeader, s.controller.tickets[len(s.controller.tickets)-1].value)
	}
	s.controller.mu.Unlock()
	r.Header.Set(commandIDHeader, id)
	r.Header.Set(commandSequenceHeader, strconv.FormatUint(sequence, 10))
	r.AddCookie(cookie)
	return r
}

func serveCommand(s *Server, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestCommandReplayCannotReapplyAnOldSetting(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	for _, change := range []struct {
		id       string
		sequence uint64
		speed    int
	}{{"first-command", 1, 31}, {"second-command", 2, 42}} {
		w := serveCommand(s, commandTestRequest(s, cookie, "/api/motion/quick", fmt.Sprintf(`{"speed_max_percent":%d}`, change.speed), change.id, change.sequence))
		if w.Code != http.StatusOK {
			t.Fatalf("command %s: %d %s", change.id, w.Code, w.Body.String())
		}
	}
	w := serveCommand(s, commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":31}`, "first-command", 1))
	if w.Code != http.StatusOK || w.Header().Get("X-MagicHandy-Command-Replayed") != "true" {
		t.Fatalf("replay: %d %s", w.Code, w.Body.String())
	}
	current, _ := s.store.Snapshot()
	if current.Motion.SpeedMaxPercent != 42 {
		t.Fatal("duplicate request reapplied obsolete settings")
	}
	for _, request := range []*http.Request{
		commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":53}`, "first-command", 3),
		commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":53}`, "delayed-command", 1),
	} {
		if w := serveCommand(s, request); w.Code != http.StatusConflict {
			t.Fatalf("old/reused command accepted: %d %s", w.Code, w.Body.String())
		}
	}
	current, _ = s.store.Snapshot()
	if current.Motion.SpeedMaxPercent != 42 {
		t.Fatal("reordered update overwrote the newest accepted setting")
	}
}

func TestCommandReceiptIsBoundToOriginatingSessionAndTab(t *testing.T) {
	s, store, admin, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	w := serveCommand(s, commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":37}`, "lost-response", 1))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	w = authenticatedControlRequest(s, cookie, http.MethodGet, "/api/controller/commands/lost-response", "test-controller", "")
	var receipt commandReceipt
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.State != "complete" || !receipt.Replayable || receipt.HTTPStatus != http.StatusOK || !strings.Contains(string(receipt.Response), `"speed_max_percent":37`) {
		t.Fatalf("missing durable outcome of request handling: %s", w.Body.String())
	}
	token, _, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []struct {
		cookie *http.Cookie
		tab    string
	}{{testSessionCookie(token), "test-controller"}, {cookie, "different-tab"}} {
		w := authenticatedControlRequest(s, actor.cookie, http.MethodGet, "/api/controller/commands/lost-response", actor.tab, "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("receipt leaked across identity: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestExpiredDeliveryTicketCannotUseALiveControllerLease(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	r := commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":38}`, "delayed-ticket", 1)
	s.controller.mu.Lock()
	for i := range s.controller.tickets {
		s.controller.tickets[i].expires = time.Now().Add(-time.Second)
	}
	s.controller.mu.Unlock()
	w := serveCommand(s, r)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "ticket expired") {
		t.Fatalf("expired delivery admitted: %d %s", w.Code, w.Body.String())
	}
	current, _ := s.store.Snapshot()
	if current.Motion.SpeedMaxPercent == 38 {
		t.Fatal("expired command changed settings")
	}
}

func TestStopCancelsQueuedSettingBeforeDurableCommit(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, cookie)
	before, _ := s.store.Snapshot()
	s.settingsLifecycleMu.Lock()
	unlocked := false
	defer func() {
		if !unlocked {
			s.settingsLifecycleMu.Unlock()
		}
	}()
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- serveCommand(s, commandTestRequest(s, cookie, "/api/motion/quick", `{"speed_max_percent":39}`, "waiting-setting", 1))
	}()
	deadline := time.Now().Add(2 * time.Second)
	admitted := false
	for time.Now().Before(deadline) {
		s.commands.mu.Lock()
		admitted = len(s.commands.receipts) == 1
		s.commands.mu.Unlock()
		if admitted {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !admitted {
		t.Fatal("setting request was not admitted")
	}
	stop := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/motion/stop", "test-controller", `{}`)
	if stop.Code != http.StatusOK {
		t.Fatalf("Stop waited on the settings lane: %d %s", stop.Code, stop.Body.String())
	}
	s.settingsLifecycleMu.Unlock()
	unlocked = true
	select {
	case result := <-response:
		if result.Code == http.StatusOK {
			t.Fatal("canceled setting was acknowledged as saved")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled setting did not finish")
	}
	after, _ := s.store.Snapshot()
	if before.Motion != after.Motion {
		t.Fatalf("stopped request committed a late change: %+v", after.Motion)
	}
}

func TestOwnerCancellationIsSynchronousAndOriginCanDetach(t *testing.T) {
	controller := newControllerRuntime()
	actor := controllerIdentity{clientID: "tab", sessionKey: "session"}
	controller.Heartbeat(actor, 0)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	origin, cancelOrigin := context.WithCancel(t.Context())
	defer cancelOrigin()
	controller.bindRequest(actor, cancel)
	detach := controller.bindRequest(actor, cancelOrigin)
	if detach == nil || !detach() {
		t.Fatal("origin was not registered")
	}
	controller.AdvanceStopGeneration()
	if ctx.Err() == nil {
		t.Fatal("Stop returned before old request authority was canceled")
	}
	if origin.Err() != nil {
		t.Fatal("Stop canceled its own acknowledgment")
	}
}

func TestCommandReceiptMemoryAndLifetimeAreBounded(t *testing.T) {
	runtime := newCommandRuntime()
	now := time.Now()
	runtime.clock = func() time.Time { return now }
	scope := commandScope{actor: controllerIdentity{sessionKey: "session", clientID: "tab"}, generation: 1}
	for sequence := uint64(1); sequence <= maxCommandReceipts+10; sequence++ {
		id := fmt.Sprintf("receipt-%d", sequence)
		receipt := &commandReceipt{key: commandReceiptKey{session: "session", client: "tab", id: id}, Sequence: sequence}
		if err := runtime.begin(scope, receipt); err != nil {
			t.Fatal(err)
		}
		runtime.finish(receipt, &commandResponse{status: http.StatusNoContent}, newCommandBody(httptest.NewRequest(http.MethodPost, "/", nil)))
		now = now.Add(time.Millisecond)
	}
	if len(runtime.receipts) > maxCommandReceipts {
		t.Fatal("receipt memory grew beyond the limit")
	}
	if err := runtime.begin(scope, &commandReceipt{key: commandReceiptKey{id: "evicted-replay"}, Sequence: 1}); err == nil {
		t.Fatal("eviction allowed an old command sequence to execute again")
	}
	now = now.Add(commandReceiptLifetime)
	if runtime.lookup(commandReceiptKey{session: "session", client: "tab", id: "receipt-266"}) != nil || len(runtime.receipts) != 0 {
		t.Fatal("expired responses were retained")
	}
}
