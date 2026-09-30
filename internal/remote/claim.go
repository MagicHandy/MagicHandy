package remote

import (
	"errors"
	"strings"
	"time"
)

// ErrCommandUnavailable covers expired, canceled, already claimed and stale-target commands.
var ErrCommandUnavailable = errors.New("the remote command is no longer available")

// CommandSender permits the HTTP edge to revalidate the originating login
// outside the hub lock. Claim rechecks the queue afterwards; Stop never waits
// for a database operation holding this lock.
func (h *Hub) CommandSender(executor Identity, id string) (Identity, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.expireLocked(h.now())
	index := h.claimableLocked(executor, id)
	if index < 0 {
		return Identity{}, false
	}
	return h.pending[index].sender, true
}

// Claim admits one execution, including after SSE redelivery or a page reload.
// Delivery alone is never permission to execute a buffered browser event.
func (h *Hub) Claim(executor Identity, id string) (Command, time.Duration, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	h.expireLocked(now)
	index := h.claimableLocked(executor, id)
	if index < 0 {
		return Command{}, 0, ErrCommandUnavailable
	}
	h.pending[index].claimed = true
	command := h.pending[index]
	return command, CommandTTL - now.Sub(command.IssuedAt), nil
}

// Cancel retires one command without disturbing newer requests.
func (h *Hub) Cancel(id, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completeLocked(Outcome{CommandID: id, Error: reason}, h.now())
	h.touchLocked()
}

func (h *Hub) claimableLocked(executor Identity, id string) int {
	if h.presence == nil || h.executor != executor {
		return -1
	}
	for index, command := range h.pending {
		if command.ID == id && !command.claimed && h.targetCurrentLocked(command) {
			return index
		}
	}
	return -1
}

func (h *Hub) targetCurrentLocked(command Command) bool {
	if command.Action == "open" {
		return true
	}
	if command.Target == "video" {
		return h.presence.Video != nil && command.VideoID == h.presence.Video.VideoID
	}
	return h.presence.Chat != nil && command.SessionID == h.presence.Chat.SessionID
}

func (h *Hub) bindTargetLocked(command *Command) error {
	if command.Action == "open" {
		command.Text = ""
		command.SessionID = ""
		return nil
	}
	if command.Target == "video" {
		id := strings.TrimSpace(command.VideoID)
		if id != "" && id != h.presence.Video.VideoID {
			return ErrTargetUnavailable
		}
		command.VideoID = h.presence.Video.VideoID
		command.SessionID, command.Text = "", ""
		return nil
	}
	id := strings.TrimSpace(command.SessionID)
	if id != "" && id != h.presence.Chat.SessionID {
		return ErrTargetUnavailable
	}
	command.SessionID = h.presence.Chat.SessionID
	command.VideoID = ""
	return nil
}
