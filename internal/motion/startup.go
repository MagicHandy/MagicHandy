package motion

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/transport"
)

const (
	// startupPositionTolerancePercent is how far the device may settle from the
	// commanded lead-in target and still count as arrived. It is a correctness
	// check on a move the app just asked for, so it stays tight.
	startupPositionTolerancePercent = 1.0
	// startupCalibrationTolerancePercent is a different question with a different
	// answer: how far outside its own calibrated travel the device may be found
	// resting before its geometry is treated as nonsense.
	//
	// These shared one constant, and that was wrong in the direction that
	// refused real hardware. A Handy reported slide 0-100% as absolute
	// 5.00-102.83 while parked at absolute 4.00 -- one unit below its own stated
	// minimum, which is 1.02% of the 97.83-unit travel. Against a 1.0% bound it
	// failed by two hundredths of a percentage point and refused every start.
	//
	// One absolute unit is the measured device's quantization at the ends of its
	// range, so this has to clear 1.02% with room rather than land just under it.
	// The 3% boundary still rejects genuinely wrong geometry: the regression case
	// sits at -15.3%. The value is clamped into 0-100 immediately after the check,
	// so a small excursion was always meant to be absorbed rather than refused.
	startupCalibrationTolerancePercent = 3.0
	startupStationarySpeedAbsolute     = 5.0
	startupStationaryRetryDelay        = 150 * time.Millisecond
	startupArrivalRetryDelay           = 150 * time.Millisecond
	startupMinimumLeadIn               = 500 * time.Millisecond
	startupArrivalSettle               = 150 * time.Millisecond
	startupTargetHold                  = 10 * time.Second
	startupStationaryAttempts          = 3
	startupArrivalAttempts             = 6
)

type startupMotionProfile struct {
	settings           transport.StrokeWindowCommand
	currentFullPercent float64
	targetFullPercent  float64
	targetAbsolute     float64
	finalMinAbsolute   float64
	finalMaxAbsolute   float64
	fullTravelAbsolute float64
	currentSemantic    float64
	targetSemantic     float64
	unionWindow        transport.StrokeWindowCommand
	leadInDuration     time.Duration
	startupStreamID    string
	finalWindowReady   bool
}

// prepareMotionStartup anchors a new stream to measured physical state when
// the owner can provide it. The timed lead-in is engine-owned HSP content, so
// every motion source gets the same startup behavior and media time remains at
// zero until the device has reached the script's first point.
func (e *Engine) prepareMotionStartup(ctx context.Context, runEpoch uint64, prefix string) error {
	provider, ok := e.transport.(transport.MotionStartupStateProvider)
	if !ok {
		return e.setStrokeWindow(ctx, runEpoch, prefix+"_stroke_window", false)
	}

	if err := e.stopTransportForStartup(ctx, runEpoch, prefix+"_startup_stop"); err != nil {
		return err
	}
	state, err := e.readStationaryStartupState(ctx, runEpoch, provider, prefix)
	if err != nil {
		return err
	}
	profile, err := e.buildStartupMotionProfile(runEpoch, state)
	if err != nil {
		return err
	}

	currentInsideFinalWindow := profile.currentFullPercent >= float64(profile.settings.MinPercent) &&
		profile.currentFullPercent <= float64(profile.settings.MaxPercent)
	if currentInsideFinalWindow &&
		math.Abs(profile.targetFullPercent-profile.currentFullPercent) <= startupPositionTolerancePercent {
		return e.setStrokeWindowCommand(ctx, runEpoch, prefix+"_stroke_window", profile.settings, false)
	}

	windowReason := prefix + "_startup_window"
	if sameStrokeWindow(profile.unionWindow, profile.settings) {
		windowReason = prefix + "_stroke_window"
		profile.finalWindowReady = true
	}
	if err := e.setStrokeWindowCommand(ctx, runEpoch, windowReason, profile.unionWindow, false); err != nil {
		return err
	}
	if err := e.appendStartupLeadIn(ctx, runEpoch, profile, prefix+"_startup_points"); err != nil {
		return err
	}
	if err := e.playStartupLeadIn(ctx, runEpoch, profile, prefix+"_startup_play"); err != nil {
		return err
	}
	if err := e.waitForStartupLeadIn(ctx, profile.leadInDuration); err != nil {
		return err
	}
	if _, err := e.waitForStartupArrival(ctx, runEpoch, provider, profile, prefix); err != nil {
		return err
	}
	if err := e.stopTransportForStartup(ctx, runEpoch, prefix+"_startup_lead_in_complete"); err != nil {
		return err
	}

	verified, _, err := e.readMotionStartupState(ctx, runEpoch, provider, prefix+"_startup_stopped_verify")
	if err != nil {
		return err
	}
	if err := startupArrivalError(verified, profile, "stopped"); err != nil {
		return err
	}
	if profile.finalWindowReady {
		return nil
	}
	return e.setStrokeWindowCommand(ctx, runEpoch, prefix+"_stroke_window", profile.settings, false)
}

