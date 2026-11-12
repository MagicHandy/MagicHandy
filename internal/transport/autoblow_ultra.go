package transport

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
	"github.com/mapledaemon/MagicHandy/internal/companion"
)

const (
	// ultraFullSpeed is the Ultra's travel at go-to speed 100, in percent of
	// the stroke per second: Autoblow documents 400 units per second as the
	// fastest move its scripts can make.
	ultraFullSpeed = 400.0
	// ultraStartupSpeed brings the slider to the first point during preroll.
	ultraStartupSpeed = 30.0
	ultraPreroll      = 900 * time.Millisecond
	// ultraLookahead caps one go-to inside a long leg, so a retarget still
	// reaches the device within about half a second.
	ultraLookahead         = 600.0
	ultraMinimumMove       = 100 * time.Millisecond
	ultraPreferredInterval = 250 * time.Millisecond
	ultraTick              = 20 * time.Millisecond
	ultraStarveAfter       = 750 * time.Millisecond
	ultraMinimumLead       = 1500 * time.Millisecond
	ultraMediaLead         = 2 * time.Second
	ultraPruneMillis       = 2000
	ultraSamePosition      = 0.5
)

var _ Transport = (*AutoblowUltra)(nil)

// AutoblowUltra owns one Autoblow Ultra through Autoblow's cloud. It is a
// position owner like the Handy owners: the engine's neutral frame enters
// through AppendPoints and leaves as go-to commands, one per stroke leg, each
// timed to arrive as its leg begins (ADR 0069). The cloud is the only path to
// the device, so Stop reaches it only while the cloud does.
type AutoblowUltra struct {
	autoblowBase
	tick   time.Duration
	window StrokeWindowCommand
	stream ultraStream
	late   uint64
}

type ultraStream struct {
	timeline  companion.Timeline
	id        string
	origin    companion.Clock
	coverage  companion.Coverage
	started   bool
	sentUntil float64
	target    float64
	targetSet bool
}

func (s *ultraStream) stop() {
	s.timeline.Reset()
	s.id = ""
	s.origin.Stop()
	s.started = false
	s.sentUntil = 0
	s.coverage.Reset()
}

type ultraMove struct {
	position, speed float64
}

// NewAutoblowUltra returns a disconnected owner.
func NewAutoblowUltra(options AutoblowOptions) *AutoblowUltra {
	tick := options.Tick
	if tick <= 0 {
		tick = ultraTick
	}
	return &AutoblowUltra{
		autoblowBase: newAutoblowBase(autoblow.FamilyUltra, AutoblowUltraDevice, options),
		tick:         tick,
		window:       StrokeWindowCommand{MinPercent: 0, MaxPercent: 100},
		stream:       ultraStream{coverage: companion.NewCoverage(ultraStarveAfter)},
	}
}

// Configure applies the saved token and timing offset. A new token takes
// effect at the next Connect.
func (u *AutoblowUltra) Configure(config AutoblowConfig) {
	u.configureBase(config)
}

// Connect opens the cloud connection and stops the Ultra.
func (u *AutoblowUltra) Connect(ctx context.Context) (AutoblowStatus, error) {
	err := u.connect(ctx, stopUltra, func(loops *autoblowLoops) {
		u.stream.stop()
		u.stream.targetSet = false
		loops.done.Add(1)
		go u.pace(loops)
	})
	return u.Status(), err
}

// Disconnect stops the Ultra, then closes the connection.
func (u *AutoblowUltra) Disconnect() (AutoblowStatus, error) {
	u.mu.Lock()
	u.stream.stop()
	u.mu.Unlock()
	err := u.disconnect(stopUltra)
	return u.Status(), err
}

// Close disconnects and refuses later connections.
func (u *AutoblowUltra) Close() error {
	u.mu.Lock()
	u.stream.stop()
	u.mu.Unlock()
	return u.close(stopUltra)
}

func stopUltra(ctx context.Context, client *autoblow.Client) error {
	_, err := client.StopUltra(ctx)
	return err
}

