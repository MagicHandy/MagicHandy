package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"time"
)

// A tab ID identifies a document. Only the authenticated session grants that
// document authority. Neither a public tab ID nor a generation is a credential.
type controllerIdentity struct {
	clientID     string
	sessionKey   string
	grantID      string
	grantExpires time.Time
}

type controllerRuntime struct {
	mu          sync.Mutex
	clock       func() time.Time
	leaseTTL    time.Duration
	active      controllerIdentity
	pending     controllerIdentity
	activeSince time.Time
	lastSeenAt  time.Time
	generation  uint64
	epoch       string
	stopping    bool
	ownerCtx    context.Context
	ownerCancel context.CancelFunc
}

func newControllerRuntime() controllerRuntime {
	return controllerRuntime{clock: time.Now, leaseTTL: controllerLeaseTTL, epoch: rand.Text()}
}

// Observe never renews an authenticated lease. Trusted, unprotected loopback
// clients retain the original auto-claim contract until accounts are enabled.
func (c *controllerRuntime) Observe(actor controllerIdentity) controllerSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if actor.sessionKey == "" {
		c.touchLocalLocked(actor)
	}
	return c.snapshotLocked(actor)
}

func (c *controllerRuntime) Heartbeat(actor controllerIdentity, generation uint64) controllerSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if actor.clientID == "" || c.stopping || c.pending.clientID != "" || (actor.sessionKey != "" && generation != c.generation) {
		return c.snapshotLocked(actor)
	}
	// An initial heartbeat may claim a stopped fresh server. After any loss or
	// takeover, claiming again requires the explicit stop-first takeover route.
	if c.active.clientID == "" && c.generation == 0 {
		c.activateLocked(actor)
	} else if c.active == actor && !c.expiredLocked() {
		c.lastSeenAt = c.clock()
	}
	return c.snapshotLocked(actor)
}

func (c *controllerRuntime) touchLocalLocked(actor controllerIdentity) {
	if actor.clientID == "" || c.stopping || c.pending.clientID != "" || c.active.sessionKey != "" {
		return
	}
	if c.active.clientID == "" || c.expiredLocked() {
		c.activateLocked(actor)
	} else if c.active == actor {
		c.lastSeenAt = c.clock()
	}
}

func (c *controllerRuntime) BeginTakeover(actor controllerIdentity) (controllerSnapshot, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if actor.clientID == "" {
		return c.snapshotLocked(actor), false, errors.New("controller takeover requires a client id")
	}
	if c.stopping || c.pending.clientID != "" {
		return c.snapshotLocked(actor), false, errControllerTakeoverInProgress
	}
	if c.active == actor && !c.expiredLocked() {
		return c.snapshotLocked(actor), false, nil
	}
	c.cancelOwnerLocked()
	c.pending = actor
	return c.snapshotLocked(actor), true, nil
}

func (c *controllerRuntime) CompleteTakeover(actor controllerIdentity) (controllerSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if actor.clientID == "" || c.pending != actor || c.stopping {
		return c.snapshotLocked(actor), errControllerTakeoverNotPending
	}
	c.activateLocked(actor)
	c.pending = controllerIdentity{}
	return c.snapshotLocked(actor), nil
}

func (c *controllerRuntime) CancelTakeover(actor controllerIdentity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == actor {
		c.pending = controllerIdentity{}
		// A failed handoff must not revive the canceled previous lease.
		c.clearOwnerLocked()
	}
}

func (c *controllerRuntime) Authority(actor controllerIdentity) (controllerSnapshot, context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if actor.sessionKey == "" {
		c.touchLocalLocked(actor)
	}
	snapshot := c.snapshotLocked(actor)
	if !snapshot.Active {
		return snapshot, nil
	}
	return snapshot, c.ownerCtx
}

// BeginLoss fences every client before callers stop shared motion. The mutex is
// never held across transport or database work. FinishLoss opens takeover only
// after that stop has completed (or failed with local invalidation preserved).
func (c *controllerRuntime) BeginLoss(sessionKey string, expiredOnly bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopping {
		return false
	}
	matches := sessionKey != "" && (c.active.sessionKey == sessionKey || c.pending.sessionKey == sessionKey)
	if expiredOnly {
		matches = c.active.sessionKey != "" && c.expiredLocked()
	}
	if !matches {
		return false
	}
	c.stopping = true
	c.pending = controllerIdentity{}
	c.clearOwnerLocked()
	return true
}

func (c *controllerRuntime) FinishLoss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopping = false
}

func (c *controllerRuntime) BeginLocalLoss() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopping || c.active.clientID == "" || c.active.sessionKey != "" {
		return false
	}
	c.stopping = true
	c.pending = controllerIdentity{}
	c.clearOwnerLocked()
	return true
}

func (c *controllerRuntime) SessionKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active.sessionKey
}

func (c *controllerRuntime) AdvanceStopGeneration() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active.sessionKey == "" {
		return
	}
	c.cancelOwnerLocked()
	c.generation++
	c.ownerCtx, c.ownerCancel = context.WithCancel(context.Background())
}

func (c *controllerRuntime) activateLocked(actor controllerIdentity) {
	c.cancelOwnerLocked()
	c.active = actor
	c.generation++
	c.activeSince = c.clock()
	c.lastSeenAt = c.activeSince
	c.ownerCtx, c.ownerCancel = context.WithCancel(context.Background())
}

func (c *controllerRuntime) cancelOwnerLocked() {
	if c.ownerCancel != nil {
		c.ownerCancel()
	}
}

func (c *controllerRuntime) clearOwnerLocked() {
	c.cancelOwnerLocked()
	c.active = controllerIdentity{}
	c.activeSince = time.Time{}
	c.lastSeenAt = time.Time{}
	c.generation++
}

func (c *controllerRuntime) expiredLocked() bool {
	return c.active.clientID != "" && (c.clock().Sub(c.lastSeenAt) > c.leaseTTL ||
		(!c.active.grantExpires.IsZero() && !c.clock().Before(c.active.grantExpires)))
}

func (c *controllerRuntime) Owner() controllerIdentity {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

func (c *controllerRuntime) snapshotLocked(actor controllerIdentity) controllerSnapshot {
	now := c.clock()
	active := actor.clientID != "" && c.active == actor && !c.expiredLocked() &&
		!c.stopping && c.pending.clientID == "" && c.ownerCtx != nil && c.ownerCtx.Err() == nil
	reason := ""
	if !active {
		reason = "another browser tab is the active controller"
		if c.stopping || c.pending.clientID != "" {
			reason = "controller handoff is stopping active work"
		} else if c.active.clientID == "" || c.expiredLocked() {
			reason = "take control to establish a fresh controller lease"
		} else if actor.clientID == "" {
			reason = "missing controller client id"
		}
	}
	var age, remaining int64
	if !c.activeSince.IsZero() {
		age = max(0, now.Sub(c.activeSince).Milliseconds())
		remaining = max(0, c.leaseTTL.Milliseconds()-now.Sub(c.lastSeenAt).Milliseconds())
	}
	return controllerSnapshot{
		ClientID: actor.clientID, Active: active, ReadOnly: !active, Reason: reason,
		ActiveClientID: c.active.clientID, ActiveClientAgeMillis: age,
		LeaseExpiresInMillis: remaining, TakeoverInProgress: c.stopping || c.pending.clientID != "",
		Generation: c.generation, Epoch: c.epoch, HeartbeatRequired: actor.sessionKey != "",
	}
}
