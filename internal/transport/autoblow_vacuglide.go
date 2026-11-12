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
)

const (
	vacuglideTick        = 100 * time.Millisecond
	vacuglideStarveAfter = 750 * time.Millisecond
	// vacuglideSendGap caps speed changes at four a second; a change smaller
	// than vacuglideSendStep waits for the next real change.
	vacuglideSendGap  = 250 * time.Millisecond
	vacuglideSendStep = 2.0
	// The frame covers the cloud's lead and the pacer's tick ahead of it.
	vacuglideMinimumLead = 1200 * time.Millisecond
	vacuglideMediaLead   = 2 * time.Second
	vacuglidePruneMillis = 3000
	// vacuglideFloorPercent keeps a moving motor above its stall point, about
	// 4% on Autoblow's scale; tempo below vacuglideHoldLevel stops it.
	vacuglideFloorPercent = 5.0
	vacuglideHoldLevel    = 0.5
	// A valve step opens one valve this long. Motion may move the air at most
	// vacuglideMotionAirSteps either way from where playback began, and only
	// when the person allowed it (ADR 0071).
	vacuglideValvePulse     = 300 * time.Millisecond
	vacuglideMotionAirSteps = 3
	vacuglideMaxSteps       = 12
	vacuglideTestSpeed      = 15.0
	vacuglideMaxTempoPoints = 512
)

var (
	_ Transport = (*VacuGlide)(nil)

	errVacuGlideStrokeLimit = errors.New("the stroke has been adjusted as far as MagicHandy allows in one connection")
	errVacuGlideAirLimit    = errors.New("motion has moved the air as far as it may in this session")
	errVacuGlideAirOff      = errors.New("motion may not change the VacuGlide's air; allow it on the VacuGlide's page")
	errVacuGlideNotPlaying  = errors.New("the VacuGlide is not playing")
)

// VacuGlide owns one VacuGlide through Autoblow's cloud. It has no position
// input: a crank-driven piston moves air that strokes the sleeve, so the motor
// sets the tempo, and the air sealed in the sleeve sets how long and tight
// each stroke is (ADRs 0069 and 0071). The owner plays the engine's tempo
// channel across the person's Tempo range; it never derives motion itself.
type VacuGlide struct {
	autoblowBase
	tick      time.Duration
	stream    vacuglideStream
	commanded float64
	sentAt    time.Time
	valveOpen string
	steps     int
	airSteps  int        // model steps in the logical session, retained across Pause
	valveMu   sync.Mutex // a whole valve pulse, including cleanup
}

type vacuglideStream struct {
	tempo       companion.Timeline
	revision    uint64
	id          string
	origin      companion.Clock
	coverage    companion.Coverage
	airRevision uint64
}

func (s *vacuglideStream) stop() {
	s.tempo.Reset()
	s.revision = 0
	s.id = ""
	s.origin.Stop()
	s.coverage.Reset()
	s.airRevision = 0
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
		stream:       vacuglideStream{coverage: companion.NewCoverage(vacuglideStarveAfter)},
	}
}

// Configure applies the saved token, offset, Tempo range and air permission.
// A new token takes effect at the next Connect.
func (v *VacuGlide) Configure(config AutoblowConfig) {
	v.configureBase(config)
}

// TakesTempo marks the VacuGlide as a tempo owner (ADR 0071).
func (v *VacuGlide) TakesTempo() bool { return true }

// motorPercentLocked maps a tempo level onto the Tempo range: below the hold
// level the motor stops; 1..100 span the range, never under the stall floor.
func (v *VacuGlide) motorPercentLocked(level float64) float64 {
	if !(level >= vacuglideHoldLevel) {
		return 0
	}
	low := math.Max(vacuglideFloorPercent, math.Min(100, v.config.TempoMinPercent))
	high := 100.0
	if v.config.TempoMaxPercent > 0 {
		high = math.Max(low, math.Min(100, v.config.TempoMaxPercent))
	}
	level = math.Max(1, math.Min(100, level))
	return math.Max(vacuglideFloorPercent, math.Min(100, low+(high-low)*(level-1)/99))
}

