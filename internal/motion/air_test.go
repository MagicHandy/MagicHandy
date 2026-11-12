package motion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/transport"
)

func TestEngineAirSharesAdmissionAndNeverMovesTheIdleSlider(t *testing.T) {
	linear, tempo := transport.NewFake(), transport.NewFake(transport.WithTempo())
	owner := transport.NewLinkedSession([]transport.LinkedParticipant{{ID: "handy", Transport: linear}, {ID: "vacuglide", Transport: tempo}}, false)
	engine := newTestEngine(t, owner, diagnostics.NewTraceRing(64), time.Hour)
	t.Cleanup(func() { _, _ = engine.Stop(context.Background(), "cleanup") })
	target := MotionTarget{TempoOnly: true, Tempo: &TempoSpec{Mode: TempoModeSteady, LevelPercent: 40}}
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	admission := engine.AdmissionGeneration()
	before := len(tempo.Commands())
	if _, err := engine.ApplyAir(t.Context(), 1, admission); err != nil {
		t.Fatal(err)
	}
	commands := tempo.Commands()[before:]
	if len(commands) != 1 || commands[0].PointsAdd == nil || commands[0].PointsAdd.Air == nil || len(linear.Commands()) != 0 {
		t.Fatalf("air left its semantic output: %+v, linear=%d", commands, len(linear.Commands()))
	}
	if _, err := engine.Stop(t.Context(), "stop"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(t.Context(), target, config.DefaultSettings().Motion); err != nil {
		t.Fatal(err)
	}
	before = len(tempo.Commands())
	if _, err := engine.ApplyAir(t.Context(), 1, admission); !errors.Is(err, errRunInvalidated) {
		t.Fatalf("old air admission = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := engine.ApplyAir(ctx, 1, engine.AdmissionGeneration()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled air = %v", err)
	}
	if len(tempo.Commands()) != before {
		t.Fatal("stale/canceled air reached transport")
	}
}
