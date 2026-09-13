package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/audit"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

func TestAccessAuditAttributesControlWithoutDuplicatingReplayedCommands(t *testing.T) {
	s, store, admin, adminCookie := newControllerSessionFixture(t)
	operator := newAdmissionIdentity(t, store, admin.ID, "operator", true)
	controller := claimAuthenticatedController(t, s, operator.cookie)
	const requestID = "private-command-identifier-fixture"
	for i := 0; i < 2; i++ {
		w := authenticatedControlRequest(s, operator.cookie, http.MethodPost, "/api/motion/quick", "test-controller", `{"speed_min_percent":19}`, func(r *http.Request) { r.Header.Set(commandIDHeader, requestID) })
		if w.Code != http.StatusOK {
			t.Fatalf("quick result %d %s", w.Code, w.Body.String())
		}
		if i == 1 && w.Header().Get("X-MagicHandy-Command-Replayed") != "true" {
			t.Fatal("fixture did not replay the original receipt")
		}
	}
	s.accessAudit.Close()
	w := authenticatedControlRequest(s, adminCookie, http.MethodGet, "/api/audit/export", "audit-observer", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("audit download %d %s", w.Code, w.Body.String())
	}
	if len(w.Body.Bytes()) > audit.PageMaxBytes {
		t.Fatal("audit download exceeded its byte budget")
	}
	var page audit.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	commands, claims := 0, 0
	for _, event := range page.Events {
		if event.Kind == audit.CommandFinished {
			commands++
			assertAuditCommandAttribution(t, event, admin.ID, requestID, controller.Generation)
		}
		if event.Kind == audit.ControlClaimed {
			claims++
		}
	}
	if commands != 1 || claims != 1 {
		t.Fatalf("commands=%d claims=%d", commands, claims)
	}
	for _, secret := range []string{requestID, operator.cookie.Value, adminCookie.Value, "speed_min_percent", "token_hash"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("audit download disclosed a credential or command payload")
		}
	}
	denied := authenticatedControlRequest(s, operator.cookie, http.MethodGet, "/api/audit", "audit-observer", "")
	if denied.Code != http.StatusForbidden {
		t.Fatal("operator could inspect administrator audit history")
	}
}

func assertAuditCommandAttribution(t *testing.T, event audit.Event, administratorID, requestID string, generation uint64) {
	t.Helper()
	if event.Actor.Type != "account" || event.Actor.AccountID == administratorID || event.Actor.SessionID == "" || event.Correlation != audit.CorrelateCommand(requestID) || event.Generation != generation || event.Operation != "motion" {
		t.Fatal("command lost its authenticated actor or control correlation")
	}
}

func TestPublicStopDoesNotWaitForAuditStorage(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	engine, _, err := s.motionEngineForStart()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.store.Snapshot()
	if _, err := engine.Start(t.Context(), motion.MotionTarget{PatternID: motion.PatternHardAndRegular, SpeedPercent: 20}, settings.Motion); err != nil {
		t.Fatal(err)
	}
	locked, release, writerDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		writerDone <- s.store.Datastore().WithTx(context.Background(), func(*sql.Tx) error { close(locked); <-release; return nil })
	}()
	<-locked
	defer func() {
		close(release)
		if err := <-writerDone; err != nil {
			t.Error(err)
		}
	}()
	for i := 0; i < audit.QueueLimit+20; i++ {
		s.recordAccessEvent(context.Background(), audit.Event{Kind: audit.StopFinished, Outcome: "unconfirmed"})
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/motion/stop", nil))
		result <- w
	}()
	select {
	case w := <-result:
		if w.Code != http.StatusOK || engine.Snapshot().Running {
			t.Fatal("Stop did not complete while audit storage was blocked")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop waited for audit persistence")
	}
}

func TestOverlappingStopsRetainTheirOwnAuditSequence(t *testing.T) {
	device := newBlockingControllerStopTransport()
	s := newTestServerWithRuntime(t, Runtime{Transport: device, MotionTransport: device})
	var release sync.Once
	defer release.Do(func() { close(device.release) })
	done := make(chan error, 2)
	go func() {
		_, err := s.emergencyStop(audit.WithActor(t.Context(), audit.Actor{Type: "local"}), "ui_stop")
		done <- err
	}()
	<-device.started
	go func() {
		_, err := s.emergencyStop(audit.WithActor(t.Context(), audit.Actor{Type: "public"}), "ui_stop")
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for s.stopSequence.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.stopSequence.Load() != 2 {
		t.Fatal("second Stop did not invalidate promptly")
	}
	release.Do(func() { close(device.release) })
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	s.accessAudit.Close()
	page, err := s.auditStore.Page(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]uint64{}
	for _, event := range page.Events {
		if event.Kind == audit.StopFinished {
			seen[event.Actor.Type] = event.StopSequence
		}
	}
	if seen["local"] != 1 || seen["public"] != 2 {
		t.Fatalf("overlapping Stop attribution: %v", seen)
	}
}
