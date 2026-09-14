package httpapi

import (
	"crypto/rand"
	"errors"
	"time"
)

type controllerCommandTicket struct {
	value   string
	expires time.Time
}

func (c *controllerRuntime) issueCommandTicketLocked() {
	if c.active.sessionKey == "" {
		return
	}
	now := c.clock()
	// Coalesce unusually frequent heartbeat requests; normal UI heartbeats
	// produce one ticket every two seconds. Keep a bounded overlap window.
	if len(c.tickets) > 0 && c.tickets[len(c.tickets)-1].expires.Sub(now) > commandTicketLifetime-time.Second {
		return
	}
	if len(c.tickets) >= 8 {
		c.tickets = c.tickets[1:]
	}
	c.tickets = append(c.tickets, controllerCommandTicket{value: rand.Text(), expires: now.Add(commandTicketLifetime)})
}

func (c *controllerRuntime) admitCommand(actor controllerIdentity, generation uint64, value string, admit func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshotLocked(actor).Active && generation == c.generation {
		for _, ticket := range c.tickets {
			if value == ticket.value && c.clock().Before(ticket.expires) {
				return admit()
			}
		}
	}
	return errors.New("control delivery ticket expired or ownership changed; refresh before another command")
}
