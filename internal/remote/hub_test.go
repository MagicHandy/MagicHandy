package remote

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time              { return c.at }
func (c *fakeClock) advance(delta time.Duration) { c.at = c.at.Add(delta) }

func newTestHub() (*Hub, *fakeClock) {
	clock := &fakeClock{at: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	return NewHub(clock.now), clock
}

var (
	desktop = Identity{ClientID: "desktop-tab", AccountID: "owner"}
	phone   = Identity{ClientID: "phone-tab", AccountID: "owner"}
)

func videoPresence() Presence {
	return Presence{Route: "videos", Video: &VideoPresence{VideoID: "clip", Title: "Take 07", Ready: true, Rate: 1, Volume: 1}}
}

func play() Command { return Command{Target: "video", Action: "play"} }

func int64Pointer(value int64) *int64     { return &value }
func floatPointer(value float64) *float64 { return &value }
func boolPointer(value bool) *bool        { return &value }
func mustSend(t *testing.T, hub *Hub, command Command) Command {
	t.Helper()
	sent, err := hub.Send(phone, command)
	if err != nil {
		t.Fatalf("Send(%+v): %v", command, err)
	}
	return sent
}

func TestSendNeedsAPresentDesktopOnTheSameAccount(t *testing.T) {
	hub, _ := newTestHub()
	if _, err := hub.Send(phone, play()); !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("send without a desktop err = %v", err)
	}
	hub.Report(desktop, videoPresence())
	if _, err := hub.Send(Identity{ClientID: "guest-phone", AccountID: "guest"}, play()); !errors.Is(err, ErrOtherAccount) {
		t.Fatalf("send from another account err = %v", err)
	}
	sent := mustSend(t, hub, play())
	if sent.ID == "" || sent.Sequence != 1 {
		t.Fatalf("queued command = %+v", sent)
	}
	commands, _ := hub.Commands(desktop, 0)
	if len(commands) != 1 || commands[0].ID != sent.ID {
		t.Fatalf("desktop commands = %+v", commands)
	}
	if others, _ := hub.Commands(phone, 0); len(others) != 0 {
		t.Fatalf("a tab that is not the executor received %+v", others)
	}
}

func TestAnotherAccountSeesOnlyThatADesktopIsPresent(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, videoPresence())
	mustSend(t, hub, play())
	state, _ := hub.State(Identity{ClientID: "guest-phone", AccountID: "guest"})
	if !state.Connected || !state.OtherAccount || state.Video != nil || state.Revision != 0 || state.Pending != 0 || len(state.Recent) != 0 || state.UpdatedAt != nil {
		t.Fatalf("state for another account = %+v", state)
	}
	if own, _ := hub.State(phone); own.OtherAccount || own.Video == nil || own.Pending != 1 {
		t.Fatalf("state for the same account = %+v", own)
	}
}

func TestLocalInstallTabsShareTheEmptyAccount(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(Identity{ClientID: "desktop"}, videoPresence())
	if _, err := hub.Send(Identity{ClientID: "second-window"}, play()); err != nil {
		t.Fatalf("local send: %v", err)
	}
}

func TestCommandsNeedTheSurfaceTheyTarget(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, Presence{Route: "chat", Chat: &ChatPresence{SessionID: "s", Ready: true}})
	if _, err := hub.Send(phone, play()); !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("video command without a video err = %v", err)
	}
	mustSend(t, hub, Command{Target: "video", Action: "open", VideoID: " clip "})
	sent := mustSend(t, hub, Command{Target: "chat", Action: "send", Text: "  hello  "})
	if sent.Text != "hello" {
		t.Fatalf("chat text = %q, want it trimmed", sent.Text)
	}
	hub.Report(desktop, videoPresence())
	if _, err := hub.Send(phone, Command{Target: "chat", Action: "send", Text: "hi"}); !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("chat command without a chat err = %v", err)
	}
	// Opening the chat is how a phone gets one when the desktop shows none.
	mustSend(t, hub, Command{Target: "chat", Action: "open"})
}

func TestReportsAreBounded(t *testing.T) {
	hub, _ := newTestHub()
	long := strings.Repeat("é", 500)
	state := hub.Report(desktop, Presence{Route: long, Video: &VideoPresence{VideoID: "clip", Title: long}, Chat: &ChatPresence{SessionID: "s", PersonaName: long}})
	if utf8.RuneCountInString(state.Route) != 40 || utf8.RuneCountInString(state.Video.Title) != maxTitle || utf8.RuneCountInString(state.Chat.PersonaName) != 80 {
		t.Fatalf("report was not clipped: route %d, title %d, persona %d runes",
			utf8.RuneCountInString(state.Route), utf8.RuneCountInString(state.Video.Title), utf8.RuneCountInString(state.Chat.PersonaName))
	}
}

func TestReportedOutcomesCompleteCommands(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, videoPresence())
	first := mustSend(t, hub, play())
	second := mustSend(t, hub, Command{Target: "video", Action: "seek", Millis: int64Pointer(5_000)})
	presence := videoPresence()
	presence.Outcomes = []Outcome{{CommandID: first.ID, OK: true, Error: "ignored"}, {CommandID: second.ID, Error: "The video is still loading."}, {CommandID: "unknown", OK: true}}
	state := hub.Report(desktop, presence)
	if state.Pending != 0 || len(state.Recent) != 2 {
		t.Fatalf("state after outcomes = %+v", state)
	}
	if !state.Recent[0].OK || state.Recent[0].Error != "" || state.Recent[1].OK || state.Recent[1].Error != "The video is still loading." {
		t.Fatalf("recent outcomes = %+v", state.Recent)
	}
	if commands, _ := hub.Commands(desktop, 0); len(commands) != 0 {
		t.Fatalf("completed commands remained queued: %+v", commands)
	}
}

