package transport

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/autoblow"
	"github.com/mapledaemon/MagicHandy/internal/companion"
	"github.com/mapledaemon/MagicHandy/internal/vacuglide"
)

const (
	vacuglideTick        = 100 * time.Millisecond
	vacuglideStarveAfter = 750 * time.Millisecond
	// vacuglideSendGap caps speed changes at four a second; a change smaller
	// than vacuglideSendStep waits for the next real change.
	vacuglideSendGap  = 250 * time.Millisecond
	vacuglideSendStep = 2.0
	// The window covers the speed average and the frame ahead of it.
	vacuglideMinimumLead = 1200 * time.Millisecond
	vacuglideMediaLead   = 2 * time.Second
	vacuglidePruneMillis = 3000
	// A valve step opens one valve this long. The follower takes at most one
	// step per vacuglideFollowEvery and stays within vacuglideFollowSteps of
	// where the session began; a step stands for vacuglideReachPerStep percent
	// of reach (ADR 0069).
	vacuglideValvePulse   = 300 * time.Millisecond
	vacuglideFollowEvery  = 2 * time.Second
	vacuglideFollowSteps  = 3
	vacuglideReachPerStep = 20.0
	vacuglideMaxSteps     = 12
	vacuglideTestSpeed    = 15.0
)

var (
	_ Transport = (*VacuGlide)(nil)

	errVacuGlideStrokeLimit = errors.New("the stroke has been adjusted as far as MagicHandy allows in one connection")
)

// VacuGlide owns one VacuGlide through Autoblow's cloud. It has no position
// input: its motor follows how fast the shared frame moves, and its stroke
// valves change how much air the sleeve holds, which sets how long and tight
// each stroke is (ADR 0069). The owner never reshapes the frame; it reads its
// pace, and, when following is on, how far its strokes reach.
type VacuGlide struct {
	autoblowBase
	tick      time.Duration
	renderer  *vacuglide.Renderer
	stream    vacuglideStream
	commanded float64
	sentAt    time.Time
	valveOpen string
	steps     int
	valveMu   sync.Mutex // a whole valve pulse, including cleanup
}

type vacuglideStream struct {
	timeline    companion.Timeline
	id          string
	origin      companion.Clock
	coverage    companion.Coverage
	reachBase   float64
	reachKnown  bool
	followAt    time.Time
	followSteps int
}

func (s *vacuglideStream) stop() {
	s.timeline.Reset()
	s.id = ""
	s.origin.Stop()
	s.coverage.Reset()
	s.reachKnown = false
	s.followSteps = 0
	s.followAt = time.Time{}
}

// NewVacuGlide returns a disconnected owner.
func NewVacuGlide(options AutoblowOptions) *VacuGlide {
	tick := options.Tick
	if tick <= 0 {
		tick = vacuglideTick
	}
	return &VacuGlide{
		autoblowBase: newAutoblowBase(autoblow.FamilyVacuGlide, VacuGlideDevice, options),
		tick:         tick,
		renderer:     vacuglide.NewRenderer(vacuglide.Profile{Scale: 1}),
		stream:       vacuglideStream{coverage: companion.NewCoverage(vacuglideStarveAfter)},
	}
}

// Configure applies the saved token, offset, speed scale and stroke
// following. A new token takes effect at the next Connect.
func (v *VacuGlide) Configure(config AutoblowConfig) {
	v.configureBase(config)
	v.mu.Lock()
	defer v.mu.Unlock()
	v.renderer.SetProfile(vacuglide.Profile{Scale: config.SpeedScale})
}

// Connect opens the cloud connection, stops the motor and closes both valves.
func (v *VacuGlide) Connect(ctx context.Context) (AutoblowStatus, error) {
	err := v.connect(ctx, settleVacuGlide, func(loops *autoblowLoops) {
		v.stream.stop()
		v.renderer.Restart()
		v.commanded, v.valveOpen, v.steps = 0, "", 0
		loops.done.Add(1)
		go v.pace(loops)
	})
	return v.Status(), err
}

