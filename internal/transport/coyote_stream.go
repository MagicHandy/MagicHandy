package transport

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/estim"
)

const (
	// coyoteSendLead is the expected pipe and radio time from a write to the
	// device playing it; each packet renders the frame that far ahead.
	coyoteSendLead = 40 * time.Millisecond
	// coyotePreroll places stream time 0 slightly after Play so the first
	// packet is never already late.
	coyotePreroll = 120 * time.Millisecond
	// coyoteStarveAfter is how long uncovered output may last before the
	// owner reports starvation (Intiface uses the same stop-and-report rule).
	coyoteStarveAfter      = 750 * time.Millisecond
	coyoteMinimumLead      = 500 * time.Millisecond
	coyoteMediaLead        = time.Second
	coyotePruneMillis      = 1000
	coyoteMaxOffsetMillis  = 300
	coyoteResyncMillis     = estim.PacketMillis
	coyoteDiagnosticPrefix = "coyote-"
)

// coyoteStream is the playing stream's copy of the neutral frame and the
// renderer state. Stream milliseconds map to local time through anchor.
type coyoteStream struct {
	renderer       *estim.Renderer
	timeline       estim.Timeline
	offsetMillis   float64
	id             string
	anchor         time.Time
	next           float64
	started        bool
	lastTick       time.Time
	uncoveredSince time.Time
}

func newCoyoteStream(profile estim.Profile) coyoteStream {
	return coyoteStream{renderer: estim.NewRenderer(profile)}
}

func (s *coyoteStream) configure(profile estim.Profile, offsetMillis int) {
	s.renderer.SetProfile(profile)
	s.offsetMillis = float64(max(-coyoteMaxOffsetMillis, min(coyoteMaxOffsetMillis, offsetMillis)))
}

func (s *coyoteStream) stop() {
	s.timeline.Reset()
	s.id = ""
	s.anchor = time.Time{}
	s.started = false
	s.lastTick = time.Time{}
	s.uncoveredSince = time.Time{}
}

func (s *coyoteStream) playing() bool {
	return !s.anchor.IsZero()
}

// AppendPoints stores the stream's neutral points. The owner only samples
// them; it never reshapes the frame.
func (c *Coyote) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	points := make([]estim.Point, len(command.Points))
	for index, point := range command.Points {
		points[index] = estim.Point{TimeMillis: point.TimeMillis, Position: point.PositionPercent}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.checkSessionLocked(ctx); err != nil {
		return c.recordLocked(Command{Kind: CommandKindPointsAdd}, err)
	}
	if c.link == nil {
		return c.recordLocked(Command{Kind: CommandKindPointsAdd}, errCoyoteNotConnected)
	}
	if command.StreamID != c.stream.id {
		c.stream.stop()
		c.stream.id = command.StreamID
	}
	err := c.stream.timeline.Append(points)
	return c.recordLocked(Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}, err)
}

// Play starts the prepared stream. Output fades in from its first frame.
func (c *Coyote) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	recorded := Command{Kind: CommandKindPointsPlay, PointsPlay: &command}
	if err := ctx.Err(); err != nil {
		return c.recordLocked(recorded, err)
	}
	if err := c.checkSessionLocked(ctx); err != nil {
		return c.recordLocked(recorded, err)
	}
	if c.link == nil {
		return c.recordLocked(recorded, errCoyoteNotConnected)
	}
	if !c.playbackReadyLocked() {
		return c.recordLocked(recorded, errCoyoteNotReady)
	}
	if command.StreamID != c.stream.id {
		return c.recordLocked(recorded, fmt.Errorf("the Coyote stream %q has no points", command.StreamID))
	}
	c.stream.anchor = c.clock().Add(coyotePreroll - time.Duration(command.StartTimeMillis)*time.Millisecond)
	c.stream.started = false
	c.stream.uncoveredSince = time.Time{}
	c.stream.renderer.Restart(float64(command.StartTimeMillis))
	c.reference = coyoteReference{}
	c.state = "playing"
	return c.recordLocked(recorded, nil)
}

