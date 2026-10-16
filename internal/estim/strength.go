package estim

import (
	"errors"
	"time"
)

// Channel identifies one of the two independent output channels.
type Channel int

// Output channels in device order.
const (
	ChannelA Channel = iota
	ChannelB
)

// A zero may produce no B1 when the device was already zero. Only a successful
// acknowledged write may establish that commanded baseline; the timer merely
// ends the wait for B1. A missing relative-change report invalidates strength.
const (
	zeroConfirmTimeout     = time.Second
	relativeConfirmTimeout = 1500 * time.Millisecond
)

// Strength sources distinguish a device report from an accepted zero command.
const (
	StrengthUnknown     = "unknown"
	StrengthReported    = "reported"
	StrengthZeroCommand = "zero_command"
)

var (
	// ErrStrengthUnconfirmed rejects an increase while the device's actual
	// strength is unknown.
	ErrStrengthUnconfirmed = errors.New("the Coyote has not confirmed its current strength yet")
	// ErrStrengthCeiling rejects an increase at the configured ceiling.
	ErrStrengthCeiling = errors.New("this channel is at its strength ceiling")
	errUnknownChannel  = errors.New("unknown channel")
)

type changeKind int

const (
	changeNone changeKind = iota
	changeRelative
	changeZero
)

// StrengthState is a snapshot of the handshake for status views.
type StrengthState struct {
	A        int
	B        int
	Known    bool
	Source   string
	CeilingA int
	CeilingB int
	Pending  bool
}

// StrengthController owns the user-driven strength handshake. It batches
// user steps, keeps one change in flight until the device's B1 report
// confirms it, never asks for more than the ceiling, and gives a zero request
// priority over every pending step. Automation never calls Step: channel
// strength changes only on an explicit user action or on the device itself.
type StrengthController struct {
	device          [2]int
	known           bool
	source          string
	ceiling         [2]int
	pending         [2]int
	inflight        int
	inflightKind    changeKind
	inflightAt      time.Time
	lastSequence    int
	zero            bool
	reports         uint64
	inflightReports uint64
}

// NewStrengthController starts with an unknown device strength.
func NewStrengthController(ceilingA, ceilingB int) *StrengthController {
	controller := &StrengthController{}
	controller.SetCeilings(ceilingA, ceilingB)
	return controller
}

// SetCeilings updates the per-channel ceilings and trims pending increases.
func (c *StrengthController) SetCeilings(ceilingA, ceilingB int) {
	c.ceiling = [2]int{clampInt(ceilingA, 0, MaxStrength), clampInt(ceilingB, 0, MaxStrength)}
	for channel := range c.pending {
		if c.known && c.pending[channel] > 0 {
			c.pending[channel] = min(c.pending[channel], max(0, c.ceiling[channel]-c.device[channel]))
		}
	}
}

// Step queues a user strength change of delta on channel.
func (c *StrengthController) Step(channel Channel, delta int) error {
	if channel != ChannelA && channel != ChannelB {
		return errUnknownChannel
	}
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		if !c.known || c.zero || c.inflightKind == changeZero {
			return ErrStrengthUnconfirmed
		}
		headroom := c.ceiling[channel] - c.device[channel] - c.pending[channel]
		if headroom <= 0 {
			return ErrStrengthCeiling
		}
		delta = min(delta, headroom)
	}
	c.pending[channel] += delta
	return nil
}

// RequestZero sets both channels to zero on the next packet, ahead of any
// queued or in-flight change.
func (c *StrengthController) RequestZero() {
	c.zero = true
	c.pending = [2]int{}
	c.known = false
	c.source = StrengthUnknown
}

// CancelPending discards user steps that have not reached the wire. A Stop
// must not turn a queued increase into a later background strength change.
func (c *StrengthController) CancelPending() {
	c.pending = [2]int{}
}

// ZeroWritten records successful GATT delivery, not a device measurement. A
// notification received since this zero was composed takes precedence, even
// if it was a physical-switch report with sequence zero. Matching B1 may also
// have completed the handshake before the write returned.
func (c *StrengthController) ZeroWritten(sequence int) {
	if c.inflightKind != changeZero || c.inflight != sequence || c.reports != c.inflightReports {
		return
	}
	c.device = [2]int{}
	c.known = true
	c.source = StrengthZeroCommand
}