// Disconnect stops the motor and closes the valves, then the connection.
func (v *VacuGlide) Disconnect() (AutoblowStatus, error) {
	v.mu.Lock()
	v.stream.stop()
	v.mu.Unlock()
	err := v.disconnect(v.stopAll)
	return v.Status(), err
}

// Close disconnects and refuses later connections.
func (v *VacuGlide) Close() error {
	v.mu.Lock()
	v.stream.stop()
	v.mu.Unlock()
	return v.close(v.stopAll)
}

func settleVacuGlide(ctx context.Context, client *autoblow.Client) error {
	motorCtx, cancel := context.WithTimeout(ctx, autoblowCommandTimeout)
	_, motorErr := client.StopVacuGlide(motorCtx)
	cancel()
	return errors.Join(motorErr, closeVacuGlideValves(ctx, client))
}

func closeVacuGlideValves(ctx context.Context, client *autoblow.Client) error {
	var errs []error
	for _, valve := range []string{autoblow.ValveStrokePlus, autoblow.ValveStrokeMinus} {
		if _, err := client.Valve(ctx, valve, false); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// stopAll stops the motor and closes a valve left open by a step in flight.
func (v *VacuGlide) stopAll(ctx context.Context, client *autoblow.Client) error {
	motorCtx, cancel := context.WithTimeout(ctx, autoblowCommandTimeout)
	_, err := client.StopVacuGlide(motorCtx)
	cancel()
	closeErr := closeVacuGlideValves(ctx, client)
	v.mu.Lock()
	if err == nil {
		v.commanded = 0
	}
	if closeErr == nil {
		v.valveOpen = ""
	}
	v.mu.Unlock()
	return errors.Join(err, closeErr)
}

// Status returns a safe snapshot.
func (v *VacuGlide) Status() AutoblowStatus {
	v.mu.Lock()
	defer v.mu.Unlock()
	status := v.statusLocked()
	status.StrokeSteps = v.steps
	status.StrokeFollow = v.config.StrokeFollow
	status.ValveOpen = v.valveOpen
	if v.client != nil {
		speed := v.commanded
		status.CommandedPercent = &speed
	}
	return status
}

// MotionTransport admits an engine bound to the current connection.
func (v *VacuGlide) MotionTransport() (Transport, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.client == nil {
		return nil, errAutoblowNotConnected
	}
	return &autoblowSession{owner: v, client: v.client}, nil
}

// BeginEmergencyStop stops the motor beside the engine's Stop and returns a
// wait.
func (v *VacuGlide) BeginEmergencyStop() func() error {
	v.mu.Lock()
	v.stream.stop()
	if v.client != nil {
		v.state = "idle"
	}
	v.testing = false
	admission := v.admitStopLocked()
	v.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- v.finishStop(admission, v.stopAll) }()
	return func() error { return <-done }
}

// TestAt runs the motor slowly for a few seconds while motion is idle.
func (v *VacuGlide) TestAt(generation uint64) error {
	v.mu.Lock()
	switch {
	case v.client == nil:
		v.mu.Unlock()
		return errAutoblowNotConnected
	case v.generation != generation || v.stopHolds > 0 || v.state == "stale":
		v.mu.Unlock()
		return errAutoblowSuperseded
	case v.stream.origin.Playing() || v.testing:
		v.mu.Unlock()
		return errAutoblowBusy
	}
	v.testing = true
	loops := v.loops
	v.mu.Unlock()
	err := v.command(generation, autoblowCommandTimeout, func(ctx context.Context, client *autoblow.Client) error {
		_, err := client.TargetSpeed(ctx, vacuglideTestSpeed)
		if err == nil {
			v.mu.Lock()
			v.commanded = vacuglideTestSpeed
			v.mu.Unlock()
		}
		return err
	})
	if err != nil {
		_ = v.haltGeneration(loops, generation, v.stopAll)
		v.mu.Lock()
		v.testing = false
		v.mu.Unlock()
		return err
	}
	v.endTestLater(loops, generation, v.stopAll)
	return nil
}

// AppendPoints stores the stream's neutral points.
func (v *VacuGlide) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	recorded := Command{Kind: CommandKindPointsAdd, PointsAdd: cloneAppendPoints(command)}
	if err := v.checkSessionLocked(ctx); err != nil {
		return v.recordLocked(recorded, err)
	}
	if v.client == nil {
		return v.recordLocked(recorded, errAutoblowNotConnected)
	}
	if command.StreamID != v.stream.id {
		v.stream.stop()
		v.stream.id = command.StreamID
	}
	return v.recordLocked(recorded, v.stream.timeline.Append(companionPoints(command.Points)))
}

