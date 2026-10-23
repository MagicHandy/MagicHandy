package transport

import (
	"context"
	"sync"
	"time"
)

// PlayAligned starts the selected linear output at the shared stream's current
// position. The existing anchor and paced-command safety path remain in use.
func (i *Intiface) PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	if !start.IsZero() {
		command.StartTimeMillis = max(command.StartTimeMillis, time.Since(start).Milliseconds())
	}
	return i.Play(ctx, command)
}

// IntifaceHub is one Intiface Central connection shared by every output it
// reaches. Intiface Central serves a single client, so the linear owner (a
// stroker) and any other output reached through the same server bind to this
// one session. The hub never paces motion itself.
type IntifaceHub struct {
	mu       sync.Mutex
	linears  map[string]*Intiface
	session  *intifaceSession
	linear   *Intiface
	vibrator *IntifaceVibrator
}

// LinearFor binds a named stroker to the same socket and device-claim registry.
// It can be added after connection; each output still has its own queue/lease.
func (h *IntifaceHub) LinearFor(id string) *Intiface {
	if id == "handy" {
		return h.linear
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.linears == nil {
		h.linears = map[string]*Intiface{}
	}
	if owner := h.linears[id]; owner != nil {
		return owner
	}
	owner := newIntifaceLinear(h.session)
	h.linears[id] = owner
	return owner
}

// RemoveLinear stops and removes only this output; other hub leases survive.
func (h *IntifaceHub) RemoveLinear(ctx context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	owner := h.linears[id]
	if owner == nil {
		return nil
	}
	err := owner.Release(ctx)
	owner.mu.Lock()
	if owner.ownerStop != nil {
		owner.ownerStop()
	}
	owner.mu.Unlock()
	h.session.unbind(owner)
	delete(h.linears, id)
	return err
}

// IntifaceHubStatus is the shared connection's safe snapshot.
type IntifaceHubStatus struct {
	Connected         bool  `json:"connected"`
	Scanning          bool  `json:"scanning"`
	MaxPingTimeMillis int64 `json:"max_ping_time_ms"`
	// PingWatchdog reports whether the server stops its devices when this
	// client stops answering its keepalive. Without it, only a closed
	// connection stops them.
	PingWatchdog bool             `json:"ping_watchdog"`
	Devices      []IntifaceDevice `json:"devices"`
}

// NewIntifaceHub validates options and returns a disconnected hub with its
// linear and vibrator outputs bound.
func NewIntifaceHub(options IntifaceOptions) (*IntifaceHub, error) {
	session, err := newIntifaceSession(options)
	if err != nil {
		return nil, err
	}
	return &IntifaceHub{session: session, linear: newIntifaceLinear(session), vibrator: newIntifaceVibrator(session)}, nil
}

// Vibrator returns the vibrator output bound to this session.
func (h *IntifaceHub) Vibrator() *IntifaceVibrator {
	return h.vibrator
}

// Connect opens the shared session; bound outputs start once it is ready.
func (h *IntifaceHub) Connect(ctx context.Context) error {
	return h.session.Connect(ctx)
}

// Close stops every bound output's playback and device, then closes the
// session.
func (h *IntifaceHub) Close() error {
	return h.session.Close()
}

// Linear returns the stroker output bound to this session.
func (h *IntifaceHub) Linear() *Intiface {
	return h.linear
}

// Status returns the shared connection's state and discovered devices.
func (h *IntifaceHub) Status() IntifaceHubStatus {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return IntifaceHubStatus{
		Connected:         h.session.connected,
		Scanning:          h.session.scanning,
		MaxPingTimeMillis: h.session.maxPingTime.Milliseconds(),
		PingWatchdog:      h.session.maxPingTime > 0,
		Devices:           cloneIntifaceDevicesLocked(h.session.devices),
	}
}

// StartScanning asks Intiface to begin device discovery.
func (h *IntifaceHub) StartScanning(ctx context.Context) error {
	return h.session.StartScanning(ctx)
}

// StopScanning asks Intiface to end device discovery.
func (h *IntifaceHub) StopScanning(ctx context.Context) error {
	return h.session.StopScanning(ctx)
}

// EmergencyStop stops every device the server has, behind a barrier that no
// bound output's write can pass until the Stop completes.
func (h *IntifaceHub) EmergencyStop(ctx context.Context) error {
	return h.session.EmergencyStop(ctx)
}