// Next returns the strength instructions for the packet being composed.
// Sequence 0 means the packet carries no strength change.
func (c *StrengthController) Next(now time.Time) (StrengthChange, StrengthChange, int) {
	keep := StrengthChange{Mode: StrengthKeep}
	if c.zero {
		c.zero = false
		c.begin(changeZero, now)
		set := StrengthChange{Mode: StrengthSet, Value: 0}
		return set, set, c.inflight
	}
	if c.inflight != 0 {
		if !c.expire(now) {
			return keep, keep, 0
		}
	}
	changeA, changeB := c.relativeChange(ChannelA), c.relativeChange(ChannelB)
	c.pending = [2]int{}
	if changeA.Mode == StrengthKeep && changeB.Mode == StrengthKeep {
		return keep, keep, 0
	}
	c.begin(changeRelative, now)
	return changeA, changeB, c.inflight
}

// Pending reports whether a change is queued or still awaiting confirmation.
func (c *StrengthController) Pending() bool {
	return c.zero || c.inflight != 0 || c.pending != [2]int{}
}

func (c *StrengthController) relativeChange(channel Channel) StrengthChange {
	delta := c.pending[channel]
	if delta == 0 || delta > 0 && !c.known {
		return StrengthChange{Mode: StrengthKeep}
	}
	if c.known {
		current := c.device[channel]
		target := current + delta
		if delta > 0 {
			// Never above the ceiling, and never further above it when the
			// device already sits higher (a ceiling was just lowered).
			target = min(target, max(current, c.ceiling[channel]))
		}
		delta = clampInt(target, 0, MaxStrength) - current
	}
	switch {
	case delta > 0:
		return StrengthChange{Mode: StrengthIncrease, Value: delta}
	case delta < 0:
		return StrengthChange{Mode: StrengthDecrease, Value: min(-delta, MaxStrength)}
	default:
		return StrengthChange{Mode: StrengthKeep}
	}
}

func (c *StrengthController) begin(kind changeKind, now time.Time) {
	c.lastSequence = c.lastSequence%MaxSequence + 1
	c.inflight = c.lastSequence
	c.inflightKind = kind
	c.inflightAt = now
	c.inflightReports = c.reports
}

// expire clears a change whose report never arrived and reports whether the
// controller may send another one.
func (c *StrengthController) expire(now time.Time) bool {
	timeout := relativeConfirmTimeout
	if c.inflightKind == changeZero {
		timeout = zeroConfirmTimeout
	}
	if now.Sub(c.inflightAt) < timeout {
		return false
	}
	if c.inflightKind == changeRelative {
		c.known = false
		c.source = StrengthUnknown
		// These steps were accepted under the old, now uncertain baseline.
		// A later report must not silently resurrect them.
		for channel := range c.pending {
			c.pending[channel] = min(c.pending[channel], 0)
		}
	}
	c.inflight = 0
	c.inflightKind = changeNone
	return true
}

// Report applies a B1 notification. Every report carries the device's actual
// values; a matching sequence also completes the change in flight.
func (c *StrengthController) Report(report StrengthReport) {
	c.reports++
	c.device = [2]int{clampInt(report.A, 0, MaxStrength), clampInt(report.B, 0, MaxStrength)}
	c.known = true
	c.source = StrengthReported
	if report.Sequence != 0 && report.Sequence == c.inflight {
		c.inflight = 0
		c.inflightKind = changeNone
	}
}

// Reset forgets device state after a disconnect.
func (c *StrengthController) Reset() {
	ceiling := c.ceiling
	*c = StrengthController{ceiling: ceiling}
}

// Snapshot returns the current handshake state.
func (c *StrengthController) Snapshot() StrengthState {
	source := c.source
	if !c.known {
		source = StrengthUnknown
	}
	return StrengthState{
		A:        c.device[ChannelA],
		B:        c.device[ChannelB],
		Known:    c.known,
		Source:   source,
		CeilingA: c.ceiling[ChannelA],
		CeilingB: c.ceiling[ChannelB],
		Pending:  c.Pending(),
	}
}

func clampInt(value, low, high int) int {
	return max(low, min(high, value))
}
