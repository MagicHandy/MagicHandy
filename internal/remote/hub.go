// Package remote relays intent from a signed-in phone to the desktop tab that
// holds control: play, pause or seek its open video, open another video, or
// open the chat and send it a message. The desktop tab carries each command
// out through its own controls, with its own controller authority, so this
// package adds no motion path and never talks to the engine or a transport.
// It only queues commands, records what the desktop reports about itself, and
// tells remotes.
package remote

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// PresenceTTL is how long a desktop counts as present after its last report.
	PresenceTTL = 20 * time.Second
	// CommandTTL bounds how long a command may wait. An older command is
	// dropped rather than carried out late.
	CommandTTL = 10 * time.Second
	// MaxChatRunes matches the chat composer's limit.
	MaxChatRunes = 1000
	maxPending   = 32
	maxRecent    = 16
	maxTitle     = 200
)

var (
	// ErrNoDesktop reports that no desktop tab is showing a video or chat.
	ErrNoDesktop = errors.New("no desktop is showing a video or a chat")
	// ErrOtherAccount reports that the desktop is signed in to another account.
	ErrOtherAccount = errors.New("the desktop is signed in to another account")
	// ErrInvalidCommand reports a malformed command.
	ErrInvalidCommand = errors.New("invalid remote command")
	// ErrTargetUnavailable reports a command for a surface the desktop is not showing.
	ErrTargetUnavailable = errors.New("the desktop is not showing that")
	// ErrBusy reports a full queue.
	ErrBusy = errors.New("too many remote commands are waiting; try again")
)

// Identity names one browser tab and the account it is signed in with. The
// account is empty on an unprotected local install, where every tab is local.
type Identity struct {
	ClientID  string
	AccountID string
}

// Command is one request from a remote. Fields beyond Target and Action
// depend on the action; Validate states which ones are required.
type Command struct {
	ID       string   `json:"id"`
	Sequence uint64   `json:"sequence"`
	Target   string   `json:"target"`
	Action   string   `json:"action"`
	VideoID  string   `json:"video_id,omitempty"`
	Millis   *int64   `json:"ms,omitempty"`
	Value    *float64 `json:"value,omitempty"`
	Flag     *bool    `json:"flag,omitempty"`
	Text     string   `json:"text,omitempty"`
	// Source is the video's motion source for the "source" action.
	Source   string    `json:"source,omitempty"`
	IssuedAt time.Time `json:"issued_at"`
}

