package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

const (
	coyoteTransportName          = "coyote_ble"
	coyoteSimulatedTransportName = "coyote_simulated"
	coyotePacketInterval         = estim.PacketMillis * time.Millisecond
	coyoteConnectTimeout         = 6 * time.Second
	coyoteScanTimeout            = 12 * time.Second
	coyoteWriteTimeout           = 500 * time.Millisecond
	coyoteBatteryTimeout         = 2 * time.Second
)

var (
	errCoyoteNotConnected     = errors.New("connect the Coyote first")
	errCoyoteAlreadyConnected = errors.New("the Coyote is already connected")
	errCoyoteClosed           = errors.New("the Coyote owner is closed")
	errCoyoteSuperseded       = errors.New("the Coyote packet was superseded by Stop")
	errCoyoteNotReady         = errors.New("wait for the Coyote strength and limits before starting output")
	errCoyoteTestWhilePlaying = errors.New("pause motion before testing a Coyote channel")
)

var _ Transport = (*Coyote)(nil)

// CoyoteConfig is the user configuration a connection applies: the device
// ceilings and balance parameters (BF), the conversion profile and timing.
type CoyoteConfig struct {
	Limits            estim.Limits
	Profile           estim.Profile
	OffsetMillis      int
	RememberedAddress string
}

// CoyoteStatus is a safe connection, strength and output snapshot. Output
// levels are the pulse widths MagicHandy last commanded, not a measurement.
type CoyoteStatus struct {
	Connected             bool   `json:"connected"`
	Connecting            bool   `json:"connecting"`
	Simulated             bool   `json:"simulated"`
	DeviceName            string `json:"device_name,omitempty"`
	Address               string `json:"address,omitempty"`
	BatteryPercent        *int   `json:"battery_percent,omitempty"`
	StrengthA             int    `json:"strength_a"`
	StrengthB             int    `json:"strength_b"`
	StrengthKnown         bool   `json:"strength_known"`
	StrengthSource        string `json:"strength_source"`
	StrengthPending       bool   `json:"strength_pending"`
	LimitsApplied         bool   `json:"limits_applied"`
	CeilingA              int    `json:"ceiling_a"`
	CeilingB              int    `json:"ceiling_b"`
	PlaybackState         string `json:"playback_state"`
	ReferencePulse        bool   `json:"reference_pulse"`
	CommandedA            int    `json:"commanded_a"`
	CommandedB            int    `json:"commanded_b"`
	PacketsSent           uint64 `json:"packets_sent"`
	WriteFailures         uint64 `json:"write_failures"`
	StarvedPackets        uint64 `json:"starved_packets"`
	MaxSendLatenessMillis int64  `json:"max_send_lateness_ms"`
	LastError             string `json:"last_error,omitempty"`
}

// CoyoteOptions configures the owner.
type CoyoteOptions struct {
	Connector estim.Connector
	Clock     func() time.Time
	// Tick overrides the pacer interval in tests.
	Tick time.Duration
}

// Coyote owns one DG-LAB Coyote 3.0 connection. It is a dispatch owner like
// the Handy owners: the engine's neutral frame enters through AppendPoints
// and leaves as two-channel pulse frames through a fixed 100 ms pacer. It
// also owns the user's strength handshake, which nothing automated can raise.
type Coyote struct {
	connector estim.Connector
	clock     func() time.Time
	tick      time.Duration

	opMu    sync.Mutex
	writeMu sync.Mutex

	mu            sync.Mutex
	closed        bool
	connecting    bool
	link          estim.Link
	device        estim.DeviceInfo
	battery       *int
	config        CoyoteConfig
	appliedLimits estim.Limits
	limitsApplied bool
	zeroWrites    int
	stopHolds     int
	strength      *estim.StrengthController
	stream        coyoteStream
	reference     coyoteReference
	generation    uint64
	sessionID     uint64
	stats         coyoteStats
	state         string
	lastError     string
	diag          coyoteDiagnostics
	loops         *coyoteLoops
}

type coyoteStats struct {
	packets       uint64
	writeFailures uint64
	starved       uint64
	maxLateness   time.Duration
	commanded     [2]int
}

type coyoteReference struct {
	active  bool
	channel estim.Channel
	index   int
}

type coyoteLoops struct {
	stop chan struct{}
	done sync.WaitGroup
}

