package httpapi

import "time"

// A phone remote in front is a responsive foreground document of the desktop's
// account (ADR 0055). While the desktop tab is hidden and pauses its own
// heartbeat (ADR 0029), the phone's keepalive renews that tab's lease, so the
// session is not stopped as abandoned while someone is holding the remote. It
// never claims control, crosses a Stop or a handoff, or revives an expired
// lease: when the phone also goes quiet, the lease expires and the usual
// lost-controller Stop runs.
//
// The renewal also issues the owner tab's short-lived command tickets, and the
// owner's snapshot reports that its phone holds the session. A hidden owner
// then stays awake, without renewing anything itself, and carries out the
// phone's video and chat commands with those tickets (ADR 0056). Once the
// phone is quiet no new ticket is issued, so the hidden tab loses its device
// authority within one ticket lifetime and the session stops with the lease.

// ownerLiveLocked reports whether owner still holds an unexpired lease with no
// Stop or handoff pending. Callers hold c.mu.
func (c *controllerRuntime) ownerLiveLocked(owner controllerIdentity) bool {
	return owner.clientID != "" && c.active == owner && !c.stopping && c.pending.clientID == "" &&
		!c.expiredLocked() && c.ownerCtx != nil && c.ownerCtx.Err() == nil
}

// remoteHeldLocked reports whether the owner's phone renewed its lease within
// one lease period. Callers hold c.mu.
func (c *controllerRuntime) remoteHeldLocked(now time.Time) bool {
	return !c.remoteSeenAt.IsZero() && now.Sub(c.remoteSeenAt) <= c.leaseTTL
}

// OwnerLive reports whether owner still holds live control.
func (c *controllerRuntime) OwnerLive(owner controllerIdentity) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ownerLiveLocked(owner)
}

// RemoteRenew extends owner's live lease on behalf of its phone remote and
// issues the command ticket a hidden owner acts with. The caller has matched
// the phone to owner's account.
func (c *controllerRuntime) RemoteRenew(owner controllerIdentity) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ownerLiveLocked(owner) {
		return false
	}
	c.lastSeenAt = c.clock()
	c.remoteSeenAt = c.lastSeenAt
	c.issueCommandTicketLocked()
	return true
}