// Outcome reports what happened to a command.
type Outcome struct {
	CommandID string    `json:"command_id"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	At        time.Time `json:"at"`
}

// VideoPresence describes the desktop's open video.
type VideoPresence struct {
	VideoID        string  `json:"video_id"`
	Title          string  `json:"title"`
	Playing        bool    `json:"playing"`
	PositionMillis int64   `json:"position_ms"`
	DurationMillis int64   `json:"duration_ms"`
	Volume         float64 `json:"volume"`
	Muted          bool    `json:"muted"`
	Rate           float64 `json:"rate"`
	Synchronized   bool    `json:"synchronized"`
	SyncState      string  `json:"sync_state,omitempty"`
	// Ready is false while a paired script is still loading.
	Ready bool `json:"ready"`
	// MotionSource is what moves the device for this video: script, chat or off.
	MotionSource string `json:"motion_source,omitempty"`
	// HasScript reports a paired script, which the script source needs.
	HasScript bool `json:"has_script"`
}

// ChatPresence describes the desktop's open conversation.
type ChatPresence struct {
	SessionID   string `json:"session_id"`
	PersonaName string `json:"persona_name,omitempty"`
	Busy        bool   `json:"busy"`
	Ready       bool   `json:"ready"`
}

// Presence is what the desktop tab reports about itself.
type Presence struct {
	Route    string         `json:"route"`
	Video    *VideoPresence `json:"video,omitempty"`
	Chat     *ChatPresence  `json:"chat,omitempty"`
	Outcomes []Outcome      `json:"outcomes,omitempty"`
}

// State is what remotes see. It carries no account or tab identity.
type State struct {
	Revision  uint64 `json:"revision"`
	Connected bool   `json:"connected"`
	// OtherAccount is set, and every detail withheld, when the desktop is
	// signed in to an account other than the viewer's.
	OtherAccount bool           `json:"other_account,omitempty"`
	Route        string         `json:"route,omitempty"`
	Video        *VideoPresence `json:"video,omitempty"`
	Chat         *ChatPresence  `json:"chat,omitempty"`
	Pending      int            `json:"pending"`
	Recent       []Outcome      `json:"recent"`
	UpdatedAt    *time.Time     `json:"updated_at,omitempty"`
}

// Hub holds one desktop's presence and its queue. It starts no goroutines:
// expiry is applied whenever the hub is read or written, and waiters use the
// change channel.
type Hub struct {
	mu         sync.Mutex
	now        func() time.Time
	executor   Identity
	presence   *Presence
	presenceAt time.Time
	revision   uint64
	sequence   uint64
	pending    []Command
	recent     []Outcome
	changed    chan struct{}
}

// NewHub returns an empty hub; now defaults to time.Now.
func NewHub(now func() time.Time) *Hub {
	if now == nil {
		now = time.Now
	}
	return &Hub{now: now, changed: make(chan struct{})}
}

// Report records the executing tab's presence and the outcomes it reports.
// A different tab replaces the executor; commands queued for the old one are
// dropped, because they were meant for the screen it was showing.
func (h *Hub) Report(executor Identity, presence Presence) State {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.expireLocked(now)
	if h.presence != nil && h.executor != executor {
		h.dropPendingLocked(now, "The desktop changed; send the command again.")
	}
	for _, outcome := range presence.Outcomes {
		h.completeLocked(outcome, now)
	}
	presence.Outcomes = nil
	presence.Route = clip(presence.Route, 40)
	presence.Video = cleanVideo(presence.Video)
	presence.Chat = cleanChat(presence.Chat)
	h.executor = executor
	h.presence = &presence
	h.presenceAt = now
	h.touchLocked()
	return h.stateLocked(now)
}

// Withdraw ends a tab's presence: it closed its video and chat, or it lost
// control. Commands still waiting for it are dropped with the reason.
func (h *Hub) Withdraw(executor Identity, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.presence == nil || h.executor != executor {
		return
	}
	now := h.now()
	h.dropPendingLocked(now, reason)
	h.presence = nil
	h.touchLocked()
}

// Clear drops every waiting command. Emergency Stop calls it: a command
// requested before a Stop must not run after it.
func (h *Hub) Clear(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.pending) == 0 {
		return
	}
	h.dropPendingLocked(h.now(), reason)
	h.touchLocked()
}

// Send validates a command from a remote and queues it for the desktop.
func (h *Hub) Send(sender Identity, command Command) (Command, error) {
	if err := command.Validate(); err != nil {
		return Command{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.expireLocked(now)
	if !h.presentLocked(now) {
		return Command{}, ErrNoDesktop
	}
	if sender.AccountID != h.executor.AccountID {
		return Command{}, ErrOtherAccount
	}
	switch {
	case command.Target == "video" && command.Action != "open" && h.presence.Video == nil:
		return Command{}, fmt.Errorf("%w: no video is open on the desktop", ErrTargetUnavailable)
	case command.Target == "chat" && command.Action != "open" && h.presence.Chat == nil:
		return Command{}, fmt.Errorf("%w: no chat is open on the desktop", ErrTargetUnavailable)
	}
	if len(h.pending) >= maxPending {
		return Command{}, ErrBusy
	}
	command.Text = strings.TrimSpace(command.Text)
	command.VideoID = strings.TrimSpace(command.VideoID)
	h.sequence++
	command.ID = rand.Text()
	command.Sequence = h.sequence
	command.IssuedAt = now
	h.pending = append(h.pending, command)
	h.touchLocked()
	return command, nil
}

// Commands returns the queued commands after a sequence for the current
// executor, and a channel that closes on the next change.
func (h *Hub) Commands(executor Identity, after uint64) ([]Command, <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.expireLocked(h.now())
	if h.presence == nil || h.executor != executor {
		return nil, h.changed
	}
	commands := make([]Command, 0, len(h.pending))
	for _, command := range h.pending {
		if command.Sequence > after {
			commands = append(commands, command)
		}
	}
	return commands, h.changed
}

// State returns what a viewer may see and a channel that closes on the next
// change. A viewer on another account learns only that a desktop is present;
// its revision stays zero so a stream has nothing new to send it.
func (h *Hub) State(viewer Identity) (State, <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.expireLocked(now)
	state := h.stateLocked(now)
	if state.Connected && viewer.AccountID != h.executor.AccountID {
		state = State{Connected: true, OtherAccount: true, Recent: []Outcome{}}
	}
	return state, h.changed
}

func (h *Hub) stateLocked(now time.Time) State {
	state := State{Revision: h.revision, Pending: len(h.pending), Recent: append([]Outcome{}, h.recent...)}
	if h.presentLocked(now) {
		updated := h.presenceAt
		state.Connected = true
		state.Route = h.presence.Route
		state.Video = h.presence.Video
		state.Chat = h.presence.Chat
		state.UpdatedAt = &updated
	}
	return state
}

func (h *Hub) presentLocked(now time.Time) bool {
	return h.presence != nil && now.Sub(h.presenceAt) <= PresenceTTL
}

// expireLocked drops commands that waited too long and forgets a desktop that
// stopped reporting. It changes state, so it notifies waiters when it acts.
func (h *Hub) expireLocked(now time.Time) {
	changed := false
	kept := h.pending[:0]
	for _, command := range h.pending {
		if now.Sub(command.IssuedAt) > CommandTTL {
			h.recordLocked(Outcome{CommandID: command.ID, Error: "The desktop did not respond.", At: now})
			changed = true
			continue
		}
		kept = append(kept, command)
	}
	h.pending = kept
	if h.presence != nil && now.Sub(h.presenceAt) > PresenceTTL {
		h.dropPendingLocked(now, "The desktop stopped responding.")
		h.presence = nil
		changed = true
	}
	if changed {
		h.touchLocked()
	}
}

func (h *Hub) completeLocked(outcome Outcome, now time.Time) {
	for index, command := range h.pending {
		if command.ID != outcome.CommandID {
			continue
		}
		h.pending = append(h.pending[:index], h.pending[index+1:]...)
		outcome.At = now
		outcome.Error = clip(outcome.Error, 300)
		if outcome.OK {
			outcome.Error = ""
		}
		h.recordLocked(outcome)
		return
	}
}

func (h *Hub) dropPendingLocked(now time.Time, reason string) {
	for _, command := range h.pending {
		h.recordLocked(Outcome{CommandID: command.ID, Error: reason, At: now})
	}
	h.pending = nil
}

func (h *Hub) recordLocked(outcome Outcome) {
	h.recent = append(h.recent, outcome)
	if len(h.recent) > maxRecent {
		h.recent = append([]Outcome{}, h.recent[len(h.recent)-maxRecent:]...)
	}
}

func (h *Hub) touchLocked() {
	h.revision++
	close(h.changed)
	h.changed = make(chan struct{})
}

// Validate checks that a command is complete and within bounds.
func (c Command) Validate() error {
	switch c.Target {
	case "video":
		return c.validateVideo()
	case "chat":
		switch c.Action {
		case "open":
			return nil
		case "send":
		default:
			return invalidCommand("unknown chat action")
		}
		text := strings.TrimSpace(c.Text)
		if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxChatRunes {
			return invalidCommand(fmt.Sprintf("a message needs 1 to %d characters", MaxChatRunes))
		}
		return nil
	default:
		return invalidCommand("unknown target")
	}
}

// maxSeekMillis bounds a seek well past any real video, so a malformed value
// fails here instead of in the player.
var maxSeekMillis = (30 * 24 * time.Hour).Milliseconds()

func (c Command) validateVideo() error {
	switch c.Action {
	case "play", "pause", "toggle", "close":
		return nil
	case "seek", "seek_by":
		return c.validateSeek()
	case "volume", "rate", "mute":
		return c.validateLevel()
	case "open":
		if strings.TrimSpace(c.VideoID) == "" || len(c.VideoID) > 128 {
			return invalidCommand("open needs a video id")
		}
		return nil
	case "source":
		switch c.Source {
		case "script", "chat", "off":
			return nil
		}
		return invalidCommand("the motion source must be script, chat or off")
	default:
		return invalidCommand("unknown video action")
	}
}

func (c Command) validateLevel() error {
	switch {
	case c.Action == "volume" && (c.Value == nil || *c.Value < 0 || *c.Value > 1):
		return invalidCommand("volume must be from 0 to 1")
	case c.Action == "rate" && (c.Value == nil || *c.Value < 0.25 || *c.Value > 4):
		return invalidCommand("the playback rate must be from 0.25 to 4")
	case c.Action == "mute" && c.Flag == nil:
		return invalidCommand("mute needs a flag")
	default:
		return nil
	}
}

func (c Command) validateSeek() error {
	switch {
	case c.Millis == nil:
		return invalidCommand("a seek needs ms")
	case c.Action == "seek" && *c.Millis < 0:
		return invalidCommand("a seek position cannot be negative")
	case *c.Millis > maxSeekMillis || *c.Millis < -maxSeekMillis:
		return invalidCommand("the seek is out of range")
	default:
		return nil
	}
}

func invalidCommand(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidCommand, reason)
}

func cleanVideo(video *VideoPresence) *VideoPresence {
	if video == nil {
		return nil
	}
	cleaned := *video
	cleaned.Title = clip(cleaned.Title, maxTitle)
	cleaned.SyncState = clip(cleaned.SyncState, 40)
	cleaned.MotionSource = clip(cleaned.MotionSource, 10)
	return &cleaned
}

func cleanChat(chat *ChatPresence) *ChatPresence {
	if chat == nil {
		return nil
	}
	cleaned := *chat
	cleaned.SessionID = clip(cleaned.SessionID, 128)
	cleaned.PersonaName = clip(cleaned.PersonaName, 80)
	return &cleaned
}

func clip(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