// Status returns a safe snapshot.
func (u *AutoblowUltra) Status() AutoblowStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	status := u.statusLocked()
	status.LateSegments = u.late
	if u.stream.targetSet && u.client != nil {
		target := u.stream.target
		status.CommandedPercent = &target
	}
	return status
}

// MotionTransport admits an engine bound to the current connection.
func (u *AutoblowUltra) MotionTransport() (Transport, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.client == nil {
		return nil, errAutoblowNotConnected
	}
	return &autoblowSession{owner: u, client: u.client}, nil
}

// BeginEmergencyStop stops the Ultra beside the engine's Stop and returns a
// wait. A disconnected Ultra has nothing to stop.
func (u *AutoblowUltra) BeginEmergencyStop() func() error {
	u.mu.Lock()
	u.stream.stop()
	if u.client != nil {
		u.state = "idle"
	}
	u.testing = false
	admission := u.admitStopLocked()
	u.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- u.finishStop(admission, stopUltra) }()
	return func() error { return <-done }
}

// TestAt strokes gently for a few seconds while motion is idle. generation
// fences a test requested before a Stop.
func (u *AutoblowUltra) TestAt(generation uint64) error {
	u.mu.Lock()
	switch {
	case u.client == nil:
		u.mu.Unlock()
		return errAutoblowNotConnected
	case u.generation != generation || u.stopHolds > 0 || u.state == "stale":
		u.mu.Unlock()
		return errAutoblowSuperseded
	case u.stream.origin.Playing() || u.testing:
		u.mu.Unlock()
		return errAutoblowBusy
	}
	u.testing = true
	loops := u.loops
	u.mu.Unlock()
	err := u.command(generation, autoblowCommandTimeout, func(ctx context.Context, client *autoblow.Client) error {
		_, err := client.Oscillate(ctx, 20, 35, 65)
		return err
	})
	if err != nil {
		_ = u.haltGeneration(loops, generation, stopUltra)
		u.mu.Lock()
		u.testing = false
		u.mu.Unlock()
		return err
	}
	u.endTestLater(loops, generation, stopUltra)
	return nil
}

// AppendPoints stores the stream's neutral points. The owner reads them; it
// never reshapes the frame.
func (u *AutoblowUltra) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	recorded := Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}
	if err := u.checkSessionLocked(ctx); err != nil {
		return u.recordLocked(recorded, err)
	}
	if u.client == nil {
		return u.recordLocked(recorded, errAutoblowNotConnected)
	}
	if u.stopHolds > 0 || u.state == "stale" {
		return u.recordLocked(recorded, errAutoblowSuperseded)
	}
	if command.StreamID != u.stream.id {
		u.stream.stop()
		u.stream.id = command.StreamID
	}
	return u.recordLocked(recorded, u.stream.timeline.Append(companionPoints(command.Points)))
}

func companionPoints(points []TimedPoint) []companion.Point {
	converted := make([]companion.Point, len(points))
	for index, point := range points {
		converted[index] = companion.Point{TimeMillis: point.TimeMillis, Position: point.PositionPercent}
	}
	return converted
}

// Play starts the prepared stream after a preroll that brings the slider to
// its first point.
func (u *AutoblowUltra) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	return u.play(ctx, command, time.Time{})
}

// PlayAligned starts on a linked session's clock.
func (u *AutoblowUltra) PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	return u.play(ctx, command, start)
}

