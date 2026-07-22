package motion

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEngineAnchorsCloudStyleStartupToMeasuredPosition(t *testing.T) {
	owner := &startupStateTransport{
		Fake: transport.NewFake(),
		states: []transport.MotionStartupState{
			{PositionWithinStrokePercent: 90, PositionAbsolute: 90, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
			{PositionWithinStrokePercent: 20, PositionAbsolute: 20, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
		},
	}
	traces := diagnostics.NewTraceRing(64)
	engine := newTestEngine(t, owner, traces, time.Hour)
	engine.startupWait = func(context.Context, time.Duration) error { return nil }
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 20
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80

	if _, err := engine.Start(context.Background(), MotionTarget{
		PatternID: PatternStroke, SpeedPercent: 20,
	}, settings); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	commands := owner.Commands()
	startupAdd := findAppendForStreamSuffix(t, commands, "-startup")
	if len(startupAdd.Points) != 2 {
		t.Fatalf("startup points = %+v, want measured anchor and target", startupAdd.Points)
	}
	if startupAdd.Points[0].PositionPercent != 90 || startupAdd.Points[0].TimeMillis != 0 {
		t.Fatalf("startup anchor = %+v, want measured 90%% at t=0", startupAdd.Points[0])
	}
	if startupAdd.Points[1].PositionPercent != 20 || startupAdd.Points[1].TimeMillis != 3500 {
		t.Fatalf("startup target = %+v, want 20%% reached over 3500ms at the 20%% cap", startupAdd.Points[1])
	}
	assertStartupCommandOrder(t, commands)
	if countCommands(commands, transport.CommandKindPointsPlay) != 2 {
		t.Fatalf("commands = %+v, want one lead-in Play and one main Play", commands)
	}
	if !hasTraceReason(traces.Rows(), "start_startup_state_stroke") ||
		!hasTraceReason(traces.Rows(), "start_startup_verify_stroke") {
		t.Fatalf("startup observations missing from trace: %+v", traces.Rows())
	}
}

func TestEngineSkipsLeadInWhenDeviceIsAlreadyAtFirstPoint(t *testing.T) {
	owner := &startupStateTransport{
		Fake: transport.NewFake(),
		states: []transport.MotionStartupState{
			{PositionWithinStrokePercent: 20, PositionAbsolute: 20, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
		},
	}
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(32), time.Hour)
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 20
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80

	if _, err := engine.Start(context.Background(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, settings); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	commands := owner.Commands()
	for _, command := range commands {
		if command.PointsAdd != nil && strings.HasSuffix(command.PointsAdd.StreamID, "-startup") {
			t.Fatalf("already-aligned device received a startup lead-in: %+v", command)
		}
	}
	if countCommands(commands, transport.CommandKindPointsPlay) != 1 {
		t.Fatalf("commands = %+v, want only the main Play", commands)
	}
}

func TestEngineUsesLeadInBeforeNarrowingWindowAroundNearbySlider(t *testing.T) {
	owner := &startupStateTransport{
		Fake: transport.NewFake(),
		states: []transport.MotionStartupState{
			{PositionWithinStrokePercent: 19, PositionAbsolute: 19, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
			{PositionWithinStrokePercent: 20, PositionAbsolute: 20, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
		},
	}
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(32), time.Hour)
	engine.startupWait = func(context.Context, time.Duration) error { return nil }
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 20
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80

	if _, err := engine.Start(context.Background(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, settings); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })

	startupAdd := findAppendForStreamSuffix(t, owner.Commands(), "-startup")
	if startupAdd.Points[0].PositionPercent != 19 || startupAdd.Points[1].PositionPercent != 20 {
		t.Fatalf("startup points = %+v, want a bounded 19%% to 20%% lead-in", startupAdd.Points)
	}
}

func TestEngineRejectsLeadInArrivalOutsideFinalWindow(t *testing.T) {
	owner := &startupStateTransport{
		Fake: transport.NewFake(),
		states: []transport.MotionStartupState{
			{PositionWithinStrokePercent: 0, PositionAbsolute: 0, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
			{PositionWithinStrokePercent: 19, PositionAbsolute: 19, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
		},
	}
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(32), time.Hour)
	engine.startupWait = func(context.Context, time.Duration) error { return nil }
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 20
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80

	_, err := engine.Start(context.Background(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, settings)
	if err == nil || !strings.Contains(err.Error(), "outside the requested stroke window") {
		t.Fatalf("Start error = %v, want fail-closed arrival rejection", err)
	}
	for _, command := range owner.Commands() {
		if command.PointsPlay != nil && !strings.HasSuffix(command.PointsPlay.StreamID, "-startup") {
			t.Fatalf("main stream played after rejected startup arrival: %+v", owner.Commands())
		}
	}
}

func TestStopCancelsStartupLeadInBeforeMainPlayback(t *testing.T) {
	owner := &startupStateTransport{
		Fake: transport.NewFake(),
		states: []transport.MotionStartupState{
			{PositionWithinStrokePercent: 90, PositionAbsolute: 90, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100},
		},
	}
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	waiting := make(chan struct{})
	var once sync.Once
	engine.startupWait = func(ctx context.Context, _ time.Duration) error {
		once.Do(func() { close(waiting) })
		<-ctx.Done()
		return ctx.Err()
	}
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 20
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80

	startDone := make(chan error, 1)
	go func() {
		_, err := engine.Start(context.Background(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, settings)
		startDone <- err
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("startup did not enter its cancellable lead-in wait")
	}
	if _, err := engine.Stop(context.Background(), "test_stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-startDone:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, errRunInvalidated) {
			t.Fatalf("Start error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start remained blocked after Stop")
	}

	mainPlays := 0
	for _, command := range owner.Commands() {
		if command.PointsPlay != nil && !strings.HasSuffix(command.PointsPlay.StreamID, "-startup") {
			mainPlays++
		}
	}
	if mainPlays != 0 {
		t.Fatalf("main stream played after startup cancellation: %+v", owner.Commands())
	}
}

func TestStartupLeadInDurationHonorsSpeedAsMaximumTravelRate(t *testing.T) {
	for _, test := range []struct {
		delta float64
		speed int
		want  time.Duration
	}{
		{delta: 100, speed: 20, want: 5 * time.Second},
		{delta: 40, speed: 40, want: time.Second},
		{delta: 2, speed: 100, want: startupMinimumLeadIn},
	} {
		if got := startupLeadInDuration(test.delta, test.speed); got != test.want {
			t.Fatalf("startupLeadInDuration(%.1f, %d) = %v, want %v", test.delta, test.speed, got, test.want)
		}
	}
}

func TestStartupCalibrationUsesAbsolutePositionInsteadOfWindowRelativePosition(t *testing.T) {
	state := transport.MotionStartupState{
		PositionWithinStrokePercent: 12.498569,
		PositionAbsolute:            29.333334,
		StrokeMinPercent:            17,
		StrokeMaxPercent:            80,
		StrokeMinAbsolute:           21.630419,
		StrokeMaxAbsolute:           83.260796,
	}
	calibration, err := newStartupCalibration(state)
	if err != nil {
		t.Fatalf("newStartupCalibration: %v", err)
	}
	fullPercent := calibration.fullPercentAt(state.PositionAbsolute)
	if fullPercent < 24.8 || fullPercent > 25.0 {
		t.Fatalf("full-travel position = %.4f%%, want about 24.9%%", fullPercent)
	}
	if fullPercent == state.PositionWithinStrokePercent {
		t.Fatal("window-relative slider position was mistaken for full-travel position")
	}
	targetAbsolute := calibration.absoluteAt(20)
	if targetAbsolute < 24.5 || targetAbsolute > 24.7 {
		t.Fatalf("20%% full-travel target = %.4fmm, want about 24.6mm", targetAbsolute)
	}
}

func TestStartupCalibrationAllowsSliderOutsideActiveStrokeWindow(t *testing.T) {
	state := transport.MotionStartupState{
		PositionWithinStrokePercent: -54.7983,
		PositionAbsolute:            5.333334,
		StrokeMinPercent:            25,
		StrokeMaxPercent:            70,
		StrokeMinAbsolute:           29.4565,
		StrokeMaxAbsolute:           73.478195,
	}
	if err := validateStartupState(state); err != nil {
		t.Fatalf("validateStartupState: %v", err)
	}
	calibration, err := newStartupCalibration(state)
	if err != nil {
		t.Fatalf("newStartupCalibration: %v", err)
	}
	fullPercent := calibration.fullPercentAt(state.PositionAbsolute)
	if fullPercent < 0.3 || fullPercent > 0.4 {
		t.Fatalf("full-travel position = %.4f%%, want about 0.34%%", fullPercent)
	}
	if targetAbsolute := calibration.absoluteAt(20); targetAbsolute < 24.5 || targetAbsolute > 24.7 {
		t.Fatalf("20%% full-travel target = %.4fmm, want about 24.6mm", targetAbsolute)
	}
}

func TestEngineRejectsStartupPositionOutsideCalibratedTravel(t *testing.T) {
	owner := &startupStateTransport{
		Fake: transport.NewFake(),
		states: []transport.MotionStartupState{{
			PositionWithinStrokePercent: -90,
			PositionAbsolute:            -10,
			StrokeMinPercent:            25,
			StrokeMaxPercent:            70,
			StrokeMinAbsolute:           29.4565,
			StrokeMaxAbsolute:           73.478195,
		}},
	}
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(32), time.Hour)
	settings := config.DefaultSettings().Motion

	_, err := engine.Start(context.Background(), MotionTarget{
		PatternID: PatternStroke, SpeedPercent: 20,
	}, settings)
	if err == nil || !strings.Contains(err.Error(), "outside calibrated full travel") {
		t.Fatalf("Start error = %v, want fail-closed absolute-position rejection", err)
	}
	if countCommands(owner.Commands(), transport.CommandKindPointsPlay) != 0 {
		t.Fatalf("commands = %+v, want no playback for invalid absolute geometry", owner.Commands())
	}
}

type startupStateTransport struct {
	*transport.Fake
	mu     sync.Mutex
	states []transport.MotionStartupState
	reads  int
}

func (s *startupStateTransport) ReadMotionStartupState(context.Context) (
	transport.MotionStartupState,
	transport.MotionStartupStateResults,
	error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.states) == 0 {
		return transport.MotionStartupState{}, transport.MotionStartupStateResults{}, errors.New("no startup state")
	}
	index := min(s.reads, len(s.states)-1)
	s.reads++
	result := func(kind transport.CommandKind) transport.CommandResult {
		return transport.CommandResult{Kind: kind, Transport: "startup_fake", OK: true, Status: "recorded"}
	}
	return s.states[index], transport.MotionStartupStateResults{
		Slider: result(transport.CommandKindSliderState),
		Stroke: result(transport.CommandKindStrokeWindowState),
	}, nil
}

func findAppendForStreamSuffix(t *testing.T, commands []transport.Command, suffix string) transport.AppendPointsCommand {
	t.Helper()
	for _, command := range commands {
		if command.PointsAdd != nil && strings.HasSuffix(command.PointsAdd.StreamID, suffix) {
			return *command.PointsAdd
		}
	}
	t.Fatalf("no append command ended with stream suffix %q: %+v", suffix, commands)
	return transport.AppendPointsCommand{}
}

func assertStartupCommandOrder(t *testing.T, commands []transport.Command) {
	t.Helper()
	want := []transport.CommandKind{
		transport.CommandKindStop,
		transport.CommandKindStrokeWindow,
		transport.CommandKindPointsAdd,
		transport.CommandKindPointsPlay,
		transport.CommandKindStop,
		transport.CommandKindStrokeWindow,
		transport.CommandKindPointsAdd,
	}
	if len(commands) < len(want) {
		t.Fatalf("commands = %+v, want at least %v", commands, want)
	}
	for index, kind := range want {
		if commands[index].Kind != kind {
			t.Fatalf("command %d = %s, want %s: %+v", index, commands[index].Kind, kind, commands)
		}
	}
}

func hasTraceReason(rows []diagnostics.MotionTraceRow, reason string) bool {
	for _, row := range rows {
		if row.Reason == reason {
			return true
		}
	}
	return false
}
