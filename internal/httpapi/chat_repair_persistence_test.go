package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

type persistenceContentionProvider struct {
	*scriptedLLMProvider
	beforeReturn func()
}

func (p *persistenceContentionProvider) StreamChat(ctx context.Context, req llm.ChatRequest, delta func(string) error) (string, error) {
	response, err := p.scriptedLLMProvider.StreamChat(ctx, req, delta)
	if before := p.beforeReturn; before != nil {
		p.beforeReturn = nil
		before()
	}
	return response, err
}

// Occupy the real shared writer only after the user turn has been saved, so
// the contention exercises failure persistence rather than request admission.
func occupyFailureWriter(t *testing.T, s *Server) func() {
	t.Helper()
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.store.Datastore().WithTx(context.Background(), func(*sql.Tx) error {
			close(ready)
			<-release
			return nil
		})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("occupy diagnostic writer: %v", err)
	}
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		unblock()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	return unblock
}

func TestManualRepairSurvivesBriefDiagnosticWriterContention(t *testing.T) {
	s := independentVibrationServer(t)
	configureResponseRepair(t, s, false)
	p := &persistenceContentionProvider{scriptedLLMProvider: &scriptedLLMProvider{responses: []string{
		`{"action":"none","edits":{},"vibration":{"mode":"wave","min_percent":20,"max_percent":160,"period_seconds":4},"reply":"Invalid."}`,
		vibrationResumeResponse,
	}}}
	p.beforeReturn = func() {
		unblock := occupyFailureWriter(t, s)
		// Exceeds the former diagnostics-only 500 ms deadline.
		timer := time.AfterFunc(750*time.Millisecond, unblock)
		t.Cleanup(func() { timer.Stop() })
	}
	s.llm.provider = p
	stopVibrationForResume(t, s)
	session, id := repairOffer(t, s, postChatStream(t, s, `{"message":"Resume a gentle vibration wave."}`))
	if !s.intifaceSnapshot().Vibrator.Suspended || p.callCount() != 1 {
		t.Fatal("rejected response resumed motion or retried automatically")
	}
	if _, err := s.chatLog.Failure(t.Context(), session, id, false); err != nil {
		t.Fatalf("repair has no durable failure record: %v", err)
	}
	r := postRepair(s, session, id)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"applied":true`) || s.intifaceSnapshot().Vibrator.Suspended {
		t.Fatalf("fresh repair failed after brief contention: %d %s", r.Code, r.Body.String())
	}
}

func TestManualRepairDoesNotWaitIndefinitelyForDiagnosticWriter(t *testing.T) {
	s := independentVibrationServer(t)
	configureResponseRepair(t, s, false)
	p := &persistenceContentionProvider{scriptedLLMProvider: &scriptedLLMProvider{responses: []string{
		`{"action":"none","edits":{},"vibration":{"mode":"wave","min_percent":20,"max_percent":160,"period_seconds":4},"reply":"Invalid."}`,
	}}}
	p.beforeReturn = func() { occupyFailureWriter(t, s) }
	s.llm.provider = p
	stopVibrationForResume(t, s)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	r := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"Resume a gentle vibration wave."}`)).WithContext(ctx))
	r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if ctx.Err() != nil || !strings.Contains(w.Body.String(), "event: error") || strings.Contains(w.Body.String(), `"available":true`) ||
		!s.intifaceSnapshot().Vibrator.Suspended || p.callCount() != 1 {
		t.Fatalf("blocked writer lost its bound or gained repair/motion authority: %s", w.Body.String())
	}
}