func (u *AutoblowUltra) play(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	recorded := Command{Kind: CommandKindPointsPlay, PointsPlay: &command}
	if err := ctx.Err(); err != nil {
		return u.recordLocked(recorded, err)
	}
	if err := u.checkSessionLocked(ctx); err != nil {
		return u.recordLocked(recorded, err)
	}
	if u.client == nil {
		return u.recordLocked(recorded, errAutoblowNotConnected)
	}
	if command.StreamID != u.stream.id || u.stream.timeline.Len() == 0 {
		return u.recordLocked(recorded, fmt.Errorf("the Autoblow Ultra stream %q has no points", command.StreamID))
	}
	if u.stopHolds > 0 || u.state == "stale" {
		return u.recordLocked(recorded, errAutoblowSuperseded)
	}
	if start.IsZero() {
		u.stream.origin.Start(u.clock(), command.StartTimeMillis, ultraPreroll)
	} else {
		u.stream.origin.StartAt(start)
	}
	u.generation++
	u.stream.started = false
	u.stream.coverage.Reset()
	u.failures = 0
	u.testing = false
	u.state = "playing"
	return u.recordLocked(recorded, nil)
}

// Stop ends the stream and stops the Ultra whatever it is doing.
func (u *AutoblowUltra) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	u.mu.Lock()
	recorded := Command{Kind: CommandKindStop, Stop: &command}
	if err := u.checkSessionLocked(ctx); err != nil {
		u.mu.Unlock()
		return CommandResult{}, err
	}
	u.stream.stop()
	u.testing = false
	if u.client != nil {
		u.state = "idle"
	}
	u.mu.Unlock()
	err := u.stopDevice(stopUltra)
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.recordLocked(recorded, err)
}

// SetStrokeWindow applies the physical window host-side, exactly once, like
// the Intiface owner: the Ultra has no window of its own.
func (u *AutoblowUltra) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	recorded := Command{Kind: CommandKindStrokeWindow, StrokeWindow: &command}
	if err := u.checkSessionLocked(ctx); err != nil {
		return u.recordLocked(recorded, err)
	}
	if err := validateStrokeWindow(command); err != nil {
		return u.recordLocked(recorded, err)
	}
	u.window = command
	return u.recordLocked(recorded, nil)
}

// MotionTimingCapabilities asks for legs no shorter than the Ultra can make
// and enough frame ahead for the lookahead, the preroll and the cloud.
func (u *AutoblowUltra) MotionTimingCapabilities() MotionTimingCapabilities {
	return MotionTimingCapabilities{MinimumPointInterval: ultraMinimumMove, MinimumBufferedLead: ultraMinimumLead, MinimumMediaBufferedLead: ultraMediaLead}
}

// MotionSamplingCapabilities asks for sparse curves: every go-to is a cloud
// request, and positions are whole percent after the window.
func (u *AutoblowUltra) MotionSamplingCapabilities() MotionSamplingCapabilities {
	return MotionSamplingCapabilities{PositionResolutionPercent: 1, ResolutionAfterStrokeWindow: true,
		PreferredPointInterval: ultraPreferredInterval, PreferSparseGeneratedMotion: true}
}

// PlaybackStartTime reports the local time of stream millisecond 0.
func (u *AutoblowUltra) PlaybackStartTime() time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stream.origin.Anchor()
}

// AlignPlaybackStart re-anchors a playing stream on another owner's start.
func (u *AutoblowUltra) AlignPlaybackStart(start time.Time) {
	u.alignPlaybackStart(nil, start)
}

func (u *AutoblowUltra) alignPlaybackStart(client *autoblow.Client, start time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if (client == nil || u.client == client) && u.stream.origin.Playing() && !start.IsZero() {
		u.stream.origin.StartAt(start)
	}
}

// Diagnostics returns the snapshot the engine reads for playback health.
func (u *AutoblowUltra) Diagnostics() TransportDiagnostics {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.diagnosticsLocked()
}

func (u *AutoblowUltra) pace(loops *autoblowLoops) {
	defer loops.done.Done()
	ticker := time.NewTicker(u.tick)
	defer ticker.Stop()
	for {
		select {
		case <-loops.stop:
			return
		case <-ticker.C:
			u.paceOnce()
		}
	}
}