// Play starts following the prepared stream.
func (v *VacuGlide) Play(ctx context.Context, command PlayCommand) (CommandResult, error) {
	return v.play(ctx, command, time.Time{})
}

// PlayAligned starts on a linked session's clock.
func (v *VacuGlide) PlayAligned(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	return v.play(ctx, command, start)
}

func (v *VacuGlide) play(ctx context.Context, command PlayCommand, start time.Time) (CommandResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	recorded := Command{Kind: CommandKindPointsPlay, PointsPlay: &command}
	if err := ctx.Err(); err != nil {
		return v.recordLocked(recorded, err)
	}
	if err := v.checkSessionLocked(ctx); err != nil {
		return v.recordLocked(recorded, err)
	}
	if v.client == nil {
		return v.recordLocked(recorded, errAutoblowNotConnected)
	}
	if command.StreamID != v.stream.id || v.stream.timeline.Len() == 0 {
		return v.recordLocked(recorded, fmt.Errorf("the VacuGlide stream %q has no points", command.StreamID))
	}
	if v.stopHolds > 0 || v.state == "stale" {
		return v.recordLocked(recorded, errAutoblowSuperseded)
	}
	if start.IsZero() {
		v.stream.origin.Start(v.clock(), command.StartTimeMillis, 0)
	} else {
		v.stream.origin.StartAt(start)
	}
	v.generation++
	v.stream.coverage.Reset()
	v.stream.reachKnown, v.stream.followSteps, v.stream.followAt = false, 0, time.Time{}
	v.renderer.Restart()
	v.failures = 0
	v.testing = false
	v.state = "playing"
	return v.recordLocked(recorded, nil)
}

// Stop ends the stream, stops the motor and closes any open valve.
func (v *VacuGlide) Stop(ctx context.Context, command StopCommand) (CommandResult, error) {
	v.mu.Lock()
	recorded := Command{Kind: CommandKindStop, Stop: &command}
	if err := v.checkSessionLocked(ctx); err != nil {
		v.mu.Unlock()
		return CommandResult{}, err
	}
	v.stream.stop()
	v.testing = false
	if v.client != nil {
		v.state = "idle"
	}
	v.mu.Unlock()
	err := v.stopDevice(v.stopAll)
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.recordLocked(recorded, err)
}

// SetStrokeWindow is accepted for contract compatibility: the VacuGlide's
// stroke comes from its air, not a position window, and pace is read from
// the semantic span.
func (v *VacuGlide) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.recordLocked(Command{Kind: CommandKindStrokeWindow, StrokeWindow: &command}, v.checkSessionLocked(ctx))
}

// MotionTimingCapabilities keeps the speed window and the cloud's lead covered.
func (v *VacuGlide) MotionTimingCapabilities() MotionTimingCapabilities {
	return MotionTimingCapabilities{MinimumBufferedLead: vacuglideMinimumLead, MinimumMediaBufferedLead: vacuglideMediaLead}
}

// MotionSamplingCapabilities places no limit: only pace is read.
func (v *VacuGlide) MotionSamplingCapabilities() MotionSamplingCapabilities {
	return MotionSamplingCapabilities{PreferSparseGeneratedMotion: true}
}

// PlaybackStartTime reports the local time of stream millisecond 0.
func (v *VacuGlide) PlaybackStartTime() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.stream.origin.Anchor()
}

// AlignPlaybackStart re-anchors a playing stream on another owner's start.
func (v *VacuGlide) AlignPlaybackStart(start time.Time) {
	v.alignPlaybackStart(nil, start)
}

func (v *VacuGlide) alignPlaybackStart(client *autoblow.Client, start time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if (client == nil || v.client == client) && v.stream.origin.Playing() && !start.IsZero() {
		v.stream.origin.StartAt(start)
	}
}