// Connect opens the cloud connection, stops the motor and closes both valves.
func (v *VacuGlide) Connect(ctx context.Context) (AutoblowStatus, error) {
	err := v.connect(ctx, settleVacuGlide, func(loops *autoblowLoops) {
		v.stream.stop()
		v.commanded, v.valveOpen, v.steps = 0, "", 0
		v.airSteps = 0
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
	status.MotionAir = v.config.MotionAir
	status.MotionAirSteps = v.airSteps
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

// AppendPoints stores the stream's tempo. The VacuGlide makes its own strokes,
// so it reads only the tempo channel and ignores positions.
func (v *VacuGlide) AppendPoints(ctx context.Context, command AppendPointsCommand) (CommandResult, error) {
	if command.Air != nil {
		return v.appendAir(ctx, command)
	}
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
	if command.Tempo == nil {
		return v.recordLocked(recorded, nil)
	}
	points, err := tempoFramePoints(command.Tempo)
	if err == nil && command.Tempo.Revision >= v.stream.revision {
		v.stream.revision = command.Tempo.Revision
		err = v.stream.tempo.Append(points)
	}
	return v.recordLocked(recorded, err)
}

// tempoFramePoints checks an engine tempo frame before the owner keeps it.
func tempoFramePoints(frame *TempoFrame) ([]companion.Point, error) {
	if len(frame.Points) == 0 || len(frame.Points) > vacuglideMaxTempoPoints {
		return nil, errors.New("invalid tempo frame size")
	}
	points := make([]companion.Point, len(frame.Points))
	for index, point := range frame.Points {
		if math.IsNaN(point.LevelPercent) || point.LevelPercent < 0 || point.LevelPercent > 100 {
			return nil, errors.New("tempo frame levels must be within 0..100")
		}
		if point.TimeMillis < 0 || (index > 0 && point.TimeMillis <= frame.Points[index-1].TimeMillis) {
			return nil, errors.New("tempo frame times must be nonnegative and strictly increasing")
		}
		points[index] = companion.Point{TimeMillis: point.TimeMillis, Position: point.LevelPercent}
	}
	return points, nil
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
	if command.StreamID != v.stream.id || v.stream.tempo.Len() == 0 {
		return v.recordLocked(recorded, fmt.Errorf("the VacuGlide stream %q has no tempo", command.StreamID))
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
	if !command.Resume {
		v.airSteps = 0
	}
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
// stroke comes from its air, not a position window.
func (v *VacuGlide) SetStrokeWindow(ctx context.Context, command StrokeWindowCommand) (CommandResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.recordLocked(Command{Kind: CommandKindStrokeWindow, StrokeWindow: &command}, v.checkSessionLocked(ctx))
}

// MotionTimingCapabilities keeps the cloud's lead covered.
func (v *VacuGlide) MotionTimingCapabilities() MotionTimingCapabilities {
	return MotionTimingCapabilities{MinimumBufferedLead: vacuglideMinimumLead, MinimumMediaBufferedLead: vacuglideMediaLead}
}

// MotionSamplingCapabilities places no limit: positions are not read.
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
	send  bool
	speed float64
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
}

// decideLocked plays the tempo a tick ahead by the cloud's lead and decides
// whether the motor speed is worth a request; a tempo that ran out stops the
// motor at once rather than leaving it running.
func (v *VacuGlide) decideLocked(now time.Time) vacuglideAction {
	at := v.stream.origin.StreamMillis(now.Add(v.leadLocked()))
	first, _ := v.stream.tempo.Start()
	if at < float64(first) {
		return vacuglideAction{}
	}
	level, covered := v.stream.tempo.At(at)
	v.state = v.stream.coverage.Observe(now, covered)
	if v.state == companion.StateStarved {
		v.stream.stop()
	}
	v.stream.tempo.Prune(int64(at) - vacuglidePruneMillis)
	target := 0.0
	if covered {
		target = math.Round(v.motorPercentLocked(level))
	}
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
	return action
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
