package motion

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

// Captured from the installed alpha.42 Cloud REST instance: a fractional seek
// target becomes HSP 16%, and the Handy Original settles at 20.00 mm. Checking
// the unencoded 16.458% target instead incorrectly reports a 1.10 mm miss.
func TestStartupAcceptsMeasuredArrivalAtEncodedCloudTarget(t *testing.T) {
	for _, test := range []struct {
		name    string
		initial float64
		plays   int
	}{
		{name: "lead_in", initial: 30.67, plays: 2},
		{name: "already_settled_retry", initial: 20, plays: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := &startupResolutionTransport{
				startupStateTransport: &startupStateTransport{
					Fake: transport.NewFake(),
					states: []transport.MotionStartupState{
						capturedStartupState(test.initial), capturedStartupState(20),
					},
				},
				capabilities: transport.MotionSamplingCapabilities{PositionResolutionPercent: 1},
			}
			engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
			engine.startupWait = func(context.Context, time.Duration) error { return nil }
			t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
			settings := config.DefaultSettings().Motion
			settings.SpeedMaxPercent = 54
			first := 16.458333333333336
			target := startupResolutionMediaTarget(first)
			if _, err := engine.Start(context.Background(), target, settings); err != nil {
				t.Fatalf("captured settled Cloud arrival: %v", err)
			}
			commands := owner.Commands()
			if got := countCommands(commands, transport.CommandKindPointsPlay); got != test.plays {
				t.Fatalf("Play count = %d, want %d", got, test.plays)
			}
			if test.plays == 2 {
				leadIn := findAppendForStreamSuffix(t, commands, "-startup")
				if math.Abs(leadIn.Points[1].PositionPercent-first) > 1e-10 {
					t.Fatalf("semantic lead-in was rounded before the owner: %+v", leadIn.Points)
				}
				if leadIn.Points[1].TimeMillis != 500 || leadIn.Points[2].TimeMillis != 10_500 {
					t.Fatalf("lead-in timing changed: %+v", leadIn.Points)
				}
			}
		})
	}
}

func TestStartupStillRejectsMissedEncodedTargets(t *testing.T) {
	for _, test := range []struct {
		name       string
		initial    float64
		arrival    float64
		stopped    float64
		speed      float64
		resolution float64
		stops      int
	}{
		{name: "below_tolerance", initial: 30.67, arrival: 19.5, resolution: 1, stops: 2},
		{name: "raw_target_close_but_encoded_target_missed", initial: 21.7, arrival: 21.7, resolution: 1, stops: 2},
		{name: "drift_after_stop", initial: 30.67, arrival: 20, stopped: 21.7, resolution: 1, stops: 3},
		{name: "still_moving", initial: 30.67, arrival: 20, speed: 6, resolution: 1, stops: 2},
		{name: "continuous_owner_keeps_exact_target", initial: 30.67, arrival: 20, stops: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			arrival := capturedStartupState(test.arrival)
			arrival.SpeedAbsolute = test.speed
			states := []transport.MotionStartupState{capturedStartupState(test.initial), arrival}
			if test.stopped != 0 {
				states = append(states, capturedStartupState(test.stopped))
			}
			owner := &startupResolutionTransport{
				startupStateTransport: &startupStateTransport{Fake: transport.NewFake(), states: states},
				capabilities:          transport.MotionSamplingCapabilities{PositionResolutionPercent: test.resolution},
			}
			engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
			engine.startupWait = func(context.Context, time.Duration) error { return nil }
			_, err := engine.Start(context.Background(), startupResolutionMediaTarget(16.458333333333336), config.DefaultSettings().Motion)
			if err == nil || !strings.Contains(err.Error(), "motion startup lead-in") {
				t.Fatalf("unsafe arrival error = %v", err)
			}
			if countCommands(owner.Commands(), transport.CommandKindPointsPlay) != 1 ||
				countCommands(owner.Commands(), transport.CommandKindStop) != test.stops {
				t.Fatalf("unsafe arrival did not stop before main playback: %+v", owner.Commands())
			}
			if engine.Snapshot().Running {
				t.Fatal("engine remained running after failed arrival")
			}
		})
	}
}