// Diagnostics returns the snapshot the engine reads for playback health.
func (v *VacuGlide) Diagnostics() TransportDiagnostics {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.diagnosticsLocked()
}

func (v *VacuGlide) pace(loops *autoblowLoops) {
	defer loops.done.Done()
	ticker := time.NewTicker(v.tick)
	defer ticker.Stop()
	for {
		select {
		case <-loops.stop:
			return
		case <-ticker.C:
			v.paceOnce()
		}
	}
}

// vacuglideAction is what one tick decided to send.
type vacuglideAction struct {
	send   bool
	speed  float64
	step   int
	follow bool
}

func (v *VacuGlide) paceOnce() {
	v.mu.Lock()
	if v.client == nil || !v.stream.origin.Playing() || v.stopHolds != 0 || v.state == "stale" {
		v.mu.Unlock()
		return
	}
	now := v.clock()
	action := v.decideLocked(now)
	generation := v.generation
	v.mu.Unlock()
	if action.send {
		v.sendSpeed(generation, action.speed)
	}
	if action.follow && action.step != 0 {
		_ = v.pulse(generation, action.step > 0, true)
	}
}

// decideLocked renders the motor speed a tick ahead by the cloud's lead and
// decides whether it is worth a request; a frame that ran out stops the motor
// at once rather than leaving it running.
func (v *VacuGlide) decideLocked(now time.Time) vacuglideAction {
	at := v.stream.origin.StreamMillis(now.Add(v.leadLocked()))
	first, _ := v.stream.timeline.Start()
	if at < float64(first) {
		return vacuglideAction{}
	}
	speed, covered := v.renderer.Speed(&v.stream.timeline, at)
	v.state = v.stream.coverage.Observe(now, covered)
	if v.state == companion.StateStarved {
		v.stream.stop()
	}
	v.stream.timeline.Prune(int64(at) - vacuglidePruneMillis)
	target := math.Round(speed)
	action := vacuglideAction{speed: target}
	switch {
	case target == v.commanded:
	case target == 0 || v.commanded == 0:
		action.send = true
	case math.Abs(target-v.commanded) >= vacuglideSendStep && now.Sub(v.sentAt) >= vacuglideSendGap:
		action.send = true
	}
	if action.send {
		v.sentAt = now
	}
	if covered && v.config.StrokeFollow {
		action.step, action.follow = v.followStepLocked(now, at), true
	}
	return action
}

// followStepLocked compares how far the strokes reach now with where they
// reached when the session began, and takes one valve step toward it.
func (v *VacuGlide) followStepLocked(now time.Time, at float64) int {
	reach, ok := v.renderer.Reach(&v.stream.timeline, at)
	if !ok || now.Sub(v.stream.followAt) < vacuglideFollowEvery {
		return 0
	}
	v.stream.followAt = now
	if !v.stream.reachKnown {
		v.stream.reachBase, v.stream.reachKnown = reach, true
		return 0
	}
	want := int(math.Round((reach - v.stream.reachBase) / vacuglideReachPerStep))
	want = max(-vacuglideFollowSteps, min(vacuglideFollowSteps, want))
	switch {
	case want > v.stream.followSteps && v.steps < vacuglideMaxSteps:
		return 1
	case want < v.stream.followSteps && v.steps > -vacuglideMaxSteps:
		return -1
	}
	return 0
}

func (v *VacuGlide) sendSpeed(generation uint64, speed float64) {
	err := v.command(generation, autoblowCommandTimeout, func(ctx context.Context, client *autoblow.Client) error {
		var err error
		if speed <= 0 {
			_, err = client.StopVacuGlide(ctx)
		} else {
			_, err = client.TargetSpeed(ctx, speed)
		}
		return err
	})
	v.mu.Lock()
	defer v.mu.Unlock()
	if generation != v.generation {
		return
	}
	if err == nil {
		v.commanded = speed
		v.failures = 0
		return
	}
	if v.noteFailureLocked(err) && generation == v.generation {
		v.stream.stop()
		v.state = "stale"
		go v.failConnection(v.client, err)
	}
}