// waitForStartupArrival observes the physical slider while the final lead-in
// point remains active. Cloud playback can finish its scheduled HSP time before
// the device has physically settled; stopping first freezes that lag and makes
// repeated user retries inch toward the same target. A bounded observation
// window lets the existing command complete without starting semantic/media
// time. The caller's startup failure path issues Stop on every error.
func (e *Engine) waitForStartupArrival(
	ctx context.Context,
	runEpoch uint64,
	provider transport.MotionStartupStateProvider,
	profile startupMotionProfile,
	prefix string,
) (transport.MotionStartupState, error) {
	var state transport.MotionStartupState
	for attempt := range startupArrivalAttempts {
		var err error
		state, _, err = e.readMotionStartupState(ctx, runEpoch, provider, prefix+"_startup_arrival")
		if err != nil {
			return transport.MotionStartupState{}, err
		}
		ready, err := startupArrivalReady(state, profile)
		if err != nil {
			return transport.MotionStartupState{}, err
		}
		if ready {
			return state, nil
		}
		if attempt+1 < startupArrivalAttempts {
			if err := e.startupWait(ctx, startupArrivalRetryDelay); err != nil {
				return transport.MotionStartupState{}, err
			}
		}
	}
	return state, startupArrivalError(state, profile, "did not settle")
}

func startupArrivalReady(state transport.MotionStartupState, profile startupMotionProfile) (bool, error) {
	if err := validateStartupState(state); err != nil {
		return false, err
	}
	arrivalTolerance := profile.fullTravelAbsolute * startupPositionTolerancePercent / 100
	fullMinimum := profile.targetAbsolute - profile.targetFullPercent/100*profile.fullTravelAbsolute
	fullMaximum := fullMinimum + profile.fullTravelAbsolute
	if state.PositionAbsolute < fullMinimum-arrivalTolerance || state.PositionAbsolute > fullMaximum+arrivalTolerance {
		return false, errors.New("motion startup position moved outside calibrated full travel")
	}
	return math.Abs(state.PositionAbsolute-profile.targetAbsolute) <= arrivalTolerance &&
		state.SpeedAbsolute <= startupStationarySpeedAbsolute &&
		state.PositionAbsolute >= profile.finalMinAbsolute-arrivalTolerance &&
		state.PositionAbsolute <= profile.finalMaxAbsolute+arrivalTolerance, nil
}

func startupArrivalError(state transport.MotionStartupState, profile startupMotionProfile, disposition string) error {
	ready, err := startupArrivalReady(state, profile)
	if err != nil {
		return err
	}
	if ready {
		return nil
	}
	arrivalError := math.Abs(state.PositionAbsolute - profile.targetAbsolute)
	arrivalTolerance := profile.fullTravelAbsolute * startupPositionTolerancePercent / 100
	if arrivalError > arrivalTolerance {
		return fmt.Errorf(
			"motion startup lead-in %s %.2f mm from target, outside %.1f%% travel tolerance",
			disposition,
			arrivalError,
			startupPositionTolerancePercent,
		)
	}
	if state.SpeedAbsolute > startupStationarySpeedAbsolute {
		return fmt.Errorf(
			"motion startup lead-in %s while device slider speed remained %.2f",
			disposition,
			state.SpeedAbsolute,
		)
	}
	return fmt.Errorf("motion startup lead-in %s outside the requested stroke window", disposition)
}

