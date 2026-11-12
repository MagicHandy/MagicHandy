package transport

import (
	"context"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
)

// autoblowOwner is what a session needs from either Autoblow owner.
type autoblowOwner interface {
	Transport
	AlignedPlayer
	playbackAligner
	PlaybackStartTimeProvider
	MotionTimingCapabilitiesProvider
	MotionSamplingCapabilitiesProvider
	alignPlaybackStart(*autoblow.Client, time.Time)
	shared() *autoblowBase
}

func (b *autoblowBase) shared() *autoblowBase {
	return b
}

// autoblowSession binds an engine to one cloud connection. Reconnecting must
// not let an old engine append, play, align or stop through the new one.
type autoblowSession struct {
	owner  autoblowOwner
	client *autoblow.Client
}

func (s *autoblowSession) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, autoblowSessionKey{}, s.client)
}

func (s *autoblowSession) current() (current, lost bool) {
	base := s.owner.shared()
	base.mu.Lock()
	defer base.mu.Unlock()
	return base.client == s.client, base.client == nil && base.state == "stale"
}

func (s *autoblowSession) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	return s.owner.AppendPoints(s.context(ctx), command)
}

func (s *autoblowSession) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	return s.owner.Play(s.context(ctx), command)
}

func (s *autoblowSession) PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	return s.owner.PlayAligned(s.context(ctx), command, start)
}

func (s *autoblowSession) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	return s.owner.SetStrokeWindow(s.context(ctx), command)
}

// Stop reaches the device only through the connection this engine was built
// for. A lost connection cannot confirm Stop.
func (s *autoblowSession) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	current, lost := s.current()
	if !current {
		if lost {
			return CommandResult{Kind: CommandKindStop, Status: "unconfirmed", Error: errAutoblowStale.Error()}, errAutoblowStale
		}
		return CommandResult{OK: true, Status: "connection_retired"}, nil
	}
	return s.owner.Stop(s.context(ctx), command)
}

func (s *autoblowSession) Diagnostics() TransportDiagnostics {
	diagnostics := s.owner.Diagnostics()
	if current, _ := s.current(); !current {
		diagnostics.Connected = false
		diagnostics.PlaybackState = "stale"
		diagnostics.LastError = "the device connection changed; start a new playback session"
	}
	return diagnostics
}

func (s *autoblowSession) MotionTimingCapabilities() MotionTimingCapabilities {
	return s.owner.MotionTimingCapabilities()
}

func (s *autoblowSession) MotionSamplingCapabilities() MotionSamplingCapabilities {
	return s.owner.MotionSamplingCapabilities()
}

func (s *autoblowSession) PlaybackStartTime() time.Time {
	if current, _ := s.current(); !current {
		return time.Time{}
	}
	return s.owner.PlaybackStartTime()
}

func (s *autoblowSession) AlignPlaybackStart(start time.Time) {
	// Check connection identity under the same lock as the clock change;
	// reconnect may retire this session while alignment waits for that lock.
	s.owner.alignPlaybackStart(s.client, start)
}
