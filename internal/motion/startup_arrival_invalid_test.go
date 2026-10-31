package motion

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestInvalidArrivalTelemetryStopsWithRecoverableTypedFailure(t *testing.T) {
	for _, stage := range []string{"arrival", "stopped_verification"} {
		for _, invalid := range []string{"nonfinite_position", "negative_speed", "invalid_window"} {
			t.Run(stage+"/"+invalid, func(t *testing.T) {
				bad := resetTestPosition(0)
				switch invalid {
				case "nonfinite_position":
					bad.PositionAbsolute = math.NaN()
				case "negative_speed":
					bad.SpeedAbsolute = -1
				case "invalid_window":
					bad.StrokeMaxAbsolute = 0
				}
				states := []transport.MotionStartupState{resetTestPosition(50)}
				if stage == "stopped_verification" {
					states = append(states, resetTestPosition(0))
				}
				states = append(states, bad)
				owner := &startupStateTransport{Fake: transport.NewFake(), states: states}
				e := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
				e.startupWait = func(context.Context, time.Duration) error { return nil }
				state, err := e.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, config.DefaultSettings().Motion)
				if !errors.Is(err, ErrStartupArrival) || state.Running || state.StartupRecovery == nil || state.StartupRecovery.Kind != "invalid_telemetry" {
					t.Fatalf("unverifiable arrival must halt autonomous retry with invalid telemetry recovery: running=%v recovery=%+v err=%v", state.Running, state.StartupRecovery, err)
				}
				commands := owner.Commands()
				if countCommands(commands, transport.CommandKindPointsPlay) != 1 || commands[len(commands)-1].Kind != transport.CommandKindStop {
					t.Fatal("invalid arrival must stop after lead-in without starting semantic playback")
				}
			})
		}
	}
}