// Stop ends the stream and silences the device. It never touches strength:
// Pause also arrives here, and the global Stop zeroes through EmergencyZero.
func (c *Coyote) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	c.mu.Lock()
	recorded := Command{Kind: CommandKindStop, Stop: &command}
	if err := c.checkSessionLocked(ctx); err != nil {
		c.mu.Unlock()
		return CommandResult{}, err
	}
	link := c.link
	c.generation++
	c.stream.stop()
	c.reference = coyoteReference{}
	c.strength.CancelPending()
	c.stats.commanded = [2]int{}
	if link != nil {
		c.state = "idle"
	}
	c.mu.Unlock()
	if link == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.recordLocked(recorded, nil)
	}
	if command.Fault {
		err := c.zero(link)
		if err != nil {
			c.release(link, err.Error())
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.recordLocked(recorded, err)
	}
	payload, err := encodePacket(estim.Packet{Frames: silentFrames()})
	if err == nil {
		err = c.writeChecked(link, payload, true, func() bool { return true })
	}
	if err != nil && !errors.Is(err, errCoyoteSuperseded) {
		c.release(link, err.Error())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recordLocked(recorded, err)
}

// SetStrokeWindow is accepted for contract compatibility. The Handy's
// physical window does not apply to stimulation: the profile reads the
// semantic span directly.
func (c *Coyote) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recordLocked(Command{Kind: CommandKindStrokeWindow, StrokeWindow: &command}, c.checkSessionLocked(ctx))
}

// MotionTimingCapabilities asks the engine to keep enough of the frame ahead
// for the render lead and the largest negative timing offset.
func (c *Coyote) MotionTimingCapabilities() MotionTimingCapabilities {
	return MotionTimingCapabilities{MinimumBufferedLead: coyoteMinimumLead, MinimumMediaBufferedLead: coyoteMediaLead}
}

// PlaybackStartTime reports the local time of stream millisecond 0.
func (c *Coyote) PlaybackStartTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream.anchor
}

// AlignPlaybackStart re-anchors a playing stream to another owner's start,
// so the Coyote follows the Handy when both run together.
func (c *Coyote) AlignPlaybackStart(start time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stream.playing() && !start.IsZero() {
		c.stream.anchor = start
		c.stream.started = false
	}
}

// Diagnostics returns the safe transport snapshot the engine reads for
// playback health.
func (c *Coyote) Diagnostics() TransportDiagnostics {
	c.mu.Lock()
	defer c.mu.Unlock()
	diagnostics := c.diag.snapshot()
	diagnostics.Name = c.transportName()
	diagnostics.Connected = c.link != nil
	diagnostics.PlaybackState = c.state
	diagnostics.LastError = redactedError(c.lastError)
	return diagnostics
}

func (c *Coyote) transportName() string {
	if c.connector != nil && c.connector.Simulated() {
		return coyoteSimulatedTransportName
	}
	return coyoteTransportName
}

func (c *Coyote) recordLocked(command Command, err error) (CommandResult, error) {
	result := c.diag.record(command, c.transportName(), c.clock(), err)
	return result, err
}

// pace sends one packet per tick while there is something to send: a
// playing stream, a reference pulse or a strength change. Idle ticks send
// nothing, and the device then plays nothing.
func (c *Coyote) pace(loops *coyoteLoops) {
	defer loops.done.Done()
	ticker := time.NewTicker(c.tick)
	defer ticker.Stop()
	for {
		select {
		case <-loops.stop:
			return
		case <-ticker.C:
			c.paceOnce()
		}
	}
}

func (c *Coyote) paceOnce() {
	c.mu.Lock()
	link := c.link
	if link == nil || !c.limitsApplied || c.zeroWrites != 0 || c.stopHolds != 0 {
		c.mu.Unlock()
		return
	}
	payload, ok := c.composeLocked(c.clock())
	generation := c.generation
	c.mu.Unlock()
	if !ok {
		return
	}
	if err := c.write(generation, link, payload, false); err != nil && !errors.Is(err, errCoyoteSuperseded) {
		// Release waits for this pacer's goroutine, so it must run outside it.
		go c.release(link, err.Error())
	}
}