func (e *Engine) readStationaryStartupState(
	ctx context.Context,
	runEpoch uint64,
	provider transport.MotionStartupStateProvider,
	prefix string,
) (transport.MotionStartupState, error) {
	var state transport.MotionStartupState
	var err error
	for attempt := range startupStationaryAttempts {
		state, _, err = e.readMotionStartupState(ctx, runEpoch, provider, prefix+"_startup_state")
		if err != nil {
			return transport.MotionStartupState{}, err
		}
		if state.SpeedAbsolute <= startupStationarySpeedAbsolute {
			return state, nil
		}
		if attempt+1 < startupStationaryAttempts {
			if err := e.startupWait(ctx, startupStationaryRetryDelay); err != nil {
				return transport.MotionStartupState{}, err
			}
		}
	}
	return transport.MotionStartupState{}, errors.New("device slider remained in motion after the motion startup Stop")
}

func (e *Engine) buildStartupMotionProfile(runEpoch uint64, state transport.MotionStartupState) (startupMotionProfile, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.validateRunLocked(runEpoch); err != nil {
		return startupMotionProfile{}, err
	}
	if err := validateStartupState(state); err != nil {
		return startupMotionProfile{}, err
	}

	settings := transport.StrokeWindowCommand{
		MinPercent:       e.settings.StrokeMinPercent,
		MaxPercent:       e.settings.StrokeMaxPercent,
		ReverseDirection: e.settings.ReverseDirection,
	}
	calibration, err := newStartupCalibration(state)
	if err != nil {
		return startupMotionProfile{}, err
	}
	first := sampleMotionPath(e.plan, e.transition, 0).PositionPercent
	targetFullPercent := physicalPositionForSemantic(first, settings)
	currentFullPercent := calibration.fullPercentAt(state.PositionAbsolute)
	if currentFullPercent < -startupCalibrationTolerancePercent ||
		currentFullPercent > 100+startupCalibrationTolerancePercent {
		// The geometry the device reported is the only thing that can explain this
		// refusal, so it belongs in the message. Without it the failure is a dead
		// end: it names a calibration nobody can see.
		return startupMotionProfile{}, fmt.Errorf(
			"motion startup absolute position was outside calibrated full travel "+
				"(position %.2f maps to %.1f%% of travel; device reported slide %.1f-%.1f%% as %.2f-%.2f)",
			state.PositionAbsolute, currentFullPercent,
			state.StrokeMinPercent, state.StrokeMaxPercent,
			state.StrokeMinAbsolute, state.StrokeMaxAbsolute)
	}
	currentFullPercent = math.Max(0, math.Min(100, currentFullPercent))
	unionMinimum := math.Floor(min(currentFullPercent, state.StrokeMinPercent, float64(settings.MinPercent)))
	unionMaximum := math.Ceil(max(currentFullPercent, state.StrokeMaxPercent, float64(settings.MaxPercent)))
	union := transport.StrokeWindowCommand{
		MinPercent:       int(math.Max(0, unionMinimum)),
		MaxPercent:       int(math.Min(100, unionMaximum)),
		ReverseDirection: settings.ReverseDirection,
	}
	if union.MinPercent >= union.MaxPercent {
		return startupMotionProfile{}, errors.New("motion startup could not construct a valid physical stroke window")
	}

	delta := math.Abs(targetFullPercent - currentFullPercent)
	return startupMotionProfile{
		settings:           settings,
		currentFullPercent: currentFullPercent,
		targetFullPercent:  targetFullPercent,
		targetAbsolute:     calibration.absoluteAt(targetFullPercent),
		finalMinAbsolute:   calibration.absoluteAt(float64(settings.MinPercent)),
		finalMaxAbsolute:   calibration.absoluteAt(float64(settings.MaxPercent)),
		fullTravelAbsolute: calibration.fullTravelAbsolute,
		currentSemantic:    semanticPositionForPhysical(currentFullPercent, union),
		targetSemantic:     semanticPositionForPhysical(targetFullPercent, union),
		unionWindow:        union,
		leadInDuration:     startupLeadInDuration(delta, e.plan.Target.SpeedPercent),
		startupStreamID:    e.streamID + "-startup",
	}, nil
}

