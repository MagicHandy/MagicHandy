package transport

import (
	"context"
	"errors"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

var errCoyoteSessionEnded = errors.New("the Coyote connection changed; start a new playback session")

type coyoteSessionKey struct{}

// coyoteSession binds an engine to one physical link. Reconnecting the owner
// must not let an old engine resume appending, playing, aligning or stopping
// the new connection. Validation and mutation share the owner's mutex.
type coyoteSession struct {
	owner *Coyote
	link  estim.Link
}

// SessionID changes only at link boundaries, independently of pause/Stop.
func (c *Coyote) SessionID() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// MotionTransport admits a new engine only after limits and strength are known.
// A known zero is a valid, silent baseline, not a readiness failure.
func (c *Coyote) MotionTransport() (Transport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.link == nil {
		return nil, errCoyoteNotConnected
	}
	if !c.playbackReadyLocked() {
		return nil, errCoyoteNotReady
	}
	return &coyoteSession{owner: c, link: c.link}, nil
}

func (c *Coyote) playbackReadyLocked() bool {
	strength := c.strength.Snapshot()
	return c.link != nil && c.limitsApplied && c.zeroWrites == 0 && c.stopHolds == 0 && strength.Known && !strength.Pending
}

func (c *Coyote) checkSessionLocked(ctx context.Context) error {
	if link, ok := ctx.Value(coyoteSessionKey{}).(estim.Link); ok && link != c.link {
		return errCoyoteSessionEnded
	}
	return nil
}

func (s *coyoteSession) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, coyoteSessionKey{}, s.link)
}

func (s *coyoteSession) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	return s.owner.AppendPoints(s.context(ctx), command)
}

func (s *coyoteSession) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	return s.owner.Play(s.context(ctx), command)
}

func (s *coyoteSession) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	s.owner.mu.Lock()
	current := s.owner.link == s.link
	s.owner.mu.Unlock()
	if !current {
		return CommandResult{OK: true, Status: "connection_retired"}, nil
	}
	return s.owner.Stop(s.context(ctx), command)
}

func (s *coyoteSession) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	return s.owner.SetStrokeWindow(s.context(ctx), command)
}

func (s *coyoteSession) Diagnostics() TransportDiagnostics {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	d := s.owner.diag.snapshot()
	d.Name = s.owner.transportName()
	d.Connected = s.owner.link == s.link
	d.PlaybackState = s.owner.state
	if s.owner.stopHolds > 0 {
		d.PlaybackState = "stopping"
		return d
	}
	if !d.Connected || s.owner.stream.playing() && (!s.owner.limitsApplied || !s.owner.strength.Snapshot().Known) {
		d.PlaybackState = "stale"
		d.LastError = "Coyote playback readiness lost; start a new playback session"
	}
	return d
}

func (s *coyoteSession) MotionTimingCapabilities() MotionTimingCapabilities {
	return s.owner.MotionTimingCapabilities()
}

func (s *coyoteSession) PlaybackStartTime() time.Time {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.owner.link != s.link {
		return time.Time{}
	}
	return s.owner.stream.anchor
}

func (s *coyoteSession) AlignPlaybackStart(start time.Time) {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	if s.owner.link == s.link && s.owner.stream.playing() && !start.IsZero() {
		s.owner.stream.anchor = start
		s.owner.stream.started = false
	}
}