func (c *Coyote) composeLocked(now time.Time) ([]byte, bool) {
	changeA, changeB, sequence := c.strength.Next(now)
	var frames [estim.FramesPerPacket]estim.Frame
	switch {
	case c.stream.playing() && !c.strength.Snapshot().Known:
		c.stream.stop()
		c.state = "stale"
		frames = silentFrames()
	case c.stream.playing():
		frames = c.renderStreamLocked(now)
	case c.reference.active:
		frames = estim.ReferencePacket(c.config.Profile, c.reference.channel, c.reference.index, estim.ReferencePackets)
		c.reference.index++
		if c.reference.index >= estim.ReferencePackets {
			c.reference = coyoteReference{}
		}
	case sequence != 0:
		frames = silentFrames()
	default:
		c.stats.commanded = [2]int{}
		return nil, false
	}
	payload, err := encodePacket(estim.Packet{Sequence: sequence, A: changeA, B: changeB, Frames: frames})
	if err != nil {
		c.lastError = err.Error()
		return nil, false
	}
	last := frames[estim.FramesPerPacket-1]
	c.stats.commanded = [2]int{last.A.Intensity, last.B.Intensity}
	return payload, true
}

// renderStreamLocked renders the next contiguous 100 ms of the stream. The
// window advances by exactly one packet per tick and resynchronizes only
// when the pacer drifts by more than a packet, so frames are neither skipped
// nor repeated in normal operation.
func (c *Coyote) renderStreamLocked(now time.Time) [estim.FramesPerPacket]estim.Frame {
	if !c.stream.lastTick.IsZero() {
		if late := now.Sub(c.stream.lastTick) - c.tick; late > c.stats.maxLateness {
			c.stats.maxLateness = late
		}
	}
	c.stream.lastTick = now
	ideal := float64(now.Add(coyoteSendLead).Sub(c.stream.anchor)) / float64(time.Millisecond)
	ideal -= c.stream.offsetMillis
	start := c.stream.next
	if !c.stream.started || math.Abs(ideal-start) > coyoteResyncMillis {
		start = ideal
	}
	c.stream.started = true
	c.stream.next = start + estim.PacketMillis
	frames, covered := c.stream.renderer.Packet(&c.stream.timeline, start)
	c.trackCoverageLocked(now, start, covered)
	c.stream.timeline.Prune(int64(start) - coyotePruneMillis)
	return frames
}

func (c *Coyote) trackCoverageLocked(now time.Time, start float64, covered int) {
	end, ok := c.stream.timeline.End()
	ranOut := covered < estim.FramesPerPacket && start >= 0 && (!ok || float64(end) < start+estim.PacketMillis)
	if !ranOut {
		c.stream.uncoveredSince = time.Time{}
		c.state = "playing"
		return
	}
	c.stats.starved++
	if c.stream.uncoveredSince.IsZero() {
		c.stream.uncoveredSince = now
	}
	c.state = "starving"
	if now.Sub(c.stream.uncoveredSince) >= coyoteStarveAfter {
		// Stop and report, like the Intiface pacer: the engine reads the
		// state and recovers; nothing further is sent meanwhile.
		c.state = "starved"
		c.stream.stop()
	}
}

// coyoteDiagnostics records the transport command history the engine and
// diagnostics read.
type coyoteDiagnostics struct {
	nextID      int
	count       int
	lastCommand *Command
	lastResult  *CommandResult
}

func (d *coyoteDiagnostics) record(command Command, name string, now time.Time, err error) CommandResult {
	d.nextID++
	d.count++
	command.ID = fmt.Sprintf("%s%06d", coyoteDiagnosticPrefix, d.nextID)
	command.IssuedAt = now.UTC().Format(time.RFC3339Nano)
	result := CommandResult{
		CommandID:   command.ID,
		Kind:        command.Kind,
		Transport:   name,
		OK:          err == nil,
		Status:      "accepted",
		CompletedAt: command.IssuedAt,
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	safe := SafeCommand(command)
	safeResult := SafeCommandResult(result)
	d.lastCommand, d.lastResult = &safe, &safeResult
	return result
}

func (d *coyoteDiagnostics) snapshot() TransportDiagnostics {
	diagnostics := TransportDiagnostics{CommandCount: d.count}
	if d.lastCommand != nil {
		command := cloneCommand(*d.lastCommand)
		diagnostics.LastCommand = &command
	}
	if d.lastResult != nil {
		result := *d.lastResult
		diagnostics.LastResult = &result
	}
	return diagnostics
}
