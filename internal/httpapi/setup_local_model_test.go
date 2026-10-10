package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSetupLocalCheckKeepsHostedRouteAndRejectsInvalidOutput(t *testing.T) {
	for _, raw := range []string{`{"ready":true}`, `{"reply":"hello"}`, "invalid"} {
		t.Run(raw, func(t *testing.T) {
			var localCalls, hostedCalls atomic.Int32
			local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				localCalls.Add(1)
				var request struct {
					Model    string                     `json:"model"`
					Messages []struct{ Content string } `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if r.Header.Get("Authorization") != "" || request.Model != "saved-local-model" || len(request.Messages) != 2 {
					t.Error("local check used the wrong model, credentials or conversation context")
				}
				writeHostedFixtureReply(w, raw)
			}))
			defer local.Close()
			s, fake, primary := hostedFixture(t, func(http.ResponseWriter, *http.Request) { hostedCalls.Add(1) })
			defer s.Close()
			configureLocalRetry(t, s, local.URL, false)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, withController(httptest.NewRequest(http.MethodPost, "/api/llm/local/test", strings.NewReader(`{}`))))
			var result struct {
				Ready bool `json:"ready"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusOK || result.Ready != (raw == `{"ready":true}`) || localCalls.Load() != 1 || hostedCalls.Load() != 0 {
				t.Fatalf("unexpected local check: %d %s, local=%d hosted=%d", w.Code, w.Body, localCalls.Load(), hostedCalls.Load())
			}
			saved, _ := s.store.Snapshot()
			if saved.LLM.ConversationConnectionID != primary.ID || saved.LLM.RetryRefusalLocally {
				t.Fatal("probe changed saved routes or enabled retry")
			}
			assertNoRetryMotion(t, fake)
		})
	}
}

func TestSetupLocalCheckStopCancelsGeneration(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	local := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer local.Close()
	s, fake, _ := hostedFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("used hosted model") })
	defer s.Close()
	configureLocalRetry(t, s, local.URL, true)
	w, done := httptest.NewRecorder(), make(chan struct{})
	go func() {
		defer close(done)
		s.Handler().ServeHTTP(w, withController(httptest.NewRequest(http.MethodPost, "/api/llm/local/test", strings.NewReader(`{}`))))
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	stop := httptest.NewRecorder()
	s.Handler().ServeHTTP(stop, withController(httptest.NewRequest(http.MethodPost, "/api/motion/stop", strings.NewReader(`{}`))))
	if stop.Code != http.StatusOK {
		t.Fatalf("Stop failed: %d %s", stop.Code, stop.Body)
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not cancel local HTTP generation")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not finish")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("canceled probe reported success: %d %s", w.Code, w.Body)
	}
	assertNoRetryMotion(t, fake)
}
