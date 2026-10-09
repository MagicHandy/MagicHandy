package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/chat"
	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestCloudAdmissionRejectsRetiredStopSettingsConversationAndSession(t *testing.T) {
	for _, change := range []string{"stop", "settings", "message", "session", "memory", "prompt"} {
		t.Run(change, func(t *testing.T) {
			server := newTestServer(t)
			if change == "prompt" {
				custom, err := server.personalization.prompts.Create("Review prompt", "original prompt")
				if err != nil {
					t.Fatal(err)
				}
				saveSettings(t, server.store, func(s config.Settings) config.Settings { s.LLM.PromptSet = custom.ID; return s })
			}
			settings, _ := server.store.Snapshot()
			admission, err := server.captureCloudAdmission(t.Context(), settings)
			if err != nil {
				t.Fatal(err)
			}
			if err := server.validateCloudAdmission(t.Context(), admission); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "stop":
				server.stopSequence.Add(1)
			case "settings":
				saveSettings(t, server.store, func(s config.Settings) config.Settings { s.Motion.SpeedMaxPercent--; return s })
			case "message":
				_, err = server.chatLog.Append(chat.MessageRoleUser, "new intent", "test")
			case "session":
				_, err = server.chatLog.CreateSession(true)
			case "memory":
				_, err = server.personalization.memory.Add("a changed enabled memory")
			case "prompt":
				_, err = server.personalization.prompts.Update(settings.LLM.PromptSet, "Updated", "a changed prompt")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := server.validateCloudAdmission(t.Context(), admission); err == nil {
				t.Fatal("retired cloud proposal admitted")
			}
		})
	}
}

func TestHostedChatDiscardsLateResultAfterProviderChange(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	server, fake, _ := hostedFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		writeHostedFixtureReply(w, `{"reply":"late reply"}`)
	})
	finished := make(chan string, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := withController(httptest.NewRequest(http.MethodPost, "/api/chat/stream", strings.NewReader(`{"message":"text only"}`)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(stopSequenceHeader, "0")
		server.Handler().ServeHTTP(recorder, request)
		finished <- recorder.Body.String()
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach hosted provider")
	}
	server.invalidateCloudPlanning()
	select {
	case response := <-finished:
		if strings.Contains(response, "late reply") || strings.Contains(response, "event: motion") {
			t.Fatalf("stale output was published: %s", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("provider change did not retire hosted chat")
	}
	for _, command := range fake.Commands() {
		if command.Kind == transport.CommandKindPointsAdd || command.Kind == transport.CommandKindPointsPlay {
			t.Fatal("retired request dispatched motion")
		}
	}
}