func TestLateCommandsExpireInsteadOfRunning(t *testing.T) {
	hub, clock := newTestHub()
	hub.Report(desktop, videoPresence())
	sent := mustSend(t, hub, play())
	clock.advance(CommandTTL + time.Millisecond)
	hub.Report(desktop, videoPresence())
	if commands, _ := hub.Commands(desktop, 0); len(commands) != 0 {
		t.Fatalf("an expired command was delivered: %+v", commands)
	}
	state, _ := hub.State(phone)
	if len(state.Recent) != 1 || state.Recent[0].CommandID != sent.ID || state.Recent[0].OK {
		t.Fatalf("expiry outcome = %+v", state.Recent)
	}
}

func TestSilentDesktopDisappears(t *testing.T) {
	hub, clock := newTestHub()
	hub.Report(desktop, videoPresence())
	mustSend(t, hub, play())
	clock.advance(PresenceTTL + time.Second)
	state, _ := hub.State(phone)
	if state.Connected || state.Video != nil || state.Pending != 0 {
		t.Fatalf("state for a silent desktop = %+v", state)
	}
	if _, err := hub.Send(phone, play()); !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("send to a silent desktop err = %v", err)
	}
}

func TestANewExecutorDropsTheOldQueue(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, videoPresence())
	mustSend(t, hub, play())
	laptop := Identity{ClientID: "laptop-tab", AccountID: "owner"}
	state := hub.Report(laptop, videoPresence())
	if state.Pending != 0 || len(state.Recent) != 1 || state.Recent[0].OK {
		t.Fatalf("state after executor change = %+v", state)
	}
	if commands, _ := hub.Commands(desktop, 0); commands != nil {
		t.Fatalf("the old executor still receives commands: %+v", commands)
	}
}

func TestClearAndWithdrawDropWaitingCommands(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, videoPresence())
	mustSend(t, hub, play())
	hub.Clear("Emergency Stop cleared the command.")
	state, _ := hub.State(phone)
	if state.Pending != 0 || state.Recent[0].Error != "Emergency Stop cleared the command." {
		t.Fatalf("state after Clear = %+v", state)
	}
	hub.Withdraw(phone, "closed")
	if state, _ := hub.State(phone); !state.Connected {
		t.Fatal("another tab withdrew the executor's presence")
	}
	hub.Withdraw(desktop, "closed")
	if state, _ := hub.State(phone); state.Connected {
		t.Fatal("withdraw left the desktop present")
	}
}

func TestChangesWakeWaiters(t *testing.T) {
	hub, _ := newTestHub()
	_, changed := hub.State(phone)
	hub.Report(desktop, videoPresence())
	select {
	case <-changed:
	default:
		t.Fatal("a report did not wake state waiters")
	}
	_, next := hub.Commands(desktop, 0)
	mustSend(t, hub, play())
	select {
	case <-next:
	default:
		t.Fatal("a queued command did not wake the executor")
	}
}

func TestQueueAndHistoryStayBounded(t *testing.T) {
	hub, _ := newTestHub()
	hub.Report(desktop, videoPresence())
	for index := 0; index < maxPending; index++ {
		mustSend(t, hub, play())
	}
	if _, err := hub.Send(phone, play()); !errors.Is(err, ErrBusy) {
		t.Fatalf("send beyond the bound err = %v", err)
	}
	hub.Clear("cleared")
	state, _ := hub.State(phone)
	if len(state.Recent) != maxRecent {
		t.Fatalf("recent outcomes = %d, want %d", len(state.Recent), maxRecent)
	}
}

func TestValidateRejectsIncompleteCommands(t *testing.T) {
	long := make([]rune, MaxChatRunes+1)
	for index := range long {
		long[index] = 'a'
	}
	for _, command := range []Command{
		{Target: "device", Action: "play"},
		{Target: "video", Action: "eject"},
		{Target: "video", Action: "seek"},
		{Target: "video", Action: "seek", Millis: int64Pointer(-1)},
		{Target: "video", Action: "volume", Value: floatPointer(1.5)},
		{Target: "video", Action: "rate", Value: floatPointer(8)},
		{Target: "video", Action: "mute"},
		{Target: "video", Action: "open"},
		{Target: "chat", Action: "send", Text: "   "},
		{Target: "chat", Action: "send", Text: string(long)},
		{Target: "chat", Action: "stop"},
		{Target: "video", Action: "source"},
		{Target: "video", Action: "source", Source: "autopilot"},
	} {
		if err := command.Validate(); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("Validate(%+v) = %v, want invalid", command, err)
		}
	}
	for _, command := range []Command{
		{Target: "video", Action: "seek_by", Millis: int64Pointer(-10_000)},
		{Target: "video", Action: "volume", Value: floatPointer(0)},
		{Target: "video", Action: "mute", Flag: boolPointer(true)},
		{Target: "video", Action: "rate", Value: floatPointer(1.25)},
		{Target: "video", Action: "source", Source: "chat"},
		{Target: "video", Action: "source", Source: "off"},
	} {
		if err := command.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", command, err)
		}
	}
}
