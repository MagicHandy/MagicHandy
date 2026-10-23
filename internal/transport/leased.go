package transport

import (
	"context"
	"errors"
	"time"
)

// A lease binds an engine's transport to one connection incarnation. Replacing
// the connection never lets delayed work reach the newly connected device.
type leasedTransport struct {
	owner   Transport
	current func() bool
}
type leasedStartupTransport struct {
	*leasedTransport
	startup MotionStartupStateProvider
}

// NewLeasedTransport fences commands to one connection incarnation.
func NewLeasedTransport(owner Transport, current func() bool) Transport {
	lease := &leasedTransport{owner: owner, current: current}
	if startup, ok := owner.(MotionStartupStateProvider); ok {
		return &leasedStartupTransport{lease, startup}
	}
	return lease
}

func (l *leasedTransport) valid() error {
	if l.current == nil || !l.current() {
		return errors.New("handy connection changed; start a new session")
	}
	return nil
}
func (l *leasedTransport) Stop(ctx context.Context, c StopCommand) (CommandResult, error) {
	if l.valid() != nil {
		return CommandResult{OK: true, Status: "connection_retired"}, nil
	}
	return l.owner.Stop(ctx, c)
}
func (l *leasedTransport) SetStrokeWindow(ctx context.Context, c StrokeWindowCommand) (CommandResult, error) {
	if err := l.valid(); err != nil {
		return CommandResult{}, err
	}
	return l.owner.SetStrokeWindow(ctx, c)
}
func (l *leasedTransport) AppendPoints(ctx context.Context, c AppendPointsCommand) (CommandResult, error) {
	if err := l.valid(); err != nil {
		return CommandResult{}, err
	}
	return l.owner.AppendPoints(ctx, c)
}
func (l *leasedTransport) Play(ctx context.Context, c PlayCommand) (CommandResult, error) {
	if err := l.valid(); err != nil {
		return CommandResult{}, err
	}
	return l.owner.Play(ctx, c)
}
func (l *leasedTransport) PlayAligned(ctx context.Context, c PlayCommand, start time.Time) (CommandResult, error) {
	if err := l.valid(); err != nil {
		return CommandResult{}, err
	}
	return playCompanion(ctx, l.owner, c, start)
}
func (l *leasedTransport) Diagnostics() TransportDiagnostics {
	d := l.owner.Diagnostics()
	if l.valid() != nil {
		d.Connected = false
		d.PlaybackState = "disconnected"
	}
	return d
}
func (l *leasedTransport) PlaybackStartTime() time.Time {
	if p, ok := l.owner.(PlaybackStartTimeProvider); ok {
		return p.PlaybackStartTime()
	}
	return time.Time{}
}
func (l *leasedTransport) MotionTimingCapabilities() MotionTimingCapabilities {
	return timingOf(l.owner)
}
func (l *leasedTransport) MotionSamplingCapabilities() MotionSamplingCapabilities {
	if p, ok := l.owner.(MotionSamplingCapabilitiesProvider); ok {
		return p.MotionSamplingCapabilities()
	}
	return MotionSamplingCapabilities{}
}
func (l *leasedStartupTransport) ReadMotionStartupState(ctx context.Context) (MotionStartupState, MotionStartupStateResults, error) {
	if err := l.valid(); err != nil {
		return MotionStartupState{}, MotionStartupStateResults{}, err
	}
	state, results, err := l.startup.ReadMotionStartupState(ctx)
	if err == nil {
		err = l.valid()
	}
	return state, results, err
}
