package httpapi

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublicStopAcknowledgesBeforeAnUnfinishedHTTP1Body(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	host := httptest.NewServer(s.Handler())
	defer host.Close()
	connection, err := net.DialTimeout("tcp", host.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// The peer sends headers, but never finishes even the first body chunk.
	if _, err := fmt.Fprintf(connection, "POST /api/motion/stop HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\n\r\n", host.Listener.Addr()); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatalf("Stop response waited for an unused request body: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal("Stop acknowledgement was truncated", err)
	}
	if response.StatusCode != http.StatusOK || s.stopSequence.Load() != 1 {
		t.Fatal("Stop did not run through the public safety path")
	}
}

func TestSessionLookupStorageWaitIsBoundedAndDoesNotEraseCookie(t *testing.T) {
	s, _, _, cookie := newControllerSessionFixture(t)
	connections := holdRequestTestConnections(t, s)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/state", nil).WithContext(ctx)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if ctx.Err() != nil {
		t.Fatal("session lookup retained its request until caller timeout")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("storage outage misclassified as login failure: %d", w.Code)
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("temporary storage pressure erased the login cookie")
	}
	// Loading the shell and Stop must remain independent of session storage.
	for _, request := range []struct{ method, path string }{{http.MethodGet, "/"}, {http.MethodPost, "/api/motion/stop"}} {
		probe := httptest.NewRequest(request.method, request.path, nil)
		probe.AddCookie(cookie)
		result := httptest.NewRecorder()
		s.Handler().ServeHTTP(result, probe)
		if result.Code != http.StatusOK {
			t.Fatalf("safety shell/Stop blocked by storage: %s %d", request.path, result.Code)
		}
	}
}

func holdRequestTestConnections(t *testing.T, s *Server) []*sql.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	connections := make([]*sql.Conn, 0, 4)
	for range 4 {
		connection, err := s.store.Datastore().SQL().Conn(ctx)
		if err != nil {
			for _, acquired := range connections {
				_ = acquired.Close()
			}
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	return connections
}

func TestSignedOutAuthenticationStatusDoesNotWaitForeverForStorage(t *testing.T) {
	s, _, _, _ := newControllerSessionFixture(t)
	connections := holdRequestTestConnections(t, s)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil).WithContext(ctx))
	if ctx.Err() != nil || w.Code != http.StatusServiceUnavailable {
		t.Fatal("signed-out status retained unbounded storage work")
	}
}
