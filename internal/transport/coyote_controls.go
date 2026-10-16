package transport

import (
	"context"
	"errors"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

// ControlGeneration fences a control request admitted before Stop, disconnect
// or a ceiling transition, including requests still reading their HTTP body.
func (c *Coyote) ControlGeneration() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

// AdjustStrengthAt applies a host request only in the generation that admitted
// it. Lowering remains available as a safety action and never starts output.
func (c *Coyote) AdjustStrengthAt(ctx context.Context, generation uint64, channel estim.Channel, delta int) (CoyoteStatus, error) {
	c.mu.Lock()
	err := c.adjustStrengthLocked(ctx, generation, channel, delta)
	c.mu.Unlock()
	return c.Status(), err
}

func (c *Coyote) adjustStrengthLocked(ctx context.Context, generation uint64, channel estim.Channel, delta int) error {
	if c.link == nil {
		return errCoyoteNotConnected
	}
	if delta > 0 {
		if ctx.Err() != nil || c.generation != generation {
			return errCoyoteSuperseded
		}
		if !c.limitsApplied || c.zeroWrites != 0 || c.stopHolds != 0 {
			return estim.ErrStrengthUnconfirmed
		}
	}
	return c.strength.Step(channel, delta)
}

// TestChannel starts the existing bounded calibration waveform only on an
// explicit host action. The shared dispatch pacer owns its timing, and Stop
// invalidates it just like a motion packet. Strength is never changed here.
func (c *Coyote) TestChannel(ctx context.Context, generation uint64, channel estim.Channel) (CoyoteStatus, error) {
	c.mu.Lock()
	err := c.startReferenceLocked(ctx, generation, channel)
	c.mu.Unlock()
	return c.Status(), err
}

func (c *Coyote) startReferenceLocked(ctx context.Context, generation uint64, channel estim.Channel) error {
	if channel != estim.ChannelA && channel != estim.ChannelB {
		return errors.New("unknown Coyote channel")
	}
	if ctx.Err() != nil || generation != c.generation {
		return errCoyoteSuperseded
	}
	if c.link == nil {
		return errCoyoteNotConnected
	}
	strength := c.strength.Snapshot()
	if !c.limitsApplied || c.zeroWrites != 0 || c.stopHolds != 0 || !strength.Known || strength.Pending {
		return errCoyoteNotReady
	}
	if c.stream.playing() {
		return errCoyoteTestWhilePlaying
	}
	c.reference = coyoteReference{active: true, channel: channel}
	return nil
}

// BeginEmergencyStop closes admission immediately and keeps it closed until
// the server has stopped every output. The returned function waits for zero,
// releases this hold, and reports its result. Zero can run beside a slow Handy
// Stop without allowing a fresh increase or test in that interval.
func (c *Coyote) BeginEmergencyStop() func() error {
	c.mu.Lock()
	c.stopHolds++
	c.generation++
	c.stream.stop()
	c.reference = coyoteReference{}
	c.strength.CancelPending()
	c.stats.commanded = [2]int{}
	c.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- c.EmergencyZero() }()
	return func() error {
		err := <-done
		c.mu.Lock()
		c.stopHolds--
		c.mu.Unlock()
		return err
	}
}