// NewCoyote returns a disconnected owner.
func NewCoyote(options CoyoteOptions) *Coyote {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Tick <= 0 {
		options.Tick = coyotePacketInterval
	}
	return &Coyote{
		connector: options.Connector,
		clock:     options.Clock,
		tick:      options.Tick,
		strength:  estim.NewStrengthController(0, 0),
		stream:    newCoyoteStream(estim.Profile{}),
		state:     "disconnected",
	}
}

// Configure applies user settings. A connected device receives the new BF
// ceilings immediately; the profile and offset apply from the next packet.
func (c *Coyote) Configure(config CoyoteConfig) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	limitsChanged := !c.limitsApplied || c.appliedLimits != config.Limits
	c.config = config
	c.strength.SetCeilings(config.Limits.StrengthA, config.Limits.StrengthB)
	c.stream.configure(config.Profile, config.OffsetMillis)
	link := c.link
	if limitsChanged {
		c.limitsApplied = false
		c.generation++
	}
	c.mu.Unlock()
	if link == nil || !limitsChanged {
		return nil
	}
	if err := c.writeLimits(link); err != nil {
		// A lower saved ceiling must never coexist with continued output at
		// the old hardware limit. Reconnect will reapply the requested limits.
		zeroErr := c.zero(link)
		c.release(link, err.Error())
		return errors.Join(err, zeroErr)
	}
	return nil
}

// Connect opens the device, re-sends the ceilings (the device does not
// confirm BF, so every connection writes it) and sets both channels to zero:
// a new connection always starts disarmed.
func (c *Coyote) Connect(ctx context.Context) (CoyoteStatus, error) {
	return c.connect(ctx, "")
}

// ConnectAddress connects only the explicitly selected discovery candidate.
// Service verification occurs before arm can write limits or zero commands.
func (c *Coyote) ConnectAddress(ctx context.Context, address string) (CoyoteStatus, error) {
	if address == "" {
		return c.Status(), estim.ErrNotFound
	}
	return c.connect(ctx, address)
}

func (c *Coyote) connect(ctx context.Context, selected string) (CoyoteStatus, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	switch {
	case c.closed:
		c.mu.Unlock()
		return c.Status(), errCoyoteClosed
	case c.link != nil:
		c.mu.Unlock()
		return c.Status(), errCoyoteAlreadyConnected
	case c.connector == nil:
		c.mu.Unlock()
		return c.Status(), estim.ErrHelperMissing
	}
	c.connecting = true
	remembered := c.config.RememberedAddress
	if selected != "" {
		remembered = selected
	}
	c.mu.Unlock()

	link, err := c.connector.Connect(ctx, estim.ConnectRequest{RememberedAddress: remembered, SelectedAddress: selected != "", ConnectTimeout: coyoteConnectTimeout, ScanTimeout: coyoteScanTimeout})
	if err != nil {
		c.mu.Lock()
		c.connecting = false
		c.lastError = err.Error()
		c.mu.Unlock()
		return c.Status(), err
	}
	if err := ctx.Err(); err != nil {
		_ = link.Close()
		c.mu.Lock()
		c.connecting = false
		c.mu.Unlock()
		return c.Status(), err
	}
	if err := c.arm(ctx, link); err != nil {
		c.release(link, err.Error())
		return c.Status(), err
	}
	return c.Status(), nil
}

func (c *Coyote) arm(ctx context.Context, link estim.Link) error {
	c.mu.Lock()
	c.connecting = false
	c.link = link
	c.sessionID++
	c.limitsApplied = false
	c.device = link.Device()
	c.generation++
	c.strength.Reset()
	c.stream.stop()
	c.reference = coyoteReference{}
	c.state = "idle"
	c.lastError = ""
	loops := &coyoteLoops{stop: make(chan struct{})}
	c.loops = loops
	c.mu.Unlock()

	loops.done.Add(2)
	go c.events(link, loops)
	go c.pace(loops)
	if err := c.writeLimits(link); err != nil {
		return errors.Join(fmt.Errorf("write Coyote ceilings: %w", err), c.zero(link))
	}
	if err := c.zero(link); err != nil {
		return fmt.Errorf("set Coyote strength to zero: %w", err)
	}
	if lastWill, err := zeroPacket(0); err == nil {
		_ = link.SetLastWill(lastWill)
	}
	c.readBattery(ctx, link)
	return nil
}

// Disconnect queues a zero before closing the link. Write Without Response
// cannot establish delivery; a device report is the only confirmed strength.
func (c *Coyote) Disconnect() (CoyoteStatus, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	link := c.link
	c.mu.Unlock()
	if link == nil {
		return c.Status(), nil
	}
	err := c.zero(link)
	c.release(link, "")
	return c.Status(), err
}

