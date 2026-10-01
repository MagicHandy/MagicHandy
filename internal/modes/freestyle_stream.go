package modes

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/motion"
)

// Freestyle plays one endless generated stroke stream (motion.FreestyleSpec)
// through the shared engine. The manager never picks patterns: it keeps the
// stream's controls and session shape as keyframes and hands the engine
// overlapping windows of the same stream. Because overlapping windows share
// their strokes exactly, a continuation or a control change joins the
// running motion with no crossfade, restart or position jump. Every change is
// placed after the motion the engine has already queued.

const (
	freestyleWindowStrokes = 128
	// freestyleContinueMillis continues the stream while plenty of the
	// current window remains, far from the window's finite end.
	freestyleContinueMillis = 45_000
	// freestyleReplanEvery refreshes an active shape's stroke-timed keyframes
	// against the measured stroke clock.
	freestyleReplanEvery = 30 * time.Second
	// freestyleShapeStepMillis spaces a shape's keyframes; each ramp settles
	// before the next begins, so the history compacts.
	freestyleShapeStepMillis = 5000
	// freestyleControlRampMillis is how long a control change takes to ease
	// in once it reaches unqueued strokes.
	freestyleControlRampMillis = 2500
	// freestyleEndLeadStrokes is how many strokes Cooldown takes to come to
	// rest once its time is up; the stream winds down over them.
	freestyleEndLeadStrokes      = 8
	freestyleDefaultStrokeMillis = 900
	// freestyleMaximumShapeKeyframes keeps a window's plan well inside the
	// keyframe bound.
	freestyleMaximumShapeKeyframes = 24
	// freestyleMaximumClockStep stops a stalled tick or clock jump from
	// advancing the shape clock by more than a moment.
	freestyleMaximumClockStep = 5 * time.Second
)

// freestyleState is the running stream. Manager.mu protects it.
type freestyleState struct {
	seed       uint32
	fromStroke int
	keyframes  []motion.FreestyleKeyframe
	endStroke  int
	applied    freestyleBasis
	started    bool
	// active is the shape clock: running, unpaused time.
	active       time.Duration
	clockShape   string
	lastTick     time.Time
	plannedAt    time.Duration
	lastStroke   int
	strokeMillis int64
}

// freestyleBasis is what a window was built from, so a changed preference or
// limit is noticed without comparing keyframes.
type freestyleBasis struct {
	preferences        config.FreestyleSettings
	minSpeed, maxSpeed int
	handyModel         string
}

type freestylePlan struct {
	target     motion.MotionTarget
	keyframes  []motion.FreestyleKeyframe
	fromStroke int
	endStroke  int
	basis      freestyleBasis
	shape      freestyleShape
	start      int
}

// FreestyleStatus is the visible state of the running stream and its shape.
type FreestyleStatus struct {
	Feel  string `json:"feel"`
	Shape string `json:"shape"`
	// ShapePhase names where the shape is: building, holding, rising,
	// falling, peak, backing_off, teasing, easing or finished.
	ShapePhase string `json:"shape_phase,omitempty"`
	// ShapeProgressPercent runs through Slow build and Cooldown.
	ShapeProgressPercent int `json:"shape_progress_percent,omitempty"`
	// EnergyPercent is the shape's pace, 100 being the chosen pace.
	EnergyPercent int  `json:"energy_percent"`
	Ending        bool `json:"ending,omitempty"`
}

func (m *Manager) freestylePreferences() config.FreestyleSettings {
	if m.options.FreestyleSettings == nil {
		return config.DefaultFreestyleSettings()
	}
	return m.options.FreestyleSettings()
}

func (m *Manager) freestyleSeed() uint32 {
	seed := m.options.Seed
	if seed == 0 {
		seed = m.options.Now().UnixNano()
	}
	mixed := uint32(seed) ^ uint32(seed>>32) // #nosec G115 -- folding a seed; wraparound is intended.
	if mixed == 0 {
		mixed = 0x5eed
	}
	return mixed
}

// tickFreestyle keeps the stream playing: it starts or recovers the engine,
// and continues or re-plans the running window when one is due.
func (m *Manager) tickFreestyle(ctx context.Context, mode string) {
	if ctx.Err() != nil || !m.modeActive(mode) {
		return
	}
	engine := m.options.Current()
	var snapshot motion.ActiveMotionState
	if engine != nil {
		snapshot = engine.Snapshot()
	}
	// A user pause suspends the stream and its shape clock entirely.
	m.syncFreestyleShapeClock()
	if m.freezeIfPaused(mode, snapshot.Paused) {
		m.advanceFreestyleClock(false)
		return
	}
	m.thawDeadline()
	m.advanceFreestyleClock(snapshot.Running)
	if !snapshot.Running {
		m.recoverFreestyle(ctx, mode)
		return
	}
	m.continueFreestyle(ctx, engine, mode, snapshot.Freestyle)
}