func (e *Engine) readMotionStartupState(
	ctx context.Context,
	runEpoch uint64,
	provider transport.MotionStartupStateProvider,
	reason string,
) (transport.MotionStartupState, transport.MotionStartupStateResults, error) {
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		return transport.MotionStartupState{}, transport.MotionStartupStateResults{}, err
	}
	state, results, err := provider.ReadMotionStartupState(commandCtx)
	cleanup()

	sliderErr := error(nil)
	strokeErr := error(nil)
	if err != nil {
		if results.Stroke.Kind != "" && !results.Stroke.OK {
			strokeErr = err
		} else {
			sliderErr = err
		}
	}
	if results.Slider.Kind != "" {
		e.recordTransportResult(reason+"_slider", nil, transport.Command{Kind: transport.CommandKindSliderState}, results.Slider, sliderErr)
		e.rememberResult(results.Slider, sliderErr)
	}
	if results.Stroke.Kind != "" {
		annotation := ""
		if err == nil {
			annotation = startupStateAnnotation(state)
		}
		e.recordTransportResultWithAnnotation(
			reason+"_stroke",
			nil,
			transport.Command{Kind: transport.CommandKindStrokeWindowState},
			results.Stroke,
			strokeErr,
			annotation,
		)
		e.rememberResult(results.Stroke, strokeErr)
	}
	if err != nil {
		e.rememberError(err)
	}
	return state, results, err
}

func (e *Engine) stopTransportForStartup(ctx context.Context, runEpoch uint64, reason string) error {
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		return err
	}
	command := transport.StopCommand{Reason: reason}
	result, err := e.transport.Stop(commandCtx, command)
	cleanup()
	e.recordTransportResult(reason, nil, transport.Command{Kind: transport.CommandKindStop, Stop: &command}, result, err)
	e.rememberResult(result, err)
	return err
}

func (e *Engine) appendStartupLeadIn(
	ctx context.Context,
	runEpoch uint64,
	profile startupMotionProfile,
	reason string,
) error {
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		return err
	}
	command := transport.AppendPointsCommand{
		StreamID: profile.startupStreamID,
		Points: []transport.TimedPoint{
			{PositionPercent: profile.currentSemantic, TimeMillis: 0},
			{PositionPercent: profile.targetSemantic, TimeMillis: profile.leadInDuration.Milliseconds()},
			// HSP pause-on-starving can pause before the final segment has
			// physically settled. Keep stationary coverage after the target so
			// arrival polling observes an active position command, then Stop it
			// as soon as the measured slider arrives.
			{PositionPercent: profile.targetSemantic, TimeMillis: (profile.leadInDuration + startupTargetHold).Milliseconds()},
		},
	}
	result, err := e.transport.AppendPoints(commandCtx, command)
	cleanup()
	e.recordTransportResult(reason, nil, transport.Command{Kind: transport.CommandKindPointsAdd, PointsAdd: &command}, result, err)
	e.rememberResult(result, err)
	return err
}

func (e *Engine) playStartupLeadIn(
	ctx context.Context,
	runEpoch uint64,
	profile startupMotionProfile,
	reason string,
) error {
	e.commandMu.Lock()
	defer e.commandMu.Unlock()
	commandCtx, cleanup, err := e.contextForRun(ctx, runEpoch)
	if err != nil {
		return err
	}
	command := transport.PlayCommand{StreamID: profile.startupStreamID}
	result, err := e.transport.Play(commandCtx, command)
	cleanup()
	e.recordTransportResult(reason, nil, transport.Command{Kind: transport.CommandKindPointsPlay, PointsPlay: &command}, result, err)
	e.rememberResult(result, err)
	return err
}

func (e *Engine) waitForStartupLeadIn(ctx context.Context, duration time.Duration) error {
	wait := duration + startupArrivalSettle
	if provider, ok := e.transport.(transport.PlaybackStartTimeProvider); ok {
		if startedAt := provider.PlaybackStartTime(); !startedAt.IsZero() {
			wait = time.Until(startedAt.Add(duration + startupArrivalSettle))
		}
	}
	if wait < 0 {
		wait = 0
	}
	return e.startupWait(ctx, wait)
}

