package remote

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHistoryDoesNotCrossAccountsAfterDisconnectOrReplacement(t *testing.T) {
	for _, change := range []string{"withdraw", "expire", "replace"} {
		t.Run(change, func(t *testing.T) {
			hub, clock := newTestHub()
			hub.Report(desktop, videoPresence())
			command := mustSend(t, hub, play())
			presence := videoPresence()
			presence.Outcomes = []Outcome{{CommandID: command.ID, Error: "private desktop failure"}}
			hub.Report(desktop, presence)
			other := Identity{ClientID: "another-desktop", AccountID: "another-account"}
			switch change {
			case "withdraw":
				hub.Withdraw(desktop, "closed")
			case "expire":
				clock.advance(PresenceTTL + time.Second)
			case "replace":
				hub.Report(other, videoPresence())
			}
			state, _ := hub.State(other)
			if len(state.Recent) != 0 {
				t.Fatalf("another account received the old desktop's history: %+v", state.Recent)
			}
		})
	}
}

func TestClaimIsOnceOnlyAndDoesNotExposeTheSender(t *testing.T) {
	hub, clock := newTestHub()
	hub.Report(desktop, videoPresence())
	sender := phone
	sender.SessionKey = "private-login-reference"
	sent, err := hub.Send(sender, play())
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(time.Second)
	issuer, found := hub.CommandSender(desktop, sent.ID)
	if !found || issuer != sender {
		t.Fatal("originating login unavailable for revalidation")
	}
	claimed, remaining, err := hub.Claim(desktop, sent.ID)
	if err != nil || claimed.ID != sent.ID || remaining != CommandTTL-time.Second {
		t.Fatalf("claim = %+v, %v, %v", claimed, remaining, err)
	}
	if _, _, err := hub.Claim(desktop, sent.ID); !errors.Is(err, ErrCommandUnavailable) {
		t.Fatalf("second claim: %v", err)
	}
	if commands, _ := hub.Commands(desktop, 0); len(commands) != 0 {
		t.Fatal("claimed command redelivered after reconnect")
	}
	encoded, err := json.Marshal(claimed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), sender.SessionKey) || strings.Contains(string(encoded), sender.ClientID) {
		t.Fatal("sender leaked in command JSON")
	}
}

func TestDeliveredCommandNeedsCurrentAdmission(t *testing.T) {
	for _, change := range []string{"stop", "expire", "withdraw", "replace", "video", "chat", "executor"} {
		t.Run(change, func(t *testing.T) {
			hub, clock := newTestHub()
			presence := videoPresence()
			presence.Chat = &ChatPresence{SessionID: "chat-one", Ready: true}
			hub.Report(desktop, presence)
			input := play()
			if change == "chat" {
				input = Command{Target: "chat", Action: "send", Text: "hello"}
			}
			sent := mustSend(t, hub, input)
			if commands, _ := hub.Commands(desktop, 0); len(commands) != 1 {
				t.Fatal("command was not delivered")
			}
			executor := desktop
			switch change {
			case "stop":
				hub.Clear("stopped")
			case "expire":
				clock.advance(CommandTTL + time.Millisecond)
			case "withdraw":
				hub.Withdraw(desktop, "closed")
			case "replace":
				hub.Report(phone, presence)
			case "video":
				presence.Video = &VideoPresence{VideoID: "another"}
				hub.Report(desktop, presence)
			case "chat":
				presence.Chat = &ChatPresence{SessionID: "chat-two"}
				hub.Report(desktop, presence)
			case "executor":
				executor = phone
			}
			if _, _, err := hub.Claim(executor, sent.ID); !errors.Is(err, ErrCommandUnavailable) {
				t.Fatalf("stale claim = %v", err)
			}
		})
	}
}

func TestSendRejectsAChangedPhoneTarget(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, Presence{Video: &VideoPresence{VideoID: "new-video"}, Chat: &ChatPresence{SessionID: "new-chat"}})
	for _, input := range []Command{
		{Target: "video", Action: "play", VideoID: "old-video"},
		{Target: "chat", Action: "send", Text: "hello", SessionID: "old-chat"},
	} {
		if _, err := hub.Send(phone, input); !errors.Is(err, ErrTargetUnavailable) {
			t.Fatalf("changed target: %v", err)
		}
	}
}

func TestQueuedCommandsKeepTheirOriginalVideo(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, videoPresence())
	sent := mustSend(t, hub, play())
	if sent.VideoID != "clip" {
		t.Fatalf("queued play is not bound to the video it targeted: %+v", sent)
	}
}