// A newly chosen arc starts when it is selected, even in a long-running
// session. Once the finite wind-down is queued it completes; later preferences
// apply to the next explicit start, without reviving a finishing stream.
func (m *Manager) syncFreestyleShapeClock() {
	preferences := m.freestylePreferences()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.freestyle.endStroke == 0 && m.freestyle.clockShape != preferences.Shape {
		m.freestyle.clockShape = preferences.Shape
		m.freestyle.active, m.freestyle.plannedAt = 0, 0
		m.freestyle.lastTick = m.options.Now()
	}
}

func (m *Manager) advanceFreestyleClock(counting bool) {
	now := m.options.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if counting && !m.freestyle.lastTick.IsZero() {
		step := now.Sub(m.freestyle.lastTick)
		m.freestyle.active += max(0, min(step, freestyleMaximumClockStep))
	}
	m.freestyle.lastTick = now
}

func (m *Manager) recoverFreestyle(ctx context.Context, mode string) {
	m.mu.Lock()
	stopped := m.user.stopped
	retryAt := m.motion.nextRetry
	generation := m.loop.generation
	finished := m.freestyle.started && m.freestyle.endStroke > 0
	m.mu.Unlock()
	if stopped {
		// The user stopped motion; Freestyle ends rather than fighting it.
		go m.Stop("user_stop_observed")
		return
	}
	if finished {
		// Cooldown came to rest at the end of its final window.
		go m.stopLoopAtGeneration(mode, generation, "freestyle_complete")
		return
	}
	if m.options.Now().Before(retryAt) {
		return
	}
	m.startFreestyle(ctx, mode, generation)
}

// startFreestyle starts the engine on the stream (first start or recovery).
// A recovery starts the stream from rest a little past the last stroke heard,
// keeping the seed, so it continues the session rather than repeating it.
func (m *Manager) startFreestyle(ctx context.Context, mode string, generation uint64) {
	operationCtx, finish, ok := m.beginStartOperation(ctx, mode, generation, 0)
	if !ok {
		return
	}
	defer finish()
	engine, err := m.options.Ensure(operationCtx)
	if err != nil {
		if operationCtx.Err() == nil {
			m.backoff(mode, generation, "start_unavailable", err)
		}
		return
	}
	plan := m.planFreestyleStart()
	if _, err := engine.Start(operationCtx, plan.target, m.options.Settings()); err != nil {
		if operationCtx.Err() == nil {
			m.handleStartFailure(mode, generation, "start_failed", err)
		}
		return
	}
	m.commitFreestylePlan(mode, generation, plan, mode+"_start")
}

func (m *Manager) continueFreestyle(ctx context.Context, engine Engine, mode string, progress *motion.FreestyleProgress) {
	m.mu.Lock()
	seed := m.freestyle.seed
	generation := m.loop.generation
	retryAt := m.motion.nextRetry
	m.mu.Unlock()
	// Another source owns the engine, or the first window is not running yet.
	// Freestyle never fights the shared engine for control.
	if progress == nil || progress.Seed != seed {
		return
	}
	m.noteFreestyleProgress(progress)
	reason := m.freestyleReplanReason(progress)
	if reason == "" || m.options.Now().Before(retryAt) {
		return
	}
	operationCtx, finish, ok := m.beginStartOperation(ctx, mode, generation, 0)
	if !ok {
		return
	}
	defer finish()
	plan := m.planFreestyleWindow(*progress)
	if _, err := engine.ApplyTarget(operationCtx, plan.target, reason); err != nil {
		if operationCtx.Err() == nil {
			m.backoff(mode, generation, "segment_failed", err)
		}
		return
	}
	m.commitFreestylePlan(mode, generation, plan, reason)
}

func (m *Manager) noteFreestyleProgress(progress *motion.FreestyleProgress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.freestyle.lastStroke = progress.PlayingStroke
	if progress.StrokeMillis > 0 {
		m.freestyle.strokeMillis = progress.StrokeMillis
	}
}

