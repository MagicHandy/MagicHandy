package handyble

import (
	"context"
	"errors"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/ble"
)

// SimulatedConnector exercises the RPC path without a radio or a second motion
// engine. It acknowledges bytes and records requests; it does not move anything.
type SimulatedConnector struct {
	mu         sync.Mutex
	last       *SimulatedLink
	WriteOnly  bool
	WriteLimit int
}

// NewSimulatedConnector creates an RPC-only simulator with no radio access.
func NewSimulatedConnector() *SimulatedConnector {
	return &SimulatedConnector{WriteLimit: MaxFrameBytes}
}

// Simulated always reports true.
func (*SimulatedConnector) Simulated() bool { return true }

// Connect opens a fresh in-memory link without starting motion.
func (c *SimulatedConnector) Connect(ctx context.Context, _ ble.ConnectRequest) (ble.Link, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = &SimulatedLink{events: make(chan ble.LinkEvent, 256), limit: c.WriteLimit, writeOnly: c.WriteOnly}
	return c.last, nil
}

// LastLink returns the most recently created link for test inspection.
func (c *SimulatedConnector) LastLink() *SimulatedLink {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// SimulatedLink records wire requests and emits optional RPC acknowledgments.
type SimulatedLink struct {
	mu        sync.Mutex
	events    chan ble.LinkEvent
	closed    bool
	limit     int
	writeOnly bool
	writes    [][]byte
	stream    uint64
}

func (l *SimulatedLink) Write(ctx context.Context, data []byte, _ bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ble.ErrLinkClosed
	}
	if len(data) > l.limit {
		return errors.New("simulated write exceeds MTU")
	}
	outer, err := parseFields(data)
	if err != nil {
		return err
	}
	request, err := parseFields(outer.bytes(2))
	if err != nil || outer.uint(1) != 1 {
		return errors.New("invalid simulated request")
	}
	l.writes = append(l.writes, append([]byte(nil), data...))
	response := appendUint(nil, 1, request.uint(2))
	for _, f := range request {
		if f.wire != 2 {
			continue
		}
		var body []byte
		if f.number == uint32(ClockGet) {
			body = appendUint(nil, 1, 1000)
		}
		if f.number == uint32(Setup) {
			fields, _ := parseFields(f.data)
			l.stream = fields.uint(1)
		}
		if f.number == uint32(State) {
			state := appendUint(nil, 1, 2)
			state = appendUint(state, 10, l.stream)
			body = appendBytes(nil, 1, state)
		}
		response = appendBytes(response, f.number, body)
	}
	if !l.writeOnly {
		payload := appendBytes(appendUint(nil, 1, 3), 4, response)
		select {
		case l.events <- ble.LinkEvent{Kind: ble.LinkNotification, Payload: payload}:
		default:
			return errors.New("simulated notification queue is full")
		}
	}
	return nil
}

// Writes returns independent copies of recorded frames.
func (l *SimulatedLink) Writes() [][]byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([][]byte, len(l.writes))
	for i := range l.writes {
		result[i] = append([]byte(nil), l.writes[i]...)
	}
	return result
}

// Emit injects firmware feedback for isolated protocol tests.
func (l *SimulatedLink) Emit(payload []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		select {
		case l.events <- ble.LinkEvent{Kind: ble.LinkNotification, Payload: append([]byte(nil), payload...)}:
		default:
		}
	}
}

// ReadBattery reports that the Handy profile has no battery characteristic.
func (l *SimulatedLink) ReadBattery(context.Context) (int, error) {
	return 0, errors.New("handy has no battery characteristic")
}

// SetLastWill is inert because this simulator has no physical output.
func (l *SimulatedLink) SetLastWill([]byte) error { return nil }

// Events supplies the simulated notification stream.
func (l *SimulatedLink) Events() <-chan ble.LinkEvent { return l.events }

// Device reports a synthetic identity and write limit.
func (l *SimulatedLink) Device() ble.DeviceInfo {
	return ble.DeviceInfo{Name: "Simulated Handy", Address: "00:00:00:00:00:00", MaxWriteBytes: l.limit}
}

// Close retires the simulated link and ends its event stream.
func (l *SimulatedLink) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.closed = true
		close(l.events)
	}
	return nil
}
