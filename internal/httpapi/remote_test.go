package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
	"github.com/mapledaemon/MagicHandy/internal/remote"
)

const remoteVideoPresence = `{"route":"videos","video":{"video_id":"clip","title":"Take 07","ready":true,"rate":1,"volume":1}}`

func newRemoteFixture(t *testing.T) (*Server, *accounts.Store, accounts.Account, *http.Cookie, *http.Cookie) {
	t.Helper()
	s, store, admin, desktop := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, desktop)
	if response := authenticatedControlRequest(s, desktop, http.MethodPost, "/api/remote/presence", "test-controller", remoteVideoPresence); response.Code != http.StatusOK {
		t.Fatalf("presence: %d %s", response.Code, response.Body.String())
	}
	token, _, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, store, admin, desktop, testSessionCookie(token)
}

func readRemoteState(t *testing.T, s *Server, cookie *http.Cookie) remote.State {
	t.Helper()
	response := authenticatedControlRequest(s, cookie, http.MethodGet, "/api/remote/state", "phone-tab", "")
	if response.Code != http.StatusOK {
		t.Fatalf("remote state: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Remote remote.State `json:"remote"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Remote
}

func openMotionStream(t *testing.T, s *Server, cookie *http.Cookie, tab string) *bufio.Reader {
	t.Helper()
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/motion/events?client_id="+tab, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if response.StatusCode != http.StatusOK {
		t.Fatalf("motion stream: %d", response.StatusCode)
	}
	return bufio.NewReader(response.Body)
}

// nextRemoteCommand reads the motion stream until a remote command arrives,
// giving up after a few motion ticks.
func nextRemoteCommand(t *testing.T, reader *bufio.Reader) (remote.Command, bool) {
	t.Helper()
	for range 6 {
		event, data := nextSSEEvent(t, reader)
		if event == "" {
			return remote.Command{}, false
		}
		if event != "remote_command" {
			continue
		}
		var command remote.Command
		if err := json.Unmarshal([]byte(data), &command); err != nil {
			t.Fatal(err)
		}
		return command, true
	}
	return remote.Command{}, false
}

// nextSSEEvent returns the next event, or "" when the stream ends first.
func nextSSEEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()
	var event, data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", ""
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "" && event != "":
			return event, data
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestRemotePresenceBelongsToTheControllerTab(t *testing.T) {
	s, store, admin, desktop := newControllerSessionFixture(t)
	claimAuthenticatedController(t, s, desktop)
	if response := authenticatedControlRequest(s, desktop, http.MethodPost, "/api/remote/presence", "another-tab", remoteVideoPresence); response.Code != http.StatusConflict {
		t.Fatalf("a tab without control reported presence: %d", response.Code)
	}
	stale := func(r *http.Request) { r.Header.Set(controllerGenerationHeader, "999") }
	if response := authenticatedControlRequest(s, desktop, http.MethodPost, "/api/remote/presence", "test-controller", remoteVideoPresence, stale); response.Code != http.StatusConflict {
		t.Fatalf("a stale generation reported presence: %d", response.Code)
	}
	if response := authenticatedControlRequest(s, desktop, http.MethodPost, "/api/remote/presence", "test-controller", remoteVideoPresence); response.Code != http.StatusOK {
		t.Fatalf("controller presence: %d %s", response.Code, response.Body.String())
	}
	observer := newAdmissionIdentity(t, store, admin.ID, "observer", false)
	for _, request := range []struct{ method, route, body string }{
		{http.MethodGet, "/api/remote/state", ""},
		{http.MethodGet, "/api/remote/events", ""},
		{http.MethodPost, "/api/remote/commands", `{"target":"video","action":"play"}`},
	} {
		if response := authenticatedControlRequest(s, observer.cookie, request.method, request.route, "observer-tab", request.body); response.Code != http.StatusForbidden {
			t.Errorf("observer %s %s = %d, want 403", request.method, request.route, response.Code)
		}
	}
}

func TestRemoteCommandReachesTheDesktopAndReportsItsOutcome(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	stream := openMotionStream(t, s, desktop, "test-controller")
	response := authenticatedControlRequest(s, phone, http.MethodPost, "/api/remote/commands", "phone-tab", `{"target":"video","action":"seek","ms":5000}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("send: %d %s", response.Code, response.Body.String())
	}
	command, ok := nextRemoteCommand(t, stream)
	if !ok || command.Action != "seek" || command.Millis == nil || *command.Millis != 5000 {
		t.Fatalf("desktop received %+v (%v)", command, ok)
	}
	if again, ok := nextRemoteCommand(t, stream); ok {
		t.Fatalf("the command was delivered twice on one stream: %+v", again)
	}
	report := `{"route":"videos","video":{"video_id":"clip","title":"Take 07","ready":true,"rate":1,"volume":1,"position_ms":5000},"outcomes":[{"command_id":"` + command.ID + `","ok":true}]}`
	if response := authenticatedControlRequest(s, desktop, http.MethodPost, "/api/remote/presence", "test-controller", report); response.Code != http.StatusOK {
		t.Fatalf("outcome report: %d", response.Code)
	}
	state := readRemoteState(t, s, phone)
	if state.Pending != 0 || len(state.Recent) != 1 || !state.Recent[0].OK || state.Video == nil || state.Video.PositionMillis != 5000 {
		t.Fatalf("phone state = %+v", state)
	}
}

func TestCommandsOnlyReachTheExecutorTab(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	other := openMotionStream(t, s, phone, "phone-tab")
	if response := authenticatedControlRequest(s, phone, http.MethodPost, "/api/remote/commands", "phone-tab", `{"target":"video","action":"pause"}`); response.Code != http.StatusAccepted {
		t.Fatalf("send: %d", response.Code)
	}
	if command, ok := nextRemoteCommand(t, other); ok {
		t.Fatalf("a tab that is not the desktop received %+v", command)
	}
	if _, ok := nextRemoteCommand(t, openMotionStream(t, s, desktop, "test-controller")); !ok {
		t.Fatal("the desktop did not receive the command")
	}
}

func TestEmergencyStopDropsWaitingRemoteCommands(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	if response := authenticatedControlRequest(s, phone, http.MethodPost, "/api/remote/commands", "phone-tab", `{"target":"video","action":"play"}`); response.Code != http.StatusAccepted {
		t.Fatalf("send: %d", response.Code)
	}
	if response := authenticatedControlRequest(s, desktop, http.MethodPost, "/api/motion/stop", "test-controller", `{}`); response.Code != http.StatusOK {
		t.Fatalf("stop: %d", response.Code)
	}
	state := readRemoteState(t, s, phone)
	if state.Pending != 0 || len(state.Recent) != 1 || state.Recent[0].Error != "Emergency Stop cleared the command." {
		t.Fatalf("state after Stop = %+v", state)
	}
}

func TestRemoteStaysWithinOneAccount(t *testing.T) {
	s, store, admin, _, _ := newRemoteFixture(t)
	operator := newAdmissionIdentity(t, store, admin.ID, "operator", true)
	response := authenticatedControlRequest(s, operator.cookie, http.MethodPost, "/api/remote/commands", "operator-tab", `{"target":"video","action":"pause"}`)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"code":"other_account"`) {
		t.Fatalf("another account commanded the desktop: %d %s", response.Code, response.Body.String())
	}
	state := readRemoteState(t, s, operator.cookie)
	if !state.Connected || !state.OtherAccount || state.Video != nil || state.Chat != nil {
		t.Fatalf("another account saw %+v", state)
	}
}

func TestCommandsForATabThatLostControlAreDropped(t *testing.T) {
	s, _, _, desktop, laptop := newRemoteFixture(t)
	if response := authenticatedControlRequest(s, laptop, http.MethodPost, "/api/controller/takeover", "laptop-tab", `{}`); response.Code != http.StatusOK {
		t.Fatalf("takeover: %d %s", response.Code, response.Body.String())
	}
	// The old desktop's presence has not expired, so the phone can still queue.
	if response := authenticatedControlRequest(s, laptop, http.MethodPost, "/api/remote/commands", "phone-tab", `{"target":"video","action":"play"}`); response.Code != http.StatusAccepted {
		t.Fatalf("send: %d", response.Code)
	}
	if command, ok := nextRemoteCommand(t, openMotionStream(t, s, desktop, "test-controller")); ok {
		t.Fatalf("a tab without control received %+v", command)
	}
	state := readRemoteState(t, s, laptop)
	if state.Connected || state.Pending != 0 || len(state.Recent) != 1 || state.Recent[0].Error != "Control moved to another tab; send the command again." {
		t.Fatalf("state after control moved = %+v", state)
	}
}