// freestyleReplanReason names why the running window needs replacing, or
// returns "" while it can keep playing.
func (m *Manager) freestyleReplanReason(progress *motion.FreestyleProgress) string {
	preferences := m.freestylePreferences()
	basis := m.freestyleBasisFor(preferences)
	m.mu.Lock()
	state := m.freestyle
	m.mu.Unlock()
	switch {
	case state.endStroke > 0:
		return ""
	case basis.preferences != state.applied.preferences:
		return "freestyle_preferences"
	case basis != state.applied:
		return "freestyle_limits"
	case shapeAt(preferences, state.seed, state.active).finished:
		return "freestyle_ending"
	case progress.RemainingMillis < freestyleContinueMillis:
		return "freestyle_continue"
	case preferences.Shape != config.FreestyleShapeSteady && state.active-state.plannedAt >= freestyleReplanEvery:
		return "freestyle_shape"
	}
	return ""
}

func (m *Manager) freestyleBasisFor(preferences config.FreestyleSettings) freestyleBasis {
	limits := m.options.Settings()
	minimum := clampInt(limits.SpeedMinPercent, 1, 100)
	return freestyleBasis{preferences: preferences, minSpeed: minimum,
		maxSpeed: clampInt(limits.SpeedMaxPercent, minimum, 100), handyModel: limits.HandyModel}
}

// planFreestyleStart builds a stream's first window, or a recovery window
// that starts from rest just past the last stroke played.
func (m *Manager) planFreestyleStart() freestylePlan {
	preferences := m.freestylePreferences()
	m.mu.Lock()
	state := m.freestyle
	m.mu.Unlock()
	from := 0
	if state.started {
		from = state.lastStroke + 2
	}
	shape := shapeAt(preferences, state.seed, state.active)
	controls := freestyleControls(preferences, shape)
	keyframes := []motion.FreestyleKeyframe{{Stroke: from, Controls: controls}}
	plan := freestylePlan{fromStroke: from, start: from, basis: m.freestyleBasisFor(preferences), shape: shape}
	plan.keyframes = m.planShape(keyframes, from+1, from+freestyleWindowStrokes, preferences, state, from, 0)
	plan.target = freestyleTarget(state.seed, plan, freestyleWindowStrokes)
	return plan
}

// planFreestyleWindow keeps the control history that queued motion already
// depends on and re-plans everything after the editable stroke.
func (m *Manager) planFreestyleWindow(progress motion.FreestyleProgress) freestylePlan {
	preferences := m.freestylePreferences()
	m.mu.Lock()
	state := m.freestyle
	state.keyframes = append([]motion.FreestyleKeyframe(nil), m.freestyle.keyframes...)
	m.mu.Unlock()
	start := progress.PlayingStroke
	editable := max(progress.EditableStroke, start+1)
	kept := []motion.FreestyleKeyframe{state.keyframes[0]}
	for _, keyframe := range state.keyframes[1:] {
		if keyframe.Stroke < editable {
			kept = append(kept, keyframe)
		}
	}
	free := max(editable, keyframeSettles(kept[len(kept)-1]))
	shape := shapeAt(preferences, state.seed, state.active)
	plan := freestylePlan{fromStroke: state.fromStroke, start: start, basis: m.freestyleBasisFor(preferences), shape: shape}
	strokes := freestyleWindowStrokes
	if shape.finished {
		plan.endStroke = min(free+freestyleEndLeadStrokes, start+motion.FreestyleMaximumWindowStrokes)
		strokes = plan.endStroke - start
	}
	plan.keyframes = m.planShape(kept, free, start+strokes, preferences, state, start, progress.StrokeMillis)
	plan.keyframes = motion.CompactFreestyleKeyframes(plan.keyframes, start)
	plan.target = freestyleTarget(state.seed, plan, strokes)
	return plan
}

func keyframeSettles(keyframe motion.FreestyleKeyframe) int {
	return keyframe.Stroke + keyframe.RampStrokes
}

// planShape appends the keyframes that carry the stream from its last kept
// controls to the user's preferences under the session shape. A steady shape
// needs at most one eased ramp; any other shape is sampled every few seconds
// along the estimated stroke clock, with linear ramps that each settle before
// the next begins.
func (m *Manager) planShape(keyframes []motion.FreestyleKeyframe, from, until int, preferences config.FreestyleSettings,
	state freestyleState, playing int, strokeMillis int64) []motion.FreestyleKeyframe {
	if strokeMillis <= 0 {
		strokeMillis = state.strokeMillis
	}
	if strokeMillis <= 0 {
		strokeMillis = freestyleDefaultStrokeMillis
	}
	strokesFor := func(millis int64) int { return max(1, int(math.Round(float64(millis)/float64(strokeMillis)))) }
	last := keyframes[len(keyframes)-1].Controls
	if preferences.Shape == config.FreestyleShapeSteady {
		target := freestyleControls(preferences, shapeAt(preferences, state.seed, state.active))
		if target != last && from < until {
			keyframes = append(keyframes, motion.FreestyleKeyframe{Stroke: from,
				RampStrokes: min(16, max(3, strokesFor(freestyleControlRampMillis))), Controls: target})
		}
		return keyframes
	}
	step := max(3, strokesFor(freestyleShapeStepMillis), (until-from+freestyleMaximumShapeKeyframes-1)/freestyleMaximumShapeKeyframes)
	ramp := step - 1
	for stroke := from; stroke < until; stroke += step {
		// Aim each ramp at the shape's value when the ramp completes.
		ahead := time.Duration(stroke+ramp-playing) * time.Duration(strokeMillis) * time.Millisecond
		target := freestyleControls(preferences, shapeAt(preferences, state.seed, state.active+ahead))
		if target == last {
			continue
		}
		keyframes = append(keyframes, motion.FreestyleKeyframe{Stroke: stroke, RampStrokes: ramp, Linear: true, Controls: target})
		last = target
	}
	return keyframes
}