func TestStartupResolutionProjectionRespectsOwnerMappingOrder(t *testing.T) {
	for _, test := range []struct {
		name       string
		resolution float64
		after      bool
		reverse    bool
		min, max   int
		position   float64
		want       float64
	}{
		{name: "cloud", resolution: 1, max: 100, position: 16.458333333333336, want: 16},
		{name: "cloud_reverse_half_step", resolution: 1, reverse: true, max: 100, position: 16.5, want: 83},
		{name: "cloud_narrow", resolution: 1, min: 17, max: 80, position: 16.458333333333336, want: 27.08},
		{name: "cloud_narrow_reverse", resolution: 1, reverse: true, min: 17, max: 80, position: 16.5, want: 69.29},
		{name: "tenths", resolution: 0.1, max: 100, position: 16.458333333333336, want: 16.5},
		{name: "physical_steps", resolution: 1, after: true, min: 17, max: 80, position: 16.458333333333336, want: 27},
		{name: "physical_steps_reverse", resolution: 1, after: true, reverse: true, min: 17, max: 80, position: 16.5, want: 70},
		{name: "continuous", min: 17, max: 80, position: 16.5, want: 27.395},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := &Engine{positionResolutionPercent: test.resolution, resolutionAfterStrokeWindow: test.after}
			window := transport.StrokeWindowCommand{MinPercent: test.min, MaxPercent: test.max, ReverseDirection: test.reverse}
			if got := engine.startupCommandFullPercent(test.position, window); math.Abs(got-test.want) > 1e-10 {
				t.Fatalf("encoded physical position = %.12f, want %.12f", got, test.want)
			}
		})
	}
}

func TestStartupDistinguishesTemporaryAndFinalWindowTargets(t *testing.T) {
	for _, test := range []struct {
		name    string
		initial float64
		plays   int
	}{
		{name: "lead_in_needed_for_final_target", initial: 26.06, plays: 2},
		{name: "already_at_final_target", initial: 28.06, plays: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := &startupResolutionTransport{
				startupStateTransport: &startupStateTransport{
					Fake:   transport.NewFake(),
					states: []transport.MotionStartupState{capturedStartupState(5 + test.initial*0.9783), capturedStartupState(31.4141)},
				},
				capabilities: transport.MotionSamplingCapabilities{PositionResolutionPercent: 1},
			}
			traces := diagnostics.NewTraceRing(64)
			engine := newTestEngine(t, owner, traces, time.Hour)
			engine.startupWait = func(context.Context, time.Duration) error { return nil }
			t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
			settings := config.DefaultSettings().Motion
			settings.StrokeMinPercent, settings.StrokeMaxPercent = 17, 80
			if _, err := engine.Start(context.Background(), startupResolutionMediaTarget(16.458333333333336), settings); err != nil {
				t.Fatal(err)
			}
			if got := countCommands(owner.Commands(), transport.CommandKindPointsPlay); got != test.plays {
				t.Fatalf("Play count = %d, want %d", got, test.plays)
			}
			if test.plays == 2 {
				for _, row := range traces.Rows() {
					if row.Reason == "start_startup_points" && !strings.Contains(row.Annotation, "commanded_target_absolute=31.4141") {
						t.Fatalf("missing encoded temporary-window target: %s", row.Annotation)
					}
				}
			}
		})
	}
}

func capturedStartupState(position float64) transport.MotionStartupState {
	return transport.MotionStartupState{
		PositionWithinStrokePercent: (position - 5) / 97.83 * 100,
		PositionAbsolute:            position, StrokeMaxPercent: 100,
		StrokeMinAbsolute: 5, StrokeMaxAbsolute: 102.83,
	}
}

func startupResolutionMediaTarget(first float64) MotionTarget {
	return MotionTarget{
		Source: "media", SpeedPercent: 54,
		Media: &MediaTimelineDefinition{
			ID: "startup-seek", Name: "Startup seek", DurationMillis: 30_000,
			Points: []CurvePoint{{TimeMillis: 0, PositionPercent: first}, {TimeMillis: 30_000, PositionPercent: 70}},
		},
	}
}

type startupResolutionTransport struct {
	*startupStateTransport
	capabilities transport.MotionSamplingCapabilities
}

func (s *startupResolutionTransport) MotionSamplingCapabilities() transport.MotionSamplingCapabilities {
	return s.capabilities
}