func waitForMotionStartup(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func startupLeadInDuration(deltaPercent float64, speedPercent int) time.Duration {
	speedPercent = max(1, min(100, speedPercent))
	millis := int64(math.Ceil(deltaPercent * 1000 / float64(speedPercent)))
	return max(startupMinimumLeadIn, time.Duration(millis)*time.Millisecond)
}

func physicalPositionForSemantic(position float64, window transport.StrokeWindowCommand) float64 {
	wirePosition := position
	if window.ReverseDirection {
		wirePosition = 100 - wirePosition
	}
	return float64(window.MinPercent) + wirePosition/100*float64(window.MaxPercent-window.MinPercent)
}

func semanticPositionForPhysical(position float64, window transport.StrokeWindowCommand) float64 {
	span := float64(window.MaxPercent - window.MinPercent)
	wirePosition := (position - float64(window.MinPercent)) / span * 100
	wirePosition = math.Max(0, math.Min(100, wirePosition))
	if window.ReverseDirection {
		return 100 - wirePosition
	}
	return wirePosition
}

func sameStrokeWindow(left, right transport.StrokeWindowCommand) bool {
	return left.MinPercent == right.MinPercent &&
		left.MaxPercent == right.MaxPercent &&
		left.ReverseDirection == right.ReverseDirection
}

type startupCalibration struct {
	originAbsolute     float64
	fullTravelAbsolute float64
}

func newStartupCalibration(state transport.MotionStartupState) (startupCalibration, error) {
	strokePercentSpan := (state.StrokeMaxPercent - state.StrokeMinPercent) / 100
	strokeAbsoluteSpan := state.StrokeMaxAbsolute - state.StrokeMinAbsolute
	if strokePercentSpan <= 0 || strokeAbsoluteSpan <= 0 {
		return startupCalibration{}, errors.New("motion startup could not calibrate physical slider travel")
	}
	fullTravel := strokeAbsoluteSpan / strokePercentSpan
	origin := state.StrokeMinAbsolute - state.StrokeMinPercent/100*fullTravel
	if math.IsNaN(fullTravel) || math.IsInf(fullTravel, 0) || fullTravel <= 0 ||
		math.IsNaN(origin) || math.IsInf(origin, 0) {
		return startupCalibration{}, errors.New("motion startup calculated invalid physical slider calibration")
	}
	return startupCalibration{originAbsolute: origin, fullTravelAbsolute: fullTravel}, nil
}

func (c startupCalibration) fullPercentAt(positionAbsolute float64) float64 {
	return (positionAbsolute - c.originAbsolute) / c.fullTravelAbsolute * 100
}

func (c startupCalibration) absoluteAt(fullPercent float64) float64 {
	return c.originAbsolute + fullPercent/100*c.fullTravelAbsolute
}

func validateStartupState(state transport.MotionStartupState) error {
	values := []float64{
		state.PositionWithinStrokePercent,
		state.PositionAbsolute,
		state.SpeedAbsolute,
		state.StrokeMinPercent,
		state.StrokeMaxPercent,
		state.StrokeMinAbsolute,
		state.StrokeMaxAbsolute,
	}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("motion startup state contained a non-finite value")
		}
	}
	if state.SpeedAbsolute < 0 {
		return errors.New("motion startup state contained an invalid slider value")
	}
	if state.StrokeMinPercent < 0 || state.StrokeMaxPercent > 100 || state.StrokeMinPercent >= state.StrokeMaxPercent {
		return errors.New("motion startup state contained an invalid stroke window")
	}
	if state.StrokeMinAbsolute >= state.StrokeMaxAbsolute {
		return errors.New("motion startup state contained an invalid absolute stroke window")
	}
	return nil
}

func startupStateAnnotation(state transport.MotionStartupState) string {
	return fmt.Sprintf(
		"position_within_stroke_percent=%.2f;position_absolute=%.2f;speed_absolute=%.2f;stroke_percent=%.2f..%.2f;stroke_absolute=%.2f..%.2f",
		state.PositionWithinStrokePercent,
		state.PositionAbsolute,
		state.SpeedAbsolute,
		state.StrokeMinPercent,
		state.StrokeMaxPercent,
		state.StrokeMinAbsolute,
		state.StrokeMaxAbsolute,
	)
}
