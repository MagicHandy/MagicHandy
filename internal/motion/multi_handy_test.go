package motion

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func namedStartupOwner(position float64) *startupStateTransport {
	state := func(p float64) transport.MotionStartupState {
		return transport.MotionStartupState{PositionWithinStrokePercent: p, PositionAbsolute: p, StrokeMinPercent: 0, StrokeMaxPercent: 100, StrokeMinAbsolute: 0, StrokeMaxAbsolute: 100}
	}
	return &startupStateTransport{Fake: transport.NewFake(), states: []transport.MotionStartupState{state(position), state(20), state(20)}}
}
func TestMultiHandyStartupUsesOwnTelemetryAndOneSharedStream(t *testing.T) {
	a, b := namedStartupOwner(90), namedStartupOwner(45)
	third := transport.NewFake()
	linked := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "a", Label: "Cloud A", Transport: a}, {ID: "b", Label: "Cloud B", Transport: b}, {ID: "c", Label: "BLE", Transport: third}}, false)
	engine := newTestEngine(t, linked, diagnostics.NewTraceRing(256), time.Hour)
	engine.startupWait = func(context.Context, time.Duration) error { return nil }
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 20
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80
	if _, err := engine.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, settings); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = engine.Stop(context.Background(), "cleanup") }()
	for _, item := range []struct {
		o        *startupStateTransport
		position float64
	}{{a, 90}, {b, 45}} {
		startup := findAppendForStreamSuffix(t, item.o.Commands(), "-startup")
		if startup.Points[0].PositionPercent != item.position || startup.Points[1].PositionPercent != 20 {
			t.Fatalf("wrong device lead-in: %+v", startup.Points)
		}
	}
	main := func(commands []transport.Command) []transport.TimedPoint {
		for _, c := range commands {
			if c.PointsAdd != nil && !strings.HasSuffix(c.PointsAdd.StreamID, "-startup") {
				return c.PointsAdd.Points
			}
		}
		return nil
	}
	if !reflect.DeepEqual(main(a.Commands()), main(b.Commands())) || !reflect.DeepEqual(main(a.Commands()), main(third.Commands())) || len(main(a.Commands())) == 0 {
		t.Fatal("participants received different semantic frames")
	}
	for _, c := range third.Commands() {
		if c.PointsAdd != nil && strings.HasSuffix(c.PointsAdd.StreamID, "-startup") {
			t.Fatal("another device's lead-in reached BLE")
		}
	}
	if os.Getenv("MAGICHANDY_MULTI_CAPTURE") != "" {
		data, err := json.MarshalIndent(map[string]any{"cloud_a": a.Commands(), "cloud_b": b.Commands(), "ble": third.Commands()}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("../../.scratch/multi-handy/startup-capture.json", data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestMultiHandyUnsafeCompanionPreventsMainPlaybackEverywhere(t *testing.T) {
	a, b := namedStartupOwner(90), namedStartupOwner(-30)
	linked := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "a", Label: "A", Transport: a}, {ID: "b", Label: "B", Transport: b}}, true)
	engine := newTestEngine(t, linked, diagnostics.NewTraceRing(128), time.Hour)
	engine.startupWait = func(context.Context, time.Duration) error { return nil }
	settings := config.DefaultSettings().Motion
	settings.SpeedMaxPercent = 20
	settings.StrokeMinPercent = 20
	settings.StrokeMaxPercent = 80
	if _, err := engine.Start(t.Context(), MotionTarget{PatternID: PatternStroke, SpeedPercent: 20}, settings); err == nil {
		t.Fatal("unsafe startup accepted")
	}
	if engine.Snapshot().Running {
		t.Fatal("failed startup left engine running")
	}
	for _, o := range []*startupStateTransport{a, b} {
		for _, c := range o.Commands() {
			if c.PointsPlay != nil && !strings.HasSuffix(c.PointsPlay.StreamID, "-startup") {
				t.Fatal("main playback escaped failed startup")
			}
		}
	}
}
