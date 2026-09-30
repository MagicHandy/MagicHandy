package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/remote"
)

func queueRemotePlay(t *testing.T, s *Server, phone *http.Cookie) remote.Command {
	t.Helper()
	response := authenticatedRemoteRequest(s, phone, http.MethodPost, "/api/remote/commands", "phone-tab", `{"target":"video","action":"play"}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("send: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Command remote.Command `json:"command"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Command
}

func TestRemoteClaimRevalidatesTheOriginalLogin(t *testing.T) {
	s, store, _, desktop, phone := newRemoteFixture(t)
	command := queueRemotePlay(t, s, phone)
	if err := store.RevokeSession(t.Context(), phone.Value); err != nil {
		t.Fatal(err)
	}
	response := authenticatedRemoteRequest(s, desktop, http.MethodPost, "/api/remote/commands/"+command.ID+"/claim", "test-controller", `{}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("logged out sender claimed: %d %s", response.Code, response.Body.String())
	}
	if state := readRemoteState(t, s, desktop); state.Pending != 0 {
		t.Fatal("revoked command not retired")
	}
}

func TestRemoteClaimIsOnceOnlyAndFencedByStop(t *testing.T) {
	s, _, _, desktop, phone := newRemoteFixture(t)
	command := queueRemotePlay(t, s, phone)
	route := "/api/remote/commands/" + command.ID + "/claim"
	response := authenticatedRemoteRequest(s, desktop, http.MethodPost, route, "test-controller", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Command   remote.Command `json:"command"`
		Remaining int64          `json:"remaining_ms"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command.StopSequence != command.StopSequence || payload.Remaining <= 0 || payload.Remaining > remote.CommandTTL.Milliseconds() {
		t.Fatalf("claim lost its admission: %+v", payload)
	}
	if again := authenticatedRemoteRequest(s, desktop, http.MethodPost, route, "test-controller", `{}`); again.Code != http.StatusConflict {
		t.Fatalf("replay: %d", again.Code)
	}
	command = queueRemotePlay(t, s, phone)
	finish, _ := s.invalidateWorkForStop("test")
	finish()
	route = "/api/remote/commands/" + command.ID + "/claim"
	if stopped := authenticatedRemoteRequest(s, desktop, http.MethodPost, route, "test-controller", `{}`); stopped.Code != http.StatusConflict {
		t.Fatalf("claim after Stop: %d", stopped.Code)
	}
}

func TestRemoteSendRejectsMissingOrStaleStopSequence(t *testing.T) {
	s, _, _, _, phone := newRemoteFixture(t)
	for _, value := range []string{"", "999"} {
		response := authenticatedRemoteRequest(s, phone, http.MethodPost, "/api/remote/commands", "phone-tab", `{"target":"video","action":"play"}`, func(r *http.Request) { r.Header.Set(stopSequenceHeader, value) })
		if response.Code != http.StatusConflict {
			t.Fatalf("sequence %q: %d", value, response.Code)
		}
	}
}
