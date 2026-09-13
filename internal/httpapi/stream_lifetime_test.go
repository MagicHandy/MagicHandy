package httpapi

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSSEWriteDeadlineDoesNotLimitIdleGeneration(t *testing.T) {
	handler := logRequests(slog.New(slog.NewTextHandler(io.Discard, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSSEHeaders(w)
		if err := writeSSE(w, "status", map[string]string{"status": "loading"}); err != nil {
			return
		}
		timer := time.NewTimer(5500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		_ = writeSSE(w, "done", map[string]bool{"ok": true})
	}))
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.ProtoMajor != 2 {
		t.Fatalf("test requires HTTP/2, got %s", response.Proto)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil || !strings.Contains(string(body), "event: done") {
		t.Fatalf("healthy generation gap terminated streaming: %q %v", body, err)
	}
}

func TestEnablingAccountsClosesUnprotectedStreams(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/motion/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	reader := bufio.NewReader(response.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	s.auth.requireAuthentication()
	_, _ = io.Copy(io.Discard, reader)
	if ctx.Err() != nil {
		t.Fatal("pre-login stream retained access after protection was enabled")
	}
}