// Close disconnects and refuses later connections.
func (c *Coyote) Close() error {
	_, err := c.Disconnect()
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return err
}

// release tears a link down and stops its goroutines. It is idempotent per
// link and safe to call from the event loop's loss handling.
func (c *Coyote) release(link estim.Link, reason string) {
	c.mu.Lock()
	if c.link != link {
		c.mu.Unlock()
		return
	}
	c.link = nil
	c.sessionID++
	c.limitsApplied = false
	c.connecting = false
	c.generation++
	c.stream.stop()
	c.reference = coyoteReference{}
	c.stats.commanded = [2]int{}
	c.strength.Reset()
	c.battery = nil
	c.state = "disconnected"
	if reason != "" {
		c.lastError = reason
		c.state = "stale"
	}
	loops := c.loops
	c.loops = nil
	c.mu.Unlock()
	if loops != nil {
		close(loops.stop)
	}
	_ = link.Close()
	if loops != nil {
		loops.done.Wait()
	}
}

// AdjustStrength only queues a strength step. It never starts waveform output.
func (c *Coyote) AdjustStrength(channel estim.Channel, delta int) (CoyoteStatus, error) {
	c.mu.Lock()
	err := c.adjustStrengthLocked(context.Background(), c.generation, channel, delta)
	c.mu.Unlock()
	return c.Status(), err
}

// EmergencyZero is the Coyote half of the global Stop: it invalidates every
// queued packet, ends any stream or reference pulse and sets both channels
// to zero. The zero is never superseded by an ordinary pause/Stop packet.
func (c *Coyote) EmergencyZero() error {
	c.mu.Lock()
	link := c.link
	c.mu.Unlock()
	if link == nil {
		return nil
	}
	err := c.zero(link)
	if err != nil {
		c.release(link, err.Error())
	}
	return err
}

// zero invalidates every packet composed so far and writes an acknowledged
// zero for both channels. The generation bump and the zero's composition
// share one critical section: an older packet still mid-write lands first,
// and anything composed later carries no strength change while the zero is
// in flight, so nothing can raise strength after it.
func (c *Coyote) zero(link estim.Link) error {
	c.mu.Lock()
	if c.link != link {
		c.mu.Unlock()
		return errCoyoteNotConnected
	}
	c.generation++
	c.zeroWrites++
	c.stream.stop()
	c.reference = coyoteReference{}
	c.stats.commanded = [2]int{}
	if c.state != "disconnected" {
		c.state = "idle"
	}
	c.strength.RequestZero()
	changeA, changeB, sequence := c.strength.Next(c.clock())
	c.mu.Unlock()
	payload, err := encodePacket(estim.Packet{Sequence: sequence, A: changeA, B: changeB, Frames: silentFrames()})
	if err == nil {
		// A zero is a safety write. An overlapping engine Stop may invalidate
		// motion packets, but must not invalidate the zero itself.
		err = c.writeChecked(link, payload, true, func() bool { return true })
	}
	c.mu.Lock()
	c.zeroWrites--
	if c.link == link {
		if err == nil && !link.Device().UnacknowledgedWrites {
			c.strength.ZeroWritten(sequence)
		} else if err != nil {
			c.strength.Reset()
		}
	}
	c.mu.Unlock()
	return err
}

// writeLimits sends the ceilings. It is not tied to a playback generation: a
// Stop racing a ceiling change must never leave the device's own ceiling
// above the one MagicHandy enforces.
func (c *Coyote) writeLimits(link estim.Link) error {
	c.mu.Lock()
	limits := c.config.Limits
	c.mu.Unlock()
	encoded, err := limits.Encode()
	if err != nil {
		return err
	}
	if err := c.writeChecked(link, encoded[:], true, func() bool { return true }); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.link != link {
		return errCoyoteNotConnected
	}
	c.appliedLimits = limits
	c.limitsApplied = c.config.Limits == limits
	return nil
}

// write sends a packet composed for one playback generation. It re-checks the
// generation under writeMu, so a packet composed before a Stop can never
// follow the Stop's own packet.
func (c *Coyote) write(generation uint64, link estim.Link, payload []byte, acknowledged bool) error {
	return c.writeChecked(link, payload, acknowledged, func() bool { return c.generation == generation })
}