func (u *AutoblowUltra) paceOnce() {
	u.mu.Lock()
	if u.client == nil || !u.stream.origin.Playing() || u.stopHolds != 0 || u.state == "stale" {
		u.mu.Unlock()
		return
	}
	move, ok := u.nextMoveLocked(u.clock())
	generation := u.generation
	u.mu.Unlock()
	if !ok {
		return
	}
	err := u.command(generation, autoblowCommandTimeout, func(ctx context.Context, client *autoblow.Client) error {
		return client.Goto(ctx, move.position, move.speed)
	})
	u.mu.Lock()
	defer u.mu.Unlock()
	if generation != u.generation {
		return
	}
	if err == nil {
		u.failures = 0
		return
	}
	if u.noteFailureLocked(err) && generation == u.generation {
		// Stop and report, like the other pacers: the engine reads the state.
		u.stream.stop()
		u.state = "stale"
		go u.failConnection(u.client, err)
	}
}

// nextMoveLocked returns the go-to that starts the leg arriving at the device
// now: to the next turning point, or the furthest point of a long leg within
// the lookahead. Each leg is sent once, so a late tick skips to the leg that
// is current instead of replaying old ones.
func (u *AutoblowUltra) nextMoveLocked(now time.Time) (ultraMove, bool) {
	first, ok := u.stream.timeline.Start()
	if !ok {
		return ultraMove{}, false
	}
	if !u.stream.started {
		u.stream.started = true
		position, _ := u.stream.timeline.At(float64(first))
		u.stream.target, u.stream.targetSet = u.physicalLocked(position), true
		u.stream.sentUntil = float64(first)
		return ultraMove{position: u.stream.target, speed: ultraStartupSpeed}, true
	}
	at := u.stream.origin.StreamMillis(now.Add(u.leadLocked()))
	end, _ := u.stream.timeline.End()
	covered := at <= float64(end)
	if at >= float64(first) {
		u.state = u.stream.coverage.Observe(now, covered)
		if u.state == companion.StateStarved {
			u.stream.stop()
			return ultraMove{}, false
		}
	}
	if !covered || at < float64(first) {
		return ultraMove{}, false
	}
	current, _ := u.stream.timeline.At(at)
	target, ok := u.stream.timeline.PointAfter(at)
	if !ok {
		return ultraMove{}, false
	}
	direction := math.Copysign(1, target.Position-current)
	for target.Position != current {
		next, ok := u.stream.timeline.PointAfter(float64(target.TimeMillis))
		if !ok || float64(next.TimeMillis)-at > ultraLookahead || next.Position == target.Position ||
			math.Copysign(1, next.Position-target.Position) != direction {
			break
		}
		target = next
	}
	if float64(target.TimeMillis) <= u.stream.sentUntil {
		return ultraMove{}, false
	}
	if previous, ok := u.stream.timeline.PointAfter(u.stream.sentUntil); ok && previous.TimeMillis < target.TimeMillis && float64(previous.TimeMillis) < at-float64(ultraMinimumMove.Milliseconds()) {
		u.late++
	}
	u.stream.sentUntil = float64(target.TimeMillis)
	u.stream.timeline.Prune(int64(at) - ultraPruneMillis)
	physical := u.physicalLocked(target.Position)
	if math.Abs(physical-u.stream.target) < ultraSamePosition {
		return ultraMove{}, false
	}
	seconds := (float64(target.TimeMillis) - at) / 1000
	speed := 100.0
	if seconds > 0 {
		speed = math.Abs(physical-u.stream.target) / seconds / ultraFullSpeed * 100
	}
	u.stream.target = physical
	return ultraMove{position: physical, speed: math.Max(1, math.Min(100, speed))}, true
}

// physicalLocked maps a semantic position into the stroke window, reversed
// when asked: once, at this boundary (ADR 0002).
func (u *AutoblowUltra) physicalLocked(position float64) float64 {
	position = math.Max(0, math.Min(100, position))
	if u.window.ReverseDirection {
		position = 100 - position
	}
	low, high := float64(u.window.MinPercent), float64(u.window.MaxPercent)
	if high <= low {
		low, high = 0, 100
	}
	return low + (high-low)*position/100
}