// freestyleControls maps the user's preferences and the shape's reading to
// stream controls. Nothing here can exceed the speed band: energy only lowers
// the pace below the chosen pace.
func freestyleControls(preferences config.FreestyleSettings, shape freestyleShape) motion.FreestyleControls {
	accent := 0
	switch preferences.Accent {
	case config.FreestyleAccentTip:
		accent = 60
	case config.FreestyleAccentBase:
		accent = -60
	}
	return motion.FreestyleControls{PacePercent: preferences.PacePercent, LengthPercent: preferences.LengthPercent,
		FocusPercent: preferences.FocusPercent, RoamingPercent: preferences.RoamingPercent,
		VarietyPercent: preferences.VarietyPercent, AccentPercent: accent,
		EnergyPercent: int(math.Round(100 * shape.energy)), TeasePercent: int(math.Round(100 * shape.tease))}
}

func freestyleTarget(seed uint32, plan freestylePlan, strokes int) motion.MotionTarget {
	spec := motion.NewFreestyleFlow(seed, plan.basis.maxSpeed, motion.FreestyleSpec{FromStroke: plan.fromStroke,
		StartStroke: plan.start, Strokes: strokes, EndStroke: plan.endStroke, MinSpeedPercent: plan.basis.minSpeed,
		Keyframes: plan.keyframes})
	return motion.MotionTarget{Label: modeLabel(ModeFreestyle), Source: ModeFreestyle,
		SpeedPercent: plan.basis.maxSpeed, Flow: &spec}
}

func (m *Manager) commitFreestylePlan(mode string, generation uint64, plan freestylePlan, reason string) {
	m.mu.Lock()
	if m.loop.mode != mode || m.loop.generation != generation || m.user.stopped {
		m.mu.Unlock()
		return
	}
	m.freestyle.keyframes = plan.keyframes
	m.freestyle.fromStroke = plan.fromStroke
	m.freestyle.endStroke = plan.endStroke
	m.freestyle.applied = plan.basis
	m.freestyle.plannedAt = m.freestyle.active
	m.freestyle.started = true
	m.motion.segmentIdx++
	m.motion.nextRetry = time.Time{}
	seed := m.freestyle.seed
	m.mu.Unlock()
	m.trace(mode, reason, &diagnostics.MotionTracePlanner{
		Mode: mode, Event: reason, Style: plan.basis.preferences.Feel, Seed: int64(seed),
		PatternIdentifier: "freestyle_stream", SpeedPercent: plan.basis.maxSpeed, SegmentIndex: plan.start,
	}, fmt.Sprintf("strokes=%d end=%d shape=%s phase=%s energy=%d keyframes=%d", plan.start, plan.endStroke,
		plan.basis.preferences.Shape, plan.shape.phase, int(math.Round(100*plan.shape.energy)), len(plan.keyframes)))
}

// freestyleStatus reads a copy of the stream state taken under Manager.mu.
func (m *Manager) freestyleStatus(state freestyleState) *FreestyleStatus {
	preferences := m.freestylePreferences()
	if state.endStroke > 0 {
		preferences = state.applied.preferences
	} else if preferences.Shape != state.clockShape {
		state.active = 0
	}
	shape := shapeAt(preferences, state.seed, state.active)
	status := &FreestyleStatus{Feel: preferences.Feel, Shape: preferences.Shape, ShapePhase: shape.phase,
		EnergyPercent: int(math.Round(100 * shape.energy)), Ending: state.endStroke > 0}
	if preferences.Shape == config.FreestyleShapeBuild || preferences.Shape == config.FreestyleShapeCooldown {
		status.ShapeProgressPercent = int(math.Floor(100 * shape.progress))
	}
	return status
}
