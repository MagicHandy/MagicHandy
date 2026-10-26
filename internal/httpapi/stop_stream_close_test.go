package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/llm"
)

// stopStreamProvider holds a reply open until its request is canceled.
type stopStreamProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *stopStreamProvider) StreamChat(ctx context.Context, _ llm.ChatRequest, _ func(string) error) (string, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return "", ctx.Err()
}

func (p *stopStreamProvider) Status(context.Context) llm.ProviderStatus {
	return llm.ProviderStatus{Provider: "stop-stream", Available: true}
}

func signedInChatStreamRequest(t *testing.T, s *Server, baseURL string, cookie *http.Cookie, message string) *http.Request {
	t.Helper()
	r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/chat/stream", strings.NewReader(`{"message":"`+message+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(controllerHeaderName, "test-controller")
	s.controller.mu.Lock()
	r.Header.Set(controllerGenerationHeader, strconv.FormatUint(s.controller.generation, 10))
	r.Header.Set(controllerEpochHeader, s.controller.epoch)
	if len(s.controller.tickets) > 0 {
		r.Header.Set(commandTicketHeader, s.controller.tickets[len(s.controller.tickets)-1].value)
	}
	s.controller.mu.Unlock()
	s.commands.mu.Lock()
	r.Header.Set(commandSequenceHeader, strconv.FormatUint(s.commands.sequence+1, 10))
	s.commands.mu.Unlock()
	r.Header.Set(commandIDHeader, rand.Text())
	r.Header.Set(stopSequenceHeader, strconv.FormatUint(s.stopSequence.Load(), 10))
	r.AddCookie(cookie)
	return r
}

// Over HTTP/2 an immediate cancellation deadline resets the stream, which a
// browser reports only as its own "network error". Emergency Stop lets the
// canceled reply say why it ended; an ownership change still cuts it off.
func TestStopClosesSignedInChatStreamWithItsReason(t *testing.T) {
	for _, tc := range []struct {
		name, route, clientID string
		closes                bool
	}{
		{"emergency stop", "/api/motion/stop", "test-controller", true},
		{"takeover", "/api/controller/takeover", "another-tab", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &stopStreamProvider{started: make(chan struct{})}
			s, _, _, cookie := newControllerSessionFixture(t, provider)
			claimAuthenticatedController(t, s, cookie)
			server := httptest.NewUnstartedServer(s.Handler())
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()
			response, err := server.Client().Do(signedInChatStreamRequest(t, s, server.URL, cookie, "keep going"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != http.StatusOK || response.ProtoMajor != 2 {
				t.Fatalf("chat stream: %s %d", response.Proto, response.StatusCode)
			}
			select {
			case <-provider.started:
			case <-time.After(5 * time.Second):
				t.Fatal("chat did not reach the model")
			}
			if ended := authenticatedControlRequest(s, cookie, http.MethodPost, tc.route, tc.clientID, `{}`); ended.Code != http.StatusOK {
				t.Fatalf("%s: %d %s", tc.route, ended.Code, ended.Body.String())
			}
			body, readErr := io.ReadAll(response.Body)
			if !tc.closes {
				if readErr == nil {
					t.Fatalf("the previous owner's stream stayed writable after takeover:\n%s", body)
				}
				return
			}
			if readErr != nil {
				t.Fatalf("Stop reset the chat stream instead of closing it: %v\n%s", readErr, body)
			}
			if !strings.Contains(string(body), `"message":"Chat canceled by Emergency Stop."`) || !strings.Contains(string(body), "event: done") {
				t.Fatalf("stream closed without its Stop reason:\n%s", body)
			}
		})
	}
}

func TestControllerTellsBoundRequestsWhyTheyEnded(t *testing.T) {
	controller := newControllerRuntime()
	owner := controllerIdentity{clientID: "tab", sessionKey: "session"}
	controller.Heartbeat(owner, 0)
	bind := func() context.Context {
		ctx, cancel := context.WithCancelCause(t.Context())
		t.Cleanup(func() { cancel(nil) })
		if controller.bindRequestCause(owner, cancel) == nil {
			t.Fatal("the owner's request could not bind")
		}
		return ctx
	}
	stopped := bind()
	controller.AdvanceStopGeneration()
	if !errors.Is(context.Cause(stopped), errChatEmergencyStop) {
		t.Fatalf("Stop cause = %v", context.Cause(stopped))
	}
	replaced := bind()
	if _, started, err := controller.BeginTakeover(controllerIdentity{clientID: "other-tab", sessionKey: "session"}); err != nil || !started {
		t.Fatalf("takeover did not begin: %v", err)
	}
	if !errors.Is(context.Cause(replaced), errChatControlChanged) {
		t.Fatalf("takeover cause = %v", context.Cause(replaced))
	}
}

func TestOnlyStopAndShutdownGiveCanceledSocketsAGrace(t *testing.T) {
	s := &Server{}
	now := time.Unix(1_000, 0)
	for _, tc := range []struct {
		cause       error
		read, write time.Duration
	}{
		{errChatEmergencyStop, stopSocketGrace, stopSocketGrace},
		{errChatControlChanged, 0, 0},
		{errChatAccessEnded, 0, 0},
		{context.Canceled, 0, 0},
	} {
		read, write := s.interruptionDeadlines(tc.cause, now)
		if read.Sub(now) != tc.read || write.Sub(now) != tc.write {
			t.Errorf("%v: read %v write %v, want %v and %v", tc.cause, read.Sub(now), write.Sub(now), tc.read, tc.write)
		}
	}
	s.quiescing.Store(true)
	if read, write := s.interruptionDeadlines(errChatControlChanged, now); read != now || write.Sub(now) != shutdownWriteGrace {
		t.Errorf("shutdown: read %v write %v", read.Sub(now), write.Sub(now))
	}
}

// An HTTP/1.1 connection whose background read failed keeps a canceled
// context. When Stop lets the canceled response close normally, that
// keep-alive connection must still serve the tab's next request.
func TestStopLeavesSignedInKeepAliveConnectionUsable(t *testing.T) {
	provider := &stopStreamProvider{started: make(chan struct{})}
	s, _, _, cookie := newControllerSessionFixture(t, provider)
	claimAuthenticatedController(t, s, cookie)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost = 1, 1
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()

	response, err := client.Do(signedInChatStreamRequest(t, s, server.URL, cookie, "keep going"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("chat did not reach the model")
	}
	if stopped := authenticatedControlRequest(s, cookie, http.MethodPost, "/api/motion/stop", "test-controller", `{}`); stopped.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", stopped.Code, stopped.Body.String())
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || !strings.Contains(string(body), "Chat canceled by Emergency Stop.") {
		t.Fatalf("stream did not close with its Stop reason: %v\n%s", readErr, body)
	}

	reused := false
	next, err := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
	}), http.MethodGet, server.URL+"/api/controller", nil)
	if err != nil {
		t.Fatal(err)
	}
	next.Header.Set(controllerHeaderName, "test-controller")
	next.AddCookie(cookie)
	after, err := client.Do(next)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = after.Body.Close() }()
	if !reused {
		t.Fatal("the Stop-closed stream did not leave its connection open for reuse")
	}
	if after.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(after.Body)
		t.Fatalf("next request on the same connection: %d %s", after.StatusCode, detail)
	}
}
