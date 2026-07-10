package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	controllerHeaderName = "X-MagicHandy-Client-ID"
	controllerLeaseTTL   = 15 * time.Second
)

type controllerRuntime struct {
	mu             sync.Mutex
	clock          func() time.Time
	leaseTTL       time.Duration
	activeClientID string
	activeSince    time.Time
	lastSeenAt     time.Time
}

type controllerSnapshot struct {
	ClientID              string `json:"client_id,omitempty"`
	Active                bool   `json:"active"`
	ReadOnly              bool   `json:"read_only"`
	Reason                string `json:"reason,omitempty"`
	ActiveClientID        string `json:"active_client_id,omitempty"`
	ActiveClientAgeMillis int64  `json:"active_client_age_ms,omitempty"`
	LeaseExpiresInMillis  int64  `json:"lease_expires_in_ms,omitempty"`
}

func newControllerRuntime() controllerRuntime {
	return controllerRuntime{
		clock:    func() time.Time { return time.Now().UTC() },
		leaseTTL: controllerLeaseTTL,
	}
}

func (s *Server) handleControllerState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.controllerState(r))
}

func (s *Server) controllerState(r *http.Request) controllerSnapshot {
	return s.controller.Touch(clientIDFromRequest(r))
}

func (s *Server) requireController(w http.ResponseWriter, r *http.Request) bool {
	return s.requireControllerID(w, clientIDFromRequest(r))
}

func (s *Server) requireControllerID(w http.ResponseWriter, clientID string) bool {
	snapshot := s.controller.Touch(clientID)
	if snapshot.Active {
		return true
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":      "this client is read-only; the active controller owns device commands",
		"controller": snapshot,
	})
	return false
}

func clientIDFromRequest(r *http.Request) string {
	clientID := strings.TrimSpace(r.Header.Get(controllerHeaderName))
	if clientID == "" {
		clientID = strings.TrimSpace(r.URL.Query().Get("client_id"))
	}
	return cleanControllerClientID(clientID)
}

func cleanControllerClientID(clientID string) string {
	clientID = strings.TrimSpace(clientID)
	if len(clientID) > 120 {
		clientID = clientID[:120]
	}
	clientID = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return -1
		}
	}, clientID)
	return clientID
}

func (c *controllerRuntime) Touch(clientID string) controllerSnapshot {
	clientID = cleanControllerClientID(clientID)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.nowLocked()
	c.expireLocked(now)
	if clientID == "" {
		snap := c.snapshotLocked(clientID, "missing controller client id", now)
		// #region agent log
		agentDebugLog("H2", "controller.go:Touch", "missing_client_id", map[string]any{
			"read_only": snap.ReadOnly, "reason": snap.Reason,
		})
		// #endregion
		return snap
	}
	if c.activeClientID == "" {
		c.activeClientID = clientID
		c.activeSince = now
		c.lastSeenAt = now
		snap := c.snapshotLocked(clientID, "", now)
		// #region agent log
		agentDebugLog("H1", "controller.go:Touch", "lease_acquired", map[string]any{
			"client_id": clientID, "active": snap.Active, "read_only": snap.ReadOnly,
		})
		// #endregion
		return snap
	}
	if c.activeClientID == clientID {
		c.lastSeenAt = now
		snap := c.snapshotLocked(clientID, "", now)
		// #region agent log
		agentDebugLog("H1", "controller.go:Touch", "lease_renewed", map[string]any{
			"client_id": clientID, "active": snap.Active, "read_only": snap.ReadOnly,
		})
		// #endregion
		return snap
	}
	snap := c.snapshotLocked(clientID, "another browser tab is the active controller", now)
	// #region agent log
	agentDebugLog("H1", "controller.go:Touch", "lease_denied", map[string]any{
		"client_id": clientID, "active_client_id": c.activeClientID,
		"read_only": snap.ReadOnly, "reason": snap.Reason,
	})
	// #endregion
	return snap
}

func (c *controllerRuntime) Release(clientID string) {
	clientID = cleanControllerClientID(clientID)
	if clientID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeClientID == clientID {
		c.activeClientID = ""
		c.activeSince = time.Time{}
		c.lastSeenAt = time.Time{}
	}
}

func (c *controllerRuntime) snapshotLocked(clientID string, reason string, now time.Time) controllerSnapshot {
	active := clientID != "" && c.activeClientID == clientID
	readOnly := !active
	if reason == "" && readOnly {
		reason = "another browser tab is the active controller"
	}
	age := int64(0)
	expires := int64(0)
	if !c.activeSince.IsZero() {
		age = now.Sub(c.activeSince).Milliseconds()
	}
	if !c.lastSeenAt.IsZero() {
		expires = c.leaseTTL.Milliseconds() - now.Sub(c.lastSeenAt).Milliseconds()
		if expires < 0 {
			expires = 0
		}
	}
	return controllerSnapshot{
		ClientID:              clientID,
		Active:                active,
		ReadOnly:              readOnly,
		Reason:                reason,
		ActiveClientID:        c.activeClientID,
		ActiveClientAgeMillis: age,
		LeaseExpiresInMillis:  expires,
	}
}

func (c *controllerRuntime) expireLocked(now time.Time) {
	if c.activeClientID == "" || c.lastSeenAt.IsZero() {
		return
	}
	if now.Sub(c.lastSeenAt) <= c.leaseTTL {
		return
	}
	c.activeClientID = ""
	c.activeSince = time.Time{}
	c.lastSeenAt = time.Time{}
}

func (c *controllerRuntime) nowLocked() time.Time {
	if c.clock != nil {
		return c.clock()
	}
	return time.Now().UTC()
}