// writeChecked is the only path to the wire. current runs under mu while
// writeMu is held.
func (c *Coyote) writeChecked(link estim.Link, payload []byte, acknowledged bool, current func() bool) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	valid := c.link == link && current()
	c.mu.Unlock()
	if !valid {
		return errCoyoteSuperseded
	}
	ctx, cancel := context.WithTimeout(context.Background(), coyoteWriteTimeout)
	defer cancel()
	err := link.Write(ctx, payload, acknowledged)
	c.mu.Lock()
	if err != nil {
		c.stats.writeFailures++
		c.lastError = err.Error()
	} else {
		c.stats.packets++
	}
	c.mu.Unlock()
	return err
}

func (c *Coyote) readBattery(ctx context.Context, link estim.Link) {
	ctx, cancel := context.WithTimeout(ctx, coyoteBatteryTimeout)
	defer cancel()
	if percent, err := link.ReadBattery(ctx); err == nil {
		c.mu.Lock()
		if c.link == link {
			c.battery = &percent
		}
		c.mu.Unlock()
	}
}

// events applies device reports until the link ends or is released.
func (c *Coyote) events(link estim.Link, loops *coyoteLoops) {
	defer loops.done.Done()
	for {
		select {
		case <-loops.stop:
			return
		case event, ok := <-link.Events():
			if !ok {
				go c.release(link, "the Coyote connection ended")
				return
			}
			if lost := c.apply(link, event); lost != "" {
				go c.release(link, lost)
				return
			}
		}
	}
}

func (c *Coyote) apply(link estim.Link, event estim.LinkEvent) string {
	switch event.Kind {
	case estim.LinkNotification:
		report, ok, err := estim.DecodeNotification(event.Payload)
		if err != nil || !ok {
			return ""
		}
		c.mu.Lock()
		if c.link == link {
			c.strength.Report(report)
		}
		c.mu.Unlock()
	case estim.LinkBattery:
		percent := event.Battery
		c.mu.Lock()
		if c.link == link {
			c.battery = &percent
		}
		c.mu.Unlock()
	case estim.LinkLost, estim.LinkWriteFailed:
		if event.Err != nil {
			return event.Err.Error()
		}
		return "the Coyote connection was lost"
	}
	return ""
}

// Status returns a safe snapshot.
func (c *Coyote) Status() CoyoteStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	strength := c.strength.Snapshot()
	status := CoyoteStatus{
		Connected:             c.link != nil,
		Connecting:            c.connecting,
		Simulated:             c.connector != nil && c.connector.Simulated(),
		StrengthA:             strength.A,
		StrengthB:             strength.B,
		StrengthKnown:         strength.Known,
		StrengthSource:        strength.Source,
		StrengthPending:       strength.Pending,
		LimitsApplied:         c.limitsApplied,
		CeilingA:              c.config.Limits.StrengthA,
		CeilingB:              c.config.Limits.StrengthB,
		PlaybackState:         c.state,
		ReferencePulse:        c.reference.active,
		CommandedA:            c.stats.commanded[estim.ChannelA],
		CommandedB:            c.stats.commanded[estim.ChannelB],
		PacketsSent:           c.stats.packets,
		WriteFailures:         c.stats.writeFailures,
		StarvedPackets:        c.stats.starved,
		MaxSendLatenessMillis: c.stats.maxLateness.Milliseconds(),
		LastError:             c.lastError,
	}
	if c.link != nil {
		status.CeilingA = c.appliedLimits.StrengthA
		status.CeilingB = c.appliedLimits.StrengthB
		status.DeviceName = c.device.Name
		status.Address = c.device.Address
	}
	if c.stopHolds != 0 {
		status.PlaybackState = "stopping"
	}
	if c.battery != nil {
		percent := *c.battery
		status.BatteryPercent = &percent
	}
	return status
}

func encodePacket(packet estim.Packet) ([]byte, error) {
	encoded, err := packet.Encode()
	if err != nil {
		return nil, err
	}
	return encoded[:], nil
}

func silentFrames() [estim.FramesPerPacket]estim.Frame {
	return [estim.FramesPerPacket]estim.Frame{estim.SilentFrame, estim.SilentFrame, estim.SilentFrame, estim.SilentFrame}
}

// zeroPacket is the last will: both channels set to zero, nothing playing.
func zeroPacket(sequence int) ([]byte, error) {
	set := estim.StrengthChange{Mode: estim.StrengthSet}
	return encodePacket(estim.Packet{Sequence: sequence, A: set, B: set, Frames: silentFrames()})
}
