package transport

import "context"

// IntifaceHub is one Intiface Central connection shared by every output it
// reaches. Intiface Central serves a single client, so the linear owner (a
// stroker) and any other output reached through the same server bind to this
// one session. The hub never paces motion itself.
type IntifaceHub struct {
	session *intifaceSession
	linear  *Intiface
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
// linear owner bound.
func NewIntifaceHub(options IntifaceOptions) (*IntifaceHub, error) {
	session, err := newIntifaceSession(options)
	if err != nil {
		return nil, err
	}
	return &IntifaceHub{session: session, linear: newIntifaceLinear(session)}, nil
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
