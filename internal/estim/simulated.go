package estim

import (
	"context"
	"errors"
	"sync"
)

const (
	simulatedAddress     = "SIMULATED"
	simulatedEventBuffer = 64
	simulatedBattery     = 87
)

// SimulatedState is a test and review view of the modeled device.
type SimulatedState struct {
	StrengthA  int
	StrengthB  int
	CeilingA   int
	CeilingB   int
	Packets    int
	Limits     int
	OutputA    int
	OutputB    int
	LastWill   bool
	Closed     bool
	LastPacket Packet
}

// SimulatedConnector opens in-process links to a modeled Coyote 3.0. Each link
// decodes packets the way the documented device does: ceilings clamp every
// change, a sequenced change returns a B1 report, and a change made on the
// device itself reports with sequence 0. It never touches a radio.
type SimulatedConnector struct {
	mu         sync.Mutex
	last       *SimulatedLink
	connectErr error
	initialA   int
	initialB   int
}

// NewSimulatedConnector returns a connector whose devices start at strength 0.
func NewSimulatedConnector() *SimulatedConnector {
	return &SimulatedConnector{}
}

// FailNextConnect makes the next Connect return err (tests only).
func (c *SimulatedConnector) FailNextConnect(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connectErr = err
}

// StartAt sets the strength the next simulated device reports before any
// change, modeling a device left at a level by an earlier session.
func (c *SimulatedConnector) StartAt(a, b int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialA, c.initialB = a, b
}

// Connect opens a new simulated link.
func (c *SimulatedConnector) Connect(ctx context.Context, _ ConnectRequest) (Link, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.connectErr; err != nil {
		c.connectErr = nil
		return nil, err
	}
	link := &SimulatedLink{
		events:   make(chan LinkEvent, simulatedEventBuffer),
		strength: [2]int{c.initialA, c.initialB},
		ceiling:  [2]int{MaxStrength, MaxStrength},
	}
	c.last = link
	return link, nil
}

// Simulated reports that links never reach a radio.
func (c *SimulatedConnector) Simulated() bool {
	return true
}

// LastLink returns the most recently opened link (tests only).
func (c *SimulatedConnector) LastLink() *SimulatedLink {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// SimulatedLink is one modeled connection.
type SimulatedLink struct {
	mu       sync.Mutex
	events   chan LinkEvent
	closed   bool
	strength [2]int
	ceiling  [2]int
	packets  int
	limits   int
	output   [2]int
	lastWill []byte
	last     Packet
}

// Write applies one B0 or BF payload.
func (l *SimulatedLink) Write(ctx context.Context, payload []byte, _ bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ErrLinkClosed
	}
	if limits, err := DecodeLimits(payload); err == nil {
		l.applyLimitsLocked(limits)
		return nil
	}
	packet, err := DecodePacket(payload)
	if err != nil {
		return err
	}
	l.packets++
	l.last = packet
	before := l.strength
	l.strength[ChannelA] = l.nextStrength(ChannelA, packet.A)
	l.strength[ChannelB] = l.nextStrength(ChannelB, packet.B)
	l.output = [2]int{lastIntensity(packet, ChannelA), lastIntensity(packet, ChannelB)}
	if packet.Sequence != 0 && before != l.strength {
		l.reportLocked(packet.Sequence)
	}
	return nil
}

func (l *SimulatedLink) applyLimitsLocked(limits Limits) {
	l.limits++
	before := l.strength
	for channel, value := range []int{limits.StrengthA, limits.StrengthB} {
		if value >= 0 && value <= MaxStrength {
			l.ceiling[channel] = value
			l.strength[channel] = min(l.strength[channel], value)
		}
	}
	if before != l.strength {
		l.reportLocked(0)
	}
}

func (l *SimulatedLink) nextStrength(channel Channel, change StrengthChange) int {
	current := l.strength[channel]
	switch change.Mode {
	case StrengthIncrease:
		current += change.Value
	case StrengthDecrease:
		current -= change.Value
	case StrengthSet:
		current = change.Value
	default:
		return current
	}
	return clampInt(current, 0, l.ceiling[channel])
}

func lastIntensity(packet Packet, channel Channel) int {
	frame := packet.Frames[FramesPerPacket-1]
	if channel == ChannelA {
		if packet.SilenceA {
			return 0
		}
		return frame.A.Intensity
	}
	if packet.SilenceB {
		return 0
	}
	return frame.B.Intensity
}

func (l *SimulatedLink) reportLocked(sequence int) {
	payload := []byte{headerStrengthReport, 0, 0, 0}
	for index, value := range []int{sequence, l.strength[ChannelA], l.strength[ChannelB]} {
		encoded, err := boundedByte(value, MaxStrength)
		if err != nil {
			return
		}
		payload[index+1] = encoded
	}
	select {
	case l.events <- LinkEvent{Kind: LinkNotification, Payload: payload}:
	default:
	}
}

// TurnSwitch models the user changing strength on the device itself.
func (l *SimulatedLink) TurnSwitch(channel Channel, delta int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	before := l.strength[channel]
	l.strength[channel] = clampInt(before+delta, 0, l.ceiling[channel])
	if l.strength[channel] != before {
		l.reportLocked(0)
	}
}

// Lose ends the link as a dropped radio connection would (tests only).
func (l *SimulatedLink) Lose(cause error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	if cause == nil {
		cause = errors.New("simulated link lost")
	}
	l.events <- LinkEvent{Kind: LinkLost, Err: cause}
}

// ReadBattery reports a fixed charge.
func (l *SimulatedLink) ReadBattery(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return simulatedBattery, nil
}

// SetLastWill records the packet a worker would write on core loss.
func (l *SimulatedLink) SetLastWill(payload []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastWill = append([]byte(nil), payload...)
	return nil
}

// Events returns the notification channel.
func (l *SimulatedLink) Events() <-chan LinkEvent {
	return l.events
}

// Device identifies the simulator.
func (l *SimulatedLink) Device() DeviceInfo {
	return DeviceInfo{Name: DeviceName, Address: simulatedAddress}
}

// Close ends the link.
func (l *SimulatedLink) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

// Snapshot returns the modeled device state.
func (l *SimulatedLink) Snapshot() SimulatedState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return SimulatedState{
		StrengthA:  l.strength[ChannelA],
		StrengthB:  l.strength[ChannelB],
		CeilingA:   l.ceiling[ChannelA],
		CeilingB:   l.ceiling[ChannelB],
		Packets:    l.packets,
		Limits:     l.limits,
		OutputA:    l.output[ChannelA],
		OutputB:    l.output[ChannelB],
		LastWill:   len(l.lastWill) > 0,
		Closed:     l.closed,
		LastPacket: l.last,
	}
}
